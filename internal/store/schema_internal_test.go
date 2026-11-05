package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func rawDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func rawInt(t *testing.T, db *sql.DB, query string) int64 {
	t.Helper()
	var v int64
	if err := db.QueryRowContext(t.Context(), query).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// schemaVersion is the hash databases carried before migrations.
func schemaVersion(schema string) int64 {
	sum := sha256.Sum256([]byte(schema))
	return int64(binary.BigEndian.Uint32(sum[:4]) >> 1)
}

func TestBaselineHashesToAdoptHash(t *testing.T) {
	t.Parallel()
	steps, err := parseMigrations(migrationFiles)
	if err != nil {
		t.Fatal(err)
	}
	if got := schemaVersion(steps[0].sql); got != adoptHash {
		t.Errorf("schemaVersion(%s) = %d, want adoptHash %d: databases made before migrations carry it", steps[0].name, got, adoptHash)
	}
}

func TestOpenRefusesForeignDatabase(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "other.db")
	raw := rawDB(t, path)
	if _, err := raw.ExecContext(t.Context(), "CREATE TABLE notes (x)"); err != nil {
		t.Fatal(err)
	}

	_, err := Open(t.Context(), path, Options{})
	if !errors.Is(err, ErrNotKitsune) {
		t.Fatalf("err = %v, want ErrNotKitsune", err)
	}
	if n := rawInt(t, raw, "SELECT count(*) FROM sqlite_schema WHERE name = 'notes'"); n != 1 {
		t.Errorf("notes tables = %d, want 1", n)
	}
	if got := rawInt(t, raw, "PRAGMA application_id"); got != 0 {
		t.Errorf("application_id = %d, want 0", got)
	}
	var mode string
	if err = raw.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "delete" {
		t.Errorf("journal_mode = %q, want delete", mode)
	}
}

func TestFreshCreateIsAtomic(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "kitsune.db")
	w, err := openWriter(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })

	broken := []migration{{version: 1, name: "0001_broken.sql", sql: "CREATE TABLE a (x INTEGER) STRICT; SELECT x FROM nope;"}}
	err = settleSchemaWith(t.Context(), w, path, broken, newUpgrade(&Options{}, time.Now))
	if err == nil {
		t.Fatal("settleSchemaWith accepted a broken schema")
	}
	for _, q := range []string{
		"SELECT count(*) FROM sqlite_schema",
		"PRAGMA application_id",
		"PRAGMA user_version",
	} {
		if got := rawInt(t, w, q); got != 0 {
			t.Errorf("%s = %d after failed create, want 0", q, got)
		}
	}
}
