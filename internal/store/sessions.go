package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// Session is a login session, with the user it belongs to. User.PasswordHash
// is always empty: the query never selects it.
type Session struct {
	User      User
	CreatedAt time.Time
	LastSeen  time.Time
	ExpiresAt time.Time
	UserAgent string
}

// NewSession is the input to [Store.CreateSession]. Every time is chosen by the caller.
type NewSession struct {
	TokenHash []byte // SHA-256 of the token; never the token.
	UserID    int64
	CreatedAt time.Time
	ExpiresAt time.Time
	UserAgent string
}

// CreateSession stores a session whose last_seen is its CreatedAt.
func (s *Store) CreateSession(ctx context.Context, ns *NewSession) error {
	err := s.wq.CreateSession(ctx, sqlcdb.CreateSessionParams{
		TokenHash: ns.TokenHash,
		UserID:    ns.UserID,
		CreatedAt: ns.CreatedAt.UnixMilli(),
		LastSeen:  ns.CreatedAt.UnixMilli(),
		ExpiresAt: ns.ExpiresAt.UnixMilli(),
		UserAgent: ns.UserAgent,
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// SessionByTokenHash returns the session with tokenHash, expired or not, or an
// error wrapping [ErrNotFound].
func (s *Store) SessionByTokenHash(ctx context.Context, tokenHash []byte) (Session, error) {
	row, err := s.rq.SessionByTokenHash(ctx, tokenHash)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, fmt.Errorf("session: %w", ErrNotFound)
	}
	if err != nil {
		return Session{}, fmt.Errorf("session by token hash: %w", err)
	}
	return Session{
		User: User{
			ID:        row.ID,
			Name:      row.Name,
			Role:      Role(row.Role),
			CreatedAt: time.UnixMilli(row.UserCreatedAt).UTC(),
		},
		CreatedAt: time.UnixMilli(row.CreatedAt).UTC(),
		LastSeen:  time.UnixMilli(row.LastSeen).UTC(),
		ExpiresAt: time.UnixMilli(row.ExpiresAt).UTC(),
		UserAgent: row.UserAgent,
	}, nil
}

// TouchSession records that the session was used at lastSeen and moves its expiry.
func (s *Store) TouchSession(ctx context.Context, tokenHash []byte, lastSeen, expiresAt time.Time) error {
	err := s.wq.TouchSession(ctx, sqlcdb.TouchSessionParams{
		LastSeen:  lastSeen.UnixMilli(),
		ExpiresAt: expiresAt.UnixMilli(),
		TokenHash: tokenHash,
	})
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return nil
}

// DeleteSession removes the session with tokenHash. Removing a missing session is not an error.
func (s *Store) DeleteSession(ctx context.Context, tokenHash []byte) error {
	if err := s.wq.DeleteSession(ctx, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteExpiredSessions removes every session that expired at or before now
// and returns how many it removed.
func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	n, err := s.wq.DeleteExpiredSessions(ctx, now.UnixMilli())
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return n, nil
}
