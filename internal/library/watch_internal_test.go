package library

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/fsnotify/fsnotify"
)

// fakeNotifier is a notifier the test drives. Its channels belong to whoever
// makes it, so one made in a synctest bubble blocks durably.
type fakeNotifier struct {
	events chan fsnotify.Event
	errs   chan error
	mu     sync.Mutex
	added  []string
	addErr func(path string) error // Nil accepts every folder.
}

func newFakeNotifier() *fakeNotifier {
	return &fakeNotifier{events: make(chan fsnotify.Event), errs: make(chan error)}
}

func (f *fakeNotifier) Add(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.addErr != nil {
		if err := f.addErr(path); err != nil {
			return err
		}
	}
	f.added = append(f.added, path)
	return nil
}

func (*fakeNotifier) Remove(string) error             { return nil }
func (f *fakeNotifier) Events() <-chan fsnotify.Event { return f.events }
func (f *fakeNotifier) Errors() <-chan error          { return f.errs }
func (*fakeNotifier) Close() error                    { return nil }

// watching starts w on its own goroutine until the test ends.
func watching(t *testing.T, w *Watcher) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.WithoutCancel(t.Context()))
	var run sync.WaitGroup
	run.Go(func() { w.Run(ctx) })
	t.Cleanup(func() { cancel(); run.Wait() })
}

// eventually fails the test unless ok holds within a few seconds.
func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting: %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// realWatcher returns a watcher with a short settle over the OS's notifications.
func (l *testLib) realWatcher() *Watcher {
	l.t.Helper()
	w := NewWatcher(l.scanner, WatchOptions{Settle: 150 * time.Millisecond})
	watching(l.t, w)
	eventually(l.t, "the roots are watched", func() bool {
		modes := w.Modes()
		return len(modes) == len(l.roots) && !slices.ContainsFunc(modes, func(m RootWatch) bool { return m.Mode != Watching })
	})
	return w
}

func TestWatcherScansAnAlbumCopiedIn(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "old/one.flac", sampleFLAC)
	l.scan(Quick)
	l.realWatcher()
	l.copyTo(root, "Artist/Album/CD1/one.flac", sampleFLAC)
	l.copyTo(root, "Artist/Album/CD1/two.mp3", sampleMP3)
	l.copyTo(root, "Artist/Album/cover.jpg", sampleM4A)
	eventually(t, "the album is in the library", func() bool { return len(l.live(root)) == 3 })
}

func TestWatcherMoveKeepsTheID(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "A/Album/one.flac", sampleFLAC)
	l.copyTo(root, "B/keep.mp3", sampleMP3)
	l.scan(Quick)
	l.fingerprint()
	id := l.id(root, "A/Album/one.flac")
	l.realWatcher()
	if err := os.Rename(filepath.Join(root, "A", "Album"), filepath.Join(root, "B", "Album")); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the move is scanned", func() bool { return slices.Contains(l.live(root), "B/Album/one.flac") })
	if got := l.id(root, "B/Album/one.flac"); got != id {
		t.Errorf("ID after the move = %d, want %d", got, id)
	}
	// The moved folder is watched at its new path.
	l.copyTo(root, "B/Album/two.mp3", sampleMP3)
	eventually(t, "a file added to the moved folder is scanned", func() bool {
		return slices.Contains(l.live(root), "B/Album/two.mp3")
	})
}

func TestWatcherRootRemovedDeletesNothing(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 2)
	gone, kept := l.roots[0], l.roots[1]
	l.copyTo(gone, "a/one.flac", sampleFLAC)
	l.copyTo(gone, "b/two.mp3", sampleMP3)
	l.copyTo(kept, "c/three.flac", sampleFLAC)
	l.scan(Quick)
	w := l.realWatcher()
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the removed root is off", func() bool {
		return slices.Contains(w.Modes(), RootWatch{Root: gone, Mode: WatchOff, Reason: ReasonMissing})
	})
	l.copyTo(kept, "d/four.mp3", sampleMP3) // A later change elsewhere still scans.
	eventually(t, "the other root is still watched", func() bool { return len(l.live(kept)) == 2 })
	if got := l.live(gone); len(got) != 2 {
		t.Errorf("live under the removed root = %v, want both tracks", got)
	}
}

func TestWatcherRearmsARootAfterAScan(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	w := NewWatcher(l.scanner, WatchOptions{Settle: 150 * time.Millisecond})
	watching(t, w)
	eventually(t, "the missing root is off", func() bool {
		return slices.Equal(w.Modes(), []RootWatch{{Root: root, Mode: WatchOff, Reason: ReasonMissing}})
	})
	if err := os.Mkdir(root, dirMode); err != nil {
		t.Fatal(err)
	}
	l.scan(Quick)
	eventually(t, "the root is watched again", func() bool {
		return slices.Equal(w.Modes(), []RootWatch{{Root: root, Mode: Watching}})
	})
}

func TestWatcherFollowsTheRoots(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 2)
	w := NewWatcher(l.scanner, WatchOptions{Settle: 150 * time.Millisecond})
	if err := l.scanner.Configure(t.Context(), Live{Roots: l.roots[:1], MissingGrace: testGrace}); err != nil {
		t.Fatal(err)
	}
	watching(t, w)
	eventually(t, "one root is watched", func() bool { return len(w.Modes()) == 1 })
	if err := l.scanner.Configure(t.Context(), Live{Roots: l.roots[1:], MissingGrace: testGrace}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the new root is watched", func() bool {
		return slices.Equal(w.Modes(), []RootWatch{{Root: l.roots[1], Mode: Watching}})
	})
}

// synctestLib is a library and a watcher over a fake notifier, for a bubble.
func synctestLib(t *testing.T) (*testLib, *fakeNotifier) {
	t.Helper()
	l := newTestLib(t, 1)
	n := newFakeNotifier()
	w := NewWatcher(l.scanner, WatchOptions{notify: func() (notifier, error) { return n, nil }})
	watching(t, w)
	synctest.Wait()
	return l, n
}

func TestWatcherWaitsForChangesToSettle(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		l, n := synctestLib(t)
		root := l.roots[0]
		send := func(rel string) {
			l.copyTo(root, rel, sampleFLAC)
			n.events <- fsnotify.Event{Name: filepath.Join(root, rel), Op: fsnotify.Create}
		}
		send("a/one.flac")
		time.Sleep(5 * time.Second)
		send("a/two.flac")
		time.Sleep(defaultSettle - time.Millisecond)
		synctest.Wait()
		if got := l.live(root); len(got) != 0 {
			t.Fatalf("live before the settle = %v, want none", got)
		}
		time.Sleep(time.Millisecond)
		synctest.Wait()
		if got := l.live(root); len(got) != 2 {
			t.Errorf("live after the settle = %v, want both", got)
		}
	})
}

func TestWatcherScansAFolderThatNeverSettles(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		l, n := synctestLib(t)
		root := l.roots[0]
		l.copyTo(root, "a/one.flac", sampleFLAC)
		for range int(defaultMaxWait / time.Second) {
			n.events <- fsnotify.Event{Name: filepath.Join(root, "a", "one.flac"), Op: fsnotify.Write}
			time.Sleep(time.Second)
		}
		synctest.Wait()
		if got := l.live(root); len(got) != 1 {
			t.Errorf("live after the longest wait = %v, want the file", got)
		}
	})
}

func TestWatcherIgnoresWhatAScanSkips(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		l, n := synctestLib(t)
		root := l.roots[0]
		for _, rel := range []string{"@eaDir/a.flac", ".hidden/b.flac", "a/.c.flac", "a/.d.flac.audiometa-1.tmp", "a/notes.txt"} {
			l.copyTo(root, rel, sampleFLAC)
			n.events <- fsnotify.Event{Name: filepath.Join(root, rel), Op: fsnotify.Create}
		}
		time.Sleep(2 * defaultSettle)
		synctest.Wait()
		if got := l.live(root); len(got) != 0 {
			t.Errorf("live = %v, want none", got)
		}
	})
}

func TestWatcherLeavesChangesARunningScanSaw(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		l, n := synctestLib(t)
		root := l.roots[0]
		l.copyTo(root, "a/one.flac", sampleFLAC)
		n.events <- fsnotify.Event{Name: filepath.Join(root, "a", "one.flac"), Op: fsnotify.Create}
		run, err := l.scanner.Claim(Quick) // A scan claimed before the settle, which walks after the change.
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(defaultSettle)
		synctest.Wait()
		stats, err := run(t.Context())
		if err != nil || stats.Added != 1 {
			t.Fatalf("scan = %+v, %v; want the file added", stats, err)
		}
		synctest.Wait()
		if got := l.live(root); len(got) != 1 {
			t.Errorf("live = %v, want the file once", got)
		}
	})
}
