package piper

import (
	"regexp"
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
// acronyms, documents and borrowed words that are said the same way in every
// product, and no word that belongs to one business. A domain adds its own
// through Speech.Lexicon, and an entry of the domain wins over one of this list
// that has the same Written. It returns a copy, so a caller may append to it.
//
// Words espeak-ng already says well ("Nubank", "Itaú", "Serasa") are not here.
func BaseLexicon() []LexiconEntry {
	return append([]LexiconEntry(nil), baseLexicon...)
}

var baseLexicon = []LexiconEntry{
	// Documents and the State.
	{"CPF", "cê pê efe", false},
	{"CNPJ", "cê ene pê jota", false},
	{"RG", "érre gê", false},
	{"CNH", "cê ene agá", false},
	{"CEP", "cép", false},
	{"IRPF", "i érre pê efe", false},
	{"IRPJ", "i érre pê jota", false},
	{"INSS", "i ene ésse ésse", false},
	{"FGTS", "éfe gê tê ésse", false},
	{"CLT", "cê éle tê", false},
	{"MEI", "mêi", false},
	{"SUS", "sus", false},
	{"IPTU", "i pê tê u", false},
	{"IPVA", "i pê vê á", false},
	{"LGPD", "éle gê pê dê", false},
	{"PIX", "pics", true},
	{"PJ", "pê jota", false},
	{"PF", "pê efe", false},
	{"EUA", "Estados Unidos", false},
	{"ONG", "ôngue", false},
	{"UTI", "u tê i", false},
	// Health and training.
	{"IMC", "i ême cê", false},
	{"TMB", "tê ême bê", false},
	{"RPE", "érre pê é", false},
	{"RIR", "érre i érre", false},
	// Technology.
	{"PDF", "pê dê efe", false},
	{"API", "á pê i", false},
	{"URL", "u érre éle", false},
	{"SMS", "ésse ême ésse", false},
	{"USB", "u ésse bê", false},
	{"GPS", "gê pê ésse", false},
	{"CPU", "cê pê u", false},
	{"TV", "tê vê", false},
	{"OK", "ôquei", false},
	{"WhatsApp", "uótsap", true},
	{"Wi-Fi", "uái fai", true},
}

type lexiconRule struct {
	re   *regexp.Regexp
	said string
}

// compileLexicon builds the rules for entries in the order they are given.
// Entries that are empty or repeat an earlier Written are dropped, so the
// first one wins.
func compileLexicon(entries []LexiconEntry) []lexiconRule {
	seen := map[string]bool{}
	var rules []lexiconRule
	for _, e := range entries {
		key := e.Written
		if e.AnyCase {
			key = strings.ToLower(key)
		}
		if e.Written == "" || seen[key] {
			continue
		}
		seen[key] = true
		pattern := regexp.QuoteMeta(e.Written)
		if e.AnyCase {
			pattern = "(?i)" + pattern
		}
		rules = append(rules, lexiconRule{regexp.MustCompile(pattern), e.Said})
	}
	return rules
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
}

// unitPattern lists the units longest first. Those of more than one letter match
// in any case ("KM", "Kg"); the single letters h, s, m, g, l and L only as
// written.
const unitPattern = `(?i:kg/m²|km/h|kcal|[°º][cf]|bpm|rpm|reps|rep|kg|km|cm|mm|ml|mg|min|seg|cal|m/s|m²|m³)|[hsmglL]|%|°`

// lookupUnit finds a unit as it was written.
func lookupUnit(written string) (unit, bool) {
	key := strings.ToLower(strings.ReplaceAll(written, "º", "°"))
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
