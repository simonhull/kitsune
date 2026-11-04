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
	runDaily(ctx, s.now, hour, minute, s.logScan, ready)
}

// logScan runs one quick scan and logs how it went.
func (s *Scanner) logScan(ctx context.Context) {
	start := time.Now()
	stats, err := s.Scan(ctx, Quick)
	elapsed := time.Since(start)
	switch {
	case err != nil && ctx.Err() != nil && errors.Is(err, ctx.Err()):
		s.log.InfoContext(ctx, "scan canceled", "duration", elapsed)
	case err != nil:
		s.log.ErrorContext(ctx, "scan failed", "err", err, "duration", elapsed)
	default:
		s.log.LogAttrs(ctx, slog.LevelInfo, "scan finished",
			slog.Int("seen", stats.Seen), slog.Int("read", stats.Read),
			slog.Int("added", stats.Added), slog.Int("updated", stats.Updated),
			slog.Int("moved", stats.Moved), slog.Int("missing", stats.Missing),
			slog.Int("issues", stats.Issues), slog.Int64("deleted", stats.Deleted),
			slog.Int64("prunedAlbums", stats.Pruned.Albums), slog.Int64("prunedArtists", stats.Pruned.Artists),
			slog.Int("prunedArtwork", len(stats.Pruned.Artwork)), slog.Int("covers", stats.Covers),
			slog.Int("skippedRoots", stats.SkippedRoots), slog.Duration("duration", elapsed))
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
