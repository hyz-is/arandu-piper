package piper

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The phonetic layer of the verbalizer. A voice engine takes text, not markup,
// so everything here is resolved in the spelling: an acronym spelled with the
// names of the letters, a domain said with "ponto" and "barra", a foreign word
// written the way a Brazilian says it. The accents are on purpose: they are what
// tells the engine where the stress falls and which vowels are open.

// letterNames are the names of the letters of the alphabet in Brazilian
// Portuguese, written the way the engines read them. The accents carry the
// stress and the open vowels ("éfe", "éle", "érre", "agá", "jota"), with one
// exception that was measured: espeak-ng spells out a single letter that has a
// circumflex or an acute on a vowel it names ("í" is read "i agudo", "ê" "e
// circunflexo", "ú" "u agudo"), so the vowels i and u are written plain ("i",
// "u", which are stressed when alone), and e and o get an "h" ("êh", "ôh"),
// which gives the closed vowel and the stress. "á", "é" and "ó" are read as
// letters and stay.
var letterNames = map[rune]string{
	'a': "á", 'b': "bê", 'c': "cê", 'd': "dê", 'e': "êh", 'f': "éfe", 'g': "gê", 'h': "agá", 'i': "i", 'j': "jota",
	'k': "cá", 'l': "éle", 'm': "ême", 'n': "êne", 'o': "ôh", 'p': "pê", 'q': "quê", 'r': "érre", 's': "ésse", 't': "tê",
	'u': "u", 'v': "vê", 'w': "dáblio", 'x': "xis", 'y': "ípsilon", 'z': "zê",
}

var letterNameSet = func() map[string]bool {
	set := map[string]bool{}
	for _, name := range letterNames {
		set[name] = true
	}
	return set
}()

// LetterName is the name of a letter in Brazilian Portuguese, written for a
// voice: "a" is "á", "b" "bê", "e" "êh", "f" "éfe", "h" "agá", "i" "i", "w"
// "dáblio", "y" "ípsilon". A letter with an
// accent is named as the letter without it. It is empty for a rune that is not
// a letter of the Latin alphabet.
func LetterName(r rune) string {
	return letterNames[foldAccent(unicode.ToLower(r))]
}

// SpellAcronym says a sequence of letters, and digits, one by one, with the
// names of the letters: "GPU" is "gê pê ú", "ALPR" "á éle pê érre". Whatever is
// not a letter or a digit is left out.
func SpellAcronym(s string) string {
	var parts []string
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			parts = append(parts, smallWords[r-'0'])
		case unicode.IsLetter(r):
			if name := LetterName(r); name != "" {
				parts = append(parts, name)
			}
		}
	}
	return strings.Join(parts, " ")
}

// foldAccent is the letter without its accent: "á" is "a", "ç" is "c".
func foldAccent(r rune) rune {
	switch r {
	case 'á', 'à', 'â', 'ã', 'ä':
		return 'a'
	case 'é', 'è', 'ê', 'ë':
		return 'e'
	case 'í', 'ì', 'î', 'ï':
		return 'i'
	case 'ó', 'ò', 'ô', 'õ', 'ö':
		return 'o'
	case 'ú', 'ù', 'û', 'ü':
		return 'u'
	case 'ç':
		return 'c'
	case 'ñ':
		return 'n'
	}
	return r
}

func isVowel(r rune) bool {
	switch foldAccent(r) {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	}
	return false
}

// ReadsAsWord decides how a token typed in capitals is said, when the
// dictionary does not know it: as a word ("NASA", "SIM", "ATENÇÃO") or spelled
// letter by letter ("GPU", "ALPR", "SDK"). It looks at the pattern of vowels and
// consonants the way a Portuguese syllable has it: a token with no vowel, or
// that starts with a cluster no Portuguese word starts with ("GP", "RP"), or
// that ends in a consonant or cluster no Portuguese word ends in ("ALPR",
// "IMC", "OCR", "USB"), is an acronym and is spelled. Anything else can be said
// as it is written. A short word with no vowel and one with a vowel in two
// letters ("EU", "OI") are left as they are.
//
// It is a heuristic, and a wrong guess is fixed in the dictionary, which is
// always asked first.
func ReadsAsWord(token string) bool {
	runes := []rune(strings.ToLower(token))
	vowels := 0
	for _, r := range runes {
		if isVowel(r) {
			vowels++
		}
	}
	switch {
	case vowels == 0:
		return false
	case len(runes) <= 2 || len(runes) > 6:
		return true
	}
	first, last := -1, -1
	for i, r := range runes {
		if isVowel(r) {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if !validOnset(string(runes[:first]), true) {
		return false
	}
	switch tail := string(runes[last+1:]); tail {
	case "", "r", "s", "l", "m", "n", "z", "x", "ns", "rs", "ls", "ps":
	default:
		return false
	}
	// A cluster between two vowels must split into the end of one syllable and
	// the start of the next.
	for i := first; i < last; {
		if isVowel(runes[i]) {
			i++
			continue
		}
		j := i
		for !isVowel(runes[j]) {
			j++
		}
		if !validMedial(string(runes[i:j])) {
			return false
		}
		i = j
	}
	return true
}

// validOnset: what a Portuguese word may start with before its first vowel.
func validOnset(c string, initial bool) bool {
	switch len([]rune(c)) {
	case 0, 1:
		return true
	case 2:
		switch c {
		case "ch", "lh", "nh", "qu", "gu", "ps", "pn", "gn", "tm":
			return true
		}
		return strings.ContainsRune("bcdfgpt", []rune(c)[0]) && strings.ContainsRune("lr", []rune(c)[1]) || (!initial && c == "rr") || (!initial && c == "ss")
	case 3:
		return !initial && (c == "str" || c == "scr" || c == "spr")
	}
	return false
}

// validMedial: a cluster between two vowels is the coda of one syllable and the
// onset of the next ("ns|p" in "transporte", "b|str" in "abstrato").
func validMedial(c string) bool {
	r := []rune(c)
	if len(r) == 1 {
		return true
	}
	for coda := 0; coda <= 2 && coda < len(r); coda++ {
		if coda == 2 {
			switch string(r[:2]) {
			case "ns", "rs", "ls", "bs", "ps":
			default:
				continue
			}
		}
		if rest := string(r[coda:]); len([]rune(rest)) > 0 && validOnset(rest, false) {
			return true
		}
	}
	return false
}

// romanLetters are the letters of the Roman numerals: a token made only of them
// ("XX", "IV", "CD") is more likely a numeral than an acronym, and is left alone.
func onlyRoman(token string) bool {
	return strings.Trim(token, "IVXLCDM") == ""
}

// reAcronym is a run of two or more capitals, with an optional plural "s".
var reAcronym = regexp.MustCompile(`[A-ZÀ-ÖØ-Þ]{2,}s?`)

// acronyms spells the tokens typed in capitals that the dictionary did not
// know and that are not said as a word: "RTSP" and "GPU" are spelled, "NASA" and
// "SIM" are left for the rest of the rules, which say a capital word as a word.
func acronyms(line string) string {
	return sub(reAcronym, line, func(m []int) (string, int, bool) {
		token := line[m[0]:m[1]]
		if !wordEdges(line, m[0], m[1]) {
			return "", 0, false
		}
		plural := false
		if strings.HasSuffix(token, "s") {
			plural = true
			token = token[:len(token)-1]
			if utf8.RuneCountInString(token) < 2 {
				return "", 0, false
			}
		}
		if onlyRoman(token) || ReadsAsWord(token) || afterANumber(line, m[0]) {
			return "", 0, false
		}
		out := SpellAcronym(token)
		if plural {
			out += "s"
		}
		return out, m[1], true
	})
}

// afterANumber reports whether the token at start follows a digit, with at most
// a space between: "80 KG", "5 GB", "24 FPS" are units, said by the unit rule.
func afterANumber(line string, start int) bool {
	before := strings.TrimRight(line[:start], " ")
	if before == "" {
		return false
	}
	return runeBefore(before, len(before)) >= '0' && runeBefore(before, len(before)) <= '9'
}

// ---- domains, e-mails and addresses ----

// tldNames say the parts of a domain that come after the last name: "com" is
// said as a word, "br" and "sh" are spelled.
var tldNames = map[string]string{
	"com": "com", "net": "nét", "org": "órg", "dev": "dév", "app": "áp", "fit": "fít", "info": "ínfo", "gov": "gov", "edu": "édu",
	"io": "i ôh", "sh": "ésse agá", "ai": "á i", "br": "bê érre", "is": "i ésse", "co": "cê ôh", "me": "ême êh", "tv": "tê vê",
	"pt": "pê tê", "us": "u ésse", "xyz": "xis ípsilon zê",
}

// reDomain finds an address that ends in a top-level domain the dictionary
// knows: a bare domain ("alpr.sh"), with a path ("exemplo.com/login"), with the
// scheme and "www" ("https://www.exemplo.com.br"). The top-level domain is in
// lower case, so a missing space after a full stop ("isso.Me") is not an address.
var reDomain = regexp.MustCompile(`(?:https?://)?(?:www\.)?(?:[\p{L}\d](?:[\p{L}\d-]*[\p{L}\d])?\.)+(?:com|net|org|dev|app|fit|info|gov|edu|io|sh|ai|br|is|co|me|tv|pt|us|xyz)(?:/[\p{L}\d._~%+/-]*)?(?:[?#]\S*)?`)

// addresses says the web addresses of a line. A domain, an e-mail's domain and a
// short path are said out ("alpr ponto sh barra login"); an address that cannot
// be said ("?id=7", numbers in the path, a long path) stays "o link", which the
// chat still shows.
func (n *Normalizer) addresses(line string) string {
	line = sub(reDomain, line, func(m []int) (string, int, bool) {
		if m[0] > 0 {
			if r := runeBefore(line, m[0]); isWordRune(r) || r == '@' || r == '.' || r == '/' || r == '-' {
				return "", 0, false
			}
		}
		raw := strings.TrimRight(line[m[0]:m[1]], ".,;:!?)")
		end := m[0] + len(raw)
		if end < len(line) && isWordRune(runeAt(line, end)) && runeAt(line, end) != '_' {
			return "", 0, false // "exemplo.comida": not a top-level domain
		}
		scheme := strings.HasPrefix(raw, "http")
		raw = strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
		host, path, _ := strings.Cut(raw, "/")
		hasPath := strings.Contains(raw, "/")
		if i := strings.IndexAny(host, "?#"); i >= 0 {
			host, path, hasPath = host[:i], host[i:], true
		}
		said, ok := n.domainWords(host)
		if !ok {
			return "", 0, false
		}
		if hasPath && strings.Trim(path, "/") != "" {
			spoken, ok := pathWords(path)
			if !ok {
				if scheme || strings.HasPrefix(host, "www.") {
					return linkWord(line[:m[0]]), end, true
				}
				return said + ", endereço completo na conversa", end, true
			}
			said += " barra " + spoken
		}
		return said, end, true
	})
	return replaceURLs(line)
}

// linkWord is what an address that cannot be said is replaced by.
func linkWord(before string) string {
	switch strings.ToLower(lastWord(before)) {
	case "link", "site", "endereço", "página", "pagina":
		return "mostrado na conversa"
	}
	return "o link"
}

// domainWords says a host name: "www" as "dáblio dáblio dáblio", each name as
// the dictionary has it or as a word, the top-level domain as the table has it,
// all joined by "ponto". The host is the whole dictionary term first ("ALPR.sh").
func (n *Normalizer) domainWords(host string) (string, bool) {
	host = strings.TrimSuffix(host, ".")
	www := false
	if strings.HasPrefix(strings.ToLower(host), "www.") {
		www, host = true, host[4:]
	}
	if host == "" {
		return "", false
	}
	prefix := ""
	if www {
		prefix = "dáblio dáblio dáblio ponto "
	}
	if said, ok := n.index[foldKey(host)]; ok {
		return prefix + said, true
	}
	labels := strings.Split(host, ".")
	tld := len(labels)
	for tld > 1 {
		if _, ok := tldNames[labels[tld-1]]; !ok {
			break
		}
		tld--
	}
	if tld == len(labels) { // the last label is not a top-level domain we know
		return "", false
	}
	if tld == 0 {
		tld = 1
	}
	parts := make([]string, 0, len(labels))
	for i, label := range labels {
		if i >= tld {
			parts = append(parts, tldNames[label])
			continue
		}
		parts = append(parts, n.labelWords(label))
	}
	return prefix + strings.Join(parts, " ponto "), true
}

// labelWords says one name of a domain: the dictionary's reading in any case, or
// the name in lower case with its hyphens said.
func (n *Normalizer) labelWords(label string) string {
	if said, ok := n.index[foldKey(label)]; ok {
		return said
	}
	if noVowel(label) {
		return SpellAcronym(label) // "ctrl.fit": no engine reads "ctrl" as a word
	}
	return strings.ReplaceAll(strings.ToLower(label), "-", " hífen ")
}

// noVowel reports whether a name of two or more letters has no vowel in it, so a
// voice has nothing to pronounce but the names of the letters.
func noVowel(label string) bool {
	n := 0
	for _, r := range label {
		if !unicode.IsLetter(r) || isVowel(r) || unicode.ToLower(r) == 'y' {
			return false
		}
		n++
	}
	return n >= 2
}

// pathWords says a path that is made of one or two plain words ("login",
// "minha-conta"): anything with digits, a long path or a strange character
// cannot be written down by the listener and is not said.
func pathWords(path string) (string, bool) {
	if strings.ContainsAny(path, "?#=&%+~") {
		return "", false
	}
	segments := strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
	if len(segments) == 0 || len(segments) > 2 || len(path) > 32 {
		return "", false
	}
	for i, s := range segments {
		for _, r := range s {
			if !unicode.IsLetter(r) && r != '-' && r != '_' && r != '.' {
				return "", false
			}
		}
		s = strings.ReplaceAll(strings.ToLower(s), "-", " ")
		s = strings.ReplaceAll(s, "_", " underline ")
		s = strings.ReplaceAll(s, ".", " ponto ")
		segments[i] = strings.Join(strings.Fields(s), " ")
	}
	return strings.Join(segments, " barra "), true
}

// emailWords says an e-mail address: the part before the arroba with its dots,
// underscores and hyphens said, and the domain as domainWords says it.
func (n *Normalizer) emailWords(m string) string {
	local, domain, _ := strings.Cut(m, "@")
	say := strings.NewReplacer(".", " ponto ", "_", " underline ", "-", " hífen ", "+", " mais ")
	said, ok := n.domainWords(domain)
	if !ok {
		said = say.Replace(domain)
		labels := strings.Split(domain, ".")
		if last := labels[len(labels)-1]; !ReadsAsWord(strings.ToUpper(last)) && utf8.RuneCountInString(last) <= 3 {
			said = say.Replace(strings.TrimSuffix(domain, last)) + SpellAcronym(last)
		}
	}
	return n.localWords(local, say) + " arroba " + said
}

// localWords says the part of an e-mail before the arroba: its dots, underscores,
// hyphens and pluses said, and each name between them said as the dictionary has
// it ("login@..." is "lóguin"), the dictionary of the application first.
func (n *Normalizer) localWords(local string, say *strings.Replacer) string {
	var out strings.Builder
	for _, part := range strings.FieldsFunc(local, func(r rune) bool { return strings.ContainsRune("._-+", r) }) {
		i := strings.Index(local, part)
		out.WriteString(say.Replace(local[:i]))
		if said, ok := n.index[foldKey(part)]; ok && utf8.RuneCountInString(part) >= 2 {
			out.WriteString(said)
		} else {
			out.WriteString(part)
		}
		local = local[i+len(part):]
	}
	out.WriteString(say.Replace(local))
	return out.String()
}

// ---- the units of data ----

// byteSaid are the units of data written the way a Brazilian says them: "byte" is
// "báite", and the "g" of "giga" is hard ("guigabáite"). Measured: the engines read
// "gigabyte" in a way that whisper wrote "gigabits" or "Girobytes", and this
// spelling came back right with both. It is the last rule of a line, so the unit
// of "10 GB" and the word "gigabytes" typed in a text are said the same way.
var byteSaid = strings.NewReplacer(
	"gigabytes", "guigabáites", "gigabyte", "guigabáite",
	"megabytes", "megabáites", "megabyte", "megabáite",
	"quilobytes", "quilobáites", "quilobyte", "quilobáite",
	"terabytes", "terabáites", "terabyte", "terabáite",
)

var reByte = regexp.MustCompile(`(?i)\b(?:giga|mega|quilo|tera)bytes?\b`)

// bytesWords says the units of data of a line as they are pronounced.
func bytesWords(line string) string {
	return reByte.ReplaceAllStringFunc(line, func(m string) string {
		return byteSaid.Replace(strings.ToLower(m))
	})
}

// pronouncedUnit is the word of a unit as the verbalizer says it: the validator
// accepts it beside the written one.
func pronouncedUnit(word string) string { return byteSaid.Replace(word) }
