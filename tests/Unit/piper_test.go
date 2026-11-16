package unit_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hyz-is/arandu-piper"
)

// rig is a set of stand-in programs: shell scripts that log their arguments
// and write canned files. They exercise the real os/exec path without Piper,
// ffmpeg or a voice.
type rig struct {
	dir     string // holds the scripts and the logs
	tempDir string // the client's TempDir, which must be empty after a call
	cfg     piper.Config
	// voice is the cadu voice's files; config puts it in cfg.Voices unless a
	// test set Voices itself.
	voice piper.VoiceFiles
}

// config is cfg with the rig's voice.
func (r *rig) config() piper.Config {
	cfg := r.cfg
	if cfg.Voices == nil {
		cfg.Voices = map[piper.Voice]piper.VoiceFiles{piper.VoiceCadu: r.voice}
	}
	return cfg
}

const pronto = "Pronto, seu Antônio. Anotei mil e duzentos reais de adubo no talhão três. Está certo?"

// newRig writes the stand-ins. piperBody is the shell that runs inside the
// fake piper after it has logged its arguments, saved its standard input and
// found the -f value in $out; "@OK@" in it stands for copying the canned WAV
// to $out, as piper does.
func newRig(t *testing.T, piperBody string) *rig {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in programs are POSIX shell scripts")
	}
	dir := t.TempDir()
	r := &rig{dir: dir, tempDir: filepath.Join(dir, "tmp")}
	if err := os.Mkdir(r.tempDir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Two seconds of 22 050 Hz mono 16-bit silence, as a medium voice writes it.
	if err := os.WriteFile(filepath.Join(dir, "canned.wav"), wav(22050, 2*time.Second), 0o600); err != nil {
		t.Fatal(err)
	}

	piperBody = strings.ReplaceAll(piperBody, "@OK@", fmt.Sprintf(`cp %s/canned.wav "$out"`, dir))
	script(t, dir, "piper", fmt.Sprintf(`
echo "piper $*" >> %[1]s/calls.log
cat > %[1]s/stdin.txt
env > %[1]s/env.txt
pwd > %[1]s/pwd.txt
while [ $# -gt 0 ]; do
  if [ "$1" = "-f" ]; then out=$2; fi
  shift
done
%[2]s
`, dir, piperBody))
	script(t, dir, "ffmpeg", fmt.Sprintf(`
echo "ffmpeg $*" >> %[1]s/calls.log
for a; do out=$a; done
printf 'OggS stand-in opus' > "$out"
`, dir))

	model := filepath.Join(dir, "pt_BR-cadu-medium.onnx")
	if err := os.WriteFile(model, []byte("not a real voice"), 0o600); err != nil {
		t.Fatal(err)
	}
	voice := `{"audio": {"sample_rate": 22050}, "num_speakers": 1, "piper_version": "1.5.0"}`
	if err := os.WriteFile(model+".json", []byte(voice), 0o600); err != nil {
		t.Fatal(err)
	}
	r.cfg = piper.Config{
		BinaryPath: filepath.Join(dir, "piper"),
		TempDir:    r.tempDir,
	}
	r.voice = piper.VoiceFiles{ModelPath: model}
	return r
}

// okPiper copies the canned WAV to the output path, as piper does.
func (r *rig) okPiper() string {
	return fmt.Sprintf(`cp %s/canned.wav "$out"`, r.dir)
}

func script(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// wav is a mono 16-bit PCM WAV of the given rate and length.
func wav(rate int, d time.Duration) []byte {
	n := int(d.Seconds()*float64(rate)) * 2
	var b bytes.Buffer
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(36+n))
	b.WriteString("WAVEfmt ")
	for _, v := range []any{uint32(16), uint16(1), uint16(1), uint32(rate), uint32(rate * 2), uint16(2), uint16(16)} {
		_ = binary.Write(&b, binary.LittleEndian, v)
	}
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, uint32(n))
	b.Write(make([]byte, n))
	return b.Bytes()
}

func (r *rig) client(t *testing.T) *piper.Client {
	t.Helper()
	c, err := piper.New(r.config())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func (r *rig) read(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(r.dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func (r *rig) calls(t *testing.T) []string {
	t.Helper()
	raw := strings.TrimSpace(r.read(t, "calls.log"))
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

func (r *rig) call(t *testing.T, program string) string {
	t.Helper()
	for _, line := range r.calls(t) {
		if strings.HasPrefix(line, program+" ") {
			return line
		}
	}
	t.Fatalf("%s was never called; calls: %q", program, r.calls(t))
	return ""
}

func (r *rig) assertTempDirEmpty(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir(r.tempDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("the working directory was not removed: %v", entries)
	}
}

const neverRuns = "exit 99"

func TestSpeakReadsTheTextFromStdinAndCleansUp(t *testing.T) {
	r := newRig(t, "")
	script(t, r.dir, "piper", fmt.Sprintf(`
echo "piper $*" >> %[1]s/calls.log
cat > %[1]s/stdin.txt
while [ $# -gt 0 ]; do
  if [ "$1" = "-f" ]; then out=$2; fi
  shift
done
echo "AUDIO ON STDOUT"
%[2]s
`, r.dir, r.okPiper()))

	got, err := r.client(t).Speak(context.Background(), pronto)
	if err != nil {
		t.Fatalf("Speak: %v", err)
	}
	if got.MIMEType != "audio/wav" {
		t.Errorf("MIMEType = %q", got.MIMEType)
	}
	if got.Duration != 2*time.Second {
		t.Errorf("Duration = %v", got.Duration)
	}
	if !bytes.Equal(got.Data, wav(22050, 2*time.Second)) {
		t.Errorf("Data is not what piper wrote (%d bytes)", len(got.Data))
	}

	cli := r.call(t, "piper")
	for _, arg := range []string{"-m " + r.voice.ModelPath, "-c " + r.voice.ModelPath + ".json", "-f ", "/speech.wav"} {
		if !strings.Contains(cli, arg) {
			t.Errorf("piper call lacks %q: %q", arg, cli)
		}
	}
	for _, arg := range []string{" -s ", "--length_scale", "--sentence_silence", "-q"} {
		if strings.Contains(cli, arg) {
			t.Errorf("piper call has the optional argument %q by default: %q", arg, cli)
		}
	}
	if strings.Contains(cli, "Antônio") || strings.Contains(cli, "adubo") {
		t.Errorf("the text is in the argument list: %q", cli)
	}
	if stdin := r.read(t, "stdin.txt"); stdin != pronto+"\n" {
		t.Errorf("stdin = %q", stdin)
	}
	if len(r.calls(t)) != 1 {
		t.Errorf("only piper should have run: %q", r.calls(t))
	}
	r.assertTempDirEmpty(t)
}

func TestSpeakPassesTheOptionalSettings(t *testing.T) {
	r := newRig(t, "")
	script(t, r.dir, "piper", fmt.Sprintf("echo \"piper $*\" >> %s/calls.log\ncat >/dev/null\nwhile [ $# -gt 0 ]; do [ \"$1\" = \"-f\" ] && out=$2; shift; done\n%s\n", r.dir, r.okPiper()))
	r.voice.SpeakerID = 1
	r.cfg.LengthScale = 1.25
	r.cfg.NoiseScale = 0.75
	r.cfg.NoiseW = 0.9
	r.cfg.SentenceSilence = 350 * time.Millisecond
	if err := os.WriteFile(r.voice.ModelPath+".json", []byte(`{"num_speakers": 2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.client(t).Speak(context.Background(), pronto); err != nil {
		t.Fatal(err)
	}
	cli := r.call(t, "piper")
	for _, arg := range []string{" -s 1", "--length_scale 1.25", "--noise_scale 0.75", "--noise_w 0.9", "--sentence_silence 0.35"} {
		if !strings.Contains(cli, arg) {
			t.Errorf("piper call lacks %q: %q", arg, cli)
		}
	}
}

func TestSpeakLeavesTheNoiseToTheVoiceWhenItIsZero(t *testing.T) {
	r := newRig(t, "")
	script(t, r.dir, "piper", fmt.Sprintf("echo \"piper $*\" >> %s/calls.log\ncat >/dev/null\nwhile [ $# -gt 0 ]; do [ \"$1\" = \"-f\" ] && out=$2; shift; done\n%s\n", r.dir, r.okPiper()))
	if _, err := r.client(t).Speak(context.Background(), pronto); err != nil {
		t.Fatal(err)
	}
	cli := r.call(t, "piper")
	for _, arg := range []string{"--noise_scale", "--noise_w"} {
		if strings.Contains(cli, arg) {
			t.Errorf("piper call has %q although the noise was left at zero: %q", arg, cli)
		}
	}
}

func TestSpeakGivesPiperOneCleanLine(t *testing.T) {
	r := newRig(t, "")
	r.cfg.MaxChars = 200
	script(t, r.dir, "piper", fmt.Sprintf("cat > %s/stdin.txt\nwhile [ $# -gt 0 ]; do [ \"$1\" = \"-f\" ] && out=$2; shift; done\n%s\n", r.dir, r.okPiper()))
	text := "--help\r\n\r\nPrimeiro parágrafo.\n\tSegundo\x00 parágrafo.\u2028Terceiro."
	if _, err := r.client(t).Speak(context.Background(), text); err != nil {
		t.Fatal(err)
	}
	want := "--help Primeiro parágrafo. Segundo parágrafo. Terceiro.\n"
	if got := r.read(t, "stdin.txt"); got != want {
		t.Errorf("stdin = %q, want %q", got, want)
	}
}

func TestTextWithNothingToSayStartsNothing(t *testing.T) {
	r := newRig(t, neverRuns)
	c := r.client(t)
	for _, text := range []string{"", "   ", "\n\t\r\n", "...", "?!", "- - -"} {
		_, err := c.Speak(context.Background(), text)
		if !errors.Is(err, piper.ErrEmptyText) {
			t.Errorf("Speak(%q) err = %v, want ErrEmptyText", text, err)
		}
	}
	if calls := r.calls(t); len(calls) != 0 {
		t.Errorf("programs were started: %q", calls)
	}
	r.assertTempDirEmpty(t)
}

func TestTooLongTextIsRefusedBeforeAnythingRuns(t *testing.T) {
	r := newRig(t, "@OK@")
	r.cfg.MaxChars = 10
	c := r.client(t)

	// Ten characters are allowed even when they take twenty bytes: the limit
	// counts characters.
	if _, err := c.Speak(context.Background(), "ãéíõúçãéíõ"); err != nil {
		t.Fatalf("ten characters: %v", err)
	}
	before := len(r.calls(t))

	_, err := c.Speak(context.Background(), "ãéíõúçãéíõú")
	if !errors.Is(err, piper.ErrTextTooLong) {
		t.Fatalf("err = %v, want ErrTextTooLong", err)
	}
	if strings.Contains(err.Error(), "ãéíõú") {
		t.Errorf("the error carries the text: %q", err)
	}
	if after := len(r.calls(t)); after != before {
		t.Errorf("piper ran for text over the limit")
	}
	r.assertTempDirEmpty(t)
}

func TestTheDefaultLimitIsTheDocumentedOne(t *testing.T) {
	r := newRig(t, neverRuns)
	c := r.client(t)
	_, err := c.Speak(context.Background(), strings.Repeat("a", piper.DefaultMaxChars+1))
	if !errors.Is(err, piper.ErrTextTooLong) {
		t.Fatalf("err = %v", err)
	}
	if calls := r.calls(t); len(calls) != 0 {
		t.Errorf("programs were started: %q", calls)
	}
}

func TestTimeoutKillsPiperAndNamesTheCause(t *testing.T) {
	r := newRig(t, `exec sleep 30`)
	r.cfg.Timeout = 400 * time.Millisecond
	start := time.Now()
	_, err := r.client(t).Speak(context.Background(), pronto)
	if !errors.Is(err, piper.ErrTimeout) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want ErrTimeout and DeadlineExceeded", err)
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Errorf("the process was not killed: took %v", took)
	}
	r.assertTempDirEmpty(t)
}

func TestACancelledContextIsReportedAsCancelled(t *testing.T) {
	r := newRig(t, `exec sleep 30`)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	_, err := r.client(t).Speak(ctx, pronto)
	if !errors.Is(err, context.Canceled) || errors.Is(err, piper.ErrTimeout) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	r.assertTempDirEmpty(t)
}

func TestPiperFailureCarriesAShortStderrAndNeverTheText(t *testing.T) {
	r := newRig(t, `echo "Traceback: could not phonemize: Pronto, seu Antônio. Anotei mil e duzentos reais de adubo no talhão três. Está certo?" >&2; echo "line: Anotei mil e duzentos reais de adubo no talhão três." >&2; echo "error: failed to load $out" >&2; exit 3`)
	_, err := r.client(t).Speak(context.Background(), pronto)
	if !errors.Is(err, piper.ErrFailed) {
		t.Fatalf("err = %v, want ErrFailed", err)
	}
	var perr *piper.ProcessError
	if !errors.As(err, &perr) {
		t.Fatalf("err is not a *ProcessError: %T", err)
	}
	if perr.ExitCode != 3 || perr.Program != "piper" {
		t.Errorf("ProcessError = %+v", perr)
	}
	msg := err.Error()
	if !strings.Contains(msg, "failed to load") || !strings.Contains(msg, "<tmp>/speech.wav") || !strings.Contains(msg, "<text>") {
		t.Errorf("message lacks the summary: %q", msg)
	}
	for _, leak := range []string{"Antônio", "adubo", "talhão", "reais", r.tempDir} {
		if strings.Contains(msg, leak) {
			t.Errorf("message leaks %q: %q", leak, msg)
		}
	}
	r.assertTempDirEmpty(t)
}

func TestHugeStderrIsTruncated(t *testing.T) {
	r := newRig(t, `i=0; while [ $i -lt 2000 ]; do echo "line $i of noise noise noise" >&2; i=$((i+1)); done; exit 1`)
	_, err := r.client(t).Speak(context.Background(), pronto)
	if !errors.Is(err, piper.ErrFailed) {
		t.Fatalf("err = %v", err)
	}
	if len(err.Error()) > 500 {
		t.Errorf("message is %d bytes", len(err.Error()))
	}
}

func TestOutputThatCannotBeReadIsBadOutput(t *testing.T) {
	cases := map[string]string{
		"no file":                 `true`,
		"empty file":              `: > "$out"`,
		"not a wav":               `echo "this is not audio" > "$out"`,
		"a wav with no data":      `printf 'RIFF\024\000\000\000WAVEfmt \020\000\000\000\001\000\001\000\104\126\000\000\210\130\001\000\002\000\020\000' > "$out"`,
		"a data chunk before fmt": `printf 'RIFF\024\000\000\000WAVEdata\004\000\000\000abcd' > "$out"`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, body)
			_, err := r.client(t).Speak(context.Background(), pronto)
			if !errors.Is(err, piper.ErrBadOutput) {
				t.Fatalf("err = %v, want ErrBadOutput", err)
			}
			r.assertTempDirEmpty(t)
		})
	}
}

func TestADataChunkWithAnUnsetSizeIsMeasuredByTheFile(t *testing.T) {
	r := newRig(t, "")
	data := wav(22050, time.Second)
	binary.LittleEndian.PutUint32(data[40:44], 0xFFFFFFFF)
	if err := os.WriteFile(filepath.Join(r.dir, "canned.wav"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	script(t, r.dir, "piper", fmt.Sprintf("cat >/dev/null\nwhile [ $# -gt 0 ]; do [ \"$1\" = \"-f\" ] && out=$2; shift; done\n%s\n", r.okPiper()))
	got, err := r.client(t).Speak(context.Background(), pronto)
	if err != nil {
		t.Fatal(err)
	}
	if got.Duration != time.Second {
		t.Errorf("Duration = %v, want 1s", got.Duration)
	}
}

func TestTheDurationFollowsTheSampleRateOfTheVoice(t *testing.T) {
	r := newRig(t, "")
	if err := os.WriteFile(filepath.Join(r.dir, "canned.wav"), wav(16000, 3*time.Second), 0o600); err != nil {
		t.Fatal(err)
	}
	script(t, r.dir, "piper", fmt.Sprintf("cat >/dev/null\nwhile [ $# -gt 0 ]; do [ \"$1\" = \"-f\" ] && out=$2; shift; done\n%s\n", r.okPiper()))
	got, err := r.client(t).Speak(context.Background(), pronto)
	if err != nil {
		t.Fatal(err)
	}
	if got.Duration != 3*time.Second {
		t.Errorf("Duration = %v, want 3s", got.Duration)
	}
}

func TestThePiperProcessDoesNotInheritTheApplicationsEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:SECRET@db/prod")
	t.Setenv("ANTHROPIC_API_KEY", "sk-secret")
	r := newRig(t, "")
	r.cfg.Env = []string{"OMP_NUM_THREADS=2"}
	script(t, r.dir, "piper", fmt.Sprintf("cat >/dev/null\nenv > %[1]s/env.txt\npwd > %[1]s/pwd.txt\nwhile [ $# -gt 0 ]; do [ \"$1\" = \"-f\" ] && out=$2; shift; done\n%[2]s\n", r.dir, r.okPiper()))
	if _, err := r.client(t).Speak(context.Background(), pronto); err != nil {
		t.Fatal(err)
	}
	env := r.read(t, "env.txt")
	for _, secret := range []string{"SECRET", "sk-secret", "DATABASE_URL", "ANTHROPIC_API_KEY"} {
		if strings.Contains(env, secret) {
			t.Errorf("the environment of piper has %q", secret)
		}
	}
	for _, want := range []string{"OMP_NUM_THREADS=2", "PYTHONUTF8=1", "LANG=C.UTF-8", "PATH="} {
		if !strings.Contains(env, want) {
			t.Errorf("the environment of piper lacks %q:\n%s", want, env)
		}
	}
	pwd := strings.TrimSpace(r.read(t, "pwd.txt"))
	if !strings.Contains(pwd, "arandu-piper-") {
		t.Errorf("piper did not run in the private directory: %q", pwd)
	}
	// macOS reports the directory through /private, so compare the last part.
	private := filepath.Base(pwd)
	for _, name := range []string{"HOME=", "TMPDIR="} {
		line := ""
		for _, l := range strings.Split(env, "\n") {
			if strings.HasPrefix(l, name) {
				line = l
			}
		}
		if !strings.HasSuffix(line, "/"+private) {
			t.Errorf("%s is not the private directory %q: %q", name, private, line)
		}
	}
}

func TestOpusIsMadeFromTheWAVForAWhatsAppVoiceNote(t *testing.T) {
	r := newRig(t, "")
	r.cfg.Format = piper.FormatOpus
	r.cfg.FFmpegPath = filepath.Join(r.dir, "ffmpeg")
	script(t, r.dir, "piper", fmt.Sprintf("cat >/dev/null\nwhile [ $# -gt 0 ]; do [ \"$1\" = \"-f\" ] && out=$2; shift; done\n%s\n", r.okPiper()))

	got, err := r.client(t).Speak(context.Background(), pronto)
	if err != nil {
		t.Fatal(err)
	}
	if got.MIMEType != "audio/ogg; codecs=opus" {
		t.Errorf("MIMEType = %q", got.MIMEType)
	}
	if string(got.Data) != "OggS stand-in opus" {
		t.Errorf("Data = %q", got.Data)
	}
	if got.Duration != 2*time.Second {
		t.Errorf("Duration = %v, want the length of the speech", got.Duration)
	}
	conv := r.call(t, "ffmpeg")
	for _, arg := range []string{"-nostdin", "-loglevel error", "-ac 1", "-ar 16000", "-c:a libopus", "-b:a 24000", "-application voip", "-f ogg", "/speech.wav", "/speech.ogg"} {
		if !strings.Contains(conv, arg) {
			t.Errorf("ffmpeg call lacks %q: %q", arg, conv)
		}
	}
	r.assertTempDirEmpty(t)
}

func TestOpusBitrateIsConfigurable(t *testing.T) {
	r := newRig(t, "")
	r.cfg.Format = piper.FormatOpus
	r.cfg.FFmpegPath = filepath.Join(r.dir, "ffmpeg")
	r.cfg.OpusBitrate = 32000
	script(t, r.dir, "piper", fmt.Sprintf("cat >/dev/null\nwhile [ $# -gt 0 ]; do [ \"$1\" = \"-f\" ] && out=$2; shift; done\n%s\n", r.okPiper()))
	if _, err := r.client(t).Speak(context.Background(), pronto); err != nil {
		t.Fatal(err)
	}
	if conv := r.call(t, "ffmpeg"); !strings.Contains(conv, "-b:a 32000") {
		t.Errorf("ffmpeg call = %q", conv)
	}
}

func TestAnFFmpegFailureIsAConversionFailureAndNeverShowsTheText(t *testing.T) {
	r := newRig(t, "")
	r.cfg.Format = piper.FormatOpus
	r.cfg.FFmpegPath = filepath.Join(r.dir, "ffmpeg")
	script(t, r.dir, "piper", fmt.Sprintf("cat >/dev/null\nwhile [ $# -gt 0 ]; do [ \"$1\" = \"-f\" ] && out=$2; shift; done\n%s\n", r.okPiper()))
	script(t, r.dir, "ffmpeg", "echo 'Unknown encoder libopus' >&2\nexit 1\n")
	_, err := r.client(t).Speak(context.Background(), pronto)
	if !errors.Is(err, piper.ErrConvertFailed) || errors.Is(err, piper.ErrFailed) {
		t.Fatalf("err = %v, want ErrConvertFailed only", err)
	}
	if !strings.Contains(err.Error(), "Unknown encoder") {
		t.Errorf("message lacks the reason: %q", err)
	}
	r.assertTempDirEmpty(t)
}

func TestAPiperFailureNeverStartsFFmpeg(t *testing.T) {
	r := newRig(t, "exit 2")
	r.cfg.Format = piper.FormatOpus
	r.cfg.FFmpegPath = filepath.Join(r.dir, "ffmpeg")
	_, err := r.client(t).Speak(context.Background(), pronto)
	if !errors.Is(err, piper.ErrFailed) {
		t.Fatalf("err = %v", err)
	}
	for _, c := range r.calls(t) {
		if strings.HasPrefix(c, "ffmpeg ") {
			t.Errorf("ffmpeg ran after piper failed: %q", c)
		}
	}
}

func TestConcurrentCallsDoNotShareAWorkingDirectory(t *testing.T) {
	r := newRig(t, "")
	script(t, r.dir, "piper", fmt.Sprintf("cat >/dev/null\nwhile [ $# -gt 0 ]; do [ \"$1\" = \"-f\" ] && out=$2; shift; done\n%s\n", r.okPiper()))
	c := r.client(t)
	errs := make(chan error, 8)
	for i := 0; i < cap(errs); i++ {
		go func() {
			got, err := c.Speak(context.Background(), pronto)
			if err == nil && len(got.Data) == 0 {
				err = errors.New("empty audio")
			}
			errs <- err
		}()
	}
	for i := 0; i < cap(errs); i++ {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
	r.assertTempDirEmpty(t)
}

func TestNewValidatesTheConfig(t *testing.T) {
	good := func(t *testing.T) (*rig, string) {
		r := newRig(t, "")
		notExec := filepath.Join(r.dir, "plain.txt")
		if err := os.WriteFile(notExec, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		return r, notExec
	}
	write := func(t *testing.T, name, content string) string {
		p := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	cases := []struct {
		name   string
		mutate func(t *testing.T, r *rig, notExec string)
		want   string
	}{
		{"binary missing", func(t *testing.T, r *rig, _ string) { r.cfg.BinaryPath = "" }, "BinaryPath: is required"},
		{"binary absent", func(t *testing.T, r *rig, _ string) { r.cfg.BinaryPath = filepath.Join(r.dir, "nope") }, "BinaryPath"},
		{"binary is a directory", func(t *testing.T, r *rig, _ string) { r.cfg.BinaryPath = r.dir }, "BinaryPath"},
		{"binary not executable", func(t *testing.T, r *rig, notExec string) { r.cfg.BinaryPath = notExec }, "not executable"},
		{"binary not in PATH", func(t *testing.T, r *rig, _ string) { r.cfg.BinaryPath = "piper-that-does-not-exist" }, "not found in PATH"},
		{"no voice", func(t *testing.T, r *rig, _ string) { r.cfg.Voices = map[piper.Voice]piper.VoiceFiles{} }, "Voices: at least one voice"},
		{"a voice without a name", func(t *testing.T, r *rig, _ string) {
			r.cfg.Voices = map[piper.Voice]piper.VoiceFiles{"": r.voice}
		}, "empty name"},
		{"two voices and no default", func(t *testing.T, r *rig, _ string) {
			r.cfg.Voices = map[piper.Voice]piper.VoiceFiles{piper.VoiceCadu: r.voice, piper.VoiceFaber: r.voice}
		}, "DefaultVoice is required"},
		{"a default voice not configured", func(t *testing.T, r *rig, _ string) { r.cfg.DefaultVoice = piper.VoiceFaber }, `DefaultVoice "pt_BR-faber-medium" is not one of Voices`},
		{"one bad voice among good ones", func(t *testing.T, r *rig, _ string) {
			r.cfg.Voices = map[piper.Voice]piper.VoiceFiles{piper.VoiceCadu: r.voice, piper.VoiceFaber: {ModelPath: filepath.Join(r.dir, "nope.onnx")}}
			r.cfg.DefaultVoice = piper.VoiceCadu
		}, `Voices["pt_BR-faber-medium"].ModelPath`},
		{"model missing", func(t *testing.T, r *rig, _ string) { r.voice.ModelPath = "" }, "ModelPath: is required"},
		{"model absent", func(t *testing.T, r *rig, _ string) { r.voice.ModelPath = filepath.Join(r.dir, "nope.onnx") }, "ModelPath"},
		{"model is a directory", func(t *testing.T, r *rig, _ string) { r.voice.ModelPath = r.dir }, "not a regular file"},
		{"model is empty", func(t *testing.T, r *rig, _ string) {
			p := filepath.Join(r.dir, "empty.onnx")
			_ = os.WriteFile(p, nil, 0o600)
			r.voice.ModelPath = p
		}, "is empty"},
		{"config beside the model absent", func(t *testing.T, r *rig, _ string) {
			p := filepath.Join(r.dir, "lonely.onnx")
			_ = os.WriteFile(p, []byte("voice"), 0o600)
			r.voice.ModelPath = p
		}, "ConfigPath (the model's .json)"},
		{"config absent", func(t *testing.T, r *rig, _ string) { r.voice.ConfigPath = filepath.Join(r.dir, "nope.json") }, "ConfigPath"},
		{"config is not JSON", func(t *testing.T, r *rig, _ string) { r.voice.ConfigPath = write(t, "v.json", "not json at all") }, "not a voice configuration"},
		{"config is not an object", func(t *testing.T, r *rig, _ string) { r.voice.ConfigPath = write(t, "v.json", "[1, 2]") }, "not a voice configuration"},
		{"negative speaker", func(t *testing.T, r *rig, _ string) { r.voice.SpeakerID = -1 }, "SpeakerID"},
		{"speaker the voice lacks", func(t *testing.T, r *rig, _ string) { r.voice.SpeakerID = 1 }, "1 speaker(s)"},
		{"length scale too small", func(t *testing.T, r *rig, _ string) { r.cfg.LengthScale = 0.1 }, "LengthScale"},
		{"length scale too large", func(t *testing.T, r *rig, _ string) { r.cfg.LengthScale = 9 }, "LengthScale"},
		{"negative noise scale", func(t *testing.T, r *rig, _ string) { r.cfg.NoiseScale = -0.1 }, "NoiseScale"},
		{"noise scale too large", func(t *testing.T, r *rig, _ string) { r.cfg.NoiseScale = 2.5 }, "NoiseScale"},
		{"negative noise w", func(t *testing.T, r *rig, _ string) { r.cfg.NoiseW = -1 }, "NoiseW"},
		{"noise w too large", func(t *testing.T, r *rig, _ string) { r.cfg.NoiseW = 3 }, "NoiseW"},
		{"negative sentence silence", func(t *testing.T, r *rig, _ string) { r.cfg.SentenceSilence = -time.Second }, "SentenceSilence"},
		{"huge sentence silence", func(t *testing.T, r *rig, _ string) { r.cfg.SentenceSilence = time.Minute }, "SentenceSilence"},
		{"unknown format", func(t *testing.T, r *rig, _ string) { r.cfg.Format = "mp3" }, `Format "mp3"`},
		{"opus without ffmpeg", func(t *testing.T, r *rig, _ string) {
			r.cfg.Format = piper.FormatOpus
			r.cfg.FFmpegPath = filepath.Join(r.dir, "nope")
		}, "FFmpegPath"},
		{"opus bitrate too low", func(t *testing.T, r *rig, _ string) { r.cfg.OpusBitrate = 100 }, "OpusBitrate"},
		{"negative timeout", func(t *testing.T, r *rig, _ string) { r.cfg.Timeout = -time.Second }, "Timeout"},
		{"negative max chars", func(t *testing.T, r *rig, _ string) { r.cfg.MaxChars = -1 }, "MaxChars"},
		{"temp dir absent", func(t *testing.T, r *rig, _ string) { r.cfg.TempDir = filepath.Join(r.dir, "nope") }, "TempDir"},
		{"temp dir is a file", func(t *testing.T, r *rig, notExec string) { r.cfg.TempDir = notExec }, "not a directory"},
		{"env without a value", func(t *testing.T, r *rig, _ string) { r.cfg.Env = []string{"JUSTAKEY"} }, "KEY=VALUE"},
		{"env without a key", func(t *testing.T, r *rig, _ string) { r.cfg.Env = []string{"=value"} }, "KEY=VALUE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, notExec := good(t)
			tc.mutate(t, r, notExec)
			c, err := piper.New(r.config())
			if err == nil {
				t.Fatalf("New accepted the config: %+v", c)
			}
			if !strings.Contains(err.Error(), tc.want) || !strings.HasPrefix(err.Error(), "piper: ") {
				t.Errorf("err = %q, want it to contain %q", err, tc.want)
			}
		})
	}

	t.Run("a complete config works and fills the defaults", func(t *testing.T) {
		r, _ := good(t)
		if _, err := piper.New(r.config()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("an explicit config file is used", func(t *testing.T) {
		r, _ := good(t)
		r.voice.ConfigPath = write(t, "elsewhere.json", `{"num_speakers": 3}`)
		r.voice.SpeakerID = 2
		if _, err := piper.New(r.config()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("a config that does not say the speakers allows any", func(t *testing.T) {
		r, _ := good(t)
		r.voice.ConfigPath = write(t, "v.json", `{}`)
		r.voice.SpeakerID = 4
		if _, err := piper.New(r.config()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("opus with an ffmpeg in the config", func(t *testing.T) {
		r, _ := good(t)
		r.cfg.Format = piper.FormatOpus
		r.cfg.FFmpegPath = filepath.Join(r.dir, "ffmpeg")
		if _, err := piper.New(r.config()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("wav does not need ffmpeg", func(t *testing.T) {
		r, _ := good(t)
		r.cfg.FFmpegPath = filepath.Join(r.dir, "nope")
		if _, err := piper.New(r.config()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestNewDoesNotStartAnything(t *testing.T) {
	r := newRig(t, "")
	if _, err := piper.New(r.config()); err != nil {
		t.Fatal(err)
	}
	if calls := r.calls(t); len(calls) != 0 {
		t.Errorf("New started programs: %q", calls)
	}
}

func TestFakeAnswersInOrderAndRecords(t *testing.T) {
	boom := errors.New("boom")
	custom := piper.Audio{Data: []byte("OggS"), MIMEType: piper.MIMEOpus, Duration: 3 * time.Second}
	f := piper.NewFake(
		piper.Answer{},
		piper.Answer{Audio: custom},
		piper.Answer{Err: boom},
	)
	ctx := context.Background()

	a, err := f.Speak(ctx, "primeiro")
	if err != nil || a.MIMEType != piper.MIMEWAV || a.Duration != 100*time.Millisecond || !bytes.HasPrefix(a.Data, []byte("RIFF")) {
		t.Fatalf("first = %+v, %v", a, err)
	}
	b, err := f.Speak(ctx, "segundo")
	if err != nil || b.MIMEType != piper.MIMEOpus || string(b.Data) != "OggS" || b.Duration != 3*time.Second {
		t.Fatalf("second = %+v, %v", b, err)
	}
	if _, err := f.Speak(ctx, "terceiro"); !errors.Is(err, boom) {
		t.Fatalf("third err = %v", err)
	}
	if _, err := f.Speak(ctx, "quarto"); !errors.Is(err, piper.ErrScriptExhausted) {
		t.Fatalf("exhausted err = %v", err)
	}
	f.Push(piper.Answer{Audio: piper.Audio{Data: []byte("x")}})
	if got, err := f.Speak(ctx, "quinto"); err != nil || got.MIMEType != piper.MIMEWAV || string(got.Data) != "x" {
		t.Fatalf("pushed = %+v, %v", got, err)
	}

	reqs := f.Requests()
	if len(reqs) != 5 || reqs[0] != "primeiro" || reqs[4] != "quinto" {
		t.Errorf("requests = %q", reqs)
	}
}

func TestFakeRefusesWhatTheClientRefusesWithoutUsingAnAnswer(t *testing.T) {
	f := piper.NewFake(piper.Answer{})
	if _, err := f.Speak(context.Background(), " \n "); !errors.Is(err, piper.ErrEmptyText) {
		t.Fatalf("err = %v", err)
	}
	if _, err := f.Speak(context.Background(), strings.Repeat("a", piper.DefaultMaxChars+1)); !errors.Is(err, piper.ErrTextTooLong) {
		t.Fatalf("err = %v", err)
	}
	if got, err := f.Speak(context.Background(), "ok"); err != nil || len(got.Data) == 0 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

// TestRealFFmpegMakesAnOpusVoiceNote runs the real ffmpeg on a WAV written by
// a stand-in piper. It proves the conversion arguments, which the stand-in
// ffmpeg cannot: the result must be Ogg Opus, mono, 16 kHz. It is skipped when
// ffmpeg or ffprobe is missing or has no libopus.
func TestRealFFmpegMakesAnOpusVoiceNote(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skipf("ffmpeg is not in PATH: %v", err)
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skipf("ffprobe is not in PATH: %v", err)
	}
	tone := filepath.Join(t.TempDir(), "tone.wav")
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi",
		"-i", "sine=frequency=440:duration=2:sample_rate=22050", "-ac", "1", "-c:a", "pcm_s16le", tone).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg cannot generate a test tone: %v %s", err, out)
	}

	r := newRig(t, "")
	script(t, r.dir, "piper", fmt.Sprintf("cat >/dev/null\nwhile [ $# -gt 0 ]; do [ \"$1\" = \"-f\" ] && out=$2; shift; done\ncp %s \"$out\"\n", tone))
	r.cfg.Format = piper.FormatOpus
	r.cfg.FFmpegPath = ffmpeg
	got, err := r.client(t).Speak(context.Background(), pronto)
	if err != nil {
		if strings.Contains(err.Error(), "libopus") {
			t.Skipf("this ffmpeg has no libopus: %v", err)
		}
		t.Fatalf("Speak: %v", err)
	}
	if !bytes.HasPrefix(got.Data, []byte("OggS")) {
		t.Fatalf("not an Ogg file: %q", got.Data[:min(8, len(got.Data))])
	}
	if d := got.Duration - 2*time.Second; d < -50*time.Millisecond || d > 50*time.Millisecond {
		t.Errorf("Duration = %v, want about 2s", got.Duration)
	}
	file := filepath.Join(t.TempDir(), "note.ogg")
	if err := os.WriteFile(file, got.Data, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := exec.Command(ffprobe, "-v", "error", "-select_streams", "a:0",
		"-show_entries", "stream=codec_name,sample_rate,channels", "-of", "csv=p=0", file).Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(info)); got != "opus,48000,1" && got != "opus,16000,1" {
		// Opus stores 48 kHz in the stream whatever the input rate was; ffprobe
		// reports the original rate of the input, or 48 kHz.
		t.Errorf("ffprobe says %q, want opus, mono", got)
	}
	r.assertTempDirEmpty(t)
}

// TestRealPiper runs the real program. It needs Piper and a voice, so it only
// runs when PIPER_BIN and PIPER_MODEL name them (PIPER_CONFIG when the voice's
// JSON is not beside it, PIPER_ENV for extra comma-separated KEY=VALUE entries
// such as PYTHONPATH). PIPER_FFMPEG, when set, also makes Opus voice notes, at
// the voice's pace and slower. PIPER_SAMPLES, a directory, keeps what was
// said, for listening to.
func TestRealPiper(t *testing.T) {
	bin, model := os.Getenv("PIPER_BIN"), os.Getenv("PIPER_MODEL")
	if bin == "" || model == "" {
		t.Skip("set PIPER_BIN (path of piper) and PIPER_MODEL (path of a voice .onnx; PIPER_CONFIG for its JSON) to run against the real Piper")
	}
	cfg := piper.Config{BinaryPath: bin, Voices: map[piper.Voice]piper.VoiceFiles{
		piper.Voice(strings.TrimSuffix(filepath.Base(model), ".onnx")): {ModelPath: model, ConfigPath: os.Getenv("PIPER_CONFIG")},
	}}
	if env := os.Getenv("PIPER_ENV"); env != "" {
		cfg.Env = strings.Split(env, ",")
	}
	c, err := piper.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// A WhatsApp-formatted message, cleaned the way the module's users do it.
	text := piper.Plain("*Pronto*, seu Antônio. 🙂\n\nAnotei _mil e duzentos reais_ de adubo no talhão três.\n\nEstá certo?")
	if text != pronto {
		t.Fatalf("Plain gave %q, want %q", text, pronto)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	start := time.Now()
	got, err := c.Speak(ctx, text)
	took := time.Since(start)
	if err != nil {
		t.Fatalf("Speak: %v", err)
	}
	t.Logf("wav: %d bytes, %v of speech, made in %v", len(got.Data), got.Duration, took)
	if got.MIMEType != piper.MIMEWAV || !bytes.HasPrefix(got.Data, []byte("RIFF")) {
		t.Errorf("not a WAV: %q %q", got.MIMEType, got.Data[:min(8, len(got.Data))])
	}
	if got.Duration < 3*time.Second || got.Duration > 15*time.Second {
		t.Errorf("Duration = %v, a sentence of this size lasts between 3 and 15 seconds", got.Duration)
	}

	samples := map[string][]byte{"wav": got.Data}
	if ffmpeg := os.Getenv("PIPER_FFMPEG"); ffmpeg != "" {
		for suffix, scale := range map[string]float64{"": 0, ".slow": 1.2} {
			cfg := cfg
			cfg.Format, cfg.FFmpegPath, cfg.LengthScale = piper.FormatOpus, ffmpeg, scale
			oc, err := piper.New(cfg)
			if err != nil {
				t.Fatalf("New (opus): %v", err)
			}
			start = time.Now()
			note, err := oc.Speak(ctx, text)
			if err != nil {
				t.Fatalf("Speak (opus%s): %v", suffix, err)
			}
			t.Logf("opus%s: %d bytes, %v of speech, made in %v", suffix, len(note.Data), note.Duration, time.Since(start))
			if note.MIMEType != piper.MIMEOpus || !bytes.HasPrefix(note.Data, []byte("OggS")) {
				t.Errorf("not an Ogg Opus note: %q", note.MIMEType)
			}
			samples["ogg"+suffix] = note.Data
		}
	}

	if dir := os.Getenv("PIPER_SAMPLES"); dir != "" {
		name := strings.TrimSuffix(filepath.Base(model), ".onnx")
		for kind, data := range samples {
			ext, suffix, _ := strings.Cut(kind, ".")
			if suffix != "" {
				suffix = "." + suffix
			}
			if err := os.WriteFile(filepath.Join(dir, name+suffix+"."+ext), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}
