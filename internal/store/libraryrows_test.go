package store_test

import (
	"slices"
	"sync"
	"testing"

	"github.com/simonhull/kitsune/internal/store"
)

// indexAlbumTitles reads the index and returns its albums' titles, in order.
func indexAlbumTitles(t *testing.T, s *store.Store) []string {
	t.Helper()
	index, err := s.LibraryIndex(t.Context(), 0, &store.IndexOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, a := range index.Artists {
		for j := range a.Albums {
			titles = append(titles, a.Albums[j].Title)
		}
	}
	return titles
}

func TestIndexFollowsEveryLibraryWrite(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	a := newWrite(0)
	writeAll(t, s, a)
	want := func(titles ...string) {
		t.Helper()
		if got := indexAlbumTitles(t, s); !slices.Equal(got, titles) {
			t.Errorf("index albums = %v, want %v", got, titles)
		}
	}
	want("Album 0")
	want("Album 0") // From what was kept.
	b := newWrite(1)
	writeAll(t, s, b)
	want("Album 0", "Album 1")
	renamed := newWrite(0)
	renamed.ID = idOf(t, s, &a)
	renamed.Album.Name = "Album Renamed"
	writeAll(t, s, renamed)
	want("Album Renamed", "Album 1") // Under Artist 0, then Artist 1.
	if err := s.MarkMissing(t.Context(), []int64{idOf(t, s, &b)}, fixedNow()); err != nil {
		t.Fatal(err)
	}
	want("Album Renamed")
}

// indexTrack reads the index with opts and returns its one track.
func indexTrack(t *testing.T, s *store.Store, opts *store.IndexOptions) store.IndexTrack {
	t.Helper()
	index, err := s.LibraryIndex(t.Context(), 0, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Artists) != 1 || len(index.Artists[0].Albums) != 1 || len(index.Artists[0].Albums[0].Tracks) != 1 {
		t.Fatalf("index = %+v, want one track", index)
	}
	return index.Artists[0].Albums[0].Tracks[0]
}

// What the song columns read of the library is kept too, per question, and
// follows a library write.
func TestIndexTracksFollowLibraryWrites(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	w := newWrite(0)
	w.Year = 1999
	rock, jazz := store.Tag{Key: id3Genre, Value: genreRock}, store.Tag{Key: id3Genre, Value: "Jazz Fusion"}
	ann := store.Tag{Key: "composer", Value: "Ann"}
	w.Tags = []store.Tag{rock, ann}
	writeAll(t, s, w)
	genre := &store.IndexOptions{Details: true, TagKeys: []string{id3Genre}}
	composer := &store.IndexOptions{TagKeys: []string{ann.Key}}
	check := func(opts *store.IndexOptions, title string, year int, tags ...store.Tag) {
		t.Helper()
		got := indexTrack(t, s, opts)
		if got.Title != title || got.Extra == nil || got.Extra.Year != year || !slices.Equal(got.Extra.Tags, tags) {
			t.Errorf("track = %s, %+v; want %s, year %d, tags %v", got.Title, got.Extra, title, year, tags)
		}
	}
	check(genre, "Title 0", 1999, rock)
	check(genre, "Title 0", 1999, rock) // From what was kept.
	check(composer, "Title 0", 0, ann)
	w.ID = idOf(t, s, &w)
	w.Title, w.Year = "Retitled", 2001
	w.Tags = []store.Tag{jazz}
	writeAll(t, s, w)
	check(genre, "Retitled", 2001, jazz)
	check(composer, "Retitled", 0)
}

// An album's discs come from the kept track rows, so a disc's new name shows in
// the next index.
func TestIndexDiscsFollowLibraryWrites(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	one, two := newWrite(0), newWrite(1)
	two.Album = one.Album
	one.DiscNo, one.DiscSubtitle = 1, "Side A"
	two.DiscNo, two.DiscSubtitle = 2, "Side B"
	writeAll(t, s, one, two)
	names := func() []string {
		t.Helper()
		index, err := s.LibraryIndex(t.Context(), 0, &store.IndexOptions{})
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, a := range index.Artists {
			for j := range a.Albums {
				for _, d := range a.Albums[j].Discs {
					got = append(got, d.Name)
				}
			}
		}
		return got
	}
	want := func(discs ...string) {
		t.Helper()
		if got := names(); !slices.Equal(got, discs) {
			t.Errorf("discs = %v, want %v", got, discs)
		}
	}
	want("Side A", "Side B")
	want("Side A", "Side B") // From what was kept.
	two.ID = idOf(t, s, &two)
	two.DiscSubtitle = "The Flip Side"
	writeAll(t, s, two)
	want("Side A", "The Flip Side")
}

// indexHead returns the index's first artist, who has an album.
func indexHead(t *testing.T, s *store.Store) store.IndexArtist {
	t.Helper()
	index, err := s.LibraryIndex(t.Context(), 0, &store.IndexOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Artists) == 0 || len(index.Artists[0].Albums) == 0 {
		t.Fatal("the index is empty")
	}
	return index.Artists[0]
}

// The writes an edit makes beside its batch change what the index shows too, so
// none may leave it showing what was kept from before them.
func TestIndexFollowsEveryEditWrite(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	w := newWrite(0)
	w.Year = 1999
	writeAll(t, s, w)
	if head := indexHead(t, s); head.Name != "Artist 0" || head.Albums[0].Year != 0 {
		t.Fatalf("index = %s, %d; want Artist 0, 0", head.Name, head.Albums[0].Year)
	}
	albums, err := s.TrackAlbumRows(t.Context(), []int64{idOf(t, s, &w)})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SettleAlbums(t.Context(), albums); err != nil {
		t.Fatal(err)
	}
	if year := indexHead(t, s).Albums[0].Year; year != 1999 {
		t.Errorf("year after SettleAlbums = %d, want 1999", year)
	}
	_, err = s.RenameArtists(t.Context(), []store.ArtistRename{{From: "artist-0", To: "artist-renamed", Name: "Renamed"}})
	if err != nil {
		t.Fatal(err)
	}
	if name := indexHead(t, s).Name; name != "Renamed" {
		t.Errorf("artist after RenameArtists = %s, want Renamed", name)
	}
}

func TestIndexIsWholeWhileTheLibraryChanges(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	writeAll(t, s, newWrite(0))
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := 1; i <= 30; i++ {
			if err := s.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{newWrite(i)}}); err != nil {
				t.Error(err)
				return
			}
		}
	})
	for range 30 {
		index, err := s.LibraryIndex(t.Context(), 0, &store.IndexOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range index.Artists {
			for _, al := range a.Albums {
				if len(al.Tracks) == 0 {
					t.Fatalf("album %s has no tracks: the index mixed two moments", al.Title)
				}
			}
		}
	}
	wg.Wait()
}
