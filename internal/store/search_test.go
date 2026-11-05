package store_test

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

const (
	fire       = "Fire"
	horizons   = "Hørizøns"
	secondName = "Second"
)

// searchTrack is a track that a search test seeds.
type searchTrack struct {
	n             int
	title, artist string
}

// seedSearch writes tracks, each on its own album, and returns the store.
func seedSearch(t *testing.T, tracks ...searchTrack) *store.Store {
	t.Helper()
	s, _ := openBrowse(t)
	b := &store.Batch{}
	for _, tr := range tracks {
		w := newWrite(tr.n)
		w.Title = tr.title
		if tr.artist != "" {
			w.Artists[0].Name = tr.artist
		}
		b.Tracks = append(b.Tracks, w)
	}
	if err := s.WriteBatch(t.Context(), b); err != nil {
		t.Fatal(err)
	}
	return s
}

// searchTitles returns the titles of the tracks a query finds.
func searchTitles(t *testing.T, s *store.Store, raw string) []string {
	t.Helper()
	found, err := s.Search(t.Context(), store.ParseSearch(raw))
	if err != nil {
		t.Fatalf("Search(%q): %v", raw, err)
	}
	titles := make([]string, len(found.Tracks))
	for i := range found.Tracks {
		titles[i] = found.Tracks[i].Title
	}
	return titles
}

func TestSearchTracksOrder(t *testing.T) {
	t.Parallel()
	s := seedSearch(t,
		searchTrack{n: 1, title: "Song", artist: "Love Band"},
		searchTrack{n: 2, title: "Crazy Lovers"},
		searchTrack{n: 3, title: "Crazy Love"},
		searchTrack{n: 4, title: "Lovely"},
		searchTrack{n: 5, title: "Love Song"},
		searchTrack{n: 6, title: "Love"},
	)
	want := []string{"Love", "Love Song", "Lovely", "Crazy Love", "Crazy Lovers", "Song"}
	if got := searchTitles(t, s, "love"); !reflect.DeepEqual(got, want) {
		t.Errorf("love = %q, want %q", got, want)
	}
}

func TestSearchTracksMatching(t *testing.T) {
	t.Parallel()
	s := seedSearch(t,
		searchTrack{n: 1, title: horizons, artist: "Zayde Wølf"},
		searchTrack{n: 2, title: "Café del Mar"},
		searchTrack{n: 3, title: fire},
	)
	tests := []struct{ query, want string }{
		{"horizons", horizons},
		{"HØRIZ", horizons},
		{"wolf", horizons},
		{"cafe", "Café del Mar"},
		{"fi", fire},
	}
	for _, tc := range tests {
		if got := searchTitles(t, s, tc.query); !reflect.DeepEqual(got, []string{tc.want}) {
			t.Errorf("%s = %q, want %q", tc.query, got, tc.want)
		}
	}
}

func TestSearchTrackHit(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	w := newWrite(1)
	w.Title, w.Duration = fire, 90*time.Second
	w.Artists = append(w.Artists, store.ArtistWrite{ULID: artistULID("second"), MatchKey: "second", Name: secondName, SortName: secondName})
	if err := s.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{w}}); err != nil {
		t.Fatal(err)
	}
	found, err := s.Search(t.Context(), store.ParseSearch("fire"))
	if err != nil {
		t.Fatal(err)
	}
	want := []store.TrackHit{{
		ID: w.ULID, Title: fire, Artists: []string{w.Artists[0].Name, secondName},
		AlbumID: w.Album.ULID, Album: w.Album.Name, Duration: 90 * time.Second,
	}}
	if !reflect.DeepEqual(found.Tracks, want) {
		t.Errorf("tracks = %+v, want %+v", found.Tracks, want)
	}
}

func TestSearchSkipsMissingTracks(t *testing.T) {
	t.Parallel()
	s := seedSearch(t, searchTrack{n: 1, title: fire}, searchTrack{n: 2, title: "Fire Two"})
	states, err := s.FileStates(t.Context(), testRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MarkMissing(t.Context(), []int64{states["a/1.flac"].TrackID}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := searchTitles(t, s, "fire"); !reflect.DeepEqual(got, []string{"Fire Two"}) {
		t.Errorf("fire = %q, want only the live track", got)
	}
}

func TestSearchCapsTracks(t *testing.T) {
	t.Parallel()
	tracks := make([]searchTrack, 30)
	for i := range tracks {
		tracks[i] = searchTrack{n: i + 1, title: fire}
	}
	s := seedSearch(t, tracks...)
	if got := len(searchTitles(t, s, "fire")); got != 20 {
		t.Errorf("%d tracks, want the cap of 20", got)
	}
}

func TestSearchFindsNothingInNothing(t *testing.T) {
	t.Parallel()
	s := seedSearch(t, searchTrack{n: 1, title: fire})
	for _, raw := range []string{"", "   ", `"`, "*", "!?"} {
		found, err := s.Search(t.Context(), store.ParseSearch(raw))
		if err != nil || len(found.Tracks) != 0 {
			t.Errorf("Search(%q) = %+v, %v; want nothing", raw, found, err)
		}
	}
	found, err := s.Search(t.Context(), store.SearchQuery{})
	if err != nil || len(found.Tracks) != 0 {
		t.Errorf("zero query = %+v, %v; want nothing", found, err)
	}
}

func TestSearchHostileInputKeepsThePool(t *testing.T) {
	t.Parallel()
	const readers = 2
	s, err := store.Open(t.Context(), t.TempDir()+"/kitsune.db", store.Options{Readers: readers})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	w := newWrite(1)
	w.Title = fire
	if err = s.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{w}}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, raw := range []string{strings.Repeat("a ", 5000), strings.Repeat("a", 40_000), "\x00", `"`, "*", "AND", `foo"bar`, "!"} {
		wg.Go(func() {
			if _, searchErr := s.Search(t.Context(), store.ParseSearch(raw)); searchErr != nil {
				t.Errorf("Search(%.20q): %v", raw, searchErr)
			}
		})
	}
	wg.Wait()
	if got := searchTitles(t, s, "fire"); !reflect.DeepEqual(got, []string{fire}) {
		t.Errorf("after the attack: %q", got)
	}
}

func TestSearchCanceled(t *testing.T) {
	t.Parallel()
	s := seedSearch(t, searchTrack{n: 1, title: fire})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Search(ctx, store.ParseSearch("fire")); err == nil {
		t.Error("a canceled search returned no error")
	}
}

// seedNames writes one track per artist and album name and refreshes the index.
func seedNames(t *testing.T, s *store.Store, names ...string) {
	t.Helper()
	b := &store.Batch{}
	for i, name := range names {
		w := newWrite(i + 1)
		w.Title = "Track"
		w.Album.Name = name + " Album"
		w.Artists[0].Name, w.Artists[0].SortName = name, name
		w.Artists[0].MatchKey = strings.ToLower(name)
		b.Tracks = append(b.Tracks, w)
	}
	if err := s.WriteBatch(t.Context(), b); err != nil {
		t.Fatal(err)
	}
	if err := s.RefreshNames(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestSearchFindsArtistsAndAlbums(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	seedNames(t, s, "Radiohead", "Sigur Rós")
	found, err := s.Search(t.Context(), store.ParseSearch("radiohed"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []store.ArtistHit{{ID: artistULID("artist-1"), Name: "Radiohead"}}; !reflect.DeepEqual(found.Artists, want) {
		t.Errorf("artists = %+v, want %+v", found.Artists, want)
	}
	if len(found.Albums) != 1 || found.Albums[0].Title != "Radiohead Album" || found.Albums[0].TrackCount != 1 {
		t.Errorf("albums = %+v, want the one album with its track count", found.Albums)
	}
}

func TestSearchHidesVariousArtists(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	seedNames(t, s, "Various Artists", "Various Moods")
	found, err := s.Search(t.Context(), store.ParseSearch("various"))
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Artists) != 1 || found.Artists[0].Name != "Various Moods" {
		t.Errorf("artists = %+v, want only the real one", found.Artists)
	}
}

func TestSearchNamesWaitForRefresh(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	w := newWrite(1)
	if err := s.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{w}}); err != nil {
		t.Fatal(err)
	}
	q := store.ParseSearch(w.Artists[0].Name)
	found, err := s.Search(t.Context(), q)
	if err != nil || len(found.Artists) != 0 {
		t.Fatalf("before a refresh: %+v, %v; want no artists yet", found.Artists, err)
	}
	if err = s.RefreshNames(t.Context()); err != nil {
		t.Fatal(err)
	}
	if found, err = s.Search(t.Context(), q); err != nil || len(found.Artists) != 1 {
		t.Errorf("after a refresh: %+v, %v; want the artist", found.Artists, err)
	}
}

func TestSearchNamesSurviveReopen(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), store.FileName)
	s, err := store.Open(t.Context(), path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	w := newWrite(1)
	if err = s.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{w}}); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if s, err = store.Open(t.Context(), path, store.Options{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	found, err := s.Search(t.Context(), store.ParseSearch(w.Artists[0].Name))
	if err != nil || len(found.Artists) != 1 {
		t.Errorf("after reopening: %+v, %v; want the artist", found.Artists, err)
	}
}

func TestSearchDropsGoneNames(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	seedNames(t, s, "Radiohead")
	states, err := s.FileStates(t.Context(), testRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MarkMissing(t.Context(), []int64{states["a/1.flac"].TrackID}, time.Now()); err != nil {
		t.Fatal(err)
	}
	q := store.ParseSearch("radiohead")
	found, err := s.Search(t.Context(), q)
	if err != nil || len(found.Albums) != 0 {
		t.Errorf("before a refresh: %+v, %v; want a dead album left out", found.Albums, err)
	}
	if err = s.RefreshNames(t.Context()); err != nil {
		t.Fatal(err)
	}
	if found, err = s.Search(t.Context(), q); err != nil || len(found.Artists) != 0 {
		t.Errorf("after a refresh: %+v, %v; want the artist gone", found.Artists, err)
	}
}

// TestSearchDuringRefresh searches while the index is rebuilt over and over, and
// every search must find what was there before and after.
func TestSearchDuringRefresh(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	seedNames(t, s, "Radiohead")
	ctx, stop := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	wg.Go(func() {
		for ctx.Err() == nil {
			if err := s.RefreshNames(ctx); err != nil && ctx.Err() == nil {
				t.Errorf("refresh: %v", err)
			}
		}
	})
	q := store.ParseSearch("radiohead")
	for range 200 {
		found, err := s.Search(t.Context(), q)
		if err != nil || len(found.Artists) != 1 {
			t.Errorf("during a refresh: %+v, %v", found.Artists, err)
			break
		}
	}
	stop()
	wg.Wait()
}
