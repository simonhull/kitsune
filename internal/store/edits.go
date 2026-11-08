package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// AlbumRekey gives an album row a new group key, so that the tracks an edit
// renames land back on the row they left, keeping its ULID, first-seen time and
// cover.
type AlbumRekey struct {
	Album int64  // The album's row.
	To    string // Its new group key.
}

// RekeyAlbums gives each album its new group key, in order and in one
// transaction. A key another album holds, or one an earlier rekey took, is left
// alone: tracks written there join that album. It returns the rekeys that undo
// what it did, newest first.
func (s *Store) RekeyAlbums(ctx context.Context, rekeys []AlbumRekey) ([]AlbumRekey, error) {
	if len(rekeys) == 0 {
		return nil, nil
	}
	var undo []AlbumRekey
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		undo = undo[:0]
		for _, r := range rekeys {
			taken, err := q.GroupKeyTaken(ctx, r.To)
			if err != nil {
				return fmt.Errorf("album %d: %w", r.Album, err)
			}
			if taken {
				continue
			}
			from, err := q.AlbumGroupKey(ctx, r.Album)
			if err != nil {
				return fmt.Errorf("album %d: %w", r.Album, err)
			}
			if err = q.SetAlbumGroupKey(ctx, sqlcdb.SetAlbumGroupKeyParams{GroupKey: r.To, ID: r.Album}); err != nil {
				return fmt.Errorf("album %d: %w", r.Album, err)
			}
			undo = append([]AlbumRekey{{Album: r.Album, To: from}}, undo...)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("rekey albums: %w", err)
	}
	return undo, nil
}

// AlbumMove says which tracks an edit took off an album.
type AlbumMove struct {
	From   int64   // The album's row.
	Tracks []int64 // The rows of the tracks the edit wrote.
}

// MergeEmptied merges each album an edit left with no tracks into the album its
// tracks went to, when they all went to one: that album keeps its own cover, or
// takes the other's, its first-seen time becomes the older of the two, and the
// other's hearts move to it. The emptied album is left for [Store.PruneOrphans].
// It returns how many albums it merged.
func (s *Store) MergeEmptied(ctx context.Context, moves []AlbumMove) (int, error) {
	merged := 0
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		merged = 0
		for i := range moves {
			into, err := mergeTarget(ctx, q, &moves[i])
			if err != nil {
				return fmt.Errorf("album %d: %w", moves[i].From, err)
			}
			if into == 0 {
				continue
			}
			arg := sqlcdb.MergeAlbumIntoParams{FromID: moves[i].From, IntoID: into}
			if err = q.MergeAlbumInto(ctx, arg); err != nil {
				return fmt.Errorf("album %d: %w", arg.FromID, err)
			}
			if err = q.MoveAlbumHearts(ctx, sqlcdb.MoveAlbumHeartsParams{IntoID: into, FromID: arg.FromID}); err != nil {
				return fmt.Errorf("album %d: hearts: %w", arg.FromID, err)
			}
			merged++
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("merge albums: %w", err)
	}
	return merged, nil
}

// mergeTarget returns the row of the one album m's tracks went to, or 0 when m's
// album still has tracks or they went to several.
func mergeTarget(ctx context.Context, q *sqlcdb.Queries, m *AlbumMove) (int64, error) {
	left, err := q.CountAlbumTracks(ctx, m.From)
	if err != nil || left != 0 {
		return 0, err //nolint:wrapcheck // The caller names the album.
	}
	var into int64
	for _, track := range m.Tracks {
		album, rowErr := q.TrackAlbumRow(ctx, track)
		if rowErr != nil {
			return 0, rowErr //nolint:wrapcheck // The caller names the album.
		}
		if into != 0 && album != into {
			return 0, nil
		}
		into = album
	}
	if into == m.From {
		return 0, nil
	}
	return into, nil
}
