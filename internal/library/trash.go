package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

// TrashRetention is how long a song waits in the Trash before it is deleted
// for good: [Scanner.PurgeTrash] deletes it this long after it was trashed.
const TrashRetention = 30 * 24 * time.Hour

// trashDirMode is the mode of the folders a trash or a restore makes. The
// umask has the last word.
const trashDirMode = 0o755

// errPathTaken means a restore found something where its file goes back to.
var errPathTaken = errors.New("a file is at its old path")

// PathTakenError means a restore was refused, and restored nothing: a file, or
// a song, is now where these songs were.
type PathTakenError struct {
	Titles []string // The songs that can't go back, in the order asked.
}

func (e *PathTakenError) Error() string {
	return fmt.Sprintf("%d songs can't go back: %v", len(e.Titles), errPathTaken)
}

func (*PathTakenError) Unwrap() error { return errPathTaken }

// MoveError means a song's file could not move into or out of the Trash. Its
// move and every one before it were undone, so nothing moved.
type MoveError struct {
	Title string // The song whose file would not move.
	Err   error
}

func (e *MoveError) Error() string { return fmt.Sprintf("move %q: %v", e.Title, e.Err) }

func (e *MoveError) Unwrap() error { return e.Err }

// TrashChange is what a trash or a restore moved: the songs, by public ID, and
// the albums they are on, each once.
type TrashChange struct {
	Tracks, Albums []string
}

// trashPath is where the file at rel goes in its root's Trash: under a folder
// named for the song's own ULID, so no two songs can ever want the same path
// there, and the song's own path below it, so the folder says where it was.
func trashPath(ulid, rel string) string { return filepath.Join(TrashDir, ulid, rel) }

// TrashTracks moves the songs with the public IDs ids to the Trash, for the
// user by: each file to its root's [TrashDir], and each track out of every
// read of the library, keeping its row, its ID and everything people did with
// it. It is all or nothing.
//
// It returns [ErrTagWritesOff] while tag writes are off, since then nothing
// under a root may change; [ErrBusy] while a scan or an edit holds the
// library; an error wrapping [store.ErrNotFound] when an ID names no song in
// the library; and a [*MoveError] when a file would not move.
func (s *Scanner) TrashTracks(ctx context.Context, by int64, ids []string) (TrashChange, error) {
	return s.trash(ctx, by, func(ctx context.Context) ([]store.TrashTrack, error) {
		return s.st.TrashableTracks(ctx, ids)
	})
}

// TrashAlbums is [Scanner.TrashTracks] for every live song of the albums with
// the public IDs ids.
func (s *Scanner) TrashAlbums(ctx context.Context, by int64, ids []string) (TrashChange, error) {
	return s.trash(ctx, by, func(ctx context.Context) ([]store.TrashTrack, error) {
		return s.st.TrashableAlbums(ctx, ids)
	})
}

// trash moves the tracks find returns to the Trash, holding the library lock.
func (s *Scanner) trash(ctx context.Context, by int64, find func(context.Context) ([]store.TrashTrack, error)) (TrashChange, error) {
	var change TrashChange
	err := s.exclusive(ctx, func() error {
		if !s.live().TagWrites {
			return ErrTagWritesOff
		}
		tracks, findErr := find(ctx)
		if findErr != nil {
			return findErr
		}
		// Once a file starts moving, the trash finishes or is undone, whatever ctx does.
		bg := context.WithoutCancel(ctx)
		marks := make([]store.Trashing, len(tracks))
		for i := range tracks {
			marks[i] = store.Trashing{ID: tracks[i].ID, TrashPath: trashPath(tracks[i].ULID, tracks[i].RelPath)}
		}
		// The rows go first: a server stopped between the two leaves a file where it
		// was, which the next scan finds again, at worst as a new song. The other
		// order would hide a file in the Trash behind a track about to go missing.
		if err := s.st.MarkTrashed(bg, marks, by, s.now()); err != nil {
			return err //nolint:wrapcheck // The store's error already says what failed.
		}
		roots := openRoots{}
		defer roots.close()
		for i := range tracks {
			t := &tracks[i]
			if err := roots.move(t.Root, t.RelPath, marks[i].TrashPath); err != nil {
				s.undoTrash(bg, &roots, tracks[:i], marks)
				return &MoveError{Title: t.Title, Err: err}
			}
		}
		s.settleTrash(bg, tracks)
		change = changeOf(tracks)
		return nil
	})
	return change, err
}

// undoTrash moves back the files of moved and puts every track of marks back
// in the library. What can't be undone is logged: a file left in the Trash
// stays there, under a track the next scan will find missing.
func (s *Scanner) undoTrash(ctx context.Context, roots *openRoots, moved []store.TrashTrack, marks []store.Trashing) {
	for i := range moved {
		if err := roots.move(moved[i].Root, marks[i].TrashPath, moved[i].RelPath); err != nil {
			s.log.ErrorContext(ctx, "moving a file back out of the Trash failed", "track", moved[i].ULID, "err", err)
		}
	}
	ids := make([]int64, len(marks))
	for i := range marks {
		ids[i] = marks[i].ID
	}
	if err := s.st.UnmarkTrashed(ctx, ids); err != nil {
		s.log.ErrorContext(ctx, "undoing a trash in the store failed", "err", err)
	}
}

// RestoreTracks moves the trashed songs with the public IDs ids back where
// they were, with everything they had. It is all or nothing, and never
// overwrites: it refuses with a [*PathTakenError] when a file or a song is now
// at a song's old path. Its other errors are those of [Scanner.TrashTracks],
// with [store.ErrNotFound] for an ID that names no song in the Trash.
func (s *Scanner) RestoreTracks(ctx context.Context, ids []string) (TrashChange, error) {
	var change TrashChange
	err := s.exclusive(ctx, func() error {
		if !s.live().TagWrites {
			return ErrTagWritesOff
		}
		tracks, findErr := s.st.TrashedTracks(ctx, ids)
		if findErr != nil {
			return findErr //nolint:wrapcheck // The store's error already says what failed.
		}
		roots := openRoots{}
		defer roots.close()
		if err := s.checkFree(ctx, &roots, tracks); err != nil {
			return err
		}
		bg := context.WithoutCancel(ctx)
		// The files go first, the opposite of a trash, for the same reason: a server
		// stopped between the two leaves the file in the library, at its old path.
		for i := range tracks {
			t := &tracks[i]
			if err := roots.moveBack(t.Root, t.RelPath, t.RestorePath); err != nil {
				s.undoRestore(bg, &roots, tracks[:i])
				if errors.Is(err, errPathTaken) {
					return &PathTakenError{Titles: []string{t.Title}}
				}
				return &MoveError{Title: t.Title, Err: err}
			}
		}
		rows := make([]int64, len(tracks))
		for i := range tracks {
			rows[i] = tracks[i].ID
		}
		if err := s.st.UnmarkTrashed(bg, rows); err != nil {
			s.undoRestore(bg, &roots, tracks)
			return err //nolint:wrapcheck // As above.
		}
		for i := range tracks {
			roots.tidy(tracks[i].Root, tracks[i].RelPath)
		}
		s.settleTrash(bg, tracks)
		change = changeOf(tracks)
		return nil
	})
	return change, err
}

// checkFree refuses a restore, with a [*PathTakenError], when a song or a file
// is now at the old path of any of tracks.
func (s *Scanner) checkFree(ctx context.Context, roots *openRoots, tracks []store.TrashTrack) error {
	var taken []string
	for i := range tracks {
		t := &tracks[i]
		row, err := s.st.PathTaken(ctx, t.Root, t.RestorePath)
		if err != nil {
			return err //nolint:wrapcheck // The store's error already says what failed.
		}
		file, err := roots.exists(t.Root, t.RestorePath)
		if err != nil {
			return &MoveError{Title: t.Title, Err: err}
		}
		if row || file {
			taken = append(taken, t.Title)
		}
	}
	if len(taken) > 0 {
		return &PathTakenError{Titles: taken}
	}
	return nil
}

// undoRestore moves the files of moved back into the Trash. What can't be
// undone is logged: a file left at its old path is found by the next scan.
func (s *Scanner) undoRestore(ctx context.Context, roots *openRoots, moved []store.TrashTrack) {
	for i := range moved {
		if err := roots.move(moved[i].Root, moved[i].RestorePath, moved[i].RelPath); err != nil {
			s.log.ErrorContext(ctx, "moving a file back into the Trash failed", "track", moved[i].ULID, "err", err)
		}
	}
}

// settleTrash brings the albums tracks were on up to date with their songs
// gone or back: each one's year, compilation flag and cover come from its
// first live track, and search, genres and loudness follow.
func (s *Scanner) settleTrash(ctx context.Context, tracks []store.TrashTrack) {
	var albums []int64
	for i := range tracks {
		if !slices.Contains(albums, tracks[i].AlbumID) {
			albums = append(albums, tracks[i].AlbumID)
		}
	}
	if err := s.st.SettleAlbums(ctx, albums); err != nil {
		s.log.WarnContext(ctx, "settling albums after a trash or a restore failed", "err", err)
	}
	for _, album := range albums {
		s.settleCover(ctx, album)
	}
	s.refreshNames(ctx)
	s.refreshGenres(ctx)
	s.askForPass() // An album's loudness pools its live songs.
}

// changeOf returns what moving tracks changed.
func changeOf(tracks []store.TrashTrack) TrashChange {
	c := TrashChange{Tracks: make([]string, len(tracks))}
	for i := range tracks {
		c.Tracks[i] = tracks[i].ULID
		if !slices.Contains(c.Albums, tracks[i].Album) {
			c.Albums = append(c.Albums, tracks[i].Album)
		}
	}
	return c
}

// EmptyTrash deletes the trashed songs with the public IDs ids for good, or
// every one when ids is nil: their files, then their rows with everything
// people did with them, as a missing track past its grace is deleted. It
// returns how many went. A song whose file can't be reached now stays, for a
// later try. Its errors are those of [Scanner.RestoreTracks], but for the
// paths.
func (s *Scanner) EmptyTrash(ctx context.Context, ids []string) (int, error) {
	var n int
	err := s.exclusive(ctx, func() error {
		if !s.live().TagWrites {
			return ErrTagWritesOff
		}
		bg := context.WithoutCancel(ctx) // Files are going: finish what was asked.
		defer s.pruneAfterDelete(bg)
		if ids != nil {
			tracks, err := s.st.TrashedTracks(ctx, ids)
			if err != nil {
				return err //nolint:wrapcheck // The store's error already says what failed.
			}
			n, err = s.deleteTrashed(bg, tracks)
			return err
		}
		for {
			batch, err := s.st.TrashedBefore(bg, everTrashed(), passBatchFiles)
			if err != nil {
				return err //nolint:wrapcheck // As above.
			}
			deleted, err := s.deleteTrashed(bg, batch)
			n += deleted
			if err != nil || deleted < len(batch) || len(batch) < passBatchFiles {
				return err
			}
		}
	})
	return n, err
}

// everTrashed returns a time after every song was trashed.
func everTrashed() time.Time { return time.UnixMilli(math.MaxInt64) }

// PurgeTrash deletes for good every song trashed more than [TrashRetention]
// ago, as [Scanner.EmptyTrash] does, in batches like the fingerprint pass: it
// holds the library lock for one at a time, lets a waiting scan or edit go
// first, and pauses while a scan is claimed. With tag writes off it deletes
// nothing, since then nothing under a root may change. It returns how many
// went. The daily schedule runs it after each scan.
func (s *Scanner) PurgeTrash(ctx context.Context) (int, error) {
	n := 0
	for {
		b, err := s.purgeBatch(ctx)
		n += b.deleted
		if err != nil {
			if ctx.Err() == nil {
				s.log.ErrorContext(ctx, "purging the Trash failed", "err", err)
			}
			return n, err
		}
		if !b.more {
			break
		}
	}
	if n > 0 {
		s.log.InfoContext(ctx, "purged the Trash", "deleted", n)
	}
	return n, nil
}

// purged is what one batch of [Scanner.PurgeTrash] did.
type purged struct {
	deleted int
	more    bool // The batch was full and went whole: there may be more.
}

// purgeBatch deletes one batch of songs past their time in the Trash, holding
// the library lock.
func (s *Scanner) purgeBatch(ctx context.Context) (purged, error) {
	if err := s.claim.waitFree(ctx); err != nil { // A scan, claimed or running, pauses the purge.
		return purged{}, err
	}
	if err := s.lock.lock(ctx, heldByPass); err != nil {
		return purged{}, err
	}
	defer s.lock.unlock()
	if !s.live().TagWrites {
		return purged{}, nil
	}
	batch, err := s.st.TrashedBefore(ctx, s.now().Add(-TrashRetention), passBatchFiles)
	if err != nil {
		return purged{}, fmt.Errorf("purge: %w", err)
	}
	n, err := s.deleteTrashed(ctx, batch)
	b := purged{deleted: n, more: err == nil && n == len(batch) && len(batch) == passBatchFiles}
	if !b.more {
		s.pruneAfterDelete(ctx)
	}
	return b, err
}

// deleteTrashed deletes the files of tracks, then the rows of those whose
// files went, and returns how many did. A file it can't reach is logged and
// its song stays.
func (s *Scanner) deleteTrashed(ctx context.Context, tracks []store.TrashTrack) (int, error) {
	roots := openRoots{}
	defer roots.close()
	rows := make([]int64, 0, len(tracks))
	for i := range tracks {
		t := &tracks[i]
		if err := roots.removeTrashed(t.Root, t.ULID); err != nil {
			s.log.WarnContext(ctx, "deleting a file in the Trash failed; it stays until the next try",
				"track", t.ULID, "err", StripRoot(err.Error(), t.Root))
			continue
		}
		rows = append(rows, t.ID)
	}
	n, err := s.st.DeleteTrashed(ctx, rows)
	if err != nil {
		return 0, err //nolint:wrapcheck // The store's error already says what failed.
	}
	return int(n), nil
}

// pruneAfterDelete drops what the deleted songs alone used: albums, artists
// and covers.
func (s *Scanner) pruneAfterDelete(ctx context.Context) {
	pruned, err := s.st.PruneOrphans(ctx)
	if err != nil {
		s.log.WarnContext(ctx, "pruning after deleting from the Trash failed", "err", err)
		return
	}
	s.deleteArtwork(ctx, pruned.Artwork)
}

// openRoots opens each library root once, for the moves of one trash or
// restore. Every step goes through [os.Root], so no path can leave its root.
type openRoots map[string]*os.Root

// root returns the open root at path, opening it the first time.
func (o openRoots) root(path string) (*os.Root, error) {
	if r, ok := o[path]; ok {
		return r, nil
	}
	r, err := os.OpenRoot(path)
	if err != nil {
		return nil, fmt.Errorf("open root: %w", err)
	}
	o[path] = r
	return r, nil
}

func (o openRoots) close() {
	for _, r := range o {
		_ = r.Close() // Only folders were opened: nothing to flush.
	}
}

// move renames the file at from, under root, to the path to, making its
// folders. Both lie on the root's filesystem, so it never copies.
func (o openRoots) move(root, from, to string) error {
	r, err := o.root(root)
	if err != nil {
		return err
	}
	if err = r.MkdirAll(filepath.Dir(to), trashDirMode); err != nil {
		return fmt.Errorf("make folder: %w", err)
	}
	if err = r.Rename(from, to); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// moveBack renames the file at from, under root, to the path to, making its
// folders, and never replaces anything at to: it returns [errPathTaken] instead. It links the
// file at to, which fails if anything is there, then removes from. On a
// filesystem without hard links it looks, then renames, holding the library
// lock, so only another program could slip a file in between.
func (o openRoots) moveBack(root, from, to string) error {
	r, err := o.root(root)
	if err != nil {
		return err
	}
	if err = r.MkdirAll(filepath.Dir(to), trashDirMode); err != nil {
		return fmt.Errorf("make folder: %w", err)
	}
	err = r.Link(from, to)
	switch {
	case err == nil:
		if err = r.Remove(from); err != nil {
			return fmt.Errorf("remove the Trash's link: %w", err)
		}
		return nil
	case errors.Is(err, fs.ErrExist):
		return errPathTaken
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("link: %w", err) // The file is gone from the Trash.
	}
	taken, err := o.exists(root, to)
	switch {
	case err != nil:
		return err
	case taken:
		return errPathTaken
	}
	if err = r.Rename(from, to); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// exists reports whether anything, even a dangling link, is at rel under root.
func (o openRoots) exists(root, rel string) (bool, error) {
	r, err := o.root(root)
	if err != nil {
		return false, err
	}
	_, err = r.Lstat(rel)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	}
	return false, fmt.Errorf("look at the old path: %w", err)
}

// tidy removes the folders a restored file left empty in the Trash, the
// song's own and then the Trash itself. One that isn't empty stays.
func (o openRoots) tidy(root, trashed string) {
	r, err := o.root(root)
	if err != nil {
		return
	}
	for dir := filepath.Dir(trashed); strings.HasPrefix(dir, TrashDir); dir = filepath.Dir(dir) {
		if r.Remove(dir) != nil {
			return
		}
	}
}

// removeTrashed deletes the song's folder in its root's Trash, with its file,
// and the Trash itself once it is empty. A folder already gone is no error.
func (o openRoots) removeTrashed(root, ulid string) error {
	r, err := o.root(root)
	if err != nil {
		return err
	}
	if err = r.RemoveAll(filepath.Join(TrashDir, ulid)); err != nil {
		return fmt.Errorf("remove: %w", err)
	}
	_ = r.Remove(TrashDir) // Fails while another song waits there, as it should.
	return nil
}
