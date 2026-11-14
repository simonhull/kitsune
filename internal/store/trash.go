package store

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// A trashed track is a song an administrator deleted: its row stays, with its
// ULID and everything that hangs off it, while its file waits in the Trash.
// Its missing_since is its trashed_at, so no live read lists it, streams it,
// shuffles it or counts it. The scanner never sees it: [Store.FileStates],
// [Store.MissingByDigest] and [Store.KnownRoots] leave it out, and
// [Store.DeleteMissingBefore] never deletes it. Its rel_path names where the
// file is now, in the Trash, so a new file at its old path is a new track.

// TrashTrack is a track on its way into or out of the Trash.
type TrashTrack struct {
	ID      int64
	ULID    string
	Root    string
	RelPath string // Where the file is: in the library, or in the Trash.
	// RestorePath is where a trashed track's file goes back to, relative to
	// Root. Empty for a live track.
	RestorePath string
	Title       string
	AlbumID     int64  // The album's row.
	Album       string // The album's ULID.
}

// Trashing is one track to mark trashed, with where its file now waits.
type Trashing struct {
	ID        int64
	TrashPath string // Relative to the track's root.
}

// TrashableTracks returns the live tracks with the public IDs ids, in row
// order. It returns an error wrapping [ErrNotFound] when an ID names no live
// track: an unknown one, a missing one or a trashed one.
func (s *Store) TrashableTracks(ctx context.Context, ids []string) ([]TrashTrack, error) {
	rows, err := s.rq.TrashableTracks(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("trashable tracks: %w", err)
	}
	if len(rows) != len(unique(ids)) {
		return nil, fmt.Errorf("trashable tracks: %w", ErrNotFound)
	}
	out := make([]TrashTrack, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = TrashTrack{
			ID: r.ID, ULID: r.Ulid, Root: r.Root, RelPath: r.RelPath, Title: r.Title, AlbumID: r.AlbumID, Album: r.Album,
		}
	}
	return out, nil
}

// TrashableAlbums returns the live tracks of the albums with the public IDs
// ids, in row order. It returns an error wrapping [ErrNotFound] when an ID
// names no album with a live track.
func (s *Store) TrashableAlbums(ctx context.Context, ids []string) ([]TrashTrack, error) {
	rows, err := s.rq.TrashableAlbumTracks(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("trashable albums: %w", err)
	}
	albums := map[string]struct{}{}
	out := make([]TrashTrack, len(rows))
	for i := range rows {
		r := &rows[i]
		albums[r.Album] = struct{}{}
		out[i] = TrashTrack{
			ID: r.ID, ULID: r.Ulid, Root: r.Root, RelPath: r.RelPath, Title: r.Title, AlbumID: r.AlbumID, Album: r.Album,
		}
	}
	if len(albums) != len(unique(ids)) {
		return nil, fmt.Errorf("trashable albums: %w", ErrNotFound)
	}
	return out, nil
}

// TrashedTracks returns the trashed tracks with the public IDs ids, in row
// order. It returns an error wrapping [ErrNotFound] when an ID names no
// trashed track.
func (s *Store) TrashedTracks(ctx context.Context, ids []string) ([]TrashTrack, error) {
	rows, err := s.rq.TrashedTracks(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("trashed tracks: %w", err)
	}
	if len(rows) != len(unique(ids)) {
		return nil, fmt.Errorf("trashed tracks: %w", ErrNotFound)
	}
	out := make([]TrashTrack, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = TrashTrack{
			ID: r.ID, ULID: r.Ulid, Root: r.Root, RelPath: r.RelPath, RestorePath: r.RestorePath,
			Title: r.Title, AlbumID: r.AlbumID, Album: r.Album,
		}
	}
	return out, nil
}

// TrashedBefore returns up to limit tracks trashed before cutoff, oldest first.
func (s *Store) TrashedBefore(ctx context.Context, cutoff time.Time, limit int) ([]TrashTrack, error) {
	rows, err := s.rq.TrashedBefore(ctx, sqlcdb.TrashedBeforeParams{Cutoff: new(ms(cutoff)), N: int64(limit)})
	if err != nil {
		return nil, fmt.Errorf("trashed before %s: %w", cutoff.Format(time.RFC3339), err)
	}
	out := make([]TrashTrack, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = TrashTrack{
			ID: r.ID, ULID: r.Ulid, Root: r.Root, RelPath: r.RelPath, RestorePath: r.RestorePath,
			Title: r.Title, AlbumID: r.AlbumID, Album: r.Album,
		}
	}
	return out, nil
}

// PathTaken reports whether a track, of any state, has the path relPath under
// root, so that a restore there would collide with it.
func (s *Store) PathTaken(ctx context.Context, root, relPath string) (bool, error) {
	taken, err := s.rq.TrackAtPath(ctx, sqlcdb.TrackAtPathParams{Root: root, RelPath: relPath})
	if err != nil {
		return false, fmt.Errorf("track at %s: %w", relPath, err)
	}
	return taken, nil
}

// MarkTrashed marks the tracks trashed by the user by (0 for none) as of the
// time given, each with the path its file now has in the Trash, in one transaction, and notes
// their albums changed. It returns an error wrapping [ErrNotFound], and marks
// none, when a track is not live.
func (s *Store) MarkTrashed(ctx context.Context, tracks []Trashing, by int64, at time.Time) error {
	var who *int64
	if by != 0 {
		who = &by
	}
	var touched []string
	err := s.inLibraryTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		touched = touched[:0] // The writer may retry a transaction.
		for _, t := range tracks {
			album, err := q.TrackAlbumULID(ctx, t.ID)
			if err != nil {
				return fmt.Errorf("track %d: %w", t.ID, notFound(err))
			}
			n, err := q.MarkTrashed(ctx, sqlcdb.MarkTrashedParams{At: new(ms(at)), By: who, TrashPath: t.TrashPath, ID: t.ID})
			if err != nil {
				return fmt.Errorf("track %d: %w", t.ID, err)
			}
			if n == 0 {
				return fmt.Errorf("track %d: %w", t.ID, ErrNotFound)
			}
			touched = append(touched, album)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("mark trashed: %w", err)
	}
	s.changed.add(touched)
	return nil
}

// UnmarkTrashed puts the trashed tracks ids back in the library at their
// restore paths, in one transaction, and notes their albums changed. It
// returns an error wrapping [ErrNotFound], and puts none back, when a track is
// not trashed.
func (s *Store) UnmarkTrashed(ctx context.Context, ids []int64) error {
	var touched []string
	err := s.inLibraryTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		touched = touched[:0]
		for _, id := range ids {
			album, err := q.TrackAlbumULID(ctx, id)
			if err != nil {
				return fmt.Errorf("track %d: %w", id, notFound(err))
			}
			n, err := q.UnmarkTrashed(ctx, id)
			if err != nil {
				return fmt.Errorf("track %d: %w", id, err)
			}
			if n == 0 {
				return fmt.Errorf("track %d: %w", id, ErrNotFound)
			}
			touched = append(touched, album)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("unmark trashed: %w", err)
	}
	s.changed.add(touched)
	return nil
}

// DeleteTrashed deletes the trashed tracks ids for good, as
// [Store.DeleteMissingBefore] deletes a missing track past its grace, and
// returns how many went. An ID that is not a trashed track is skipped.
func (s *Store) DeleteTrashed(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	arg, err := json.Marshal(ids)
	if err != nil {
		return 0, fmt.Errorf("delete trashed: %w", err)
	}
	var n int64
	err = s.inLibraryTx(ctx, func(_ *sqlcdb.Queries, tx *sql.Tx) error {
		deleted, delErr := deleteTracks(ctx, tx, trashedIn, string(arg))
		n = deleted
		return delErr
	})
	if err != nil {
		return 0, fmt.Errorf("delete trashed: %w", err)
	}
	return n, nil
}

// Trash is everything in the Trash, as its page shows it.
type Trash struct {
	Albums []TrashAlbum // Newest first: by the latest song each had trashed.
	Count  int
	Bytes  int64
}

// TrashAlbum is one album's songs in the Trash.
type TrashAlbum struct {
	ID     string // The album's ULID.
	Title  string
	Artist string
	Cover  *Cover
	Tracks []TrashedTrack // In album order.
	newest time.Time
}

// TrashedTrack is one song in the Trash.
type TrashedTrack struct {
	ID        string // The track's ULID.
	Title     string
	Size      int64
	TrashedAt time.Time
	TrashedBy string // The administrator's name; empty once their account is gone.
}

// Trash lists the Trash.
func (s *Store) Trash(ctx context.Context) (Trash, error) {
	rows, err := s.rq.ListTrash(ctx)
	if err != nil {
		return Trash{}, fmt.Errorf("trash: %w", err)
	}
	var t Trash
	at := map[string]int{} // Album ULID → index in t.Albums.
	for i := range rows {
		r := &rows[i]
		k, ok := at[r.Album]
		if !ok {
			k = len(t.Albums)
			at[r.Album] = k
			t.Albums = append(t.Albums, TrashAlbum{
				ID: r.Album, Title: r.AlbumTitle, Artist: r.Artist,
				Cover: cover(r.ArtworkHash, r.ArtworkWidth, r.ArtworkHeight, r.ArtworkAccent, r.ArtworkGround),
			})
		}
		when := fromMS(*r.TrashedAt) // Never NULL: the query asks for trashed tracks.
		a := &t.Albums[k]
		a.Tracks = append(a.Tracks, TrashedTrack{
			ID: r.Ulid, Title: r.Title, Size: r.Size, TrashedAt: when, TrashedBy: strOrEmpty(r.TrashedBy),
		})
		if when.After(a.newest) {
			a.newest = when
		}
		t.Count++
		t.Bytes += r.Size
	}
	slices.SortStableFunc(t.Albums, func(a, b TrashAlbum) int { return b.newest.Compare(a.newest) })
	return t, nil
}

// unique returns ids without repeats.
func unique(ids []string) []string {
	return slices.Compact(slices.Sorted(slices.Values(ids)))
}

// notFound turns a row that isn't there into [ErrNotFound].
func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
