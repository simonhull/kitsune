package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// NewPlay is the input to [Store.RecordPlay].
type NewPlay struct {
	UserID   int64
	TrackID  string // The track's ULID.
	PlayedAt time.Time
	Source   string
	ClientID string // Unique among the user's plays: a second report of it changes nothing.
}

// Play is a play as [Store.RecentPlays] returns it.
type Play struct {
	TrackID  string
	PlayedAt time.Time
	Source   string
}

// RecordPlay stores a play, once however often it is reported. A track missing
// from disk still takes plays: its file may be back. It returns an error
// wrapping [ErrNotFound] for a track that isn't in the library.
func (s *Store) RecordPlay(ctx context.Context, p *NewPlay) error {
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		trackID, err := q.TrackRowID(ctx, p.TrackID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err //nolint:wrapcheck // Wrapped once, below.
		}
		return q.InsertPlay(ctx, sqlcdb.InsertPlayParams{
			UserID:   p.UserID,
			TrackID:  trackID,
			PlayedAt: ms(p.PlayedAt),
			Source:   p.Source,
			ClientID: p.ClientID,
		})
	})
	if err != nil {
		return fmt.Errorf("record play: %w", err)
	}
	return nil
}

// RecentPlays returns up to limit of a user's plays, newest first.
func (s *Store) RecentPlays(ctx context.Context, userID, limit int64) ([]Play, error) {
	rows, err := s.rq.RecentPlays(ctx, sqlcdb.RecentPlaysParams{UserID: userID, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("recent plays: %w", err)
	}
	plays := make([]Play, len(rows))
	for i, row := range rows {
		plays[i] = Play{TrackID: row.Ulid, PlayedAt: time.UnixMilli(row.PlayedAt).UTC(), Source: row.Source}
	}
	return plays, nil
}
