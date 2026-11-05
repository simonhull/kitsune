package store

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

const (
	andWord = "AND"
	acdc    = "AC/DC"
	love    = "love"
	lovers  = "Crazy Lovers"

	acdcTyped  = "acdc"
	acdcSpaced = "ac dc"
	simon      = "Simon & Garfunkel"
	simonAnd   = "simon and garfunkel"
	tokyo      = "東京事変"
	crazyLove  = "Crazy Love"
	plainABC   = "abc"
)

// hostileQueries are the inputs that hurt a raw MATCH, and some that only look as if they should.
func hostileQueries() map[string]string {
	return map[string]string{
		"empty":            "",
		"5,000 tokens":     strings.Repeat("a ", 5000),
		"40 KB word":       strings.Repeat("a", 40_000),
		"NUL":              "foo\x00bar",
		"only NUL":         "\x00",
		"inner quote":      `foo"bar`,
		"lone quote":       `"`,
		"open quote":       `"foo`,
		"star":             "*",
		andWord:            "AND",
		"OR and NOT":       "foo OR bar NOT baz",
		"NEAR":             "NEAR(foo bar, 2)",
		"column filter":    "title:foo",
		"column set":       "{title artists}: foo",
		"caret":            "^foo",
		"parenthesis":      "(",
		"minus":            "-foo",
		"plus":             "foo + bar",
		"lone punctuation": "!?.,;:&",
		"bad UTF-8":        "foo\xff\xfebar",
		"only bad UTF-8":   "\xff\xfe",
		"newer letter":     "\U0001E900", // Adlam: a letter to Go, a separator to SQLite's older tables.
		"marks only":       "́̂",
		"zero width":       "\u200b",
		"long multibyte":   strings.Repeat("é", 300),
		"many quotes":      strings.Repeat(`"a`, 10_000),
	}
}

func TestParseSearchBounds(t *testing.T) {
	t.Parallel()
	for name, raw := range hostileQueries() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			q := ParseSearch(raw)
			if !utf8.ValidString(q.match) || strings.ContainsRune(q.match, 0) {
				t.Errorf("match %q is not clean text", q.match)
			}
			if n := strings.Count(q.match, `"*`); n > maxQueryTokens {
				t.Errorf("%d tokens, want at most %d", n, maxQueryTokens)
			}
			// Doubling can at most double the text; each token adds `"`, `"*` and a space.
			if limit := 2*maxQueryBytes + 4*maxQueryTokens; len(q.match) > limit {
				t.Errorf("match is %d bytes, want at most %d", len(q.match), limit)
			}
		})
	}
}

func TestParseSearchDropsWhatCannotMatch(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"", "   ", "\x00", `"`, "*", "(", "-", "!?.", "\xff", "́"} {
		if q := ParseSearch(raw); q.match != "" {
			t.Errorf("ParseSearch(%q).match = %q, want nothing", raw, q.match)
		}
	}
	tests := []struct{ raw, want string }{
		{`foo"bar`, `"foo""bar"*`},
		{"a * b", `"a"* "b"*`},
		{andWord, `"AND"*`},
		{"Wølf", `"Wolf"*`},
		{"é", "\"é\"*"},
		{"fo\x00o", `"foo"*`},
	}
	for _, tc := range tests {
		if got := ParseSearch(tc.raw).match; got != tc.want {
			t.Errorf("ParseSearch(%q).match = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestParseSearchCutsOnARune(t *testing.T) {
	t.Parallel()
	q := ParseSearch(strings.Repeat("a", maxQueryBytes-1) + "é")
	if !utf8.ValidString(q.match) || strings.ContainsRune(q.match, utf8.RuneError) {
		t.Errorf("match %q was cut inside a rune", q.match)
	}
}

// TestWordRunesAreSafe puts a spread of the runes Go calls letters or numbers
// through MATCH, whatever SQLite's older Unicode tables make of them.
func TestWordRunesAreSafe(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	writeAll(t, s, newWrite(1))
	const runeStride = 97 // Every rune passes too, but takes half a minute under the race detector.
	var batch strings.Builder
	words := 0
	flush := func() {
		if _, err := s.Search(t.Context(), ParseSearch(batch.String())); err != nil {
			t.Fatalf("Search(%q): %v", batch.String(), err)
		}
		batch.Reset()
		words = 0
	}
	for r := rune(1); r <= unicode.MaxRune; r += runeStride {
		if !isWordRune(r) {
			continue
		}
		batch.WriteRune(r)
		batch.WriteByte(' ')
		words++
		if words == maxQueryTokens {
			flush()
		}
	}
	flush()
}

func TestForms(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want []string
	}{
		{"Sigur Rós", []string{"sigur ros"}},
		{"MØ", []string{"mo"}},
		{simon, []string{simonAnd}},
		{acdc, []string{acdcSpaced, acdcTyped}},
		{"R.E.M.", []string{"r e m", "rem"}},
		{"Don't Stop", []string{"don t stop", "dont stop"}},
		{"  spaced   out ", []string{"spaced out"}},
		{"ＡＢＣ", []string{plainABC}},
		{"Сплин", []string{"сплин"}},
		{tokyo, []string{tokyo}},
		{"한글", []string{"한글"}},
		{"...", nil},
		{"", nil},
	}
	for _, tc := range tests {
		var got []string
		for _, f := range forms(tc.in) {
			got = append(got, f.text)
		}
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("forms(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCloseness(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, query string
		want        closeness
	}{
		{"Love", love, closeExact},
		{"Lové", "LOVE", closeExact},
		{acdc, acdcTyped, closeExact},
		{acdc, acdcSpaced, closeExact},
		{simon, simonAnd, closeExact},
		{"Love Song", love, closeLeadWords},
		{"Lovely", love, closeLead},
		{crazyLove, love, closeWholeWords},
		{"The Dark Side of the Moon", "moon dark", closeWholeWords},
		{lovers, love, closeWordStarts},
		{lovers, "love cra", closeWordStarts},
		{"Glove", love, closeNone},
		{crazyLove, "love sick", closeNone},
	}
	for _, tc := range tests {
		if got := closenessOf(forms(tc.name), forms(tc.query)); got != tc.want {
			t.Errorf("closenessOf(%q, %q) = %d, want %d", tc.name, tc.query, got, tc.want)
		}
	}
}
