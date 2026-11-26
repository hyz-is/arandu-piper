package unit_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hyz-is/arandu-piper"
)

func TestAKokoroVoiceGetsTheSpeakableText(t *testing.T) {
	k := newKokoroRig(t, "")
	c := k.client(t)
	if _, err := c.Speak(context.Background(), "Próximo treino: *supino* 3x10 com 42,5kg às 06:30 💪"); err != nil {
		t.Fatalf("Speak: %v", err)
	}
	want := "Próximo treino: supino três séries de dez com quarenta e dois vírgula cinco quilos às seis e meia.\n"
	if got := k.read(t, "stdin.txt"); got != want {
		t.Errorf("stdin = %q, want %q", got, want)
	}
	if cli := k.call(t, "kokoro-speak"); !strings.Contains(cli, "--lang pt-br") || strings.Contains(cli, "--lang pt ") {
		t.Errorf("the language must be Brazilian Portuguese, pt-br: %q", cli)
	}
}

func TestRawSpeechLeavesTheTextToTheVoice(t *testing.T) {
	k := newKokoroRig(t, "")
	k.cfg.Speech.Raw = true
	if _, err := k.client(t).Speak(context.Background(), "Supino 3x10 com R$ 24,90"); err != nil {
		t.Fatalf("Speak: %v", err)
	}
	if got := k.read(t, "stdin.txt"); got != "Supino 3x10 com R$ 24,90\n" {
		t.Errorf("stdin = %q", got)
	}
}

func TestTheKeyFollowsTheSpokenTextAndNothingElse(t *testing.T) {
	c := newKokoroRig(t, "").client(t)
	key := func(text string) string {
		t.Helper()
		k, err := c.Key("", text)
		if err != nil {
			t.Fatalf("Key(%q): %v", text, err)
		}
		return k
	}
	// Two writings that are spoken the same make the same audio, so one key.
	if key("Custa R$ 1,00.") != key("Custa um real.") {
		t.Error("the same speech has two keys")
	}
	if key("Faça *3x10*") != key("Faça três séries de dez.") {
		t.Error("the same speech has two keys")
	}
	// A change that changes what is said changes the key.
	if key("Custa R$ 1,00.") == key("Custa R$ 2,00.") {
		t.Error("two speeches have one key")
	}
	// Raw speech keeps the key the text had before the verbalizer existed.
	raw := newKokoroRig(t, "")
	raw.cfg.Speech.Raw = true
	rc := raw.client(t)
	a, _ := rc.Key("", "Custa R$ 1,00.")
	b, _ := c.Key("", "Custa R$ 1,00.")
	if a == b {
		t.Error("a raw client and a speaking one share a key for a text they say differently")
	}
}

func TestTheClientTellsWhatItSaysAndWithWhichRules(t *testing.T) {
	k := newKokoroRig(t, "")
	c := k.client(t)
	if got := c.Speakable("R$ 24,90 em 26/11/2026"); got != "vinte e quatro reais e noventa centavos em vinte e seis de novembro de dois mil e vinte e seis." {
		t.Errorf("Speakable = %q", got)
	}
	if c.NormalizerVersion() != piper.NormalizerVersion {
		t.Errorf("version %q", c.NormalizerVersion())
	}
	k.cfg.Speech.Lexicon = []piper.LexiconEntry{{Written: "whey", Said: "uêi"}}
	custom := k.client(t)
	if custom.NormalizerVersion() == piper.NormalizerVersion || custom.Normalizer().Version() != custom.NormalizerVersion() {
		t.Errorf("version %q", custom.NormalizerVersion())
	}
	if got := custom.Speakable("Tome whey"); got != "Tome uêi." {
		t.Errorf("Speakable = %q", got)
	}
}

func TestTheLimitIsOnTheTextGivenAndTheSpokenTextMayGrow(t *testing.T) {
	k := newKokoroRig(t, "")
	k.cfg.MaxChars = 40
	c := k.client(t)
	// Fourteen characters, many more once spoken: accepted.
	if _, err := c.Speak(context.Background(), "R$ 1.234.567,89"); err != nil {
		t.Errorf("err = %v", err)
	}
	// Too long as given.
	if _, err := c.Speak(context.Background(), strings.Repeat("a ", 30)); !errors.Is(err, piper.ErrTextTooLong) {
		t.Errorf("err = %v", err)
	}
	// Nothing left to say after the rules.
	if _, err := c.Speak(context.Background(), "🎉 *** ---"); !errors.Is(err, piper.ErrEmptyText) {
		t.Errorf("err = %v", err)
	}
}
