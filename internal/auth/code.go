package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"slices"
	"strings"
)

const (
	// codeBytes is a one-time code's randomness: 80 bits.
	codeBytes = 10
	// codeGroup is how many characters a code shows between dashes.
	codeGroup = 4
)

// crockford is Crockford's base32 in lower case: no i, l, o or u, so a code
// reads aloud and copies by hand without confusion.
var crockford = base32.NewEncoding("0123456789abcdefghjkmnpqrstvwxyz").WithPadding(base32.NoPadding) //nolint:gochecknoglobals // A fixed encoding, never written.

// NewCode returns a fresh one-time code, such as the setup code or a reset
// code: 80 random bits as four groups of four lower-case Crockford base32
// characters, such as k7m2-q9xd-4tbn-r8ve.
func NewCode() string {
	b := make([]byte, codeBytes)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error; it crashes the program instead.
	groups := make([]string, 0, codeGroup)
	for g := range slices.Chunk([]byte(crockford.EncodeToString(b)), codeGroup) {
		groups = append(groups, string(g))
	}
	return strings.Join(groups, "-")
}

// NormalizeCode lower-cases code and drops dashes and spaces, so it may be
// typed in any case and grouping. As Crockford's base32 decodes, o reads as 0
// and i and l as 1: a code never holds those letters, so nothing else can match.
func NormalizeCode(code string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '-', ' ':
			return -1
		case 'o':
			return '0'
		case 'i', 'l':
			return '1'
		}
		return r
	}, strings.ToLower(code))
}

// codeMatches reports whether given, normalized, is want, a normalized code,
// in constant time for codes of want's length. The length is not a secret:
// every code has the same one.
func codeMatches(given string, want []byte) bool {
	return want != nil && subtle.ConstantTimeCompare([]byte(NormalizeCode(given)), want) == 1
}
