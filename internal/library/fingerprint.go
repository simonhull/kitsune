package library

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/store"
)

// A batch of the fingerprint pass is this many files, or as many as it reads
// in about this long, whichever comes first. It holds the library lock for a
// batch, so an edit or a scan waits at most this long for it.
const (
	passBatchFiles = 32
	passBatchTime  = 2 * time.Second
)

// fingerprintFunc returns the fingerprint of the audio in the file at path, as
// [audiometa.File.AudioDigestContext] does.
type fingerprintFunc func(ctx context.Context, path string) ([32]byte, error)

// readFingerprint opens the file at path and fingerprints its audio.
func readFingerprint(ctx context.Context, path string) ([32]byte, error) {
	f, err := audiometa.OpenContext(ctx, path, audiometa.WithMaxArtworkSize(maxArtworkBytes))
	if err != nil {
		return [32]byte{}, fmt.Errorf("open: %w", err)
	}
	// The file was only read: a close error after a good read changes nothing.
	defer func() { _ = f.Close() }()
	digest, err := f.AudioDigestContext(ctx)
	if err != nil {
		return [32]byte{}, fmt.Errorf("fingerprint: %w", err)
	}
	return digest, nil
}

// FingerprintProgress is where the fingerprint pass stands: Done of the Total
// tracks it started with. Both are zero while no pass runs.
type FingerprintProgress struct{ Done, Total int }

// FingerprintStats counts what a fingerprint pass did with each track it read.
type FingerprintStats struct {
	Stored     int // Fingerprinted.
	Changed    int // The file changed or went since the scan; the next scan reads it again.
	Unreadable int // Skipped until the next pass.
}

// Fingerprints returns the fingerprint pass's progress. It never waits.
func (s *Scanner) Fingerprints() FingerprintProgress { return *s.fingerprints.Load() }

// askForPass asks [Scanner.RunFingerprints] for a pass, without waiting.
func (s *Scanner) askForPass() {
	select {
	case s.wantPass <- struct{}{}:
	default: // A pass is already asked for.
	}
}

// RunFingerprints runs the fingerprint pass at once, and again after every scan
// and edit, until ctx ends. A pass with nothing to do ends at once. A failed
// pass is logged, and the next one starts over.
func (s *Scanner) RunFingerprints(ctx context.Context) {
	s.askForPass()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wantPass:
		}
		_, _ = s.Fingerprint(ctx) // The pass logs how it went.
	}
}

// Fingerprint fills the fingerprints the scans left out, reading one file at a
// time in path order, so a disk reads mostly in sequence. It holds the library
// lock for one batch at a time and lets a waiting edit or scan go first, and it
// pauses while a scan is claimed. A file that can't be read is logged and
// skipped until the next pass; one that changed since the scan is left for the
// next scan. A canceled ctx ends the pass with an error wrapping ctx.Err(), with
// the fingerprints stored so far kept.
func (s *Scanner) Fingerprint(ctx context.Context) (FingerprintStats, error) {
	s.passing.Lock()
	defer s.passing.Unlock()
	var stats FingerprintStats
	total, err := s.st.CountUndigested(ctx)
	if err != nil {
		return stats, s.passFailed(ctx, err)
	}
	if total == 0 {
		return stats, nil
	}
	start := time.Now()
	s.fingerprints.Store(&FingerprintProgress{Total: total})
	defer s.fingerprints.Store(&FingerprintProgress{})
	var after store.FileKey
	for more := true; more; {
		if more, err = s.fingerprintBatch(ctx, &after, &stats); err != nil {
			return stats, s.passFailed(ctx, err)
		}
	}
	s.log.LogAttrs(ctx, slog.LevelInfo, "fingerprint pass finished", slog.Int("stored", stats.Stored),
		slog.Int("changed", stats.Changed), slog.Int("unreadable", stats.Unreadable),
		slog.Duration("duration", time.Since(start)))
	return stats, nil
}

// passFailed logs how a pass ended early and returns err wrapped, or ctx's
// error when that is why.
func (s *Scanner) passFailed(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		s.log.InfoContext(ctx, "fingerprint pass canceled")
		return fmt.Errorf("fingerprint: %w", ctxErr) // A canceled query says only "interrupted".
	}
	s.log.ErrorContext(ctx, "fingerprint pass failed", "err", err)
	return fmt.Errorf("fingerprint: %w", err)
}

// fingerprintBatch fingerprints the tracks after *after, for one batch, holding
// the library lock, and moves *after past them. It reports whether there may be
// more.
func (s *Scanner) fingerprintBatch(ctx context.Context, after *store.FileKey, stats *FingerprintStats) (bool, error) {
	if err := s.claim.waitFree(ctx); err != nil { // A scan, claimed or running, pauses the pass.
		return false, err
	}
	if err := s.lock.lock(ctx, heldByPass); err != nil {
		return false, err
	}
	defer s.lock.unlock()
	rows, err := s.st.Undigested(ctx, *after, passBatchFiles)
	if err != nil {
		return false, err //nolint:wrapcheck // The store's error already says what failed.
	}
	start := time.Now()
	for i := range rows {
		if i > 0 && time.Since(start) >= passBatchTime {
			break
		}
		if trackErr := s.fingerprintTrack(ctx, &rows[i], stats); trackErr != nil {
			return false, trackErr
		}
		*after = store.FileKey{Root: rows[i].Root, RelPath: rows[i].RelPath}
	}
	return len(rows) > 0, nil
}

// fingerprintTrack reads and stores one track's fingerprint, if its file is
// still the one the scan read: a stat after the read must match the row.
func (s *Scanner) fingerprintTrack(ctx context.Context, row *store.Undigested, stats *FingerprintStats) error {
	defer s.publishPass(stats)
	path := filepath.Join(row.Root, row.RelPath)
	digest, err := s.fingerprint(ctx, path)
	if err != nil {
		if ctx.Err() != nil {
			return err
		}
		stats.Unreadable++
		s.log.WarnContext(ctx, "a file could not be fingerprinted; the next pass tries again",
			"root", row.Root, "path", row.RelPath, "err", err)
		return nil
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != row.Size || info.ModTime().UnixMilli() != row.MTime.UnixMilli() {
		stats.Changed++
		return nil // A file that changed or went is the next scan's to read.
	}
	stored, err := s.st.SetDigest(ctx, row.ID, row.Size, row.MTime, digest)
	if err != nil {
		return err //nolint:wrapcheck // The store's error already says what failed.
	}
	if stored {
		stats.Stored++
	} else {
		stats.Changed++
	}
	return nil
}

// publishPass swaps in the pass's progress after a track. Only the pass, which
// holds passing, calls it. A scan between batches can add tracks the pass then
// reaches, so Total grows to keep Done within it.
func (s *Scanner) publishPass(stats *FingerprintStats) {
	next := s.Fingerprints()
	next.Done = stats.Stored + stats.Changed + stats.Unreadable
	next.Total = max(next.Total, next.Done)
	s.fingerprints.Store(&next)
}
