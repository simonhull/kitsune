package store

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/internal/smartlist"
)

// punctuation is a value made of what SQL reads: quotes, a statement end, a
// comment, parentheses, LIKE's wildcards, a parameter and a backslash.
const punctuation = `'"; DROP TABLE tracks; -- )(%_?\`

// everyRule returns rules with one rule of every field and operator, text and
// pick values from text, numbers from n.
func everyRule(t *testing.T, text string, n int64) smartlist.Rules {
	t.Helper()
	fields := []smartlist.Field{
		smartlist.Title, smartlist.Album, smartlist.Artist, smartlist.AlbumArtist, smartlist.Genre,
		smartlist.Year, smartlist.Rating, smartlist.Plays, smartlist.Skips, smartlist.LastPlayed,
		smartlist.Added, smartlist.Length, smartlist.Format, smartlist.Bitrate, smartlist.Playlist,
	}
	var rules []string
	for _, f := range fields {
		kind, _ := smartlist.KindOf(f)
		for _, op := range smartlist.Ops(f) {
			var value string
			switch {
			case f == smartlist.Playlist:
				value = fmt.Sprintf("%q", (text + strings.Repeat("x", 26))[:26])
			case kind == smartlist.KindText || kind == smartlist.KindPick:
				value = fmt.Sprintf("%q", text)
			default:
				value = strconv.FormatInt(min(n, 10), 10)
			}
			rules = append(rules, fmt.Sprintf(`{"field":%q,"op":%q,"value":%s}`, f, op, value))
		}
	}
	in := `{"match":"any","rules":[` + strings.Join(rules[:40], ",") +
		`,{"match":"all","rules":[` + strings.Join(rules[40:], ",") + `]}]}`
	parsed, err := smartlist.Parse([]byte(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return parsed
}

// compileEvery compiles [everyRule]'s rules of text and n in e.
func compileEvery(t *testing.T, e *evaluator, text string, n int64) *statement {
	t.Helper()
	rules := everyRule(t, text, n)
	st, err := e.compile(t.Context(), &rules, false, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// punctuationGenre stands in for the library's genres: the album with the ULID
// album has the genre [punctuation], and no other album has one.
func punctuationGenre(album string) GenreAlbums {
	return func(keep func(string) bool) []string {
		if keep(FoldText(punctuation)) {
			return []string{album}
		}
		return nil
	}
}

// No request text ever reaches the statement: rules whose every value is SQL
// punctuation compile to the same text as rules of plain values, and the
// punctuation travels only as a bound value.
func TestNoRuleValueReachesTheStatement(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	w := newWrite(1)
	if err := s.WriteBatch(t.Context(), &Batch{Tracks: []TrackWrite{w}}); err != nil {
		t.Fatal(err)
	}
	e, err := s.newEvaluator(t.Context(), 1, punctuationGenre(w.Album.ULID))
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	plain, hostile := compileEvery(t, e, "plain", 1), compileEvery(t, e, punctuation, 9)
	if plain.text.String() != hostile.text.String() {
		t.Fatalf("the statement changed with its values:\n%s\n---\n%s", plain.text.String(), hostile.text.String())
	}
	for _, piece := range []string{"DROP", "--", "plain", "%_"} {
		if strings.Contains(hostile.text.String(), piece) {
			t.Errorf("the statement holds %q", piece)
		}
	}
	if !slices.Contains(hostile.args, any(punctuation)) {
		t.Error("the punctuation was never bound")
	}
	// And the statement runs, the punctuation compared as a value: the song is
	// found, since among the any rules its genre is the punctuation.
	songs, err := e.run(t.Context(), hostile, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(songs) != 1 {
		t.Errorf("found %d songs, want the one", len(songs))
	}
}
