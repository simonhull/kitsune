package store

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// searchFolder replaces the letters that the FTS tokenizer's diacritic removal
// leaves alone. The tokenizer handles combining marks; this handles the rest.
var searchFolder = strings.NewReplacer( //nolint:gochecknoglobals // Immutable after init; strings.Replacer is safe for concurrent use.
	"ø", "o", "Ø", "O",
	"ł", "l", "Ł", "L",
	"đ", "d", "Đ", "D", "ð", "d", "Ð", "D",
	"ħ", "h", "Ħ", "H",
	"ı", "i",
	"æ", "ae", "Æ", "AE",
	"œ", "oe", "Œ", "OE",
	"ß", "ss", "ẞ", "SS",
	"þ", "th", "Þ", "TH",
)

// foldSearch prepares text for the tracks_fts table, and search queries must
// fold the same way. It returns s unchanged when it holds none of the letters.
func foldSearch(s string) string { return searchFolder.Replace(s) }

// form is one way of writing a name or query for comparison: folded text and
// its words.
type form struct {
	text  string
	words []string
}

// forms folds s for comparison. Accents, case and the letters of searchFolder
// go, and "&" becomes "and". Punctuation gives two forms, because people type
// both: as a space ("ac dc"), and as nothing ("acdc"). A name with no letter or
// digit has no forms.
func forms(s string) []form {
	folded := strings.ToLower(foldSearch(norm.NFKD.String(strings.ReplaceAll(s, "&", " and "))))
	var out []form
	for _, punctuation := range []rune{' ', dropRune} {
		spelled := strings.Map(func(r rune) rune { return spell(r, punctuation) }, folded)
		words := strings.Fields(norm.NFC.String(spelled))
		text := strings.Join(words, " ")
		if text != "" && (len(out) == 0 || out[0].text != text) {
			out = append(out, form{text: text, words: words})
		}
	}
	return out
}

// dropRune tells [strings.Map] to drop a rune.
const dropRune = -1

// spell maps a folded rune to its comparison form, with punctuation as given.
func spell(r, punctuation rune) rune {
	switch {
	case unicode.Is(unicode.Mn, r):
		return dropRune
	case isWordRune(r):
		return r
	case unicode.IsSpace(r):
		return ' '
	}
	return punctuation
}

// isWordRune reports whether r can be part of a word.
func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }

// closeness says how well a name matches a query. Lower is closer.
type closeness int

const (
	closeExact      closeness = iota // The whole name.
	closeLeadWords                   // The name starts with the query, on a word boundary.
	closeLead                        // The name starts with the query.
	closeWholeWords                  // Every query word is a word of the name.
	closeWordStarts                  // Every query word starts a word of the name.
	closeTypo                        // The whole name, with typos.
	closeWordTypo                    // Every query word in the name, with typos.
	closeNone
)

// closenessOf returns how well the best of a name's forms matches the best of
// a query's.
func closenessOf(name, query []form) closeness {
	best := closeNone
	for i := range name {
		for j := range query {
			best = min(best, literal(&name[i], &query[j]))
		}
	}
	return best
}

// literal compares one form of a name with one of a query, without tolerance
// for typos.
func literal(name, query *form) closeness {
	rest, lead := strings.CutPrefix(name.text, query.text)
	switch {
	case lead && rest == "":
		return closeExact
	case lead && rest[0] == ' ':
		return closeLeadWords
	case lead:
		return closeLead
	}
	worst := closeWholeWords
	for _, q := range query.words {
		worst = max(worst, wordIn(name.words, q))
	}
	return worst
}

// wordIn says how well q matches any of a name's words.
func wordIn(words []string, q string) closeness {
	best := closeNone
	for _, w := range words {
		if w == q {
			return closeWholeWords
		}
		if strings.HasPrefix(w, q) {
			best = closeWordStarts
		}
	}
	return best
}
