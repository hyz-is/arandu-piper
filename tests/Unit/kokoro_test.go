package unit_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hyz-is/arandu-piper"
)

// kokoroRig is a stand-in kokoro-speak (a shell script that logs its
// arguments, keeps what it was given on standard input and writes a canned WAV
// at the sample rate of Kokoro), a stand-in model, a real .npz voices file and
// a stand-in ffmpeg. No test here needs Kokoro, Python or a model.
type kokoroRig struct {
	*rig
	cfg piper.Config
}

const kokoroScript = `
echo "kokoro-speak $*" >> %[1]s/calls.log
cat > %[1]s/stdin.txt
env > %[1]s/env.txt
pwd > %[1]s/pwd.txt
while [ $# -gt 0 ]; do
  if [ "$1" = "--output" ]; then out=$2; fi
  shift
done
%[2]s
`

func newKokoroRig(t *testing.T, body string) *kokoroRig {
	t.Helper()
	r := newRig(t, "")
	if err := os.WriteFile(filepath.Join(r.dir, "canned24.wav"), wav(24000, 2*time.Second), 0o600); err != nil {
		t.Fatal(err)
	}
	if body == "" {
		body = fmt.Sprintf(`cp %s/canned24.wav "$out"`, r.dir)
	}
	script(t, r.dir, "kokoro-speak", fmt.Sprintf(kokoroScript, r.dir, body))
	model := filepath.Join(r.dir, "kokoro-v1.0.onnx")
	if err := os.WriteFile(model, []byte("not a real model"), 0o600); err != nil {
		t.Fatal(err)
	}
	voices := filepath.Join(r.dir, "voices-v1.0.bin")
	writeVoices(t, voices, "pf_dora", "pm_alex", "pm_santa", "af_heart")
	return &kokoroRig{rig: r, cfg: piper.Config{
		TempDir:      r.tempDir,
		DefaultVoice: piper.VoiceDora,
		Kokoro: piper.KokoroConfig{
			ProgramPath: filepath.Join(r.dir, "kokoro-speak"),
			ModelPath:   model,
			VoicesPath:  voices,
		},
	}}
}

// writeVoices writes a .npz (a zip of .npy entries), the shape of Kokoro's
// voices file, with one entry per voice.
func writeVoices(t *testing.T, path string, voices ...string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, v := range voices {
		w, err := zw.Create(v + ".npy")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte("\x93NUMPY stand-in for " + v))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (k *kokoroRig) client(t *testing.T) *piper.Client {
	t.Helper()
	c, err := piper.New(k.cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestAKokoroVoiceSpeaksThroughTheStandInProgramWithTheTextOnStdin(t *testing.T) {
	k := newKokoroRig(t, "")
	got, err := k.client(t).Speak(context.Background(), pronto)
	if err != nil {
		t.Fatalf("Speak: %v", err)
	}
	if got.MIMEType != piper.MIMEWAV || got.Duration != 2*time.Second {
		t.Errorf("%q %v", got.MIMEType, got.Duration)
	}
	if !bytes.Equal(got.Data, wav(24000, 2*time.Second)) {
		t.Errorf("Data is not what the program wrote (%d bytes)", len(got.Data))
	}

	cli := k.call(t, "kokoro-speak")
	for _, arg := range []string{
		"--model " + k.cfg.Kokoro.ModelPath, "--voices " + k.cfg.Kokoro.VoicesPath,
		"--voice pf_dora", "--lang pt-br", "--threads 2", "--output ", "/speech.wav",
	} {
		if !strings.Contains(cli, arg) {
			t.Errorf("the call lacks %q: %q", arg, cli)
		}
	}
	for _, arg := range []string{"--speed", "--sentence-pause"} {
		if strings.Contains(cli, arg) {
			t.Errorf("the call has %q by default: %q", arg, cli)
		}
	}
	if strings.Contains(cli, "Antônio") || strings.Contains(cli, "adubo") {
		t.Errorf("the text is in the argument list: %q", cli)
	}
	if stdin := k.read(t, "stdin.txt"); stdin != pronto+"\n" {
		t.Errorf("stdin = %q", stdin)
	}
	if len(k.calls(t)) != 1 {
		t.Errorf("only kokoro-speak should have run: %q", k.calls(t))
	}
	k.assertTempDirEmpty(t)
}

func TestTheNameOfTheVoiceChoosesTheEngine(t *testing.T) {
	k := newKokoroRig(t, "")
	piperRig := k.rig
	script(t, k.dir, "piper", fmt.Sprintf("echo \"piper $*\" >> %s/calls.log\ncat >/dev/null\nwhile [ $# -gt 0 ]; do [ \"$1\" = \"-f\" ] && out=$2; shift; done\n%s\n", k.dir, piperRig.okPiper()))
	k.cfg.BinaryPath = filepath.Join(k.dir, "piper")
	k.cfg.Voices = map[piper.Voice]piper.VoiceFiles{piper.VoiceCadu: k.voice}
	k.cfg.DefaultVoice = piper.VoiceCadu
	c := k.client(t)

	want := []piper.Voice{piper.VoiceAlex, piper.VoiceDora, piper.VoiceSanta, piper.VoiceCadu}
	got := c.Voices()
	if len(got) != len(want) {
		t.Fatalf("Voices = %v", got)
	}
	for _, v := range want {
		found := false
		for _, g := range got {
			found = found || g == v
		}
		if !found {
			t.Errorf("Voices = %v lacks %q", got, v)
		}
	}

	if _, err := c.Speak(context.Background(), pronto); err != nil { // the default: Piper
		t.Fatal(err)
	}
	if _, err := c.SpeakAs(context.Background(), piper.VoiceAlex, pronto); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SpeakAs(context.Background(), piper.VoiceCadu, pronto); err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, line := range k.calls(t) {
		program, _, _ := strings.Cut(line, " ")
		order = append(order, program)
	}
	if strings.Join(order, ",") != "piper,kokoro-speak,piper" {
		t.Errorf("programs started: %q", k.calls(t))
	}
	if !strings.Contains(k.calls(t)[1], "--voice pm_alex") {
		t.Errorf("the second call is not pm_alex: %q", k.calls(t)[1])
	}
}

func TestAKokoroOnlyClientNeedsNoPiper(t *testing.T) {
	k := newKokoroRig(t, "")
	k.cfg.DefaultVoice = ""
	k.cfg.Kokoro.Voices = []piper.Voice{piper.VoiceDora}
	c := k.client(t)
	if c.DefaultVoice() != piper.VoiceDora || len(c.Voices()) != 1 {
		t.Fatalf("%q %v", c.DefaultVoice(), c.Voices())
	}
	// A voice of the file that this client was not asked to serve is unknown.
	if _, err := c.SpeakAs(context.Background(), piper.VoiceAlex, pronto); !errors.Is(err, piper.ErrUnknownVoice) {
		t.Fatalf("err = %v", err)
	}
	if len(k.calls(t)) != 0 {
		t.Errorf("a program was started: %q", k.calls(t))
	}
}

func TestTheKokoroPaceThePauseAndTheThreadsReachTheProgram(t *testing.T) {
	k := newKokoroRig(t, "")
	k.cfg.LengthScale = 1.25
	k.cfg.SentenceSilence = 350 * time.Millisecond
	k.cfg.Kokoro.Threads = 1
	if _, err := k.client(t).Speak(context.Background(), pronto); err != nil {
		t.Fatal(err)
	}
	cli := k.call(t, "kokoro-speak")
	for _, arg := range []string{"--speed 0.8", "--sentence-pause 0.35", "--threads 1"} {
		if !strings.Contains(cli, arg) {
			t.Errorf("the call lacks %q: %q", arg, cli)
		}
	}
}

func TestTheKokoroProgramDoesNotInheritTheApplicationsEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:SECRET@db/prod")
	t.Setenv("ANTHROPIC_API_KEY", "sk-secret")
	k := newKokoroRig(t, "")
	k.cfg.Env = []string{"PYTHONPATH=/opt/piper", "X=from-config"}
	k.cfg.Kokoro.Env = []string{"PYTHONPATH=/opt/kokoro"}
	if _, err := k.client(t).Speak(context.Background(), pronto); err != nil {
		t.Fatal(err)
	}
	env := k.read(t, "env.txt")
	for _, secret := range []string{"SECRET", "sk-secret", "DATABASE_URL", "ANTHROPIC_API_KEY"} {
		if strings.Contains(env, secret) {
			t.Errorf("the environment of kokoro-speak has %q", secret)
		}
	}
	for _, want := range []string{"PYTHONPATH=/opt/kokoro", "X=from-config", "PYTHONUTF8=1", "LANG=C.UTF-8"} {
		if !strings.Contains(env, want) {
			t.Errorf("the environment lacks %q:\n%s", want, env)
		}
	}
	// Kokoro.Env goes after Config.Env, so the last PYTHONPATH wins.
	if strings.LastIndex(env, "PYTHONPATH=/opt/kokoro") < strings.LastIndex(env, "PYTHONPATH=/opt/piper") {
		t.Errorf("Kokoro.Env does not follow Config.Env:\n%s", env)
	}
	if pwd := strings.TrimSpace(k.read(t, "pwd.txt")); !strings.Contains(pwd, "arandu-piper-") {
		t.Errorf("kokoro-speak did not run in the private directory: %q", pwd)
	}
}

func TestKokoroOpusIsMadeFromTheWAVAtTheRateOfTheModel(t *testing.T) {
	k := newKokoroRig(t, "")
	k.cfg.Format = piper.FormatOpus
	k.cfg.FFmpegPath = filepath.Join(k.dir, "ffmpeg")
	got, err := k.client(t).Speak(context.Background(), pronto)
	if err != nil {
		t.Fatal(err)
	}
	if got.MIMEType != piper.MIMEOpus || got.Duration != 2*time.Second {
		t.Errorf("%q %v", got.MIMEType, got.Duration)
	}
	cli := k.call(t, "ffmpeg")
	for _, arg := range []string{"-ac 1", "-ar 16000", "-c:a libopus"} {
		if !strings.Contains(cli, arg) {
			t.Errorf("ffmpeg call lacks %q: %q", arg, cli)
		}
	}
	k.assertTempDirEmpty(t)
}

func TestAKokoroFailureCarriesAShortStderrAndNeverTheText(t *testing.T) {
	k := newKokoroRig(t, `echo "Traceback: could not phonemize: Pronto, seu Antônio. Anotei mil e duzentos reais de adubo no talhão três. Está certo?" >&2; echo "kokoro-speak: RuntimeError at $out" >&2; exit 1`)
	_, err := k.client(t).Speak(context.Background(), pronto)
	if !errors.Is(err, piper.ErrFailed) {
		t.Fatalf("err = %v, want ErrFailed", err)
	}
	var perr *piper.ProcessError
	if !errors.As(err, &perr) || perr.Program != "kokoro-speak" || perr.ExitCode != 1 {
		t.Fatalf("ProcessError = %+v (%v)", perr, err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "RuntimeError") || !strings.Contains(msg, "<text>") {
		t.Errorf("message lacks the summary: %q", msg)
	}
	for _, leak := range []string{"Antônio", "adubo", "talhão", k.tempDir} {
		if strings.Contains(msg, leak) {
			t.Errorf("message leaks %q: %q", leak, msg)
		}
	}
	k.assertTempDirEmpty(t)
}

func TestTheTimeoutKillsKokoro(t *testing.T) {
	k := newKokoroRig(t, `exec sleep 30`)
	k.cfg.Timeout = 400 * time.Millisecond
	start := time.Now()
	_, err := k.client(t).Speak(context.Background(), pronto)
	if !errors.Is(err, piper.ErrTimeout) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Errorf("the process was not killed: took %v", took)
	}
	k.assertTempDirEmpty(t)
}

func TestKokoroTextIsCheckedBeforeAnythingRuns(t *testing.T) {
	k := newKokoroRig(t, "exit 99")
	k.cfg.MaxChars = 10
	c := k.client(t)
	if _, err := c.Speak(context.Background(), " ... "); !errors.Is(err, piper.ErrEmptyText) {
		t.Errorf("err = %v", err)
	}
	if _, err := c.Speak(context.Background(), "uma frase bem longa demais"); !errors.Is(err, piper.ErrTextTooLong) {
		t.Errorf("err = %v", err)
	}
	if calls := k.calls(t); len(calls) != 0 {
		t.Errorf("programs were started: %q", calls)
	}
}

func TestNewRefusesAWrongKokoroConfigurationAtBoot(t *testing.T) {
	for name, tc := range map[string]struct {
		mod  func(*kokoroRig)
		want string
	}{
		"no program":       {func(k *kokoroRig) { k.cfg.Kokoro.ProgramPath = "" }, "Kokoro.ProgramPath"},
		"missing program":  {func(k *kokoroRig) { k.cfg.Kokoro.ProgramPath = filepath.Join(k.dir, "nope") }, "Kokoro.ProgramPath"},
		"not executable":   {func(k *kokoroRig) { k.cfg.Kokoro.ProgramPath = k.cfg.Kokoro.ModelPath }, "not executable"},
		"no model":         {func(k *kokoroRig) { k.cfg.Kokoro.ModelPath = "" }, "Kokoro.ModelPath"},
		"missing model":    {func(k *kokoroRig) { k.cfg.Kokoro.ModelPath = filepath.Join(k.dir, "nope.onnx") }, "Kokoro.ModelPath"},
		"no voices file":   {func(k *kokoroRig) { k.cfg.Kokoro.VoicesPath = "" }, "Kokoro.VoicesPath"},
		"voices not a zip": {func(k *kokoroRig) { k.cfg.Kokoro.VoicesPath = k.cfg.Kokoro.ModelPath }, "not a Kokoro voices file"},
		"a voice that the file lacks": {func(k *kokoroRig) {
			writeVoices(t, k.cfg.Kokoro.VoicesPath, "pf_dora")
			k.cfg.Kokoro.Voices = []piper.Voice{piper.VoiceDora, piper.VoiceAlex}
		}, `"pm_alex" is not in the voices file`},
		"the default voice lacks": {func(k *kokoroRig) {
			writeVoices(t, k.cfg.Kokoro.VoicesPath, "pm_alex")
		}, `"pf_dora" is not in the voices file`},
		"a voice of Piper":    {func(k *kokoroRig) { k.cfg.Kokoro.Voices = []piper.Voice{piper.VoiceCadu} }, "not a Kokoro voice"},
		"a voice twice":       {func(k *kokoroRig) { k.cfg.Kokoro.Voices = []piper.Voice{piper.VoiceDora, piper.VoiceDora} }, "twice"},
		"too many threads":    {func(k *kokoroRig) { k.cfg.Kokoro.Threads = 99 }, "Kokoro.Threads"},
		"negative threads":    {func(k *kokoroRig) { k.cfg.Kokoro.Threads = -1 }, "Kokoro.Threads"},
		"bad environment":     {func(k *kokoroRig) { k.cfg.Kokoro.Env = []string{"nope"} }, "Kokoro.Env"},
		"a default of nobody": {func(k *kokoroRig) { k.cfg.DefaultVoice = piper.VoiceCadu }, "DefaultVoice"},
		"a Kokoro voice among": {func(k *kokoroRig) {
			k.cfg.BinaryPath = filepath.Join(k.dir, "piper")
			k.cfg.Voices = map[piper.Voice]piper.VoiceFiles{piper.VoiceDora: {ModelPath: k.cfg.Kokoro.ModelPath}}
		}, "Config.Kokoro, not Voices"},
		"piper without voices": {func(k *kokoroRig) { k.cfg.BinaryPath = filepath.Join(k.dir, "piper") }, "Voices: at least one voice"},
	} {
		t.Run(name, func(t *testing.T) {
			k := newKokoroRig(t, "")
			tc.mod(k)
			_, err := piper.New(k.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("New err = %v, want it to say %q", err, tc.want)
			}
		})
	}
}

func TestNewDoesNotStartKokoro(t *testing.T) {
	k := newKokoroRig(t, "exit 99")
	k.client(t)
	if calls := k.calls(t); len(calls) != 0 {
		t.Errorf("New started a program: %q", calls)
	}
}

func TestTheKokoroKeyChangesWithWhatChangesTheAudioAndOnlyWithThat(t *testing.T) {
	key := func(mod func(*kokoroRig), voice piper.Voice, text string) string {
		k := newKokoroRig(t, "")
		if mod != nil {
			mod(k)
		}
		got, err := k.client(t).Key(voice, text)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	base := key(nil, piper.VoiceDora, pronto)
	if len(base) != 64 || strings.Trim(base, "0123456789abcdef") != "" {
		t.Fatalf("key = %q", base)
	}
	if again := key(nil, "", "  "+pronto+"\n"); again != base {
		t.Error("the default voice and a cleaned text give another key")
	}
	for name, other := range map[string]string{
		"voice":  key(nil, piper.VoiceAlex, pronto),
		"text":   key(nil, piper.VoiceDora, pronto+" Obrigado."),
		"pace":   key(func(k *kokoroRig) { k.cfg.LengthScale = 1.2 }, piper.VoiceDora, pronto),
		"pause":  key(func(k *kokoroRig) { k.cfg.SentenceSilence = time.Second }, piper.VoiceDora, pronto),
		"format": key(func(k *kokoroRig) { k.cfg.Format = piper.FormatOpus; k.cfg.FFmpegPath = filepath.Join(k.dir, "ffmpeg") }, piper.VoiceDora, pronto),
		"bit rate": key(func(k *kokoroRig) {
			k.cfg.Format = piper.FormatOpus
			k.cfg.FFmpegPath = filepath.Join(k.dir, "ffmpeg")
			k.cfg.OpusBitrate = 32000
		}, piper.VoiceDora, pronto),
		"model": key(func(k *kokoroRig) {
			_ = os.WriteFile(k.cfg.Kokoro.ModelPath, []byte("another model, of another size"), 0o600)
		}, piper.VoiceDora, pronto),
		"voices file": key(func(k *kokoroRig) {
			writeVoices(t, k.cfg.Kokoro.VoicesPath, "pf_dora", "pm_alex", "pm_santa", "ff_siwis")
		}, piper.VoiceDora, pronto),
	} {
		if other == base {
			t.Errorf("the key does not change with the %s", name)
		}
	}
	for name, same := range map[string]string{
		"threads":     key(func(k *kokoroRig) { k.cfg.Kokoro.Threads = 4 }, piper.VoiceDora, pronto),
		"noise":       key(func(k *kokoroRig) { k.cfg.NoiseScale, k.cfg.NoiseW = 0.5, 0.5 }, piper.VoiceDora, pronto),
		"environment": key(func(k *kokoroRig) { k.cfg.Kokoro.Env = []string{"A=b"} }, piper.VoiceDora, pronto),
		"temp dir":    key(func(k *kokoroRig) { k.cfg.TempDir = os.TempDir() }, piper.VoiceDora, pronto),
	} {
		if same != base {
			t.Errorf("the key changes with the %s, which does not change the audio", name)
		}
	}
	if _, err := newKokoroRig(t, "").client(t).Key(piper.VoiceCadu, pronto); !errors.Is(err, piper.ErrUnknownVoice) {
		t.Errorf("err = %v", err)
	}
}

func TestKokoroVoicesAreParsedAndKnowTheirEngine(t *testing.T) {
	for in, want := range map[string]piper.Voice{
		"dora": piper.VoiceDora, " Alex ": piper.VoiceAlex, "SANTA": piper.VoiceSanta,
		"pf_dora": piper.VoiceDora, "pm_alex": piper.VoiceAlex, "pm_santa": piper.VoiceSanta,
	} {
		got, err := piper.ParseVoice(in)
		if err != nil || got != want {
			t.Errorf("ParseVoice(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"pf_maria", "pm_dora", "af_heart", "pf_dora.npy"} {
		if got, err := piper.ParseVoice(in); !errors.Is(err, piper.ErrUnknownVoice) {
			t.Errorf("ParseVoice(%q) = %q, %v; want ErrUnknownVoice", in, got, err)
		}
	}
	for _, v := range piper.KokoroVoices() {
		if v.Engine() != piper.EngineKokoro {
			t.Errorf("%q: engine %q", v, v.Engine())
		}
	}
	for _, v := range []piper.Voice{piper.VoiceFaber, piper.VoiceCadu, "en_US-lessac-medium"} {
		if v.Engine() != piper.EnginePiper {
			t.Errorf("%q: engine %q", v, v.Engine())
		}
	}
}

// TestRealKokoro runs the real program and model: set KOKORO_BIN (the
// kokoro-speak program), KOKORO_MODEL and KOKORO_VOICES; add KOKORO_ENV
// (comma-separated KEY=VALUE, such as PYTHONPATH=/opt/kokoro) when the packages
// were installed with pip --target, KOKORO_FFMPEG for the Opus note and
// KOKORO_SAMPLES (a directory) to keep the audio.
func TestRealKokoro(t *testing.T) {
	bin, model, voices := os.Getenv("KOKORO_BIN"), os.Getenv("KOKORO_MODEL"), os.Getenv("KOKORO_VOICES")
	if bin == "" || model == "" || voices == "" {
		t.Skip("set KOKORO_BIN (kokoro-speak), KOKORO_MODEL (kokoro-v1.0.onnx) and KOKORO_VOICES (voices-v1.0.bin) to run against the real Kokoro")
	}
	if runtime.GOOS == "windows" {
		t.Skip("kokoro-speak is a script with a shebang")
	}
	cfg := piper.Config{
		Kokoro:       piper.KokoroConfig{ProgramPath: bin, ModelPath: model, VoicesPath: voices},
		DefaultVoice: piper.VoiceDora,
	}
	if env := os.Getenv("KOKORO_ENV"); env != "" {
		cfg.Kokoro.Env = strings.Split(env, ",")
	}
	c, err := piper.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	text := "Bom dia! Hoje é dia de treino. Para acordar o corpo: dez polichinelos, um alongamento rápido e cinco minutos de cardio. Me avisa quando terminar."

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	samples := map[string][]byte{}
	for _, v := range c.Voices() {
		start := time.Now()
		got, err := c.SpeakAs(ctx, v, text)
		if err != nil {
			t.Fatalf("SpeakAs(%s): %v", v, err)
		}
		t.Logf("%s: %d bytes, %v of speech, made in %v", v, len(got.Data), got.Duration, time.Since(start))
		if got.MIMEType != piper.MIMEWAV || !bytes.HasPrefix(got.Data, []byte("RIFF")) {
			t.Errorf("%s: not a WAV: %q", v, got.MIMEType)
		}
		if got.Duration < 4*time.Second || got.Duration > 20*time.Second {
			t.Errorf("%s: Duration = %v, a text of this size lasts between 4 and 20 seconds", v, got.Duration)
		}
		samples[string(v)+".wav"] = got.Data
	}
	if ffmpeg := os.Getenv("KOKORO_FFMPEG"); ffmpeg != "" {
		oc := cfg
		oc.Format, oc.FFmpegPath = piper.FormatOpus, ffmpeg
		client, err := piper.New(oc)
		if err != nil {
			t.Fatalf("New (opus): %v", err)
		}
		start := time.Now()
		note, err := client.Speak(ctx, text)
		if err != nil {
			t.Fatalf("Speak (opus): %v", err)
		}
		t.Logf("opus: %d bytes, %v of speech, made in %v", len(note.Data), note.Duration, time.Since(start))
		if note.MIMEType != piper.MIMEOpus || !bytes.HasPrefix(note.Data, []byte("OggS")) {
			t.Errorf("not an Ogg Opus note: %q", note.MIMEType)
		}
		samples["pf_dora.ogg"] = note.Data
	}
	if dir := os.Getenv("KOKORO_SAMPLES"); dir != "" {
		for name, data := range samples {
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}
