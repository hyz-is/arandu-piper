package unit_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hyz-is/arandu-piper"
)

// twoVoices gives the rig a faber voice beside its cadu one, with cadu as the
// default, and a piper that logs its arguments.
func twoVoices(t *testing.T) *rig {
	t.Helper()
	r := newRig(t, "")
	script(t, r.dir, "piper", fmt.Sprintf("echo \"piper $*\" >> %s/calls.log\ncat >/dev/null\nwhile [ $# -gt 0 ]; do [ \"$1\" = \"-f\" ] && out=$2; shift; done\n%s\n", r.dir, r.okPiper()))
	faber := filepath.Join(r.dir, string(piper.VoiceFaber)+".onnx")
	if err := os.WriteFile(faber, []byte("another voice"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(faber+".json", []byte(`{"num_speakers": 1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	r.cfg.Voices = piper.VoicesIn(r.dir, piper.VoiceCadu, piper.VoiceFaber)
	r.cfg.DefaultVoice = piper.VoiceCadu
	return r
}

func TestOneClientSpeaksWithEachVoiceItHolds(t *testing.T) {
	r := twoVoices(t)
	c := r.client(t)
	if got := c.Voices(); len(got) != 2 || got[0] != piper.VoiceCadu || got[1] != piper.VoiceFaber {
		t.Fatalf("Voices = %v", got)
	}
	if c.DefaultVoice() != piper.VoiceCadu {
		t.Fatalf("DefaultVoice = %v", c.DefaultVoice())
	}
	ctx := context.Background()
	if _, err := c.Speak(ctx, pronto); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SpeakAs(ctx, piper.VoiceFaber, pronto); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SpeakAs(ctx, "", pronto); err != nil {
		t.Fatal(err)
	}
	calls := r.calls(t)
	if len(calls) != 3 {
		t.Fatalf("calls = %q", calls)
	}
	for i, want := range []piper.Voice{piper.VoiceCadu, piper.VoiceFaber, piper.VoiceCadu} {
		if !strings.Contains(calls[i], "/"+string(want)+".onnx ") {
			t.Errorf("call %d should use %s: %q", i, want, calls[i])
		}
	}
}

func TestAVoiceTheClientDoesNotHoldIsRefusedBeforeAnythingRuns(t *testing.T) {
	r := newRig(t, neverRuns)
	c := r.client(t)
	_, err := c.SpeakAs(context.Background(), piper.VoiceFaber, pronto)
	if !errors.Is(err, piper.ErrUnknownVoice) {
		t.Fatalf("err = %v", err)
	}
	if _, err := c.Key(piper.VoiceFaber, pronto); !errors.Is(err, piper.ErrUnknownVoice) {
		t.Fatalf("Key err = %v", err)
	}
	if calls := r.calls(t); len(calls) != 0 {
		t.Errorf("programs ran: %q", calls)
	}
}

func TestASingleVoiceNeedsNoDefault(t *testing.T) {
	r := newRig(t, "")
	if got := r.client(t).DefaultVoice(); got != piper.VoiceCadu {
		t.Fatalf("DefaultVoice = %v", got)
	}
}

func TestParseVoice(t *testing.T) {
	for in, want := range map[string]piper.Voice{
		"faber":                  piper.VoiceFaber,
		" Cadu ":                 piper.VoiceCadu,
		"pt_BR-faber-medium":     piper.VoiceFaber,
		"en_US-lessac-medium":    "en_US-lessac-medium",
		"pt_PT-tugão-medium":     "",
		"luciana":                "",
		"":                       "",
		"../etc/passwd":          "",
		"pt_BR-cadu-medium.onnx": "",
	} {
		got, err := piper.ParseVoice(in)
		if want == "" {
			if !errors.Is(err, piper.ErrUnknownVoice) {
				t.Errorf("ParseVoice(%q) = %q, %v; want ErrUnknownVoice", in, got, err)
			}
			continue
		}
		if err != nil || got != want {
			t.Errorf("ParseVoice(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestVoicesInNamesTheFilesByTheCatalogue(t *testing.T) {
	got := piper.VoicesIn("/opt/voices", piper.VoiceFaber)
	if got[piper.VoiceFaber].ModelPath != filepath.Join("/opt/voices", "pt_BR-faber-medium.onnx") || got[piper.VoiceFaber].ConfigPath != "" {
		t.Fatalf("VoicesIn = %+v", got)
	}
}

func TestTheKeyChangesWithWhatChangesTheAudioAndOnlyWithThat(t *testing.T) {
	r := twoVoices(t)
	c := r.client(t)
	key := func(c *piper.Client, v piper.Voice, text string) string {
		t.Helper()
		k, err := c.Key(v, text)
		if err != nil {
			t.Fatal(err)
		}
		if len(k) != 64 || strings.Trim(k, "0123456789abcdef") != "" {
			t.Fatalf("key %q is not 64 hexadecimal characters", k)
		}
		return k
	}
	base := key(c, piper.VoiceCadu, "Anotado. Está certo?")

	same := map[string]string{
		"the default voice":     key(c, "", "Anotado. Está certo?"),
		"other spacing":         key(c, piper.VoiceCadu, "  Anotado.\n\nEstá   certo? "),
		"another client, alike": key(r.client(t), piper.VoiceCadu, "Anotado. Está certo?"),
	}
	for name, k := range same {
		if k != base {
			t.Errorf("%s changed the key", name)
		}
	}

	slower := r.config()
	slower.LengthScale = 1.2
	cs, err := piper.New(slower)
	if err != nil {
		t.Fatal(err)
	}
	opus := r.config()
	opus.Format, opus.FFmpegPath = piper.FormatOpus, filepath.Join(r.dir, "ffmpeg")
	co, err := piper.New(opus)
	if err != nil {
		t.Fatal(err)
	}
	pause := r.config()
	pause.SentenceSilence = 500 * time.Millisecond
	cp, err := piper.New(pause)
	if err != nil {
		t.Fatal(err)
	}
	different := map[string]string{
		"another voice":     key(c, piper.VoiceFaber, "Anotado. Está certo?"),
		"another word":      key(c, piper.VoiceCadu, "Anotado. Está errado?"),
		"other punctuation": key(c, piper.VoiceCadu, "Anotado, está certo?"),
		"other case":        key(c, piper.VoiceCadu, "ANOTADO. Está certo?"),
		"another pace":      key(cs, piper.VoiceCadu, "Anotado. Está certo?"),
		"another format":    key(co, piper.VoiceCadu, "Anotado. Está certo?"),
		"another pause":     key(cp, piper.VoiceCadu, "Anotado. Está certo?"),
	}
	seen := map[string]string{base: "base"}
	for name, k := range different {
		if prev, ok := seen[k]; ok {
			t.Errorf("%s has the key of %s", name, prev)
		}
		seen[k] = name
	}

	// A new version of the voice, with another JSON, is another audio.
	if err := os.WriteFile(r.voice.ModelPath+".json", []byte(`{"num_speakers": 1, "piper_version": "1.8.0"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if key(r.client(t), piper.VoiceCadu, "Anotado. Está certo?") == base {
		t.Error("a changed voice kept the key")
	}

	if _, err := c.Key("", " "); !errors.Is(err, piper.ErrEmptyText) {
		t.Errorf("empty text: %v", err)
	}
	if calls := r.calls(t); len(calls) != 0 {
		t.Errorf("Key ran programs: %q", calls)
	}
}

func TestTheFakeRecordsTheVoice(t *testing.T) {
	f := piper.NewFake(piper.Answer{}, piper.Answer{})
	ctx := context.Background()
	if _, err := f.Speak(ctx, "um"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.SpeakAs(ctx, piper.VoiceFaber, "dois"); err != nil {
		t.Fatal(err)
	}
	calls := f.Calls()
	if len(calls) != 2 || calls[0] != (piper.Call{Text: "um"}) || calls[1] != (piper.Call{Voice: piper.VoiceFaber, Text: "dois"}) {
		t.Fatalf("calls = %+v", calls)
	}
}
