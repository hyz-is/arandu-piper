package piper

import (
	"strconv"
	"strings"
)

// The numbers of the Brazilian Portuguese verbalizer: cardinals with the
// gender of what they count, ordinals, decimals and money. They follow the
// short scale of the Brazilian norm (um bilhão is 10^9) and the spelling of the
// Orthographic Agreement of 1990 (dezesseis, dezessete, catorze).

var (
	smallWords    = [...]string{"zero", "um", "dois", "três", "quatro", "cinco", "seis", "sete", "oito", "nove", "dez", "onze", "doze", "treze", "catorze", "quinze", "dezesseis", "dezessete", "dezoito", "dezenove"}
	tensWords     = [...]string{"", "", "vinte", "trinta", "quarenta", "cinquenta", "sessenta", "setenta", "oitenta", "noventa"}
	hundredsWords = [...]string{"", "cento", "duzentos", "trezentos", "quatrocentos", "quinhentos", "seiscentos", "setecentos", "oitocentos", "novecentos"}
)

// NumberInWords is a whole number said in Brazilian Portuguese, in the
// masculine, up to the hundreds of trillions: 1200 is "mil e duzentos", 98000
// "noventa e oito mil", 2500000 "dois milhões e quinhentos mil". A negative
// number starts with "menos". A number of a quadrillion or more is said digit
// by digit.
func NumberInWords(n int64) string {
	switch {
	case n < 0:
		if n == -n { // the most negative int64
			return strconv.FormatInt(n, 10)
		}
		return "menos " + NumberInWords(-n)
	case n == 0:
		return "zero"
	case n >= 1_000_000_000_000_000:
		return digitsInWords(strconv.FormatInt(n, 10))
	}
	groups := []struct {
		value int64
		one   string
		many  string
	}{
		{n / 1_000_000_000_000 % 1000, "um trilhão", "trilhões"},
		{n / 1_000_000_000 % 1000, "um bilhão", "bilhões"},
		{n / 1_000_000 % 1000, "um milhão", "milhões"},
		{n / 1000 % 1000, "mil", "mil"},
		{n % 1000, "", ""},
	}
	var parts []string
	var last int64
	for _, g := range groups {
		if g.value == 0 {
			continue
		}
		last = g.value
		switch {
		case g.value == 1 && g.one != "":
			parts = append(parts, g.one)
		case g.many == "":
			parts = append(parts, belowThousand(g.value))
		default:
			parts = append(parts, belowThousand(g.value)+" "+g.many)
		}
	}
	// "mil e duzentos", "dois mil e cinquenta": the last group is joined with
	// "e" when it is below one hundred or a round hundred.
	if len(parts) > 1 && (last < 100 || last%100 == 0) {
		return strings.Join(parts[:len(parts)-1], " ") + " e " + parts[len(parts)-1]
	}
	return strings.Join(parts, " ")
}

func belowThousand(n int64) string {
	if n == 100 {
		return "cem"
	}
	var parts []string
	if h := n / 100; h > 0 {
		parts = append(parts, hundredsWords[h])
	}
	switch r := n % 100; {
	case r == 0:
	case r < 20:
		parts = append(parts, smallWords[r])
	default:
		t := tensWords[r/10]
		if r%10 != 0 {
			t += " e " + smallWords[r%10]
		}
		parts = append(parts, t)
	}
	return strings.Join(parts, " e ")
}

// NumberInWordsFeminine is NumberInWords for a feminine noun: "uma", "duas",
// "duzentas". Only the words that agree change ("milhão" is masculine, so
// what stands before it stays as it is): 2000 is "duas mil", 2000000 "dois
// milhões".
func NumberInWordsFeminine(n int64) string {
	words := strings.Fields(NumberInWords(n))
	from := 0
	for i, w := range words {
		switch w {
		case "milhão", "milhões", "bilhão", "bilhões", "trilhão", "trilhões":
			from = i + 1
		}
	}
	for i := from; i < len(words); i++ {
		switch w := words[i]; w {
		case "um":
			words[i] = "uma"
		case "dois":
			words[i] = "duas"
		case "duzentos", "trezentos", "quatrocentos", "quinhentos", "seiscentos", "setecentos", "oitocentos", "novecentos":
			words[i] = strings.TrimSuffix(w, "os") + "as"
		}
	}
	return strings.Join(words, " ")
}

// digitsInWords says each digit of a run: "123" is "um dois três".
func digitsInWords(digits string) string {
	parts := make([]string, 0, len(digits))
	for _, d := range digits {
		if d >= '0' && d <= '9' {
			parts = append(parts, smallWords[d-'0'])
		}
	}
	return strings.Join(parts, " ")
}

var (
	ordinalUnits    = [...]string{"", "primeiro", "segundo", "terceiro", "quarto", "quinto", "sexto", "sétimo", "oitavo", "nono"}
	ordinalTens     = [...]string{"", "décimo", "vigésimo", "trigésimo", "quadragésimo", "quinquagésimo", "sexagésimo", "septuagésimo", "octogésimo", "nonagésimo"}
	ordinalHundreds = [...]string{"", "centésimo", "ducentésimo", "trecentésimo", "quadringentésimo", "quingentésimo", "sexcentésimo", "septingentésimo", "octingentésimo", "nongentésimo"}
)

// OrdinalInWords is an ordinal number from 1 to 999 said in full: 1 is
// "primeiro" ("primeira" when feminine), 21 "vigésimo primeiro", 100
// "centésimo". Past 999, and for zero or a negative number, the cardinal is
// said instead, since nobody says an ordinal that large.
func OrdinalInWords(n int, feminine bool) string {
	if n < 1 || n > 999 {
		return NumberInWords(int64(n))
	}
	var parts []string
	if h := n / 100; h > 0 {
		parts = append(parts, ordinalHundreds[h])
	}
	if t := n % 100 / 10; t > 0 {
		parts = append(parts, ordinalTens[t])
	}
	if u := n % 10; u > 0 {
		parts = append(parts, ordinalUnits[u])
	}
	out := strings.Join(parts, " ")
	if feminine {
		words := strings.Fields(out)
		for i, w := range words {
			words[i] = strings.TrimSuffix(w, "o") + "a"
		}
		out = strings.Join(words, " ")
	}
	return out
}

// MoneyInWords is an amount in cents of reais said in full: "mil e duzentos
// reais", "três reais e setenta e cinco centavos", "cinquenta centavos", "um
// real".
func MoneyInWords(cents int64) string {
	return currencyInWords(cents, currencies["R$"])
}

type currency struct {
	one, many         string // "real", "reais"
	centOne, centMany string // "centavo", "centavos"
	zero              string // what zero is said as
	millionDe         string // "de reais": what follows a round million
}

var currencies = map[string]currency{
	"R$":  {"real", "reais", "centavo", "centavos", "zero real", "de reais"},
	"US$": {"dólar", "dólares", "centavo", "centavos", "zero dólar", "de dólares"},
	"$":   {"dólar", "dólares", "centavo", "centavos", "zero dólar", "de dólares"},
	"€":   {"euro", "euros", "centavo", "centavos", "zero euro", "de euros"},
}

func currencyInWords(cents int64, cur currency) string {
	if cents < 0 {
		return "menos " + currencyInWords(-cents, cur)
	}
	units, rest := cents/100, cents%100
	var parts []string
	switch {
	case units == 1:
		parts = append(parts, "um "+cur.one)
	case units >= 1_000_000 && units%1_000_000 == 0:
		parts = append(parts, NumberInWords(units)+" "+cur.millionDe)
	case units > 1:
		parts = append(parts, NumberInWords(units)+" "+cur.many)
	}
	switch {
	case rest == 1:
		parts = append(parts, "um "+cur.centOne)
	case rest > 1:
		parts = append(parts, NumberInWords(rest)+" "+cur.centMany)
	}
	if len(parts) == 0 {
		return cur.zero
	}
	return strings.Join(parts, " e ")
}

// fractionWords says the decimals of a number: "2" is "dois", "25" is "vinte e
// cinco", "05" is "zero cinco", and four digits or more are said one by one.
func fractionWords(f string) string {
	if len(f) > 3 {
		return digitsInWords(f)
	}
	var zeros []string
	for len(f) > 1 && f[0] == '0' {
		zeros = append(zeros, "zero")
		f = f[1:]
	}
	n, _ := strconv.ParseInt(f, 10, 64)
	return strings.Join(append(zeros, NumberInWords(n)), " ")
}
