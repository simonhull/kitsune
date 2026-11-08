package store_test

import (
	"slices"
	"testing"

	"github.com/simonhull/kitsune/internal/store"
)

// wantChanged checks the albums the store says changed, by their writes' IDs.
func wantChanged(t *testing.T, s *store.Store, want ...string) {
	t.Helper()
	slices.Sort(want)
	if got := s.TakeChangedAlbums(); !slices.Equal(got, want) {
		t.Errorf("changed albums = %v, want %v", got, want)
	}
}

func TestChangedAlbumsFollowWritesAndMarks(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	a, b := newWrite(0), newWrite(1)
	writeAll(t, s, a, b)
	wantChanged(t, s, a.Album.ULID, b.Album.ULID)
	wantChanged(t, s) // Taken.

	// A track that moves to another album touches both.
	moved := newWrite(0)
	moved.ID = idOf(t, s, &a)
	moved.Album = b.Album
	moved.Album.ULID = "02J99999999999999999999999" // Ignored: the album exists.
	writeAll(t, s, moved)
	wantChanged(t, s, a.Album.ULID, b.Album.ULID)

	if err := s.MarkMissing(t.Context(), []int64{idOf(t, s, &b), 999}, fixedNow()); err != nil {
		t.Fatal(err)
	}
	wantChanged(t, s, b.Album.ULID)
}

func TestChangedAlbumsLeaveOutAFailedBatch(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	good, unknown := newWrite(0), newWrite(1)
	unknown.ID = 999
	if err := s.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{good, unknown}}); err == nil {
		t.Fatal("a batch updating an unknown track was stored")
	}
	wantChanged(t, s)
}
