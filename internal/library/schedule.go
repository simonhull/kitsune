package library

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// Run scans [Quick] at once, calls ready when that scan ends, whatever its
// outcome, then scans again every day at hour:minute local time, until ctx
// ends. A failed scan is logged and does not stop the schedule: a broken scan
// should not take down a library that plays fine.
func (s *Scanner) Run(ctx context.Context, hour, minute int, ready func()) {
	runDaily(ctx, s.now, hour, minute, s.scheduled, ready)
}

// scheduled runs one quick scan. One that meets a running scan retries once,
// when that scan ends, so a long Full scan never costs the day's scan.
func (s *Scanner) scheduled(ctx context.Context) {
	_, err := s.Scan(ctx, Quick)
	if !errors.Is(err, ErrScanRunning) {
		return // The scan has logged how it went.
	}
	s.log.InfoContext(ctx, "a scan is running; the scheduled scan will retry when it ends")
	if s.claim.lock(ctx, heldByScan) != nil {
		return
	}
	_, _ = s.claimed(Quick)(ctx) // As above.
}

// logScan logs how a scan went.
func (s *Scanner) logScan(ctx context.Context, mode Mode, stats *Stats, err error, elapsed time.Duration) {
	switch {
	case err != nil && ctx.Err() != nil && errors.Is(err, ctx.Err()):
		s.log.InfoContext(ctx, "scan canceled", "mode", mode.String(), "duration", elapsed)
	case err != nil:
		s.log.ErrorContext(ctx, "scan failed", "mode", mode.String(), "err", err, "duration", elapsed)
	default:
		s.log.LogAttrs(ctx, slog.LevelInfo, "scan finished", slog.String("mode", mode.String()),
			slog.Int("seen", stats.Seen), slog.Int("read", stats.Read),
			slog.Int("added", stats.Added), slog.Int("updated", stats.Updated),
			slog.Int("moved", stats.Moved), slog.Int("missing", stats.Missing),
			slog.Int("issues", stats.Issues), slog.Int64("deleted", stats.Deleted),
			slog.Int64("prunedAlbums", stats.Pruned.Albums), slog.Int64("prunedArtists", stats.Pruned.Artists),
			slog.Int("prunedArtwork", len(stats.Pruned.Artwork)), slog.Int("covers", stats.Covers),
			slog.Int("skippedRoots", stats.SkippedRoots), slog.Int("swept", stats.Swept), slog.Int("hashed", stats.Hashed),
			slog.Duration("duration", elapsed))
	}
}

// runDaily calls scan, then ready, then scan again at every hour:minute until
// ctx ends.
func runDaily(ctx context.Context, now func() time.Time, hour, minute int, scan func(context.Context), ready func()) {
	scan(ctx)
	ready()
	for {
		at := now()
		timer := time.NewTimer(nextDaily(at, hour, minute).Sub(at))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			scan(ctx)
		}
	}
}

// nextDaily returns the next hour:minute in now's location after now: today if
// that is still ahead, otherwise tomorrow. time.Date handles daylight saving,
// moving a time that does not exist across the gap.
func nextDaily(now time.Time, hour, minute int) time.Time {
	y, m, d := now.Date()
	next := time.Date(y, m, d, hour, minute, 0, 0, now.Location())
	if next.After(now) {
		return next
	}
	return time.Date(y, m, d+1, hour, minute, 0, 0, now.Location())
}
