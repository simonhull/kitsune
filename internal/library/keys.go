package library

import (
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// MatchKey returns the identity key for a name: it trims the name, collapses
// every run of white space to one space, normalizes to NFC and folds case,
// then normalizes to NFC again because folding can leave text that is not NFC.
// It strips no punctuation and splits nothing, so "A feat. B" is one key.
// Every identity key in the library goes through it.
func MatchKey(s string) string {
	s = strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
	s = norm.NFC.String(s)
	// A cases.Caser is not safe for concurrent use, so each call makes its own.
	s = cases.Fold().String(s)
	return norm.NFC.String(s)
}
