package library

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/store"
)

// StartGainUndo claims the scanner for an undo of the run with the public ID
// id, by the user by: a run over what that run wrote, with the same batches,
// journal and progress, so it can be stopped and recovered too. Each song is
// found by its track, wherever it is now, and its file gets back exactly the
// ReplayGain tags it held before the run, unless its tags no longer read what
// the run wrote (retagged since), it changed since the last scan, or it is
// trashed or gone: those stay, journaled with why. The caller runs the
// returned func as [Scanner.StartGain] says.
//
// It returns an error wrapping [store.ErrNotFound] for a run the journal
// doesn't hold, [ErrNothingToUndo] for an undo, a run still going, one undone,
// or one whose writes are all gone, and the errors of [Scanner.StartGain]
// but [ErrNothingToWrite].
func (s *Scanner) StartGainUndo(ctx context.Context, id string, by int64) (GainRun, func(context.Context), error) {
	run, err := s.st.GainRunByULID(ctx, id)
	if err != nil {
		return GainRun{}, nil, err //nolint:wrapcheck // The store's error already says what failed.
	}
	if run.Kind == store.GainKindUndo || run.State == store.GainRunning || run.State == store.GainUndone || !run.StillWritten {
		return GainRun{}, nil, ErrNothingToUndo
	}
	written, err := s.st.WrittenGain(ctx, run.ID)
	if err != nil {
		return GainRun{}, nil, err //nolint:wrapcheck // As above.
	}
	t := &gainTask{kind: store.GainKindUndo, undoOf: run.ID, undoOfULID: run.ULID, batches: undoGainBatches(written)}
	if len(t.batches) == 0 {
		return GainRun{}, nil, ErrNothingToUndo
	}
	return s.startGainTask(ctx, t, &store.GainRunStart{Kind: store.GainKindUndo, By: by})
}

// undoGainBatches cuts what a run wrote into batches, album by album in the
// run's order. A write whose song is deleted for good has nothing to undo.
func undoGainBatches(written []store.GainMade) []gainBatch {
	var batches []gainBatch
	for i := range written {
		w := &written[i]
		if w.Track == 0 {
			continue
		}
		last := len(batches) - 1
		if last < 0 || batches[last].album != w.Album || len(batches[last].items) == gainBatchSongs {
			batches = append(batches, gainBatch{album: w.Album})
			last++
		}
		batches[last].items = append(batches[last].items, gainItem{
			track: w.Track, album: w.Album, ord: w.Ord, root: w.Root, rel: w.RelPath,
			values: w.Values, prior: w.Prior, undoes: w.ID,
		})
	}
	return batches
}

// checkUndo reads the songs of an undo's batch as they are now, wherever they
// are, and returns those it may put back and those it leaves, with why.
func (s *Scanner) checkUndo(ctx context.Context, items []gainItem) ([]gainReady, []gainLeft, error) {
	states, err := s.st.GainTrackStates(ctx, trackIDs(items), loudnessVersion)
	if err != nil {
		return nil, nil, err //nolint:wrapcheck // The store's error already says what failed.
	}
	probed := map[string]error{}
	var ready []gainReady
	var left []gainLeft
	for i := range items {
		it := items[i]
		st, ok := states[it.track]
		if reason := storeReason(&st, ok); reason != "" {
			left = append(left, gainLeft{item: it, reason: reason})
			continue
		}
		// Where the song is now: the organizer may have moved it since.
		it.root, it.rel, it.size, it.mtime = st.Root, st.RelPath, st.Size, st.MTime.UnixMilli()
		r, reason := s.checkGainFile(ctx, &it, probed)
		if reason == "" && !r.prior.reads(&it.values) {
			reason = GainRetagged
		}
		if reason != "" {
			left = append(left, gainLeft{item: it, reason: reason})
			continue
		}
		ready = append(ready, r)
	}
	return ready, left, nil
}

// settleGainUndone marks the run an undo reversed undone once nothing it
// wrote is left to take out.
func (s *Scanner) settleGainUndone(ctx context.Context, t *gainTask) {
	run, err := s.st.GainRunByULID(ctx, t.undoOfULID)
	if err != nil {
		s.log.WarnContext(ctx, "reading the run an undo reversed failed", "err", err)
		return
	}
	if run.StillWritten {
		return // Some songs stayed: what is left can be tried again.
	}
	if err = s.st.SetGainRunState(ctx, run.ID, store.GainUndone); err != nil {
		s.log.WarnContext(ctx, "marking a run undone failed", "err", fmt.Errorf("run %s: %w", run.ULID, err))
	}
}

// RecoverGain settles what a server stopped in the middle of a ReplayGain run
// left: every journal row still planned, by what its file holds now. It runs at
// start, before the first scan and the watcher, holding the claim and the
// library lock.
//
//   - A run's row whose file reads the values it planned: the write landed, so
//     the file is read back into the store, keeping its fingerprint and
//     measure, and the row is written.
//   - An undo's row whose file holds exactly the tags from before: the same.
//   - Anything else: the write never happened (or the file was retagged), and
//     the row is skipped, interrupted; a file gone, missing.
//
// A row whose root looks down stays planned for the next start. Every run still
// marked running is then stopped.
func (s *Scanner) RecoverGain(ctx context.Context) error {
	if err := s.claim.lock(ctx, heldByScan); err != nil {
		return err
	}
	defer s.claim.unlock()
	if err := s.lock.lock(ctx, heldByScan); err != nil {
		return err
	}
	defer s.lock.unlock()
	planned, err := s.st.PlannedGain(ctx)
	if err != nil {
		return err //nolint:wrapcheck // The store's error already says what failed.
	}
	settled := 0
	for i := range planned {
		ok, recErr := s.recoverGainWrite(ctx, &planned[i])
		if recErr != nil {
			return recErr
		}
		if ok {
			settled++
		}
	}
	stopped, err := s.st.StopRunningGainRuns(ctx, s.now())
	if err != nil {
		return err //nolint:wrapcheck // As above.
	}
	if settled > 0 || stopped > 0 {
		s.log.WarnContext(ctx, "settled a ReplayGain run a stop interrupted", "writes", settled, "runs", stopped)
	}
	s.notifyLibrary(true)
	return nil
}

// recoverGainWrite settles one planned write by what its file holds now, and
// reports whether it could.
func (s *Scanner) recoverGainWrite(ctx context.Context, w *store.GainWrite) (bool, error) {
	if rootLooksDown(w.Root) {
		s.log.WarnContext(ctx, "an interrupted ReplayGain write waits for its library root", "root", w.Root)
		return false, nil
	}
	landed, reason := recoverLanded(ctx, w)
	if !landed || w.Track == 0 {
		return true, s.st.SettleGain(ctx, w.Run, //nolint:wrapcheck // The store's error already says what failed.
			[]store.GainSettled{{ID: w.ID, State: store.GainSkipped, Reason: string(reason)}}, store.GainTally{Left: 1})
	}
	info, err := s.reindexFile(ctx, w.Track, w.Root, w.RelPath, true)
	if err != nil {
		return false, fmt.Errorf("recover a ReplayGain write: %w", err)
	}
	return true, s.st.SettleGain(ctx, w.Run, //nolint:wrapcheck // As above.
		[]store.GainSettled{{ID: w.ID, State: store.GainWritten, Size: info.Size(), MTime: info.ModTime(), Undoes: w.Undoes}},
		store.GainTally{Written: 1})
}

// recoverLanded reports whether w's write landed in its file, and if not,
// why it stays.
func recoverLanded(ctx context.Context, w *store.GainWrite) (bool, GainReason) {
	f, err := audiometa.OpenContext(ctx, filepath.Join(w.Root, w.RelPath))
	if err != nil {
		return false, GainMissing
	}
	defer func() { _ = f.Close() }() // Only read.
	now := gainTagsOf(f)
	if w.Undoes == 0 {
		return now.reads(&w.Values), GainInterrupted
	}
	prior, err := decodeGainTags(w.Prior)
	if err != nil {
		return false, GainInterrupted
	}
	return now.same(prior), GainInterrupted
}
