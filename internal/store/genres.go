package store

import (
	"context"
	"fmt"
)

// AlbumTagCount says how many live tracks of an album hold a raw tag value.
type AlbumTagCount struct {
	AlbumID string // The album's ULID.
	Value   string
	Tracks  int
}

// AlbumTagCounts counts the values of the raw tags keys on every album with a
// live track, in no particular order.
func (s *Store) AlbumTagCounts(ctx context.Context, keys []string) ([]AlbumTagCount, error) {
	rows, err := s.rq.ListAlbumTagCounts(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("album tag counts: %w", err)
	}
	out := make([]AlbumTagCount, len(rows))
	for i := range rows {
		out[i] = AlbumTagCount{AlbumID: rows[i].Ulid, Value: rows[i].Value, Tracks: int(rows[i].Tracks)}
	}
	return out, nil
}
