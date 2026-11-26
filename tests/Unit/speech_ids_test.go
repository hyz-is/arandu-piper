package unit_test

import (
	"testing"

	"github.com/hyz-is/arandu-piper"
)

// An identifier is dictated digit by digit and a quantity is said as a number.
func TestSpeakableIdentifiers(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"mercosul plate", "A placa ABC1D23 está aqui.", "A placa á bê cê um dê dois três está aqui."},
		{"old plate", "A antiga ABC-1234.", "A antiga á bê cê um dois três quatro."},
		{"ipv4", "O IP 192.168.0.1 responde.", "O i pê um nove dois, ponto, um seis oito, ponto, zero, ponto, um responde."},
		{"parts above 255 are a grouped number, not an address", "Somou 300.400.500.600 no total.", "Somou trezentos bilhões quatrocentos milhões quinhentos mil e seiscentos no total."},
		{"version with a V", "Versão V2.1 e v1.4.2.", "Versão dois ponto um e versão um ponto quatro ponto dois."},
		{"version with the word", "Use a versão 3.0.", "Use a versão três ponto zero."},
		{"day count", "A liquidação é em D+1 e a taxa em D-2.", "A liquidação é em dê mais um e a taxa em dê menos dois."},
		{"round the clock", "Atendimento 24/7.", "Atendimento vinte e quatro horas por dia, sete dias por semana."},
		{"24/7 as a date", "Reunião em 24/7.", "Reunião em vinte e quatro de julho."},
		{"hour range", "Funcionamos das 08h às 17h.", "Funcionamos das oito às dezessete horas."},
		{"hour range with de", "Aberto de 08h às 18h.", "Aberto das oito às dezoito horas."},
		{"hour range with minutes", "Das 8h às 10h30.", "Das oito às dez e meia."},
		{"hour range with a clock", "Das 08:00 às 17:30.", "Das oito às dezessete e meia."},
		{"hour range with entre", "Entre 08h e 10h do dia 05/12.", "Entre oito horas e dez horas do dia cinco de dezembro."},
		{"international telephone", "Ligue +55 11 99999-8888.", "Ligue mais cinco cinco, um um, nove nove nove nove nove, oito oito oito oito."},
		{"toll free", "Ligue 0800 123 4567.", "Ligue zero oito zero zero, um dois três, quatro cinco seis sete."},
		{"protocol", "Protocolo 20261126-001.", "Protocolo dois zero dois seis um um dois seis, zero zero um."},
		{"code of letters and digits", "Seu código AB12CD chegou.", "Seu código á bê um dois cê dê chegou."},
		{"a short order number is a number", "O pedido 15 chegou.", "O pedido quinze chegou."},
		{"a word after pedido is a word", "O pedido pendente.", "O pedido pendente."},
		{"a quantity is not an identifier", "Gastou 1.250 passos.", "Gastou mil duzentos e cinquenta passos."},
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

// Units, currencies and symbols added by the phonetic layer.
func TestSpeakableUnitsAndSymbols(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"bytes", "Tem 10 GB e 32 MB.", "Tem dez guigabáites e trinta e dois megabáites."},
		{"frame rate and clock", "Roda a 60 FPS e 2,4 GHz.", "Roda a sessenta quadros por segundo e dois vírgula quatro gigahertz."},
		{"electric", "Liga em 220 V e gasta 15 W.", "Liga em duzentos e vinte volts e gasta quinze watts."},
		{"a unit of data said as it is pronounced", "Escreveu 5 terabytes e 3 kb.", "Escreveu cinco terabáites e três quilobáites."},
		{"singular", "Pesa 1 GB.", "Pesa um guigabáite."},
		{"american dollar", "Custa US$ 50.", "Custa cinquenta dólares americanos."},
		{"paragraph", "Veja o § 2º.", "Veja o parágrafo segundo."},
		{"paragraphs", "Veja os §§ 3 e 4.", "Veja os parágrafos três e quatro."},
		{"number sign", "Veja o #1.", "Veja o número um."},
		{"hashtag", "Use a #meta hoje.", "Use a hashtag meta hoje."},
		{"limited company", "A Ltda. e a S.A. pagam.", "A limitada e a sociedade anônima pagam."},
		{"example", "Ex.: 2 séries.", "exemplo: duas séries."},
		{"avenue", "Av. Brasil.", "avenida Brasil."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := piper.Speakable(tc.in); got != tc.want {
				t.Errorf("Speakable(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}
