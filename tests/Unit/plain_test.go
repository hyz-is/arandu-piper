package unit_test

import (
	"testing"

	"github.com/hyz-is/arandu-piper"
)

func TestPlain(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain text is untouched", "Pronto, seu Antônio. Está certo?", "Pronto, seu Antônio. Está certo?"},
		{"bold", "*Pronto*, seu Antônio!", "Pronto, seu Antônio!"},
		{"italic", "Anotei _mil e duzentos reais_ de adubo.", "Anotei mil e duzentos reais de adubo."},
		{"strikethrough", "No ~talhão dois~ talhão três.", "No talhão dois talhão três."},
		{"markdown bold", "**Atenção:** a ferrugem apareceu.", "Atenção: a ferrugem apareceu."},
		{"bold and italic nested", "*_negrito e itálico_* aqui", "negrito e itálico aqui"},
		{"after an opening parenthesis and quote", `(*entre parênteses*), e "_aspas_".`, `(entre parênteses), e "aspas".`},
		{"formatting several times in a line", "*um* e *dois* e _três_", "um e dois e três"},
		{"a marker inside a word is not formatting", "o talhao_3 e o snake_case", "o talhao 3 e o snake case"},
		{"an unmatched marker is dropped", "*sem fechar a frase", "sem fechar a frase"},
		{"a multiplication is not lost", "2*3 sacas", "2 3 sacas"},
		{"inline code", "Rode `aru migrate` agora.", "Rode aru migrate agora."},
		{"code block", "```\nbloco\n```\nfim", "bloco. fim"},
		{"heading", "# Resumo\nTudo certo.", "Resumo. Tudo certo."},
		{"quote", "> disse ele\nentão está certo", "disse ele. então está certo"},
		{"rules are dropped", "um\n---\ndois\n***\ntrês", "um. dois. três"},
		{"bullets become sentences", "Compre:\n- adubo\n- calcário\n* veneno\n• cal", "Compre: adubo. calcário. veneno. cal"},
		{"numbered items", "1. Primeiro\n2) Segundo\n10. Décimo", "Primeiro. Segundo. Décimo"},
		{"a number at the start of a sentence stays", "2026 foi um bom ano", "2026 foi um bom ano"},
		{"a list item that ends in a mark keeps it", "- Está certo?\n- Sim!", "Está certo? Sim!"},
		{"paragraphs are separate sentences", "Primeiro parágrafo\n\nSegundo parágrafo.", "Primeiro parágrafo. Segundo parágrafo."},
		{"a colon is kept and not doubled", "Faça assim:\npasso um", "Faça assim: passo um"},
		{"a closing quote does not hide the mark", "ele disse \"sim\"\ne foi", `ele disse "sim". e foi`},
		{"link keeps its words", "Veja [o boletim](https://exemplo.com/b) hoje.", "Veja o boletim hoje."},
		{"address is removed", "Veja https://exemplo.com/a?b=1, ok. E www.exemplo.com.br/x.", "Veja, ok. E."},
		{"emoji", "Bom dia 🙂 seu Antônio 🌱☕", "Bom dia seu Antônio"},
		{"emoji with joiner, skin tone, flag and keycap", "👨‍🌾 👍🏽 🇧🇷 1️⃣ Aduba", "1 Aduba"},
		{"emoji before a mark leaves no gap", "Certo 👍!", "Certo!"},
		{"arrow becomes a comma", "adubo → produtividade", "adubo, produtividade"},
		{"only emoji", "🙂🙂", ""},
		{"only formatting", "*** ---", ""},
		{"money and accents stay", "R$ 1.200,00 de adubo no talhão três, ação rápida", "R$ 1.200,00 de adubo no talhão três, ação rápida"},
		{"windows line ends", "um\r\ndois\r\n", "um. dois"},
		{"whatsapp message", "*Resumo do mês* 📋\n\n- Adubo: R$ 1.200,00\n- Colheita: 40 sacas\n\n_Está certo?_", "Resumo do mês. Adubo: R$ 1.200,00. Colheita: 40 sacas. Está certo?"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := piper.Plain(tc.in); got != tc.want {
				t.Errorf("Plain(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPlainIsIdempotent(t *testing.T) {
	for _, in := range []string{
		"*Resumo* 📋\n- Adubo: R$ 1.200,00\n- Colheita\n\n_Está certo?_",
		"# Título\n> citação\n1. um\n2. dois",
		"plain text. Nothing to do?",
	} {
		once := piper.Plain(in)
		if twice := piper.Plain(once); twice != once {
			t.Errorf("Plain is not idempotent on %q: %q then %q", in, once, twice)
		}
	}
}
