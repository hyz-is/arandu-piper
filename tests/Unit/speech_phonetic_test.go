package unit_test

import (
	"strings"
	"testing"
	"unicode"

	"github.com/hyz-is/arandu-piper"
)

func TestLetterName(t *testing.T) {
	want := map[rune]string{
		'a': "á", 'b': "bê", 'c': "cê", 'd': "dê", 'e': "êh", 'f': "éfe", 'g': "gê", 'h': "agá", 'i': "i", 'j': "jota",
		'k': "cá", 'l': "éle", 'm': "ême", 'n': "êne", 'o': "ôh", 'p': "pê", 'q': "quê", 'r': "érre", 's': "ésse", 't': "tê",
		'u': "u", 'v': "vê", 'w': "dáblio", 'x': "xis", 'y': "ípsilon", 'z': "zê",
		'A': "á", 'W': "dáblio", 'ç': "cê", 'é': "êh",
	}
	for r, name := range want {
		if got := piper.LetterName(r); got != name {
			t.Errorf("LetterName(%q) = %q, want %q", r, got, name)
		}
	}
	for _, r := range "1-. 日" {
		if got := piper.LetterName(r); got != "" {
			t.Errorf("LetterName(%q) = %q, want nothing", r, got)
		}
	}
}

func TestSpellAcronym(t *testing.T) {
	for in, want := range map[string]string{
		"GPU":     "gê pê u",
		"ALPR":    "á éle pê érre",
		"ABC1D23": "á bê cê um dê dois três",
		"Wi-Fi":   "dáblio i éfe i",
		"":        "",
	} {
		if got := piper.SpellAcronym(in); got != want {
			t.Errorf("SpellAcronym(%q) = %q, want %q", in, got, want)
		}
	}
}

// A token in capitals the dictionary does not know is spelled or said as a word
// by the shape of its syllables.
func TestReadsAsWord(t *testing.T) {
	for token, want := range map[string]bool{
		"NASA": true, "SIM": true, "NÃO": true, "ATENÇÃO": true, "UFO": true, "TREINO": true, "OI": true, "EU": true,
		"GPU": false, "ALPR": false, "SDK": false, "RTSP": false, "OCR": false, "USB": false, "IMC": false, "XYZ": false, "BR": false, "CTRL": false,
	} {
		if got := piper.ReadsAsWord(token); got != want {
			t.Errorf("ReadsAsWord(%q) = %v, want %v", token, got, want)
		}
	}
}

// The five cases of the brief, from fitness and general use, as the products write them.
func TestSpeakableTheCasesOfTheProducts(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"sets, reps and load", "Faça supino 3x12 com 22,5 kg.", "Faça supino três séries de doze com vinte e dois vírgula cinco quilos."},
		{"the contact address of the product", "Escreva para ajuda@ctrl.fit.", "Escreva para ajuda arroba cê tê érre éle ponto fít."},
		{"HIIT is said as a word", "Faça HIIT hoje.", "Faça rít hoje."},
		{"whey", "Tome whey depois do treino.", "Tome uêi depois do treino."},
		{"BMI with a decimal", "Seu IMC 24,7 é normal.", "Seu i ême cê vinte e quatro vírgula sete é normal."},
		{"the line of a workout", "Hoje: supino 3x12 com 22,5 kg, descanso de 60-90 s.", "Hoje: supino três séries de doze com vinte e dois vírgula cinco quilos, descanso de sessenta a noventa segundos."},
		{"basal rate and effort", "Pré-treino com TMB de 1.500 kcal e RPE 7.", "Pré-treino com tê ême bê de mil e quinhentas calorias e érre pê êh sete."},
		{"borrowed words of the gym", "Treine com o CrossFit e faça check-in no app.", "Treine com o cróssfit e faça tchék ín no áp."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := piper.Speakable(tc.in)
			if got != tc.want {
				t.Errorf("Speakable(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
			if again := piper.Speakable(got); again != got {
				t.Errorf("not idempotent: Speakable(%q)\n got %q\nwant %q", got, again, got)
			}
		})
	}
}

// Acronyms: spelled with the names of the letters, or said as a word; the
// dictionary decides first and the shape of the syllables decides the rest.
func TestSpeakableAcronyms(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"spelled, in the dictionary", "O ALPR usa GPU, API e SDK.", "O á éle pê érre usa gê pê u, á pê i e ésse dê cá."},
		{"read as a word, in the dictionary", "A NASA e o SaaS usam JSON.", "A nása e o sás usam djêison."},
		{"read as a word by its shape", "Responda SIM ou NÃO. Veja o UFO.", "Responda sim ou não. Veja o ufo."},
		{"spelled by its shape", "Resultado de ABC e XYZ.", "Resultado de á bê cê e xis ípsilon zê."},
		{"two letters", "Estado do RJ e do BR.", "Estado do érre jota e do bê érre."},
		{"a plural", "Os APIs e as CPUs são rápidos.", "Os á pê is e as cê pê us são rápidos."},
		{"health and training", "Medi o IMC, a TMB e o RIR.", "Medi o i ême cê, a tê ême bê e o érre i érre."},
		{"documents", "Cadastre o CPF e a CNH.", "Cadastre o cê pê éfe e a cê êne agá."},
		{"the State", "Fale com a ONG e o SUS na UTI.", "Fale com a ôngue e o sus na u tê i."},
		{"a unit after a number is not spelled", "A câmera grava 60 FPS com 10 GB.", "A câmera grava sessenta quadros por segundo com dez guigabáites."},
		{"a roman numeral is left alone", "Século XX.", "Século XX."},
		{"the dictionary beats the longest term first", "O ALPR.sh é do ALPR.", "O á éle pê érre ponto ésse agá é do á éle pê érre."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := piper.Speakable(tc.in)
			if got != tc.want {
				t.Errorf("Speakable(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
			if again := piper.Speakable(got); again != got {
				t.Errorf("not idempotent: Speakable(%q)\n got %q\nwant %q", got, again, got)
			}
		})
	}
}

// Domains, addresses and e-mails are said in the speech text only.
func TestSpeakableDomainsAndEmails(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"a domain of the dictionary", "Veja alpr.sh.", "Veja á éle pê érre ponto ésse agá."},
		{"com and br", "Veja exemplo.com.br.", "Veja exemplo ponto com ponto bê érre."},
		{"a short path", "Veja exemplo.io/login.", "Veja exemplo ponto i ôh barra lóguin."},
		{"www", "Acesse www.exemplo.com agora.", "Acesse dáblio dáblio dáblio ponto exemplo ponto com agora."},
		{"a name with no vowel is spelled", "Veja ctrl.fit/conta.", "Veja cê tê érre éle ponto fít barra conta."},
		{"an e-mail", "Escreva para contato@exemplo.com.br.", "Escreva para contato arroba exemplo ponto com ponto bê érre."},
		{"an e-mail with a dot, an underscore and a digit", "Meu e-mail é joao.silva_01@exemplo.com.", "Meu imêil é joao ponto silva underline um arroba exemplo ponto com."},
		{"an e-mail of the products", "Escreva para ajuda@hyz.is.", "Escreva para ajuda arroba hyz ponto i ésse."},
		{"an address that cannot be dictated stays the link", "Veja https://exemplo.com/treino?id=1 agora.", "Veja o linque agora."},
		{"not a domain", "Isso é exemplo.comida mesmo.", "Isso é exemplo.comida mesmo."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := piper.Speakable(tc.in)
			if got != tc.want {
				t.Errorf("Speakable(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}

// Foreign words and brands are written the way a Brazilian says them.
func TestSpeakableForeignWords(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"technology", "O framework do dashboard no cloud com Docker e Redis.", "O frêim uârk do désh bórd no cláud com dóquer e rédis."},
		{"brands", "O Wi-Fi do WhatsApp e o login do Stripe.", "O uái fái do uóts áp e o lóguin do stráip."},
		{"product words", "Dê um feedback online pelo link do site.", "Dê um fídbék onláin pelo linque do sáite."},
		{"a word inside another is left alone", "O servidor, o cloudflare e o loginho.", "O servidor, o cloudflare e o loginho."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := piper.Speakable(tc.in); got != tc.want {
				t.Errorf("Speakable(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}

// espeak-ng reads a lone "í" as "i agudo", "ê" as "e circunflexo", "ô" and "ú" the
// same way: measured with Kokoro and Piper, whisper heard "iagudo p" for "í pê".
// No word of the dictionary, of a letter's name or of what Speakable makes may be
// one of those four letters on its own.
func TestNoLetterIsSaidAsALoneAccentedVowel(t *testing.T) {
	lone := func(where, text string) {
		for _, w := range strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) }) {
			switch w {
			case "í", "ê", "ô", "ú", "Í", "Ê", "Ô", "Ú":
				t.Errorf("%s: %q has the lone letter %q, which an engine spells out", where, text, w)
			}
		}
	}
	for r := 'a'; r <= 'z'; r++ {
		lone("LetterName("+string(r)+")", piper.LetterName(r))
	}
	for _, e := range piper.BaseLexicon() {
		lone("dictionary "+e.Written, e.Said)
	}
	for _, in := range []string{
		"O IP, a UI, o UX, a SEO, o ROI, o ERP e o LED.", "Escreva para ajuda@exemplo.io, a@exemplo.ai, b@exemplo.is, c@exemplo.me e d@exemplo.us.",
		"Placa ABC1D23, código XEOI9.", "Veja o ID e o RPE.",
	} {
		lone("Speakable", piper.Speakable(in))
	}
}
