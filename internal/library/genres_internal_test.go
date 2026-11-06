package library

import (
	"reflect"
	"slices"
	"testing"

	"github.com/simonhull/kitsune/internal/store"
)

// Genres the tests below share.
const (
	genreSpiritual = "Spiritual"
	popAndRock     = "Pop, Rock"
)

func TestSplitGenres(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, value string
		want        []string
	}{
		{"one", rock, []string{rock}},
		{"comma", popAndRock, []string{pop, rock}},
		{"no space after the comma", "Pop,Rock", []string{pop, rock}},
		{"padding", "  Rock  ", []string{rock}},
		{"blanks", " , Rock,, ", []string{rock}},
		{"only blanks", " , ", []string{}},
		{"empty", "", []string{}},
		{"ID3v1 number", "(12)Spiritual", []string{genreSpiritual}},
		{"ID3v1 number on every part", "(12)Spiritual, (17) Rock", []string{genreSpiritual, rock}},
		{"only a number", "(12)", []string{}},
		{"words in brackets stay", "(Alt)Rock", []string{"(Alt)Rock"}},
		{"a number later stays", "Rock (1970)", []string{"Rock (1970)"}},
		{"an open bracket stays", "(12", []string{"(12"}},
		{"empty brackets stay", "()Rock", []string{"()Rock"}},
		{"an ampersand is a name", "Drum & Bass", []string{"Drum & Bass"}},
		{"a slash is a name", "Rock/Pop", []string{"Rock/Pop"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := splitGenres(tt.value); !slices.Equal(got, tt.want) {
				t.Errorf("splitGenres(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestDisplayGenres(t *testing.T) {
	t.Parallel()
	got := displayGenres([]store.AlbumTagCount{
		{AlbumID: "a", Value: "rock", Tracks: 1},
		{AlbumID: "a", Value: popAndRock, Tracks: 3},
		{AlbumID: "b", Value: rock, Tracks: 2},
		{AlbumID: "b", Value: "(12)Électronique", Tracks: 2},
		{AlbumID: "b", Value: "Electronic", Tracks: 2},
		{AlbumID: "c", Value: " , ", Tracks: 9},
	})
	want := map[string][]string{
		// One spelling across the library, the most common; most tracks first; then by key.
		"a": {rock, pop},
		// Synonyms stay apart.
		"b": {"Electronic", rock, "Électronique"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("displayGenres = %q, want %q", got, want)
	}
}

func TestDisplayGenresTieIsStable(t *testing.T) {
	t.Parallel()
	counts := []store.AlbumTagCount{{AlbumID: "a", Value: "rock", Tracks: 1}, {AlbumID: "a", Value: "ROCK", Tracks: 1}}
	for range 20 {
		if got := displayGenres(counts)["a"]; !slices.Equal(got, []string{"ROCK"}) {
			t.Fatalf("got %q", got)
		}
	}
}

func TestGenresBeforeARefreshKnowNothing(t *testing.T) {
	t.Parallel()
	if got := NewGenres(newTestLib(t, 1).st).Of("any"); got != nil {
		t.Errorf("Of = %q, want nil", got)
	}
}

func TestEditsAndScansRefreshGenres(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	want := []string{pop, rock, genreSpiritual}
	edit := &Edit{Genres: []string{popAndRock, "(12)Spiritual"}}
	if err := l.scanner.EditAlbum(t.Context(), l.album.ID, l.album.ETag, edit); err != nil {
		t.Fatal(err)
	}
	if got := l.scanner.genres.Of(l.album.ID); !slices.Equal(got, want) {
		t.Errorf("after an edit: %q, want %q", got, want)
	}
	l.opts.Genres = NewGenres(l.st)
	l.scanner = NewScanner(l.st, l.opts)
	l.scan(Quick)
	if got := l.opts.Genres.Of(l.album.ID); !slices.Equal(got, want) {
		t.Errorf("after a scan: %q, want %q", got, want)
	}
}
