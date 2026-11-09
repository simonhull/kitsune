package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/simonhull/kitsune/internal/loudness"
	"github.com/simonhull/kitsune/internal/pcm"
	"github.com/simonhull/kitsune/internal/store"
)

// loudnessVersion is the version of the loudness measurement. Raising it
// measures every file again: do so when a change to [loudness] or [pcm] moves
// what they read.
const loudnessVersion = 1

// errYield ends a measurement midway because a scan or an edit is waiting for
// the library. The file waits for the next batch.
var errYield = errors.New("giving way to a scan or an edit")

// measureFunc measures the file of a waiting track. It returns an error
// wrapping [pcm.ErrUnsupported] for audio nothing here decodes, and
// [pcm.ErrCorrupt] for broken audio; yield is asked between chunks, and true
// ends the measurement with [errYield].
type measureFunc func(ctx context.Context, row *store.LoudnessWaiting, yield func() bool) (loudness.Result, error)

// pcmKind is the decoder for a track's format and codec, and false for one
// nothing here measures yet: AAC, ALAC, Opus, Vorbis and compressed WAV or
// AIFF.
func pcmKind(format, codec string) (pcm.Kind, bool) {
	uncompressed := codec == "PCM" || codec == "IEEE float"
	switch {
	case format == "FLAC":
		return pcm.FLAC, true
	case format == "MP3":
		return pcm.MP3, true
	case format == "WAV" && uncompressed:
		return pcm.WAV, true
	case format == "AIFF" && uncompressed:
		return pcm.AIFF, true
	}
	return 0, false
}

// measureFile decodes and measures the file of row, read under its root.
func measureFile(ctx context.Context, row *store.LoudnessWaiting, yield func() bool) (loudness.Result, error) {
	kind, ok := pcmKind(row.Format, row.Codec)
	if !ok {
		return loudness.Result{}, fmt.Errorf("%w: %s %s", pcm.ErrUnsupported, row.Format, row.Codec)
	}
	f, err := os.OpenInRoot(row.Root, row.RelPath)
	if err != nil {
		return loudness.Result{}, fmt.Errorf("open: %w", err)
	}
	// The file was only read: a close error after a good read changes nothing.
	defer func() { _ = f.Close() }()
	stream, err := pcm.Open(f, kind, row.Channels)
	if err != nil {
		return loudness.Result{}, fmt.Errorf("decode: %w", err)
	}
	return meter(ctx, stream, yield)
}

// meter measures the rest of stream, asking yield between chunks.
func meter(ctx context.Context, stream *pcm.Stream, yield func() bool) (loudness.Result, error) {
	m, err := loudness.NewMeter(stream.Rate, stream.Channels)
	if err != nil {
		return loudness.Result{}, fmt.Errorf("%w: %w", pcm.ErrUnsupported, err)
	}
	for {
		if ctx.Err() != nil {
			return loudness.Result{}, fmt.Errorf("measure: %w", ctx.Err())
		}
		if yield() {
			return loudness.Result{}, errYield
		}
		planes, readErr := stream.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return loudness.Result{}, fmt.Errorf("decode: %w", readErr)
		}
		if err = m.Push(planes...); err != nil {
			return loudness.Result{}, fmt.Errorf("%w: %w", pcm.ErrCorrupt, err)
		}
	}
	r, err := m.Finish()
	if err != nil {
		return loudness.Result{}, fmt.Errorf("measure: %w", err)
	}
	return r, nil
}

// LoudnessProgress is where the loudness pass stands, Done of the Total files
// it started with, both zero while no pass runs, and what the library holds:
// the songs with no ReplayGain and how many of them are measured, or can't be.
type LoudnessProgress struct {
	Done, Total int
	Counts      store.LoudnessCounts
}

// LoudnessStats counts what a loudness pass did with each file it read.
type LoudnessStats struct {
	Measured     int // Measured and stored.
	Unmeasurable int // A format nothing measures yet, or broken audio: kept until the file changes.
	Changed      int // The file changed or went since the scan; the next scan reads it again.
	Unreadable   int // Couldn't be read; the next pass tries again.
	Albums       int // Albums measured whole.
}

// files is how many files the pass has finished with.
func (st *LoudnessStats) files() int {
	return st.Measured + st.Unmeasurable + st.Changed + st.Unreadable
}

// Loudness returns the loudness pass's progress. It never waits.
func (s *Scanner) Loudness() LoudnessProgress { return *s.loudness.Load() }

// RunLoudness runs the loudness pass at once, and again after every scan and
// edit, until ctx ends. A pass with nothing to do ends at once. A failed pass
// is logged, and the next one starts over.
func (s *Scanner) RunLoudness(ctx context.Context) {
	signal(s.wantLoudness)
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wantLoudness:
		}
		_, _ = s.MeasureLoudness(ctx) // The pass logs how it went.
	}
}

// MeasureLoudness measures the files of the albums with a song that has no
// ReplayGain, one file at a time in path order, and then each album whole,
// from its files' gated blocks pooled. Files are measured only when their
// size, mtime or the measurement version changed; nothing is ever written to
// them. It goes after the fingerprint pass, holds the library lock for one
// batch at a time, and gives way to a scan or an edit, even midway through a
// file. A canceled ctx ends the pass with an error wrapping ctx.Err(), with
// what was measured so far kept.
func (s *Scanner) MeasureLoudness(ctx context.Context) (LoudnessStats, error) {
	s.measuring.Lock()
	defer s.measuring.Unlock()
	var stats LoudnessStats
	s.refreshLoudnessCounts(ctx)
	total, err := s.st.CountLoudnessWaiting(ctx, loudnessVersion)
	if err != nil {
		return stats, s.loudnessFailed(ctx, err)
	}
	start := time.Now()
	s.publishLoudness(&LoudnessProgress{Total: total}, true)
	defer s.publishLoudness(&LoudnessProgress{}, true)
	var after store.FileKey
	for more := total > 0; more; {
		if more, err = s.loudnessBatch(ctx, &after, &stats); err != nil {
			return stats, s.loudnessFailed(ctx, err)
		}
	}
	if err = s.poolWaitingAlbums(ctx, &stats); err != nil {
		return stats, s.loudnessFailed(ctx, err)
	}
	if total > 0 || stats.Albums > 0 {
		s.log.LogAttrs(ctx, slog.LevelInfo, "loudness pass finished", slog.Int("measured", stats.Measured),
			slog.Int("unmeasurable", stats.Unmeasurable), slog.Int("changed", stats.Changed),
			slog.Int("unreadable", stats.Unreadable), slog.Int("albums", stats.Albums),
			slog.Duration("duration", time.Since(start)))
	}
	return stats, nil
}

// loudnessFailed logs how a pass ended early and returns err wrapped, or ctx's
// error when that is why.
func (s *Scanner) loudnessFailed(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		s.log.InfoContext(ctx, "loudness pass canceled")
		return fmt.Errorf("loudness: %w", ctxErr) // A canceled query says only "interrupted".
	}
	s.log.ErrorContext(ctx, "loudness pass failed", "err", err)
	return fmt.Errorf("loudness: %w", err)
}

// holdForPass waits for the fingerprint pass to finish, for no scan to be
// claimed, and then for the library lock. The caller must call the returned
// func to let them go.
func (s *Scanner) holdForPass(ctx context.Context) (func(), error) {
	s.passing.Lock() // The fingerprint pass goes first: it keeps moved files their tracks.
	if err := s.claim.waitFree(ctx); err != nil {
		s.passing.Unlock()
		return nil, err
	}
	if err := s.lock.lock(ctx, heldByPass); err != nil {
		s.passing.Unlock()
		return nil, err
	}
	return func() {
		s.lock.unlock()
		s.passing.Unlock()
	}, nil
}

// loudnessBatch measures the files after *after, for one batch, holding the
// library lock, and moves *after past them. Then it measures each album whose
// last file it measured. It reports whether there may be more.
func (s *Scanner) loudnessBatch(ctx context.Context, after *store.FileKey, stats *LoudnessStats) (bool, error) {
	release, err := s.holdForPass(ctx)
	if err != nil {
		return false, err
	}
	defer release()
	rows, err := s.st.LoudnessWaiting(ctx, *after, loudnessVersion, passBatchFiles)
	if err != nil {
		return false, err //nolint:wrapcheck // The store's error already says what failed.
	}
	touched, err := s.measureRows(ctx, rows, after, stats)
	if err != nil {
		return false, err
	}
	for album := range touched {
		if poolErr := s.poolAlbum(ctx, album, stats); poolErr != nil {
			return false, poolErr
		}
	}
	s.publishLoudnessCounts(ctx, stats, false)
	return len(rows) > 0, nil
}

// measureRows measures rows in turn, for as long as a batch lasts and no one
// waits for the library, moving *after past each. It returns the albums of the
// files it stored a measure of.
func (s *Scanner) measureRows(ctx context.Context, rows []store.LoudnessWaiting, after *store.FileKey, stats *LoudnessStats) (map[int64]struct{}, error) {
	start := time.Now()
	touched := make(map[int64]struct{})
	for i := range rows {
		if i > 0 && (time.Since(start) >= passBatchTime || s.lock.contended()) {
			break
		}
		measured, err := s.measureTrack(ctx, &rows[i], stats)
		if errors.Is(err, errYield) {
			break // The file waits for the next batch.
		}
		if err != nil {
			return nil, err
		}
		if measured {
			touched[rows[i].AlbumID] = struct{}{}
		}
		*after = store.FileKey{Root: rows[i].Root, RelPath: rows[i].RelPath}
	}
	return touched, nil
}

// measureTrack measures and stores one file, if it is still the one the scan
// read: a stat after the read must match the row. It reports whether it
// stored a row.
func (s *Scanner) measureTrack(ctx context.Context, row *store.LoudnessWaiting, stats *LoudnessStats) (bool, error) {
	r, err := s.measure(ctx, row, s.lock.contended)
	if errors.Is(err, errYield) {
		return false, err
	}
	if ctx.Err() != nil {
		return false, fmt.Errorf("measure: %w", ctx.Err())
	}
	m, keep, err := s.outcome(ctx, row, &r, err)
	if !keep {
		stats.Unreadable++
		return false, err
	}
	if !stillRead(row) {
		stats.Changed++
		return false, nil // A file that changed or went is the next scan's to read.
	}
	stored, err := s.st.SetTrackLoudness(ctx, row.ID, row.Size, row.MTime, loudnessVersion, &m)
	if err != nil {
		return false, err //nolint:wrapcheck // The store's error already says what failed.
	}
	switch {
	case !stored:
		stats.Changed++
	case m.Outcome == store.LoudnessMeasured:
		stats.Measured++
	default:
		stats.Unmeasurable++
	}
	s.publishLoudnessCounts(ctx, stats, false)
	return stored, nil
}

// outcome is what to store of a measurement that returned r and err. It
// reports false for a file to try again next pass, which it logs: one that
// couldn't be read, as opposed to one nothing can measure.
func (s *Scanner) outcome(ctx context.Context, row *store.LoudnessWaiting, r *loudness.Result, err error) (store.TrackLoudness, bool, error) {
	switch {
	case errors.Is(err, pcm.ErrUnsupported):
		return store.TrackLoudness{Outcome: store.LoudnessUnsupported}, true, nil
	case errors.Is(err, pcm.ErrCorrupt):
		s.log.DebugContext(ctx, "a file's audio can't be measured", "root", row.Root, "path", row.RelPath, "err", err)
		return store.TrackLoudness{Outcome: store.LoudnessUnreadable}, true, nil
	case err != nil:
		s.log.WarnContext(ctx, "a file could not be measured; the next pass tries again",
			"root", row.Root, "path", row.RelPath, "err", err)
		return store.TrackLoudness{}, false, nil
	}
	blocks, err := r.Blocks.MarshalBinary()
	if err != nil {
		return store.TrackLoudness{}, false, fmt.Errorf("measure: %w", err)
	}
	return store.TrackLoudness{
		Outcome: store.LoudnessMeasured, Integrated: r.Integrated, Silent: r.Silent,
		TruePeak: r.TruePeak, SamplePeak: r.SamplePeak, Blocks: blocks,
	}, true, nil
}

// stillRead reports whether row's file still has the size and mtime the scan
// read.
func stillRead(row *store.LoudnessWaiting) bool {
	info, err := os.Stat(filepath.Join(row.Root, row.RelPath))
	return err == nil && info.Size() == row.Size && info.ModTime().UnixMilli() == row.MTime.UnixMilli()
}

// poolWaitingAlbums measures every album that is ready to be and isn't: one
// whose last file another pass measured, or whose tracks changed without a
// file to measure. Each album is one transaction, which reads no file, so it
// needs no library lock: an album a scan changes after it is pooled again.
func (s *Scanner) poolWaitingAlbums(ctx context.Context, stats *LoudnessStats) error {
	if err := s.claim.waitFree(ctx); err != nil {
		return err
	}
	albums, err := s.st.AlbumsToPool(ctx)
	if err != nil {
		return err //nolint:wrapcheck // The store's error already says what failed.
	}
	for _, album := range albums {
		if poolErr := s.poolAlbum(ctx, album, stats); poolErr != nil {
			return poolErr
		}
	}
	s.publishLoudnessCounts(ctx, stats, true)
	return nil
}

// poolAlbum measures album from its tracks' measures, once every one has one.
func (s *Scanner) poolAlbum(ctx context.Context, album int64, stats *LoudnessStats) error {
	pooled, err := s.st.PoolAlbum(ctx, album, poolTracks)
	if err != nil {
		return err //nolint:wrapcheck // The store's error already says what failed.
	}
	if pooled {
		stats.Albums++
	}
	return nil
}

// poolTracks is an album's loudness, from its tracks' gated blocks pooled, and
// its true peak, the largest of theirs.
func poolTracks(tracks []store.PooledTrack) (store.AlbumLoudness, error) {
	histograms := make([]*loudness.Histogram, len(tracks))
	var album store.AlbumLoudness
	for i := range tracks {
		histograms[i] = new(loudness.Histogram)
		if err := histograms[i].UnmarshalBinary(tracks[i].Blocks); err != nil {
			return store.AlbumLoudness{}, fmt.Errorf("pool: %w", err)
		}
		album.TruePeak = max(album.TruePeak, tracks[i].TruePeak)
	}
	album.Integrated, album.Silent = loudness.Pool(histograms...)
	return album, nil
}

// publishLoudness swaps in the pass's progress and tells the notifier, at
// most once a second unless force is set.
func (s *Scanner) publishLoudness(p *LoudnessProgress, force bool) {
	next := *p
	next.Counts = s.Loudness().Counts
	s.loudness.Store(&next)
	if s.loudnessNote.due(force) {
		s.notify.LoudnessChanged(s.Loudness())
	}
}

// publishLoudnessCounts swaps in the pass's progress after a file, with the
// library's counts read again when the notifier is due. Only the pass, which
// holds measuring, calls it. A scan between batches can add files the pass
// then reaches, so Total grows to keep Done within it.
func (s *Scanner) publishLoudnessCounts(ctx context.Context, stats *LoudnessStats, force bool) {
	next := s.Loudness()
	next.Done = stats.files()
	next.Total = max(next.Total, next.Done)
	if !s.loudnessNote.due(force) {
		s.loudness.Store(&next)
		return
	}
	if counts, err := s.st.CountLoudness(ctx); err == nil {
		next.Counts = counts
	} else if ctx.Err() == nil {
		s.log.WarnContext(ctx, "counting measured songs failed", "err", err)
	}
	s.loudness.Store(&next)
	s.notify.LoudnessChanged(next)
}

// refreshLoudnessCounts reads the library's counts as a pass starts: the pass
// keeps them after that.
func (s *Scanner) refreshLoudnessCounts(ctx context.Context) {
	counts, err := s.st.CountLoudness(ctx)
	if err != nil {
		if ctx.Err() == nil {
			s.log.WarnContext(ctx, "counting measured songs failed", "err", err)
		}
		return
	}
	next := s.Loudness()
	next.Counts = counts
	s.loudness.Store(&next)
}
