package piper

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Identifiers against quantities. A quantity is said as a number a person can
// weigh ("mil e duzentos"); an identifier (a plate, an address of a network, a
// protocol, a version, a telephone) is said digit by digit, because the listener
// has to write it down and a quantity cannot be written down by ear.

var (
	// A Mercosul plate (ABC1D23) or an old one (ABC-1234, ABC1234).
	rePlate = regexp.MustCompile(`\b[A-Z]{3}-?\d[A-Z0-9]\d{2}\b`)
	// An IPv4 address.
	reIPv4 = regexp.MustCompile(`\b(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})\b`)
	// A version: "V2.1", "v1.4.2", "versão 3.0".
	reVersion = regexp.MustCompile(`\b(?:((?i:vers[ãa]o))\s|[vV])(\d+(?:\.\d+){1,3})\b`)
	// D+1, D-2: the day count of a settlement.
	reDayCount = regexp.MustCompile(`\bD([+-])(\d{1,3})\b`)
	// 24/7: a service that never stops, or the 24th of July.
	reAllDay = regexp.MustCompile(`\b24/7\b`)
	// An international telephone number: +55 11 99999-8888.
	reIntlPhone = regexp.MustCompile(`\+(\d{1,3})[\s-]?\(?(\d{2})\)?[\s-]?(9?\d{4})[\s-]?(\d{4})\b`)
	// A toll-free number: 0800 123 4567.
	reTollFree = regexp.MustCompile(`\b0[3589]00[\s-]?\d{3}[\s-]?\d{3,4}\b`)
	// A protocol, a code, an order: said after a word that says what it is.
	reCode = regexp.MustCompile(`(?i)\b(protocolo|código|codigo|ticket|chamado|pedido)(\s+(?:n[ºo°.]{1,2}\s?|número\s)?)([A-Za-z0-9][A-Za-z0-9-]{3,})\b`)
	// A range of hours: "08h às 17h", "08:00 às 17:30".
	reHourRange = regexp.MustCompile(`\b(\d{1,2})(?:h(\d{2})?|:(\d{2}))\s?(?:às|as|a|até|-|–)\s?(\d{1,2})(?:h(\d{2})?|:(\d{2}))\b`)
)

// plateWords says a plate letter by letter and digit by digit: "ABC1D23" is "á bê
// cê um dê dois três".
func plateWords(m string) string { return SpellAcronym(m) }

// ipWords says an address of a network: each part digit by digit, a "ponto"
// between them, as a person dictates it. A part above 255 means it is not an
// address, and a longer chain of numbers ("1.2.3.4.5") is not one either.
func ipWords(s string, m []int) (string, int, bool) {
	words := make([]string, 0, 4)
	for i := 1; i <= 4; i++ {
		part := s[m[2*i]:m[2*i+1]]
		if n, _ := strconv.Atoi(part); n > 255 {
			return "", 0, false
		}
		words = append(words, digitsInWords(part))
	}
	if m[1]+1 < len(s) && s[m[1]] == '.' && s[m[1]+1] >= '0' && s[m[1]+1] <= '9' {
		return "", 0, false
	}
	if m[0] > 0 && (s[m[0]-1] == '.' || s[m[0]-1] == '/') {
		return "", 0, false
	}
	return strings.Join(words, ", ponto, "), m[1], true
}

// versionWords says a version number: "V2.1" is "versão dois ponto um". A part
// with a leading zero ("05") is a code and is said digit by digit.
func versionWords(s string, m []int) (string, int, bool) {
	prefix := "versão "
	if m[2] >= 0 || strings.EqualFold(lastWord(s[:m[0]]), "versão") || strings.EqualFold(lastWord(s[:m[0]]), "versao") {
		prefix = "" // "Versão V2.1": the word is already there
		if m[2] >= 0 {
			prefix = s[m[2]:m[3]] + " "
		}
	}
	parts := strings.Split(s[m[4]:m[5]], ".")
	words := make([]string, len(parts))
	for i, p := range parts {
		if len(p) > 1 && p[0] == '0' || len(p) > 4 {
			words[i] = digitsInWords(p)
			continue
		}
		n, _ := strconv.ParseInt(p, 10, 64)
		words[i] = NumberInWords(n)
	}
	return prefix + strings.Join(words, " ponto "), m[1], true
}

// dayCountWords says D+1 as "dê mais um".
func dayCountWords(m string) string {
	sign := "mais"
	if m[1] == '-' {
		sign = "menos"
	}
	n, _ := strconv.ParseInt(m[2:], 10, 64)
	return "dê " + sign + " " + NumberInWords(n)
}

// beforeADate are the words before "24/7" that make it a date and not the hours
// of a service: "reunião em 24/7", "até 24/7".
var beforeADate = map[string]bool{"em": true, "no": true, "dia": true, "até": true, "ate": true, "desde": true, "para": true, "pra": true, "de": true, "do": true, "da": true, "na": true}

// allDayWords says 24/7 as the hours of a service, unless a word before it
// makes it a date.
func allDayWords(s string, m []int) (string, int, bool) {
	if beforeADate[strings.ToLower(lastWord(s[:m[0]]))] {
		return "", 0, false
	}
	return "vinte e quatro horas por dia, sete dias por semana", m[1], true
}

// intlPhoneWords says a telephone number with its country code digit by digit,
// in groups, after "mais".
func intlPhoneWords(s string, m []int) (string, int, bool) {
	if m[0] > 0 && isWordRune(runeBefore(s, m[0])) {
		return "", 0, false
	}
	var groups []string
	for i := 1; i <= 4; i++ {
		groups = append(groups, digitsInWords(s[m[2*i]:m[2*i+1]]))
	}
	return "mais " + strings.Join(groups, ", "), m[1], true
}

// tollFreeWords says 0800 123 4567 digit by digit, in groups.
func tollFreeWords(m string) string {
	var groups []string
	for _, g := range regexp.MustCompile(`\d+`).FindAllString(m, -1) {
		groups = append(groups, digitsInWords(g))
	}
	return strings.Join(groups, ", ")
}

// codeWords says the code after "protocolo", "código", "pedido": a run of
// letters and digits is dictated one by one, in the groups the hyphens make. A
// word with no digit ("pedido pendente") is not a code.
func codeWords(s string, m []int) (string, int, bool) {
	code := s[m[6]:m[7]]
	hasDigit, hasLetter := false, false
	for _, r := range code {
		hasDigit = hasDigit || unicode.IsDigit(r)
		hasLetter = hasLetter || unicode.IsLetter(r)
	}
	if !hasDigit {
		return "", 0, false
	}
	// A code of letters and digits is spelled; a plain number is only a number
	// when it is short ("pedido 15" is the fifteenth).
	if !hasLetter && len(strings.ReplaceAll(code, "-", "")) < 4 {
		return "", 0, false
	}
	groups := strings.Split(code, "-")
	for i, g := range groups {
		groups[i] = SpellAcronym(g)
	}
	return s[m[2]:m[3]] + s[m[4]:m[5]] + strings.Join(groups, ", "), m[1], true
}

// hourRangeWords says "08h às 17h" as "das oito às dezessete horas": the first
// hour without its word, the last one with it, and the "de" that came before the
// range ("de 08h às 17h") folded into the "das".
func hourRangeWords(s string, m []int) (string, int, bool) {
	h1, _ := strconv.Atoi(s[m[2]:m[3]])
	h2, _ := strconv.Atoi(s[m[8]:m[9]])
	m1, m2 := 0, 0
	for _, idx := range [][2]int{{4, 5}, {6, 7}} {
		if m[idx[0]] >= 0 {
			m1, _ = strconv.Atoi(s[m[idx[0]]:m[idx[1]]])
		}
	}
	for _, idx := range [][2]int{{10, 11}, {12, 13}} {
		if m[idx[0]] >= 0 {
			m2, _ = strconv.Atoi(s[m[idx[0]]:m[idx[1]]])
		}
	}
	if h1 > 23 || h2 > 24 || m1 > 59 || m2 > 59 || (h1 == h2 && m1 == m2) {
		return "", 0, false
	}
	before := strings.ToLower(lastWord(s[:m[0]]))
	article := "das"
	switch {
	case before == "de":
		// "de 08h às 17h": the "de" is folded into the article.
		trimmed := strings.TrimRight(s[:m[0]], " ")
		m[0] = len(trimmed) - 2
	case before == "das" || before == "às" || before == "as" || before == "da" || before == "do":
		article = ""
	}
	first := hourOfRange(h1, m1, false)
	switch {
	case article == "":
	case h1 == 1:
		article = "da"
	case h1 == 0:
		article = "da"
	case h1 == 12 && m1 == 0:
		article = "do"
	}
	connector := "às"
	switch {
	case h2 == 1:
		connector = "à"
	case h2 == 12 && m2 == 0:
		connector = "ao"
	case h2 == 0 || h2 == 24:
		connector = "à"
	}
	last := hourOfRange(h2%24, m2, true)
	out := first + " " + connector + " " + last
	if article != "" {
		out = article + " " + out
	}
	return out, m[1], true
}

// hourOfRange says an hour of a range: "oito", "oito e meia", "meio-dia", and with
// its word ("dezessete horas") when it is the last one.
func hourOfRange(h, mi int, withWord bool) string {
	if mi != 0 || h == 0 || h == 12 {
		return clockOfDay(h, mi)
	}
	if withWord {
		return countWords(int64(h), "hora", "horas", true)
	}
	return NumberInWordsFeminine(int64(h))
}
