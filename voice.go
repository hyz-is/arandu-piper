package piper

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Voice names a voice. A Piper voice is named by the base name of its files,
// the name it has in the rhasspy/piper-voices catalogue: "pt_BR-faber-medium"
// is the pair pt_BR-faber-medium.onnx and pt_BR-faber-medium.onnx.json. A
// Kokoro voice is named as the Kokoro model names it: "pf_dora". The name
// decides the engine (see Engine). One client holds every voice it was
// configured with, whatever their engines, and each call says which one speaks.
type Voice string

// Engine is the program that makes a voice's speech.
type Engine string

const (
	// EnginePiper is Piper, the engine of every voice named like a catalogue
	// file (pt_BR-faber-medium).
	EnginePiper Engine = "piper"
	// EngineKokoro is Kokoro-82M, run by docs/kokoro-speak.py, the engine of
	// the voices of KokoroVoices.
	EngineKokoro Engine = "kokoro"
)

// The Brazilian Portuguese voices this package is tested with. Both are
// published under licenses that allow commercial use; the README says what
// each one asks of the application.
const (
	// VoiceFaber is a male voice, trained on the Hi-Fi TTS dataset. Its
	// license asks the application to credit that dataset.
	VoiceFaber Voice = "pt_BR-faber-medium"
	// VoiceCadu is a male voice published in the public domain (CC0).
	VoiceCadu Voice = "pt_BR-cadu-medium"
)

// The Brazilian Portuguese voices of Kokoro-82M (Apache-2.0), run by the
// Kokoro engine. They need Config.Kokoro; see the README.
const (
	// VoiceDora is a female voice (Kokoro's pf_dora).
	VoiceDora Voice = "pf_dora"
	// VoiceAlex is a male voice (Kokoro's pm_alex).
	VoiceAlex Voice = "pm_alex"
	// VoiceSanta is a male voice (Kokoro's pm_santa).
	VoiceSanta Voice = "pm_santa"
)

// kokoroLanguages are the Kokoro voices this package serves, with the espeak-ng
// language that turns their text into phonemes.
var kokoroLanguages = map[Voice]string{
	VoiceDora:  "pt-br",
	VoiceAlex:  "pt-br",
	VoiceSanta: "pt-br",
}

// KokoroVoices lists the voices the Kokoro engine serves, in a stable order.
func KokoroVoices() []Voice { return []Voice{VoiceDora, VoiceAlex, VoiceSanta} }

// Engine is the engine that speaks the voice: Kokoro for the voices of
// KokoroVoices, Piper for any other name.
func (v Voice) Engine() Engine {
	if _, ok := kokoroLanguages[v]; ok {
		return EngineKokoro
	}
	return EnginePiper
}

// ErrUnknownVoice means a call named a voice the client was not configured
// with, or ParseVoice was given a name that is not a voice's.
var ErrUnknownVoice = errors.New("piper: unknown voice")

// shortNames are the names ParseVoice accepts besides a voice's full name, so
// an environment variable can say PIPER_VOICE=cadu.
var shortNames = map[string]Voice{
	"faber": VoiceFaber,
	"cadu":  VoiceCadu,
	"dora":  VoiceDora,
	"alex":  VoiceAlex,
	"santa": VoiceSanta,
}

// voiceName is the shape of a catalogue name: language_REGION-name-quality.
var voiceName = regexp.MustCompile(`^[a-z]{2,3}_[A-Z]{2}-[a-z0-9_]+-(x_low|low|medium|high)$`)

// ParseVoice reads a voice from configuration. It accepts the short names
// "faber", "cadu", "dora", "alex" and "santa", in any case, the Kokoro names
// ("pf_dora", "pm_alex", "pm_santa") and any full Piper catalogue name such as
// "pt_BR-faber-medium" or "en_US-lessac-medium". It answers ErrUnknownVoice
// for anything else, so a typo in the environment fails at boot.
func ParseVoice(s string) (Voice, error) {
	s = strings.TrimSpace(s)
	if v, ok := shortNames[strings.ToLower(s)]; ok {
		return v, nil
	}
	if _, ok := kokoroLanguages[Voice(s)]; ok {
		return Voice(s), nil
	}
	if voiceName.MatchString(s) {
		return Voice(s), nil
	}
	return "", fmt.Errorf("%w: %q is neither faber, cadu, dora, alex, santa nor a Piper catalogue name like pt_BR-faber-medium", ErrUnknownVoice, s)
}

// Files is where a Piper voice's files are when they keep their catalogue names
// in dir: dir/<voice>.onnx, with its JSON beside it. A Kokoro voice has no
// files of its own: the model and the voices file are Config.Kokoro's.
func (v Voice) Files(dir string) VoiceFiles {
	return VoiceFiles{ModelPath: filepath.Join(dir, string(v)+".onnx")}
}

// VoiceFiles is one voice on disk.
type VoiceFiles struct {
	// ModelPath is the voice, an .onnx file. Required.
	ModelPath string
	// ConfigPath is the voice's JSON configuration. Empty means ModelPath plus
	// ".json", where Piper itself looks for it.
	ConfigPath string
	// SpeakerID selects the speaker of a multi-speaker voice. Zero is the
	// first speaker, and the only one of a single-speaker voice. New refuses a
	// number the voice does not have.
	SpeakerID int
}

// VoicesIn is the Voices of a client whose Piper voices all keep their
// catalogue names in one directory. Kokoro voices are not listed here: they go
// in Config.Kokoro.
//
//	piper.VoicesIn("/opt/piper/voices", piper.VoiceFaber, piper.VoiceCadu)
func VoicesIn(dir string, voices ...Voice) map[Voice]VoiceFiles {
	out := make(map[Voice]VoiceFiles, len(voices))
	for _, v := range voices {
		out[v] = v.Files(dir)
	}
	return out
}

// voiceModel is a voice New has checked.
type voiceModel struct {
	engine  Engine
	model   string
	config  string
	speaker int
	// identity changes whenever the voice's files change: a hash of its JSON
	// and the size of its model. It is part of every cache key, so a new
	// version of a voice is never answered with audio of the old one.
	identity string
}

// resolveVoice checks one voice's files.
func resolveVoice(v Voice, files VoiceFiles) (voiceModel, error) {
	field := func(name string) string { return fmt.Sprintf("Voices[%q].%s", v, name) }

	model, err := resolveFile(files.ModelPath)
	if err != nil {
		return voiceModel{}, fmt.Errorf("piper: %s: %w", field("ModelPath"), err)
	}
	configPath := files.ConfigPath
	name := field("ConfigPath")
	if strings.TrimSpace(configPath) == "" {
		configPath = model + ".json"
		name = field("ConfigPath (the model's .json)")
	}
	configPath, err = resolveFile(configPath)
	if err != nil {
		return voiceModel{}, fmt.Errorf("piper: %s: %w", name, err)
	}
	speakers, err := readVoice(configPath)
	if err != nil {
		return voiceModel{}, fmt.Errorf("piper: %s: %w", field("ConfigPath"), err)
	}
	if files.SpeakerID < 0 {
		return voiceModel{}, fmt.Errorf("piper: %s must not be negative", field("SpeakerID"))
	}
	if speakers > 0 && files.SpeakerID >= speakers {
		return voiceModel{}, fmt.Errorf("piper: %s %d, but the voice has %d speaker(s)", field("SpeakerID"), files.SpeakerID, speakers)
	}
	identity, err := voiceIdentity(model, configPath)
	if err != nil {
		return voiceModel{}, fmt.Errorf("piper: %s: %w", field("ModelPath"), err)
	}
	return voiceModel{engine: EnginePiper, model: model, config: configPath, speaker: files.SpeakerID, identity: identity}, nil
}

// voiceIdentity hashes the voice's JSON and the model's size. Hashing the
// model itself, some 60 MB, would slow every boot for nothing: a retrained
// voice comes with a new JSON.
func voiceIdentity(model, config string) (string, error) {
	info, err := os.Stat(model)
	if err != nil {
		return "", err
	}
	f, err := os.Open(config)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	fmt.Fprintf(h, "%d\x00", info.Size())
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// sortedVoices lists the voices of a map in a stable order.
func sortedVoices[T any](m map[Voice]T) []Voice {
	out := make([]Voice, 0, len(m))
	for v := range m {
		out = append(out, v)
	}
	slices.Sort(out)
	return out
}
