package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed" // Embeds schema.sql.
	"encoding/binary"
	"errors"
	"fmt"
)

//go:embed schema.sql
var schemaSQL string

// kitsuneAppID marks a SQLite file as Kitsune's ("KTSN").
const kitsuneAppID = 0x4B54534E

// probeSQL reads what settleSchema decides on: the number of schema rows, the
// application_id and the user_version.
const probeSQL = `SELECT (SELECT count(*) FROM sqlite_schema), application_id, user_version
FROM pragma_application_id, pragma_user_version`

// schemaVersion derives user_version from the schema text, so any edit to
// schema.sql is detected.
func schemaVersion(schema string) int32 {
	sum := sha256.Sum256([]byte(schema))
	return int32(binary.BigEndian.Uint32(sum[:4]) >> 1)
}

// settleSchema creates the schema in an empty database, and checks that a
// populated one is Kitsune's and was made from the embedded schema.sql.
func settleSchema(ctx context.Context, w *sql.DB, path string) error {
	return settleSchemaWith(ctx, w, path, schemaSQL)
}

// settleSchemaWith is settleSchema with the schema text injected, so tests can
// try a broken one.
func settleSchemaWith(ctx context.Context, w *sql.DB, path, schema string) error {
	var rows, appID, version int64
	if err := w.QueryRowContext(ctx, probeSQL).Scan(&rows, &appID, &version); err != nil {
		return fmt.Errorf("open %s: probe: %w", path, err)
	}

	switch {
	case rows == 0:
		return createSchema(ctx, w, path, schema)
	case appID != kitsuneAppID:
		return fmt.Errorf("open %s: %w", path, ErrNotKitsune)
	case version != int64(schemaVersion(schema)):
		return fmt.Errorf("open %s: %w", path, ErrStaleSchema)
	}
	return nil
}

// createSchema runs schema and marks the file as Kitsune's, in one transaction.
func createSchema(ctx context.Context, w *sql.DB, path, schema string) error {
	tx, err := w.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("open %s: begin: %w", path, err)
	}
	// Pragmas can't take bound parameters; both values are integers computed here.
	for _, stmt := range []string{
		schema,
		fmt.Sprintf("PRAGMA application_id = %d", kitsuneAppID),
		fmt.Sprintf("PRAGMA user_version = %d", schemaVersion(schema)),
	} {
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			err = fmt.Errorf("open %s: create schema: %w", path, err)
			if rbErr := tx.Rollback(); rbErr != nil {
				err = errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
			}
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("open %s: commit: %w", path, err)
	}
	return nil
}
