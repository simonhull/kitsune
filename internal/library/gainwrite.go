package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/store"
)

// A ReplayGain run writes what Kitsune measured into the files of songs that
// carry no ReplayGain, so every player levels them as Kitsune plays them. It
// follows the organizer's run: it holds the claim from start to end, so no scan
// starts, and the library lock one batch at a time (one album, at most
// [gainBatchSongs] songs), as a background pass does, so an edit or a scan
// waits out at most one batch. For each batch it
//
//  1. checks each song is still as the store has it, with no ReplayGain, its
//     measure and its album's pool current, and its file as the scan found it,
//     and reads the ReplayGain tags the file holds (its prior);
//  2. journals the batch, planned, in one transaction;
//  3. writes each file through audiometa, in place where the tags fit and no
//     stream has the file open, else by the atomic rewrite;
//  4. reads each file back into the store, keeping its fingerprint and its
//     loudness measure, since only its tags changed; and
//  5. settles the journal, written or skipped with why, in one transaction.
//
// A server stopped anywhere in that leaves planned rows, which
// [Scanner.RecoverGain] settles at the next start.

const (
	// gainBatchSongs is the most songs one batch writes.
	gainBatchSongs = 64
	// gainKeep and gainKeepFor are how many runs, and how long, the journal
	// keeps for Undo: whichever keeps more.
	gainKeep    = 10
	gainKeepFor = 90 * 24 * time.Hour
	// gainEvery is how often a run tells its progress, at most.
	gainEvery = 250 * time.Millisecond
)

// ErrNothingToWrite means a ReplayGain run was asked for albums with nothing
// Kitsune could write now.
var ErrNothingToWrite = errors.New("nothing to write")

// GainReason is why a run left a song as it was.
type GainReason string

// Why a run leaves a song as it was.
const (
	GainChanged     GainReason = "changed"     // The file isn't as the scan found it, or another tool put ReplayGain in it.
	GainUnwritable  GainReason = "unwritable"  // The file can't be written.
	GainStale       GainReason = "stale"       // Its measure, or its album's pool, is out of date.
	GainTrashed     GainReason = "trashed"     // In the Trash.
	GainMissing     GainReason = "missing"     // The file is gone.
	GainRetagged    GainReason = "retagged"    // An undo's: its tags no longer read what the run wrote.
	GainInterrupted GainReason = "interrupted" // The server stopped before it was written.
	GainFailed      GainReason = "failed"      // Anything else; logged.
)

// OpenFiles says whether a stream has a track's file open now. A run writes a
// file in place only when none has: a stream holding the file keeps reading
// the old one through an atomic rewrite.
type OpenFiles interface {
	Open(path string) bool
}

// GainProgress is a run's progress, as its events carry it.
type GainProgress struct {
	Run   string // Its public ID.
	State string // store.GainRunning, Done, Stopped or Undone.
	// Done counts the songs dealt with, written or not, of Total.
	Done, Total int
	// AlbumsDone counts the batches finished, of the run's albums.
	AlbumsDone int
	Album      string // The album in hand, by public ID.
	Stopping   bool
}

// GainRun is a run that was started: its public ID.
type GainRun struct {
	ID string
}

// GainPlanAlbum is one album of a running run, in the order it takes them.
type GainPlanAlbum struct {
	ID    string
	Songs int
}

// gainItem is one song a batch writes, or an undo puts back.
type gainItem struct {
	track     int64
	album     string // By public ID.
	ord       int    // Its place in the run.
	root, rel string
	size      int64 // What the store says the file is, for a run; 0 for an undo, which reads it then.
	mtime     int64 // Unix ms.
	values    store.GainValues
	prior     string // For an undo: the tags to put back, as journaled.
	undoes    int64  // For an undo: the journal row it reverses.
}

// gainBatch is one album's songs, or a part of a big album's.
type gainBatch struct {
	album string
	items []gainItem
}

// gainTask is a run under way.
type gainTask struct {
	id         int64
	ulid, kind string
	undoOf     int64
	undoOfULID string
	batches    []gainBatch
	albums     []GainPlanAlbum
	total      int

	release func() // Gives the claim back, once.

	stop     atomic.Bool
	mu       sync.Mutex
	progress GainProgress
}

// snapshot returns the task's progress.
func (t *gainTask) snapshot() GainProgress {
	t.mu.Lock()
	defer t.mu.Unlock()
	p := t.progress
	p.Stopping = t.stop.Load()
	return p
}

// update changes the task's progress.
func (t *gainTask) update(fn func(p *GainProgress)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	fn(&t.progress)
}

// roots returns the roots t would touch, each once.
func (t *gainTask) roots() []string {
	var out []string
	for i := range t.batches {
		for j := range t.batches[i].items {
			if root := t.batches[i].items[j].root; !slices.Contains(out, root) {
				out = append(out, root)
			}
		}
	}
	return out
}

// The steps of a run its fault hook hears.
const (
	gainStepJournaled = "journaled" // A batch is journaled; no file written.
	gainStepWritten   = "written"   // A file is written, and not read back.
	gainStepReindexed = "reindexed" // A file is read back; the batch isn't settled.
	gainStepBatch     = "batch"     // A batch is settled.
)

// gainFaulted tells the fault hook a run reached step, at rel. A hook that
// panics stops the run there, as a crash would.
func (s *Scanner) gainFaulted(step, rel string) {
	if s.gainFault != nil {
		s.gainFault(step, rel)
	}
}

// GainLibrary returns what a run may write now, and what waits.
func (s *Scanner) GainLibrary(ctx context.Context) (*store.GainLibrary, error) {
	lib, err := s.st.GainLibrary(ctx, loudnessVersion)
	if err != nil {
		return nil, fmt.Errorf("gain library: %w", err)
	}
	return lib, nil
}

// GainProgress returns the progress of the run going now, or nil.
func (s *Scanner) GainProgress() *GainProgress {
	t := s.gaining.Load()
	if t == nil {
		return nil
	}
	p := t.snapshot()
	return &p
}

// GainPlan returns the albums of the run with the public ID id, in the order
// it takes them, while it runs, and nil otherwise.
func (s *Scanner) GainPlan(id string) []GainPlanAlbum {
	t := s.gaining.Load()
	if t == nil || t.ulid != id {
		return nil
	}
	return slices.Clone(t.albums)
}

// StopGain asks the run with the public ID id to stop once its batch in hand
// is settled. It returns [ErrNotRunning] when that run isn't going.
func (s *Scanner) StopGain(id string) error {
	t := s.gaining.Load()
	if t == nil || t.ulid != id {
		return ErrNotRunning
	}
	t.stop.Store(true)
	s.notifyGain(t, true)
	return nil
}

// StartGain claims the scanner for a run that writes Kitsune's measure into
// the files of the albums with the public IDs albums (none: every album it
// can), by the user by, and returns the run and the func that runs it. The
// caller must call that func once, on any goroutine, as the app's background
// work: it gives the claim back when the run ends. A canceled ctx ends the run
// after the batch in hand, as Stop does.
//
// It returns [ErrNothingToWrite] when nothing of what was asked can be written
// now; [ErrTagWritesOff] while tag writes are off; [ErrBusy] while a scan, an
// edit or another run holds the library; and [ErrRootDown] when a root the run
// would touch looks down.
func (s *Scanner) StartGain(ctx context.Context, albums []string, by int64) (GainRun, func(context.Context), error) {
	lib, err := s.GainLibrary(ctx)
	if err != nil {
		return GainRun{}, nil, err
	}
	t := &gainTask{kind: store.GainKindRun, batches: gainBatches(lib, albums, s.live().Roots)}
	if len(t.batches) == 0 {
		return GainRun{}, nil, ErrNothingToWrite
	}
	return s.startGainTask(ctx, t, &store.GainRunStart{Kind: store.GainKindRun, By: by})
}

// gainBatches cuts the songs of lib's albums that ids names (none: all) under
// a configured root into batches, album by album.
func gainBatches(lib *store.GainLibrary, ids, roots []string) []gainBatch {
	want := setOf(ids)
	var batches []gainBatch
	ord := 0
	for i := range lib.Albums {
		a := &lib.Albums[i]
		if len(ids) > 0 && !want[a.ID] {
			continue
		}
		for j := range a.Songs {
			song := &a.Songs[j]
			if !slices.Contains(roots, song.Root) {
				continue // A root gone from the settings: the next scan marks its songs missing.
			}
			last := len(batches) - 1
			if last < 0 || batches[last].album != a.ID || len(batches[last].items) == gainBatchSongs {
				batches = append(batches, gainBatch{album: a.ID})
				last++
			}
			ord++
			batches[last].items = append(batches[last].items, gainItem{
				track: song.ID, album: a.ID, ord: ord, root: song.Root, rel: song.RelPath,
				size: song.Size, mtime: song.MTime.UnixMilli(), values: gainValuesOf(&song.Measure),
			})
		}
	}
	return batches
}

// planAlbums lists t's albums, each once, in the order it takes them.
func (t *gainTask) planAlbums() []GainPlanAlbum {
	var out []GainPlanAlbum
	for i := range t.batches {
		b := &t.batches[i]
		if n := len(out); n > 0 && out[n-1].ID == b.album {
			out[n-1].Songs += len(b.items)
			continue
		}
		out = append(out, GainPlanAlbum{ID: b.album, Songs: len(b.items)})
	}
	return out
}

// startGainTask claims the scanner for t, records it and returns its run func.
func (s *Scanner) startGainTask(ctx context.Context, t *gainTask, start *store.GainRunStart) (GainRun, func(context.Context), error) {
	release, err := s.claimRun(ctx, "writing ReplayGain", t.roots())
	if err != nil {
		return GainRun{}, nil, err
	}
	t.albums = t.planAlbums()
	for i := range t.batches {
		t.total += len(t.batches[i].items)
	}
	ulid, err := NewID(s.now())
	if err != nil {
		release()
		return GainRun{}, nil, err
	}
	start.ULID, start.At, start.Total, start.Albums, start.UndoOf = ulid, s.now(), t.total, len(t.albums), t.undoOf
	if t.id, err = s.st.StartGainRun(ctx, start); err != nil {
		release()
		return GainRun{}, nil, err //nolint:wrapcheck // The store's error already says what failed.
	}
	t.ulid = ulid
	t.progress = GainProgress{Run: ulid, State: store.GainRunning, Total: t.total}
	s.gaining.Store(t)
	s.notifyGain(t, true)
	return GainRun{ID: ulid}, func(ctx context.Context) {
		t.release = sync.OnceFunc(release)
		defer t.release()
		s.runGainTask(ctx, t)
	}, nil
}

// runGainTask runs t's batches, then finishes it.
func (s *Scanner) runGainTask(ctx context.Context, t *gainTask) {
	start := time.Now()
	state := store.GainDone
	for i := range t.batches {
		if t.stop.Load() || ctx.Err() != nil {
			state = store.GainStopped
			break
		}
		b := &t.batches[i]
		t.update(func(p *GainProgress) { p.Album = b.album })
		if err := s.runGainBatch(ctx, t, b); err != nil {
			if ctx.Err() == nil {
				s.log.ErrorContext(ctx, "a ReplayGain run stopped", "run", t.ulid, "err", err)
			}
			state = store.GainStopped
			break
		}
		if i+1 == len(t.batches) || t.batches[i+1].album != b.album {
			t.update(func(p *GainProgress) { p.AlbumsDone++ })
		}
		s.gainFaulted(gainStepBatch, b.album)
		s.notifyGain(t, false)
		s.notifyLibrary(false)
		s.tellLoudnessCounts(ctx, false)
	}
	s.finishGainTask(context.WithoutCancel(ctx), t, state, time.Since(start))
}

// finishGainTask records how t ended and tells everyone.
func (s *Scanner) finishGainTask(ctx context.Context, t *gainTask, state string, took time.Duration) {
	// Stop is refused from here on: the run is no longer the one going.
	t.update(func(p *GainProgress) { p.State, p.Album = state, "" })
	s.gaining.CompareAndSwap(t, nil)
	t.release() // The library is whole: a scan or the next run may go.
	if t.kind == store.GainKindUndo && t.undoOf != 0 {
		s.settleGainUndone(ctx, t) // First, so the undo is never seen done before its run is undone.
	}
	if err := s.st.FinishGainRun(ctx, t.id, state, s.now()); err != nil {
		s.log.ErrorContext(ctx, "recording the end of a ReplayGain run failed", "run", t.ulid, "err", err)
	}
	if _, err := s.st.PruneGainRuns(ctx, gainKeep, s.now().Add(-gainKeepFor)); err != nil {
		s.log.WarnContext(ctx, "pruning old ReplayGain runs failed", "err", err)
	}
	s.notifyGain(t, true)
	s.notifyLibrary(true)
	s.tellLoudnessCounts(ctx, true)
	p := t.snapshot()
	s.log.InfoContext(ctx, "ReplayGain run ended", "run", t.ulid, "kind", t.kind, "state", state,
		"songs", p.Done, "of", p.Total, "duration", took)
}

// notifyGain tells the notifier t's progress, at most every gainEvery unless
// force is set.
func (s *Scanner) notifyGain(t *gainTask, force bool) {
	if s.gainNote.dueEvery(force, gainEvery) {
		p := t.snapshot()
		s.notify.GainChanged(&p)
	}
}

// tellLoudnessCounts reads the library's loudness counts again, which a run
// moves, and tells the notifier when it is due or force is set.
func (s *Scanner) tellLoudnessCounts(ctx context.Context, force bool) {
	if !s.loudnessNote.due(force) {
		return
	}
	s.refreshLoudnessCounts(ctx)
	s.notify.LoudnessChanged(s.Loudness())
}

// gainLeft is a song a batch leaves as it was, and why.
type gainLeft struct {
	item   gainItem
	reason GainReason
}

// gainReady is a song a batch may write: the file it writes, and what the
// file holds now.
type gainReady struct {
	item  gainItem
	path  string // Resolved under its root.
	prior gainTags
}

// runGainBatch writes one batch, holding the library lock. Once its files
// start changing it finishes, whatever ctx does.
func (s *Scanner) runGainBatch(ctx context.Context, t *gainTask, b *gainBatch) error {
	if err := s.lock.lock(ctx, heldByPass); err != nil {
		return err
	}
	defer s.lock.unlock()
	if !s.live().TagWrites {
		return ErrTagWritesOff
	}
	if rootLooksDown(b.items[0].root) {
		return fmt.Errorf("%w: %s", ErrRootDown, b.items[0].root)
	}
	bg := context.WithoutCancel(ctx)
	var ready []gainReady
	var left []gainLeft
	var err error
	if t.kind == store.GainKindUndo {
		ready, left, err = s.checkUndo(bg, b.items)
	} else {
		ready, left, err = s.checkGain(bg, b.items)
	}
	if err != nil {
		return err
	}
	rows, err := s.journalGain(bg, t, ready, left)
	if err != nil {
		return err
	}
	s.gainFaulted(gainStepJournaled, "")
	settled, tally, err := s.writeGainBatch(bg, t, ready, rows)
	for i := range left {
		settled = append(settled, store.GainSettled{ID: rows[len(ready)+i].ID, State: store.GainSkipped, Reason: string(left[i].reason)})
		tally.Left++
	}
	if settleErr := s.st.SettleGain(bg, t.id, settled, tally); settleErr != nil {
		return errors.Join(err, settleErr)
	}
	t.update(func(p *GainProgress) { p.Done += len(b.items) })
	return err
}

// journalGain journals a batch: the songs it writes as planned, then those it
// leaves as skipped, with why, in one transaction.
func (s *Scanner) journalGain(ctx context.Context, t *gainTask, ready []gainReady, left []gainLeft) ([]store.GainWrite, error) {
	rows := make([]store.GainWrite, 0, len(ready)+len(left))
	for i := range ready {
		w, err := ready[i].journal(t.kind)
		if err != nil {
			return nil, err
		}
		rows = append(rows, w)
	}
	for i := range left {
		w := left[i].item.row()
		w.State, w.Reason = store.GainSkipped, string(left[i].reason)
		rows = append(rows, w)
	}
	if err := s.st.JournalGain(ctx, t.id, rows); err != nil {
		return nil, err //nolint:wrapcheck // The store's error already says what failed.
	}
	return rows, nil
}

// row is the item's journal row.
func (it *gainItem) row() store.GainWrite {
	return store.GainWrite{
		Track: it.track, Album: it.album, Ord: it.ord, Root: it.root, RelPath: it.rel, Values: it.values,
		Prior: it.prior, Undoes: it.undoes,
	}
}

// journal is the ready song's journal row: for a run, with the tags the file
// holds now, which an undo puts back.
func (r *gainReady) journal(kind string) (store.GainWrite, error) {
	w := r.item.row()
	if kind == store.GainKindRun {
		prior, err := r.prior.encode()
		if err != nil {
			return store.GainWrite{}, err
		}
		w.Prior = prior
	}
	return w, nil
}

// checkGain reads the songs of a run's batch as they are now, in the store and
// on disk, and returns those it may write and those it leaves, with why.
func (s *Scanner) checkGain(ctx context.Context, items []gainItem) ([]gainReady, []gainLeft, error) {
	states, err := s.st.GainTrackStates(ctx, trackIDs(items), loudnessVersion)
	if err != nil {
		return nil, nil, err //nolint:wrapcheck // The store's error already says what failed.
	}
	probed := map[string]error{}
	var ready []gainReady
	var left []gainLeft
	for i := range items {
		it := &items[i]
		st, ok := states[it.track]
		reason := gainStoreReason(&st, ok, it)
		var r gainReady
		if reason == "" {
			r, reason = s.checkGainFile(ctx, it, probed)
		}
		if reason == "" && r.prior.tagged() {
			reason = GainChanged // Another tool tagged it since the scan, keeping its size and mtime.
		}
		if reason != "" {
			left = append(left, gainLeft{item: *it, reason: reason})
			continue
		}
		ready = append(ready, r)
	}
	return ready, left, nil
}

// gainStoreReason is why the song of it, which the store holds as st (ok
// false: gone), can't be written now, or "".
func gainStoreReason(st *store.GainTrackState, ok bool, it *gainItem) GainReason {
	if reason := storeReason(st, ok); reason != "" {
		return reason
	}
	switch {
	case st.Root != it.root || st.RelPath != it.rel || st.Size != it.size || st.MTime.UnixMilli() != it.mtime || !st.Ungained:
		return GainChanged
	case st.Why != store.GainWritable || gainValuesOf(&st.Measure) != it.values:
		return GainStale
	}
	return ""
}

// storeReason is why a song the store holds as st (ok false: gone) can't be
// written or put back, or "".
func storeReason(st *store.GainTrackState, ok bool) GainReason {
	switch {
	case !ok:
		return GainMissing
	case st.Trashed:
		return GainTrashed
	case st.Missing:
		return GainMissing
	}
	return ""
}

// trackIDs returns the items' tracks.
func trackIDs(items []gainItem) []int64 {
	out := make([]int64, len(items))
	for i := range items {
		out[i] = items[i].track
	}
	return out
}

// checkGainFile checks the file of it on disk: under its root, as the store
// has it, writable, and read by audiometa, whose ReplayGain tags it returns.
// probed keeps each folder's write probe, so a batch probes a folder once.
func (s *Scanner) checkGainFile(ctx context.Context, it *gainItem, probed map[string]error) (gainReady, GainReason) {
	file := &store.EditFile{Root: it.root, RelPath: it.rel, Size: it.size, MTime: time.UnixMilli(it.mtime)}
	path, err := s.resolve(file)
	if err != nil {
		return gainReady{}, s.fileReason(ctx, it, err)
	}
	if err = checkDrift(file); err != nil {
		return gainReady{}, s.fileReason(ctx, it, err)
	}
	if err = checkWritableOnce(path, probed); err != nil {
		return gainReady{}, s.fileReason(ctx, it, err)
	}
	f, err := audiometa.OpenContext(ctx, path)
	if err != nil {
		return gainReady{}, s.fileReason(ctx, it, err)
	}
	defer func() { _ = f.Close() }() // Only read.
	if err = f.WritableContext(ctx); err != nil {
		return gainReady{}, s.fileReason(ctx, it, err)
	}
	return gainReady{item: *it, path: path, prior: gainTagsOf(f)}, ""
}

// checkWritableOnce is checkWritable, probing each folder once.
func checkWritableOnce(path string, probed map[string]error) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}
	if info.Mode().Perm()&anyWrite == 0 {
		return fmt.Errorf("it is read-only: %w", ErrNotWritable)
	}
	dir := filepath.Dir(path)
	if probe, ok := probed[dir]; ok {
		return probe
	}
	err = checkWritable(path)
	probed[dir] = err
	return err
}

// fileReason is why a song stays when its file failed with err, logged when
// it is nothing a person can be told plainly.
func (s *Scanner) fileReason(ctx context.Context, it *gainItem, err error) GainReason {
	switch err = classify(err); {
	case errors.Is(err, errGainChanged), errors.Is(err, ErrStale):
		return GainChanged
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOTDIR):
		return GainMissing
	case errors.Is(err, ErrNotWritable), errors.Is(err, ErrUnsupportedEdit):
		return GainUnwritable
	}
	if _, bad := errors.AsType[*audiometa.CorruptedFileError](err); bad {
		return GainUnwritable
	}
	if _, unsupported := errors.AsType[*audiometa.UnsupportedFormatError](err); unsupported {
		return GainUnwritable
	}
	s.log.WarnContext(ctx, "a ReplayGain write failed; the file stays as it was",
		"track", it.track, "err", StripRoot(err.Error(), it.root))
	return GainFailed
}

// writeGainBatch writes each ready song, whose journal rows are rows, and
// reads it back into the store. It returns how each ended and what they add to
// the run's counts. A file that was written but not read back stops the batch
// with the error, its row left planned for recovery.
func (s *Scanner) writeGainBatch(ctx context.Context, t *gainTask, ready []gainReady, rows []store.GainWrite) ([]store.GainSettled, store.GainTally, error) {
	settled := make([]store.GainSettled, 0, len(rows))
	var tally store.GainTally
	for i := range ready {
		r := &ready[i]
		row := &rows[i]
		edit, err := r.edit(t.kind)
		if err == nil {
			err = s.write(ctx, r.path, edit, s.writeMode(filepath.Join(r.item.root, r.item.rel))...)
		}
		if err != nil {
			reason := s.fileReason(ctx, &r.item, err)
			if reason == GainChanged && t.kind == store.GainKindUndo {
				reason = GainRetagged
			}
			settled = append(settled, store.GainSettled{ID: row.ID, State: store.GainSkipped, Reason: string(reason)})
			if reason == GainFailed {
				tally.Failed++
			} else {
				tally.Left++
			}
			continue
		}
		s.gainFaulted(gainStepWritten, r.item.rel)
		info, err := s.reindexFile(ctx, r.item.track, r.item.root, r.item.rel, true)
		if err != nil {
			return settled, tally, fmt.Errorf("read back %s: %w", StripRoot(r.item.rel, r.item.root), err)
		}
		s.gainFaulted(gainStepReindexed, r.item.rel)
		settled = append(settled, store.GainSettled{
			ID: row.ID, State: store.GainWritten, Size: info.Size(), MTime: info.ModTime(), Undoes: r.item.undoes,
		})
		tally.Written++
	}
	return settled, tally, nil
}

// edit is the audiometa edit that writes the ready song: a run's values, or
// for an undo the tags from before.
func (r *gainReady) edit(kind string) (func(*audiometa.File) error, error) {
	if kind == store.GainKindRun {
		return writeGain(r.prior, &r.item.values), nil
	}
	prior, err := decodeGainTags(r.item.prior)
	if err != nil {
		return nil, err
	}
	return unwriteGain(prior, &r.item.values), nil
}

// writeMode is how a run writes the file at path: in place, so only its
// metadata is read and written, unless a stream has it open, when the atomic
// rewrite leaves the stream its old file. A stream that opens the file during
// an in-place patch of a few milliseconds may read a part of the tags as they
// were: the known limit.
func (s *Scanner) writeMode(path string) []audiometa.WriteOption {
	if s.streams != nil && s.streams.Open(path) {
		return nil
	}
	return []audiometa.WriteOption{audiometa.WithInPlace()}
}
