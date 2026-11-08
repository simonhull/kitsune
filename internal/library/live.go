package library

import (
	"context"
	"errors"
	"slices"
	"time"
)

// ErrTagWritesOff means an edit was asked for while tag writes are off.
var ErrTagWritesOff = errors.New("tag writes are off")

// Live is what an administrator may change while the scanner runs.
type Live struct {
	// Roots are the absolute library folders, none inside another.
	Roots []string
	// MissingGrace is how long a vanished file keeps its track.
	MissingGrace time.Duration
	// TagWrites lets an edit rewrite files under Roots. Off, nothing under
	// them ever changes.
	TagWrites bool
}

// Daily is the local time of the day's quick scan.
type Daily struct {
	Hour, Minute int
}

// live returns what the scanner runs with now. A scan and an edit read it under
// the library lock, which [Scanner.Configure] takes, so neither sees it change.
func (s *Scanner) live() *Live { return s.cfg.Load() }

// Roots returns the library roots the scanner uses now. Don't change the slice.
func (s *Scanner) Roots() []string { return s.live().Roots }

// TagWrites reports whether edits may rewrite files now.
func (s *Scanner) TagWrites() bool { return s.live().TagWrites }

// Configure makes l what the scanner runs with. It waits until no scan, edit or
// fingerprint batch holds the library, so a scan never changes roots or grace
// midway and an edit in progress finishes under the rules it began with: tag
// writes turned off take effect after it. New roots are scanned at the next
// scan. The outage rules stay: a root removed here is a removed root, whose
// tracks go missing only when every root left was walked and had audio. A wait
// that ctx ends returns an error wrapping ctx.Err() and changes nothing.
func (s *Scanner) Configure(ctx context.Context, l Live) error {
	l.Roots = slices.Clone(l.Roots)
	if err := s.lock.lock(ctx, heldByEdit); err != nil {
		return err
	}
	defer s.lock.unlock()
	s.cfg.Store(&l)
	return nil
}

// SetDaily moves the day's quick scan to d. The schedule waits for the new time
// from now; a scan running now goes on.
func (s *Scanner) SetDaily(d Daily) {
	s.daily.Store(&d)
	select {
	case s.redaily <- struct{}{}:
	default: // A move is already pending; it reads the latest time.
	}
}
