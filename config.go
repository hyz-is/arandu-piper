package piper

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The values New applies when Config leaves a field at its zero value.
const (
	// DefaultTimeout bounds one Speak call, conversion included.
	DefaultTimeout = time.Minute
	// DefaultMaxChars is the longest text Speak accepts, in characters. At the
	// pace of a calm speaker it is about a minute and a half of audio.
	DefaultMaxChars = 1500
	// DefaultOpusBitrate is the bit rate, in bits per second, of the Opus audio
	// FormatOpus produces. 24 kbit/s is plenty for one voice.
	DefaultOpusBitrate = 24000
)

// Format is the audio format Speak returns.
type Format string

const (
	// FormatWAV returns what Piper writes: 16-bit mono PCM at the sample rate
	// of the voice (22 050 Hz for the medium voices). It is the default.
	FormatWAV Format = "wav"
	// FormatOpus returns Ogg Opus, mono, 16 kHz: the format of a WhatsApp
	// voice note. It needs ffmpeg, built with libopus.
	FormatOpus Format = "opus"
)

// MIME types of the Audio that Speak returns.
const (
	// MIMEWAV is the MIME type of FormatWAV audio.
	MIMEWAV = "audio/wav"
	// MIMEOpus is the MIME type of FormatOpus audio, the one WhatsApp expects
	// for a voice note.
	MIMEOpus = "audio/ogg; codecs=opus"
)

// Config is what the application passes when it builds the client. Every path
// is checked by New, so a wrong deployment fails at boot and not on the first
// voice note. This package never reads the environment: the application reads
// it and passes the values here.
type Config struct {
	// BinaryPath is the Piper command-line program: the "piper" script that
	// pip installs, or the standalone "piper" binary. An absolute path, a
	// relative one, or a bare name looked up in PATH. Required.
	BinaryPath string
	// Voices are the voices the client can speak with, by name. At least one
	// is required; every one is checked by New. VoicesIn builds the map when
	// the files keep their catalogue names in one directory.
	Voices map[Voice]VoiceFiles
	// DefaultVoice is the voice Speak uses. It may be left empty when Voices
	// holds a single voice, and must name one of Voices otherwise.
	DefaultVoice Voice
	// LengthScale is the pace: below 1 speaks faster and above 1 slower than
	// the voice's own. Zero leaves the voice's own pace, which is 1. The
	// accepted range is 0.25 to 4.
	LengthScale float64
	// NoiseScale is the generator noise (Piper's --noise_scale): how much the
	// voice varies from one phrase to the next. Lower is flatter and more
	// regular, higher is livelier and less stable. Zero leaves the voice's own
	// value (0.667 in the catalogue voices). The accepted range is above 0 to 2.
	NoiseScale float64
	// NoiseW is the phoneme width noise (Piper's --noise_w): how much the
	// length of each sound varies, which is most of what makes the rhythm
	// sound spoken and not metronomic. Zero leaves the voice's own value (0.8
	// in the catalogue voices). The accepted range is above 0 to 2.
	NoiseW float64
	// SentenceSilence is the pause after each sentence. Zero leaves Piper's own
	// default, 0.2 seconds.
	SentenceSilence time.Duration
	// Format is FormatWAV, or FormatOpus to get a WhatsApp voice note. Empty
	// means FormatWAV.
	Format Format
	// FFmpegPath is the ffmpeg program that converts to Opus. Empty means
	// "ffmpeg" in PATH. It is only looked for when Format is FormatOpus.
	FFmpegPath string
	// OpusBitrate is the bit rate of FormatOpus, in bits per second, between
	// 6000 and 128000. Zero means DefaultOpusBitrate.
	OpusBitrate int
	// Timeout bounds one Speak call: Piper and the conversion together. Zero
	// means DefaultTimeout. The first call after a restart is the slowest, as
	// the voice is loaded from disk.
	Timeout time.Duration
	// MaxChars is the longest text accepted, counted in characters; longer
	// text is refused with ErrTextTooLong before Piper runs. Zero means
	// DefaultMaxChars.
	MaxChars int
	// TempDir is the directory under which each call makes its own private
	// working directory, removed when the call returns. Empty means the
	// system's temporary directory.
	TempDir string
	// Env holds extra "KEY=VALUE" entries for the program, such as
	// "OMP_NUM_THREADS=2" to limit the CPU it uses. The program does not
	// inherit the application's environment: it gets PATH, LANG, PYTHONUTF8,
	// HOME and TMPDIR (the last two pointing at the call's private directory)
	// and these entries, so a secret the application holds is never handed to
	// it.
	Env []string
}

// Client runs Piper on the text it is given. It is safe for concurrent use;
// every call works in its own directory. Speech synthesis is CPU bound, so the
// application decides how many calls may run at once.
type Client struct {
	binary      string
	voices      map[Voice]voiceModel
	voice       Voice
	lengthScale float64
	noiseScale  float64
	noiseW      float64
	silence     time.Duration
	format      Format
	ffmpeg      string
	opusBitrate int
	timeout     time.Duration
	maxChars    int
	tempDir     string
	env         []string
}

var _ Speaker = (*Client)(nil)

// New validates cfg and returns a client. It refuses a program or voice that
// is missing or is not a regular file, a program that is not executable, no
// voice at all, a default voice that is not configured, a voice configuration
// that is not JSON, a speaker the voice does not have, an
// unknown format, a number out of its range (pace, noise, pauses, bit rate), and a TempDir that is not a
// directory. Nothing is started here: the programs are only looked at, not
// run.
func New(cfg Config) (*Client, error) {
	binary, err := resolveProgram(cfg.BinaryPath)
	if err != nil {
		return nil, fmt.Errorf("piper: BinaryPath: %w", err)
	}

	if len(cfg.Voices) == 0 {
		return nil, errors.New("piper: Voices: at least one voice is required")
	}
	voices := make(map[Voice]voiceModel, len(cfg.Voices))
	for _, v := range sortedVoices(cfg.Voices) {
		if strings.TrimSpace(string(v)) == "" {
			return nil, errors.New("piper: Voices: a voice has an empty name")
		}
		m, err := resolveVoice(v, cfg.Voices[v])
		if err != nil {
			return nil, err
		}
		voices[v] = m
	}
	voice := cfg.DefaultVoice
	if voice == "" {
		if len(voices) > 1 {
			return nil, fmt.Errorf("piper: DefaultVoice is required when Voices holds %d voices (%s)", len(voices), joinVoices(sortedVoices(voices)))
		}
		voice = sortedVoices(voices)[0]
	}
	if _, ok := voices[voice]; !ok {
		return nil, fmt.Errorf("piper: DefaultVoice %q is not one of Voices (%s)", voice, joinVoices(sortedVoices(voices)))
	}

	if cfg.LengthScale != 0 && (math.IsNaN(cfg.LengthScale) || cfg.LengthScale < 0.25 || cfg.LengthScale > 4) {
		return nil, fmt.Errorf("piper: LengthScale %v is outside 0.25 to 4 (zero keeps the voice's own pace)", cfg.LengthScale)
	}
	for _, n := range []struct {
		name  string
		value float64
	}{{"NoiseScale", cfg.NoiseScale}, {"NoiseW", cfg.NoiseW}} {
		if n.value != 0 && (math.IsNaN(n.value) || n.value < 0 || n.value > 2) {
			return nil, fmt.Errorf("piper: %s %v is outside 0 to 2 (zero keeps the voice's own value)", n.name, n.value)
		}
	}
	if cfg.SentenceSilence < 0 || cfg.SentenceSilence > 10*time.Second {
		return nil, errors.New("piper: SentenceSilence must be between zero and 10 seconds")
	}

	format := cfg.Format
	if format == "" {
		format = FormatWAV
	}
	var ffmpeg string
	switch format {
	case FormatWAV:
	case FormatOpus:
		name := cfg.FFmpegPath
		if name == "" {
			name = "ffmpeg"
		}
		ffmpeg, err = resolveProgram(name)
		if err != nil {
			return nil, fmt.Errorf("piper: FFmpegPath: %w", err)
		}
	default:
		return nil, fmt.Errorf("piper: Format %q is neither %q nor %q", cfg.Format, FormatWAV, FormatOpus)
	}
	if cfg.OpusBitrate != 0 && (cfg.OpusBitrate < 6000 || cfg.OpusBitrate > 128000) {
		return nil, errors.New("piper: OpusBitrate must be between 6000 and 128000")
	}

	if cfg.Timeout < 0 {
		return nil, errors.New("piper: Timeout must not be negative")
	}
	if cfg.MaxChars < 0 {
		return nil, errors.New("piper: MaxChars must not be negative")
	}

	tempDir := cfg.TempDir
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	tempDir, err = filepath.Abs(tempDir)
	if err != nil {
		return nil, fmt.Errorf("piper: TempDir: %w", err)
	}
	info, err := os.Stat(tempDir)
	if err != nil {
		return nil, fmt.Errorf("piper: TempDir: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("piper: TempDir %q is not a directory", tempDir)
	}

	for _, entry := range cfg.Env {
		if i := strings.IndexByte(entry, '='); i < 1 || strings.ContainsRune(entry, 0) {
			return nil, fmt.Errorf("piper: Env entry %q is not KEY=VALUE", entry)
		}
	}

	c := &Client{
		binary:      binary,
		voices:      voices,
		voice:       voice,
		lengthScale: cfg.LengthScale,
		noiseScale:  cfg.NoiseScale,
		noiseW:      cfg.NoiseW,
		silence:     cfg.SentenceSilence,
		format:      format,
		ffmpeg:      ffmpeg,
		opusBitrate: cfg.OpusBitrate,
		timeout:     cfg.Timeout,
		maxChars:    cfg.MaxChars,
		tempDir:     tempDir,
		env:         append([]string(nil), cfg.Env...),
	}
	if c.opusBitrate == 0 {
		c.opusBitrate = DefaultOpusBitrate
	}
	if c.timeout == 0 {
		c.timeout = DefaultTimeout
	}
	if c.maxChars == 0 {
		c.maxChars = DefaultMaxChars
	}
	return c, nil
}

// Voices lists the voices the client speaks with, sorted by name.
func (c *Client) Voices() []Voice { return sortedVoices(c.voices) }

// DefaultVoice is the voice Speak uses.
func (c *Client) DefaultVoice() Voice { return c.voice }

func joinVoices(vs []Voice) string {
	names := make([]string, len(vs))
	for i, v := range vs {
		names[i] = string(v)
	}
	return strings.Join(names, ", ")
}

// readVoice reads the part of a voice's JSON that New checks: that it is a
// JSON object and how many speakers the voice has (zero when it does not say).
func readVoice(path string) (speakers int, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("%q cannot be read", path)
	}
	var doc struct {
		NumSpeakers int `json:"num_speakers"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return 0, fmt.Errorf("%q is not a voice configuration (JSON): %v", path, shortJSONError(err))
	}
	return doc.NumSpeakers, nil
}

func shortJSONError(err error) string {
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		return fmt.Sprintf("syntax error at byte %d", syntax.Offset)
	}
	return "unexpected structure"
}

// resolveProgram turns a path or a bare name into the absolute path of an
// executable regular file.
func resolveProgram(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", errors.New("is required")
	}
	if !strings.ContainsRune(name, filepath.Separator) {
		found, err := exec.LookPath(name)
		if err != nil {
			return "", fmt.Errorf("%q was not found in PATH", name)
		}
		name = found
	}
	path, err := resolveFile(name)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("%q is not executable", path)
	}
	return path, nil
}

// resolveFile returns the absolute path of a non-empty regular file. A symbolic
// link is followed, so a "piper" script in a virtual environment's bin
// directory is accepted.
func resolveFile(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", errors.New("is required")
	}
	path, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("%q cannot be read: %w", path, errors.Unwrap(err))
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%q is not a regular file", path)
	}
	if info.Size() == 0 {
		return "", fmt.Errorf("%q is empty", path)
	}
	return path, nil
}
