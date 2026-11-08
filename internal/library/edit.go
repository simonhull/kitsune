package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/store"
)

var (
	// ErrStale means an edit named an ETag that is not current, or a file no
	// longer matches what the store knows.
	ErrStale = errors.New("the file changed since it was read")
	// ErrNotWritable means the server may not rewrite a file: it is read-only,
	// its folder is, or it lies outside the library roots.
	ErrNotWritable = errors.New("the file can't be written")
)

// anyWrite is the mode bits of a file somebody may write. Root writes without
// them, so the bits are checked as well as the open.
const anyWrite = 0o222

// probePattern names the file that tests a folder. It matches [isWriteTemp], so
// a crash can't leave one for good.
const probePattern = ".kitsune-probe.audiometa-*.tmp"

// EditError says how far a failed edit got. Each track is atomic and an album is
// not: the tracks in Changed hold the edit and the ones in Remaining don't,
// whatever Err says. Err wraps [ErrUnsupportedEdit], [ErrNotWritable], [ErrStale]
// or something unexpected. A preflight failure leaves Changed empty.
type EditError struct {
	Changed, Remaining []string // Track ULIDs.
	Err                error
}

// Error implements error.
func (e *EditError) Error() string {
	return fmt.Sprintf("edit stopped with %d tracks changed and %d remaining: %v", len(e.Changed), len(e.Remaining), e.Err)
}

// Unwrap returns the cause.
func (e *EditError) Unwrap() error { return e.Err }

// AlbumEdit is a change to the tracks of one album. All goes to every track, and
// Tracks, by ULID, to the ones it names, on top of All: what both name takes the
// track's value.
type AlbumEdit struct {
	All    Edit
	Tracks map[string]Edit
}

// fileEdit is what one file of an album will take.
type fileEdit struct {
	file *store.EditFile
	edit Edit
	// before and after are where the file's track hangs now and will once
	// written, which preflight works out with a dry run of the edit.
	before, after placement
}

// placement is what decides which rows a track hangs off: its album's group
// key and its artists.
type placement struct {
	groupKey              string
	artists, albumArtists []store.ArtistWrite
}

// placementOf returns where the track f is read from will hang.
func placementOf(f *audiometa.File, file *store.EditFile) placement {
	tw := NewTrackWrite(f, &FileInfo{Root: file.Root, RelPath: file.RelPath})
	return placement{groupKey: tw.Album.GroupKey, artists: tw.Artists, albumArtists: tw.AlbumArtists}
}

// forFiles returns the edit of each of files that gets one. A track that isn't
// among files is refused: edits never cross albums.
func (e *AlbumEdit) forFiles(files []store.EditFile) ([]fileEdit, error) {
	for id := range e.Tracks {
		if !slices.ContainsFunc(files, func(f store.EditFile) bool { return f.ULID == id }) {
			return nil, fmt.Errorf("track %s is not on this album: %w", id, ErrUnsupportedEdit)
		}
	}
	var out []fileEdit
	for i := range files {
		own, named := e.Tracks[files[i].ULID]
		if named || !e.All.Empty() {
			out = append(out, fileEdit{file: &files[i], edit: e.All.over(&own)})
		}
	}
	return out, nil
}

// EditAlbum saves e to the files of the album id, and to the store. Each file is
// written and reindexed before the next, so the library follows the files even
// if the edit stops midway. A canceled ctx stops the edit between or inside
// files, never leaving one half written.
//
// It waits out a batch of the fingerprint pass, returns [ErrBusy] while a scan or
// another edit holds the library, and returns an error
// wrapping [store.ErrNotFound] for an unknown ID, [ErrStale] when ifMatch is not
// the current ETag or a file drifted from the store, and an [*EditError] when
// the edit was refused or stopped.
//
// It returns the ID of the album the tracks are on now: id, unless the edit moved
// every one of them, when it is the album the first went to.
func (s *Scanner) EditAlbum(ctx context.Context, id, ifMatch string, e *AlbumEdit) (string, error) {
	landed := id
	err := s.exclusive(ctx, func() error {
		if !s.live().TagWrites {
			return ErrTagWritesOff
		}
		files, etag, err := s.st.AlbumEditFiles(ctx, id)
		if err != nil {
			return err //nolint:wrapcheck // The store's error already says what failed.
		}
		if ifMatch != etag {
			return ErrStale
		}
		edits, err := e.forFiles(files)
		if err != nil {
			return &EditError{Remaining: ulids(fileEdits(files)), Err: err}
		}
		err = s.edit(ctx, edits, map[int64]int{files[0].Album: len(files)})
		if err != nil {
			return err
		}
		landed, err = s.landed(context.WithoutCancel(ctx), id, files[0].ULID)
		return err
	})
	return landed, err
}

// landed returns the album the edit of album id left its tracks on: id while it
// has any, else the album of its first track.
func (s *Scanner) landed(ctx context.Context, id, first string) (string, error) {
	_, _, err := s.st.AlbumEditFiles(ctx, id)
	if !errors.Is(err, store.ErrNotFound) {
		return id, err //nolint:wrapcheck // The store's error already says what failed.
	}
	return s.st.TrackAlbum(ctx, first) //nolint:wrapcheck // As above.
}

// fileEdits lists files with no edit, for reporting.
func fileEdits(files []store.EditFile) []fileEdit {
	out := make([]fileEdit, len(files))
	for i := range files {
		out[i].file = &files[i]
	}
	return out
}

// edit checks every file, and only then writes them in order. live counts the
// live tracks of each album the files are on. The caller holds the library lock.
func (s *Scanner) edit(ctx context.Context, edits []fileEdit, live map[int64]int) error {
	for i := range edits {
		err := checkDrift(edits[i].file)
		if errors.Is(err, ErrStale) {
			return ErrStale
		}
		if err == nil {
			err = s.preflight(ctx, &edits[i])
		}
		if err != nil {
			return stopped(edits, 0, err)
		}
	}
	undo, err := s.moveRows(context.WithoutCancel(ctx), edits, live)
	if err != nil {
		return stopped(edits, 0, err)
	}
	// Tracks written before a failure hold the edit, and a canceled edit still counts.
	// A rewritten file loses its fingerprint, which the pass makes again.
	defer s.askForPass()
	written := 0
	defer func() { s.settle(ctx, edits[:written]) }()
	for i := range edits {
		file := edits[i].file
		path := filepath.Join(file.Root, file.RelPath)
		if err = s.write(ctx, path, edits[i].edit.apply); err != nil {
			if i == 0 {
				undo()
			}
			return stopped(edits, i, err)
		}
		written = i + 1
		// The file has changed, so a canceled edit still indexes it.
		if err = s.reindex(context.WithoutCancel(ctx), file.ID, file.Root, file.RelPath); err != nil {
			return stopped(edits, i+1, err)
		}
	}
	return nil
}

// rekeys returns the albums an edit renames whole: every live track of the album
// is edited, and all of them will group under one new key.
func rekeys(edits []fileEdit, live map[int64]int) []store.AlbumRekey {
	type move struct {
		from, to string
		tracks   int
		split    bool
	}
	moves := make(map[int64]*move)
	var order []int64
	for i := range edits {
		file, to := edits[i].file, edits[i].after.groupKey
		m, seen := moves[file.Album]
		if !seen {
			m = &move{from: file.GroupKey, to: to}
			moves[file.Album] = m
			order = append(order, file.Album)
		}
		m.tracks++
		m.split = m.split || m.to != to
	}
	var out []store.AlbumRekey
	for _, album := range order {
		if m := moves[album]; !m.split && m.to != m.from && m.tracks == live[album] {
			out = append(out, store.AlbumRekey{Album: album, To: m.to})
		}
	}
	return out
}

// moveRows renames, before anything is written, the rows an edit renames whole,
// so its tracks land back on them as they are written and a retry after a failure
// finds them there: an album every live track of which moves to one new key, and
// an artist every link of which moves to one new name. They keep their ULIDs,
// first-seen times, covers and hearts. It returns what puts them back, for an edit
// that then changes no file. A failure to put them back is logged: the rows keep
// the new names until a full scan reads their files.
func (s *Scanner) moveRows(ctx context.Context, edits []fileEdit, live map[int64]int) (func(), error) {
	albums, err := s.st.RekeyAlbums(ctx, rekeys(edits, live))
	if err != nil {
		return nil, err //nolint:wrapcheck // The store's error already says what failed.
	}
	artists, err := s.st.RenameArtists(ctx, renames(edits))
	if err != nil {
		if _, undoErr := s.st.RekeyAlbums(ctx, albums); undoErr != nil {
			s.log.WarnContext(ctx, "undoing an album rename failed", "err", undoErr)
		}
		return nil, err //nolint:wrapcheck // As above.
	}
	return func() {
		_, artistErr := s.st.RenameArtists(ctx, artists)
		_, albumErr := s.st.RekeyAlbums(ctx, albums)
		if joined := errors.Join(artistErr, albumErr); joined != nil {
			s.log.WarnContext(ctx, "undoing a rename failed", "err", joined)
		}
	}, nil
}

// renames returns the artists an edit renames on every track it touches that
// names them: in the same place of the same role, one name becomes one other,
// and nowhere does it stay. Whether those are all of an artist's links is the
// store's to check, which it is given the count of.
func renames(edits []fileEdit) []store.ArtistRename {
	type rename struct {
		to    store.ArtistWrite
		links int
		kept  bool // Some track keeps the old name, or renames it otherwise.
	}
	byKey := make(map[string]*rename)
	var order []string
	note := func(from, to *store.ArtistWrite) {
		r, seen := byKey[from.MatchKey]
		if !seen {
			r = &rename{to: *to}
			byKey[from.MatchKey] = r
			order = append(order, from.MatchKey)
		}
		r.links++
		r.kept = r.kept || to.MatchKey == from.MatchKey || to.MatchKey != r.to.MatchKey
	}
	for i := range edits {
		before, after := &edits[i].before, &edits[i].after
		for _, pair := range [][2][]store.ArtistWrite{{before.artists, after.artists}, {before.albumArtists, after.albumArtists}} {
			old, now := pair[0], pair[1]
			for j := range old {
				if len(old) == len(now) {
					note(&old[j], &now[j])
				} else {
					note(&old[j], &old[j]) // Gone or moved: not a rename.
				}
			}
		}
	}
	var out []store.ArtistRename
	for _, key := range order {
		if r := byKey[key]; !r.kept {
			out = append(out, store.ArtistRename{From: key, To: r.to.MatchKey, Name: r.to.Name, SortName: r.to.SortName, MBID: r.to.MBID, Links: r.links})
		}
	}
	return out
}

// settle brings what the store derives from its tracks up to date, once per edit
// and with the lock held, however the edit ended: written are the files that
// changed. An album they all left merges into the one they went to, and what no
// track uses any more goes. A failure is logged: the files and tracks are right,
// and the next scan prunes what is left.
func (s *Scanner) settle(ctx context.Context, written []fileEdit) {
	ctx = context.WithoutCancel(ctx)
	moves := albumMoves(written)
	if len(moves) > 0 {
		if _, err := s.st.MergeEmptied(ctx, moves); err != nil {
			s.log.WarnContext(ctx, "merging albums after an edit failed", "err", err)
		}
	}
	s.settleAlbums(ctx, written, moves)
	if pruned, err := s.st.PruneOrphans(ctx); err != nil {
		s.log.WarnContext(ctx, "pruning after an edit failed", "err", err)
	} else {
		s.deleteArtwork(ctx, pruned.Artwork)
	}
	s.refreshNames(ctx)
	s.refreshGenres(ctx)
}

// settleAlbums gives every album the written tracks were on or went to the
// fields of its first track, and every album that gained or lost tracks the
// cover the scanner would find it: tracks are reindexed one at a time and without
// art, so neither the last one written nor the old cover decides.
func (s *Scanner) settleAlbums(ctx context.Context, written []fileEdit, moves []store.AlbumMove) {
	tracks := make([]int64, len(written))
	var albums []int64
	for i := range written {
		tracks[i] = written[i].file.ID
		if a := written[i].file.Album; !slices.Contains(albums, a) {
			albums = append(albums, a)
		}
	}
	now, err := s.st.TrackAlbumRows(ctx, tracks)
	if err == nil {
		err = s.st.SettleAlbums(ctx, unique(albums, now))
	}
	if err != nil {
		s.log.WarnContext(ctx, "settling albums after an edit failed", "err", err)
		return
	}
	var left, moved []int64
	for i := range moves {
		left = append(left, moves[i].From)
		moved = append(moved, moves[i].Tracks...)
	}
	joined, err := s.st.TrackAlbumRows(ctx, moved)
	if err != nil {
		s.log.WarnContext(ctx, "settling covers after an edit failed", "err", err)
		return
	}
	for _, album := range unique(left, joined) {
		s.settleCover(ctx, album)
	}
}

// unique returns the values of lists, each once, in order.
func unique(lists ...[]int64) []int64 {
	var out []int64
	for _, list := range lists {
		for _, v := range list {
			if !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
	}
	return out
}

// settleCover gives the album the cover of its first track that has one, as a
// scan would. With none, it keeps the one it has. Art never fails an edit: a
// failure is logged.
func (s *Scanner) settleCover(ctx context.Context, album int64) {
	if s.artwork == nil {
		return
	}
	files, err := s.st.AlbumCoverFiles(ctx, album)
	if err != nil {
		s.log.WarnContext(ctx, "finding an album's cover after an edit failed", "err", err)
		return
	}
	for _, file := range files {
		art := s.coverOf(ctx, file)
		if art == nil {
			continue
		}
		if err = s.st.SetAlbumArtwork(ctx, album, art); err != nil {
			s.log.WarnContext(ctx, "storing an album's cover after an edit failed", "err", err)
		}
		return
	}
}

// coverOf returns the cover the file offers its album, or nil.
func (s *Scanner) coverOf(ctx context.Context, file store.CoverFile) *store.ArtworkWrite {
	f, err := audiometa.OpenContext(ctx, filepath.Join(file.Root, file.RelPath), audiometa.WithMaxArtworkSize(maxArtworkBytes))
	if err != nil {
		return nil // The track was just written; the next scan reports a file gone bad.
	}
	defer func() { _ = f.Close() }() // Only read.
	look := search{covers: newCovers(s.artwork, s.log), f: f, j: &job{Root: file.Root, RelPath: file.RelPath}}
	return look.run(ctx)
}

// albumMoves lists the tracks written from each album that a written track may
// have left: the ones whose group key changed.
func albumMoves(written []fileEdit) []store.AlbumMove {
	var out []store.AlbumMove
	for i := range written {
		file := written[i].file
		if written[i].after.groupKey == file.GroupKey {
			continue
		}
		j := slices.IndexFunc(out, func(m store.AlbumMove) bool { return m.From == file.Album })
		if j < 0 {
			out = append(out, store.AlbumMove{From: file.Album})
			j = len(out) - 1
		}
		out[j].Tracks = append(out[j].Tracks, file.ID)
	}
	return out
}

// stopped reports an edit that ended with edits[:changed] written.
func stopped(edits []fileEdit, changed int, err error) *EditError {
	return &EditError{Changed: ulids(edits[:changed]), Remaining: ulids(edits[changed:]), Err: classify(err)}
}

func ulids(edits []fileEdit) []string {
	out := make([]string, len(edits))
	for i := range edits {
		out[i] = edits[i].file.ULID
	}
	return out
}

// classify wraps err in the sentinel a caller branches on.
func classify(err error) error {
	switch {
	case errors.Is(err, ErrUnsupportedEdit), errors.Is(err, ErrNotWritable):
		return err
	case errors.Is(err, audiometa.ErrConcurrentModification):
		return fmt.Errorf("%w: %w", ErrStale, err)
	case errors.Is(err, audiometa.ErrUnsupportedWrite), errors.Is(err, audiometa.ErrDRMProtected), errors.Is(err, audiometa.ErrTagTooLarge),
		errors.Is(err, audiometa.ErrInvalidArtwork), errors.Is(err, audiometa.ErrConflictingEdits), errors.Is(err, audiometa.ErrVerifyFailed):
		return fmt.Errorf("%w: %w", ErrUnsupportedEdit, err)
	case errors.Is(err, fs.ErrPermission), errors.Is(err, syscall.EROFS):
		return fmt.Errorf("%w: %w", ErrNotWritable, err)
	}
	return err
}

// checkDrift returns [ErrStale] when file is not what the store knows.
func checkDrift(file *store.EditFile) error {
	info, err := os.Stat(filepath.Join(file.Root, file.RelPath))
	if errors.Is(err, fs.ErrNotExist) {
		return ErrStale
	}
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}
	if info.Size() != file.Size || info.ModTime().UnixMilli() != file.MTime.UnixMilli() {
		return ErrStale
	}
	return nil
}

// preflight does everything but the write: the target must lie under a
// configured root, the file and its folder must be writable, and the edit must
// go through a dry write to nowhere. It sets where e's track hangs now and will
// hang once the edit is written.
func (s *Scanner) preflight(ctx context.Context, e *fileEdit) error {
	path, err := s.resolve(e.file)
	if err != nil {
		return err
	}
	if writeErr := checkWritable(path); writeErr != nil {
		return writeErr
	}
	f, err := audiometa.OpenContext(ctx, path)
	if err != nil {
		return fmt.Errorf("preflight: %w", err)
	}
	defer func() { _ = f.Close() }() // Only read.
	e.before = placementOf(f, e.file)
	if applyErr := e.edit.apply(f); applyErr != nil {
		return applyErr
	}
	if _, err = f.WriteToContext(ctx, io.Discard); err != nil {
		return fmt.Errorf("preflight: %w", err)
	}
	e.after = placementOf(f, e.file)
	return nil
}

// resolve returns the real path of file, refusing a root that left the
// configuration and a symlink that leads out of the root: a rewrite would land
// on the link's target.
func (s *Scanner) resolve(file *store.EditFile) (string, error) {
	if !slices.Contains(s.live().Roots, file.Root) {
		return "", fmt.Errorf("its root is not configured: %w", ErrNotWritable)
	}
	root, err := filepath.EvalSymlinks(file.Root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	path, err := filepath.EvalSymlinks(filepath.Join(file.Root, file.RelPath))
	if err != nil {
		return "", fmt.Errorf("resolve file: %w", err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("it lies outside its root: %w", ErrNotWritable)
	}
	return path, nil
}

// checkWritable refuses a read-only file or folder. A rename-replace needs only
// the folder, so it would rewrite a 0444 file without complaint.
func checkWritable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}
	if info.Mode().Perm()&anyWrite == 0 {
		return fmt.Errorf("it is read-only: %w", ErrNotWritable)
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0) //nolint:gosec // G304: path was resolved under a library root.
	if err != nil {
		return fmt.Errorf("open for writing: %w", err)
	}
	_ = f.Close() // Nothing was written.
	probe, err := os.CreateTemp(filepath.Dir(path), probePattern)
	if err != nil {
		return fmt.Errorf("write to its folder: %w", err)
	}
	_ = probe.Close() // Empty.
	if err = os.Remove(probe.Name()); err != nil {
		return fmt.Errorf("remove the folder probe: %w", err)
	}
	return nil
}
