package unit_test

import (
	"testing"
	"time"

	"github.com/hyz-is/arandu-piper"
)

// The five defects of 0.5.0 that Corujão.ai reported with examples. Each one
// has its own test, so none comes back.

var defectsNow = time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)

// 1. "vence dia 20/12" was said "vence dia dia vinte de dezembro".
func TestADateAfterTheWordDiaIsNotSaidWithItsOwnDia(t *testing.T) {
	n := piper.NewNormalizer(piper.Speech{})
	for _, tc := range []struct {
		name, in, now, without string
	}{
		{"after dia", "Vence dia 20/12.", "Vence dia vinte de dezembro.", "Vence dia vinte de dezembro."},
		{"the first", "Vence dia 1/12.", "Vence dia primeiro de dezembro.", "Vence dia primeiro de dezembro."},
		{"em dia", "Vence em dia 20/12.", "Vence em dia vinte de dezembro.", "Vence em dia vinte de dezembro."},
		{"no dia, with a year", "Vence no dia 05/12/2026.", "Vence no dia cinco de dezembro.", "Vence no dia cinco de dezembro de dois mil e vinte e seis."},
		{"a capital", "Dia 20/12 vence.", "Dia vinte de dezembro vence.", "Dia vinte de dezembro vence."},
		{"another year", "Vence dia 15/01/2027.", "Vence dia quinze de janeiro de dois mil e vinte e sete.", "Vence dia quinze de janeiro de dois mil e vinte e sete."},
		{"a day of this month", "Vence dia 30/11.", "Vence dia trinta.", "Vence dia trinta de novembro."},
		{"without the word dia the day is still counted", "Vence 20/12.", "Vence dia vinte de dezembro.", "Vence vinte de dezembro."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := n.Speakable(tc.in, defectsNow); got != tc.now {
				t.Errorf("with now: Speakable(%q)\n got %q\nwant %q", tc.in, got, tc.now)
			}
			if got := n.Speakable(tc.in, time.Time{}); got != tc.without {
				t.Errorf("without now: Speakable(%q)\n got %q\nwant %q", tc.in, got, tc.without)
			}
		})
	}
}

// 2. "Parcela 3/12" was read as the third of December.
func TestAnInstallmentIsNotADate(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"after parcela", "Parcela 3/12 paga.", "Parcela três de doze paga."},
		{"with a zero", "Parcela 03/12 paga.", "Parcela três de doze paga."},
		{"in a sentence", "Pague a parcela 3/12 hoje.", "Pague a parcela três de doze hoje."},
		{"the last one", "Parcela 12/12 quitada.", "Parcela doze de doze quitada."},
		{"with a number sign", "Parcela nº 4/10 paga.", "Parcela número quatro de dez paga."},
		{"plural", "Parcelas 10/12 e 11/12 pagas.", "Parcelas dez de doze e onze de doze pagas."},
		{"before the word", "Faltam 5/10 parcelas.", "Faltam cinco de dez parcelas."},
		{"prestação", "Prestação 7/24 vencida.", "Prestação sete de vinte e quatro vencida."},
		{"up to 48", "Parcela 5/48 paga.", "Parcela cinco de quarenta e oito paga."},
		{"a fraction word follows", "Parcela 1/2 paga.", "Parcela um de dois paga."},
		{"above 48 is not an installment", "Parcela 5/60 paga.", "Parcela cinco, sessenta paga."},
		{"the first above the second is a date", "Parcela 15/12 vence.", "Parcela quinze de dezembro vence."},
		{"no context, a date as before", "Vence em 3/12.", "Vence em três de dezembro."},
		{"no context, a fraction as before", "Misture 1/2 xícara.", "Misture meia xícara."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := piper.Speakable(tc.in)
			if got != tc.want {
				t.Errorf("Speakable(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
	// The validator does not take it for a date it failed to say.
	n := piper.NewNormalizer(piper.Speech{})
	for _, in := range []string{"Parcela 3/12 paga, vence 20/12.", "Faltam 5/10 parcelas.", "Parcela nº 4/10 paga."} {
		if speech, issues := n.Check(in, time.Time{}); piper.HasErrors(issues) {
			t.Errorf("Check(%q) = %q, %v", in, speech, issues)
		}
	}
	if issues := n.Validate("Parcela 3/12 paga.", "Parcela quatro de doze paga."); !piper.HasErrors(issues) {
		t.Error("an installment that changed is not an error")
	}
}

// 3. The lexicon of the application did not reach into an address.
func TestTheApplicationLexiconSpeaksInsideDomainsAndEmails(t *testing.T) {
	for _, tc := range []struct {
		name     string
		lexicon  []piper.LexiconEntry
		in, want string
	}{
		{"the word, in an e-mail", []piper.LexiconEntry{{Written: "Corujão", Said: "corujão", AnyCase: true}},
			"Escreva para ajuda@corujao.ai.", "Escreva para ajuda arroba corujão ponto á i."},
		{"the word, in a domain", []piper.LexiconEntry{{Written: "Corujão", Said: "corujão", AnyCase: true}},
			"Veja corujao.ai e o site corujão.ai.", "Veja corujão ponto á i e o sáite corujão ponto á i."},
		{"the whole domain", []piper.LexiconEntry{{Written: "Corujão.ai", Said: "corujão ponto ai", AnyCase: true}},
			"Escreva para ajuda@corujao.ai.", "Escreva para ajuda arroba corujão ponto ai."},
		{"the name in the local part", []piper.LexiconEntry{{Written: "Corujão", Said: "corujão", AnyCase: true}},
			"Escreva para corujao@exemplo.com.", "Escreva para corujão arroba exemplo ponto com."},
		{"it wins over the shared dictionary", []piper.LexiconEntry{{Written: "ALPR.sh", Said: "alper ponto ésse agá"}},
			"Escreva para ajuda@alpr.sh.", "Escreva para ajuda arroba alper ponto ésse agá."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := piper.NewNormalizer(piper.Speech{Lexicon: tc.lexicon})
			if got := n.Speakable(tc.in, time.Time{}); got != tc.want {
				t.Errorf("Speakable(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
	// Without it the word is said as it is written.
	if got := piper.Speakable("Escreva para ajuda@corujao.ai."); got != "Escreva para ajuda arroba corujao ponto á i." {
		t.Errorf("without the lexicon: %q", got)
	}
}

// 4. "ontem, 26/11" was said "ontem, ontem".
func TestARelativeDateIsNotSaidTwice(t *testing.T) {
	n := piper.NewNormalizer(piper.Speech{})
	for _, tc := range []struct{ name, in, want string }{
		{"after a comma", "Ontem, 26/11, treinei.", "Ontem, treinei."},
		{"after a space", "Ontem 26/11.", "Ontem."},
		{"today", "Hoje, 27/11, vence a fatura.", "Hoje, vence a fatura."},
		{"tomorrow at the end", "Vence amanhã, 28/11.", "Vence amanhã."},
		{"the day after tomorrow", "Vence depois de amanhã 29/11 cedo.", "Vence depois de amanhã cedo."},
		{"in parentheses", "Ontem (26/11) treinei.", "Ontem, treinei."},
		{"a different day keeps its own words", "Hoje, 28/11, treinei.", "Hoje, amanhã, treinei."},
		{"another word in between", "Ontem de manhã, 26/11, treinei.", "Ontem de manhã, ontem, treinei."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := n.Speakable(tc.in, defectsNow); got != tc.want {
				t.Errorf("Speakable(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
	// Without a moment nothing is relative: the date is said in full, after the word.
	if got := n.Speakable("Ontem, 26/11, treinei.", time.Time{}); got != "Ontem, vinte e seis de novembro, treinei." {
		t.Errorf("without now: %q", got)
	}
}

// 5. Words of every product that the shared dictionary lacked.
func TestTheSharedDictionaryHasTheWordsOfTheApplications(t *testing.T) {
	for in, want := range map[string]string{
		"Abra o app.":          "Abra o áp.",
		"Entre com o login.":   "Entre com o lóguin.",
		"Vá ao checkout.":      "Vá ao tchék aut.",
		"Confirme o e-mail.":   "Confirme o imêil.",
		"Clique no link.":      "Clique no linque.",
		"Os apps e os links.":  "Os apps e os links.",
		"O Link e o E-mail.":   "O linque e o imêil.",
		"Veja o link do site.": "Veja o linque do sáite.",
	} {
		if got := piper.Speakable(in); got != want {
			t.Errorf("Speakable(%q) = %q, want %q", in, got, want)
		}
	}
}
