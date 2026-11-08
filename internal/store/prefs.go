package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// Pref returns the user's value for a preference key, as it was written. It
// returns an error wrapping [ErrNotFound] when the user has never set it. The
// store doesn't know the keys: the caller checks the key and the value.
func (s *Store) Pref(ctx context.Context, userID int64, key string) ([]byte, error) {
	value, err := s.rq.GetPref(ctx, sqlcdb.GetPrefParams{UserID: userID, Key: key})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("pref %s: %w", key, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("pref %s: %w", key, err)
	}
	return []byte(value), nil
}

// SetPref replaces the user's value for a preference key. The value must be JSON
// the key's rule has already accepted.
func (s *Store) SetPref(ctx context.Context, userID int64, key string, value []byte) error {
	err := s.wq.UpsertPref(ctx, sqlcdb.UpsertPrefParams{
		UserID: userID, Key: key, Value: string(value), UpdatedAt: ms(s.now()),
	})
	if err != nil {
		return fmt.Errorf("set pref %s: %w", key, err)
	}
	return nil
}
