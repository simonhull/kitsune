package store_test

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
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
	writeArtistAlbums(t, s, albums...)
	return s
}

// writeArtistAlbums writes the albums in one batch.
func writeArtistAlbums(t *testing.T, s *store.Store, albums ...artistAlbum) {
	t.Helper()
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
}

func listed(t *testing.T, s *store.Store) map[string]int {
	t.Helper()
	artists, err := s.Artists(t.Context(), 0)
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

func TestArtistsSayWhenTheyArrivedAndHowOftenTheyPlayed(t *testing.T) {
	t.Parallel()
	s, clock := openBrowse(t)
	first := clock.Now()
	writeArtistAlbums(t, s,
		artistAlbum{n: 1, albumArtists: []string{ann}, tracks: [][]string{{ann}, {ann, bob}}},
		artistAlbum{n: 2, tracks: [][]string{{bob}}},
	)
	clock.Advance(time.Hour)
	writeArtistAlbums(t, s, artistAlbum{n: 3, albumArtists: []string{ann}, tracks: [][]string{{ann}}})
	listener, err := s.CreateUser(t.Context(), store.NewUser{Name: "listener", PasswordHash: "h", Role: store.RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateUser(t.Context(), store.NewUser{Name: "someone else", PasswordHash: "h", Role: store.RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	// Bob is a guest on Ann's second song: its plays are hers, since she owns the album.
	for i, play := range []struct {
		user  int64
		track int
	}{{listener.ID, 100}, {listener.ID, 101}, {listener.ID, 101}, {listener.ID, 200}, {other.ID, 300}} {
		recordErr := s.RecordPlay(t.Context(), &store.NewPlay{
			UserID: play.user, TrackID: fmt.Sprintf("01J%023d", play.track), PlayedAt: first, Source: "test", ClientID: strconv.Itoa(i),
		})
		if recordErr != nil {
			t.Fatal(recordErr)
		}
	}
	artists, err := s.Artists(t.Context(), listener.ID)
	if err != nil {
		t.Fatal(err)
	}
	type fact struct {
		added time.Time
		plays int
	}
	got := map[string]fact{}
	for i := range artists {
		got[artists[i].Name] = fact{artists[i].AddedAt, artists[i].Plays}
	}
	want := map[string]fact{ann: {first.Add(time.Hour), 3}, bob: {first, 1}}
	if !reflect.DeepEqual(got, want) {
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
	artists, err := s.Artists(t.Context(), 0)
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
	if got, want := albumIDs(d.AppearsOn), []string{albumID(4), albumID(5)}; !reflect.DeepEqual(got, want) {
		t.Errorf("appears on = %q, want %q, compilations last, even the newest", got, want)
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

func TestAlbumLinks(t *testing.T) {
	t.Parallel()
	link := func(names ...string) []store.ArtistLink {
		out := make([]store.ArtistLink, len(names))
		for i, n := range names {
			out[i] = store.ArtistLink{ID: artistULID(strings.ToLower(n)), Name: n}
		}
		return out
	}
	tests := []struct {
		name  string
		album artistAlbum
		want  []store.ArtistLink
	}{
		{"album artist", artistAlbum{albumArtists: []string{ann}, tracks: [][]string{{ann}}}, link(ann)},
		{"album artists in tag order, one link each", artistAlbum{albumArtists: []string{bob, ann}, tracks: [][]string{{ann}, {ann, bob}}}, link(bob, ann)},
		{"album artist not among the track artists", artistAlbum{albumArtists: []string{ann}, tracks: [][]string{{bob}}}, link(ann)},
		{"no album artist", artistAlbum{tracks: [][]string{{ann, bob}, {ann}}}, link(ann)},
		{"compilation with no album artist", artistAlbum{compilation: true, tracks: [][]string{{ann}, {bob}}}, link()},
		{"compilation with an album artist", artistAlbum{compilation: true, albumArtists: []string{ann}, tracks: [][]string{{bob}}}, link(ann)},
		{"Various Artists, flagged", artistAlbum{compilation: true, albumArtists: []string{various}, tracks: [][]string{{ann}}}, link()},
		{"Various Artists, not flagged", artistAlbum{albumArtists: []string{various}, tracks: [][]string{{ann}}}, link()},
		{"no artists at all", artistAlbum{tracks: [][]string{{}}}, link()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.album.n = 1
			s := seedArtistAlbums(t, tc.album)
			d, err := s.Album(t.Context(), albumID(1))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(d.Artists, tc.want) {
				t.Errorf("artists = %+v, want %+v", d.Artists, tc.want)
			}
		})
	}
}

func TestAlbumLinkSurvivesAMissingTrack(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	tagged, plain := newWrite(1), newWrite(2)
	plain.Album = tagged.Album
	tagged.Artists, tagged.AlbumArtists = artistWrites(bob), artistWrites(ann)
	plain.Artists = artistWrites(bob)
	if err := s.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{tagged, plain}}); err != nil {
		t.Fatal(err)
	}
	states, err := s.FileStates(t.Context(), testRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MarkMissing(t.Context(), []int64{states[tagged.RelPath].TrackID}, time.Now()); err != nil {
		t.Fatal(err)
	}
	d, err := s.Album(t.Context(), tagged.Album.ULID)
	if err != nil {
		t.Fatal(err)
	}
	// The missing track's album artist no longer counts, so the fallback takes over.
	if len(d.Artists) != 1 || d.Artists[0].Name != bob {
		t.Errorf("artists = %+v, want only the live track's artist", d.Artists)
	}
}

// sortAlbum returns a one-track album with the given owners.
func sortAlbum(n int, compilation bool, artists, albumArtists []store.ArtistWrite) store.TrackWrite {
	w := albumTrack(n, 1, fmt.Sprintf("album %d", n))
	w.Album.Artist = fmt.Sprintf("Shown %d", n)
	w.Album.Compilation = compilation
	w.Artists, w.AlbumArtists = artists, albumArtists
	return w
}

func TestAlbumsSayWhoTheySortUnder(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	beatles := store.ArtistWrite{ULID: "03J00000000000000000000001", MatchKey: "the beatles", Name: "The Beatles", SortName: "Beatles, The"}
	cash := store.ArtistWrite{ULID: "03J00000000000000000000002", MatchKey: "johnny cash", Name: "Johnny Cash", SortName: "Cash, Johnny"}
	guest := store.ArtistWrite{ULID: "03J00000000000000000000003", MatchKey: "someone", Name: "Someone", SortName: "Someone"}
	writeAll(t, s,
		sortAlbum(1, false, []store.ArtistWrite{guest}, []store.ArtistWrite{beatles, cash}),
		sortAlbum(2, false, []store.ArtistWrite{cash, guest}, nil),
		sortAlbum(3, true, []store.ArtistWrite{guest}, nil),
	)
	page, err := s.Albums(t.Context(), store.SortName, store.MaxAlbumLimit, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Beatles, The", "Cash, Johnny", "Shown 3"}
	for i := range page.Albums {
		if got := page.Albums[i].SortArtist; got != want[i] {
			t.Errorf("album %d sorts under %q, want %q", i, got, want[i])
		}
	}
}
