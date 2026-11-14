package library

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

const (
	firstFLAC = "a/one.flac"
	twoFLAC   = "a/two.flac"
)

// trashLib is a scanned library with tag writes on and two songs on one album.
func trashLib(t *testing.T) *testLib {
	t.Helper()
	l := newTestLib(t, 1)
	l.opts.TagWrites = true
	l.scanner = NewScanner(l.st, l.opts)
	l.copyTo(l.roots[0], firstFLAC, sampleFLAC)
	l.copyTo(l.roots[0], twoFLAC, sampleFLAC)
	l.scan(Quick)
	return l
}

// ulidAt returns the public ID of the track at rel.
func (l *testLib) ulidAt(rel string) string {
	l.t.Helper()
	tracks := l.albumDetail().Tracks
	for i := range tracks {
		f, err := l.st.TrackFile(l.t.Context(), tracks[i].ID)
		if err == nil && f.RelPath == rel {
			return tracks[i].ID
		}
	}
	l.t.Fatalf("no live track at %s", rel)
	return ""
}

// exists reports whether root/rel exists.
func exists(t *testing.T, root, rel string) bool {
	t.Helper()
	_, err := os.Lstat(filepath.Join(root, rel))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return err == nil
}

// A trashed song's file waits in the Trash and its track leaves the library;
// scans leave both alone; a restore puts the file back, and the next scan finds
// it at its old path with its old ID.
func TestTrashAndRestoreRoundTrip(t *testing.T) {
	t.Parallel()
	l := trashLib(t)
	root := l.roots[0]
	id := l.ulidAt(twoFLAC)
	change, err := l.scanner.TrashTracks(t.Context(), 0, []string{id})
	if err != nil {
		t.Fatal(err)
	}
	if want := (TrashChange{Tracks: []string{id}, Albums: []string{l.albumDetail().ID}}); !reflect.DeepEqual(change, want) {
		t.Errorf("change = %+v, want %+v", change, want)
	}
	inTrash := trashPath(id, twoFLAC)
	if got := []bool{exists(t, root, firstFLAC), exists(t, root, twoFLAC), exists(t, root, inTrash)}; !slices.Equal(got, []bool{true, false, true}) {
		t.Fatalf("after trash: the other file, the file, the file in the Trash = %v; want only the file gone", got)
	}
	if got := l.live(root); !slices.Equal(got, []string{firstFLAC}) {
		t.Errorf("live = %v, want the other song alone", got)
	}
	l.scansLeaveTheTrash(id)
	if _, err = l.scanner.RestoreTracks(t.Context(), []string{id}); err != nil {
		t.Fatal(err)
	}
	if got := []bool{exists(t, root, twoFLAC), exists(t, root, TrashDir)}; !slices.Equal(got, []bool{true, false}) {
		t.Errorf("after restore: the file back, the Trash left = %v; want back, and no Trash", got)
	}
	if stats := l.scan(Full); stats.Added+stats.Missing != 0 {
		t.Errorf("scan after restore = %+v, want nothing added or missing", stats)
	}
	if got := l.ulidAt(twoFLAC); got != id {
		t.Errorf("restored song's ID = %s, want %s", got, id)
	}
}

// scansLeaveTheTrash scans quick and full, and fails unless both leave the
// trashed song id alone: nothing missing, added or deleted.
func (l *testLib) scansLeaveTheTrash(id string) {
	l.t.Helper()
	for _, mode := range []Mode{Quick, Full} {
		if stats := l.scan(mode); stats.Missing+stats.Added != 0 || stats.Deleted != 0 {
			l.t.Errorf("%s scan with a song in the Trash = %+v, want nothing missing, added or deleted", mode, stats)
		}
	}
	if _, err := l.st.TrashedTracks(l.t.Context(), []string{id}); err != nil {
		l.t.Fatalf("the scans took the song out of the Trash: %v", err)
	}
}

// A restore never overwrites: a file that appeared at the old path, scanned or
// not, refuses it, and both files stay as they were.
func TestRestoreRefusedWhenThePathIsTaken(t *testing.T) {
	t.Parallel()
	for _, scanned := range []bool{false, true} {
		l := trashLib(t)
		root := l.roots[0]
		id := l.ulidAt(twoFLAC)
		if _, err := l.scanner.TrashTracks(t.Context(), 0, []string{id}); err != nil {
			t.Fatal(err)
		}
		l.write(t, root, twoFLAC, []byte("a new file"))
		if scanned {
			l.scan(Quick)
		}
		_, err := l.scanner.RestoreTracks(t.Context(), []string{id})
		taken, ok := errors.AsType[*PathTakenError](err)
		if !ok || len(taken.Titles) != 1 {
			t.Fatalf("scanned %t: restore = %v, want a PathTakenError naming the song", scanned, err)
		}
		if b, readErr := os.ReadFile(filepath.Join(root, twoFLAC)); readErr != nil || string(b) != "a new file" {
			t.Errorf("scanned %t: the new file = %q, %v; want it untouched", scanned, b, readErr)
		}
		if !exists(t, root, trashPath(id, twoFLAC)) {
			t.Errorf("scanned %t: the trashed file left the Trash", scanned)
		}
	}
}

// An album goes to the Trash whole, and Undo brings it back whole.
func TestTrashAlbum(t *testing.T) {
	t.Parallel()
	l := trashLib(t)
	album := l.albumDetail().ID
	change, err := l.scanner.TrashAlbums(t.Context(), 0, []string{album})
	if err != nil {
		t.Fatal(err)
	}
	if len(change.Tracks) != 2 || !slices.Equal(change.Albums, []string{album}) {
		t.Errorf("change = %+v, want both songs of %s", change, album)
	}
	if got := l.live(l.roots[0]); len(got) != 0 {
		t.Errorf("live = %v, want none", got)
	}
	if _, err = l.scanner.TrashAlbums(t.Context(), 0, []string{album}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("trashing the empty album again = %v, want ErrNotFound", err)
	}
	if _, err = l.scanner.RestoreTracks(t.Context(), change.Tracks); err != nil {
		t.Fatal(err)
	}
	if got := l.live(l.roots[0]); len(got) != 2 {
		t.Errorf("live after restore = %v, want both", got)
	}
}

// A file that won't move undoes the whole trash: every file stays, every song
// stays live.
func TestTrashIsAllOrNothing(t *testing.T) {
	t.Parallel()
	l := trashLib(t)
	root := l.roots[0]
	ids := []string{l.ulidAt(firstFLAC), l.ulidAt(twoFLAC)}
	// The second song's folder in the Trash can't be made: a file has its name.
	l.write(t, root, filepath.Join(TrashDir, ids[1]), []byte("in the way"))
	_, err := l.scanner.TrashTracks(t.Context(), 0, ids)
	if _, ok := errors.AsType[*MoveError](err); !ok {
		t.Fatalf("trash = %v, want a MoveError", err)
	}
	if !exists(t, root, firstFLAC) || !exists(t, root, twoFLAC) || exists(t, root, trashPath(ids[0], firstFLAC)) {
		t.Error("a file moved, or did not come back")
	}
	if got := l.live(root); len(got) != 2 {
		t.Errorf("live = %v, want both songs", got)
	}
}

// With tag writes off nothing under a root may change, so the Trash refuses
// everything and purges nothing.
func TestTrashNeedsTagWrites(t *testing.T) {
	t.Parallel()
	l := trashLib(t)
	id := l.ulidAt(twoFLAC)
	if _, err := l.scanner.TrashTracks(t.Context(), 0, []string{id}); err != nil {
		t.Fatal(err)
	}
	l.clock.Advance(TrashRetention + time.Hour)
	l.opts.TagWrites = false
	l.scanner = NewScanner(l.st, l.opts)
	ops := map[string]func() error{
		"trash": func() error {
			_, err := l.scanner.TrashTracks(t.Context(), 0, []string{l.ulidAt(firstFLAC)})
			return err
		},
		"trash an album": func() error {
			_, err := l.scanner.TrashAlbums(t.Context(), 0, []string{l.albumDetail().ID})
			return err
		},
		"restore": func() error {
			_, err := l.scanner.RestoreTracks(t.Context(), []string{id})
			return err
		},
		"empty": func() error {
			_, err := l.scanner.EmptyTrash(t.Context(), nil)
			return err
		},
	}
	for name, op := range ops {
		if err := op(); !errors.Is(err, ErrTagWritesOff) {
			t.Errorf("%s = %v, want ErrTagWritesOff", name, err)
		}
	}
	if n, err := l.scanner.PurgeTrash(t.Context()); err != nil || n != 0 {
		t.Errorf("purge = %d, %v; want nothing", n, err)
	}
	if !exists(t, l.roots[0], trashPath(id, twoFLAC)) {
		t.Error("the trashed file went")
	}
}

// The Trash waits behind a scan or an edit, as an edit does.
func TestTrashIsBusyDuringAScan(t *testing.T) {
	t.Parallel()
	l := trashLib(t)
	if !l.scanner.lock.tryLock(heldByScan) {
		t.Fatal("the library lock is held")
	}
	defer l.scanner.lock.unlock()
	if _, err := l.scanner.TrashTracks(t.Context(), 0, []string{l.ulidAt(firstFLAC)}); !errors.Is(err, ErrBusy) {
		t.Errorf("trash during a scan = %v, want ErrBusy", err)
	}
}

// Emptying deletes the files and the songs for good, and the album they alone
// were on.
func TestEmptyTrash(t *testing.T) {
	t.Parallel()
	l := trashLib(t)
	root := l.roots[0]
	album := l.albumDetail().ID
	change, err := l.scanner.TrashAlbums(t.Context(), 0, []string{album})
	if err != nil {
		t.Fatal(err)
	}
	n, err := l.scanner.EmptyTrash(t.Context(), change.Tracks[:1])
	if err != nil || n != 1 {
		t.Fatalf("empty one = %d, %v; want 1", n, err)
	}
	if n, err = l.scanner.EmptyTrash(t.Context(), nil); err != nil || n != 1 {
		t.Fatalf("empty all = %d, %v; want the other 1", n, err)
	}
	if exists(t, root, TrashDir) {
		t.Error("the Trash is still there")
	}
	if _, err = l.st.Album(t.Context(), album); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("album = %v, want it gone", err)
	}
	if !exists(t, root, "a") {
		t.Error("the album's folder went: only the files may")
	}
}

// A trashed song is deleted for good 30 days after it was trashed, and not a
// moment before.
func TestPurgeTrashAfterThirtyDays(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		l := newTestLib(t, 1)
		l.opts.TagWrites, l.opts.Now = true, time.Now
		l.scanner = NewScanner(l.st, l.opts)
		root := l.roots[0]
		l.copyTo(root, firstFLAC, sampleFLAC)
		l.copyTo(root, twoFLAC, sampleFLAC)
		l.scan(Quick)
		first := l.ulidAt(firstFLAC)
		if _, err := l.scanner.TrashTracks(t.Context(), 0, []string{first}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(24 * time.Hour)
		second := l.ulidAt(twoFLAC)
		if _, err := l.scanner.TrashTracks(t.Context(), 0, []string{second}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(TrashRetention - 24*time.Hour - time.Second)
		if n, err := l.scanner.PurgeTrash(t.Context()); err != nil || n != 0 {
			t.Fatalf("purge a second early = %d, %v; want nothing", n, err)
		}
		time.Sleep(2 * time.Second)
		if n, err := l.scanner.PurgeTrash(t.Context()); err != nil || n != 1 {
			t.Fatalf("purge at 30 days = %d, %v; want the first song", n, err)
		}
		if exists(t, root, trashPath(first, firstFLAC)) || !exists(t, root, trashPath(second, twoFLAC)) {
			t.Error("purged the wrong file")
		}
		if _, err := l.st.TrashedTracks(t.Context(), []string{second}); err != nil {
			t.Errorf("the second song went a day early: %v", err)
		}
	})
}
