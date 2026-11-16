package library

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/simonhull/kitsune/internal/organize"
)

// The organizer's moves on disk. They go through the same [openRoots] as the
// Trash's: every step under [os.Root], a rename on the root's own filesystem,
// never a copy, and never over anything ([openRoots.moveBack]).

// errOtherFilesystem means a move would cross to another filesystem, which a
// rename can't do; a copy is never made instead.
var errOtherFilesystem = errors.New("the target is on another filesystem")

// errSourceGone means a song's file is not where the store says it is.
var errSourceGone = errors.New("the file is gone")

// errFileChanged means a song's file is not what the store says it is: it
// changed since the last scan, and its tags may say something else now.
var errFileChanged = errors.New("the file changed since the last scan")

// unchanged returns [errFileChanged] when it is a song whose file's size or
// mtime isn't what the store says, and [errSourceGone] when it has none.
func (o openRoots) unchanged(it *organizeItem) error {
	if it.track == 0 {
		return nil
	}
	r, err := o.root(it.root)
	if err != nil {
		return err
	}
	info, err := r.Lstat(it.from)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return errSourceGone
	case err != nil:
		return fmt.Errorf("look at the file: %w", err)
	case info.Size() != it.size || info.ModTime().UnixMilli() != it.mtime:
		return errFileChanged
	}
	return nil
}

// errCaseFolder means a folder a song would go to exists in another case on a
// filesystem that ignores case, so the song would land in that folder instead.
var errCaseFolder = errors.New("the folder is there in another case")

// organizeTemp is the hidden name a file takes, in its own folder, on the way
// to a name that differs only in case. Every walker skips it, and no scan
// sweeps it: only recovery puts it right.
func organizeTemp(rel string) string {
	return filepath.Join(filepath.Dir(rel), "."+filepath.Base(rel)+".kitsune-organize")
}

// readDir lists the folder rel under root.
func (o openRoots) readDir(root, rel string) ([]fs.DirEntry, error) {
	r, err := o.root(root)
	if err != nil {
		return nil, err
	}
	f, err := r.Open(rel)
	if err != nil {
		return nil, fmt.Errorf("open folder: %w", err)
	}
	defer f.Close() //nolint:errcheck // Read-only.
	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, fmt.Errorf("read folder: %w", err)
	}
	return entries, nil
}

// organizeMove moves the file at from to to, under root, as a run does: the
// folders first (refusing one there only in another case), then
// [openRoots.moveBack], which never replaces anything. A change of case alone
// goes through [organizeTemp]. It returns the folders it made, so a failed
// move can take them away again.
func (o openRoots) organizeMove(root, from, to string, caseOnly bool) ([]string, error) {
	r, err := o.root(root)
	if err != nil {
		return nil, err
	}
	if _, err = r.Lstat(from); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, errSourceGone
		}
		return nil, fmt.Errorf("look at the file: %w", err)
	}
	made, err := o.makeFolders(r, filepath.Dir(to))
	if err != nil {
		return made, err
	}
	if caseOnly {
		return made, o.caseMove(root, from, to)
	}
	err = crossed(o.moveBack(root, from, to))
	if err != nil && !errors.Is(err, errPathTaken) && !errors.Is(err, errOtherFilesystem) {
		o.unlink(r, from, to) // A link made and its source not removed: take the link away again.
	}
	return made, err
}

// unlink removes to when it is the same file as from: what is left of a move
// that linked and then failed to remove its source.
func (openRoots) unlink(r *os.Root, from, to string) {
	a, errA := r.Lstat(from)
	b, errB := r.Lstat(to)
	if errA == nil && errB == nil && os.SameFile(a, b) {
		_ = r.Remove(to) // Best effort: recovery and the next scan see what is left.
	}
}

// caseMove renames from to to, which differ only in case, in two steps through
// a hidden name: on a filesystem that ignores case, to is from itself.
func (o openRoots) caseMove(root, from, to string) error {
	tmp := organizeTemp(from)
	if err := crossed(o.moveBack(root, from, tmp)); err != nil {
		return err
	}
	if err := crossed(o.moveBack(root, tmp, to)); err != nil {
		if back := o.moveBack(root, tmp, from); back != nil {
			return errors.Join(err, fmt.Errorf("put back: %w", back))
		}
		return err
	}
	return nil
}

// crossed returns [errOtherFilesystem] for an error that says a rename or a
// link would cross filesystems, else err.
func crossed(err error) error {
	if errors.Is(err, syscall.EXDEV) {
		return errOtherFilesystem
	}
	return err
}

// makeFolders makes dir and the folders above it that aren't there, under r,
// and returns those it made, deepest last. A folder that is there in another
// case only, on a filesystem that ignores case, is [errCaseFolder].
func (o openRoots) makeFolders(r *os.Root, dir string) ([]string, error) {
	var missing []string
	for d := dir; d != "." && d != string(filepath.Separator); d = filepath.Dir(d) {
		info, err := r.Lstat(d)
		if err == nil {
			if !info.IsDir() {
				return nil, fmt.Errorf("make folder: %s: %w", filepath.Base(d), fs.ErrExist)
			}
			if !exactName(r, d) {
				return nil, errCaseFolder
			}
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("look at a folder: %w", err)
		}
		missing = append(missing, d)
	}
	slices.Reverse(missing)
	for i, d := range missing {
		if err := r.Mkdir(d, trashDirMode); err != nil && !errors.Is(err, fs.ErrExist) {
			return missing[:i], fmt.Errorf("make folder: %w", err)
		}
	}
	return missing, nil
}

// exactName reports whether the folder at rel under r is named exactly so in
// its parent, and not only reached by that name, as a filesystem that ignores
// case allows.
func exactName(r *os.Root, rel string) bool {
	parent, err := r.Open(filepath.Dir(rel))
	if err != nil {
		return true // Can't tell: the move itself will say.
	}
	defer parent.Close() //nolint:errcheck // Read-only.
	names, err := parent.Readdirnames(-1)
	if err != nil {
		return true
	}
	return slices.Contains(names, filepath.Base(rel))
}

// removeEmpty removes the folder dir under root if it is empty, then each
// folder above it that is left empty, up to the root, and returns those it
// removed. A folder with anything in it, desktop.ini or @eaDir included,
// stays, and so does every folder above it.
func (o openRoots) removeEmpty(root, dir string) []string {
	r, err := o.root(root)
	if err != nil {
		return nil
	}
	var removed []string
	for d := dir; d != "." && d != string(filepath.Separator) && d != ""; d = filepath.Dir(d) {
		if r.Remove(d) != nil { // Remove refuses a folder that isn't empty.
			break
		}
		removed = append(removed, d)
	}
	return removed
}

// reasonOf says why a move failed, as a run reports it.
func reasonOf(err error) organize.Reason {
	switch {
	case errors.Is(err, errSourceGone):
		return organize.Missing
	case errors.Is(err, errFileChanged):
		return organize.Changed
	case errors.Is(err, errOtherFilesystem):
		return organize.OtherFilesystem
	case errors.Is(err, errPathTaken):
		return organize.UnknownFile
	}
	return organize.Failed
}

// slashPath returns the store's rel_path for a slash-separated path.
func slashPath(rel string) string { return filepath.FromSlash(rel) }
