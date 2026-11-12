package store

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"time"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// Playback is where a person left off, as the store keeps it. Queue is JSON the
// caller has checked, and the store never reads it.
type Playback struct {
	Kind       string
	Queue      []byte
	At         int // The current song's index in the queue's tracks.
	PositionMs int64
	Ordered    bool
	// ShuffleID and ShuffleAfter name a shuffle session and the position after
	// the queue's songs; empty and 0 for any other kind.
	ShuffleID    string
	ShuffleAfter int64
	ClientID     string
	Device       string
	UpdatedAt    time.Time // Set by the store on every save.
}

// Playback returns where the user left off, or an error wrapping [ErrNotFound]
// when nothing is remembered.
func (s *Store) Playback(ctx context.Context, userID int64) (Playback, error) {
	row, err := s.rq.GetPlayback(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return Playback{}, fmt.Errorf("playback: %w", ErrNotFound)
	}
	if err != nil {
		return Playback{}, fmt.Errorf("playback: %w", err)
	}
	var after int64
	if row.ShuffleAfter != nil {
		after = *row.ShuffleAfter
	}
	return Playback{
		Kind: row.Kind, Queue: []byte(row.Queue), At: int(row.At), PositionMs: row.PositionMs,
		Ordered: row.Ordered != 0, ShuffleID: strOrEmpty(row.ShuffleID), ShuffleAfter: after,
		ClientID: row.ClientID, Device: row.Device, UpdatedAt: time.UnixMilli(row.UpdatedAt).UTC(),
	}, nil
}

// SetPlayback replaces where the user left off with p, stamped with the store's
// clock. The last write wins.
func (s *Store) SetPlayback(ctx context.Context, userID int64, p *Playback) error {
	ordered := int64(0)
	if p.Ordered {
		ordered = 1
	}
	var after *int64
	if p.ShuffleID != "" {
		after = new(p.ShuffleAfter)
	}
	err := s.wq.UpsertPlayback(ctx, sqlcdb.UpsertPlaybackParams{
		UserID: userID, Kind: p.Kind, Queue: string(p.Queue), At: int64(p.At), PositionMs: p.PositionMs,
		Ordered:   ordered,
		ShuffleID: nullStr(p.ShuffleID), ShuffleAfter: after,
		ClientID: p.ClientID, Device: p.Device, UpdatedAt: ms(s.now()),
	})
	if err != nil {
		return fmt.Errorf("set playback: %w", err)
	}
	return nil
}

// LiveTracks returns the album ULID of each of the tracks with the given ULIDs
// that is live, by track ULID. A missing or deleted track is left out.
func (s *Store) LiveTracks(ctx context.Context, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("encode IDs: %w", err)
	}
	rows, err := s.rq.ListLiveTracksByULID(ctx, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("live tracks: %w", err)
	}
	for _, r := range rows {
		out[r.Ulid] = r.AlbumUlid
	}
	return out, nil
}
