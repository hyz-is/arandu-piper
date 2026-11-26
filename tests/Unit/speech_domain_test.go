package unit_test

import (
	"strings"
	"testing"
	"time"

	"github.com/hyz-is/arandu-piper"
)

// financeLexicon is the lexicon of a finance assistant, passed in by the
// application: the words the base lexicon does not hold, and the spellings the
// product prefers. It is what Corujão.ai passes.
var financeLexicon = []piper.LexiconEntry{
	{Written: "Corujão.ai", Said: "Corujão ponto ai", AnyCase: true},
	{Written: "HYZIS", Said: "Ráiz is", AnyCase: true},
	{Written: "WhatsApp", Said: "uótsap", AnyCase: true},
	{Written: "Procon", Said: "Prócon", AnyCase: true},
	{Written: "Wi-Fi", Said: "uái fai", AnyCase: true},
	{Written: "MEI", Said: "mêi"},
	{Written: "DAS", Said: "dás"},
	{Written: "DASN", Said: "dás ene"},
	{Written: "DRE", Said: "dê erre é"},
	{Written: "EPP", Said: "é pê pê"},
	{Written: "ME", Said: "ême"},
	{Written: "CPF", Said: "cê pê efe"},
	{Written: "CNPJ", Said: "cê ene pê jota"},
	{Written: "LGPD", Said: "éle gê pê dê"},
	{Written: "IPTU", Said: "i pê tê u"},
	{Written: "IPVA", Said: "i pê vê á"},
	{Written: "INSS", Said: "i ene ésse ésse"},
	{Written: "PDF", Said: "pê dê éfe"},
	{Written: "API", Said: "á pê i"},
	{Written: "BC", Said: "bê cê"},
	{Written: "FGTS", Said: "éfe gê tê ésse"},
}

func financeSpeech() piper.Speech {
	return piper.Speech{
		Lexicon:          financeLexicon,
		MaxWords:         75,
		RestNote:         "O resto eu deixei por escrito, na conversa.",
		SkipLinePrefixes: []string{"fonte:"},
	}
}

// The cases of Corujão.ai's spoken_internal_test.go, run through the module with
// its lexicon passed in: the speech must be the one the application already had.
func TestTheFinanceAssistantSpeaksAsItDid(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	n := piper.NewNormalizer(financeSpeech())

	got := n.Speakable("Registrei: R$ 87,50 — Mercado, hoje.\nPara apagar, responda *desfazer*.", now)
	for _, want := range []string{"oitenta e sete reais e cinquenta centavos", "Mercado", "hoje", "desfazer"} {
		if !strings.Contains(got, want) {
			t.Errorf("Speakable = %q, missing %q", got, want)
		}
	}
	for _, bad := range []string{"R$", "*", "—", "8"} {
		if strings.Contains(got, bad) {
			t.Errorf("Speakable = %q still holds %q", got, bad)
		}
	}

	got = n.Speakable("As parcelas levam 45% da renda. Procure o Procon. Seu MEI vence dia 20/12. Fale pelo WhatsApp.", now)
	for _, want := range []string{"quarenta e cinco por cento", "Prócon", "mêi", "dia vinte de dezembro", "uótsap"} {
		if !strings.Contains(got, want) {
			t.Errorf("Speakable = %q, missing %q", got, want)
		}
	}

	got = n.Speakable("Registrei 2 parcelas e 3 contas.", time.Now())
	if !strings.Contains(got, "duas parcelas") || !strings.Contains(got, "três contas") {
		t.Errorf("Speakable = %q", got)
	}

	long := strings.Repeat("Esta é uma frase sobre o seu dinheiro do mês. ", 40)
	got = n.Speakable(long, time.Now())
	if words := len(strings.Fields(got)); words > 75+10 {
		t.Errorf("a voice note of %d words", words)
	}
	if !strings.Contains(got, "por escrito") {
		t.Error("the cut does not say the rest is written")
	}

	got = n.Speakable("Retrato: 1. Renda líquida: R$ 3.500,00\n5. Renda comprometida com dívidas: 31%\n6. Reserva dura 1,5 mês", time.Now())
	if strings.ContainsAny(got, "0123456789") {
		t.Errorf("Speakable = %q holds a digit", got)
	}
}

func TestTheFinanceLexiconSpeaksTheWordsOfMoney(t *testing.T) {
	n := piper.NewNormalizer(financeSpeech())
	for in, want := range map[string]string{
		"Fale com o Corujão.ai.":     "Fale com o Corujão ponto ai.",
		"Entregue a DASN do MEI.":    "Entregue a dás ene do mêi.",
		"Pague o DAS, o IPTU e IPVA": "Pague o dás, o i pê tê u e i pê vê á.",
		"Envie o CPF e o CNPJ.":      "Envie o cê pê efe e o cê ene pê jota.",
		"Veja o Wi-Fi e o PDF.":      "Veja o uái fai e o pê dê éfe.",
		"A ME e a EPP.":              "A ême e a é pê pê.",
		"Fonte: o banco\nTudo bem.":  "Tudo bem.",
	} {
		if got := n.Speakable(in, time.Time{}); got != want {
			t.Errorf("Speakable(%q) = %q, want %q", in, got, want)
		}
	}
}
