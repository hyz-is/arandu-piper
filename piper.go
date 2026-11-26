// Package piper turns text into speech by running a voice engine on the
// application's own server: no text leaves the machine and there is no
// per-minute cost. The engines are Piper, and Kokoro-82M for the voices that
// belong to it (VoiceDora, VoiceAlex, VoiceSanta): a voice belongs to one
// engine, and the name in each call decides which one speaks.
//
// It is a thin layer over a program the application installs, Piper
// (https://github.com/OHF-Voice/piper1-gpl) or Kokoro's kokoro-speak
// (docs/kokoro-speak.py), and optionally ffmpeg, which turns
// the WAV that Piper writes into the Ogg Opus of a WhatsApp voice note. Each
// program is started with fixed arguments through os/exec, never through a
// shell, inside a private temporary directory that is removed before the call
// returns, and Piper reads the text from its standard input, so the text never
// appears in an argument list. The package adds what an application otherwise
// writes by hand every time: a configuration that is refused at boot rather
// than on the first voice note, a limit on the size of the text and a timeout
// that also kills the process, errors that never carry what was said, Plain,
// which strips a chat message's formatting so it is not read aloud, and an
// interface with a Fake so the code that speaks is tested without the program
// or a voice.
//
// It owns no table, no route and no policy, and it does not import the
// framework, so an application on any framework version installs it. It
// executes Piper and does not link it: Piper's license applies to Piper, which
// the application installs, and not to this package.
package piper

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Audio is one recording: its bytes, the MIME type that describes them and
// how long it lasts.
type Audio struct {
	// Data is the whole file.
	Data []byte
	// MIMEType is MIMEWAV or MIMEOpus.
	MIMEType string
	// Duration is the length of the speech.
	Duration time.Duration
}

// Speaker is what turns text into speech. *Client implements it with Piper,
// *Fake in memory and *Cache over another Speaker, so the code that depends on
// it is tested without the program, the voice or the CPU time.
type Speaker interface {
	// Speak says text with the default voice.
	Speak(ctx context.Context, text string) (Audio, error)
	// SpeakAs says text with the voice given; an empty voice is the default
	// one. A voice the speaker does not have is ErrUnknownVoice.
	SpeakAs(ctx context.Context, voice Voice, text string) (Audio, error)
}

// The errors Speak returns. Test for them with errors.Is.
var (
	// ErrEmptyText means there was nothing to say: the text is blank, or holds
	// no letter and no digit.
	ErrEmptyText = errors.New("piper: the text is empty")
	// ErrTextTooLong means the text is longer than Config.MaxChars.
	ErrTextTooLong = errors.New("piper: the text is too long")
	// ErrTimeout means Config.Timeout, or the caller's deadline, elapsed. The
	// running program was killed. It also satisfies
	// errors.Is(err, context.DeadlineExceeded).
	ErrTimeout = errors.New("piper: speech synthesis timed out")
	// ErrFailed means Piper (or kokoro-speak, for a Kokoro voice) could not run
	// or exited with an error. The error is a *ProcessError carrying a short
	// summary.
	ErrFailed = errors.New("piper: piper failed")
	// ErrBadOutput means Piper exited successfully but wrote no audio this
	// package can read.
	ErrBadOutput = errors.New("piper: piper wrote no readable audio")
	// ErrConvertFailed means ffmpeg could not make the Opus audio. The error
	// is a *ProcessError carrying a short summary.
	ErrConvertFailed = errors.New("piper: the conversion to Opus failed")
)

// ProcessError says that one of the programs failed. Its text is built from
// the exit status and a short summary of the program's standard error, with
// the working directory replaced by "<tmp>" and the text that was being
// spoken replaced by "<text>". The program's standard output is never
// captured, so a ProcessError cannot leak the audio.
type ProcessError struct {
	// Program is the base name of the program that failed.
	Program string
	// ExitCode is the exit status, or -1 when the program did not start or
	// was ended by a signal.
	ExitCode int
	// Stderr is the summarized standard error.
	Stderr string

	kind error
}

// Error describes the failure.
func (e *ProcessError) Error() string {
	msg := fmt.Sprintf("%v: %s exited with status %d", e.kind, e.Program, e.ExitCode)
	if e.ExitCode < 0 {
		msg = fmt.Sprintf("%v: %s did not finish", e.kind, e.Program)
	}
	if e.Stderr != "" {
		msg += ": " + e.Stderr
	}
	return msg
}

// Unwrap returns ErrFailed for Piper and ErrConvertFailed for ffmpeg.
func (e *ProcessError) Unwrap() error { return e.kind }

// Speak is SpeakAs with the default voice.
func (c *Client) Speak(ctx context.Context, text string) (Audio, error) {
	return c.SpeakAs(ctx, "", text)
}

// SpeakAs makes speech of text with voice, or with the default voice when
// voice is empty, and returns it as Config.Format says. The text is
// cleaned of control characters and its lines are joined into one, so a
// message with several paragraphs is read straight through. Unless
// Config.Speech.Raw is set the text is first made speakable (see Speakable):
// numbers, money, dates and symbols in words, formatting and emoji removed. Speak refuses text longer
// than MaxChars before running anything, and ends all work, killing the
// running program, when ctx is done or Timeout elapses.
func (c *Client) SpeakAs(ctx context.Context, voice Voice, text string) (Audio, error) {
	v, spoken, err := c.prepare(voice, text)
	if err != nil {
		return Audio{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	dir, err := os.MkdirTemp(c.tempDir, "arandu-piper-")
	if err != nil {
		return Audio{}, fmt.Errorf("piper: making the working directory: %w", err)
	}
	defer os.RemoveAll(dir)

	wav := filepath.Join(dir, "speech.wav")
	if v.engine == EngineKokoro {
		if voice == "" {
			voice = c.voice
		}
		err = c.synthesizeKokoro(ctx, dir, voice, spoken, text, wav)
	} else {
		err = c.synthesize(ctx, dir, v, spoken, text, wav)
	}
	if err != nil {
		return Audio{}, err
	}
	duration, err := wavDuration(wav)
	if err != nil {
		return Audio{}, err
	}

	if c.format == FormatOpus {
		ogg := filepath.Join(dir, "speech.ogg")
		if err := c.convert(ctx, dir, spoken, text, wav, ogg); err != nil {
			return Audio{}, err
		}
		data, err := readOutput(ogg)
		if err != nil {
			return Audio{}, err
		}
		return Audio{Data: data, MIMEType: MIMEOpus, Duration: duration}, nil
	}

	data, err := readOutput(wav)
	if err != nil {
		return Audio{}, err
	}
	return Audio{Data: data, MIMEType: MIMEWAV, Duration: duration}, nil
}

// prepare picks the voice and cleans the text, refusing what SpeakAs and Key
// refuse before anything runs.
func (c *Client) prepare(voice Voice, text string) (voiceModel, string, error) {
	if voice == "" {
		voice = c.voice
	}
	v, ok := c.voices[voice]
	if !ok {
		return voiceModel{}, "", fmt.Errorf("%w: %q is not one of %s", ErrUnknownVoice, voice, joinVoices(c.Voices()))
	}
	spoken := normalize(text)
	if !speakable(spoken) {
		return voiceModel{}, "", ErrEmptyText
	}
	// The limit is on the text that was given, and is checked before the rules
	// run: a number in words is longer than the number.
	if n := len([]rune(spoken)); n > c.maxChars {
		return voiceModel{}, "", fmt.Errorf("%w: %d characters, the limit is %d", ErrTextTooLong, n, c.maxChars)
	}
	if c.speech.raw {
		return v, spoken, nil
	}
	spoken = c.speech.Speakable(text, time.Time{})
	if spoken == "" {
		return voiceModel{}, "", ErrEmptyText
	}
	if n := len([]rune(spoken)); n > c.maxChars*speechGrowth {
		return voiceModel{}, "", fmt.Errorf("%w: %d characters once spoken, the limit is %d", ErrTextTooLong, n, c.maxChars*speechGrowth)
	}
	return v, spoken, nil
}

// speechGrowth is how many times longer than MaxChars the spoken text may be:
// "R$ 1.234.567,89" is five times as long in words.
const speechGrowth = 4

// Speakable is the text SpeakAs would say for text: what Config.Speech makes of
// it, with dates in full. It is for an application that stores the text it
// spoke beside the text it was given, to audit a voice note; it never fails and
// is empty when nothing is left to say. NormalizerVersion says which rules made
// it.
func (c *Client) Speakable(text string) string {
	return c.speech.Speakable(text, time.Time{})
}

// Normalizer is the verbalizer the client speaks through, for an application
// that wants to say dates relative to a moment of its own
// (Normalizer().Speakable(text, now)) and then speak the result with Speech.Raw
// set, or with this client, which leaves it as it is.
func (c *Client) Normalizer() *Normalizer { return c.speech }

// NormalizerVersion is the version of the rules the client speaks through: put
// it beside the spoken text you store, and in the key of a cache that is not
// made with Key.
func (c *Client) NormalizerVersion() string { return c.speech.Version() }

// Key names the audio SpeakAs would make of text with voice (the default
// voice when empty), without making it: two calls whose audio would be the
// same have the same key, and any change that would change the audio changes
// the key -- the voice and its files, the speaker, the pace, the noise, the pauses, the
// format and its bit rate, and the text once its spacing and line breaks are
// cleaned as SpeakAs cleans them. Case and punctuation are kept, as they
// change how the voice reads. The key is 64 lowercase hexadecimal characters,
// safe as a file or object name; it is a hash, so the text cannot be read
// back from it. It refuses what SpeakAs refuses, with the same errors.
func (c *Client) Key(voice Voice, text string) (string, error) {
	v, spoken, err := c.prepare(voice, text)
	if err != nil {
		return "", err
	}
	if voice == "" {
		voice = c.voice
	}
	h := sha256.New()
	if v.engine == EngineKokoro {
		// Kokoro has no speaker number and no noise settings: only what reaches
		// its command line changes the audio.
		fmt.Fprintf(h, "arandu-piper/kokoro/1\x00%s\x00%s\x00%v\x00%d\x00%s\x00%d\x00",
			voice, v.identity, c.lengthScale, c.silence, c.format, c.opusBitrate)
		io.WriteString(h, spoken)
		return hex.EncodeToString(h.Sum(nil)), nil
	}
	fmt.Fprintf(h, "arandu-piper/1\x00%s\x00%s\x00%d\x00%v\x00%d\x00%s\x00%d\x00",
		voice, v.identity, v.speaker, c.lengthScale, c.silence, c.format, c.opusBitrate)
	// The noise is hashed only when set, so a client that leaves it at the
	// voice's own value keeps the keys, and the audio stored under them, that
	// v0.1.0 made.
	if c.noiseScale != 0 || c.noiseW != 0 {
		fmt.Fprintf(h, "noise\x00%v\x00%v\x00", c.noiseScale, c.noiseW)
	}
	io.WriteString(h, spoken)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// maxOutputBytes is the largest audio file read back. A limit on the text
// already bounds the audio, so this is only a guard against a program that
// writes without end.
const maxOutputBytes = 128 << 20

// synthesize runs Piper. The text goes in through standard input, one line, as
// the program reads one utterance per line. The arguments are the ones the
// standalone Piper and the pip-installed one both accept.
func (c *Client) synthesize(ctx context.Context, dir string, v voiceModel, spoken, original, out string) error {
	args := []string{"-m", v.model, "-c", v.config, "-f", out}
	if v.speaker > 0 {
		args = append(args, "-s", strconv.Itoa(v.speaker))
	}
	if c.lengthScale != 0 {
		args = append(args, "--length_scale", strconv.FormatFloat(c.lengthScale, 'f', -1, 64))
	}
	if c.noiseScale != 0 {
		args = append(args, "--noise_scale", strconv.FormatFloat(c.noiseScale, 'f', -1, 64))
	}
	if c.noiseW != 0 {
		args = append(args, "--noise_w", strconv.FormatFloat(c.noiseW, 'f', -1, 64))
	}
	if c.silence > 0 {
		args = append(args, "--sentence_silence", strconv.FormatFloat(c.silence.Seconds(), 'f', -1, 64))
	}
	return c.run(ctx, dir, c.binary, ErrFailed, nil, strings.NewReader(spoken+"\n"), []string{spoken, original}, args...)
}

// convert writes a mono 16 kHz Ogg Opus file, the format of a WhatsApp voice
// note. The voip application tuning favours the intelligibility of speech.
func (c *Client) convert(ctx context.Context, dir, spoken, original, in, out string) error {
	return c.run(ctx, dir, c.ffmpeg, ErrConvertFailed, nil, nil, []string{spoken, original},
		"-hide_banner",
		"-loglevel", "error",
		"-nostdin",
		"-y",
		"-i", in,
		"-vn",
		"-ac", "1",
		"-ar", "16000",
		"-c:a", "libopus",
		"-b:a", strconv.Itoa(c.opusBitrate),
		"-application", "voip",
		"-f", "ogg",
		out,
	)
}

// run starts one program without a shell and waits for it. Standard output is
// discarded; standard error is kept only to summarize a failure. extraEnv goes
// after Config.Env.
func (c *Client) run(ctx context.Context, dir, program string, kind error, extraEnv []string, stdin io.Reader, secrets []string, args ...string) error {
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Dir = dir
	cmd.Env = append(c.environment(dir), extraEnv...)
	cmd.Stdin = stdin
	cmd.Stdout = io.Discard
	cmd.WaitDelay = 2 * time.Second
	stderr := &tailBuffer{max: 4096}
	cmd.Stderr = stderr

	err := cmd.Run()
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			return fmt.Errorf("%w: %w", ErrTimeout, ctxErr)
		}
		return ctxErr
	}
	perr := &ProcessError{
		Program:  filepath.Base(program),
		ExitCode: -1,
		Stderr:   summarize(stderr.String(), dir, secrets),
		kind:     kind,
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		perr.ExitCode = exit.ExitCode()
	} else {
		perr.Stderr = summarize(err.Error(), dir, secrets)
	}
	return perr
}

// environment is what the program gets: a minimum, never the application's
// own variables, plus Config.Env. The caller's PATH stays, because a script
// installed by pip finds its interpreter through it on some systems.
func (c *Client) environment(dir string) []string {
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"LANG=C.UTF-8",
		"PYTHONUTF8=1",
		"HOME=" + dir,
		"TMPDIR=" + dir,
	}
	return append(env, c.env...)
}

// normalize makes the one line Piper reads: control characters become spaces
// and runs of white space become one.
func normalize(text string) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			return ' '
		}
		return r
	}, text)
	return strings.Join(strings.Fields(text), " ")
}

// speakable reports whether text has anything a voice can say.
func speakable(text string) bool {
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// readOutput reads a file the program wrote, refusing one that is missing,
// empty or absurdly large.
func readOutput(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > maxOutputBytes {
		return nil, ErrBadOutput
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrBadOutput
	}
	return data, nil
}

// wavDuration reads the length of the WAV Piper wrote from its "fmt " and
// "data" chunks, so it holds for any sample rate a voice has.
func wavDuration(path string) (time.Duration, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, ErrBadOutput
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() > maxOutputBytes {
		return 0, ErrBadOutput
	}

	var header [12]byte
	if _, err := io.ReadFull(f, header[:]); err != nil || string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return 0, ErrBadOutput
	}
	var bytesPerSecond int64
	offset := int64(12)
	for {
		var chunk [8]byte
		if _, err := io.ReadFull(f, chunk[:]); err != nil {
			return 0, ErrBadOutput
		}
		size := int64(binary.LittleEndian.Uint32(chunk[4:8]))
		offset += 8
		switch string(chunk[0:4]) {
		case "fmt ":
			if size < 16 {
				return 0, ErrBadOutput
			}
			var body [16]byte
			if _, err := io.ReadFull(f, body[:]); err != nil {
				return 0, ErrBadOutput
			}
			bytesPerSecond = int64(binary.LittleEndian.Uint32(body[8:12]))
			if bytesPerSecond <= 0 {
				return 0, ErrBadOutput
			}
		case "data":
			if bytesPerSecond == 0 {
				return 0, ErrBadOutput
			}
			// A writer that could not seek back leaves the size unset.
			if remaining := info.Size() - offset; size == 0xFFFFFFFF || size > remaining {
				size = remaining
			}
			if size <= 0 {
				return 0, ErrBadOutput
			}
			return time.Duration(size) * time.Second / time.Duration(bytesPerSecond), nil
		}
		offset += size + size%2
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return 0, ErrBadOutput
		}
	}
}

// summarize reduces a program's standard error to one short line: the working
// directory is replaced, a line that repeats what was being spoken is replaced
// by "<text>", white space is collapsed and the length is capped. A line counts
// as repeating the text when it holds three words of it in a row (the whole
// text when it has fewer), so a program that echoes a sentence it was given, or
// a line of source code that holds it, does not put it in an error.
func summarize(text, dir string, spoken []string) string {
	text = strings.ReplaceAll(text, dir, "<tmp>")
	echo := newEcho(spoken)
	var kept []string
	for _, line := range strings.Split(text, "\n") {
		if echo.repeats(line) {
			line = "<text>"
		}
		kept = append(kept, line)
	}
	text = strings.Join(strings.Fields(strings.Join(kept, " ")), " ")
	const max = 300
	if r := []rune(text); len(r) > max {
		text = string(r[:max]) + "..."
	}
	return text
}

// gramSize is how many words in a row make a line "repeat" the text.
const gramSize = 3

// echo recognizes lines that repeat the text that was spoken.
type echo struct {
	grams map[string]bool
	sizes map[int]bool
}

func newEcho(texts []string) echo {
	e := echo{grams: map[string]bool{}, sizes: map[int]bool{}}
	for _, text := range texts {
		w := words(text)
		n := min(gramSize, len(w))
		for i := 0; n > 0 && i+n <= len(w); i++ {
			e.grams[strings.Join(w[i:i+n], " ")] = true
			e.sizes[n] = true
		}
	}
	return e
}

func (e echo) repeats(line string) bool {
	w := words(line)
	for n := range e.sizes {
		for i := 0; i+n <= len(w); i++ {
			if e.grams[strings.Join(w[i:i+n], " ")] {
				return true
			}
		}
	}
	return false
}

func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	max int
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string { return string(bytes.ToValidUTF8(t.buf, nil)) }
