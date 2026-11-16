package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/simonhull/kitsune/internal/organize"
	"github.com/simonhull/kitsune/internal/store"
)

// RecoverOrganize settles what a server stopped in the middle of an organize
// run left: every journal row still planned, by where its file is now. It runs
// at start, before the first scan and the watcher, holding the claim and the
// library lock, so nothing sees a song at neither path.
//
//   - The file at to and not at from: the move happened, so the song's path
//     follows and the row is moved.
//   - At from and not at to: it never happened; the row is skipped.
//   - At both, the same file: a link with no remove. The link at from goes,
//     then as the first case. Two different files stay, and are logged.
//   - At neither, but at the hidden name of a change of case: the second step
//     is made. At none of them: the row is skipped, the file missing.
//
// A row whose root can't be opened stays planned for the next start: the
// root may be down. Every run still marked running is then stopped.
func (s *Scanner) RecoverOrganize(ctx context.Context) error {
	if err := s.claim.lock(ctx, heldByScan); err != nil {
		return err
	}
	defer s.claim.unlock()
	if err := s.lock.lock(ctx, heldByScan); err != nil {
		return err
	}
	defer s.lock.unlock()
	planned, err := s.st.PlannedOrganize(ctx)
	if err != nil {
		return err //nolint:wrapcheck // The store's error already says what failed.
	}
	roots := openRoots{}
	defer roots.close()
	settled := 0
	for i := range planned {
		ok, recErr := s.recoverMove(ctx, &roots, &planned[i])
		if recErr != nil {
			return recErr
		}
		if ok {
			settled++
		}
	}
	stopped, err := s.st.StopRunningOrganizeRuns(ctx, s.now())
	if err != nil {
		return err //nolint:wrapcheck // As above.
	}
	if settled > 0 || stopped > 0 {
		s.log.WarnContext(ctx, "settled an organize run a stop interrupted", "moves", settled, "runs", stopped)
	}
	s.notifyLibrary(true)
	return nil
}

// recoverMove settles one planned move by where its file is now, and reports
// whether it could.
func (s *Scanner) recoverMove(ctx context.Context, roots *openRoots, m *store.OrganizeMove) (bool, error) {
	r, err := roots.root(m.Root)
	if err != nil {
		s.log.WarnContext(ctx, "an interrupted organize move waits for its library root", "root", m.Root, "err", err)
		return false, nil // A root that won't open may be down: the row waits for the next start.
	}
	if foldEqual(m.From, m.To) {
		return s.recoverCase(ctx, roots, r, m)
	}
	both, err := lookBoth(r, m)
	if err != nil {
		s.log.WarnContext(ctx, "an interrupted organize move can't be looked at; it waits", "err", StripRoot(err.Error(), m.Root))
		return false, nil
	}
	return true, s.recoverBoth(ctx, r, m, both)
}

// bothPaths is what is at a move's two paths: nil where nothing is.
type bothPaths struct{ from, to fs.FileInfo }

// lookBoth returns what is at m's two paths.
func lookBoth(r *os.Root, m *store.OrganizeMove) (bothPaths, error) {
	from, errFrom := r.Lstat(m.From)
	if errors.Is(errFrom, fs.ErrNotExist) {
		from, errFrom = nil, nil
	}
	to, errTo := r.Lstat(m.To)
	if errors.Is(errTo, fs.ErrNotExist) {
		to, errTo = nil, nil
	}
	if err := errors.Join(errFrom, errTo); err != nil {
		return bothPaths{}, fmt.Errorf("look: %w", err)
	}
	return bothPaths{from: from, to: to}, nil
}

// recoverBoth settles m by what is at its two paths.
func (s *Scanner) recoverBoth(ctx context.Context, r *os.Root, m *store.OrganizeMove, both bothPaths) error {
	switch atFrom, atTo := both.from != nil, both.to != nil; {
	case atFrom && atTo && os.SameFile(both.from, both.to):
		if err := r.Remove(m.From); err != nil {
			return fmt.Errorf("recover: remove the old link: %w", err)
		}
		return s.recoverLanded(ctx, m)
	case atFrom && atTo:
		s.log.ErrorContext(ctx, "an interrupted organize move left two different files; both stay, and the song where it was",
			"track", m.Track)
		return s.recoverSkipped(ctx, m, organize.Failed)
	case atTo:
		return s.recoverLanded(ctx, m)
	case atFrom:
		return s.recoverSkipped(ctx, m, organize.Failed)
	}
	return s.recoverSkipped(ctx, m, organize.Missing)
}

// recoverCase settles a planned change of case. Its two names may be one file
// on a filesystem that ignores case, so it never removes one for the other: it
// goes by the names the folder lists, and by the hidden name between them.
func (s *Scanner) recoverCase(ctx context.Context, roots *openRoots, r *os.Root, m *store.OrganizeMove) (bool, error) {
	tmp := organizeTemp(m.From)
	if _, err := r.Lstat(tmp); err == nil {
		if listed(r, m.From) {
			roots.unlink(r, m.From, tmp) // The first step linked and never removed its source.
			return true, s.recoverSkipped(ctx, m, organize.Failed)
		}
		if err = roots.moveBack(m.Root, tmp, m.To); err != nil {
			return false, fmt.Errorf("recover: finish a change of case: %w", err)
		}
		return true, s.recoverLanded(ctx, m)
	}
	switch atFrom, atTo := listed(r, m.From), listed(r, m.To); {
	case atTo && !atFrom:
		return true, s.recoverLanded(ctx, m)
	case atFrom:
		return true, s.recoverSkipped(ctx, m, organize.Failed)
	}
	return true, s.recoverSkipped(ctx, m, organize.Missing)
}

// listed reports whether the file rel is under r by that very name, as its
// folder lists it, and not only reached by it, as a filesystem that ignores case
// allows.
func listed(r *os.Root, rel string) bool {
	if _, err := r.Lstat(rel); err != nil {
		return false
	}
	return exactName(r, rel)
}

// recoverLanded settles m as moved: its song's path follows its file, unless
// the row is there already.
func (s *Scanner) recoverLanded(ctx context.Context, m *store.OrganizeMove) error {
	l := store.OrganizeLanded{Move: *m, State: store.MoveMoved}
	var t store.OrganizeTally
	switch m.Kind {
	case store.MoveSong:
		states, err := s.st.OrganizeTrackStates(ctx, []int64{m.Track})
		if err != nil {
			return err //nolint:wrapcheck // The store's error already says what failed.
		}
		st, ok := states[m.Track]
		if !ok || st.RelPath == m.To || st.RelPath != m.From {
			l.Move.Kind = "" // The row is gone or elsewhere: settle the journal only.
		} else {
			l.Album = st.Album
		}
		t.Moved = 1
	case store.MoveCompanion:
		t.Companions = 1
	}
	return s.st.LandOrganize(ctx, m.Run, []store.OrganizeLanded{l}, t) //nolint:wrapcheck // As above.
}

// recoverSkipped settles m as skipped, for reason.
func (s *Scanner) recoverSkipped(ctx context.Context, m *store.OrganizeMove, reason organize.Reason) error {
	l := store.OrganizeLanded{Move: *m, State: store.MoveSkipped, Reason: string(reason)}
	l.Move.Kind = "" // Nothing moved: the journal only.
	var t store.OrganizeTally
	if m.Kind == store.MoveSong {
		t.Left = 1
	}
	return s.st.LandOrganize(ctx, m.Run, []store.OrganizeLanded{l}, t) //nolint:wrapcheck // The store's error already says what failed.
}
