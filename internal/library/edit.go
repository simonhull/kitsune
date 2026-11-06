package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
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
// whatever Err says. Err wraps [ErrUnsupportedEdit], [ErrNotWritable] or
// something unexpected. A preflight failure leaves Changed empty.
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
// Tracks, by ULID, to the ones it names, on top of All: a field in both takes the
// track's value.
type AlbumEdit struct {
	All    Edit
	Tracks map[string]Edit
}

// fileEdit is what one file of an album will take.
type fileEdit struct {
	file *store.EditFile
	edit Edit
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
		if !named && len(e.All.Fields) == 0 {
			continue
		}
		fields := maps.Clone(e.All.Fields)
		if fields == nil {
			fields = make(map[Field][]string, len(own.Fields))
		}
		maps.Copy(fields, own.Fields)
		out = append(out, fileEdit{file: &files[i], edit: Edit{Fields: fields}})
	}
	return out, nil
}

// EditAlbum saves e to the files of the album id, and to the store. Each file is
// written and reindexed before the next, so the library follows the files even
// if the edit stops midway. A canceled ctx stops the edit between or inside
// files, never leaving one half written.
//
// It returns [ErrBusy] while a scan or another edit holds the library, an error
// wrapping [store.ErrNotFound] for an unknown ID, [ErrStale] when ifMatch is not
// the current ETag or a file drifted from the store, and an [*EditError] when
// the edit was refused or stopped.
func (s *Scanner) EditAlbum(ctx context.Context, id, ifMatch string, e *AlbumEdit) error {
	return s.exclusive(func() error {
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
		return s.edit(ctx, edits)
	})
}

// fileEdits lists files with no edit, for reporting.
func fileEdits(files []store.EditFile) []fileEdit {
	out := make([]fileEdit, len(files))
	for i := range files {
		out[i].file = &files[i]
	}
	return out
}

// edit checks every file, and only then writes them in order. The caller holds
// the library lock.
func (s *Scanner) edit(ctx context.Context, edits []fileEdit) error {
	for i := range edits {
		err := checkDrift(edits[i].file)
		if errors.Is(err, ErrStale) {
			return ErrStale
		}
		if err == nil {
			err = s.preflight(ctx, edits[i].file, &edits[i].edit)
		}
		if err != nil {
			return stopped(edits, 0, err)
		}
	}
	// Tracks written before a failure hold the edit, and a canceled edit still counts.
	defer s.settle(ctx)
	for i := range edits {
		file := edits[i].file
		path := filepath.Join(file.Root, file.RelPath)
		if err := s.write(ctx, path, edits[i].edit.apply); err != nil {
			return stopped(edits, i, err)
		}
		// The file has changed, so a canceled edit still indexes it.
		if err := s.reindex(context.WithoutCancel(ctx), file.ID, file.Root, file.RelPath); err != nil {
			return stopped(edits, i+1, err)
		}
	}
	return nil
}

// settle brings what the store derives from its tracks up to date, once per edit
// and with the lock held, however the edit ended: some files may have changed.
func (s *Scanner) settle(ctx context.Context) {
	ctx = context.WithoutCancel(ctx)
	s.refreshNames(ctx)
	s.refreshGenres(ctx)
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
	case errors.Is(err, audiometa.ErrUnsupportedWrite), errors.Is(err, audiometa.ErrDRMProtected), errors.Is(err, audiometa.ErrTagTooLarge):
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
// go through a dry write to nowhere.
func (s *Scanner) preflight(ctx context.Context, file *store.EditFile, e *Edit) error {
	path, err := s.resolve(file)
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
	if applyErr := e.apply(f); applyErr != nil {
		return applyErr
	}
	if _, err = f.WriteToContext(ctx, io.Discard); err != nil {
		return fmt.Errorf("preflight: %w", err)
	}
	return nil
}

// resolve returns the real path of file, refusing a root that left the
// configuration and a symlink that leads out of the root: a rewrite would land
// on the link's target.
func (s *Scanner) resolve(file *store.EditFile) (string, error) {
	if !slices.Contains(s.roots, file.Root) {
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
