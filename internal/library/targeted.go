package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

// ErrRootUnavailable means a targeted scan found its root missing, unreadable,
// or looking like an outage, and changed nothing: the full-scan rules decide
// what such a root means.
var ErrRootUnavailable = errors.New("library root is unavailable")

// errNotRoot means a targeted scan named a folder that is no longer a root.
var errNotRoot = errors.New("not a library root")

// ScanDirs reads the folders dirs under root, and only those, through the
// scan's read path and identity rules, holding the claim and the library lock
// as a scan does. dirs are relative to root, "." for root itself; a folder that
// no longer exists stands for its nearest one that does.
//
// It marks a track missing only when the folder holding it, or the nearest
// folder above it, was read and the file is gone; a folder new to the store is
// read whole. It never deletes or prunes: that waits for a full scan, which
// alone can tell an outage from a change. A root that is missing, unreadable,
// empty while the store has tracks under it, or that would lose most of its
// tracks, is left alone and ScanDirs returns [ErrRootUnavailable].
//
// It returns [ErrScanRunning] while another scan holds the scanner, and waits
// for an edit to finish. A canceled ctx ends the scan with an error wrapping
// ctx.Err(); everything read by then is stored. It leaves [Scanner.Progress]
// alone: a targeted scan is not a scan the library page shows.
func (s *Scanner) ScanDirs(ctx context.Context, root string, dirs []string) (Stats, error) {
	if !s.claim.tryLock(heldByScan) {
		return Stats{}, ErrScanRunning
	}
	defer s.claim.unlock()
	start := time.Now()
	stats, err := s.exclusiveDirs(ctx, root, dirs)
	if ctxErr := ctx.Err(); err != nil && ctxErr != nil {
		err = fmt.Errorf("targeted scan: %w", ctxErr)
	}
	s.logTargeted(ctx, len(dirs), &stats, err, time.Since(start))
	s.notifyLibrary(true)
	if stats.Read > 0 || stats.Missing > 0 {
		s.askForPass()
	}
	return stats, err
}

// exclusiveDirs waits for the library lock and runs the targeted scan holding it.
func (s *Scanner) exclusiveDirs(ctx context.Context, root string, dirs []string) (Stats, error) {
	if err := s.lock.lock(ctx, heldByScan); err != nil {
		return Stats{}, fmt.Errorf("targeted scan: %w", err)
	}
	defer s.lock.unlock()
	if !slices.Contains(s.live().Roots, root) {
		return Stats{}, fmt.Errorf("targeted scan of %s: %w", root, errNotRoot)
	}
	r, err := s.surveyDirs(ctx, root, dirs)
	if err != nil {
		return Stats{}, fmt.Errorf("targeted scan: survey: %w", err)
	}
	if r.skipped {
		return Stats{SkippedRoots: 1}, ErrRootUnavailable
	}
	defer s.refreshNames(ctx)
	defer s.refreshGenres(ctx)
	sv := &surveyed{Jobs: r.jobs, Mark: r.mark, Temps: r.temps, Seen: r.seen, IssueDeletes: map[string][]string{}}
	if len(r.issueDeletes) > 0 {
		sv.IssueDeletes[root] = r.issueDeletes
	}
	stats := Stats{Seen: sv.Seen}
	if err = s.applySurvey(ctx, sv, &stats); err != nil {
		return stats, fmt.Errorf("targeted scan: %w", err)
	}
	if _, err = s.read(ctx, sv.Jobs, &stats); err != nil {
		return stats, fmt.Errorf("targeted scan: %w", err)
	}
	return stats, nil
}

// logTargeted logs how a targeted scan went.
func (s *Scanner) logTargeted(ctx context.Context, dirs int, stats *Stats, err error, elapsed time.Duration) {
	switch {
	case errors.Is(err, ErrScanRunning):
	case err != nil && ctx.Err() != nil && errors.Is(err, ctx.Err()):
		s.log.InfoContext(ctx, "targeted scan canceled", "folders", dirs, "duration", elapsed)
	case errors.Is(err, ErrRootUnavailable):
		s.log.WarnContext(ctx, "targeted scan left an unavailable library root alone", "folders", dirs)
	case err != nil:
		s.log.ErrorContext(ctx, "targeted scan failed", "folders", dirs, "err", err, "duration", elapsed)
	default:
		s.log.InfoContext(ctx, "targeted scan finished", "folders", dirs,
			"seen", stats.Seen, "read", stats.Read, "added", stats.Added, "updated", stats.Updated,
			"moved", stats.Moved, "missing", stats.Missing, "issues", stats.Issues, "duration", elapsed)
	}
}

// dirListing is what a targeted scan read of the folders it was given.
type dirListing struct {
	walked

	read    map[string]map[string]struct{} // Folder read → the subfolders it holds, by name.
	visited map[string]struct{}            // Folders read whole, because the store knew nothing in them.
}

// surveyDirs reads dirs under root and diffs what they hold against the store,
// as [Scanner.surveyRoot] does for a whole root. A root that looks like an
// outage comes back skipped.
func (s *Scanner) surveyDirs(ctx context.Context, root string, dirs []string) (rootSurvey, error) {
	states, err := s.st.FileStates(ctx, root)
	if err != nil {
		return rootSurvey{}, fmt.Errorf("root %s: %w", root, err)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		s.log.ErrorContext(ctx, "library root is unreadable; leaving it to the next scan", "root", root, "err", err)
		return skipped(states), nil
	}
	if why := rootDown(resolved, states); why != "" {
		s.log.ErrorContext(ctx, "library root "+why+"; is a disk unmounted? leaving it to the next scan", "root", root)
		return skipped(states), nil
	}
	l := &dirListing{read: map[string]map[string]struct{}{}, visited: map[string]struct{}{}}
	for _, dir := range cleanDirs(dirs) {
		if listErr := l.list(ctx, resolved, dir, states); listErr != nil {
			return rootSurvey{}, listErr
		}
	}
	r := l.diff(root, states)
	if absent, live := len(r.mark), countLive(states); losesMost(absent, live, countNew(r.jobs)) {
		s.log.ErrorContext(ctx, "a targeted scan would mark most of a library root's tracks missing; leaving it to the next scan",
			"root", root, "vanished", absent, "live", live)
		return skipped(states), nil
	}
	if len(l.unreadable) > 0 {
		s.log.WarnContext(ctx, "some library folders or files are unreadable; leaving their tracks alone",
			"root", root, "paths", l.unreadable)
	}
	return r, nil
}

// countLive counts the states that are tracks not missing.
func countLive(states map[string]store.FileState) int {
	n := 0
	for _, st := range states {
		if st.TrackID != 0 && !st.Missing {
			n++
		}
	}
	return n
}

// losesMost reports whether marking absent of a root's live tracks missing,
// with newFiles new ones read, looks like an outage: a mass disappearance, or
// every track gone with nothing new, which a full scan would call an empty root.
func losesMost(absent, live, newFiles int) bool {
	return massDisappearance(absent, live, newFiles) || (absent > 0 && absent == live && newFiles == 0)
}

// rootDown says why the resolved root looks down, or "" when it doesn't: it
// can't be read, or it shows nothing while the store has tracks under it, which
// is what an unmounted disk leaves behind, an empty mount point.
func rootDown(resolved string, states map[string]store.FileState) string {
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return "is unreadable"
	}
	visible := slices.ContainsFunc(entries, func(e fs.DirEntry) bool { return !strings.HasPrefix(e.Name(), ".") })
	if !visible && countTracks(states) > 0 {
		return "is empty but the database has tracks under it"
	}
	return ""
}

// cleanDirs returns dirs cleaned, without duplicates, in order.
func cleanDirs(dirs []string) []string {
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		out = append(out, filepath.Clean(d))
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// list reads dir, relative to the resolved root, into l. A folder that is gone
// stands for its nearest one that is there; a folder below a skipped one, or
// outside the root, is no folder a scan reads. A subfolder the store knows
// nothing in is read whole. It returns an error only when ctx ends.
func (l *dirListing) list(ctx context.Context, resolved, dir string, states map[string]store.FileState) error {
	if !insideRoot(dir) {
		return nil
	}
	dir = nearestPresent(resolved, dir)
	if _, done := l.read[dir]; done || l.walkedWhole(dir) {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(resolved, dir))
	if err != nil {
		l.unreadable = append(l.unreadable, dir)
		return nil //nolint:nilerr // An unreadable folder is recorded, and its tracks stay as they are.
	}
	subdirs := map[string]struct{}{}
	l.read[dir] = subdirs
	for _, e := range entries {
		if entryErr := l.entry(ctx, resolved, dir, e, states, subdirs); entryErr != nil {
			return entryErr
		}
	}
	return nil
}

// insideRoot reports whether dir, relative to a root, is a folder a scan reads:
// inside the root, and not in a skipped folder.
func insideRoot(dir string) bool {
	outside := dir == ".." || strings.HasPrefix(dir, ".."+string(filepath.Separator)) || filepath.IsAbs(dir)
	return !outside && !skippedPath(dir)
}

// nearestPresent returns dir, or the nearest folder above it that is there.
func nearestPresent(resolved, dir string) string {
	for dir != "." {
		if _, err := os.Lstat(filepath.Join(resolved, dir)); !errors.Is(err, fs.ErrNotExist) {
			break
		}
		dir = filepath.Dir(dir)
	}
	return dir
}

// entry handles one entry of the folder dir: a file is visited as a walk
// would, and a folder is recorded in subdirs, and read whole when the store
// knows nothing in it.
func (l *dirListing) entry(ctx context.Context, resolved, dir string, e fs.DirEntry, states map[string]store.FileState, subdirs map[string]struct{}) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	rel := filepath.Join(dir, e.Name())
	switch {
	case !e.IsDir():
		_ = l.visitEntry(filepath.Join(resolved, rel), rel, e) // For a file, it only records.
	case SkipsFolder(e.Name()):
	default:
		subdirs[e.Name()] = struct{}{}
		if !knowsUnder(states, rel) {
			return l.walkNew(ctx, resolved, rel)
		}
	}
	return nil
}

// walkNew reads the folder rel, which the store knows nothing in, and all below it.
func (l *dirListing) walkNew(ctx context.Context, resolved, rel string) error {
	start := filepath.Join(resolved, rel)
	l.visited[rel] = struct{}{}
	_ = filepath.WalkDir(start, func(path string, d fs.DirEntry, err error) error {
		return l.visit(ctx, resolved, path, d, err)
	})
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("walk %s: %w", rel, err)
	}
	return nil
}

// walkedWhole reports whether dir was read whole already: it is a folder
// walkNew walked, or lies below one.
func (l *dirListing) walkedWhole(dir string) bool {
	for d := dir; ; d = filepath.Dir(d) {
		if _, ok := l.visited[d]; ok {
			return true
		}
		if d == "." {
			return false
		}
	}
}

// skippedPath reports whether rel lies in a folder a scan skips.
func skippedPath(rel string) bool {
	return slices.ContainsFunc(strings.Split(rel, string(filepath.Separator)), func(part string) bool {
		return part != "." && SkipsFolder(part)
	})
}

// knowsUnder reports whether the store has anything at or below the folder rel.
func knowsUnder(states map[string]store.FileState, rel string) bool {
	prefix := rel + string(filepath.Separator)
	for path := range states {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// diff compares what was read with the stored states: the files seen become
// jobs, and a state goes missing only when the folder holding it was read and
// its file is gone, or the nearest folder above it that was read no longer
// holds the subfolder it was in.
func (l *dirListing) diff(root string, states map[string]store.FileState) rootSurvey {
	var r rootSurvey
	seen := make(map[string]struct{}, len(l.files))
	for i := range l.files {
		f := &l.files[i]
		seen[f.rel] = struct{}{}
		st := states[f.rel]
		if unchanged(&st, f) {
			continue
		}
		r.jobs = append(r.jobs, job{
			Root: root, RelPath: f.rel, Size: f.size, MTime: f.mtime, TrackID: st.TrackID, Missing: st.Missing,
		})
	}
	r.seen = len(l.files)
	r.temps = l.temps
	covered := make(map[string]store.FileState)
	for rel, st := range states {
		if _, ok := seen[rel]; !ok && l.covers(rel) {
			covered[rel] = st
		}
	}
	gone := absentPaths(covered, nil, l.unreadable)
	r.mark, r.issueDeletes = gone.mark, gone.issues
	return r
}

// covers reports whether the read says anything about the stored path rel:
// its own folder was read, or the nearest read folder above it no longer holds
// the subfolder rel is in.
func (l *dirListing) covers(rel string) bool {
	child := rel
	for dir := filepath.Dir(rel); ; dir = filepath.Dir(dir) {
		if subdirs, ok := l.read[dir]; ok {
			if child == rel {
				return true
			}
			_, present := subdirs[filepath.Base(child)]
			return !present
		}
		if _, ok := l.visited[dir]; ok {
			return true
		}
		if dir == "." {
			return false
		}
		child = dir
	}
}
