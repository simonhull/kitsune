package store_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

// shuffleLib is a store of five songs and two people.
type shuffleLib struct {
	s        *store.Store
	ann, bob int64
	songs    []string // Track ULIDs.
}

func newShuffleLib(t *testing.T) *shuffleLib {
	t.Helper()
	l := &shuffleLib{s: openUsers(t)}
	for i := range 5 {
		w := newWrite(i)
		writeAll(t, l.s, w)
		l.songs = append(l.songs, w.ULID)
	}
	for _, name := range []string{"ann", "bob"} {
		u, err := l.s.CreateUser(t.Context(), store.NewUser{Name: name, PasswordHash: "h", Role: store.RoleUser})
		if err != nil {
			t.Fatal(err)
		}
		if name == "ann" {
			l.ann = u.ID
		} else {
			l.bob = u.ID
		}
	}
	return l
}

// runID is a 26-character run ULID for n.
func runID(n int) string { return fmt.Sprintf("05J%023d", n) }

func (l *shuffleLib) run(t *testing.T, user int64, n int, songs ...int) store.ShuffleRun {
	t.Helper()
	ids := make([]string, len(songs))
	for i, song := range songs {
		ids[i] = l.songs[song]
	}
	run, err := l.s.SetRun(t.Context(), user, runID(n), ids)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func (l *shuffleLib) rules(t *testing.T, user int64) store.ShuffleRules {
	t.Helper()
	rules, err := l.s.ShuffleRulesOf(t.Context(), user, l.songs)
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

func TestRunsArePerPerson(t *testing.T) {
	t.Parallel()
	l := newShuffleLib(t)
	l.run(t, l.ann, 1, 0, 1)
	l.run(t, l.bob, 2, 1, 0, 2)
	if got := l.rules(t, l.ann).Runs; len(got) != 1 || !slices.Equal(got[0].Tracks, []string{l.songs[0], l.songs[1]}) {
		t.Errorf("ann's runs = %+v, want hers alone", got)
	}
	if got := l.rules(t, l.bob).Runs; len(got) != 1 || got[0].ID != runID(2) || len(got[0].Tracks) != 3 {
		t.Errorf("bob's runs = %+v, want his own", got)
	}
}

func TestAReorderIsTheSameRun(t *testing.T) {
	t.Parallel()
	l := newShuffleLib(t)
	l.run(t, l.ann, 1, 0, 1, 2)
	again := l.run(t, l.ann, 2, 2, 0, 1)
	if again.ID != runID(1) {
		t.Errorf("reordered run = %s, want %s kept", again.ID, runID(1))
	}
	got := l.rules(t, l.ann).Runs
	if len(got) != 1 || !slices.Equal(got[0].Tracks, []string{l.songs[2], l.songs[0], l.songs[1]}) {
		t.Errorf("runs = %+v, want one in the new order", got)
	}
}

func TestASongMovesBetweenRuns(t *testing.T) {
	t.Parallel()
	l := newShuffleLib(t)
	l.run(t, l.ann, 1, 0, 1, 2)
	l.run(t, l.ann, 2, 3, 2)
	got := l.rules(t, l.ann).Runs
	if len(got) != 2 || !slices.Equal(got[0].Tracks, []string{l.songs[0], l.songs[1]}) ||
		!slices.Equal(got[1].Tracks, []string{l.songs[3], l.songs[2]}) {
		t.Errorf("runs = %+v, want song 2 moved to the second", got)
	}
}

func TestARunOfOneGoes(t *testing.T) {
	t.Parallel()
	l := newShuffleLib(t)
	l.run(t, l.ann, 1, 0, 1)
	l.run(t, l.ann, 2, 1, 2)
	got := l.rules(t, l.ann).Runs
	if len(got) != 1 || got[0].ID != runID(1) || !slices.Equal(got[0].Tracks, []string{l.songs[1], l.songs[2]}) {
		t.Errorf("runs = %+v, want the first run gone with its last-but-one song", got)
	}
}

func TestDeletingASongCascades(t *testing.T) {
	t.Parallel()
	l := newShuffleLib(t)
	l.run(t, l.ann, 1, 0, 1)
	l.run(t, l.bob, 2, 0, 1, 2)
	if err := l.s.SetExcluded(t.Context(), l.ann, []string{l.songs[0]}, true); err != nil {
		t.Fatal(err)
	}
	if err := l.s.MarkMissing(t.Context(), []int64{1}, time.UnixMilli(0)); err != nil {
		t.Fatal(err)
	}
	if _, err := l.s.DeleteMissingBefore(t.Context(), time.UnixMilli(1)); err != nil {
		t.Fatal(err)
	}
	if got := l.rules(t, l.ann); len(got.Runs) != 0 || len(got.Excluded) != 0 {
		t.Errorf("ann's rules = %+v, want the run of two and the exclusion gone with the song", got)
	}
	if got := l.rules(t, l.bob).Runs; len(got) != 1 || !slices.Equal(got[0].Tracks, []string{l.songs[1], l.songs[2]}) {
		t.Errorf("bob's runs = %+v, want the rest of his run", got)
	}
}

func TestDeleteRun(t *testing.T) {
	t.Parallel()
	l := newShuffleLib(t)
	l.run(t, l.ann, 1, 0, 1)
	if err := l.s.DeleteRun(t.Context(), l.bob, runID(1)); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("bob deleting ann's run = %v, want ErrNotFound", err)
	}
	if err := l.s.DeleteRun(t.Context(), l.ann, runID(1)); err != nil {
		t.Fatal(err)
	}
	if got := l.rules(t, l.ann).Runs; len(got) != 0 {
		t.Errorf("runs = %+v, want none", got)
	}
}

func TestSetRunRefuses(t *testing.T) {
	t.Parallel()
	l := newShuffleLib(t)
	for _, tracks := range [][]string{{l.songs[0]}, {l.songs[0], l.songs[0]}} {
		if _, err := l.s.SetRun(t.Context(), l.ann, runID(1), tracks); !errors.Is(err, store.ErrShortRun) {
			t.Errorf("SetRun(%q) = %v, want ErrShortRun", tracks, err)
		}
	}
	if _, err := l.s.SetRun(t.Context(), l.ann, runID(1), []string{l.songs[0], "nope"}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("SetRun of an unknown song = %v, want ErrNotFound", err)
	}
}

func TestExcludedIsPerPerson(t *testing.T) {
	t.Parallel()
	l := newShuffleLib(t)
	if err := l.s.SetExcluded(t.Context(), l.ann, []string{l.songs[3], l.songs[4]}, true); err != nil {
		t.Fatal(err)
	}
	if err := l.s.SetExcluded(t.Context(), l.ann, []string{l.songs[4]}, false); err != nil {
		t.Fatal(err)
	}
	if got := l.rules(t, l.ann).Excluded; !slices.Equal(got, []string{l.songs[3]}) {
		t.Errorf("ann excluded %q, want song 3", got)
	}
	if got := l.rules(t, l.bob).Excluded; len(got) != 0 {
		t.Errorf("bob excluded %q, want none", got)
	}
	if err := l.s.SetExcluded(t.Context(), l.ann, []string{"nope"}, true); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("excluding an unknown song = %v, want ErrNotFound", err)
	}
}

// indexMarks returns the shuffle marks the index gives user, by track.
func (l *shuffleLib) indexMarks(t *testing.T, user int64) map[string]store.ShuffleMark {
	t.Helper()
	index, err := l.s.LibraryIndex(t.Context(), user, &store.IndexOptions{Shuffle: true})
	if err != nil {
		t.Fatal(err)
	}
	marks := map[string]store.ShuffleMark{}
	for _, a := range index.Artists {
		for _, album := range a.Albums {
			for _, tr := range album.Tracks {
				if tr.Extra != nil && tr.Extra.Shuffle != nil {
					marks[tr.ID] = *tr.Extra.Shuffle
				}
			}
		}
	}
	return marks
}

func TestIndexCarriesShuffleMarks(t *testing.T) {
	t.Parallel()
	l := newShuffleLib(t)
	l.run(t, l.ann, 1, 2, 0)
	if err := l.s.SetExcluded(t.Context(), l.ann, []string{l.songs[4]}, true); err != nil {
		t.Fatal(err)
	}
	marks := l.indexMarks(t, l.ann)
	want := map[string]store.ShuffleMark{
		l.songs[2]: {Run: runID(1), Position: 1, Length: 2},
		l.songs[0]: {Run: runID(1), Position: 2, Length: 2},
		l.songs[4]: {Excluded: true},
	}
	if len(marks) != len(want) {
		t.Fatalf("marks = %+v, want %+v", marks, want)
	}
	for id, m := range want {
		if marks[id] != m {
			t.Errorf("mark of %s = %+v, want %+v", id, marks[id], m)
		}
	}
}
