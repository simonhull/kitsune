package store_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

const (
	shuffleKind = "shuffle"
	aClient     = "0123456789abcdef"
	aComputer   = "computer"
)

// setAndGet saves p as user's playback and reads it back, with its time cleared
// once it's checked.
func setAndGet(t *testing.T, s *store.Store, user int64, p *store.Playback) store.Playback {
	t.Helper()
	if err := s.SetPlayback(t.Context(), user, p); err != nil {
		t.Fatal(err)
	}
	got, err := s.Playback(t.Context(), user)
	if err != nil {
		t.Fatal(err)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero, want the store's clock")
	}
	got.UpdatedAt = time.Time{}
	return got
}

func TestPlaybackUpsertAndGet(t *testing.T) {
	t.Parallel()
	l := newShuffleLib(t)
	if _, err := l.s.Playback(t.Context(), l.ann); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Playback with none = %v, want ErrNotFound", err)
	}
	first := store.Playback{
		Kind: "album", Queue: []byte(`{"tracks":["a"]}`), At: 0, PositionMs: 133_000, Ordered: true,
		ClientID: aClient, Device: aComputer,
	}
	if got := setAndGet(t, l.s, l.ann, &first); !reflect.DeepEqual(got, first) {
		t.Errorf("Playback = %+v, want %+v", got, first)
	}
	second := store.Playback{
		Kind: shuffleKind, Queue: []byte(`{"tracks":["b"]}`), At: 0, ShuffleID: runID(9), ShuffleAfter: 4,
		ClientID: "fedcba9876543210", Device: "phone",
	}
	if got := setAndGet(t, l.s, l.ann, &second); !reflect.DeepEqual(got, second) {
		t.Errorf("Playback after a replace = %+v, want %+v", got, second)
	}
	if _, err := l.s.Playback(t.Context(), l.bob); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("bob's Playback = %v, want ErrNotFound: it's per person", err)
	}
}

func TestLiveTracks(t *testing.T) {
	t.Parallel()
	l := newShuffleLib(t)
	if err := l.s.MarkMissing(t.Context(), []int64{1}, time.UnixMilli(0)); err != nil {
		t.Fatal(err)
	}
	moved := newWrite(2)
	moved.ID, moved.Album = 3, newWrite(3).Album // Row 3 is song 2.
	writeAll(t, l.s, moved)

	got, err := l.s.LiveTracks(t.Context(), append(l.songs, "01JNOSUCHTRACK000000000000"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Errorf("LiveTracks = %v, want the four live songs", got)
	}
	if _, ok := got[l.songs[0]]; ok {
		t.Errorf("LiveTracks has the missing song")
	}
	if got[l.songs[2]] != newWrite(3).Album.ULID {
		t.Errorf("moved song's album = %s, want %s", got[l.songs[2]], newWrite(3).Album.ULID)
	}
	if empty, emptyErr := l.s.LiveTracks(t.Context(), nil); emptyErr != nil || len(empty) != 0 {
		t.Errorf("LiveTracks(nil) = %v, %v", empty, emptyErr)
	}
}

func TestPruneKeepsTheRememberedShuffle(t *testing.T) {
	t.Parallel()
	l := newShuffleLib(t)
	start := time.UnixMilli(1_761_000_000_000)
	create := func(n int) {
		t.Helper()
		at := start.Add(time.Duration(n) * time.Minute)
		err := l.s.CreateShuffleSession(t.Context(), l.ann, &store.ShuffleSession{
			ID: fmt.Sprintf("05S%023d", n), Scope: `{"kind":"library"}`, Mode: "endless",
			State: []byte("x"), CreatedAt: at, UpdatedAt: at,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	create(0)
	create(1)
	remembered := store.Playback{
		Kind: shuffleKind, Queue: []byte(`{"tracks":["a"]}`), ShuffleID: fmt.Sprintf("05S%023d", 0),
		ClientID: aClient, Device: aComputer,
	}
	if err := l.s.SetPlayback(t.Context(), l.ann, &remembered); err != nil {
		t.Fatal(err)
	}
	for n := 2; n < 30; n++ {
		create(n)
	}
	if _, err := l.s.ShuffleSessionOf(t.Context(), l.ann, remembered.ShuffleID); err != nil {
		t.Errorf("the remembered session = %v, want it kept", err)
	}
	if _, err := l.s.ShuffleSessionOf(t.Context(), l.ann, fmt.Sprintf("05S%023d", 1)); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("an old session = %v, want it pruned", err)
	}
	if _, err := l.s.ShuffleSessionOf(t.Context(), l.ann, fmt.Sprintf("05S%023d", 29)); err != nil {
		t.Errorf("the newest session = %v, want it kept", err)
	}
}
