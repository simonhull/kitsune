package library

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"testing/synctest"
	"time"
)

// pollLib is a library and a watcher over a fake notifier, for a bubble, with
// the options optsFor gives for the root.
func pollLib(t *testing.T, optsFor func(root string) WatchOptions) (*testLib, *Watcher) {
	t.Helper()
	l := newTestLib(t, 1)
	opts := optsFor(l.roots[0])
	if opts.notify == nil {
		n := newFakeNotifier()
		opts.notify = func() (notifier, error) { return n, nil }
	}
	w := NewWatcher(l.scanner, opts)
	watching(t, w)
	synctest.Wait()
	return l, w
}

// noMounts is a host whose mounts can't be read.
func noMounts() []mount { return nil }

// onNetwork says the root is a Samba share.
func onNetwork(root string) func() []mount {
	return func() []mount { return []mount{{point: "/", fstype: fsExt4}, {point: root, fstype: fsCIFS}} }
}

// wantPolledIn checks that a file copied under the polled root reaches the
// library after a poll and a settle, and not before the poll.
func wantPolledIn(t *testing.T, l *testLib) {
	t.Helper()
	root := l.roots[0]
	l.copyTo(root, "Artist/Album/one.flac", sampleFLAC)
	time.Sleep(defaultPoll - time.Second)
	synctest.Wait()
	if got := l.live(root); len(got) != 0 {
		t.Fatalf("live before the poll = %v, want none", got)
	}
	time.Sleep(time.Second + defaultSettle)
	synctest.Wait()
	if got := l.live(root); !slices.Equal(got, []string{"Artist/Album/one.flac"}) {
		t.Errorf("live after the poll = %v, want the new file", got)
	}
}

func TestWatcherPollsANetworkRoot(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		l, w := pollLib(t, func(root string) WatchOptions { return WatchOptions{mounts: onNetwork(root)} })
		want := []RootWatch{{Root: l.roots[0], Mode: Polling, Reason: ReasonNetwork, FSType: fsCIFS}}
		if got := w.Modes(); !slices.Equal(got, want) {
			t.Fatalf("modes = %+v, want %+v", got, want)
		}
		wantPolledIn(t, l)
	})
}

func TestWatcherPollsAtTheWatchLimit(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		n := newFakeNotifier()
		n.addErr = func(path string) error {
			if filepath.Base(path) == "deep" {
				return syscall.ENOSPC
			}
			return nil
		}
		l := newTestLib(t, 1)
		root := l.roots[0]
		if err := os.MkdirAll(filepath.Join(root, "a", "deep"), dirMode); err != nil {
			t.Fatal(err)
		}
		w := NewWatcher(l.scanner, WatchOptions{mounts: noMounts, notify: func() (notifier, error) { return n, nil }})
		watching(t, w)
		synctest.Wait()
		want := []RootWatch{{Root: root, Mode: Polling, Reason: ReasonWatchLimit}}
		if got := w.Modes(); !slices.Equal(got, want) {
			t.Fatalf("modes = %+v, want %+v", got, want)
		}
		wantPolledIn(t, l)
	})
}

func TestWatcherPollsWithoutANotifier(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		l, w := pollLib(t, func(string) WatchOptions {
			return WatchOptions{mounts: noMounts, notify: func() (notifier, error) { return nil, errors.New("too many open files") }}
		})
		want := []RootWatch{{Root: l.roots[0], Mode: Polling, Reason: ReasonWatchLimit}}
		if got := w.Modes(); !slices.Equal(got, want) {
			t.Fatalf("modes = %+v, want %+v", got, want)
		}
		wantPolledIn(t, l)
	})
}

func TestPollTurnsAVanishedRootOff(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		l := newTestLib(t, 1)
		root := l.roots[0]
		l.copyTo(root, "a/one.flac", sampleFLAC)
		l.scan(Quick)
		n := newFakeNotifier()
		w := NewWatcher(l.scanner, WatchOptions{mounts: onNetwork(root), notify: func() (notifier, error) { return n, nil }})
		watching(t, w)
		synctest.Wait()
		if err := os.RemoveAll(root); err != nil {
			t.Fatal(err)
		}
		time.Sleep(defaultPoll + defaultSettle)
		synctest.Wait()
		want := []RootWatch{{Root: root, Mode: WatchOff, Reason: ReasonMissing}}
		if got := w.Modes(); !slices.Equal(got, want) {
			t.Errorf("modes = %+v, want %+v", got, want)
		}
		if got := l.live(root); len(got) != 1 {
			t.Errorf("live = %v, want the track kept", got)
		}
	})
}

func TestChangedDirs(t *testing.T) {
	t.Parallel()
	before := map[string]uint64{".": 1, "a": 2, "b": 3}
	after := map[string]uint64{".": 1, "a": 9, "c": 4}
	if got, want := changedDirs(before, after), []string{"a", "b", "c"}; !slices.Equal(got, want) {
		t.Errorf("changedDirs = %v, want %v", got, want)
	}
}

func TestSnapshotSeesAFileRewrittenInPlace(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "a/one.flac", sampleFLAC)
	l.copyTo(root, "b/two.flac", sampleFLAC)
	l.touch(root, "b/notes.txt")
	before := snapshot(t.Context(), root)
	bump(t, filepath.Join(root, "a", "one.flac"), 3)
	l.write(t, root, "b/notes.txt", []byte("not audio")) // Rewritten in place: b itself is unchanged.
	if got, want := changedDirs(before, snapshot(t.Context(), root)), []string{"a"}; !slices.Equal(got, want) {
		t.Errorf("changed = %v, want %v", got, want)
	}
}
