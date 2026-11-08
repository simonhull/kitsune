package library

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// ErrBusy means an edit was asked for while a scan or another edit held the
// library.
var ErrBusy = errors.New("the library is busy")

// holder says who holds a [libraryLock].
type holder int

const (
	heldByNone holder = iota
	heldByScan
	heldByEdit
	heldByPass // The fingerprint pass, which gives way to everyone else.
)

// libraryLock is the one lock over the library's files. A scan holds it from
// the walk to the last write, and an edit for all its files, because a walk
// during a rename-replace can miss a live file and a batch read before an edit
// can put old tags back. The fingerprint pass holds it for one batch at a time.
// Readers never take it: browsing and streaming go on through all three.
//
// It knows its holder so that an edit can wait out a batch of the pass, which
// is short, but never a scan or another edit; and the pass lets anyone waiting
// go first. Every wait ends with its context.
type libraryLock struct {
	mu      sync.Mutex
	holder  holder
	waiting int           // Scans and edits waiting for the lock.
	freed   chan struct{} // Closed, and replaced, at every unlock.
}

func newLibraryLock() *libraryLock { return &libraryLock{freed: make(chan struct{})} }

// tryLock takes the lock for h if it is free, and reports whether it did.
func (l *libraryLock) tryLock(h holder) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.holder != heldByNone {
		return false
	}
	l.holder = h
	return true
}

// lock waits for the lock and takes it for h. The pass waits, too, while a scan
// or an edit is waiting. A wait that ctx ends returns an error wrapping ctx.Err().
func (l *libraryLock) lock(ctx context.Context, h holder) error {
	return l.await(ctx, h, func(holder) bool { return true })
}

// lockUnlessBusy takes the lock for h, waiting only while the pass holds it. It
// reports false when a scan or an edit holds it.
func (l *libraryLock) lockUnlessBusy(ctx context.Context, h holder) (bool, error) {
	busy := false
	err := l.await(ctx, h, func(held holder) bool {
		busy = held != heldByPass
		return !busy
	})
	return !busy && err == nil, err
}

// await takes the lock for h once it is free, or gives up, with a nil error,
// as soon as wait says not to wait for its holder.
func (l *libraryLock) await(ctx context.Context, h holder, wait func(holder) bool) error {
	l.mu.Lock()
	if h != heldByPass {
		l.waiting++
		defer func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.waiting--
			l.wake() // A pass may be waiting for no one else to be.
		}()
	}
	for {
		free := l.holder == heldByNone && (h != heldByPass || l.waiting == 0)
		if free {
			l.holder = h
			l.mu.Unlock()
			return nil
		}
		if l.holder != heldByNone && !wait(l.holder) {
			l.mu.Unlock()
			return nil
		}
		freed := l.freed
		l.mu.Unlock()
		select {
		case <-freed:
		case <-ctx.Done():
			return fmt.Errorf("wait for the library: %w", ctx.Err())
		}
		l.mu.Lock()
	}
}

// waitFree waits, without taking the lock, until no one holds it.
func (l *libraryLock) waitFree(ctx context.Context) error {
	for {
		l.mu.Lock()
		if l.holder == heldByNone {
			l.mu.Unlock()
			return nil
		}
		freed := l.freed
		l.mu.Unlock()
		select {
		case <-freed:
		case <-ctx.Done():
			return fmt.Errorf("wait for the library: %w", ctx.Err())
		}
	}
}

func (l *libraryLock) unlock() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.holder = heldByNone
	l.wake()
}

// wake tells every waiter to look again. The caller holds mu.
func (l *libraryLock) wake() {
	close(l.freed)
	l.freed = make(chan struct{})
}

// exclusive runs fn holding the library lock. It returns [ErrBusy] without
// running fn when a scan or another edit holds the lock: an edit never queues
// behind a scan that may take minutes. It waits out a batch of the
// fingerprint pass, which takes a few seconds at most.
func (s *Scanner) exclusive(ctx context.Context, fn func() error) error {
	ok, err := s.lock.lockUnlessBusy(ctx, heldByEdit)
	if err != nil {
		return err
	}
	if !ok {
		return ErrBusy
	}
	defer s.lock.unlock()
	defer s.notifyLibrary(true) // An edit, whole or stopped midway, may have changed albums.
	return fn()
}
