package store_test

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

func openMatching(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "kitsune.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

const lookupSource = "musicbrainz"

// wantLookup checks what s keeps for source and key at notBefore: want, or
// nothing when want is "".
func wantLookup(t *testing.T, s *store.Store, source, key string, notBefore time.Time, want string) {
	t.Helper()
	body, err := s.Lookup(t.Context(), source, key, notBefore)
	if string(body) != want || (want == "") != errors.Is(err, store.ErrNotFound) {
		t.Errorf("Lookup(%s, %s) = %q, %v; want %q", source, key, body, err, want)
	}
}

func TestLookupsKeepADay(t *testing.T) {
	t.Parallel()
	s := openMatching(t)
	put := func(key, body string, at time.Time) {
		t.Helper()
		if err := s.PutLookup(t.Context(), lookupSource, key, []byte(body), at, at.Add(-24*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	at := time.UnixMilli(1_762_000_000_000)
	put("/release/a", "old", at)
	wantLookup(t, s, lookupSource, "/release/a", at.Add(-time.Hour), "old")
	wantLookup(t, s, lookupSource, "/release/a", at.Add(time.Millisecond), "")
	wantLookup(t, s, "discogs", "/release/a", at.Add(-time.Hour), "")
	// A day on, keeping another answer drops the first.
	later := at.Add(25 * time.Hour)
	put("/release/b", "new", later)
	wantLookup(t, s, lookupSource, "/release/a", time.UnixMilli(0), "")
	put("/release/b", "newer", later)
	wantLookup(t, s, lookupSource, "/release/b", later, "newer")
}

// queued is the queue's row for the album of newWrite(i).
func queued(i int, state store.MatchState) store.QueuedAlbum {
	w := newWrite(i)
	return store.QueuedAlbum{ID: w.Album.ULID, Name: w.Album.Name, Artist: w.Album.Artist, State: state, Tracks: 1}
}

// wantQueue checks the queue s holds.
func wantQueue(t *testing.T, s *store.Store, want ...store.QueuedAlbum) {
	t.Helper()
	got, err := s.MatchQueue(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("queue = %+v, want %+v", got, want)
	}
}

func TestMatchQueue(t *testing.T) {
	t.Parallel()
	s := openMatching(t)
	ctx := t.Context()
	tagged := newWrite(2)
	tagged.Album.MBID = "7c1e9a52-4d0b-4f6e-9a1c-2b77d5e03b04"
	missing := newWrite(4)
	writeAll(t, s, newWrite(1), tagged, newWrite(3), missing)
	if err := s.MarkMissing(ctx, []int64{idOf(t, s, &missing)}, time.UnixMilli(1_762_000_000_000)); err != nil {
		t.Fatal(err)
	}
	// Untagged albums with live tracks.
	wantQueue(t, s, queued(1, store.MatchToDo), queued(3, store.MatchToDo))
	set := func(i int, state store.MatchState) {
		t.Helper()
		if err := s.SetMatchState(ctx, newWrite(i).Album.ULID, state); err != nil {
			t.Fatal(err)
		}
	}
	set(1, store.MatchSkipped)
	set(3, store.MatchMatched)
	set(2, store.MatchMatched) // Matched stays in the queue, tagged or not.
	wantQueue(t, s, queued(1, store.MatchSkipped), queued(2, store.MatchMatched), queued(3, store.MatchMatched))
	set(1, store.MatchToDo)
	wantQueue(t, s, queued(1, store.MatchToDo), queued(2, store.MatchMatched), queued(3, store.MatchMatched))
	for _, state := range []store.MatchState{store.MatchToDo, store.MatchSkipped} {
		if err := s.SetMatchState(ctx, "02J00000000000000000000099", state); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("%s on an unknown album = %v, want ErrNotFound", state, err)
		}
	}
}
