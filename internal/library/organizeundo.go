package library

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/simonhull/kitsune/internal/organize"
	"github.com/simonhull/kitsune/internal/store"
)

// StartUndo claims the scanner for an undo of the run with the public ID id, by
// the user by: a run in reverse over what that run moved, with the same
// batches, journal and progress, so it can be stopped and recovered too. Each
// song goes back where it was, unless it isn't where the run put it any more
// (moved again, edited elsewhere, trashed or gone), or something is at its old
// path now: those stay, journaled with why. Companions go back with them, and
// playlist files the run rewrote are rewritten back. The caller runs the
// returned func as [Scanner.StartOrganize] says.
//
// It returns an error wrapping [store.ErrNotFound] for a run the journal
// doesn't hold, [ErrNothingToUndo] for an undo, a run still going, one undone,
// or one none of whose songs is still where it put them, and the errors of
// [Scanner.StartOrganize].
func (s *Scanner) StartUndo(ctx context.Context, id string, by int64) (OrganizeRun, func(context.Context), error) {
	run, err := s.st.OrganizeRunByULID(ctx, id)
	if err != nil {
		return OrganizeRun{}, nil, err //nolint:wrapcheck // The store's error already says what failed.
	}
	if run.Kind == store.OrganizeKindUndo || run.State == store.OrganizeRunning || run.State == store.OrganizeUndone || !run.StillThere {
		return OrganizeRun{}, nil, ErrNothingToUndo
	}
	moved, err := s.st.MovedOrganize(ctx, run.ID)
	if err != nil {
		return OrganizeRun{}, nil, err //nolint:wrapcheck // As above.
	}
	t := &organizeTask{kind: store.OrganizeKindUndo, undoOf: run.ID, undoOfULID: run.ULID, rewritePlaylists: run.Playlists > 0, batches: undoBatches(moved)}
	if len(t.batches) == 0 {
		return OrganizeRun{}, nil, ErrNothingToUndo
	}
	return s.startTask(ctx, t, &store.OrganizeRunStart{Kind: store.OrganizeKindUndo, Template: run.Template, By: by})
}

// undoBatches cuts what a run moved into batches that put it back, the last
// album moved first, each album's songs in order. A companion goes back in the
// batch of a song that left its folder, else in the last batch.
func undoBatches(moved []store.OrganizeMoved) []organizeBatch {
	var batches []organizeBatch
	batchOf := map[string]int{} // A folder songs go back from → its batch.
	songs := slices.DeleteFunc(slices.Clone(moved), func(m store.OrganizeMoved) bool { return m.Kind != store.MoveSong })
	for i := len(songs) - 1; i >= 0; i-- {
		m := &songs[i]
		last := len(batches) - 1
		if last < 0 || batches[last].album != m.Album || len(batches[last].songs) == organizeBatchSongs {
			batches = append(batches, organizeBatch{album: m.Album})
			last++
		}
		batches[last].songs = append(batches[last].songs, undoItem(m))
		batchOf[m.Root+"\x00"+filepath.Dir(m.To)] = last
	}
	for i := range batches {
		slices.Reverse(batches[i].songs) // Each album's songs in the order they moved.
	}
	for i := range moved {
		m := &moved[i]
		if m.Kind != store.MoveCompanion || len(batches) == 0 {
			continue
		}
		at, ok := batchOf[m.Root+"\x00"+filepath.Dir(m.To)]
		if !ok {
			at = len(batches) - 1
		}
		batches[at].companions = append(batches[at].companions, organizeItem{
			root: m.Root, from: m.To, to: m.From, caseOnly: foldEqual(m.To, m.From), undoes: m.ID,
		})
	}
	return batches
}

// undoItem is the move that puts back the song m moved: it may go while the
// song is live and still where the run put it.
func undoItem(m *store.OrganizeMoved) organizeItem {
	root, at := m.Root, m.To
	return organizeItem{
		track: m.Track, title: m.Title, album: m.Album, root: m.Root, from: m.To, to: m.From,
		caseOnly: foldEqual(m.To, m.From), undoes: m.ID,
		check: func(st *store.OrganizeTrackState) organize.Reason {
			if !st.Live || st.Root != root || st.RelPath != at {
				return organize.MovedSince
			}
			return organize.NoReason
		},
	}
}

// settleUndone marks the run an undo reversed undone once none of its songs is
// still where it put them.
func (s *Scanner) settleUndone(ctx context.Context, t *organizeTask) {
	run, err := s.st.OrganizeRunByULID(ctx, t.undoOfULID)
	if err != nil {
		s.log.WarnContext(ctx, "reading the run an undo reversed failed", "err", err)
		return
	}
	if run.StillThere {
		return // Some songs stayed: what is left can be tried again.
	}
	if err = s.st.SetOrganizeRunState(ctx, run.ID, store.OrganizeUndone); err != nil {
		s.log.WarnContext(ctx, "marking a run undone failed", "err", fmt.Errorf("run %s: %w", run.ULID, err))
	}
}
