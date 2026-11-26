package piper

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// NormalizerVersion names the rules of the verbalizer. It changes whenever a
// rule changes what a text is turned into, so an application that stores the
// original text and the spoken text beside it can tell which rules made the
// second, and put the version in the key of a cache. The text of the
// version is "ptbr-" and a number; Normalizer.Version adds the lexicon of the
// application when it has one.
const NormalizerVersion = "ptbr-1"

// Speech configures how the text is turned into what a voice says. The zero
// value is the default: Brazilian Portuguese, the base lexicon, every rule on.
type Speech struct {
	// Raw turns the verbalizer off: the text is spoken as it is given, with
	// only its spacing cleaned. Use it when the application has already made the
	// spoken text (with a Normalizer of its own, or by hand).
	Raw bool
	// Lexicon holds the words of the application's domain that a voice reads
	// wrong ("supino", "whey", the name of the product). It is applied before
	// BaseLexicon and wins over it.
	Lexicon []LexiconEntry
	// MaxWords cuts a text longer than this many words at the last sentence
	// that fits, and appends RestNote. Zero does not cut.
	MaxWords int
	// RestNote is said after a cut text, such as "O resto eu deixei por escrito."
	// It is only used when MaxWords is set.
	RestNote string
	// SkipLinePrefixes drops every line that starts with one of them, in any
	// case: "Fonte:" in a text that is spoken has no use.
	SkipLinePrefixes []string
	// KeepCapitals leaves a word typed in capitals ("SIM") as it is. By default
	// it is said as a word, in lower case, after the lexicon has spelled the
	// acronyms it knows.
	KeepCapitals bool
}

// Normalizer turns the text of a chat message into the text a voice says:
// numbers, money, dates, times, units and symbols in words, the lexicon
// spelled, formatting and emoji gone, sentences short and punctuated, since the
// punctuation is what the engines turn into pauses. It is deterministic and
// safe for concurrent use. Build one with NewNormalizer.
type Normalizer struct {
	raw      bool
	rules    []lexiconRule
	maxWords int
	restNote string
	skip     []string
	keepCaps bool
	version  string
}

// NewNormalizer builds a Normalizer from speech.
func NewNormalizer(speech Speech) *Normalizer {
	entries := append(append([]LexiconEntry(nil), speech.Lexicon...), baseLexicon...)
	n := &Normalizer{
		raw:      speech.Raw,
		rules:    compileLexicon(entries),
		maxWords: speech.MaxWords,
		restNote: speech.RestNote,
		keepCaps: speech.KeepCapitals,
	}
	if n.maxWords < 0 {
		n.maxWords = 0
	}
	for _, p := range speech.SkipLinePrefixes {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			n.skip = append(n.skip, p)
		}
	}
	n.version = NormalizerVersion
	if speech.Raw {
		n.version = "raw"
	} else if len(speech.Lexicon) > 0 || n.maxWords > 0 || len(n.skip) > 0 || speech.KeepCapitals {
		h := sha256.New()
		fmt.Fprintf(h, "%d\x00%s\x00%v\x00", n.maxWords, n.restNote, n.keepCaps)
		for _, e := range speech.Lexicon {
			fmt.Fprintf(h, "%s\x00%s\x00%v\x00", e.Written, e.Said, e.AnyCase)
		}
		for _, p := range n.skip {
			fmt.Fprintf(h, "skip\x00%s\x00", p)
		}
		n.version += "+" + hex.EncodeToString(h.Sum(nil))[:8]
	}
	return n
}

var defaultNormalizer = NewNormalizer(Speech{})

// Speakable turns text into what a voice says, with the default Normalizer and
// dates in full ("26/11/2026" is "vinte e seis de novembro de dois mil e vinte
// e seis"). It does not touch Plain's contract: Plain only removes formatting
// and emoji, and Speakable does that and all the rest.
func Speakable(text string) string {
	return defaultNormalizer.Speakable(text, time.Time{})
}

// Version is the version of this normalizer: NormalizerVersion, followed by "+"
// and eight hexadecimal characters that stand for the lexicon and the options
// when the application set any, or "raw" for a Raw one.
func (n *Normalizer) Version() string { return n.version }

// Speakable turns text into what a voice says. now is the moment the text is
// spoken: with it a date is said the way people say it ("hoje", "amanhã", "dia
// vinte", "dia vinte de dezembro"); with the zero time a date is said in full.
// The result is one line, empty when nothing speakable is left, and saying it
// again changes nothing, so a text that was already made speakable may go through
// again.
func (n *Normalizer) Speakable(text string, now time.Time) string {
	if n.raw {
		return normalize(text)
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.Map(func(r rune) rune {
		if r != '\n' && (unicode.IsControl(r) || r == '\u2028' || r == '\u2029') {
			return ' '
		}
		return r
	}, text)

	var lines []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || rulePattern.MatchString(line) || n.skipped(line) {
			continue
		}
		line = headingPattern.ReplaceAllString(line, "")
		line = quotePattern.ReplaceAllString(line, "")
		line = bulletPattern.ReplaceAllString(line, "")
		line = numberPattern.ReplaceAllString(line, "")
		line = n.line(line, now)
		if line == "" || !speakable(line) {
			continue
		}
		lines = append(lines, endLine(line))
	}
	s := strings.Join(lines, " ")
	if !n.keepCaps {
		s = reCapitals.ReplaceAllStringFunc(s, strings.ToLower)
	}
	s = tidy(s)
	s = breaths(s)
	s = n.limit(s)
	if !speakable(s) {
		return ""
	}
	return s
}

func (n *Normalizer) skipped(line string) bool {
	lower := strings.ToLower(line)
	for _, p := range n.skip {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

// endLine closes a line with a full stop, so that the lines of a list, which
// were separate items, are separate sentences with a pause between them once
// they are joined.
func endLine(line string) string {
	switch last, _ := utf8.DecodeLastRuneInString(line); last {
	case '.', '!', '?', ':', ';':
		return line
	case ',':
		return strings.TrimRight(line, ",") + "."
	}
	return line + "."
}

var (
	reMdLink    = regexp.MustCompile(`\[([^\]\n]+)\]\([^)\s]+\)`)
	reEmail     = regexp.MustCompile(`[\p{L}\d._%+-]+@[\p{L}\d-]+(?:\.[\p{L}\d-]+)*\.\p{L}{2,}`)
	reApprox    = regexp.MustCompile(`≈\s?|~\s?(\d)`)
	reCapitals  = regexp.MustCompile(`\b[A-ZÀ-Ý]{3,}\b`)
	reDoubled   = regexp.MustCompile(`,(\s*,)+`)
	reRepeated  = regexp.MustCompile(`([!?])[!?]+`)
	reDots      = regexp.MustCompile(`\.{2,}`)
	reParen     = regexp.MustCompile(`\s*[(\[]([^()\[\]\n]*)[)\]]`)
	reSign      = regexp.MustCompile(`(^|[\s(:;,])([-−+])(\d)`)
	reEOu       = regexp.MustCompile(`(?i)\be/ou\b`)
	reSlashWord = regexp.MustCompile(`\s?/\s?`)
	reSpacedDsh = regexp.MustCompile(`\s[-–—]\s`)
	reAmper     = regexp.MustCompile(`\s?&\s?`)
	rePlus      = regexp.MustCompile(`\s\+\s`)
	reHashNum   = regexp.MustCompile(`#(\d)`)
	reSlashPer  = regexp.MustCompile(`(?i)\s?/\s?(mês|mes|ano|dia|semana|hora|min|minuto|treino|série|serie|repetição|rep|sessão|aula|parcela|pessoa|unidade|refeição)\b`)
)

// line makes one line speakable: the order is the order of precedence of the
// rules, from the largest unit ("a link", an e-mail) to the smallest (a bare
// number), so that a rule never takes apart what an earlier one has said.
func (n *Normalizer) line(line string, now time.Time) string {
	line = reMdLink.ReplaceAllString(line, "$1")
	line = reEmail.ReplaceAllStringFunc(line, emailWords)
	line = replaceURLs(line)
	line = reApprox.ReplaceAllString(line, "cerca de $1")
	line = plainLine(line)
	for _, r := range n.rules {
		line = r.apply(line)
	}

	line = reIDs.ReplaceAllStringFunc(line, idWords)
	line = reISODate.ReplaceAllStringFunc(line, func(m string) string {
		p := reISODate.FindStringSubmatch(m)
		y, _ := strconv.Atoi(p[1])
		mo, _ := strconv.Atoi(p[2])
		d, _ := strconv.Atoi(p[3])
		if out, ok := dateWords(d, mo, y, true, now); ok {
			return out
		}
		return m
	})
	line = reDateFull.ReplaceAllStringFunc(line, func(m string) string {
		p := reDateFull.FindStringSubmatch(m)
		d, _ := strconv.Atoi(p[1])
		mo, _ := strconv.Atoi(p[2])
		y, _ := strconv.Atoi(p[3])
		if out, ok := dateWords(d, mo, y, true, now); ok {
			return out
		}
		return m
	})
	line = sub(reFraction, line, func(m []int) (string, int, bool) { return fractionOf(line, m) })
	line = reDateShort.ReplaceAllStringFunc(line, func(m string) string {
		p := reDateShort.FindStringSubmatch(m)
		d, _ := strconv.Atoi(p[1])
		mo, _ := strconv.Atoi(p[2])
		if out, ok := dateWords(d, mo, yearOf(now), false, now); ok {
			return out
		}
		return m
	})
	line = reSlashPair.ReplaceAllStringFunc(line, func(m string) string {
		p := reSlashPair.FindStringSubmatch(m)
		return sayNumber(p[1], false) + ", " + sayNumber(p[2], false)
	})
	line = sub(reClock3, line, func(m []int) (string, int, bool) { return clockDuration3(line, m) })
	line = sub(reClock, line, func(m []int) (string, int, bool) { return clockWords(line, m) })
	line = sub(reHourH, line, func(m []int) (string, int, bool) { return hourHWords(line, m) })

	line = reSlashPer.ReplaceAllString(line, " por $1")
	line = sub(reMoney, line, func(m []int) (string, int, bool) { return moneyWords(line, m) })
	line = sub(reSeries, line, func(m []int) (string, int, bool) { return seriesWords(line, m) })
	line = sub(reTimes, line, func(m []int) (string, int, bool) { return timesWords(line, m) })
	line = sub(reRange, line, func(m []int) (string, int, bool) { return rangeWords(line, m) })
	line = sub(reOrdinal, line, func(m []int) (string, int, bool) { return ordinalWords(line, m) })
	line = reNumberSign.ReplaceAllString(line, "número ")
	line = reHashNum.ReplaceAllString(line, "número $1")
	line = reSign.ReplaceAllStringFunc(line, func(m string) string {
		p := reSign.FindStringSubmatch(m)
		word := "menos "
		if p[2] == "+" {
			word = "mais "
		}
		return p[1] + word + p[3]
	})
	line = sub(reQuantity, line, func(m []int) (string, int, bool) { return quantityWords(line, m) })

	line = replaceAbbreviations(line)
	line = strings.ReplaceAll(line, "R$", "reais")
	line = reEOu.ReplaceAllString(line, "e ou")
	line = reAmper.ReplaceAllString(line, " e ")
	line = rePlus.ReplaceAllString(line, " mais ")
	line = reParen.ReplaceAllString(line, ", $1,")
	line = reSpacedDsh.ReplaceAllString(line, ", ")
	line = reSlashWord.ReplaceAllString(line, ", ")
	line = strings.NewReplacer(
		"≥", " maior ou igual a ", "≤", " menor ou igual a ", "=", " igual a ", "°", " graus ", "%", " por cento ",
		"·", ", ", "•", ", ", "—", ", ", "–", ", ", "…", ".", " | ", ", ", "@", " arroba ",
	).Replace(line)
	line = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.So, r) || unicode.Is(unicode.Sk, r) {
			return -1
		}
		return r
	}, line)
	line = reRepeated.ReplaceAllString(line, "$1")
	line = reDots.ReplaceAllString(line, ".")
	return tidy(line)
}

// tidy collapses the punctuation the rules leave behind: doubled commas, a
// comma before a full stop, white space before a mark.
func tidy(s string) string {
	s = reDoubled.ReplaceAllString(s, ",")
	for _, p := range []string{",.", ", .", ",!", ",?", ",;", ",:"} {
		for strings.Contains(s, p) {
			s = strings.ReplaceAll(s, p, string(p[len(p)-1]))
		}
	}
	s = spaceBeforePunctuation.ReplaceAllString(s, "$1")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimLeft(s, ", ")
}

// sub replaces the matches of re in s with what fn says: the text, the offset
// in s where the match ends (past m[1] when fn took more text with it, such as
// the unit after a range) and whether to replace at all.
func sub(re *regexp.Regexp, s string, fn func(m []int) (string, int, bool)) string {
	var b strings.Builder
	last := 0
	changed := false
	for _, m := range re.FindAllStringSubmatchIndex(s, -1) {
		if m[0] < last {
			continue
		}
		out, end, ok := fn(m)
		if !ok {
			continue
		}
		b.WriteString(s[last:m[0]])
		b.WriteString(out)
		last = end
		changed = true
	}
	if !changed {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}

// ---- e-mail and links ----

func emailWords(m string) string {
	local, domain, _ := strings.Cut(m, "@")
	say := strings.NewReplacer(".", " ponto ", "_", " underline ", "-", " hífen ", "+", " mais ")
	return say.Replace(local) + " arroba " + say.Replace(domain)
}

// replaceURLs says a web address as "o link": a voice cannot read it out, and
// the text of the chat still has it. After a word that already says what it is
// ("link", "site") it is "mostrado na conversa".
func replaceURLs(line string) string {
	var b strings.Builder
	last := 0
	for _, m := range urlPattern.FindAllStringIndex(line, -1) {
		b.WriteString(line[last:m[0]])
		switch strings.ToLower(lastWord(line[:m[0]])) {
		case "link", "site", "endereço", "página", "pagina":
			b.WriteString("mostrado na conversa")
		default:
			b.WriteString("o link")
		}
		last = m[1]
	}
	b.WriteString(line[last:])
	return b.String()
}

// lastWord is the word that ends s, without the punctuation after it.
func lastWord(s string) string {
	s = strings.TrimRightFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	i := strings.LastIndexFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if i < 0 {
		return s
	}
	_, size := utf8.DecodeRuneInString(s[i:])
	return s[i+size:]
}

// nextWord is the word that starts s, after the white space, lower case.
func nextWord(s string) string {
	s = strings.TrimLeft(s, " \t")
	end := strings.IndexFunc(s, func(r rune) bool { return !unicode.IsLetter(r) })
	if end < 0 {
		end = len(s)
	}
	return strings.ToLower(s[:end])
}

func runeBefore(s string, i int) rune {
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return r
}

func runeAt(s string, i int) rune {
	r, _ := utf8.DecodeRuneInString(s[i:])
	return r
}

// ---- identifiers: documents, postcodes, telephones ----

var reIDs = regexp.MustCompile(`\b\d{3}\.\d{3}\.\d{3}-\d{2}\b|\b\d{2}\.\d{3}\.\d{3}/\d{4}-\d{2}\b|\b\d{5}-\d{3}\b|(?:\(\d{2}\)\s?|\b\d{2}\s)?\b9?\d{4}-\d{4}\b`)

// idWords says a document number, a postcode or a telephone digit by digit, in
// groups: a number said as a quantity cannot be written down by the listener.
func idWords(m string) string {
	groups := regexp.MustCompile(`\d+`).FindAllString(m, -1)
	out := make([]string, len(groups))
	for i, g := range groups {
		out[i] = digitsInWords(g)
	}
	return strings.Join(out, ", ")
}

// ---- dates ----

var (
	reISODate   = regexp.MustCompile(`\b(\d{4})-(\d{2})-(\d{2})\b`)
	reDateFull  = regexp.MustCompile(`\b(\d{1,2})/(\d{1,2})/(\d{4})\b`)
	reDateShort = regexp.MustCompile(`\b(\d{1,2})/(\d{1,2})\b`)
	reSlashPair = regexp.MustCompile(`\b(\d{2,4})/(\d{2})\b`)
	reFraction  = regexp.MustCompile(`\b([1-3])/([2-4])\b`)
)

var monthNames = [...]string{"janeiro", "fevereiro", "março", "abril", "maio", "junho", "julho", "agosto", "setembro", "outubro", "novembro", "dezembro"}

// dayWord is a day of the month: the first is "primeiro", the others are
// cardinals.
func dayWord(d int) string {
	if d == 1 {
		return "primeiro"
	}
	return NumberInWords(int64(d))
}

// dateWords says a date. A text that is not a date -- "26/27", a harvest -- is
// left as it was (ok is false). Without now the date is said in full; with it
// the date is said as people do, counted from now.
func dateWords(day, month, year int, hasYear bool, now time.Time) (string, bool) {
	if month < 1 || month > 12 || day < 1 || day > 31 || year < 1 || year > 9999 {
		return "", false
	}
	// A day without a year is checked against a leap year, so 29/02 is a date.
	check := year
	if !hasYear {
		check = 2000
	}
	if time.Date(check, time.Month(month), day, 0, 0, 0, 0, time.UTC).Day() != day {
		return "", false
	}
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if date.Day() != day {
		return "", false
	}
	if now.IsZero() {
		out := dayWord(day) + " de " + monthNames[month-1]
		if hasYear {
			out += " de " + NumberInWords(int64(year))
		}
		return out, true
	}
	return SayDay(date, now), true
}

func yearOf(now time.Time) int {
	if now.IsZero() {
		return 2000
	}
	return now.Year()
}

// SayDay is a day said as people do, counted from now: "hoje", "ontem",
// "anteontem", "amanhã", "depois de amanhã", "dia vinte" (in the same month),
// "dia vinte de dezembro" (in the same year) and, in another year, with the
// year in full. Only the calendar day of each argument counts, not the hour.
func SayDay(date, now time.Time) string {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	switch int(date.Sub(today).Hours() / 24) {
	case 0:
		return "hoje"
	case -1:
		return "ontem"
	case -2:
		return "anteontem"
	case 1:
		return "amanhã"
	case 2:
		return "depois de amanhã"
	}
	switch {
	case date.Year() == today.Year() && date.Month() == today.Month():
		return "dia " + dayWord(date.Day())
	case date.Year() == today.Year():
		return "dia " + dayWord(date.Day()) + " de " + monthNames[date.Month()-1]
	}
	return "dia " + dayWord(date.Day()) + " de " + monthNames[date.Month()-1] + " de " + NumberInWords(int64(date.Year()))
}

// fractionOf says the fractions that are written with one digit: "1/2" is
// "meio" ("meia" before a feminine noun), "3/4" "três quartos".
func fractionOf(s string, m []int) (string, int, bool) {
	num, den := s[m[2]:m[3]], s[m[4]:m[5]]
	a, _ := strconv.Atoi(num)
	d, _ := strconv.Atoi(den)
	if a >= d {
		return "", 0, false
	}
	fem := feminineNouns[nextWord(s[m[1]:])]
	switch {
	case d == 2:
		if fem {
			return "meia", m[1], true
		}
		return "meio", m[1], true
	case d == 3 && a == 1:
		return "um terço", m[1], true
	case d == 3:
		return "dois terços", m[1], true
	case a == 1:
		return "um quarto", m[1], true
	}
	return "três quartos", m[1], true
}

// ---- times ----

var (
	reClock3 = regexp.MustCompile(`\b(\d{1,2}):(\d{2}):(\d{2})\b`)
	reClock  = regexp.MustCompile(`\b(\d{1,2}):(\d{2})\b`)
	reHourH  = regexp.MustCompile(`\b(\d{1,2})h(\d{2})?\b`)
)

// clockDuration3 says 1:02:30, a duration, "uma hora, dois minutos e trinta
// segundos".
func clockDuration3(s string, m []int) (string, int, bool) {
	h, _ := strconv.Atoi(s[m[2]:m[3]])
	mi, _ := strconv.Atoi(s[m[4]:m[5]])
	se, _ := strconv.Atoi(s[m[6]:m[7]])
	if mi > 59 || se > 59 {
		return "", 0, false
	}
	var parts []string
	if h > 0 {
		parts = append(parts, countWords(int64(h), "hora", "horas", true))
	}
	if mi > 0 || h > 0 {
		parts = append(parts, countWords(int64(mi), "minuto", "minutos", false))
	}
	parts = append(parts, countWords(int64(se), "segundo", "segundos", false))
	return joinAnd(parts), m[1], true
}

func joinAnd(parts []string) string {
	if len(parts) < 2 {
		return strings.Join(parts, "")
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " e " + parts[len(parts)-1]
}

func countWords(n int64, one, many string, fem bool) string {
	words := NumberInWords(n)
	if fem {
		words = NumberInWordsFeminine(n)
	}
	if n == 1 {
		return words + " " + one
	}
	return words + " " + many
}

var rePace = regexp.MustCompile(`^\s?(?:/|por\s)\s?km\b`)

// clockWords says HH:MM. It is the time of day, "seis e meia", "dezoito e
// quarenta e cinco", "meio-dia", except before "/km" (a pace, "cinco minutos e
// trinta segundos por quilômetro").
func clockWords(s string, m []int) (string, int, bool) {
	h, _ := strconv.Atoi(s[m[2]:m[3]])
	mi, _ := strconv.Atoi(s[m[4]:m[5]])
	if mi > 59 {
		return "", 0, false
	}
	if loc := rePace.FindStringIndex(s[m[1]:]); loc != nil {
		out := countWords(int64(h), "minuto", "minutos", false) + " e " + countWords(int64(mi), "segundo", "segundos", false)
		return out + " por quilômetro", m[1] + loc[1], true
	}
	if h > 23 {
		return "", 0, false
	}
	return clockOfDay(h, mi), m[1], true
}

func clockOfDay(h, mi int) string {
	var hour string
	switch h {
	case 0:
		hour = "meia-noite"
	case 12:
		hour = "meio-dia"
	default:
		hour = NumberInWordsFeminine(int64(h))
	}
	switch {
	case mi == 0 && h != 0 && h != 12:
		if h == 1 {
			return hour + " hora"
		}
		return hour + " horas"
	case mi == 0:
		return hour
	case mi == 30:
		return hour + " e meia"
	}
	return hour + " e " + NumberInWords(int64(mi))
}

// clockPrefix are the words before "7h30" that make it the time of day ("às
// 7h30", "depois das 18h15") and not a duration ("treino de 1h30").
var clockPrefix = map[string]bool{"às": true, "as": true, "à": true, "das": true, "até": true, "ate": true, "pelas": true, "umas": true}

// hourHWords says 7h, 7h30 and 18h15.
func hourHWords(s string, m []int) (string, int, bool) {
	h, _ := strconv.Atoi(s[m[2]:m[3]])
	mi := 0
	if m[4] >= 0 {
		mi, _ = strconv.Atoi(s[m[4]:m[5]])
	}
	if mi > 59 || h > 24 {
		return "", 0, false
	}
	if m[4] < 0 || mi == 0 {
		return countWords(int64(h), "hora", "horas", true), m[1], true
	}
	if clockPrefix[strings.ToLower(lastWord(s[:m[0]]))] && h < 24 {
		return clockOfDay(h, mi), m[1], true
	}
	minutes := "meia"
	if mi != 30 {
		minutes = countWords(int64(mi), "minuto", "minutos", false)
	}
	return countWords(int64(h), "hora", "horas", true) + " e " + minutes, m[1], true
}

// ---- money ----

var reMoney = regexp.MustCompile(`([-−+])?(R\$|US\$|\$|€)\s?(` + numPattern + `)(?:\s(milhões|milhão|bilhões|bilhão|mil))?`)

// moneyWords says an amount of money the way a person does: "R$ 24,90" is
// "vinte e quatro reais e noventa centavos", "R$ 1,00" "um real", "R$ 2 mil"
// "dois mil reais", "R$ 3 milhões" "três milhões de reais", "-R$ 50" "menos
// cinquenta reais".
func moneyWords(s string, m []int) (string, int, bool) {
	cur := currencies[s[m[4]:m[5]]]
	raw := s[m[6]:m[7]]
	whole, frac, hasFrac, ok := splitNumber(raw)
	if !ok {
		return "", 0, false
	}
	prefix := ""
	if m[2] >= 0 {
		sign := s[m[2]:m[3]]
		if m[0] == 0 || strings.ContainsRune(" \t(:;,", runeBefore(s, m[0])) {
			switch sign {
			case "+":
				prefix = "mais "
			default:
				prefix = "menos "
			}
		} else {
			prefix = sign
		}
	}
	end := m[1]
	if m[8] >= 0 && isWordRune(runeAt(s, m[9])) {
		end = m[7] // "R$ 2 milagres": the amount ends at the number.
		m[8] = -1
	}
	if m[8] >= 0 {
		// "R$ 2 mil": the multiplier is part of the amount.
		out := NumberInWords(whole)
		if hasFrac {
			out += " vírgula " + fractionWords(frac)
		}
		if mult := s[m[8]:m[9]]; mult == "mil" {
			return prefix + out + " mil " + cur.many, end, true
		} else {
			return prefix + out + " " + mult + " " + cur.millionDe, end, true
		}
	}
	if hasFrac && len(frac) > 2 {
		return prefix + NumberInWords(whole) + " vírgula " + fractionWords(frac) + " " + cur.many, end, true
	}
	cents := whole * 100
	if hasFrac {
		c, _ := strconv.ParseInt(frac, 10, 64)
		if len(frac) == 1 {
			c *= 10
		}
		cents += c
	}
	return prefix + currencyInWords(cents, cur), end, true
}

// ---- numbers ----

// numPattern is a number as it is written in Brazil: dots between thousands
// and a comma for the decimals ("1.234,56"), or a point for one or two
// decimals ("42.5").
const numPattern = `\d{1,3}(?:\.\d{3})+(?:,\d+)?|\d+,\d+|\d+\.\d{1,2}\b|\d+`

var (
	reSeries     = regexp.MustCompile(`\b(\d{1,3})\s?[x×]\s?(\d)`)
	reTimes      = regexp.MustCompile(`\b(\d{1,3})\s?[x×]`)
	reRange      = regexp.MustCompile(`\b(\d{1,4}(?:,\d+)?)\s?[-–]\s?(\d{1,4}(?:,\d+)?)\b`)
	reOrdinal    = regexp.MustCompile(`\b(\d{1,3})\s?([ºª])`)
	reNumberSign = regexp.MustCompile(`\b[Nn](?:\.\s?)?[º°]\.?\s?`)
	reQuantity   = regexp.MustCompile(`(` + numPattern + `)(?:(\s?)(` + unitPattern + `))?`)
	reUnitAhead  = regexp.MustCompile(`^(\s?)(` + unitPattern + `)`)
)

// splitNumber reads a written number: the whole part, the decimals as written,
// whether there are any, and whether it is a number at all.
func splitNumber(raw string) (whole int64, frac string, hasFrac, ok bool) {
	intPart := raw
	if i := strings.Index(raw, ","); i >= 0 {
		intPart, frac, hasFrac = raw[:i], raw[i+1:], true
	} else if i := strings.Index(raw, "."); i >= 0 && len(raw)-i-1 <= 2 {
		intPart, frac, hasFrac = raw[:i], raw[i+1:], true
	}
	intPart = strings.ReplaceAll(intPart, ".", "")
	n, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, "", false, false
	}
	return n, frac, hasFrac, true
}

// sayNumber says one written number. fem makes "um" and "dois" feminine. A run
// of nine digits or more with no separator is an identifier (a telephone, a
// document): it is said digit by digit.
func sayNumber(raw string, fem bool) string {
	if !strings.ContainsAny(raw, ".,") && len(raw) >= 9 {
		return digitsInWords(raw)
	}
	whole, frac, hasFrac, ok := splitNumber(raw)
	if !ok {
		return digitsInWords(raw)
	}
	if hasFrac {
		return NumberInWords(whole) + " vírgula " + fractionWords(frac)
	}
	if fem {
		return NumberInWordsFeminine(whole)
	}
	return NumberInWords(whole)
}

// isOne reports whether a written number is exactly one, which is the only
// quantity that takes the singular.
func isOne(raw string) bool {
	whole, _, hasFrac, ok := splitNumber(raw)
	return ok && !hasFrac && whole == 1
}

// quantity says a number with the unit or the noun that follows it, in
// agreement with it: "um quilo", "duas horas", "quarenta e dois vírgula cinco
// quilos", "duas séries".
func quantity(raw string, u *unit, next string) string {
	fem := (u != nil && u.fem) || (u == nil && feminineNouns[next])
	words := sayNumber(raw, fem)
	if whole, _, hasFrac, ok := splitNumber(raw); ok && !hasFrac && whole >= 1_000_000 && whole%1_000_000 == 0 && (u != nil || (next != "" && next != "de")) {
		// "dois milhões de contas", "um milhão de quilos".
		words += " de"
	}
	if u == nil {
		return words
	}
	if isOne(raw) {
		return words + " " + u.one
	}
	return words + " " + u.many
}

// quantityWords is the last rule: every number that is left, with its unit.
func quantityWords(s string, m []int) (string, int, bool) {
	if before := runeBefore(s, m[0]); m[0] > 0 && unicode.IsLetter(before) {
		return "", 0, false // "H2O", "mp3": part of a name
	}
	raw := s[m[2]:m[3]]
	end := m[3]
	var u *unit
	if m[6] >= 0 {
		if cand, ok := lookupUnit(s[m[6]:m[7]]); ok && !isWordRune(runeAt(s, m[7])) {
			u, end = &cand, m[7]
		}
	}
	if u == nil && end < len(s) && unicode.IsLetter(runeAt(s, end)) {
		return "", 0, false // "0x10", "3D", "5G": a name, not a quantity
	}
	next := ""
	if u == nil {
		next = nextWord(s[end:])
		if end < len(s) && s[end] != ' ' {
			next = ""
		}
	}
	return quantity(raw, u, next), end, true
}

// seriesWords says the fitness notation "3x10": "três séries de" and the
// number that follows it is said by the rules after this one. The letter x or
// the sign × between a count and a number is "séries de"; an application that
// means a multiplication writes "vezes".
func seriesWords(s string, m []int) (string, int, bool) {
	if m[0] > 0 && unicode.IsLetter(runeBefore(s, m[0])) {
		return "", 0, false
	}
	n, _ := strconv.Atoi(s[m[2]:m[3]])
	if n < 1 {
		return "", 0, false
	}
	return countWords(int64(n), "série", "séries", true) + " de " + s[m[4]:m[5]], m[1], true
}

// timesWords says "4x" as "quatro vezes", when no number follows it.
func timesWords(s string, m []int) (string, int, bool) {
	if m[0] > 0 && unicode.IsLetter(runeBefore(s, m[0])) {
		return "", 0, false
	}
	if m[1] < len(s) && isWordRune(runeAt(s, m[1])) {
		return "", 0, false
	}
	n, _ := strconv.Atoi(s[m[2]:m[3]])
	if n < 1 {
		return "", 0, false
	}
	return countWords(int64(n), "vez", "vezes", true), m[1], true
}

// rangeWords says "8-12" as "de oito a doze", with the unit or the noun that
// follows the range in agreement with both numbers ("de duas a três séries",
// "de oito a doze repetições"). A pair that is not a range -- a postcode, a
// score, a number with a leading zero -- is left alone.
func rangeWords(s string, m []int) (string, int, bool) {
	a, b := s[m[2]:m[3]], s[m[4]:m[5]]
	if m[0] > 0 {
		if r := runeBefore(s, m[0]); isWordRune(r) || r == '-' || r == '/' || r == '.' || r == ',' {
			return "", 0, false
		}
	}
	if m[1] < len(s) {
		if r := runeAt(s, m[1]); r == '-' || r == '/' || r == ':' || (r == ',' && m[1]+1 < len(s) && s[m[1]+1] >= '0' && s[m[1]+1] <= '9') {
			return "", 0, false
		}
	}
	if (len(a) > 1 && a[0] == '0' && a[1] != ',') || (len(b) > 1 && b[0] == '0' && b[1] != ',') {
		return "", 0, false
	}
	af, _, _, aok := splitNumber(a)
	bf, _, _, bok := splitNumber(b)
	if !aok || !bok || af > bf || (af == bf && a == b) {
		return "", 0, false
	}
	end := m[1]
	var u *unit
	if loc := reUnitAhead.FindStringSubmatchIndex(s[end:]); loc != nil {
		if cand, ok := lookupUnit(s[end+loc[4] : end+loc[5]]); ok && !isWordRune(runeAt(s, end+loc[5])) {
			u = &cand
			end += loc[5]
		}
	}
	next := ""
	if u == nil && end < len(s) && s[end] == ' ' {
		next = nextWord(s[end:])
	}
	fem := (u != nil && u.fem) || (u == nil && feminineNouns[next])
	var prefix, joiner string
	switch strings.ToLower(lastWord(s[:m[0]])) {
	case "entre":
		prefix, joiner = "", " e "
	case "de", "das", "dos", "até":
		prefix, joiner = "", " a "
	default:
		prefix, joiner = "de ", " a "
	}
	out := prefix + sayNumber(a, fem) + joiner + quantity(b, u, next)
	return out, end, true
}

// ordinalWords says "1º" as "primeiro" and "2ª" as "segunda". "25ºC" is a
// temperature and is left to the unit rule.
func ordinalWords(s string, m []int) (string, int, bool) {
	if next := runeAt(s, m[1]); m[1] < len(s) && (next == 'C' || next == 'F' || next == 'c' || next == 'f') {
		return "", 0, false
	}
	n, _ := strconv.Atoi(s[m[2]:m[3]])
	return OrdinalInWords(n, s[m[4]:m[5]] == "ª"), m[1], true
}

// ---- abbreviations ----

// abbreviations are said in full. A dot after one that is not the end of a
// sentence goes with it; "etc." keeps its dot when it ends the sentence.
var abbreviations = []struct {
	re   *regexp.Regexp
	said string
	stop bool // the dot is kept when a sentence ends here
}{
	{regexp.MustCompile(`\baprox\.`), "aproximadamente", false},
	{regexp.MustCompile(`\bp\.\s?ex\.`), "por exemplo", false},
	{regexp.MustCompile(`\bex\.(?:\s|:|$)`), "por exemplo ", false},
	{regexp.MustCompile(`\betc\.`), "etcétera", true},
	{regexp.MustCompile(`\bSra\.`), "senhora", false},
	{regexp.MustCompile(`\bSr\.`), "senhor", false},
	{regexp.MustCompile(`\bSrta\.`), "senhorita", false},
	{regexp.MustCompile(`\bDra\.`), "doutora", false},
	{regexp.MustCompile(`\bDr\.`), "doutor", false},
	{regexp.MustCompile(`\bProfa\.`), "professora", false},
	{regexp.MustCompile(`\bProf\.`), "professor", false},
	{regexp.MustCompile(`\bobs\.`), "observação", false},
	{regexp.MustCompile(`\bpágs\.`), "páginas", false},
	{regexp.MustCompile(`\bpág\.`), "página", false},
	{regexp.MustCompile(`\btel\.`), "telefone", false},
	{regexp.MustCompile(`\bcel\.`), "celular", false},
	{regexp.MustCompile(`\bmáx\.`), "máximo", false},
	{regexp.MustCompile(`\bmín\.`), "mínimo", false},
	{regexp.MustCompile(`\bvs\.?\s`), "versus ", false},
	{regexp.MustCompile(`\bc/\s`), "com ", false},
	{regexp.MustCompile(`\bs/\s`), "sem ", false},
	{regexp.MustCompile(`\bp/\s`), "para ", false},
	{regexp.MustCompile(`\bvc\b`), "você", false},
	{regexp.MustCompile(`\bvcs\b`), "vocês", false},
	{regexp.MustCompile(`\bpq\b`), "porque", false},
	{regexp.MustCompile(`\btb\b`), "também", false},
	{regexp.MustCompile(`\bhj\b`), "hoje", false},
}

func replaceAbbreviations(line string) string {
	for _, a := range abbreviations {
		line = sub(a.re, line, func(m []int) (string, int, bool) {
			out := a.said
			if a.stop {
				rest := strings.TrimLeft(line[m[1]:], " ")
				if rest == "" || unicode.IsUpper(runeAt(rest, 0)) {
					out += "."
				}
			}
			return out, m[1], true
		})
	}
	return line
}

// ---- prosody ----

// longSentence is how many words a sentence may run before the voice needs a
// place to breathe in it.
const longSentence = 16

var breathBefore = map[string]bool{"mas": true, "porque": true, "quando": true, "enquanto": true, "então": true, "depois": true}

// breaths puts a comma before a conjunction near the middle of a long
// sentence that has little punctuation, the place a person would breathe.
func breaths(s string) string {
	var out strings.Builder
	start := 0
	flush := func(end int) {
		out.WriteString(breatheIn(s[start:end]))
		start = end
	}
	for i := 0; i < len(s); i++ {
		if (s[i] == '.' || s[i] == '!' || s[i] == '?') && (i+1 == len(s) || s[i+1] == ' ') {
			flush(i + 1)
		}
	}
	flush(len(s))
	return out.String()
}

func breatheIn(sentence string) string {
	words := strings.Fields(sentence)
	if len(words) <= longSentence {
		return sentence
	}
	if strings.Count(sentence, ",")*7 >= len(words) {
		return sentence
	}
	best, bestGap := -1, len(words)
	mid := len(words) / 2
	for i := 4; i < len(words)-3; i++ {
		if !breathBefore[strings.ToLower(words[i])] || strings.HasSuffix(words[i-1], ",") {
			continue
		}
		gap := i - mid
		if gap < 0 {
			gap = -gap
		}
		if gap < bestGap {
			best, bestGap = i, gap
		}
	}
	if best < 0 {
		return sentence
	}
	words[best-1] += ","
	lead := ""
	if strings.HasPrefix(sentence, " ") {
		lead = " "
	}
	return lead + strings.Join(words, " ")
}

// limit cuts s at the last sentence that ends within MaxWords words and says
// the rest is written. A text with no sentence end inside the limit is cut at
// the limit.
func (n *Normalizer) limit(s string) string {
	if n.maxWords == 0 {
		return s
	}
	words := strings.Fields(s)
	if len(words) <= n.maxWords {
		return s
	}
	cut := strings.Join(words[:n.maxWords], " ")
	if i := strings.LastIndexAny(cut, ".!?"); i > len(cut)/3 {
		cut = cut[:i+1]
	} else {
		cut = strings.TrimRight(cut, ",;:") + "."
	}
	if n.restNote != "" {
		cut += " " + n.restNote
	}
	return cut
}
