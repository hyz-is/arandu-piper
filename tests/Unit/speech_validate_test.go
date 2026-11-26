package unit_test

import (
	"strings"
	"testing"
	"time"

	"github.com/hyz-is/arandu-piper"
)

// Whatever the verbalizer makes of a text, the validator must find nothing wrong
// with it: no number of the display text may be lost or changed.
func TestWhatTheVerbalizerSaysPassesTheValidator(t *testing.T) {
	n := piper.NewNormalizer(piper.Speech{})
	for _, in := range []string{
		"Hoje: supino 3x12 com 22,5 kg, descanso de 60-90 s.",
		"Seu IMC 24,7 é normal. Meta: perder 2,5 kg até 20/12.",
		"Plano anual R$ 228,00 (R$ 19,00/mês), renova em 12/12/2026.",
		"Treino às 06:30, 45 min, RPE 8, RIR 2, FC máxima 180 bpm.",
		"Ligue (11) 99999-8888 ou 0800 123 4567, das 08h às 18h.",
		"A placa ABC1D23 no IP 192.168.0.1, versão V2.1, protocolo 20261126-001.",
		"Atendimento 24/7, liquidação em D+1, 1/2 xícara de aveia.",
		"Escreva para ajuda@ctrl.fit ou veja alpr.sh e exemplo.com/login.",
		"A câmera grava 60 FPS com 10 GB, em 220 V e 15 W, a 100 km/h e 30°C.",
		"CPF 123.456.789-09 e CEP 01310-100.",
		"Vence 26/11/2026 e custa US$ 50 ou R$ 1.250,50.",
	} {
		speech, issues := n.Check(in, time.Time{})
		for _, i := range issues {
			if i.Severity == piper.SeverityError {
				t.Errorf("Check(%q)\n speech %q\n %s", in, speech, i)
			}
		}
	}
}

// A value that changed, or was lost, is an error: a caller refuses the audio.
func TestTheValidatorFindsWhatChanged(t *testing.T) {
	cases := []struct {
		name            string
		display, speech string
		kind            piper.IssueKind
		text            string
	}{
		{"a number changed", "Supino com 22,5 kg.", "Supino com vinte e cinco quilos.", piper.IssueMissing, "22,5"},
		{"a unit changed", "Supino com 22,5 kg.", "Supino com vinte e dois vírgula cinco gramas.", piper.IssueUnit, "22,5 kg"},
		{"a currency changed", "Plano R$ 228,00.", "Plano duzentos e vinte e oito dólares.", piper.IssueUnit, "R$ 228,00"},
		{"a date changed", "Vence 26/11/2026.", "Vence vinte e sete de novembro de dois mil e vinte e seis.", piper.IssueMissing, "26/11/2026"},
		{"an hour changed", "Às 06:30.", "Às sete e meia.", piper.IssueMissing, "06:30"},
		{"a plate changed", "Placa ABC1D23.", "Placa á bê cê um dê dois quatro.", piper.IssueMissing, "23"},
		{"an e-mail lost", "Escreva para ajuda@ctrl.fit.", "Escreva para a ajuda.", piper.IssueAddress, "ajuda@ctrl.fit"},
		{"a unit of data changed", "A GPU tem 10 GB.", "A gê pê u tem dez megabytes.", piper.IssueUnit, "10 GB"},
		{"a digit left for the engine", "São 24 GB.", "São 24 gigabytes.", piper.IssueDigits, "24"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues := piper.Validate(tc.display, tc.speech)
			found := false
			for _, i := range issues {
				if i.Kind == tc.kind && i.Text == tc.text {
					found = true
				}
			}
			if !found {
				t.Errorf("Validate(%q, %q) = %v, want a %s issue on %q", tc.display, tc.speech, issues, tc.kind, tc.text)
			}
			if !piper.HasErrors(issues) && tc.kind != piper.IssueDigits {
				t.Errorf("Validate(%q, %q) has no error", tc.display, tc.speech)
			}
		})
	}
}

func TestTheValidatorAcceptsWhatIsSaidAnotherWay(t *testing.T) {
	for _, tc := range []struct{ name, display, speech string }{
		{"a thousand", "Total: 1.000", "Total: mil."},
		{"the half hour", "Às 06:30.", "Às seis e meia."},
		{"a date said relative to now", "Vence 27/11.", "Vence hoje."},
		{"a fraction", "Misture 1/2 xícara.", "Misture meia xícara."},
		{"round the clock", "Atendimento 24/7.", "Atendimento vinte e quatro horas por dia, sete dias por semana."},
		{"digit by digit", "Placa ABC1D23.", "Placa á bê cê um dê dois três."},
		{"money", "Plano R$ 228,00.", "Plano duzentos e vinte e oito reais."},
		{"a byte as it is pronounced", "Tem 10 GB.", "Tem dez guigabáites."},
		{"a byte as it is written", "Tem 10 GB.", "Tem dez gigabytes."},
	} {
		if issues := piper.Validate(tc.display, tc.speech); piper.HasErrors(issues) {
			t.Errorf("%s: Validate(%q, %q) = %v", tc.name, tc.display, tc.speech, issues)
		}
	}
}

// What can be read two ways is a warning, never a guess.
func TestTheValidatorWarnsOfWhatIsAmbiguous(t *testing.T) {
	n := piper.NewNormalizer(piper.Speech{})
	_, issues := n.Check("Atendimento 24/7 e 1/2 xícara.", time.Time{})
	var texts []string
	for _, i := range issues {
		if i.Kind != piper.IssueAmbiguous || i.Severity != piper.SeverityWarning {
			t.Errorf("unexpected %s", i)
		}
		texts = append(texts, i.Text)
	}
	if strings.Join(texts, ",") != "24/7,1/2" {
		t.Errorf("warned of %v, want 24/7 and 1/2", texts)
	}

	// An acronym the dictionary does not know is a guess from its letters.
	_, issues = n.Check("Veja o XQZ.", time.Time{})
	if len(issues) != 1 || issues[0].Kind != piper.IssueAmbiguous {
		t.Errorf("an unknown acronym: %v", issues)
	}
	// One the dictionary knows is not.
	if _, issues = n.Check("Veja a GPU.", time.Time{}); len(issues) != 0 {
		t.Errorf("a known acronym: %v", issues)
	}
	// One of the application's own is not either.
	own := piper.NewNormalizer(piper.Speech{Lexicon: []piper.LexiconEntry{{Written: "XQZ", Said: "xis quê zê"}}})
	if _, issues = own.Check("Veja o XQZ.", time.Time{}); len(issues) != 0 {
		t.Errorf("an acronym of the application: %v", issues)
	}
}

func TestAnIssueTellsWhatItIs(t *testing.T) {
	i := piper.Issue{Kind: piper.IssueMissing, Severity: piper.SeverityError, Text: "22,5", Detail: "the number is not said in the speech text"}
	if got := i.String(); got != "error missing: 22,5: the number is not said in the speech text" {
		t.Errorf("String() = %q", got)
	}
	if piper.HasErrors(nil) || piper.HasErrors([]piper.Issue{{Severity: piper.SeverityWarning}}) {
		t.Error("HasErrors is true without an error")
	}
}
