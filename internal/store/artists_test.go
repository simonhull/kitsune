package store_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

const (
	ann     = "Ann"
	bob     = "Bob"
	various = "Various Artists"
)

// artistWrites makes the writes for the given names.
func artistWrites(names ...string) []store.ArtistWrite {
	out := make([]store.ArtistWrite, len(names))
	for i, n := range names {
		key := strings.ToLower(n)
		out[i] = store.ArtistWrite{ULID: artistULID(key), MatchKey: key, Name: n, SortName: n}
	}
	return out
}

// artistAlbum describes one album: its own artists, and the artists of each track.
type artistAlbum struct {
	n            int
	year         int
	compilation  bool
	albumArtists []string
	tracks       [][]string
}

// seedArtistAlbums writes the albums and returns the store.
func seedArtistAlbums(t *testing.T, albums ...artistAlbum) *store.Store {
	t.Helper()
	s, _ := openBrowse(t)
	b := &store.Batch{}
	for _, a := range albums {
		for i, artists := range a.tracks {
			w := newWrite(a.n*100 + i)
			w.Album = store.AlbumWrite{
				GroupKey: fmt.Sprintf("g%d", a.n), ULID: albumID(a.n), Name: fmt.Sprintf("Album %d", a.n), SortName: fmt.Sprintf("Album %d", a.n),
				Artist: "display", Year: a.year, Compilation: a.compilation,
			}
			w.TrackNo = i + 1
			w.Artists, w.AlbumArtists = artistWrites(artists...), artistWrites(a.albumArtists...)
			b.Tracks = append(b.Tracks, w)
		}
	}
	if err := s.WriteBatch(t.Context(), b); err != nil {
		t.Fatal(err)
	}
	return s
}

func listed(t *testing.T, s *store.Store) map[string]int {
	t.Helper()
	artists, err := s.Artists(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]int, len(artists))
	for i := range artists {
		out[artists[i].Name] = artists[i].AlbumCount
	}
	return out
}

func TestArtistsListsOwners(t *testing.T) {
	t.Parallel()
	s := seedArtistAlbums(t,
		artistAlbum{n: 1, albumArtists: []string{ann}, tracks: [][]string{{ann}, {ann, "Guest"}}},
		artistAlbum{n: 2, tracks: [][]string{{bob, "Second"}, {bob}}},                           // No album artist.
		artistAlbum{n: 3, albumArtists: []string{"Conductor"}, tracks: [][]string{{"Soloist"}}}, // Not among its track artists.
		artistAlbum{n: 4, compilation: true, tracks: [][]string{{"Comp One"}, {"Comp Two"}}},    // Nobody's.
		artistAlbum{n: 5, albumArtists: []string{various}, tracks: [][]string{{"Unflagged"}}},   // A real row, but hidden.
		artistAlbum{n: 6, albumArtists: []string{ann}, tracks: [][]string{{ann}}},
	)
	want := map[string]int{ann: 2, bob: 1, "Conductor": 1}
	if got := listed(t, s); !reflect.DeepEqual(got, want) {
		t.Errorf("artists = %v, want %v", got, want)
	}
}

func TestArtistsOrder(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	b := &store.Batch{}
	for i, sortName := range []string{"zed", "Beta", "able"} {
		w := newWrite(i + 1)
		w.Artists = nil
		w.AlbumArtists = []store.ArtistWrite{{ULID: artistULID(sortName), MatchKey: sortName, Name: sortName, SortName: sortName}}
		b.Tracks = append(b.Tracks, w)
	}
	if err := s.WriteBatch(t.Context(), b); err != nil {
		t.Fatal(err)
	}
	artists, err := s.Artists(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(artists))
	for i := range artists {
		names[i] = artists[i].Name
	}
	if want := []string{"able", "Beta", "zed"}; !reflect.DeepEqual(names, want) {
		t.Errorf("order = %q, want %q", names, want)
	}
}

func TestArtistsOnlyLiveAlbums(t *testing.T) {
	t.Parallel()
	s := seedArtistAlbums(t, artistAlbum{n: 1, albumArtists: []string{ann}, tracks: [][]string{{ann}}})
	states, err := s.FileStates(t.Context(), testRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MarkMissing(t.Context(), []int64{states["a/100.flac"].TrackID}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := listed(t, s); len(got) != 0 {
		t.Errorf("artists = %v, want none live", got)
	}
	if _, err = s.Artist(t.Context(), artistULID("ann")); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Artist = %v, want ErrNotFound", err)
	}
}

// albumIDs returns the IDs of albums, in order.
func albumIDs(albums []store.AlbumSummary) []string {
	ids := make([]string, len(albums))
	for i := range albums {
		ids[i] = albums[i].ID
	}
	return ids
}

func TestArtistSections(t *testing.T) {
	t.Parallel()
	s := seedArtistAlbums(t,
		artistAlbum{n: 1, year: 2001, albumArtists: []string{ann}, tracks: [][]string{{ann}, {ann, bob}}},
		artistAlbum{n: 2, year: 2010, albumArtists: []string{ann}, tracks: [][]string{{ann}}},
		artistAlbum{n: 3, year: 2005, tracks: [][]string{{ann}}},
		artistAlbum{n: 4, albumArtists: []string{bob}, tracks: [][]string{{bob, ann}}},
		artistAlbum{n: 5, year: 2020, compilation: true, tracks: [][]string{{ann}, {bob}}},
		artistAlbum{n: 6, tracks: [][]string{{bob}}},
	)
	d, err := s.Artist(t.Context(), artistULID("ann"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != ann || d.AlbumCount != 3 {
		t.Errorf("artist = %+v", d.ArtistSummary)
	}
	if got, want := albumIDs(d.Albums), []string{albumID(2), albumID(3), albumID(1)}; !reflect.DeepEqual(got, want) {
		t.Errorf("albums = %q, want %q, newest first", got, want)
	}
	if got, want := albumIDs(d.AppearsOn), []string{albumID(5), albumID(4)}; !reflect.DeepEqual(got, want) {
		t.Errorf("appears on = %q, want %q, unknown years last", got, want)
	}
	if d.Albums[0].TrackCount != 1 {
		t.Errorf("track count = %d, want 1", d.Albums[0].TrackCount)
	}
}

func TestArtistGuestOwnsNothing(t *testing.T) {
	t.Parallel()
	s := seedArtistAlbums(t, artistAlbum{n: 1, albumArtists: []string{ann}, tracks: [][]string{{ann, bob}}})
	d, err := s.Artist(t.Context(), artistULID("bob"))
	if err != nil {
		t.Fatal(err)
	}
	if d.AlbumCount != 0 || len(d.Albums) != 0 || len(d.AppearsOn) != 1 {
		t.Errorf("guest = %+v", d)
	}
}

func TestArtistHidden(t *testing.T) {
	t.Parallel()
	s := seedArtistAlbums(t, artistAlbum{n: 1, albumArtists: []string{various}, tracks: [][]string{{ann}}})
	if _, err := s.Artist(t.Context(), artistULID("various artists")); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Various Artists = %v, want ErrNotFound", err)
	}
	if _, err := s.Artist(t.Context(), "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown = %v, want ErrNotFound", err)
	}
}
