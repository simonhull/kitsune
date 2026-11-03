package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ncruces/go-sqlite3"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "kitsune.db"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestReadersCannotWrite(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	_, err := s.r.ExecContext(t.Context(), "CREATE TABLE x (y INTEGER)")
	if err == nil {
		t.Fatal("write on the read pool succeeded")
	}
	if !errors.Is(err, sqlite3.READONLY) {
		t.Errorf("err = %v, want SQLITE_READONLY", err)
	}
}

func TestForeignKeysOn(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	for name, db := range map[string]*sql.DB{"writer": s.w, "reader": s.r} {
		var v int
		if err := db.QueryRowContext(t.Context(), "PRAGMA foreign_keys").Scan(&v); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if v != 1 {
			t.Errorf("%s: foreign_keys = %d, want 1", name, v)
		}
	}
}

func TestFTS5Registered(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	tx, err := s.w.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.ExecContext(t.Context(), "CREATE VIRTUAL TABLE t USING fts5(a)")
	if rbErr := tx.Rollback(); rbErr != nil {
		t.Fatal(rbErr)
	}
	if err != nil {
		t.Fatalf("fts5 unavailable: %v", err)
	}
}
