package matching

import (
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// fold returns s for comparing names that may differ in case, accents,
// punctuation and spacing: "Björk & Friends!" and "bjork and friends" fold the
// same. It is only ever compared, never shown or written.
func fold(s string) string {
	s = strings.ReplaceAll(norm.NFKD.String(s), "&", " and ")
	var b strings.Builder
	space := true
	for _, r := range cases.Fold().String(s) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			space = false
		case !space:
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

// similarity returns how alike two names are, from 0 to 1, once folded: by
// their edit distance, or by one holding every word of the other, which is how
// "Glasshouse" and "Glasshouse (Deluxe Edition)" are near.
func similarity(a, b string) float64 {
	a, b = fold(a), fold(b)
	if a == b {
		return 1
	}
	if a == "" || b == "" {
		return 0
	}
	ra, rb := []rune(a), []rune(b)
	edit := 1 - float64(distance(ra, rb))/float64(max(len(ra), len(rb)))
	return max(edit, containment*contains(strings.Fields(a), strings.Fields(b)))
}

// containment is what one name holding every word of another counts for.
const containment = 0.85

// contains returns 1 when every word of the shorter list is in the longer, else 0.
func contains(a, b []string) float64 {
	if len(a) > len(b) {
		a, b = b, a
	}
	words := make(map[string]bool, len(b))
	for _, w := range b {
		words[w] = true
	}
	for _, w := range a {
		if !words[w] {
			return 0
		}
	}
	return 1
}

// distance is the Levenshtein distance between a and b.
func distance(a, b []rune) int {
	row := make([]int, len(b)+1)
	for j := range row {
		row[j] = j
	}
	for i := range a {
		prev := row[0]
		row[0] = i + 1
		for j := range b {
			cost := 1
			if a[i] == b[j] {
				cost = 0
			}
			prev, row[j+1] = row[j+1], min(row[j+1]+1, row[j]+1, prev+cost)
		}
	}
	return row[len(b)]
}
