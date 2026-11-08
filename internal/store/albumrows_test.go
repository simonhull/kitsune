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
