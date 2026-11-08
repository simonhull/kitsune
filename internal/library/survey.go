package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

// A scan skips a root when this many tracks, a majority of the root's live
// tracks, vanish without new files appearing in their place. See [massDisappearance].
const (
	massFloor      = 50 // Fewest vanished tracks that can count as an outage.
	massAbsentOver = 2  // Vanished tracks must outnumber the live ones divided by this... (absent*2 > live).
	massNewUnder   = 2  // ...and new files must be fewer than the vanished divided by this (new*2 < absent).
)

// errRootNotDir means a configured root is not a directory.
var errRootNotDir = errors.New("root is not a directory")

// job is one file the read phase has to read.
type job struct {
	Root, RelPath string
	Size          int64
	MTime         time.Time
	TrackID       int64 // The state's track, or 0.
	Missing       bool  // The state's track is marked missing.
}

// surveyed is what the survey found: the reads to do and the rows to mark.
// The survey only reads; [Scanner.applySurvey] does the writes.
type surveyed struct {
	Jobs         []job
	Mark         []int64             // Track IDs to mark missing.
	IssueDeletes map[string][]string // Root → relative paths.
	Temps        []string            // Absolute paths of temp files left by an interrupted edit.
	Seen         int
	SkippedRoots int
	HoldDeletes  bool // Deleting missing tracks must wait: an outage may be hiding files.
}

// rootSurvey is the survey of one root.
type rootSurvey struct {
	jobs         []job
	mark         []int64
	issueDeletes []string
	temps        []string
	seen         int
	skipped      bool
	hold         bool
}

// seenFile is a regular audio file the walk found.
type seenFile struct {
	rel   string
	size  int64
	mtime time.Time
}

// walked is what walking one root found.
type walked struct {
	files      []seenFile
	temps      []string // Absolute paths of the temp files [isWriteTemp] recognizes.
	unreadable []string // Relative paths of folders and entries the walk could not read.
	rootErr    error    // Set when the root itself is unreadable.
}

// isAudio reports whether name has an extension the scanner reads.
func isAudio(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".flac", ".mp3", ".m4a", ".m4b", ".ogg", ".oga", ".opus", ".wav", ".aif", ".aiff":
		return true
	}
	return false
}

// IsAudio reports whether a scan reads the file name, by its extension. A
// name starting with "." never is.
func IsAudio(name string) bool { return !strings.HasPrefix(name, ".") && isAudio(name) }

// SkipsFolder reports whether a scan leaves the folder name out: a hidden one,
// or a NAS's own, such as "@eaDir".
func SkipsFolder(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "@")
}

// isWriteTemp reports whether name is audiometa's temp file for a rewrite in
// progress, ".<name>.audiometa-<random>.tmp". Scans hold the library lock, so one
// they see was left by a killed edit.
func isWriteTemp(name string) bool {
	return strings.HasPrefix(name, ".") && strings.Contains(name, ".audiometa-") && strings.HasSuffix(name, ".tmp")
}

// survey walks and diffs every root against the store without writing. A
// canceled ctx returns its error: a partial walk must never reach the diff.
func (s *Scanner) survey(ctx context.Context, mode Mode) (*surveyed, error) {
	sv := &surveyed{IssueDeletes: map[string][]string{}}
	audible := 0 // Roots that were walked and held audio.
	for _, root := range s.live().Roots {
		r, err := s.surveyRoot(ctx, root, mode)
		if err != nil {
			return nil, err
		}
		sv.Jobs = append(sv.Jobs, r.jobs...)
		sv.Mark = append(sv.Mark, r.mark...)
		sv.Temps = append(sv.Temps, r.temps...)
		if len(r.issueDeletes) > 0 {
			sv.IssueDeletes[root] = r.issueDeletes
		}
		sv.Seen += r.seen
		sv.HoldDeletes = sv.HoldDeletes || r.hold
		switch {
		case r.skipped:
			sv.SkippedRoots++
		case r.seen > 0:
			audible++
		}
	}
	if err := s.removedRoots(ctx, sv, audible == len(s.live().Roots)); err != nil {
		return nil, err
	}
	return sv, nil
}

// surveyRoot walks root and diffs it against the store. It skips the root,
// touching nothing, when what it sees looks like an outage and not a change.
func (s *Scanner) surveyRoot(ctx context.Context, root string, mode Mode) (rootSurvey, error) {
	states, err := s.st.FileStates(ctx, root)
	if err != nil {
		return rootSurvey{}, fmt.Errorf("root %s: %w", root, err)
	}
	w, err := walkRoot(ctx, root)
	if err != nil {
		return rootSurvey{}, err
	}
	if w.rootUnreadable() {
		s.log.ErrorContext(ctx, "library root is unreadable; skipping it", "root", root, "err", w.rootErr)
		return skipped(states), nil
	}
	if len(w.files) == 0 && countTracks(states) > 0 {
		s.log.ErrorContext(ctx, "library root holds no audio files but the database has tracks under it; "+
			"is a disk unmounted? skipping it. If you emptied it on purpose, remove it on the Server page",
			"root", root, "tracks", countTracks(states))
		return skipped(states), nil
	}
	d := diffRoot(root, states, &w, mode, s.reader)
	r := d.rootSurvey
	r.temps = w.temps
	if massDisappearance(d.absent, d.live, countNew(r.jobs)) {
		s.log.ErrorContext(ctx, "most of a library root's tracks vanished; is a folder or pool member unmounted? skipping it. "+
			"If that is deliberate, remove the folder on the Server page, let that scan run, then add it back; "+
			"files still present reappear and keep their IDs",
			"root", root, "vanished", d.absent, "live", d.live, "newFiles", countNew(r.jobs))
		return skipped(states), nil
	}
	if len(w.unreadable) > 0 {
		s.log.WarnContext(ctx, "some library folders or files are unreadable; leaving their tracks alone",
			"root", root, "paths", w.unreadable)
	}
	return r, nil
}

// skipped is the survey of a root that is left alone. It holds deletes when a
// track there is already missing, because the outage may be hiding it.
func skipped(states map[string]store.FileState) rootSurvey {
	r := rootSurvey{skipped: true}
	for _, st := range states {
		if st.TrackID != 0 && st.Missing {
			r.hold = true
			break
		}
	}
	return r
}

// countTracks counts the states that are tracks, missing or not.
func countTracks(states map[string]store.FileState) int {
	n := 0
	for _, st := range states {
		if st.TrackID != 0 {
			n++
		}
	}
	return n
}

// countNew counts jobs for files the store has no track for.
func countNew(jobs []job) int {
	n := 0
	for i := range jobs {
		if jobs[i].TrackID == 0 {
			n++
		}
	}
	return n
}

// massDisappearance reports whether so many live tracks lost their files,
// with so few new files to take their place, that it looks like an outage.
func massDisappearance(absent, live, newFiles int) bool {
	return absent >= massFloor && absent*massAbsentOver > live && newFiles*massNewUnder < absent
}

// walkRoot lists the audio files under root. It returns an error only when
// ctx ends.
func walkRoot(ctx context.Context, root string) (walked, error) {
	var w walked
	// WalkDir does not follow a symlink root, so resolve it first.
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		w.rootErr = err
		return w, nil //nolint:nilerr // An unreadable root is a finding for the caller to act on, not a failed walk.
	}
	walkErr := filepath.WalkDir(resolved, func(path string, d fs.DirEntry, err error) error {
		return w.visit(ctx, resolved, path, d, err)
	})
	if err = ctx.Err(); err != nil {
		return walked{}, fmt.Errorf("walk %s: %w", root, err)
	}
	if walkErr != nil && w.rootErr == nil {
		w.rootErr = walkErr
	}
	return w, nil
}

// rootUnreadable reports whether the root itself could not be read.
func (w *walked) rootUnreadable() bool { return w.rootErr != nil }

// visit handles one entry of the walk.
func (w *walked) visit(ctx context.Context, resolved, path string, d fs.DirEntry, err error) error {
	if cerr := ctx.Err(); cerr != nil {
		return cerr //nolint:wrapcheck // walkRoot wraps the context error with the root.
	}
	if path == resolved {
		return w.visitRoot(d, err)
	}
	rel, relErr := filepath.Rel(resolved, path)
	if relErr != nil {
		w.rootErr = relErr
		return filepath.SkipAll
	}
	if err != nil {
		w.unreadable = append(w.unreadable, rel)
		return nil //nolint:nilerr // An unreadable entry is recorded and the walk goes on; its tracks stay as they are.
	}
	return w.visitEntry(path, rel, d)
}

// visitRoot checks the root itself: it must be a readable directory.
func (w *walked) visitRoot(d fs.DirEntry, err error) error {
	switch {
	case err != nil:
		w.rootErr = err
	case !d.IsDir():
		w.rootErr = errRootNotDir
	default:
		return nil
	}
	return filepath.SkipAll
}

// visitEntry filters one readable entry below the root. Below the root,
// folders starting with "." or "@" (Synology's @eaDir) and files starting
// with "." (macOS ._ files) are skipped.
func (w *walked) visitEntry(path, rel string, d fs.DirEntry) error {
	name := d.Name()
	switch {
	case d.IsDir() && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "@")):
		return filepath.SkipDir
	case d.Type().IsRegular() && isWriteTemp(name):
		w.temps = append(w.temps, path)
		return nil
	case d.IsDir(), strings.HasPrefix(name, "."), !isAudio(name):
		return nil
	}
	w.addFile(path, rel, d)
	return nil
}

// addFile records the regular file at path. A symlink counts when it leads to
// one. A file that can't be statted is unreadable, so its track stays live.
func (w *walked) addFile(path, rel string, d fs.DirEntry) {
	info, err := d.Info()
	if d.Type()&fs.ModeSymlink != 0 {
		info, err = os.Stat(path)
	}
	switch {
	case err != nil:
		w.unreadable = append(w.unreadable, rel)
	case info.Mode().IsRegular(): // A FIFO would block a read past its context.
		w.files = append(w.files, seenFile{rel: rel, size: info.Size(), mtime: info.ModTime()})
	}
}

// diff is a root's survey with the counts the outage checks need.
type diff struct {
	rootSurvey

	absent int // Live tracks with no file.
	live   int // All live tracks.
}

// diffRoot compares the walk with the stored states.
func diffRoot(root string, states map[string]store.FileState, w *walked, mode Mode, reader string) diff {
	var r rootSurvey
	seen := make(map[string]struct{}, len(w.files))
	for i := range w.files {
		f := &w.files[i]
		seen[f.rel] = struct{}{}
		st := states[f.rel]
		if mode == Quick && unchanged(&st, f, reader) {
			continue
		}
		r.jobs = append(r.jobs, job{
			Root: root, RelPath: f.rel, Size: f.size, MTime: f.mtime, TrackID: st.TrackID, Missing: st.Missing,
		})
	}
	r.seen = len(w.files)
	gone := absentPaths(states, seen, w.unreadable)
	r.mark, r.issueDeletes, r.hold = gone.mark, gone.issues, gone.hold
	live := 0
	for _, st := range states {
		if st.TrackID != 0 && !st.Missing {
			live++
		}
	}
	return diff{rootSurvey: r, absent: len(r.mark), live: live}
}

// unchanged reports whether the file is what the store already knows: the
// state is not missing, size and mtime (to the millisecond) agree, and a scan
// issue was raised by this reader, which would judge the file the same again.
func unchanged(st *store.FileState, f *seenFile, reader string) bool {
	return !st.Missing && st.Size == f.size && st.MTime.UnixMilli() == f.mtime.UnixMilli() &&
		(!st.Issue || st.Reader == reader)
}

// absence is what vanished from a root.
type absence struct {
	mark   []int64  // Live tracks whose files are gone.
	issues []string // Scan issues whose files are gone.
	hold   bool     // A missing track sits behind an unreadable path.
}

// absentPaths returns the live tracks and the issues whose paths were not
// seen, in path order. Paths at or under an unreadable path are left alone;
// hold is true when one of those is a missing track.
func absentPaths(states map[string]store.FileState, seen map[string]struct{}, unreadable []string) absence {
	var a absence
	for _, rel := range slices.Sorted(maps.Keys(states)) {
		if _, ok := seen[rel]; ok {
			continue
		}
		st := states[rel]
		if underAny(rel, unreadable) {
			a.hold = a.hold || (st.TrackID != 0 && st.Missing)
			continue
		}
		if st.TrackID != 0 && !st.Missing {
			a.mark = append(a.mark, st.TrackID)
		}
		if st.Issue {
			a.issues = append(a.issues, rel)
		}
	}
	return a
}

// underAny reports whether rel is one of dirs or lies below one.
func underAny(rel string, dirs []string) bool {
	for _, d := range dirs {
		if rel == d || strings.HasPrefix(rel, d+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// removedRoots marks the tracks and issues of roots that are no longer
// configured. It acts only when every configured root was walked and held
// audio: a typo in the config looks exactly like a removed root.
func (s *Scanner) removedRoots(ctx context.Context, sv *surveyed, everyRootWalked bool) error {
	known, err := s.st.KnownRoots(ctx)
	if err != nil {
		return fmt.Errorf("known roots: %w", err)
	}
	removed := slices.DeleteFunc(known, func(k string) bool { return slices.Contains(s.live().Roots, k) })
	if len(removed) == 0 {
		return nil
	}
	if !everyRootWalked {
		s.log.ErrorContext(ctx, "roots left the configuration, but a configured root was unreadable or held no audio; "+
			"leaving the removed roots alone, since a typo in library.roots looks the same",
			"removed", removed)
		return nil
	}
	for _, root := range removed {
		states, stErr := s.st.FileStates(ctx, root)
		if stErr != nil {
			return fmt.Errorf("root %s: %w", root, stErr)
		}
		var none map[string]struct{}
		gone := absentPaths(states, none, nil)
		sv.Mark = append(sv.Mark, gone.mark...)
		if len(gone.issues) > 0 {
			sv.IssueDeletes[root] = gone.issues
		}
		s.log.WarnContext(ctx, "library root left the configuration; marking its tracks missing",
			"root", root, "tracks", len(gone.mark))
	}
	return nil
}

// applySurvey writes what the survey decided: the temp files go, then the
// missing marks, then the issue deletes per root.
func (s *Scanner) applySurvey(ctx context.Context, sv *surveyed, stats *Stats) error {
	s.sweep(ctx, sv.Temps, stats)
	if err := s.st.MarkMissing(ctx, sv.Mark, s.now()); err != nil {
		return err //nolint:wrapcheck // The store's error already says what failed.
	}
	stats.Missing = len(sv.Mark)
	for _, root := range slices.Sorted(maps.Keys(sv.IssueDeletes)) {
		if err := s.st.DeleteScanIssues(ctx, root, sv.IssueDeletes[root]); err != nil {
			return err //nolint:wrapcheck // The store's error already says what failed.
		}
	}
	return nil
}

// sweep removes temp files left by an interrupted edit. Without tag writes it
// leaves them: a library the server may not write to is not its to tidy. A
// failure is logged, because leftover files cost disk, never a scan.
func (s *Scanner) sweep(ctx context.Context, temps []string, stats *Stats) {
	if !s.live().TagWrites {
		return
	}
	for _, path := range temps {
		if err := os.Remove(path); err != nil {
			s.log.WarnContext(ctx, "removing a leftover temp file failed", "err", err)
			continue
		}
		stats.Swept++
	}
}
