package library

import (
	"context"
	"fmt"
	"slices"

	"github.com/simonhull/kitsune/internal/store"
)

// StaleAlbumError means the ETag an edit sent for an album is not the album's.
// It wraps [ErrStale].
type StaleAlbumError struct {
	Album string // The album's ULID.
}

// Error implements error.
func (e *StaleAlbumError) Error() string {
	return fmt.Sprintf("album %s: %v", e.Album, ErrStale)
}

// Unwrap returns [ErrStale].
func (*StaleAlbumError) Unwrap() error { return ErrStale }

// AlbumETag is the ETag an edit read an album at.
type AlbumETag struct {
	ID, ETag string // The album's ULID, and its ETag as the edit read it.
}

// TracksEdit is a change to tracks that may lie on several albums. All goes to
// every track Tracks names, by ULID, with each one's own edit on top: what both
// name takes the track's value. Albums holds every album those tracks are on.
type TracksEdit struct {
	Albums []AlbumETag
	All    Edit
	Tracks map[string]Edit
}

// EditTracks saves e to the files of its tracks, and to the store, as
// [Scanner.EditAlbum] does for one album: every ETag is compared, and every file
// checked, before any is written, album by album in the order e names them and
// each album's in track order.
//
// It returns [ErrTagWritesOff] while tag writes are off, [ErrBusy] while a scan
// or another edit holds the library, an error
// wrapping [store.ErrNotFound] for an album that is unknown or has no live track,
// a [*StaleAlbumError] for the first album whose ETag is not current, [ErrStale]
// when a file drifted from the store, and an [*EditError] when the edit was
// refused or stopped. A track that is not on one of the albums is refused.
func (s *Scanner) EditTracks(ctx context.Context, e *TracksEdit) error {
	return s.exclusive(ctx, func() error {
		if !s.live().TagWrites {
			return ErrTagWritesOff
		}
		var files []store.EditFile
		live := make(map[int64]int, len(e.Albums))
		for _, a := range e.Albums {
			albumFiles, etag, err := s.st.AlbumEditFiles(ctx, a.ID)
			if err != nil {
				return err //nolint:wrapcheck // The store's error already names the album.
			}
			if a.ETag != etag {
				return &StaleAlbumError{Album: a.ID}
			}
			if _, twice := live[albumFiles[0].Album]; twice {
				continue
			}
			live[albumFiles[0].Album] = len(albumFiles)
			files = append(files, albumFiles...)
		}
		edits, err := e.forFiles(files)
		if err != nil {
			return &EditError{Remaining: e.trackIDs(), Err: err}
		}
		return s.edit(ctx, edits, live)
	})
}

// forFiles returns the edit of each track e names, in the order of files.
func (e *TracksEdit) forFiles(files []store.EditFile) ([]fileEdit, error) {
	for id := range e.Tracks {
		if !slices.ContainsFunc(files, func(f store.EditFile) bool { return f.ULID == id }) {
			return nil, fmt.Errorf("track %s is not on the albums named: %w", id, ErrUnsupportedEdit)
		}
	}
	out := make([]fileEdit, 0, len(e.Tracks))
	for i := range files {
		if own, named := e.Tracks[files[i].ULID]; named {
			out = append(out, fileEdit{file: &files[i], edit: e.All.over(&own)})
		}
	}
	return out, nil
}

// trackIDs returns the tracks e names, sorted.
func (e *TracksEdit) trackIDs() []string {
	out := make([]string, 0, len(e.Tracks))
	for id := range e.Tracks {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}
