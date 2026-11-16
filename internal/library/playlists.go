package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"github.com/simonhull/kitsune/internal/m3u"
	"github.com/simonhull/kitsune/internal/store"
)

// A playlist file (.m3u or .m3u8) in the library becomes a file playlist that
// everyone sees. The walk finds them beside the audio; a scan reads one after
// the songs it read, under the same claim and lock, so a file and the songs it
// names can arrive together. A file is read again only when its size or mtime
// changed, when [m3uVersion] went up, on a full scan, or when the scan added or
// moved songs and the file has lines that found none: an unchanged file costs
// one stat.
//
// A line is matched as a string against the paths the store has, never through
// the file system (see [m3u.Resolve]): exact, then the one track whose path
// folds the same under NFC and case folding. A playlist goes when its file
// does, by the same rules a missing track follows: never for a root that looks
// down.

// m3uVersion is the version of how a playlist file is read and matched. A
// change to either raises it, and every file is read again.
const m3uVersion = 1

// IsPlaylistFile reports whether a scan reads the file name as a playlist, by
// its extension. A name starting with "." never is.
func IsPlaylistFile(name string) bool {
	if strings.HasPrefix(name, ".") {
		return false
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".m3u", ".m3u8":
		return true
	}
	return false
}

// playlistSurvey is what a survey found of one root's playlist files.
type playlistSurvey struct {
	root   string
	seen   []seenFile
	states map[string]store.FilePlaylistState
	gone   []string // Stored files the survey saw are gone.
}

// surveyPlaylists diffs the playlist files w found under root with the store:
// a stored file is gone when the walk didn't see it, unless covers says the
// walk said nothing about its path or it lies under an unreadable path.
func (s *Scanner) surveyPlaylists(ctx context.Context, root string, w *walked, covers func(string) bool) (playlistSurvey, error) {
	states, err := s.st.FilePlaylistStates(ctx, root)
	if err != nil {
		return playlistSurvey{}, fmt.Errorf("root %s: %w", root, err)
	}
	p := playlistSurvey{root: root, seen: w.playlists, states: states}
	seen := make(map[string]bool, len(w.playlists))
	for i := range w.playlists {
		seen[w.playlists[i].rel] = true
	}
	for _, rel := range slices.Sorted(maps.Keys(states)) {
		if !seen[rel] && covers(rel) && !underAny(rel, w.unreadable) {
			p.gone = append(p.gone, rel)
		}
	}
	return p, nil
}

// waitingPlaylists returns, for every root, the stored playlist files with
// lines that found no song, each as it is now, leaving out those surveyed
// already holds. A file that can't be statted is left to the next full scan.
func (s *Scanner) waitingPlaylists(ctx context.Context, surveyed *playlistSurvey) ([]playlistSurvey, error) {
	var out []playlistSurvey
	for _, root := range s.live().Roots {
		states, err := s.st.FilePlaylistStates(ctx, root)
		if err != nil {
			return nil, err //nolint:wrapcheck // The store's error already says what failed.
		}
		sv := playlistSurvey{root: root, states: states}
		for _, rel := range slices.Sorted(maps.Keys(states)) {
			if states[rel].NotFound == 0 || (root == surveyed.root && slices.ContainsFunc(surveyed.seen, func(f seenFile) bool { return f.rel == rel })) {
				continue
			}
			if f, ok := statUnder(root, rel); ok {
				sv.seen = append(sv.seen, f)
			}
		}
		if len(sv.seen) > 0 {
			out = append(out, sv)
		}
	}
	return out, nil
}

// statUnder stats the file at rel under root through [os.Root].
func statUnder(root, rel string) (seenFile, bool) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return seenFile{}, false
	}
	defer r.Close() //nolint:errcheck // Nothing was written.
	info, err := r.Stat(rel)
	if err != nil || !info.Mode().IsRegular() {
		return seenFile{}, false
	}
	return seenFile{rel: rel, size: info.Size(), mtime: info.ModTime()}, true
}

// every says a walk of a whole root covers every path under it.
func every(string) bool { return true }

// readPlaylists deletes the playlists whose files are gone and reads the files
// that need it, then tells the notifier which playlists changed. changed says
// the scan added or moved songs, which may be what a file's unmatched lines
// name.
func (s *Scanner) readPlaylists(ctx context.Context, surveys []playlistSurvey, mode Mode, changed bool, stats *Stats) error {
	pr := &playlistReads{s: s, stats: stats}
	defer func() { s.notifyPlaylists(pr.touched) }()
	for i := range surveys {
		sv := &surveys[i]
		if len(sv.gone) > 0 {
			ids, err := s.st.DeleteFilePlaylists(ctx, sv.root, sv.gone)
			if err != nil {
				return err //nolint:wrapcheck // The store's error already says what failed.
			}
			pr.touched = append(pr.touched, ids...)
			stats.PlaylistsGone += len(ids)
		}
		if err := pr.root(ctx, sv, mode, changed); err != nil {
			return err
		}
	}
	return nil
}

// playlistReads is the reads of one scan's playlist files.
type playlistReads struct {
	s       *Scanner
	stats   *Stats
	m       *matcher // Made for the first file read.
	touched []string // The playlists read or deleted.
}

// root reads the files of sv that need it. A file that can't be read becomes a
// scan issue, and one that reads loses its issue.
func (pr *playlistReads) root(ctx context.Context, sv *playlistSurvey, mode Mode, changed bool) error {
	var read []string
	for i := range sv.seen {
		f := &sv.seen[i]
		if !needsRead(sv.states, f, mode, changed) {
			continue
		}
		ok, err := pr.file(ctx, sv, f)
		if err != nil {
			return err
		}
		if ok {
			read = append(read, f.rel)
		}
	}
	if len(read) == 0 {
		return nil
	}
	return pr.s.st.DeleteScanIssues(ctx, sv.root, read) //nolint:wrapcheck // The store's error already says what failed.
}

// file reads the file f of sv and reports whether it read; one it can't read
// is recorded as a scan issue.
func (pr *playlistReads) file(ctx context.Context, sv *playlistSurvey, f *seenFile) (bool, error) {
	if pr.m == nil {
		var err error
		if pr.m, err = pr.s.newMatcher(ctx); err != nil {
			return false, err
		}
	}
	id, err := pr.s.readPlaylist(ctx, pr.m, sv.root, f, sv.states[f.rel])
	switch {
	case err == nil:
		pr.touched = append(pr.touched, id)
		pr.stats.Playlists++
		return true, nil
	case ctx.Err() != nil:
		return false, fmt.Errorf("playlist %s: %w", f.rel, ctx.Err())
	}
	pr.stats.Issues++
	return false, pr.s.playlistIssue(ctx, sv.root, f, err)
}

// needsRead reports whether the playlist file f must be read: it is new to the
// store, its size or mtime changed, an older reader read it, the scan reads
// everything, or it has lines that found no song and songs came in.
func needsRead(states map[string]store.FilePlaylistState, f *seenFile, mode Mode, changed bool) bool {
	st, ok := states[f.rel]
	return !ok || mode == Full || st.Size != f.size || st.MTime.UnixMilli() != f.mtime.UnixMilli() ||
		st.Reader != m3uVersion || (changed && st.NotFound > 0)
}

// playlistIssue records a playlist file that couldn't be read as an unreadable
// scan issue, which the next scan tries again.
func (s *Scanner) playlistIssue(ctx context.Context, root string, f *seenFile, err error) error {
	s.log.WarnContext(ctx, "a playlist file could not be read", "root", root, "path", f.rel, "err", err)
	is := store.ScanIssue{
		Root: root, RelPath: f.rel, Kind: store.IssueUnreadable, Size: retrySize, MTime: f.mtime,
		Message: strings.ToValidUTF8(err.Error(), "�"), Reader: s.reader,
	}
	if werr := s.st.WriteBatch(ctx, &store.Batch{Issues: []store.ScanIssue{is}}); werr != nil {
		return werr //nolint:wrapcheck // The store's error already says what failed.
	}
	return nil
}

// notifyPlaylists tells the notifier which file playlists were read, read
// again or deleted.
func (s *Scanner) notifyPlaylists(ids []string) {
	if len(ids) > 0 {
		s.notify.PlaylistsChanged(slices.Compact(slices.Sorted(slices.Values(ids))))
	}
}

// readPlaylist reads the playlist file f under root, matches its lines and
// stores it, and returns the playlist's ULID. st is what the store knew of it.
func (s *Scanner) readPlaylist(ctx context.Context, m *matcher, root string, f *seenFile, st store.FilePlaylistState) (string, error) {
	data, err := readPlaylistBytes(root, f.rel)
	if err != nil {
		return "", err
	}
	rel := filepath.ToSlash(f.rel)
	p := m3u.Parse(data, strings.EqualFold(path.Ext(rel), ".m3u8"), false)
	var prev map[string]int64 // Read the first time a line needs it.
	var prevErr error
	before := func(line string) (int64, bool) {
		if prev == nil && prevErr == nil && st.Row != 0 {
			prev, prevErr = s.st.FileEntryLines(ctx, st.Row)
		}
		row, ok := prev[line]
		return row, ok
	}
	newID, err := NewID(s.now())
	if err != nil {
		return "", err
	}
	w := &store.FilePlaylistWrite{
		Root: root, RelPath: f.rel, Size: f.size, MTime: f.mtime, Reader: m3uVersion, NewID: newID,
		Name: playlistName(p.Name, rel), Lines: len(p.Lines), PastLimit: p.PastLimit,
	}
	m.match(w, &p, root, path.Dir(rel), before, s.live().Roots)
	if prevErr != nil {
		return "", prevErr //nolint:wrapcheck // The store's error already says what failed.
	}
	id, err := s.st.WriteFilePlaylist(ctx, w)
	if err != nil {
		return "", err //nolint:wrapcheck // The store's error already says what failed.
	}
	return id, nil
}

// readPlaylistBytes reads at most one byte past [m3u.MostBytes] of the file at
// rel under root, through [os.Root], so nothing outside the root is read.
func readPlaylistBytes(root, rel string) ([]byte, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open root: %w", err)
	}
	defer r.Close() //nolint:errcheck // Read-only.
	f, err := r.Open(rel)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close() //nolint:errcheck // Read-only.
	data, err := io.ReadAll(io.LimitReader(f, m3u.MostBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return data, nil
}

// playlistName is the name a playlist file gives: its #PLAYLIST, else its file
// name without the extension, cut to a playlist name's length.
func playlistName(named, rel string) string {
	name := strings.TrimSpace(named)
	if name == "" {
		base := path.Base(rel)
		name = strings.TrimSpace(strings.TrimSuffix(base, path.Ext(base)))
	}
	if runes := []rune(name); len(runes) > store.MostPlaylistName {
		name = strings.TrimSpace(string(runes[:store.MostPlaylistName]))
	}
	if name == "" {
		return path.Base(rel)
	}
	return name
}

// pathKey is a path under a root.
type pathKey struct{ root, rel string }

// matcher finds the track a playlist line names, by the paths the store has.
type matcher struct {
	tracks []store.TrackPath
	exact  map[pathKey]store.TrackPath // Live tracks win over others at a path.
	// folded and stems are built the first time a line finds no song at its
	// exact path, since most never do.
	folded map[pathKey][]int64  // Rows by their folded path.
	stems  map[pathKey][]string // Live tracks' file names by folder and folded name without extension.
	caser  cases.Caser
}

// newMatcher reads every track's path.
func (s *Scanner) newMatcher(ctx context.Context) (*matcher, error) {
	tracks, err := s.st.TrackPaths(ctx)
	if err != nil {
		return nil, err //nolint:wrapcheck // The store's error already says what failed.
	}
	return newMatcherOf(tracks), nil
}

func newMatcherOf(tracks []store.TrackPath) *matcher {
	m := &matcher{tracks: tracks, exact: make(map[pathKey]store.TrackPath, len(tracks)), caser: cases.Fold()}
	for i := range tracks {
		t := &tracks[i]
		t.RelPath = filepath.ToSlash(t.RelPath)
		k := pathKey{t.Root, t.RelPath}
		if had, ok := m.exact[k]; !ok || !had.Live {
			m.exact[k] = *t
		}
	}
	return m
}

// index builds folded and stems, once.
func (m *matcher) index() {
	if m.folded != nil {
		return
	}
	m.folded = make(map[pathKey][]int64, len(m.tracks))
	m.stems = make(map[pathKey][]string, len(m.tracks))
	for i := range m.tracks {
		t := &m.tracks[i]
		fk := pathKey{t.Root, m.fold(t.RelPath)}
		if !slices.Contains(m.folded[fk], t.Row) {
			m.folded[fk] = append(m.folded[fk], t.Row)
		}
		if t.Live {
			sk := m.stemKey(t.Root, t.RelPath)
			m.stems[sk] = append(m.stems[sk], path.Base(t.RelPath))
		}
	}
}

// fold returns a path under NFC and case folding: for ASCII, its lower case.
func (m *matcher) fold(rel string) string {
	ascii := true
	for i := range len(rel) {
		if rel[i] >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii {
		return strings.ToLower(rel)
	}
	return norm.NFC.String(m.caser.String(norm.NFC.String(rel)))
}

// stemKey keys a path by its folder and its folded name without extension.
func (m *matcher) stemKey(root, rel string) pathKey {
	base := path.Base(rel)
	return pathKey{root, path.Dir(rel) + "/" + m.fold(strings.TrimSuffix(base, path.Ext(base)))}
}

// find returns the row of the track at rel under root: the one there, else
// the only one whose path folds the same. Two or more is none: never a guess.
func (m *matcher) find(root, rel string) (int64, bool) {
	if t, ok := m.exact[pathKey{root, rel}]; ok {
		return t.Row, true
	}
	m.index()
	if rows := m.folded[pathKey{root, m.fold(rel)}]; len(rows) == 1 {
		return rows[0], true
	}
	return 0, false
}

// near returns the file name of a live song in rel's folder with its name and
// another extension: a hint for a line that found no song, never a match. With
// several, the first by name.
func (m *matcher) near(root, rel string) string {
	m.index()
	ext := m.fold(path.Ext(rel))
	var out string
	for _, name := range m.stems[m.stemKey(root, rel)] {
		if m.fold(path.Ext(name)) != ext && (out == "" || name < out) {
			out = name
		}
	}
	return out
}

// match fills w with what each line of p names: an entry for a line that found
// a song, a missing line for one that didn't. The file is at dir under root;
// before finds the song a line found in the file's last read, if still live,
// which the line keeps when its path no longer names one.
func (m *matcher) match(w *store.FilePlaylistWrite, p *m3u.Playlist, root, dir string, before func(string) (int64, bool), roots []string) {
	for i := range p.Lines {
		l := &p.Lines[i]
		ref := m3u.Resolve(l.Path, root, dir, roots)
		missing := store.MissingLine{Line: l.Number, Text: ref.Shown, Title: l.Title, Seconds: l.Seconds}
		switch ref.Kind {
		case m3u.NotFile:
			missing.Reason = store.MissingNotFile
		case m3u.Outside:
			missing.Reason = store.MissingOutside
		case m3u.InLibrary:
			row, ok := m.find(ref.Root, ref.Rel)
			if !ok {
				row, ok = before(l.Path)
			}
			if ok {
				w.Entries = append(w.Entries, store.FileEntry{Line: l.Number, Text: l.Path, Track: row})
				continue
			}
			missing.Reason, missing.OtherExtension = store.MissingNoSong, m.near(ref.Root, ref.Rel)
		}
		w.Missing = append(w.Missing, missing)
	}
}

// errPlaylistExists is what an [ExistsError] unwraps to.
var errPlaylistExists = errors.New("a file is there already")

// ExistsError is a save that found a file where it would write. It names
// neither the folder nor the root.
type ExistsError struct {
	ModifiedAt time.Time
	// FreeName is the next name free in the folder: "Falling Up 2.m3u8" for
	// "Falling Up.m3u8", or empty when none of the first [mostFreeTries] is.
	FreeName string
}

func (e *ExistsError) Error() string { return errPlaylistExists.Error() }

func (*ExistsError) Unwrap() error { return errPlaylistExists }

// mostFreeTries is how many numbered names a save tries for a free one.
const mostFreeTries = 1000

// freeName returns the first of "<name> 2<ext>", "<name> 3<ext>", … that is
// free beside rel under r, or "" when none of the first [mostFreeTries] is.
func freeName(r *os.Root, rel string) string {
	base := filepath.Base(rel)
	stem, ext := strings.TrimSuffix(base, filepath.Ext(base)), filepath.Ext(base)
	for n := 2; n <= mostFreeTries; n++ {
		name := fmt.Sprintf("%s %d%s", stem, n, ext)
		if _, err := r.Lstat(filepath.Join(filepath.Dir(rel), name)); errors.Is(err, fs.ErrNotExist) {
			return name
		}
	}
	return ""
}

// ErrBadFolder means a save named a folder outside a library root, or a root
// that isn't one.
var ErrBadFolder = errors.New("not a folder in a library root")

// SavedPlaylist is what [Scanner.SavePlaylist] wrote.
type SavedPlaylist struct {
	ID      string // The file playlist read from it.
	RelPath string // The file, relative to its root.
}

// SavePlaylist writes data as the playlist file name in the folder dir under
// root, then reads it at once, as a scan would, so its file playlist is there
// when it returns. The file is written beside its target as
// ".<name>.kitsune-<random>.tmp", synced and renamed over it, all through
// [os.Root], holding the library lock.
//
// It returns [ErrTagWritesOff] while tag writes are off, [ErrBusy] while a scan
// or an edit holds the library, [ErrBadFolder] for a root that isn't one or a
// folder outside it, and an [*ExistsError] when a file is there and replace
// isn't set. Then nothing is written.
func (s *Scanner) SavePlaylist(ctx context.Context, root, dir, name string, data []byte, replace bool) (SavedPlaylist, error) {
	var saved SavedPlaylist
	err := s.exclusive(ctx, func() error {
		if !s.live().TagWrites {
			return ErrTagWritesOff
		}
		if !slices.Contains(s.live().Roots, root) || !insideRoot(dir) {
			return ErrBadFolder
		}
		rel := filepath.Join(dir, name)
		info, err := writePlaylistFile(root, rel, data, replace)
		if err != nil {
			return err
		}
		id, err := s.readSaved(ctx, root, seenFile{rel: rel, size: info.Size(), mtime: info.ModTime()})
		saved = SavedPlaylist{ID: id, RelPath: rel}
		return err
	})
	if err != nil {
		return SavedPlaylist{}, fmt.Errorf("save playlist: %w", err)
	}
	return saved, nil
}

// readSaved reads the playlist file f just written under root.
func (s *Scanner) readSaved(ctx context.Context, root string, f seenFile) (string, error) {
	m, err := s.newMatcher(ctx)
	if err != nil {
		return "", err
	}
	known, err := s.st.FilePlaylistStates(ctx, root)
	if err != nil {
		return "", err //nolint:wrapcheck // The store's error already says what failed.
	}
	id, err := s.readPlaylist(ctx, m, root, &f, known[f.rel])
	if err != nil {
		return "", err
	}
	s.notifyPlaylists([]string{id})
	return id, nil
}

// writePlaylistFile writes data at rel under root through [os.Root]: a temp
// file beside it, synced, then renamed over it. It returns what the file is
// now. Without replace, a file there is an [*ExistsError].
func writePlaylistFile(root, rel string, data []byte, replace bool) (fs.FileInfo, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open root: %w", err)
	}
	defer r.Close() //nolint:errcheck // Only folders were opened here.
	if err = r.MkdirAll(filepath.Dir(rel), trashDirMode); err != nil {
		return nil, fmt.Errorf("make folder: %w", err)
	}
	if !replace {
		if there, statErr := r.Lstat(rel); statErr == nil {
			return nil, &ExistsError{ModifiedAt: there.ModTime(), FreeName: freeName(r, rel)}
		}
	}
	suffix, err := NewID(time.Now())
	if err != nil {
		return nil, err
	}
	tmp := filepath.Join(filepath.Dir(rel), "."+filepath.Base(rel)+".kitsune-"+suffix+".tmp")
	if err = writeSynced(r, tmp, data); err != nil {
		_ = r.Remove(tmp) // Best effort: a scan sweeps one left behind.
		return nil, err
	}
	if err = r.Rename(tmp, rel); err != nil {
		_ = r.Remove(tmp)
		return nil, fmt.Errorf("rename: %w", err)
	}
	info, err := r.Stat(rel)
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}
	return info, nil
}

// writeSynced creates the file name under r, writes data and syncs it.
func writeSynced(r *os.Root, name string, data []byte) error {
	f, err := r.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, playlistFileMode)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write: %w", err)
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync: %w", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	return nil
}

// playlistFileMode is a saved playlist file's mode: readable by every player.
const playlistFileMode = 0o644
