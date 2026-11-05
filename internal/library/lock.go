package library

import (
	"context"
	"errors"
	"fmt"
)

// ErrBusy means an edit was asked for while a scan or another edit held the
// library.
var ErrBusy = errors.New("the library is busy")

// libraryLock is the one lock over the library's files. A scan holds it from
// the walk to the last write, and an edit for all its files, because a walk
// during a rename-replace can miss a live file and a batch read before an edit
// can put old tags back. Readers never take it: browsing and streaming go on
// through both.
//
// It is a channel so that a scan can stop waiting when its context ends.
type libraryLock chan struct{}

func newLibraryLock() libraryLock { return make(libraryLock, 1) }

// tryLock takes the lock if it is free, and reports whether it did.
func (l libraryLock) tryLock() bool {
	select {
	case l <- struct{}{}:
		return true
	default:
		return false
	}
}

// lock waits for the lock until ctx ends, returning an error wrapping ctx.Err().
func (l libraryLock) lock(ctx context.Context) error {
	select {
	case l <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for the library: %w", ctx.Err())
	}
}

func (l libraryLock) unlock() { <-l }

// exclusive runs fn holding the library lock. It returns [ErrBusy] without
// running fn when a scan or another edit holds the lock: an edit never queues
// behind a scan that may take minutes.
func (s *Scanner) exclusive(fn func() error) error {
	if !s.lock.tryLock() {
		return ErrBusy
	}
	defer s.lock.unlock()
	return fn()
}
