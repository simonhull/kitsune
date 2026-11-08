package store

import (
	"path/filepath"
	"testing"
)

// TestSortsNeedNoTempDir is the scratch image: there is no /tmp, so a sort or
// index build too big for SQLite's cache must not reach for a temp file. It
// failed in production with IOERR_GETTEMPPATH while migrating 7,587 tracks.
func TestSortsNeedNoTempDir(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "kitsune.db"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	conn, err := s.w.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	for _, q := range []string{
		"PRAGMA cache_size = -64", // 64 KiB: anything sizeable spills.
		"CREATE TEMP TABLE big (v TEXT)",
		`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 50000)
		 INSERT INTO big SELECT hex(randomblob(32)) FROM n`,
		"CREATE INDEX temp.big_v ON big (v)",
	} {
		if _, err = conn.ExecContext(t.Context(), q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	var n int
	if err = conn.QueryRowContext(t.Context(), "SELECT count(*) FROM (SELECT v FROM big ORDER BY v DESC)").Scan(&n); err != nil {
		t.Fatalf("sort: %v", err)
	}
	if n != 50000 {
		t.Errorf("sorted %d rows, want 50000", n)
	}
}
