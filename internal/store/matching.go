package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// Lookup returns the answer source gave to the request key at or after
// notBefore, or an error wrapping [ErrNotFound] when there is none that fresh.
// The store doesn't read it: the caller keeps what it was sent.
func (s *Store) Lookup(ctx context.Context, source, key string, notBefore time.Time) ([]byte, error) {
	body, err := s.rq.GetLookup(ctx, sqlcdb.GetLookupParams{Source: source, Key: key, FetchedAt: ms(notBefore)})
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lookup %s: %w", source, err)
	}
	return body, nil
}

// PutLookup keeps source's answer to the request key, fetched at at, and drops
// every answer fetched before expired, of any source.
func (s *Store) PutLookup(ctx context.Context, source, key string, body []byte, at, expired time.Time) error {
	return s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		if err := q.PutLookup(ctx, sqlcdb.PutLookupParams{Source: source, Key: key, Body: body, FetchedAt: ms(at)}); err != nil {
			return fmt.Errorf("keep lookup %s: %w", source, err)
		}
		if err := q.PruneLookups(ctx, ms(expired)); err != nil {
			return fmt.Errorf("prune lookups: %w", err)
		}
		return nil
	})
}

// MatchState is where an album stands in the matching queue.
type MatchState string

// The states. An album with no MBID and no state is to do.
const (
	MatchToDo    MatchState = "todo"
	MatchSkipped MatchState = "skipped"
	MatchMatched MatchState = "matched"
)

// QueuedAlbum is an album the matching queue holds.
type QueuedAlbum struct {
	ID, Name, Artist string // ULID, title and display artist.
	State            MatchState
	Tracks           int // Live tracks.
}

// MatchQueue returns every album with live tracks and no MBID, and every one
// matched here whatever it holds now, in sort order.
func (s *Store) MatchQueue(ctx context.Context) ([]QueuedAlbum, error) {
	rows, err := s.rq.MatchQueue(ctx)
	if err != nil {
		return nil, fmt.Errorf("match queue: %w", err)
	}
	out := make([]QueuedAlbum, len(rows))
	for i := range rows {
		r := &rows[i]
		state := MatchToDo
		if r.State != nil {
			state = MatchState(*r.State)
		}
		out[i] = QueuedAlbum{ID: r.Ulid, Name: r.Name, Artist: r.Artist, State: state, Tracks: int(r.Tracks)}
	}
	return out, nil
}

// SetMatchState puts the album id in the queue's state. [MatchToDo] forgets the
// album's state. It returns an error wrapping [ErrNotFound] for an unknown album.
func (s *Store) SetMatchState(ctx context.Context, id string, state MatchState) error {
	if state == MatchToDo {
		known, err := s.wq.AlbumExists(ctx, id)
		if err == nil && !known {
			err = ErrNotFound
		}
		if err == nil {
			err = s.wq.ClearMatchState(ctx, id)
		}
		if err != nil {
			return fmt.Errorf("album %s: %w", id, err)
		}
		return nil
	}
	n, err := s.wq.SetMatchState(ctx, sqlcdb.SetMatchStateParams{State: string(state), At: ms(s.now()), Ulid: id})
	if err == nil && n == 0 {
		err = ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("album %s: %w", id, err)
	}
	return nil
}
