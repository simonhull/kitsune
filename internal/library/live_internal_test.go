package library

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestConfigureWaitsForTheLibrary(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	if err := l.scanner.lock.lock(t.Context(), heldByEdit); err != nil { // An edit in progress.
		t.Fatal(err)
	}
	done := make(chan error, 1)
	next := Live{Roots: []string{t.TempDir()}, MissingGrace: time.Minute, TagWrites: true}
	go func() { done <- l.scanner.Configure(t.Context(), next) }()
	time.Sleep(20 * time.Millisecond)
	if l.scanner.TagWrites() || !slices.Equal(l.scanner.Roots(), l.roots) {
		t.Fatal("Configure applied while an edit held the library")
	}
	l.scanner.lock.unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !l.scanner.TagWrites() || !slices.Equal(l.scanner.Roots(), next.Roots) || l.scanner.live().MissingGrace != time.Minute {
		t.Errorf("after the edit, live = %+v, want %+v", l.scanner.live(), next)
	}
}

func TestConfigureGivesUpWithItsContext(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	if err := l.scanner.lock.lock(t.Context(), heldByScan); err != nil {
		t.Fatal(err)
	}
	defer l.scanner.lock.unlock()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := l.scanner.Configure(ctx, Live{TagWrites: true}); !errors.Is(err, context.Canceled) || l.scanner.TagWrites() {
		t.Errorf("Configure = %v, tag writes %v; want canceled and nothing changed", err, l.scanner.TagWrites())
	}
}

func TestEditRefusedOnceTagWritesAreOff(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	if err := l.scanner.EditAlbum(t.Context(), "01J00000000000000000000000", "etag", &AlbumEdit{}); !errors.Is(err, ErrTagWritesOff) {
		t.Errorf("EditAlbum = %v, want ErrTagWritesOff", err)
	}
}

func TestRunDailyMoves(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var scans atomic.Int32
		at := atomic.Pointer[Daily]{}
		at.Store(&Daily{Hour: 3, Minute: 30})
		moved := make(chan struct{}, 1)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		utc := func() time.Time { return time.Now().UTC() } // The bubble starts at midnight UTC.
		go runDaily(ctx, utc, func() Daily { return *at.Load() }, moved,
			func(context.Context) { scans.Add(1) }, func() {})
		synctest.Wait() // The first scan, at 00:00 in the bubble.
		at.Store(&Daily{Hour: 1, Minute: 0})
		moved <- struct{}{}
		synctest.Wait()
		time.Sleep(time.Hour + time.Second)
		synctest.Wait()
		if got := scans.Load(); got != 2 {
			t.Errorf("scans by 01:00 = %d, want the moved scan to have run", got)
		}
		time.Sleep(3 * time.Hour)
		synctest.Wait()
		if got := scans.Load(); got != 2 {
			t.Errorf("scans by 04:00 = %d, want no scan at the old 03:30", got)
		}
	})
}
