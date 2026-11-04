package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

// ErrScanRunning means a scan was asked for while another was running.
var ErrScanRunning = errors.New("a scan is already running")

// Mode says which files a scan reads.
type Mode int

const (
	// Quick reads new and changed files.
	Quick Mode = iota
	// Full reads every file.
	Full
)

// Options configures [NewScanner].
type Options struct {
	Roots        []string
	MissingGrace time.Duration
	Workers      int              // Zero means runtime.NumCPU().
	Logger       *slog.Logger     // Nil discards.
	Now          func() time.Time // Nil means time.Now.
}

// Stats counts what a scan did.
type Stats struct {
	Seen, Read, Added, Updated, Moved, Missing, Issues int
	Deleted                                            int64 // Tracks past the grace period.
	Pruned                                             store.Pruned
	SkippedRoots                                       int
}

// Scanner keeps the store in step with the library roots. It is safe for
// concurrent use, but runs one scan at a time.
type Scanner struct {
	st        *store.Store
	roots     []string
	grace     time.Duration
	workers   int
	batchSize int // Tracks plus issues per write; tests lower it.
	log       *slog.Logger
	now       func() time.Time
	mu        sync.Mutex
}

// NewScanner returns a scanner that writes to st.
func NewScanner(st *store.Store, opts Options) *Scanner {
	s := &Scanner{
		st: st, roots: opts.Roots, grace: opts.MissingGrace, workers: opts.Workers,
		batchSize: defaultBatchSize, log: opts.Logger, now: opts.Now,
	}
	if s.workers <= 0 {
		s.workers = runtime.NumCPU()
	}
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

// Scan walks the roots, reads the files mode asks for and updates the store.
// It returns [ErrScanRunning] while another scan runs. A canceled ctx ends the
// scan with an error wrapping ctx.Err(); everything read by then is stored.
func (s *Scanner) Scan(ctx context.Context, mode Mode) (Stats, error) {
	if !s.mu.TryLock() {
		return Stats{}, ErrScanRunning
	}
	defer s.mu.Unlock()

	if len(s.roots) == 0 {
		// A lost config file must not start deleting the library.
		known, err := s.st.KnownRoots(ctx)
		if err != nil {
			return Stats{}, fmt.Errorf("scan: %w", err)
		}
		if len(known) > 0 {
			s.log.WarnContext(ctx, "no library roots configured; leaving the library untouched", "knownRoots", len(known))
		}
		return Stats{}, nil
	}
	sv, err := s.survey(ctx, mode)
	if err != nil {
		return Stats{}, fmt.Errorf("scan: survey: %w", err)
	}
	stats := Stats{Seen: sv.Seen, SkippedRoots: sv.SkippedRoots}
	if err = s.applySurvey(ctx, sv, &stats); err != nil {
		return stats, fmt.Errorf("scan: %w", err)
	}
	hold, err := s.read(ctx, sv.Jobs, &stats)
	if err != nil {
		return stats, fmt.Errorf("scan: %w", err)
	}
	if err = s.finish(ctx, sv.HoldDeletes || hold, &stats); err != nil {
		return stats, fmt.Errorf("scan: %w", err)
	}
	return stats, nil
}

// finish runs after a complete read: it deletes the tracks that have been
// missing past the grace period, unless hold says an outage may be hiding
// files, and prunes the albums and artists nothing uses.
func (s *Scanner) finish(ctx context.Context, hold bool, stats *Stats) error {
	if hold {
		s.log.WarnContext(ctx, "not deleting missing tracks: a library root or folder was unreadable or a missing track's file is unreadable")
	} else {
		n, err := s.st.DeleteMissingBefore(ctx, s.now().Add(-s.grace))
		if err != nil {
			return fmt.Errorf("delete missing: %w", err)
		}
		stats.Deleted = n
	}
	pruned, err := s.st.PruneOrphans(ctx)
	if err != nil {
		return fmt.Errorf("prune: %w", err)
	}
	stats.Pruned = pruned
	return nil
}
