package unit_test

import (
	"strings"
	"testing"
	"time"

	"github.com/hyz-is/arandu-piper"
)

func TestNumberInWords(t *testing.T) {
	for n, want := range map[int64]string{
		0: "zero", 1: "um", 15: "quinze", 16: "dezesseis", 21: "vinte e um", 100: "cem", 101: "cento e um", 999: "novecentos e noventa e nove",
		1000: "mil", 1001: "mil e um", 1200: "mil e duzentos", 1234: "mil duzentos e trinta e quatro",
		2050: "dois mil e cinquenta", 2026: "dois mil e vinte e seis", 98000: "noventa e oito mil", 100000: "cem mil",
		1_000_000: "um milhão", 1_001_000: "um milhão e mil", 2_500_000: "dois milhões e quinhentos mil",
		1_000_000_000: "um bilhão", -3: "menos três",
	} {
		if got := piper.NumberInWords(n); got != want {
			t.Errorf("NumberInWords(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestNumberInWordsFeminine(t *testing.T) {
	for n, want := range map[int64]string{
		1: "uma", 2: "duas", 3: "três", 21: "vinte e uma", 22: "vinte e duas", 200: "duzentas", 2000: "duas mil",
		1_000_000: "um milhão", 2_000_000: "dois milhões", 2_000_002: "dois milhões e duas",
	} {
		if got := piper.NumberInWordsFeminine(n); got != want {
			t.Errorf("NumberInWordsFeminine(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestOrdinalInWords(t *testing.T) {
	for _, tc := range []struct {
		n    int
		fem  bool
		want string
	}{
		{1, false, "primeiro"}, {1, true, "primeira"}, {2, true, "segunda"}, {3, false, "terceiro"}, {9, false, "nono"},
		{10, false, "décimo"}, {11, true, "décima primeira"}, {21, false, "vigésimo primeiro"}, {100, false, "centésimo"},
		{250, true, "ducentésima quinquagésima"}, {0, false, "zero"}, {1000, false, "mil"},
	} {
		if got := piper.OrdinalInWords(tc.n, tc.fem); got != tc.want {
			t.Errorf("OrdinalInWords(%d, %v) = %q, want %q", tc.n, tc.fem, got, tc.want)
		}
	}
}

func TestMoneyInWords(t *testing.T) {
	for cents, want := range map[int64]string{
		8700: "oitenta e sete reais", 100: "um real", 375: "três reais e setenta e cinco centavos", 50: "cinquenta centavos",
		120000: "mil e duzentos reais", 0: "zero real", 1: "um centavo", 100_000_000: "um milhão de reais", -250: "menos dois reais e cinquenta centavos",
	} {
		if got := piper.MoneyInWords(cents); got != want {
			t.Errorf("MoneyInWords(%d) = %q, want %q", cents, got, want)
		}
	}
}

func TestSayDay(t *testing.T) {
	now := time.Date(2026, 11, 27, 22, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		date time.Time
		want string
	}{
		{now, "hoje"},
		{time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC), "ontem"},
		{time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC), "anteontem"},
		{time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC), "amanhã"},
		{time.Date(2026, 11, 29, 0, 0, 0, 0, time.UTC), "depois de amanhã"},
		{time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC), "dia três"},
		{time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), "dia primeiro"},
		{time.Date(2026, 12, 20, 0, 0, 0, 0, time.UTC), "dia vinte de dezembro"},
		{time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), "dia primeiro de janeiro de dois mil e vinte e sete"},
	} {
		if got := piper.SayDay(tc.date, now); got != tc.want {
			t.Errorf("SayDay(%s) = %q, want %q", tc.date.Format("2006-01-02"), got, tc.want)
		}
	}
}

// Real phrases of the products that speak, with the text a voice should get.
func TestSpeakable(t *testing.T) {
	cases := []struct{ name, in, want string }{
		// The three phrases of the request.
		{"workout line", "Próximo treino: supino 3x10 com 42,5kg às 06:30.", "Próximo treino: supino três séries de dez com quarenta e dois vírgula cinco quilos às seis e meia."},
		{"calories", "Você gastou cerca de 292 kcal em 50 min.", "Você gastou cerca de duzentas e noventa e duas calorias em cinquenta minutos."},
		{"plan price", "Plano anual R$ 228,00 (R$ 19,00/mês).", "Plano anual duzentos e vinte e oito reais, dezenove reais por mês."},

		// Cardinals, gender, decimals, thousands, negatives.
		{"feminine two", "Faça 2 séries.", "Faça duas séries."},
		{"feminine one", "Faça 1 série.", "Faça uma série."},
		{"masculine one", "Compre 1 kg de frango.", "Compre um quilo de frango."},
		{"plural unit", "Compre 2 kg de arroz.", "Compre dois quilos de arroz."},
		{"hundreds feminine", "São 300 repetições.", "São trezentas repetições."},
		{"decimal", "Sua média é 3,2.", "Sua média é três vírgula dois."},
		{"decimal leading zeros", "A taxa é 0,05.", "A taxa é zero vírgula zero cinco."},
		{"decimal with a point", "Pesa 42.5 kg.", "Pesa quarenta e dois vírgula cinco quilos."},
		{"thousands", "Gastou 1.200 passos a mais.", "Gastou mil e duzentos passos a mais."},
		{"year", "Em 2026 você treinou mais.", "Em dois mil e vinte e seis você treinou mais."},
		{"negative", "Variação de -2,5 kg.", "Variação de menos dois vírgula cinco quilos."},
		{"positive sign", "Meta: +3 repetições.", "Meta: mais três repetições."},
		{"a round million takes de", "Foram 2.000.000 de passos.", "Foram dois milhões de passos."},
		{"a long run is an identifier", "Protocolo 123456789012.", "Protocolo um dois três quatro cinco seis sete oito nove zero um dois."},
		{"a number inside a name is left", "Use o H2O e o mp3.", "Use o H2O e o mp3."},

		// Ordinals.
		{"ordinal masculine", "Você ficou em 1º lugar.", "Você ficou em primeiro lugar."},
		{"ordinal feminine", "Faça a 2ª série.", "Faça a segunda série."},
		{"ordinal 21", "O 21º dia.", "O vigésimo primeiro dia."},
		{"numero", "Sala nº 5, n.º 12.", "Sala número cinco, número doze."},

		// Money.
		{"money cents", "Custa R$ 24,90.", "Custa vinte e quatro reais e noventa centavos."},
		{"money one real", "Custa R$ 1,00.", "Custa um real."},
		{"money no cents", "Custa R$ 87.", "Custa oitenta e sete reais."},
		{"money one cent digit", "Custa R$ 3,5.", "Custa três reais e cinquenta centavos."},
		{"money only cents", "Troco de R$ 0,50.", "Troco de cinquenta centavos."},
		{"money thousands", "Salário de R$ 1.234,56.", "Salário de mil duzentos e trinta e quatro reais e cinquenta e seis centavos."},
		{"money no space", "Pagou R$24,90 hoje.", "Pagou vinte e quatro reais e noventa centavos hoje."},
		{"money mil", "Vale R$ 2 mil.", "Vale dois mil reais."},
		{"money milhões", "Vale R$ 3 milhões.", "Vale três milhões de reais."},
		{"money negative", "Saldo de -R$ 50,00.", "Saldo de menos cinquenta reais."},
		{"dollar and euro", "US$ 10 e € 1,50.", "dez dólares e um euro e cinquenta centavos."},
		{"per month", "Pague R$ 19,00/mês.", "Pague dezenove reais por mês."},

		// Times and dates.
		{"half past", "Às 06:30.", "Às seis e meia."},
		{"quarter to", "Às 18:45.", "Às dezoito e quarenta e cinco."},
		{"on the hour", "Às 07:00.", "Às sete horas."},
		{"one o'clock", "À 01:00.", "À uma hora."},
		{"two thirty", "Às 02:30.", "Às duas e meia."},
		{"noon and midnight", "Ao 12:00 e às 00:00.", "Ao meio-dia e às meia-noite."},
		{"7h", "Acorde às 07h.", "Acorde às sete horas."},
		{"7h30 as a clock", "Acorde às 7h30.", "Acorde às sete e meia."},
		{"1h30 as a duration", "Treino de 1h30.", "Treino de uma hora e meia."},
		{"2h15 as a duration", "Dura 2h15.", "Dura duas horas e quinze minutos."},
		{"pace", "Ritmo de 5:30/km.", "Ritmo de cinco minutos e trinta segundos por quilômetro."},
		{"long duration", "Tempo 1:02:30.", "Tempo uma hora, dois minutos e trinta segundos."},
		{"full date", "Em 26/11/2026.", "Em vinte e seis de novembro de dois mil e vinte e seis."},
		{"first of the month", "Vence em 01/12/2026.", "Vence em primeiro de dezembro de dois mil e vinte e seis."},
		{"date without a year", "Vence em 20/12.", "Vence em vinte de dezembro."},
		{"iso date", "Criado em 2026-11-26.", "Criado em vinte e seis de novembro de dois mil e vinte e seis."},
		{"not a date", "Safra 26/27.", "Safra vinte e seis, vinte e sete."},
		{"fractions", "Use 1/2 xícara e 3/4 do tempo.", "Use meia xícara e três quartos do tempo."},

		// Percent, ranges, fitness notation.
		{"percent", "Subiu 45%.", "Subiu quarenta e cinco por cento."},
		{"percent decimal", "Gordura em 12,5 %.", "Gordura em doze vírgula cinco por cento."},
		{"range", "Faça 8-12 repetições.", "Faça de oito a doze repetições."},
		{"range en dash", "Faça 8–12 repetições.", "Faça de oito a doze repetições."},
		{"range feminine", "Faça 2-3 séries.", "Faça de duas a três séries."},
		{"range with a unit", "Descanse 60-90 s.", "Descanse de sessenta a noventa segundos."},
		{"range of percent", "Entre 70-80% da carga.", "Entre setenta e oitenta por cento da carga."},
		{"range after de", "Varia de 8-12 reps.", "Varia de oito a doze repetições."},
		{"series and sets of a range", "Faça 4x12-15.", "Faça quatro séries de doze a quinze."},
		{"series with spaces", "Faça 3 x 10 de agachamento.", "Faça três séries de dez de agachamento."},
		{"series of one", "Faça 1x15.", "Faça uma série de quinze."},
		{"times", "Treine 4x por semana.", "Treine quatro vezes por semana."},
		{"times of two", "Treine 2x.", "Treine duas vezes."},
		{"not a series", "O arquivo 0x10 e a caixa x10.", "O arquivo 0x10 e a caixa x10."},

		// Units.
		{"km/h", "Corra a 10 km/h.", "Corra a dez quilômetros por hora."},
		{"one km/h", "Corra a 1 km/h.", "Corra a um quilômetro por hora."},
		{"bpm", "FC em 150 bpm.", "FC em cento e cinquenta batimentos por minuto."},
		{"grams", "Coma 200g de frango.", "Coma duzentos gramas de frango."},
		{"mg and ml", "Tome 500 mg com 200 ml.", "Tome quinhentos miligramas com duzentos mililitros."},
		{"litres", "Beba 1,5 L e 2 l.", "Beba um vírgula cinco litros e dois litros."},
		{"metres", "Corra 400 m ou 5 km.", "Corra quatrocentos metros ou cinco quilômetros."},
		{"cm and mm", "Mede 180 cm e 5 mm.", "Mede cento e oitenta centímetros e cinco milímetros."},
		{"hours and seconds", "Dura 2 h e 30 s.", "Dura duas horas e trinta segundos."},
		{"degrees", "Faz 25°C e 45° de inclinação.", "Faz vinte e cinco graus Celsius e quarenta e cinco graus de inclinação."},
		{"imc units", "IMC de 24,3 kg/m².", "i ême cê de vinte e quatro vírgula três quilos por metro quadrado."},
		{"calories per day", "Meta de 2.000 kcal/dia.", "Meta de duas mil calorias por dia."},
		{"a unit that is the start of a word", "Comprei 3 sacas e 2 ms.", "Comprei três sacas e dois ms."},
		{"upper case unit", "Pesa 80 KG.", "Pesa oitenta quilos."},

		// Acronyms, abbreviations, symbols.
		{"acronyms", "Informe o CPF e o CNPJ no IRPF.", "Informe o cê pê efe e o cê ene pê jota no i érre pê efe."},
		{"fitness acronyms", "Treine com RPE 8 e RIR 2.", "Treine com érre pê é oito e érre i érre dois."},
		{"pix and inss", "Pague pelo Pix ou INSS e FGTS.", "Pague pelo pics ou i ene ésse ésse e éfe gê tê ésse."},
		{"abbreviations", "O Sr. Silva e a Dra. Lima aprox. 3 horas p.ex. hoje.", "O senhor Silva e a doutora Lima aproximadamente três horas por exemplo hoje."},
		{"etc at the end", "Frango, arroz, etc.", "Frango, arroz, etcétera."},
		{"etc in the middle", "Frango, arroz, etc. e feijão.", "Frango, arroz, etcétera e feijão."},
		{"chat shorthand", "vc pode treinar hj pq tb quer.", "você pode treinar hoje porque também quer."},
		{"e/ou", "Frango e/ou peixe.", "Frango e ou peixe."},
		{"ampersand and plus", "Peito & costas + ombros.", "Peito e costas mais ombros."},
		{"equals and approx", "Total = ~300 kcal.", "Total igual a cerca de trezentas calorias."},
		{"slash between words", "Treino A/B.", "Treino A, B."},
		{"hash number", "Meta #1 do mês.", "Meta número um do mês."},
		{"url", "Veja https://exemplo.com/treino?id=1 agora.", "Veja o link agora."},
		{"url after link", "O link https://exemplo.com é seu.", "O link mostrado na conversa é seu."},
		{"markdown link", "Veja [o plano](https://exemplo.com/p) hoje.", "Veja o plano hoje."},
		{"e-mail", "Escreva para ajuda@hyz.is.", "Escreva para ajuda arroba hyz ponto is."},
		{"cpf digits", "CPF 123.456.789-09.", "cê pê efe um dois três, quatro cinco seis, sete oito nove, zero nove."},
		{"postcode", "CEP 01310-100.", "cép zero um três um zero, um zero zero."},
		{"telephone", "Ligue (11) 99999-8888.", "Ligue um um, nove nove nove nove nove, oito oito oito oito."},

		// Prosody.
		{"bullets become sentences", "Compre:\n- adubo\n- calcário\n• cal", "Compre: adubo. calcário. cal."},
		{"numbered items", "1. Aquecer\n2) Treinar\n3. Alongar", "Aquecer. Treinar. Alongar."},
		{"parentheses become commas", "O treino (leve) acaba cedo.", "O treino, leve, acaba cedo."},
		{"repeated punctuation", "Muito bem!!! Certo??? Sério?!", "Muito bem! Certo? Sério?"},
		{"ellipsis", "Bom... vamos lá.", "Bom. vamos lá."},
		{"dashes become commas", "Hoje — treino leve – e descanso.", "Hoje, treino leve, e descanso."},
		{"emoji and formatting", "*Parabéns* 🎉 pelo _treino_ de hoje! 💪", "Parabéns pelo treino de hoje!"},
		{"capitals are said as words", "Responda SIM ou NÃO.", "Responda sim ou não."},
		{"windows line ends", "um\r\ndois\r\n", "um. dois."},
		{"empty", "", ""},
		{"only formatting", "🎉 *** ---", ""},
		{"control characters", "um\x00dois", "um dois."},
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

func TestSpeakableNeverLeavesADigitOrASymbol(t *testing.T) {
	for _, in := range []string{
		"Próximo treino: supino 3x10 com 42,5kg às 06:30.",
		"Plano anual R$ 228,00 (R$ 19,00/mês). Total: 1.234,56 = 12%.",
		"Meta: 8-12 reps, 60-90 s, RPE 8, 3x por semana, 26/11/2026, 1º dia.",
		"Retrato: 1. Renda líquida: R$ 3.500,00\n5. Renda comprometida com dívidas: 31%\n6. Reserva dura 1,5 mês",
	} {
		got := piper.Speakable(in)
		if strings.ContainsAny(got, "0123456789$%/()*_=") {
			t.Errorf("Speakable(%q) = %q still holds a digit or a symbol", in, got)
		}
	}
}

func TestSpeakableDatesRelativeToNow(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	n := piper.NewNormalizer(piper.Speech{})
	for _, tc := range []struct{ in, want string }{
		{"Vence 27/11.", "Vence hoje."},
		{"Venceu 26/11/2026.", "Venceu ontem."},
		{"Vence 28/11.", "Vence amanhã."},
		{"Vence 29/11.", "Vence depois de amanhã."},
		{"Vence 03/11.", "Vence dia três."},
		{"Vence 20/12.", "Vence dia vinte de dezembro."},
		{"Vence 01/12.", "Vence dia primeiro de dezembro."},
		{"Vence 15/01/2027.", "Vence dia quinze de janeiro de dois mil e vinte e sete."},
		{"Vence 31/02.", "Vence trinta e um, dois."},
	} {
		if got := n.Speakable(tc.in, now); got != tc.want {
			t.Errorf("Speakable(%q, now) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// Without a moment the same date is said in full, and the zero time is never "today".
	if got := n.Speakable("Vence 27/11.", time.Time{}); got != "Vence vinte e sete de novembro." {
		t.Errorf("without now: %q", got)
	}
	if got := n.Speakable("Em 29/02/2028.", time.Time{}); got != "Em vinte e nove de fevereiro de dois mil e vinte e oito." {
		t.Errorf("leap day: %q", got)
	}
}

func TestSpeakableIsDeterministic(t *testing.T) {
	in := "Próximo treino: supino 3x10 com 42,5kg às 06:30. R$ 24,90 (R$ 1,00/dia) 26/11."
	want := piper.Speakable(in)
	for i := 0; i < 50; i++ {
		if got := piper.Speakable(in); got != want {
			t.Fatalf("run %d: %q, want %q", i, got, want)
		}
	}
}

func TestADomainLexiconWinsOverTheBaseOne(t *testing.T) {
	n := piper.NewNormalizer(piper.Speech{Lexicon: []piper.LexiconEntry{
		{Written: "supino", Said: "supíno", AnyCase: true},
		{Written: "kcal", Said: "quilocalorias", AnyCase: false},
		{Written: "IMC", Said: "índice de massa corporal"},
		{Written: "HIIT", Said: "ráit"},
	}})
	got := n.Speakable("Supino e HIIT queimam 300 kcal. Seu IMC é bom.", time.Time{})
	want := "supíno e ráit queimam trezentas quilocalorias. Seu índice de massa corporal é bom."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if !strings.HasPrefix(n.Version(), piper.NormalizerVersion+"+") {
		t.Errorf("version %q does not tell the lexicon", n.Version())
	}
	if other := piper.NewNormalizer(piper.Speech{Lexicon: []piper.LexiconEntry{{Written: "supino", Said: "x"}}}); other.Version() == n.Version() {
		t.Errorf("two lexicons, one version %q", n.Version())
	}
}

func TestTheLexiconMatchesWholeWordsOnly(t *testing.T) {
	n := piper.NewNormalizer(piper.Speech{Lexicon: []piper.LexiconEntry{{Written: "ME", Said: "ême"}, {Written: "ação", Said: "assão", AnyCase: true}}})
	got := n.Speakable("ME, MEI, HOME, ME. Ação, reação, AÇÃO e ação.", time.Time{})
	want := "ême, mêi, home, ême. assão, reação, assão e assão."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTheVersionIsStable(t *testing.T) {
	if v := piper.NewNormalizer(piper.Speech{}).Version(); v != piper.NormalizerVersion {
		t.Errorf("default version %q, want %q", v, piper.NormalizerVersion)
	}
	if v := piper.NewNormalizer(piper.Speech{Raw: true}).Version(); v != "raw" {
		t.Errorf("raw version %q", v)
	}
	a := piper.NewNormalizer(piper.Speech{MaxWords: 10}).Version()
	b := piper.NewNormalizer(piper.Speech{MaxWords: 10}).Version()
	if a != b || a == piper.NormalizerVersion {
		t.Errorf("versions %q and %q", a, b)
	}
}

func TestRawLeavesTheTextAsItIs(t *testing.T) {
	n := piper.NewNormalizer(piper.Speech{Raw: true})
	if got := n.Speakable("R$ 24,90\n\n3x10  kg", time.Now()); got != "R$ 24,90 3x10 kg" {
		t.Errorf("got %q", got)
	}
}

func TestPlainIsNotTheSpeakableText(t *testing.T) {
	// Plain keeps what it always did: it does not say numbers.
	if got := piper.Plain("*Pronto* 3x10 com R$ 24,90"); got != "Pronto 3x10 com R$ 24,90" {
		t.Errorf("Plain = %q", got)
	}
}

func TestSpeakableKeepCapitalsAndSkipLines(t *testing.T) {
	n := piper.NewNormalizer(piper.Speech{KeepCapitals: true, SkipLinePrefixes: []string{"Fonte:"}})
	got := n.Speakable("Responda SIM.\nFONTE: o banco\nfonte: outro\nFim.", time.Time{})
	if got != "Responda SIM. Fim." {
		t.Errorf("got %q", got)
	}
}

func TestSpeakableCutsALongTextAtASentence(t *testing.T) {
	n := piper.NewNormalizer(piper.Speech{MaxWords: 75, RestNote: "O resto eu deixei por escrito, na conversa."})
	got := n.Speakable(strings.Repeat("Esta é uma frase sobre o seu dinheiro do mês. ", 40), time.Time{})
	if words := len(strings.Fields(got)); words > 75+10 {
		t.Errorf("a text of %d words", words)
	}
	if !strings.HasSuffix(got, "O resto eu deixei por escrito, na conversa.") {
		t.Errorf("the cut does not say the rest is written: %q", got)
	}
	short := n.Speakable("Frase curta.", time.Time{})
	if short != "Frase curta." {
		t.Errorf("short text changed: %q", short)
	}
}

func TestALongSentenceGetsAPlaceToBreathe(t *testing.T) {
	in := "Você treinou bem durante toda a semana e manteve a alimentação dentro da meta mas ainda falta dormir melhor para recuperar os músculos."
	got := piper.Speakable(in)
	want := "Você treinou bem durante toda a semana e manteve a alimentação dentro da meta, mas ainda falta dormir melhor para recuperar os músculos."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func FuzzSpeakable(f *testing.F) {
	for _, s := range []string{
		"Próximo treino: supino 3x10 com 42,5kg às 06:30.", "R$ 99999999999999999999,99", "1.2.3.4 -- 0x 00:99 99:99:99 31/02/2026",
		"\xff\xfe invalid utf-8 \x00", "((((", "))))", "R$", "x", "3x", "ª", "1º", "-", "+", "/", "%", "°C", "http://", "@", "a@b.c",
		"999999999999999999999999 kg", "0,0000000001", "12h99", "1-0", "9-9", "05:30/km", "1/2/3", "#", "n.º", "e/ou", "*_~`", "…",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		got := piper.Speakable(in)
		if strings.ContainsAny(got, "\n\r\x00") {
			t.Fatalf("Speakable(%q) = %q holds a line break or a control character", in, got)
		}
		if strings.ContainsRune(got, '�') && !strings.ContainsRune(in, '�') && !strings.Contains(in, "\xff") {
			t.Fatalf("Speakable(%q) = %q made a replacement character", in, got)
		}
	})
}
