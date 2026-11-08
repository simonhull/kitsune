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
