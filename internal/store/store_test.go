package store_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver" // Registers the "sqlite3" driver for the raw handles below.

	"github.com/simonhull/kitsune/internal/store"
)

func pragma(t *testing.T, path, name string) int64 {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var v int64
	if err = db.QueryRowContext(t.Context(), "PRAGMA "+name).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestOpenCreatesWALDatabase(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "kitsune.db")
	s, err := store.Open(t.Context(), path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var mode string
	if err = db.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
	if got := pragma(t, path, "application_id"); got != 0x4B54534E {
		t.Errorf("application_id = %#x, want 0x4B54534E", got)
	}
	if got := pragma(t, path, "user_version"); got == 0 {
		t.Error("user_version = 0, want non-zero")
	}
}

func TestOpenRejectsNegativeReaders(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "kitsune.db")
	if _, err := store.Open(t.Context(), path, store.Options{Readers: -1}); err == nil {
		t.Fatal("Open with Readers: -1 succeeded")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("file exists after failed Open: stat err = %v", err)
	}
}

func TestOpenTwiceReuses(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "kitsune.db")
	for range 2 {
		s, err := store.Open(t.Context(), path, store.Options{})
		if err != nil {
			t.Fatal(err)
		}
		if err = s.Close(); err != nil {
			t.Fatal(err)
		}
	}
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var n int
	if err = db.QueryRowContext(t.Context(), "SELECT count(*) FROM sqlite_schema WHERE name = 'users'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("users table count = %d, want 1", n)
	}
}
