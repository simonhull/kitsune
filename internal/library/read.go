package library

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/store"
)

const (
	// defaultBatchSize is how many tracks plus issues one write holds. It is small
	// so a client watching a first scan sees albums as they are found: at 50, a
	// scan of 7,586 tracks takes as long as at 500.
	defaultBatchSize = 50
	// retrySize is the size stored with an unreadable file's issue. It never
	// equals a real size, so the file is read again on every scan.
	retrySize = -1
)

// result is what a worker made of one job: a track write or an issue.
type result struct {
	job   *job
	track *store.TrackWrite
	issue *store.ScanIssue
}

// read reads jobs on a pool of workers, while the calling goroutine alone
// resolves identity and writes the results in batches. The bool is true when
// an unreadable file belongs to a missing track, which must not be deleted. A
// canceled ctx ends the read with an error wrapping ctx.Err() after the
// results already handled are written.
//
// Files are not fingerprinted, which would read all their audio, unless a
// missing track has a fingerprint: then a file with no track at its path is,
// since it may be that track moved. The survey has marked the missing tracks,
// so the store knows. The rest wait for the pass.
func (s *Scanner) read(ctx context.Context, jobs []job, stats *Stats) (bool, error) {
	if len(jobs) == 0 {
		return false, nil
	}
	hashNew, err := s.st.AnyMissingWithDigest(ctx)
	if err != nil {
		return false, err //nolint:wrapcheck // The store's error already says what failed.
	}
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	rd := &reader{covers: newCovers(s.artwork, s.log), hashNew: hashNew, version: s.reader}
	w := &writer{s: s, stats: stats, covers: rd.covers, claimed: claimedBy(jobs)}
	for r := range s.startWorkers(work, jobs, rd) {
		if w.step(ctx, work, &r) {
			cancel()
		}
	}
	stats.Covers = rd.covers.count()
	stats.Hashed = int(rd.hashed.Load())
	return w.hold, w.finish(ctx)
}

// claimedBy seeds the claimed set with every track a job already owns, so a
// missing file that reappears at its own path never loses its ID to a copy
// elsewhere.
func claimedBy(jobs []job) map[int64]struct{} {
	claimed := make(map[int64]struct{})
	for i := range jobs {
		if jobs[i].TrackID != 0 {
			claimed[jobs[i].TrackID] = struct{}{}
		}
	}
	return claimed
}

// startWorkers feeds jobs to the worker pool and returns the results. The
// channel closes once every worker has stopped, which happens when the jobs
// run out or work ends.
func (s *Scanner) startWorkers(work context.Context, jobs []job, rd *reader) <-chan result {
	queue := make(chan *job)
	results := make(chan result)
	go func() {
		defer close(queue)
		for i := range jobs {
			select {
			case queue <- &jobs[i]:
			case <-work.Done():
				return
			}
		}
	}()
	var wg sync.WaitGroup
	for range s.workers {
		wg.Go(func() {
			for j := range queue {
				r, ok := rd.readJob(work, j)
				if !ok {
					return
				}
				select {
				case results <- r:
				case <-work.Done():
					return
				}
			}
		})
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	return results
}

// reader reads files for the workers of one scan.
type reader struct {
	covers  *covers // Nil leaves album art out, so an album keeps the cover it has.
	hashNew bool    // Fingerprint the files that have no track at their path.
	version string  // The tag reader's, stamped on the issues it raises.
	hashed  atomic.Int64
}

// readJob opens and maps one file, fingerprinting it when the reader says. It reports false when the
// context ended: a canceled read must never become a scan issue.
func (rd *reader) readJob(ctx context.Context, j *job) (result, bool) {
	tw, err := rd.readFile(ctx, j)
	if err != nil {
		if ctx.Err() != nil {
			return result{}, false
		}
		is := issueFor(j, err, rd.version)
		return result{job: j, issue: &is}, true
	}
	return result{job: j, track: tw}, true
}

// readFile reads the file at j into a track write, with its album's cover when
// this file is the one to find it, and its fingerprint when it is new and the
// reader hashes new files. A file at a known path never is: its write keeps the
// stored fingerprint if the file is unchanged, and clears it if not. Its errors
// are stored verbatim as the scan issue's message, so they are not wrapped. Art
// never fails a file: only a canceled ctx is returned from the art search.
func (rd *reader) readFile(ctx context.Context, j *job) (*store.TrackWrite, error) {
	f, err := audiometa.OpenContext(ctx, filepath.Join(j.Root, j.RelPath), audiometa.WithMaxArtworkSize(maxArtworkBytes))
	if err != nil {
		return nil, err //nolint:wrapcheck // The message is stored as the scan issue verbatim.
	}
	// The file was only read: a close error after a good read changes nothing.
	defer func() { _ = f.Close() }()
	fi := FileInfo{Root: j.Root, RelPath: j.RelPath, Size: j.Size, MTime: j.MTime}
	if rd.hashNew && j.TrackID == 0 {
		digest, digestErr := f.AudioDigestContext(ctx)
		if digestErr != nil {
			return nil, digestErr //nolint:wrapcheck // The message is stored as the scan issue verbatim.
		}
		rd.hashed.Add(1)
		fi.Digest = &digest
	}
	tw := NewTrackWrite(f, &fi)
	if rd.covers == nil {
		return &tw, nil
	}
	if coverErr := rd.covers.attach(ctx, f, j, &tw); coverErr != nil {
		return nil, coverErr
	}
	return &tw, nil
}

// reindex reads track id's file again and stores the result in one batch, through
// the scan's read path. It never touches artwork. Nothing changes identity, so
// nothing needs pruning. The caller holds the library lock.
func (s *Scanner) reindex(ctx context.Context, id int64, root, relPath string) error {
	info, err := os.Stat(filepath.Join(root, relPath))
	if err != nil {
		return fmt.Errorf("reindex: %w", err)
	}
	j := job{Root: root, RelPath: relPath, Size: info.Size(), MTime: info.ModTime(), TrackID: id}
	tw, err := (&reader{}).readFile(ctx, &j)
	if err != nil {
		return fmt.Errorf("reindex: %w", err)
	}
	w := writer{s: s, stats: &Stats{}}
	if err = w.identify(ctx, tw, &j); err != nil {
		return fmt.Errorf("reindex: %w", err)
	}
	if err = s.st.WriteBatch(ctx, &store.Batch{Tracks: []store.TrackWrite{*tw}}); err != nil {
		return fmt.Errorf("reindex: %w", err)
	}
	return nil
}

// issueFor turns a failed read into a scan issue. Corrupt and unsupported
// files keep the job's size and mtime, and the version of the reader that
// judged them, so a scan leaves them alone until they change or another reader
// comes: a newer one may take them. Anything else may be a permissions slip or
// an I/O blip, so it gets [retrySize] and is tried again every scan.
func issueFor(j *job, err error, version string) store.ScanIssue {
	is := store.ScanIssue{
		Root: j.Root, RelPath: j.RelPath, Size: j.Size, MTime: j.MTime,
		Message: strings.ToValidUTF8(err.Error(), "�"), Reader: version,
	}
	_, corrupted := errors.AsType[*audiometa.CorruptedFileError](err)
	_, outOfBounds := errors.AsType[*audiometa.OutOfBoundsError](err)
	_, unsupported := errors.AsType[*audiometa.UnsupportedFormatError](err)
	switch {
	case corrupted || outOfBounds:
		is.Kind = store.IssueCorrupted
	case unsupported:
		is.Kind = store.IssueUnsupported
	default:
		is.Kind, is.Size = store.IssueUnreadable, retrySize
	}
	return is
}

// writer resolves identity and writes batches. Only the goroutine that calls
// read touches it.
type writer struct {
	s       *Scanner
	stats   *Stats
	covers  *covers
	claimed map[int64]struct{} // Track IDs taken by this scan, flushed or not.
	batch   store.Batch
	hold    bool
	err     error
}

// step handles one result unless the scan has failed or ctx has ended, when
// the remaining results are only drained. It reports whether the workers
// should stop. A failure caused by ctx ending is not recorded: the scan ends as
// canceled, with what was handled so far written.
func (w *writer) step(ctx, work context.Context, r *result) bool {
	if w.err != nil || ctx.Err() != nil {
		return false
	}
	err := w.handle(work, r)
	if err == nil {
		return false
	}
	if ctx.Err() == nil {
		w.err = err
	}
	return true
}

// finish writes what is left and returns how the read ended: with the
// failure, or with ctx's error when it was canceled.
func (w *writer) finish(ctx context.Context) error {
	if w.err != nil {
		return w.err
	}
	if err := w.flush(ctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("read: %w", err)
	}
	return nil
}

// handle counts one result, gives a track its identity and queues it, flushing
// a full batch.
func (w *writer) handle(ctx context.Context, r *result) error {
	w.stats.Read++
	w.s.publish(func(p *Progress) { p.Read = w.stats.Read })
	w.s.notifyScan(false)
	if r.issue != nil {
		w.stats.Issues++
		w.hold = w.hold || (r.job.Missing && r.issue.Kind == store.IssueUnreadable)
		w.batch.Issues = append(w.batch.Issues, *r.issue)
	} else {
		if err := w.identify(ctx, r.track, r.job); err != nil {
			return err
		}
		w.covers.settle(r.track)
		w.batch.Tracks = append(w.batch.Tracks, *r.track)
	}
	if len(w.batch.Tracks)+len(w.batch.Issues) >= w.s.batchSize {
		return w.flush(ctx)
	}
	return nil
}

// identify sets tw's ID or ULID. The row at the path wins; otherwise a single
// missing track with the same audio is taken over; otherwise it is new.
func (w *writer) identify(ctx context.Context, tw *store.TrackWrite, j *job) error {
	// First, because the update and move branches return early. The store keeps
	// the IDs of an album and artists that already exist and ignores these.
	albumID, err := newULID(w.s.now(), rand.Reader)
	if err != nil {
		return fmt.Errorf("new album ID for %s: %w", tw.RelPath, err)
	}
	tw.Album.ULID = albumID
	for _, list := range [][]store.ArtistWrite{tw.Artists, tw.AlbumArtists} {
		for i := range list {
			if list[i].ULID, err = newULID(w.s.now(), rand.Reader); err != nil {
				return fmt.Errorf("new artist ID for %s: %w", tw.RelPath, err)
			}
		}
	}
	if j.TrackID != 0 {
		tw.ID = j.TrackID
		w.stats.Updated++
		return nil
	}
	id, err := w.takeMissing(ctx, tw)
	if err != nil {
		return err
	}
	if id != 0 {
		tw.ID = id
		w.stats.Moved++
		return nil
	}
	if tw.ULID, err = newULID(w.s.now(), rand.Reader); err != nil {
		return fmt.Errorf("new track ID for %s: %w", tw.RelPath, err)
	}
	w.stats.Added++
	return nil
}

// takeMissing returns the ID of the missing track tw is a move of, claiming
// it, or 0 when there is none or tw has no fingerprint.
func (w *writer) takeMissing(ctx context.Context, tw *store.TrackWrite) (int64, error) {
	if tw.Digest == nil {
		return 0, nil // Only a fingerprint can say a file moved.
	}
	candidates, err := w.s.st.MissingByDigest(ctx, *tw.Digest)
	if err != nil {
		return 0, err //nolint:wrapcheck // The store's error already says what failed.
	}
	id := pickMissing(candidates, w.claimed, tw)
	if id != 0 {
		w.claimed[id] = struct{}{}
	}
	return id, nil
}

// pickMissing chooses among the unclaimed candidates: the only one; else the
// only one with tw's relative path, which keeps duplicates' IDs when a root is
// renamed; else the only one with tw's base name; else none (0).
func pickMissing(candidates []store.MissingTrack, claimed map[int64]struct{}, tw *store.TrackWrite) int64 {
	free := make([]store.MissingTrack, 0, len(candidates))
	for _, c := range candidates {
		if _, taken := claimed[c.ID]; !taken {
			free = append(free, c)
		}
	}
	if len(free) == 1 {
		return free[0].ID
	}
	if id, ok := only(free, func(c *store.MissingTrack) bool { return c.RelPath == tw.RelPath }); ok {
		return id
	}
	base := filepath.Base(tw.RelPath)
	id, _ := only(free, func(c *store.MissingTrack) bool { return filepath.Base(c.RelPath) == base })
	return id
}

// only returns the ID of the one candidate that matches. ok is false when
// none or several do.
func only(candidates []store.MissingTrack, match func(*store.MissingTrack) bool) (int64, bool) {
	var id int64
	found := false
	for i := range candidates {
		if !match(&candidates[i]) {
			continue
		}
		if found {
			return 0, false
		}
		id, found = candidates[i].ID, true
	}
	return id, found
}

// flush writes the batch. It ignores cancellation: those files were read in
// full, a batch takes tens of milliseconds, and the next quick scan picks up
// the rest.
func (w *writer) flush(ctx context.Context) error {
	if len(w.batch.Tracks)+len(w.batch.Issues) == 0 {
		return nil
	}
	if err := w.s.st.WriteBatch(context.WithoutCancel(ctx), &w.batch); err != nil {
		return err //nolint:wrapcheck // The store's error already says what failed.
	}
	clear(w.batch.Tracks)
	clear(w.batch.Issues)
	w.batch.Tracks, w.batch.Issues = w.batch.Tracks[:0], w.batch.Issues[:0]
	w.s.notifyLibrary(false)
	return nil
}
