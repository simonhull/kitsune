package store

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"
)

const (
	beatles   = "The Beatles"
	radiohead = "Radiohead"
	revival   = "Beatles Revival"
	kitten    = "kitten"
)

// indexOf builds an index whose IDs are the names.
func indexOf(names ...string) nameIndex {
	var ix nameIndex
	for _, n := range names {
		ix = ix.with(n, n)
	}
	return ix
}

func TestNameMatch(t *testing.T) {
	t.Parallel()
	ix := indexOf(radiohead, beatles, revival, acdc, simon, "Sigur Rós", "Сплин", tokyo, "R.E.M.", "Bea", "...")
	tests := []struct {
		query string
		want  []string
	}{
		{"radiohead", []string{radiohead}},
		{"RADIOHEAD", []string{radiohead}},
		{"radio", []string{radiohead}},
		{"radiohed", []string{radiohead}},       // A letter short.
		{"raidohead", []string{radiohead}},      // Swapped neighbors.
		{"beatles", []string{revival, beatles}}, // Starts with it, then has the word.
		{"the baetles", []string{beatles}},      // A typo in the whole name.
		{"baetles", []string{beatles, revival}}, // A typo in one word alone: the shorter name first.
		{acdcTyped, []string{acdc}},
		{acdcSpaced, []string{acdc}},
		{"ac/dc", []string{acdc}},
		{"rem", []string{"R.E.M."}},
		{simonAnd, []string{simon}},
		{"simon & garfunkel", []string{simon}},
		{"garfunkel simon", []string{simon}},
		{"sigur ros", []string{"Sigur Rós"}},
		{"сплин", []string{"Сплин"}},
		{"東京", []string{tokyo}},
		{"zzzzzzzz", []string{}},
		{"xy", []string{}}, // Too short to forgive anything.
	}
	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			t.Parallel()
			if got := ix.match(forms(tc.query)); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("match = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNameMatchOrder(t *testing.T) {
	t.Parallel()
	ix := indexOf("Crazy Lovers", crazyLove, "Lovely", "Love Song", "Love", "Lover")
	want := []string{"Love", "Love Song", "Lover", "Lovely", crazyLove, "Crazy Lovers"}
	if got := ix.match(forms(love)); !reflect.DeepEqual(got, want) {
		t.Errorf("match = %q, want %q", got, want)
	}
}

func TestNameMatchLimit(t *testing.T) {
	t.Parallel()
	var ix nameIndex
	for i := range 30 {
		ix = ix.with(strconv.Itoa(i), fmt.Sprintf("Name %02d", i))
	}
	if got := ix.match(forms("name")); len(got) != maxNameHits {
		t.Errorf("%d hits, want %d", len(got), maxNameHits)
	}
}

func TestEditDistance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		a, b  string
		limit int
		want  int
		ok    bool
	}{
		{kitten, kitten, 2, 0, true},
		{kitten, "sitten", 2, 1, true},
		{kitten, "kittn", 2, 1, true},
		{kitten, "kitteen", 2, 1, true},
		{kitten, "iktten", 2, 1, true},
		{kitten, "sitting", 3, 3, true},
		{kitten, "sitting", 2, 0, false},
		{kitten, "kit", 2, 0, false},
		{plainABC, plainABC, 0, 0, false}, // No allowance means no typo search.
		{"日本語", "日本話", 1, 1, true},
	}
	var rows editRows
	for _, tc := range tests {
		got, ok := rows.within([]rune(tc.a), []rune(tc.b), tc.limit)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("within(%q, %q, %d) = %d, %t; want %d, %t", tc.a, tc.b, tc.limit, got, ok, tc.want, tc.ok)
		}
	}
}

func TestIsVarious(t *testing.T) {
	t.Parallel()
	mbid, other := variousArtistsMBID, "other"
	tests := []struct {
		key  string
		mbid *string
		want bool
	}{
		{variousArtistsKey, nil, true},
		{"va by another name", &mbid, true},
		{"someone", &other, false},
		{"someone", nil, false},
	}
	for _, tc := range tests {
		if got := isVarious(tc.key, tc.mbid); got != tc.want {
			t.Errorf("isVarious(%q, %v) = %t", tc.key, tc.mbid, got)
		}
	}
}

// BenchmarkNameMatch measures a typo, the slowest kind of query, over a list the
// size of a real library's.
func BenchmarkNameMatch(b *testing.B) {
	var ix nameIndex
	for i := range 2000 {
		ix = ix.with(strconv.Itoa(i), fmt.Sprintf("Artist Number %d of the Band", i))
	}
	query := forms("artsit nmuber")
	for b.Loop() {
		ix.match(query)
	}
}
