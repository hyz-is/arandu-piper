package piper

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// The values New applies when KokoroConfig leaves a field at its zero value.
const (
	// DefaultKokoroThreads is how many threads onnxruntime may use for one
	// Kokoro call. Two is what a small server shares between calls; the
	// program starts as many threads as it sees cores when nothing limits it,
	// which under a CPU quota is slower than a few.
	DefaultKokoroThreads = 2
	// MaxKokoroThreads is the largest KokoroConfig.Threads New accepts.
	MaxKokoroThreads = 16
	// maxVoicesFile bounds the voices file read at boot.
	maxVoicesFile = 256 << 20
)

// KokoroConfig turns the Kokoro engine on: Kokoro-82M, run through the
// kokoro-speak program of docs/kokoro-speak.py, which this package starts the
// way it starts Piper (no shell, fixed arguments, the text on standard input,
// a private directory per call, no inherited environment). The zero value
// leaves the engine off and the client is Piper only, as before.
//
// The voices it serves are Config.Kokoro.Voices (every one of KokoroVoices by
// default); a call chooses one with SpeakAs, and Config.DefaultVoice may name
// one of them. Config.LengthScale and Config.SentenceSilence apply to the
// Kokoro voices too (the pace is 1/LengthScale); Config.NoiseScale and
// Config.NoiseW are Piper's and do not reach Kokoro.
type KokoroConfig struct {
	// ProgramPath is kokoro-speak: docs/kokoro-speak.py with its first line
	// pointing at the interpreter that has the packages of
	// docs/kokoro-requirements.txt. An absolute path, a relative one, or a bare
	// name looked up in PATH. Required to turn the engine on.
	ProgramPath string
	// ModelPath is the Kokoro model, an .onnx file (kokoro-v1.0.onnx, or the
	// int8 one, a third of the size and about twice as fast). Required.
	ModelPath string
	// VoicesPath is the voices file that goes with the model
	// (voices-v1.0.bin). Every voice asked for must be in it. Required.
	VoicesPath string
	// Voices are the Kokoro voices to serve. Empty means KokoroVoices(). Each
	// must be one of KokoroVoices and be in VoicesPath, or New refuses.
	Voices []Voice
	// Threads is how many threads onnxruntime may use for one call, 1 to
	// MaxKokoroThreads. Zero means DefaultKokoroThreads.
	Threads int
	// Env holds extra "KEY=VALUE" entries for the program, after Config.Env,
	// such as "PYTHONPATH=/opt/kokoro" when the packages were installed with
	// pip --target. The program gets no other variable of the application's.
	Env []string
}

// enabled reports whether any field was set: a half-written configuration is
// then checked and refused, never ignored.
func (k KokoroConfig) enabled() bool {
	return k.ProgramPath != "" || k.ModelPath != "" || k.VoicesPath != "" ||
		len(k.Voices) > 0 || k.Threads != 0 || len(k.Env) > 0
}

// kokoroEngine is a KokoroConfig that New has checked.
type kokoroEngine struct {
	program  string
	model    string
	voices   string
	threads  int
	env      []string
	identity string
	names    []Voice
}

// resolveKokoro checks every part of the Kokoro configuration, so a missing
// program, model or voice fails at boot and not on the first call.
func resolveKokoro(k KokoroConfig) (*kokoroEngine, error) {
	program, err := resolveProgram(k.ProgramPath)
	if err != nil {
		return nil, fmt.Errorf("piper: Kokoro.ProgramPath: %w", err)
	}
	model, err := resolveFile(k.ModelPath)
	if err != nil {
		return nil, fmt.Errorf("piper: Kokoro.ModelPath: %w", err)
	}
	voices, err := resolveFile(k.VoicesPath)
	if err != nil {
		return nil, fmt.Errorf("piper: Kokoro.VoicesPath: %w", err)
	}

	threads := k.Threads
	if threads == 0 {
		threads = DefaultKokoroThreads
	}
	if threads < 1 || threads > MaxKokoroThreads {
		return nil, fmt.Errorf("piper: Kokoro.Threads %d is outside 1 to %d", k.Threads, MaxKokoroThreads)
	}
	if err := checkEnv("Kokoro.Env", k.Env); err != nil {
		return nil, err
	}

	names := k.Voices
	if len(names) == 0 {
		names = KokoroVoices()
	}
	seen := map[Voice]bool{}
	for _, v := range names {
		if v.Engine() != EngineKokoro {
			return nil, fmt.Errorf("piper: Kokoro.Voices: %q is not a Kokoro voice (%s)", v, joinVoices(KokoroVoices()))
		}
		if seen[v] {
			return nil, fmt.Errorf("piper: Kokoro.Voices: %q is listed twice", v)
		}
		seen[v] = true
	}

	inFile, voicesSum, err := readVoicesFile(voices)
	if err != nil {
		return nil, fmt.Errorf("piper: Kokoro.VoicesPath: %w", err)
	}
	for _, v := range names {
		if !inFile[string(v)] {
			return nil, fmt.Errorf("piper: Kokoro.Voices: %q is not in the voices file %q", v, voices)
		}
	}

	info, err := os.Stat(model)
	if err != nil {
		return nil, fmt.Errorf("piper: Kokoro.ModelPath: %w", err)
	}
	h := sha256.New()
	fmt.Fprintf(h, "kokoro-onnx\x00%d\x00%s", info.Size(), voicesSum)

	return &kokoroEngine{
		program:  program,
		model:    model,
		voices:   voices,
		threads:  threads,
		env:      append([]string(nil), k.Env...),
		identity: hex.EncodeToString(h.Sum(nil)),
		names:    append([]Voice(nil), names...),
	}, nil
}

// readVoicesFile reads the voices file of Kokoro, a NumPy .npz archive with one
// "<voice>.npy" entry per voice, and returns the voices it holds and the hash of
// the file, which is part of every Kokoro cache key.
func readVoicesFile(path string) (map[string]bool, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", errors.New("cannot be read")
	}
	if info.Size() > maxVoicesFile {
		return nil, "", fmt.Errorf("%q is larger than %d MB, it is not a Kokoro voices file", path, maxVoicesFile>>20)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", errors.New("cannot be read")
	}
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, "", fmt.Errorf("%q is not a Kokoro voices file (a .npz archive)", path)
	}
	voices := map[string]bool{}
	for _, f := range archive.File {
		if name, ok := strings.CutSuffix(f.Name, ".npy"); ok {
			voices[name] = true
		}
	}
	if len(voices) == 0 {
		return nil, "", fmt.Errorf("%q holds no voice", path)
	}
	sum := sha256.Sum256(raw)
	return voices, hex.EncodeToString(sum[:]), nil
}

// checkEnv checks "KEY=VALUE" entries.
func checkEnv(field string, env []string) error {
	for _, entry := range env {
		if i := strings.IndexByte(entry, '='); i < 1 || strings.ContainsRune(entry, 0) {
			return fmt.Errorf("piper: %s entry %q is not KEY=VALUE", field, entry)
		}
	}
	return nil
}

// synthesizeKokoro runs kokoro-speak. The text goes in through standard input,
// as for Piper; the voice, the pace and the paths are one argument each, and
// every one is a value New has checked, so none can look like a flag.
func (c *Client) synthesizeKokoro(ctx context.Context, dir string, voice Voice, spoken, original, out string) error {
	k := c.kokoro
	args := []string{
		"--model", k.model,
		"--voices", k.voices,
		"--voice", string(voice),
		"--lang", kokoroLanguages[voice],
		"--threads", strconv.Itoa(k.threads),
		"--output", out,
	}
	if c.lengthScale != 0 {
		args = append(args, "--speed", strconv.FormatFloat(1/c.lengthScale, 'f', -1, 64))
	}
	if c.silence > 0 {
		args = append(args, "--sentence-pause", strconv.FormatFloat(c.silence.Seconds(), 'f', -1, 64))
	}
	return c.run(ctx, dir, k.program, ErrFailed, k.env, strings.NewReader(spoken+"\n"), []string{spoken, original}, args...)
}
