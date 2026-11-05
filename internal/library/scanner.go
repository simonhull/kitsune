package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/artwork"
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

// String returns the mode's lowercase name.
func (m Mode) String() string {
	if m == Full {
		return "full"
	}
	return "quick"
}

// ParseMode returns the mode with the name [Mode.String] gives it.
func ParseMode(name string) (Mode, bool) {
	switch name {
	case Quick.String():
		return Quick, true
	case Full.String():
		return Full, true
	}
	return Quick, false
}

// Options configures [NewScanner].
type Options struct {
	Roots        []string
	MissingGrace time.Duration
	Workers      int              // Zero means runtime.NumCPU().
	Logger       *slog.Logger     // Nil discards.
	Now          func() time.Time // Nil means time.Now.
	Artwork      *artwork.Cache   // Nil skips album art.
	TagWrites    bool             // Files may be rewritten. Off, a scan changes nothing under the roots.
	Write        WriteFunc        // Nil means audiometa.WriteContext.
}

// WriteFunc saves edit to the audio file at path, as [audiometa.WriteContext] does.
type WriteFunc func(ctx context.Context, path string, edit func(*audiometa.File) error) error

// Stats counts what a scan did.
type Stats struct {
	Seen, Read, Added, Updated, Moved, Missing, Issues int
	Deleted                                            int64 // Tracks past the grace period.
	Pruned                                             store.Pruned
	SkippedRoots                                       int
	Covers                                             int // Album covers stored: successful artwork.Cache.Put calls.
	Swept                                              int // Temp files left by an interrupted edit, removed.
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
	artwork   *artwork.Cache
	tagWrites bool
	write     WriteFunc
	claim     libraryLock // Held by the one scan, from its claim to its end.
	lock      libraryLock // Held by a running scan or edit.
	progress  atomic.Pointer[Progress]
}

// NewScanner returns a scanner that writes to st.
func NewScanner(st *store.Store, opts Options) *Scanner { //nolint:gocritic // hugeParam: Options is built once at startup; a value keeps every caller a plain literal.
	s := &Scanner{
		st: st, roots: opts.Roots, grace: opts.MissingGrace, workers: opts.Workers,
		batchSize: defaultBatchSize, log: opts.Logger, now: opts.Now, artwork: opts.Artwork,
		tagWrites: opts.TagWrites, write: opts.Write, claim: newLibraryLock(), lock: newLibraryLock(),
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
	if s.write == nil {
		s.write = func(ctx context.Context, path string, edit func(*audiometa.File) error) error {
			return audiometa.WriteContext(ctx, path, edit)
		}
	}
	s.progress.Store(&Progress{})
	return s
}

// Claim reserves the scanner for one scan without waiting, so a caller can
// refuse a request before starting anything. It returns [ErrScanRunning] while
// another scan holds the scanner. Otherwise the caller must call the returned
// func once, on any goroutine: it runs the scan as [Scanner.Scan] does, logs how
// it went and gives the scanner back. [Scanner.Progress] shows the scan from the
// moment Claim returns.
func (s *Scanner) Claim(mode Mode) (func(context.Context) (Stats, error), error) {
	if !s.claim.tryLock() {
		return nil, ErrScanRunning
	}
	return s.claimed(mode), nil
}

// Scan walks the roots, reads the files mode asks for and updates the store.
// It returns [ErrScanRunning] while another scan runs, and waits for an edit to
// finish. A canceled ctx ends the scan with an error wrapping ctx.Err();
// everything read by then is stored.
func (s *Scanner) Scan(ctx context.Context, mode Mode) (Stats, error) {
	run, err := s.Claim(mode)
	if err != nil {
		return Stats{}, err
	}
	return run(ctx)
}

// run is one scan, with the claim held, which it gives back.
func (s *Scanner) run(ctx context.Context, mode Mode) (Stats, error) {
	defer s.claim.unlock()
	start := time.Now()
	stats, err := s.exclusiveScan(ctx, mode)
	if ctxErr := ctx.Err(); err != nil && ctxErr != nil {
		err = fmt.Errorf("scan: %w", ctxErr) // A canceled query says only "interrupted".
	}
	s.logScan(ctx, mode, &stats, err, time.Since(start))
	s.ended(&stats, err)
	return stats, err
}

// exclusiveScan waits for the library lock and scans holding it.
func (s *Scanner) exclusiveScan(ctx context.Context, mode Mode) (Stats, error) {
	if err := s.lock.lock(ctx); err != nil {
		return Stats{}, fmt.Errorf("scan: %w", err)
	}
	defer s.lock.unlock()
	return s.scan(ctx, mode)
}

// scan is one scan, with the library lock held.
func (s *Scanner) scan(ctx context.Context, mode Mode) (Stats, error) {
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
	defer s.refreshNames(ctx)
	s.publish(func(p *Progress) { p.Total = len(sv.Jobs) })
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

// refreshNames brings search up to date after a scan, whatever its outcome,
// since a failed scan may have stored names too. A failure leaves search stale
// until the next scan, which costs less than failing this one.
func (s *Scanner) refreshNames(ctx context.Context) {
	if err := s.st.RefreshNames(ctx); err != nil && ctx.Err() == nil {
		s.log.WarnContext(ctx, "refreshing search names failed", "err", err)
	}
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
	s.deleteArtwork(ctx, pruned.Artwork)
	return nil
}

// deleteArtwork removes the renditions of images no album uses any more. A
// failure is logged: leftover files cost disk, never a scan.
func (s *Scanner) deleteArtwork(ctx context.Context, hashes [][32]byte) {
	if s.artwork == nil {
		return
	}
	for _, h := range hashes {
		if err := s.artwork.Delete(h); err != nil {
			s.log.WarnContext(ctx, "deleting a pruned cover failed", "err", err)
		}
	}
}
