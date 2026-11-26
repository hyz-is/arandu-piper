package piper

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// lexiconPTBR is the central pronunciation dictionary of the HYZIS products
// for Brazilian Portuguese, versioned with this module and shared by every
// application that speaks through it. Its format is the one an application may
// use for its own words (ParseLexicon):
//
//	{"language": "pt-BR", "version": 1, "dictionary": {"ALPR.sh": "á éle pê érre ponto ésse agá"}}
//
//go:embed lexicon/ptbr.json
var lexiconPTBR []byte

// baseLexicon and lexiconVersion are read once from the embedded dictionary. A
// test holds that it parses, so a panic here cannot reach an application.
var baseLexicon, lexiconVersion = mustBaseLexicon()

func mustBaseLexicon() ([]LexiconEntry, int) {
	entries, version, err := parseLexicon(lexiconPTBR)
	if err != nil {
		panic("piper: the embedded lexicon is invalid: " + err.Error())
	}
	return entries, version
}

// LexiconVersion is the version of the dictionary this module ships
// (lexicon/ptbr.json). It moves whenever an entry is added, changed or removed,
// and it is part of Normalizer.Version.
func LexiconVersion() int { return lexiconVersion }

// LexiconJSON is the shared dictionary as a JSON document: the same file the
// module embeds. A program that is not written in Go, or a service that fills
// the system prompt of a model, reads the dictionary from it, so every product
// speaks a name the same way. The caller may keep and change the bytes.
func LexiconJSON() []byte { return append([]byte(nil), lexiconPTBR...) }

// ParseLexicon reads a dictionary in the format of the shared one: a JSON
// document with "language": "pt-BR", an optional "version" and a "dictionary"
// that maps a written term to how it is said. A value is a string, or an object
// {"said": "...", "case": "any" | "exact"}.
//
// The case rule of a string value: the term matches in any case, except one
// written entirely in capitals (an acronym), which matches only as written, so
// "ME" is not the pronoun "me". Use the object form to say otherwise. The
// entries come back in the order that matters when they are applied, the
// longest term first ("ALPR.sh" before "ALPR"), so a caller passes them to
// Speech.Lexicon as they are.
func ParseLexicon(data []byte) ([]LexiconEntry, error) {
	entries, _, err := parseLexicon(data)
	return entries, err
}

func parseLexicon(data []byte) ([]LexiconEntry, int, error) {
	var doc struct {
		Language   string                     `json:"language"`
		Version    int                        `json:"version"`
		Dictionary map[string]json.RawMessage `json:"dictionary"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return nil, 0, fmt.Errorf("lexicon: %w", err)
	}
	if !strings.EqualFold(doc.Language, "pt-BR") {
		return nil, 0, fmt.Errorf("lexicon: language is %q, want \"pt-BR\"", doc.Language)
	}
	entries := make([]LexiconEntry, 0, len(doc.Dictionary))
	for written, raw := range doc.Dictionary {
		if strings.TrimSpace(written) == "" {
			return nil, 0, errors.New("lexicon: an empty term")
		}
		entry := LexiconEntry{Written: written, AnyCase: defaultAnyCase(written)}
		var said string
		if err := json.Unmarshal(raw, &said); err == nil {
			entry.Said = said
		} else {
			var obj struct {
				Said string `json:"said"`
				Case string `json:"case"`
			}
			objDec := json.NewDecoder(bytes.NewReader(raw))
			objDec.DisallowUnknownFields()
			if err := objDec.Decode(&obj); err != nil {
				return nil, 0, fmt.Errorf("lexicon: %q: %w", written, err)
			}
			entry.Said = obj.Said
			switch obj.Case {
			case "", "default":
			case "any":
				entry.AnyCase = true
			case "exact":
				entry.AnyCase = false
			default:
				return nil, 0, fmt.Errorf("lexicon: %q: case is %q, want \"any\" or \"exact\"", written, obj.Case)
			}
		}
		if strings.TrimSpace(entry.Said) == "" {
			return nil, 0, fmt.Errorf("lexicon: %q is said as nothing", written)
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		li, lj := utf8.RuneCountInString(entries[i].Written), utf8.RuneCountInString(entries[j].Written)
		if li != lj {
			return li > lj
		}
		return entries[i].Written < entries[j].Written
	})
	return entries, doc.Version, nil
}

// defaultAnyCase is the case rule of a term given as a plain string: any case,
// unless the term is written entirely in capitals.
func defaultAnyCase(written string) bool {
	hasLetter := false
	for _, r := range written {
		if unicode.IsLower(r) {
			return true
		}
		if unicode.IsUpper(r) {
			hasLetter = true
		}
	}
	return !hasLetter
}
