package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// Setting is one server setting as it is stored: a key, its JSON value, and
// where the value came from ("web", "server.toml", "env" or "flag"). The store
// doesn't know the keys: internal/settings checks them and their values.
type Setting struct {
	Key       string
	Value     []byte
	Source    string
	UpdatedAt time.Time
}

// SettingChange is one write of [Store.ChangeSettings]. A nil Value deletes
// the key's row.
type SettingChange struct {
	Key    string
	Value  []byte
	Source string
}

// Settings returns every stored setting, by key.
func (s *Store) Settings(ctx context.Context) ([]Setting, error) {
	rows, err := s.rq.ListSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("settings: %w", err)
	}
	out := make([]Setting, len(rows))
	for i, r := range rows {
		out[i] = Setting{Key: r.Key, Value: []byte(r.Value), Source: r.Source, UpdatedAt: time.UnixMilli(r.UpdatedAt).UTC()}
	}
	return out, nil
}

// ChangeSettings writes every change in one transaction: all of them or none.
// Values must be JSON their keys' rules have already accepted.
func (s *Store) ChangeSettings(ctx context.Context, changes []SettingChange) error {
	at := ms(s.now())
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		for _, c := range changes {
			if err := changeSetting(ctx, q, c, at); err != nil {
				return fmt.Errorf("%s: %w", c.Key, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("change settings: %w", err)
	}
	return nil
}

func changeSetting(ctx context.Context, q *sqlcdb.Queries, c SettingChange, at int64) error {
	if c.Value == nil {
		_, err := q.DeleteSetting(ctx, c.Key)
		return err //nolint:wrapcheck // The caller names the key.
	}
	return q.UpsertSetting(ctx, sqlcdb.UpsertSettingParams{ //nolint:wrapcheck // As above.
		Key: c.Key, Value: string(c.Value), Source: c.Source, UpdatedAt: at,
	})
}

// SeedSetting stores value for key unless the key already has a row, and
// reports whether it did.
func (s *Store) SeedSetting(ctx context.Context, key string, value []byte, source string) (bool, error) {
	n, err := s.wq.InsertSettingIfAbsent(ctx, sqlcdb.InsertSettingIfAbsentParams{
		Key: key, Value: string(value), Source: source, UpdatedAt: ms(s.now()),
	})
	if err != nil {
		return false, fmt.Errorf("seed setting %s: %w", key, err)
	}
	return n > 0, nil
}

// DeleteSetting deletes key's row, so the next start seeds it again, and
// reports whether there was one.
func (s *Store) DeleteSetting(ctx context.Context, key string) (bool, error) {
	n, err := s.wq.DeleteSetting(ctx, key)
	if err != nil {
		return false, fmt.Errorf("delete setting %s: %w", key, err)
	}
	return n > 0, nil
}
