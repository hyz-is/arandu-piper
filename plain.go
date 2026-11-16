package piper

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	// A line that is only a rule: "---", "***", "___".
	rulePattern = regexp.MustCompile(`^[-*_=~]{3,}$`)
	// The marker that opens a heading, a quote, a bullet or a numbered item.
	headingPattern = regexp.MustCompile(`^#{1,6}\s+`)
	quotePattern   = regexp.MustCompile(`^>+\s*`)
	bulletPattern  = regexp.MustCompile(`^[-*+•·]\s+`)
	numberPattern  = regexp.MustCompile(`^\d{1,3}[.)]\s+`)
	// [text](address) keeps the text.
	linkPattern = regexp.MustCompile(`\[([^\]\n]+)\]\([^)\s]+\)`)
	// A web address, without the punctuation that ends the sentence around it.
	urlPattern = regexp.MustCompile(`(?i)\b(?:https?://|www\.)[^\s<>()]*[^\s<>().,;:!?]`)
	// White space before a punctuation mark, left behind by a removed emoji.
	spaceBeforePunctuation = regexp.MustCompile(`\s+([.,;:!?])`)
)

// Plain turns a chat message into text that is fit to be read aloud: it
// removes the formatting of WhatsApp and of Markdown, which a voice would
// otherwise spell out ("asterisco, ferrugem, asterisco"), and what a voice
// cannot say.
//
// It removes *bold*, _italic_, ~strikethrough~ and `code` markers (and the
// **double** forms), the markers of headings, quotes, bullets and numbered
// lists, web addresses, and emojis, and it keeps the words of a [link](address).
// Every line ends in a punctuation mark, so that lines that were separate
// items or paragraphs are separate sentences, with a pause between them, once
// they are joined. An arrow becomes a comma. Everything else, including
// accents and numbers, is kept: it does not spell out numbers, currency or
// abbreviations, which the voice reads as it knows how.
//
// The result is one line. It is empty when nothing speakable is left.
func Plain(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	var lines []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || rulePattern.MatchString(line) {
			continue
		}
		line = headingPattern.ReplaceAllString(line, "")
		line = quotePattern.ReplaceAllString(line, "")
		line = bulletPattern.ReplaceAllString(line, "")
		line = numberPattern.ReplaceAllString(line, "")
		line = plainLine(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}

	for i := 0; i < len(lines)-1; i++ {
		lines[i] = endSentence(lines[i])
	}
	out := strings.Join(lines, " ")
	out = spaceBeforePunctuation.ReplaceAllString(out, "$1")
	out = strings.Join(strings.Fields(out), " ")
	if !speakable(out) {
		return ""
	}
	return out
}

// plainLine removes the inline formatting of one line.
func plainLine(line string) string {
	line = linkPattern.ReplaceAllString(line, "$1")
	line = urlPattern.ReplaceAllString(line, "")
	line = strings.ReplaceAll(line, "`", "")
	for _, double := range []string{"**", "__", "~~"} {
		for strings.Contains(line, double) {
			line = strings.ReplaceAll(line, double, double[:1])
		}
	}
	for _, marker := range []rune{'*', '_', '~'} {
		line = unwrap(line, marker)
	}
	// What is left of a marker is not formatting, and a voice would spell it
	// out: it becomes a space.
	line = strings.NewReplacer("*", " ", "~", " ", "_", " ").Replace(line)

	var b strings.Builder
	for _, r := range line {
		switch {
		case isEmoji(r):
		case r >= 0x2190 && r <= 0x21FF:
			b.WriteString(", ")
		default:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// unwrap removes a pair of formatting markers around some text: the opening
// one follows the start of the line, a space or an opening punctuation mark,
// and the closing one precedes the end, a space or a punctuation mark, as
// WhatsApp reads them. A marker inside a word is left alone.
func unwrap(line string, marker rune) string {
	runes := []rune(line)
	drop := make([]bool, len(runes))
	for i := 0; i < len(runes); i++ {
		if runes[i] != marker || !opens(runes, i, marker) {
			continue
		}
		for k := i + 2; k < len(runes); k++ {
			if runes[k] == marker && closes(runes, k, marker) {
				drop[i], drop[k] = true, true
				i = k
				break
			}
		}
	}
	var b strings.Builder
	for i, r := range runes {
		if !drop[i] {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func opens(runes []rune, i int, marker rune) bool {
	if i+1 >= len(runes) || unicode.IsSpace(runes[i+1]) {
		return false
	}
	if i == 0 {
		return true
	}
	prev := runes[i-1]
	return unicode.IsSpace(prev) || strings.ContainsRune(`([{"'“‘-—–/*_~`, prev)
}

func closes(runes []rune, k int, marker rune) bool {
	if unicode.IsSpace(runes[k-1]) {
		return false
	}
	if k+1 == len(runes) {
		return true
	}
	next := runes[k+1]
	return unicode.IsSpace(next) || strings.ContainsRune(`.,;:!?)]}"'”’…-—–/*_~`, next)
}

// endSentence adds a full stop to a line that does not end in punctuation.
func endSentence(line string) string {
	trimmed := strings.TrimRight(line, `"')]}”’»`)
	if trimmed == "" {
		return line
	}
	last := []rune(trimmed)[len([]rune(trimmed))-1]
	if strings.ContainsRune(".!?…:;,", last) {
		return line
	}
	return line + "."
}

// isEmoji reports whether r is an emoji or one of the invisible characters
// that build one (variation selectors, joiners, tags, keycaps).
func isEmoji(r rune) bool {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF: // pictographs, emoticons, flags, skin tones
	case r >= 0x2600 && r <= 0x27BF: // miscellaneous symbols, dingbats
	case r >= 0x2300 && r <= 0x23FF: // watches, hourglasses, media controls
	case r >= 0x2B00 && r <= 0x2BFF: // stars, large squares and circles
	case r >= 0xE0020 && r <= 0xE007F: // tags of regional flags
	case r == 0xFE0F || r == 0xFE0E || r == 0x200D || r == 0x20E3:
	case r == 0x203C || r == 0x2049 || r == 0x2122 || r == 0x2139:
	case r >= 0x2934 && r <= 0x2935, r == 0x3030, r == 0x303D, r == 0x3297, r == 0x3299:
	default:
		return false
	}
	return true
}
