package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// Ending is how a listen ended.
type Ending string

// The ways a listen ends. Only [Skipped] says anything against a song.
const (
	// UnderWay is a listen whose end hasn't been reported.
	UnderWay Ending = ""
	// Completed is a song that played to its end.
	Completed Ending = "completed"
	// Skipped is a song the person moved on from before its end.
	Skipped Ending = "skipped"
	// Stopped is playback the person stopped, or that ran out.
	Stopped Ending = "stopped"
	// Interrupted is playback that an error, or the app closing, cut off.
	Interrupted Ending = "interrupted"
)

// Valid reports whether e is one of the endings, or [UnderWay].
func (e Ending) Valid() bool {
	switch e {
	case UnderWay, Completed, Skipped, Stopped, Interrupted:
		return true
	}
	return false
}

// NewListen is the input to [Store.RecordListen]: a listen's start, or its end.
type NewListen struct {
	UserID    int64
	TrackID   string // The track's ULID.
	ClientID  string // The listen's own ID, chosen by the client: its start and its end share it.
	Shuffle   string // The shuffle session it played in, or "".
	StartedAt time.Time
	Listened  time.Duration // How much of the song played.
	Ended     Ending
}

// RecordListen stores a listen's start or its end, which are one row however
// often either is reported. An end fills in how much played and how it ended,
// and once it has ended a listen never changes. The track's length is taken
// from the library. It returns an error wrapping [ErrNotFound] for a track that
// isn't in the library, and [ErrInvalidWrite] for an ending that isn't one.
func (s *Store) RecordListen(ctx context.Context, l *NewListen) error {
	if !l.Ended.Valid() || l.Listened < 0 {
		return fmt.Errorf("record listen: %w", ErrInvalidWrite)
	}
	var ended *string
	if l.Ended != UnderWay {
		ended = new(string(l.Ended))
	}
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		track, err := q.TrackForListen(ctx, l.TrackID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err //nolint:wrapcheck // Wrapped once, below.
		}
		return q.UpsertListen(ctx, sqlcdb.UpsertListenParams{
			UserID:     l.UserID,
			TrackID:    track.ID,
			ClientID:   l.ClientID,
			Shuffle:    l.Shuffle,
			StartedAt:  ms(l.StartedAt),
			ListenedMs: l.Listened.Milliseconds(),
			DurationMs: track.DurationMs,
			Ended:      ended,
		})
	})
	if err != nil {
		return fmt.Errorf("record listen: %w", err)
	}
	return nil
}

// Listen is a listen as [Store.RecentListens] returns it.
type Listen struct {
	TrackID   string // The track's ULID.
	ClientID  string
	Shuffle   string
	StartedAt time.Time
	Listened  time.Duration
	Duration  time.Duration // The track's, as the listen began.
	Ended     Ending
}

// RecentListens returns up to limit of a user's listens, newest first.
func (s *Store) RecentListens(ctx context.Context, userID, limit int64) ([]Listen, error) {
	rows, err := s.rq.RecentListens(ctx, sqlcdb.RecentListensParams{UserID: userID, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("recent listens: %w", err)
	}
	out := make([]Listen, len(rows))
	for i, row := range rows {
		out[i] = Listen{
			TrackID: row.Ulid, ClientID: row.ClientID, Shuffle: row.Shuffle,
			StartedAt: time.UnixMilli(row.StartedAt).UTC(),
			Listened:  time.Duration(row.ListenedMs) * time.Millisecond,
			Duration:  time.Duration(row.DurationMs) * time.Millisecond,
		}
		if row.Ended != nil {
			out[i].Ended = Ending(*row.Ended)
		}
	}
	return out, nil
}
