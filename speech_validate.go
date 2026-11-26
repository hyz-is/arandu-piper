package piper

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// The validator stage. The text a person reads (the display text) and the text a
// voice says (the speech text) are two representations of one message, and the
// second must not lose or change what the first states: a quantity, a price, a
// date, an hour, a plate, a protocol. Validate reads the numbers and the
// identifiers of the display text, reads back the numbers the speech text says
// in words, and reports every value that was lost or altered, so the caller
// refuses the audio, falls back to the display text or asks a person. What it
// cannot decide it reports as a warning and never guesses.
//
// It is a safety net and not a proof: it compares what the two texts state, it
// does not judge how the speech sounds.

// Severity says what a caller should do with an Issue: an error means the speech
// text states something other than the display text; a warning is something a
// person may want to look at before the audio is made.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// IssueKind names the check that found an Issue.
type IssueKind string

const (
	// IssueMissing: a number, a date, an hour or an identifier of the display text
	// is not said in the speech text (or is said as another value).
	IssueMissing IssueKind = "missing"
	// IssueUnit: a unit of measure or a currency of the display text is not said,
	// or is said as another ("10 GB" spoken as "dez megabytes").
	IssueUnit IssueKind = "unit"
	// IssueAddress: an e-mail or a web address of the display text has no
	// counterpart in the speech text.
	IssueAddress IssueKind = "address"
	// IssueDigits: the speech text still holds digits, which an engine reads as it
	// can, without agreement, units or the context that makes them a quantity.
	IssueDigits IssueKind = "digits"
	// IssueAmbiguous: the text can be read in two ways and the speech text picked
	// one ("1/2" is a date or a fraction, "24/7" a date or round-the-clock, an
	// acronym the dictionary does not know is spelled or said as a word).
	IssueAmbiguous IssueKind = "ambiguous"
)

// Issue is one finding of Validate.
type Issue struct {
	Kind     IssueKind
	Severity Severity
	// Text is what the display text had: "R$ 15.000,00", "ABC1D23", "1/2", "GB".
	Text string
	// Detail says what is wrong in one sentence.
	Detail string
}

func (i Issue) String() string {
	return string(i.Severity) + " " + string(i.Kind) + ": " + i.Text + ": " + i.Detail
}

// HasErrors reports whether any issue is an error.
func HasErrors(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Validate compares the display text with the speech text and reports what the
// second lost or changed of the first. It knows the shared dictionary only; use
// Normalizer.Validate when the application has a lexicon of its own. Every
// number and identifier of display (quantities, money, percentages, dates,
// hours, telephones, documents, plates, addresses of a network, versions,
// protocols) must be said in speech, in the words of a cardinal, an ordinal, a
// day and a month, a clock, or digit by digit, and its unit or currency must be
// the same. A date said relative to now ("hoje", "amanhã") is accepted for a
// date. An ambiguous reading is a warning.
func Validate(display, speech string) []Issue {
	return validate(display, speech, defaultNormalizer.index)
}

// Validate is Validate with the lexicon of the normalizer, so an acronym of the
// application is not reported as guessed.
func (n *Normalizer) Validate(display, speech string) []Issue {
	return validate(display, speech, n.index)
}

// Check makes the speech text of text, and validates it against it, in one step:
// the text a voice would say and what is wrong with it. A caller that treats
// HasErrors as a refusal never sends the voice a value that changed.
func (n *Normalizer) Check(text string, now time.Time) (speech string, issues []Issue) {
	speech = n.Speakable(text, now)
	return speech, n.Validate(text, speech)
}

func validate(display, speech string, index map[string]string) []Issue {
	var issues []Issue
	seen := map[string]bool{}
	add := func(i Issue) {
		key := string(i.Kind) + "\x00" + i.Text
		if !seen[key] {
			seen[key] = true
			issues = append(issues, i)
		}
	}
	spoken := parseSpokenNumbers(speech)
	lower := strings.ToLower(speech)

	for _, f := range displayFacts(stripAddresses(display)) {
		if f.kept && strings.Contains(speech, f.text) {
			continue
		}
		if !f.satisfied(spoken, lower) {
			add(Issue{IssueMissing, SeverityError, f.text, f.detail()})
		}
		if f.ambiguous != "" {
			add(Issue{IssueAmbiguous, SeverityWarning, f.text, f.ambiguous})
		}
	}
	for _, i := range unitIssues(display, spoken, lower) {
		add(i)
	}
	for _, i := range addressIssues(display, lower) {
		add(i)
	}
	for _, i := range acronymIssues(display, index) {
		add(i)
	}
	for _, m := range reDigitRun.FindAllStringIndex(speech, -1) {
		if m[0] > 0 && unicode.IsLetter(runeBefore(speech, m[0])) || m[1] < len(speech) && unicode.IsLetter(runeAt(speech, m[1])) {
			continue // a name such as "H2O" or "5G", kept as written
		}
		add(Issue{IssueDigits, SeverityWarning, speech[m[0]:m[1]], "the speech text still has digits, which an engine reads without agreement or units"})
	}
	return issues
}

var reDigitRun = regexp.MustCompile(`\d+`)

// ---- what the speech text says ----

// spokenNumbers is every number the speech text says in words, in order, and
// the words of the days and the months it says.
type spokenNumbers struct {
	values []int64
	set    map[int64]bool
	words  map[string]bool
}

var (
	spokenUnits = map[string]int64{
		"zero": 0, "um": 1, "uma": 1, "dois": 2, "duas": 2, "três": 3, "tres": 3, "quatro": 4, "cinco": 5, "seis": 6, "sete": 7,
		"oito": 8, "nove": 9, "dez": 10, "onze": 11, "doze": 12, "treze": 13, "catorze": 14, "quatorze": 14, "quinze": 15,
		"dezesseis": 16, "dezessete": 17, "dezoito": 18, "dezenove": 19,
	}
	spokenTens = map[string]int64{
		"vinte": 20, "trinta": 30, "quarenta": 40, "cinquenta": 50, "sessenta": 60, "setenta": 70, "oitenta": 80, "noventa": 90,
	}
	spokenHundreds = map[string]int64{
		"cem": 100, "cento": 100, "duzentos": 200, "duzentas": 200, "trezentos": 300, "trezentas": 300, "quatrocentos": 400,
		"quatrocentas": 400, "quinhentos": 500, "quinhentas": 500, "seiscentos": 600, "seiscentas": 600, "setecentos": 700,
		"setecentas": 700, "oitocentos": 800, "oitocentas": 800, "novecentos": 900, "novecentas": 900,
	}
	spokenScales = map[string]int64{
		"mil": 1000, "milhão": 1_000_000, "milhao": 1_000_000, "milhões": 1_000_000, "milhoes": 1_000_000,
		"bilhão": 1_000_000_000, "bilhao": 1_000_000_000, "bilhões": 1_000_000_000, "bilhoes": 1_000_000_000,
		"trilhão": 1_000_000_000_000, "trilhao": 1_000_000_000_000, "trilhões": 1_000_000_000_000, "trilhoes": 1_000_000_000_000,
	}
	spokenOrdinalUnits = map[string]int64{
		"primeiro": 1, "primeira": 1, "segundo": 2, "segunda": 2, "terceiro": 3, "terceira": 3, "quarto": 4, "quarta": 4,
		"quinto": 5, "quinta": 5, "sexto": 6, "sexta": 6, "sétimo": 7, "sétima": 7, "oitavo": 8, "oitava": 8, "nono": 9, "nona": 9,
	}
	spokenOrdinalTens = map[string]int64{
		"décimo": 10, "décima": 10, "vigésimo": 20, "vigésima": 20, "trigésimo": 30, "trigésima": 30, "quadragésimo": 40,
		"quadragésima": 40, "quinquagésimo": 50, "quinquagésima": 50, "sexagésimo": 60, "sexagésima": 60, "septuagésimo": 70,
		"septuagésima": 70, "octogésimo": 80, "octogésima": 80, "nonagésimo": 90, "nonagésima": 90, "centésimo": 100, "centésima": 100,
	}
	// What a word says besides itself: "meia" is the half hour and the half,
	// "meio-dia" and "meia-noite" are hours, "quartos" and "terços" are the
	// denominators of a fraction.
	spokenSpecial = map[string][]int64{
		"meia": {30, 2}, "meio": {2}, "meiodia": {12, 0}, "meianoite": {0, 24}, "quarto": {4}, "quartos": {4}, "terço": {3}, "terços": {3},
	}
)

var reWords = regexp.MustCompile(`[\p{L}]+`)

func parseSpokenNumbers(speech string) spokenNumbers {
	text := strings.ToLower(speech)
	text = strings.NewReplacer("meio-dia", "meiodia", "meia-noite", "meianoite").Replace(text)
	words := reWords.FindAllString(text, -1)
	out := spokenNumbers{set: map[int64]bool{}, words: map[string]bool{}}
	emit := func(v int64) {
		out.values = append(out.values, v)
		out.set[v] = true
	}
	var (
		total, group, scaleSeen int64
		active, pendingAnd      bool
		lastKind                int // 0 none, 1 hundreds, 2 tens, 3 units
	)
	flush := func() {
		if active {
			emit(total + group)
		}
		total, group, scaleSeen, active, lastKind = 0, 0, 0, false, 0
	}
	for i := 0; i < len(words); i++ {
		w := words[i]
		out.words[w] = true
		and := pendingAnd
		pendingAnd = false
		if w == "e" {
			pendingAnd = active
			continue
		}
		if v, ok := spokenSpecial[w]; ok {
			flush()
			for _, x := range v {
				out.set[x] = true
			}
			continue
		}
		if v, ok := spokenOrdinalTens[w]; ok {
			flush()
			if i+1 < len(words) {
				if u, ok := spokenOrdinalUnits[words[i+1]]; ok && v < 100 {
					emit(v + u)
					out.set[v], out.set[u] = true, true
					i++
					continue
				}
			}
			emit(v)
			continue
		}
		if v, ok := spokenOrdinalUnits[w]; ok {
			flush()
			emit(v)
			continue
		}
		if w == "zero" {
			flush()
			emit(0)
			continue
		}
		switch {
		case spokenUnits[w] > 0:
			v := spokenUnits[w]
			if active && (lastKind == 3 || (lastKind == 2 && (!and || v >= 10)) || (lastKind == 1 && !and)) {
				flush()
			}
			group += v
			active, lastKind = true, 3
		case spokenTens[w] > 0:
			v := spokenTens[w]
			if active && lastKind >= 2 || active && lastKind == 1 && !and {
				flush()
			}
			group += v
			active, lastKind = true, 2
		case spokenHundreds[w] > 0:
			v := spokenHundreds[w]
			if active && lastKind != 0 {
				flush()
			}
			group += v
			active, lastKind = true, 1
		case spokenScales[w] > 0:
			s := spokenScales[w]
			if active && scaleSeen != 0 && s >= scaleSeen {
				flush()
			}
			if group == 0 {
				group = 1
			}
			out.set[group] = true // "dois mil": the 2 of "2 mil" is said too
			total += group * s
			group, lastKind = 0, 0
			scaleSeen, active = s, true
		default:
			flush()
		}
	}
	flush()
	return out
}

// hasDigitsInOrder reports whether the digits of run were said one by one, in
// order, among the numbers of the speech text: "um nove dois" for "192".
func (s spokenNumbers) hasDigitsInOrder(run string) bool {
	if len(run) < 2 {
		return false
	}
	for i := 0; i+len(run) <= len(s.values); i++ {
		ok := true
		for j := 0; j < len(run); j++ {
			if s.values[i+j] != int64(run[j]-'0') {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// ---- what the display text states ----

type factKind int

const (
	factNumber factKind = iota
	factDate
	factClock
	factDotted
	factInstallment // "3/12" after "parcela": two numbers, not a date
)

// fact is a number, a date, a clock or a dotted identifier of the display text.
type fact struct {
	kind      factKind
	text      string
	runs      []string // the digit runs, in order
	date      [3]int   // day, month, year (0 when absent)
	fraction  [2]int   // numerator and denominator when it can be a fraction
	kept      bool     // a name with a digit in it ("H2O"): said as written
	ambiguous string
}

var reFact = regexp.MustCompile(
	`(\d{1,2})/(\d{1,2})/(\d{4})|(\d{4})-(\d{2})-(\d{2})|(\d{1,2})/(\d{1,2})|(\d{1,2}):(\d{2})(?::(\d{2}))?|(\d{1,2})h(\d{2})?|(\d+(?:\.\d+){2,})|(\d{1,3}(?:\.\d{3})+(?:,\d+)?)|(\d+[.,]\d+)|(\d+)`)

var reGrouped = regexp.MustCompile(`^\d{1,3}(?:\.\d{3})+$`)

func displayFacts(display string) []fact {
	var facts []fact
	for _, m := range reFact.FindAllStringSubmatchIndex(display, -1) {
		text := display[m[0]:m[1]]
		before, after := rune(0), rune(0)
		if m[0] > 0 {
			before = runeBefore(display, m[0])
		}
		if m[1] < len(display) {
			after = runeAt(display, m[1])
		}
		if unicode.IsLetter(before) {
			// "H2O", "mp3", "3D" are names: kept as written, or a unit that follows
			// the number is checked by the unit rule.
			facts = append(facts, fact{kind: factNumber, text: text, runs: reDigitRun.FindAllString(text, -1), kept: true})
			continue
		}
		f := fact{text: text, runs: reDigitRun.FindAllString(text, -1)}
		num := func(g int) int {
			n, _ := strconv.Atoi(display[m[2*g]:m[2*g+1]])
			return n
		}
		has := func(g int) bool { return m[2*g] >= 0 }
		switch {
		case has(1): // dd/mm/yyyy
			f.kind, f.date = factDate, [3]int{num(1), num(2), num(3)}
		case has(4): // yyyy-mm-dd
			f.kind, f.date = factDate, [3]int{num(6), num(5), num(4)}
		case has(7): // dd/mm or a fraction
			f.kind, f.date = factDate, [3]int{num(7), num(8), 0}
			if installmentOf(display[:m[0]], display[m[1]:], num(7), num(8)) {
				f.kind = factInstallment
				break
			}
			if a, d := num(7), num(8); a >= 1 && a <= 3 && d >= 2 && d <= 4 && a < d {
				f.fraction = [2]int{a, d}
				f.ambiguous = "the text can be a date or a fraction; the speech text picked one"
			}
			if f.date[0] > 31 || f.date[1] > 12 || f.date[0] == 0 || f.date[1] == 0 {
				f.kind = factNumber // "26/27": a pair of numbers
			}
		case has(9): // clock
			f.kind = factClock
		case has(12): // 7h30
			f.kind = factClock
		case has(14):
			if reGrouped.MatchString(text) {
				f.kind = factNumber
			} else {
				f.kind = factDotted
			}
		default:
			f.kind = factNumber
		}
		if text == "24/7" && after != '/' {
			f.ambiguous = "24/7 is a date (24 July) or round-the-clock; the speech text picked one"
		}
		facts = append(facts, f)
	}
	return facts
}

func (f fact) detail() string {
	switch f.kind {
	case factDate:
		return "the date is not said in the speech text"
	case factClock:
		return "the hour is not said in the speech text"
	case factDotted:
		return "the identifier is not said digit by digit in the speech text"
	}
	return "the number is not said in the speech text"
}

// relativeDay are the words that say a date relative to now.
var relativeDay = []string{"hoje", "ontem", "anteontem", "amanhã", "amanha"}

func (f fact) satisfied(s spokenNumbers, lower string) bool {
	switch f.kind {
	case factDate:
		for _, w := range relativeDay {
			if s.words[w] {
				return true
			}
		}
		if f.fraction[1] != 0 && s.set[int64(f.fraction[0])] && s.set[int64(f.fraction[1])] {
			return true
		}
		if f.fraction == [2]int{1, 2} && (s.words["meia"] || s.words["meio"]) {
			return true // "1/2 xícara" is "meia xícara"
		}
		if f.text == "24/7" && strings.Contains(lower, "sete dias por semana") {
			return true // the service that never stops
		}
		day, month, year := f.date[0], f.date[1], f.date[2]
		if month < 1 || month > 12 {
			return runsSaid(f.runs, s)
		}
		if !s.set[int64(day)] || !s.words[monthNames[month-1]] {
			return false
		}
		return year == 0 || s.set[int64(year)]
	case factClock:
		hour, _ := strconv.Atoi(f.runs[0])
		if !s.set[int64(hour)] && !(hour == 12 && s.words["meiodia"]) {
			return false
		}
		for _, r := range f.runs[1:] {
			if n, _ := strconv.Atoi(r); n != 0 && !s.set[int64(n)] {
				return false
			}
		}
		return true
	case factDotted, factInstallment:
		return runsSaid(f.runs, s)
	}
	// A number: a grouped one ("1.250,50") is said as a whole, then its decimals.
	text := f.text
	whole, frac, hasFrac, ok := splitNumber(text)
	if ok && (strings.Contains(text, ",") || reGrouped.MatchString(text) || (hasFrac && len(frac) <= 2)) {
		if !s.set[whole] && !s.hasDigitsInOrder(strings.ReplaceAll(strings.SplitN(text, ",", 2)[0], ".", "")) {
			return false
		}
		if hasFrac && strings.Trim(frac, "0") != "" {
			return runsSaid([]string{frac}, s)
		}
		return true
	}
	return runsSaid(f.runs, s)
}

// runsSaid reports whether every run of digits is said, as a quantity or digit
// by digit.
func runsSaid(runs []string, s spokenNumbers) bool {
	for _, r := range runs {
		n, err := strconv.ParseInt(r, 10, 64)
		if err != nil {
			if !s.hasDigitsInOrder(r) {
				return false
			}
			continue
		}
		if n == 0 && strings.Trim(r, "0") == "" && len(r) > 1 {
			continue // "00": nothing to say
		}
		if s.set[n] || s.hasDigitsInOrder(r) {
			continue
		}
		return false
	}
	return true
}

// ---- units, currencies, addresses, acronyms ----

var currencyWords = map[string][]string{
	"R$": {"real", "reais"}, "US$": {"dólar", "dólares"}, "$": {"dólar", "dólares"}, "€": {"euro", "euros"},
}

func unitIssues(display string, spoken spokenNumbers, lowerSpeech string) []Issue {
	var issues []Issue
	for _, m := range reQuantity.FindAllStringSubmatchIndex(display, -1) {
		if m[6] < 0 || (m[0] > 0 && unicode.IsLetter(runeBefore(display, m[0]))) {
			continue
		}
		u, ok := lookupUnit(display[m[6]:m[7]])
		if !ok || isWordRune(runeAt2(display, m[7])) {
			continue
		}
		if display[m[6]:m[7]] == "%" {
			if !strings.Contains(lowerSpeech, "por cento") {
				issues = append(issues, Issue{IssueUnit, SeverityError, display[m[0]:m[7]], "the percent sign is not said as \"por cento\""})
			}
			continue
		}
		one, many := strings.Fields(u.one)[0], strings.Fields(u.many)[0]
		if !spoken.words[one] && !spoken.words[many] && !spoken.words[pronouncedUnit(one)] && !spoken.words[pronouncedUnit(many)] {
			issues = append(issues, Issue{IssueUnit, SeverityError, display[m[0]:m[7]], "the unit \"" + u.many + "\" is not said"})
		}
	}
	for _, m := range reMoney.FindAllStringSubmatchIndex(display, -1) {
		words := currencyWords[display[m[4]:m[5]]]
		if len(words) == 2 && !spoken.words[words[0]] && !spoken.words[words[1]] {
			issues = append(issues, Issue{IssueUnit, SeverityError, display[m[0]:m[1]], "the currency is not said (\"" + words[1] + "\")"})
		}
	}
	return issues
}

// stripAddresses blanks the e-mails and web addresses of a text: the digits in
// them ("app2.exemplo.com", "/pedido/123") are part of an address, not
// quantities the speech text must say.
func stripAddresses(display string) string {
	display = reEmail.ReplaceAllString(display, " ")
	display = urlPattern.ReplaceAllString(display, " ")
	return reDomain.ReplaceAllString(display, " ")
}

// runeAt2 is the rune at i, or zero past the end of s.
func runeAt2(s string, i int) rune {
	if i >= len(s) {
		return 0
	}
	return runeAt(s, i)
}

func addressIssues(display, lowerSpeech string) []Issue {
	var issues []Issue
	for _, e := range reEmail.FindAllString(display, -1) {
		if !strings.Contains(lowerSpeech, "arroba") {
			issues = append(issues, Issue{IssueAddress, SeverityError, e, "the e-mail has no \"arroba\" in the speech text"})
		}
	}
	rest := reEmail.ReplaceAllString(display, " ")
	for _, d := range reDomain.FindAllString(rest, -1) {
		d = strings.TrimRight(d, ".,;:!?)")
		if !strings.Contains(lowerSpeech, "ponto") && !strings.Contains(lowerSpeech, "link") && !strings.Contains(lowerSpeech, "conversa") && !strings.Contains(lowerSpeech, "endereço") {
			issues = append(issues, Issue{IssueAddress, SeverityError, d, "the address is neither said nor pointed at in the speech text"})
		}
	}
	return issues
}

func acronymIssues(display string, index map[string]string) []Issue {
	var issues []Issue
	for _, m := range reAcronym.FindAllStringIndex(display, -1) {
		token := display[m[0]:m[1]]
		if !wordEdges(display, m[0], m[1]) {
			continue
		}
		token = strings.TrimSuffix(token, "s")
		if len([]rune(token)) < 2 || onlyRoman(token) || len([]rune(token)) > 6 || afterANumber(display, m[0]) {
			continue
		}
		if m[1] < len(display) && display[m[1]] == '-' && m[1]+1 < len(display) && display[m[1]+1] >= '0' && display[m[1]+1] <= '9' {
			continue // the letters of a plate: ABC-1234
		}
		if _, known := index[foldKey(token)]; known || ReadsAsWord(token) {
			continue
		}
		issues = append(issues, Issue{IssueAmbiguous, SeverityWarning, token, "not in the dictionary: spelled letter by letter, a guess from its letters"})
	}
	return issues
}
