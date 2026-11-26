package piper

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// LexiconEntry is one word and how it is said. A voice engine takes the
// spelling through espeak-ng, which turns it into phonemes before the model
// sees anything, and neither Piper nor Kokoro takes SSML or a pronunciation
// dictionary, so a word the engine gets wrong is fixed in the writing: the
// spoken text spells it the way a Brazilian says it ("WhatsApp" is "uótsap",
// "CPF" is "cê pê efe"). The lexicon is applied only to the spoken text, never
// to what is written in the chat.
type LexiconEntry struct {
	// Written is the word as it appears in the text. It matches as a whole
	// word: "ME" is not found inside "MEI" or "ÔME".
	Written string
	// Said is what replaces it.
	Said string
	// AnyCase makes Written match in any case. Acronyms leave it false, so
	// "ME" (micro-empresa) is not "me" (a pronoun).
	AnyCase bool
}

// BaseLexicon is the lexicon the package ships for Brazilian Portuguese: the
// central dictionary of lexicon/ptbr.json, with its acronyms (spelled with the
// names of the letters), documents, borrowed words and brands that are said the
// same way in every product, and no word that belongs to one business. A domain
// adds its own through Speech.Lexicon, and an entry of the domain wins over one
// of this list that has the same Written. The entries come longest term first.
// It returns a copy, so a caller may append to it.
//
// Words espeak-ng already says well ("Nubank", "Itaú", "Serasa") are not here.
func BaseLexicon() []LexiconEntry {
	return append([]LexiconEntry(nil), baseLexicon...)
}

type lexiconRule struct {
	re   *regexp.Regexp
	said string
}

// compileLexicon builds the rules for entries. Entries that are empty or repeat
// an earlier Written are dropped, so the first one wins (the application's
// before the base dictionary's); the rules then run longest term first, so
// "ALPR.sh" is found before "ALPR" and "Wi-Fi" before "Wi". An acronym that is
// spelled letter by letter also matches its plural: "APIs" is "á pê ís".
func compileLexicon(entries []LexiconEntry) []lexiconRule {
	seen := map[string]bool{}
	var kept []LexiconEntry
	for _, e := range entries {
		key := e.Written
		if e.AnyCase {
			key = strings.ToLower(key)
		}
		if e.Written == "" || seen[key] {
			continue
		}
		seen[key] = true
		kept = append(kept, e)
	}
	for _, e := range kept {
		if !e.AnyCase && isAcronymWritten(e.Written) && saidAsLetters(e.Said) && !seen[e.Written+"s"] {
			seen[e.Written+"s"] = true
			kept = append(kept, LexiconEntry{Written: e.Written + "s", Said: e.Said + "s"})
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		return utf8.RuneCountInString(kept[i].Written) > utf8.RuneCountInString(kept[j].Written)
	})
	rules := make([]lexiconRule, 0, len(kept))
	for _, e := range kept {
		pattern := regexp.QuoteMeta(e.Written)
		if e.AnyCase {
			pattern = "(?i)" + pattern
		}
		rules = append(rules, lexiconRule{regexp.MustCompile(pattern), e.Said})
	}
	return rules
}

// foldKey is a term as the index holds it: in lower case and without accents,
// because an address is written without them ("corujao.ai") and the lexicon of
// the application has the word as it is spelled ("Corujão").
func foldKey(s string) string {
	return strings.Map(func(r rune) rune { return foldAccent(unicode.ToLower(r)) }, s)
}

// lexiconIndex maps a term, in lower case and without accents, to how it is said,
// the first entry winning. Domain names and the names in an e-mail look a label
// up in it in any case ("alpr" in "alpr.sh" is "ALPR", "corujao" in
// "ajuda@corujao.ai" is "Corujão").
func lexiconIndex(entries []LexiconEntry) map[string]string {
	index := map[string]string{}
	for _, e := range entries {
		key := foldKey(e.Written)
		if _, ok := index[key]; e.Written != "" && !ok {
			index[key] = e.Said
		}
	}
	return index
}

// isAcronymWritten reports whether a term is two or more capitals and nothing else.
func isAcronymWritten(written string) bool {
	n := 0
	for _, r := range written {
		if !unicode.IsUpper(r) {
			return false
		}
		n++
	}
	return n >= 2
}

// saidAsLetters reports whether a pronunciation is only names of letters.
func saidAsLetters(said string) bool {
	words := strings.Fields(said)
	if len(words) < 2 {
		return false
	}
	for _, w := range words {
		if !letterNameSet[w] {
			return false
		}
	}
	return true
}

// apply replaces every whole-word match of the rule. Go's \b knows only ASCII
// letters, so the edges are checked here with Unicode letters.
func (r lexiconRule) apply(text string) string {
	var b strings.Builder
	last := 0
	for _, m := range r.re.FindAllStringIndex(text, -1) {
		if m[0] < last || !wordEdges(text, m[0], m[1]) {
			continue
		}
		b.WriteString(text[last:m[0]])
		b.WriteString(r.said)
		last = m[1]
	}
	if last == 0 {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}

// wordEdges reports whether text[start:end] is not glued to a letter, a digit
// or an underscore on either side.
func wordEdges(text string, start, end int) bool {
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(text[:start])
		if isWordRune(r) {
			return false
		}
	}
	if end < len(text) {
		r, _ := utf8.DecodeRuneInString(text[end:])
		if isWordRune(r) {
			return false
		}
	}
	return true
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// unit is how a unit of measure is said, singular and plural, and its gender.
type unit struct {
	one, many string
	fem       bool
}

// units are the units of measure said after a number. "kcal" is said
// "calorias": it is what a Brazilian says about food and training, and what a
// nutrition label means by "Caloria" with a capital C; an application that wants
// "quilocalorias" puts "kcal" in its Speech.Lexicon, which is applied first.
var units = map[string]unit{
	"kg":    {"quilo", "quilos", false},
	"g":     {"grama", "gramas", false},
	"mg":    {"miligrama", "miligramas", false},
	"km":    {"quilômetro", "quilômetros", false},
	"m":     {"metro", "metros", false},
	"cm":    {"centímetro", "centímetros", false},
	"mm":    {"milímetro", "milímetros", false},
	"ml":    {"mililitro", "mililitros", false},
	"l":     {"litro", "litros", false},
	"kcal":  {"caloria", "calorias", true},
	"cal":   {"caloria", "calorias", true},
	"km/h":  {"quilômetro por hora", "quilômetros por hora", false},
	"m/s":   {"metro por segundo", "metros por segundo", false},
	"kg/m²": {"quilo por metro quadrado", "quilos por metro quadrado", false},
	"m²":    {"metro quadrado", "metros quadrados", false},
	"m³":    {"metro cúbico", "metros cúbicos", false},
	"bpm":   {"batimento por minuto", "batimentos por minuto", false},
	"rpm":   {"rotação por minuto", "rotações por minuto", true},
	"min":   {"minuto", "minutos", false},
	"h":     {"hora", "horas", true},
	"s":     {"segundo", "segundos", false},
	"seg":   {"segundo", "segundos", false},
	"rep":   {"repetição", "repetições", true},
	"reps":  {"repetição", "repetições", true},
	"%":     {"por cento", "por cento", false},
	"°":     {"grau", "graus", false},
	"°c":    {"grau Celsius", "graus Celsius", false},
	"°f":    {"grau Fahrenheit", "graus Fahrenheit", false},
	"gb":    {"gigabyte", "gigabytes", false},
	"mb":    {"megabyte", "megabytes", false},
	"kb":    {"quilobyte", "quilobytes", false},
	"tb":    {"terabyte", "terabytes", false},
	"gbps":  {"gigabit por segundo", "gigabits por segundo", false},
	"mbps":  {"megabit por segundo", "megabits por segundo", false},
	"fps":   {"quadro por segundo", "quadros por segundo", false},
	"hz":    {"hertz", "hertz", false},
	"khz":   {"quilohertz", "quilohertz", false},
	"mhz":   {"megahertz", "megahertz", false},
	"ghz":   {"gigahertz", "gigahertz", false},
	"kw":    {"quilowatt", "quilowatts", false},
	"kwh":   {"quilowatt-hora", "quilowatts-hora", false},
	"mah":   {"miliampère-hora", "miliampères-hora", false},
	"V":     {"volt", "volts", false},
	"W":     {"watt", "watts", false},
}

// unitPattern lists the units longest first. Those of more than one letter match
// in any case ("KM", "Kg", "GB"); the single letters h, s, m, g, l, L, V and W
// only as written.
const unitPattern = `(?i:kg/m²|km/h|kcal|[°º][cf]|bpm|rpm|reps|rep|kg|km|cm|mm|ml|mg|min|seg|cal|m/s|m²|m³|gbps|mbps|kwh|khz|mhz|ghz|mah|fps|gb|mb|kb|tb|kw|hz)|[hsmglLVW]|%|°`

// lookupUnit finds a unit as it was written.
func lookupUnit(written string) (unit, bool) {
	key := strings.ToLower(strings.ReplaceAll(written, "º", "°"))
	if written == "V" || written == "W" {
		return units[written], true
	}
	if len([]rune(written)) == 1 && written != "L" && written != strings.ToLower(written) {
		return unit{}, false
	}
	u, ok := units[key]
	return u, ok
}

// feminineNouns are the nouns, in the singular and the plural, after which one
// and two are "uma" and "duas" and the hundreds end in "as".
var feminineNouns = map[string]bool{}

func init() {
	for _, n := range []string{
		"parcela", "parcelas", "vez", "vezes", "hora", "horas", "semana", "semanas", "conta", "contas", "despesa", "despesas",
		"receita", "receitas", "dívida", "dívidas", "fatura", "faturas", "compra", "compras", "categoria", "categorias",
		"pergunta", "perguntas", "mensagem", "mensagens", "foto", "fotos", "reserva", "reservas", "meta", "metas",
		"entrada", "entradas", "saída", "saídas", "nota", "notas", "transferência", "transferências", "pessoa", "pessoas",
		"série", "séries", "repetição", "repetições", "caloria", "calorias", "quilocaloria", "quilocalorias",
		"refeição", "refeições", "porção", "porções", "dose", "doses", "sessão", "sessões", "aula", "aulas",
		"flexão", "flexões", "barra", "barras", "carga", "cargas", "volta", "voltas", "rodada", "rodadas",
		"caminhada", "caminhadas", "corrida", "corridas", "caixa", "caixas", "unidade", "unidades", "fase", "fases",
		"semente", "sementes", "xícara", "xícaras", "colher", "colheres", "fruta", "frutas", "garrafa", "garrafas",
	} {
		feminineNouns[n] = true
	}
}
