package unit_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hyz-is/arandu-piper"
)

// NormalizerVersion names the rules and the dictionary, and the two must agree.
func TestTheVersionNamesTheRulesAndTheDictionary(t *testing.T) {
	want := "ptbr-2.lex" + itoa(piper.LexiconVersion())
	if piper.NormalizerVersion != want {
		t.Errorf("NormalizerVersion = %q, but the dictionary is version %d: want %q", piper.NormalizerVersion, piper.LexiconVersion(), want)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestTheSharedDictionaryIsValid(t *testing.T) {
	entries, err := piper.ParseLexicon(piper.LexiconJSON())
	if err != nil {
		t.Fatalf("the shared dictionary does not parse: %v", err)
	}
	if len(entries) < 100 {
		t.Errorf("the shared dictionary has %d entries", len(entries))
	}
	for i, e := range entries {
		if strings.TrimSpace(e.Said) == "" {
			t.Errorf("%q is said as nothing", e.Written)
		}
		if i > 0 && utf8.RuneCountInString(entries[i-1].Written) < utf8.RuneCountInString(e.Written) {
			t.Fatalf("%q comes before the longer %q: the longest term must come first", entries[i-1].Written, e.Written)
		}
	}
	base := piper.BaseLexicon()
	if len(base) != len(entries) {
		t.Errorf("BaseLexicon has %d entries, the file %d", len(base), len(entries))
	}
	base[0].Said = "changed"
	if piper.BaseLexicon()[0].Said == "changed" {
		t.Error("BaseLexicon returned its own slice: a caller changed the dictionary")
	}
}

func TestLexiconJSONIsACopy(t *testing.T) {
	a := piper.LexiconJSON()
	a[0] = 'X'
	if piper.LexiconJSON()[0] == 'X' {
		t.Error("LexiconJSON gave the bytes of the embedded file")
	}
	var doc struct {
		Language string `json:"language"`
	}
	if err := json.Unmarshal(piper.LexiconJSON(), &doc); err != nil || doc.Language != "pt-BR" {
		t.Errorf("language = %q, %v", doc.Language, err)
	}
}

func TestParseLexicon(t *testing.T) {
	entries, err := piper.ParseLexicon([]byte(`{
		"language": "pt-BR", "version": 3,
		"dictionary": {
			"ME": "ême",
			"supino": "supíno",
			"ALPR": "á éle pê érre",
			"ALPR.sh": "á éle pê érre ponto ésse agá",
			"PIX": {"said": "pics", "case": "any"},
			"Rio": {"said": "ríu", "case": "exact"}
		}}`))
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	byWritten := map[string]piper.LexiconEntry{}
	for _, e := range entries {
		order = append(order, e.Written)
		byWritten[e.Written] = e
	}
	pos := map[string]int{}
	for i, w := range order {
		pos[w] = i
	}
	if pos["ALPR.sh"] > pos["ALPR"] || order[0] != "ALPR.sh" {
		t.Errorf("order %v: the longest term first", order)
	}
	for written, any := range map[string]bool{"ME": false, "supino": true, "ALPR": false, "PIX": true, "Rio": false} {
		if byWritten[written].AnyCase != any {
			t.Errorf("%q AnyCase = %v, want %v", written, byWritten[written].AnyCase, any)
		}
	}
	// What ParseLexicon returns is what Speech.Lexicon takes.
	n := piper.NewNormalizer(piper.Speech{Lexicon: entries})
	if got := n.Speakable("Supino com ALPR.sh, o ME e o Pix. Rio e RIO.", time.Time{}); got != "supíno com á éle pê érre ponto ésse agá, o ême e o pics. ríu e rio." {
		t.Errorf("got %q", got)
	}
}

func TestParseLexiconRefusesWhatItCannotTrust(t *testing.T) {
	for name, doc := range map[string]string{
		"not json":         `nope`,
		"wrong language":   `{"language": "en-US", "dictionary": {"a": "b"}}`,
		"no language":      `{"dictionary": {"a": "b"}}`,
		"empty term":       `{"language": "pt-BR", "dictionary": {" ": "b"}}`,
		"said as nothing":  `{"language": "pt-BR", "dictionary": {"a": " "}}`,
		"unknown field":    `{"language": "pt-BR", "colour": 1, "dictionary": {"a": "b"}}`,
		"unknown case":     `{"language": "pt-BR", "dictionary": {"a": {"said": "b", "case": "some"}}}`,
		"unknown in entry": `{"language": "pt-BR", "dictionary": {"a": {"said": "b", "tone": 1}}}`,
		"a number":         `{"language": "pt-BR", "dictionary": {"a": 7}}`,
	} {
		if _, err := piper.ParseLexicon([]byte(doc)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// A dictionary of an application is applied before the shared one and wins.
func TestTheApplicationDictionaryWinsOverTheSharedOne(t *testing.T) {
	n := piper.NewNormalizer(piper.Speech{Lexicon: []piper.LexiconEntry{
		{Written: "ctrl.fit", Said: "control fit", AnyCase: true},
		{Written: "whey", Said: "uéi", AnyCase: true},
		{Written: "IMC", Said: "índice de massa corporal"},
	}})
	got := n.Speakable("Veja ctrl.fit: tome whey e meça o IMC 24,7.", time.Time{})
	want := "Veja control fit: tome uéi e meça o índice de massa corporal vinte e quatro vírgula sete."
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
	// And in an e-mail's domain.
	if got := n.Speakable("Escreva para ajuda@ctrl.fit.", time.Time{}); got != "Escreva para ajuda arroba control fit." {
		t.Errorf("e-mail: %q", got)
	}
}

// The plural of an acronym that is spelled is spelled with an "s".
func TestThePluralOfASpelledAcronym(t *testing.T) {
	for in, want := range map[string]string{
		"As APIs e os SDKs.": "As á pê is e os ésse dê cás.",
		"As CPUs.":           "As cê pê us.",
	} {
		if got := piper.Speakable(in); got != want {
			t.Errorf("Speakable(%q) = %q, want %q", in, got, want)
		}
	}
}

// Whatever the dictionary says, saying it again changes nothing, so a text that
// went through the verbalizer twice, or through an application that already
// spoke it, is the same.
func TestEveryEntryOfTheDictionaryIsSaidAndStaysSaid(t *testing.T) {
	for _, e := range piper.BaseLexicon() {
		in := "Veja " + e.Written + " agora."
		got := piper.Speakable(in)
		if !strings.Contains(strings.ToLower(got), strings.ToLower(e.Said)) {
			t.Errorf("Speakable(%q) = %q, missing %q", in, got, e.Said)
		}
		if again := piper.Speakable(got); again != got {
			t.Errorf("%q is not idempotent: %q then %q", e.Written, got, again)
		}
	}
}
