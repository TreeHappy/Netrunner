package glyphs

import (
	"regexp"
	"strings"
)

// Map of netrunner-cards-json symbol tags to unicode glyphs.
var symbols = map[string]string{
	"[click]":              "◆",
	"[credit]":             "⬤",
	"[link]":               "⌁",
	"[mu]":                 "μ",
	"[recurring-credit]":   "⟳",
	"[subroutine]":         "⟐",
	"[trash]":              "✖",
	"[anarch]":             "ᴀ",
	"[criminal]":           "ᴄ",
	"[shaper]":             "ꜱ",
	"[jinteki]":            "ᴊ",
	"[haas-bioroid]":       "ʜ",
	"[nbn]":                "ɴ",
	"[weyland-consortium]": "ᴡ",
}

var tagRe = regexp.MustCompile(`\[(?:[a-z-]+)\]`)

// ReplaceSymbols converts [symbol] tags into unicode glyphs.
func ReplaceSymbols(s string) string {
	return tagRe.ReplaceAllStringFunc(s, func(m string) string {
		if g, ok := symbols[m]; ok {
			return g
		}
		return m
	})
}

var strongRe = regexp.MustCompile(`<strong>(.*?)</strong>`)
var errataRe = regexp.MustCompile(`</?errata>`)
var traceRe = regexp.MustCompile(`(\d+)\[trace\]`)

// StripMarkup removes HTML-ish markup tags after symbol substitution,
// uppercasing <strong> contents (card text uses it for keywords).
func CleanText(s string) string {
	s = ReplaceSymbols(s)
	s = strongRe.ReplaceAllStringFunc(s, func(m string) string {
		return strings.ToUpper(strongRe.FindStringSubmatch(m)[1])
	})
	s = errataRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "[trace]", "TRACE")
	s = strings.ReplaceAll(s, "\n", "\n")
	return s
}
