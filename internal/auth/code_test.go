package auth_test

import (
	"regexp"
	"testing"

	"github.com/simonhull/kitsune/internal/auth"
)

// codeShape is what every code looks like.
var codeShape = regexp.MustCompile(`^[0-9a-hjkmnp-tv-z]{4}(-[0-9a-hjkmnp-tv-z]{4}){3}$`)

func TestNewCode(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for range 100 {
		code := auth.NewCode()
		if !codeShape.MatchString(code) {
			t.Fatalf("code %q has the wrong shape", code)
		}
		if seen[code] {
			t.Fatalf("code %q came up twice", code)
		}
		seen[code] = true
	}
}

func TestNormalizeCode(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"k7m2-q9xd-4tbn-r8ve": "k7m2q9xd4tbnr8ve",
		"K7M2 Q9XD 4TBN R8VE": "k7m2q9xd4tbnr8ve",
		"oIl0":                "0110",
	} {
		if got := auth.NormalizeCode(in); got != want {
			t.Errorf("NormalizeCode(%q) = %q, want %q", in, got, want)
		}
	}
}
