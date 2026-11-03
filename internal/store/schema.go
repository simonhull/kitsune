package store

import (
	"context"
	"database/sql"
	_ "embed" // Embeds schema.sql.
	"fmt"
)

//go:embed schema.sql
var schemaSQL string

// settleSchema is a stub until the schema checks arrive.
func settleSchema(ctx context.Context, w *sql.DB, path string) error {
	var n int
	if err := w.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema").Scan(&n); err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	if n > 0 {
		return nil
	}
	if _, err := w.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("open %s: create schema: %w", path, err)
	}
	if _, err := w.ExecContext(ctx, "PRAGMA application_id = 1263817550; PRAGMA user_version = 1"); err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	return nil
}
