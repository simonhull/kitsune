package store

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/vfs"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// faultyVFS is the OS's VFS, except that once failing is set, writes to a
// rollback journal fail as a dying disk's would.
type faultyVFS struct {
	vfs.VFSFilename

	failing atomic.Bool
}

//nolint:ireturn // vfs.VFSFilename says what OpenFilename returns.
func (v *faultyVFS) OpenFilename(name *vfs.Filename, flags vfs.OpenFlag) (vfs.File, vfs.OpenFlag, error) {
	f, got, err := v.VFSFilename.OpenFilename(name, flags)
	if err != nil || flags&vfs.OPEN_MAIN_JOURNAL == 0 {
		return f, got, err //nolint:wrapcheck // SQLite codes the OS's errors itself.
	}
	return &faultyFile{File: f, vfs: v}, got, nil
}

type faultyFile struct {
	vfs.File

	vfs *faultyVFS
}

func (f *faultyFile) WriteAt(p []byte, off int64) (int, error) {
	if f.vfs.failing.Load() {
		return 0, syscall.EIO
	}
	return f.File.WriteAt(p, off) //nolint:wrapcheck // As in OpenFilename.
}

// openFaulty opens a writer-only store on a faulty VFS, with a rollback
// journal, since the faulty files have no shared memory for a WAL.
func openFaulty(t *testing.T) (*Store, *faultyVFS) {
	t.Helper()
	osVFS, ok := vfs.Find("").(vfs.VFSFilename)
	if !ok {
		t.Fatal("the OS VFS doesn't open by Filename")
	}
	fv := &faultyVFS{VFSFilename: osVFS}
	name := "faulty-" + t.Name()
	vfs.Register(name, fv)
	t.Cleanup(func() { vfs.Unregister(name) })

	q := url.Values{}
	q.Set("vfs", name)
	q.Set("_txlock", "immediate")
	path := filepath.ToSlash(filepath.Join(t.TempDir(), "faulty.db"))
	w, err := sql.Open("sqlite3", "file:"+path+"?"+q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	w.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = w.Close() })
	if err = settleSchema(t.Context(), w, path, newUpgrade(&Options{}, time.Now)); err != nil {
		t.Fatal(err)
	}
	if _, err = w.ExecContext(t.Context(), "CREATE TABLE t (x BLOB UNIQUE)"); err != nil {
		t.Fatal(err)
	}
	return &Store{w: w, wq: sqlcdb.New(w), now: time.Now}, fv
}

// insert runs one INSERT into t in inTx, failing the disk first if fail is set.
func insert(t *testing.T, s *Store, fv *faultyVFS, fail bool, stmt string) error {
	t.Helper()
	return s.inTx(t.Context(), func(_ *sqlcdb.Queries, tx *sql.Tx) error {
		if _, err := tx.ExecContext(t.Context(), "INSERT INTO t VALUES (x'00')"); err != nil {
			return fmt.Errorf("first insert: %w", err)
		}
		fv.failing.Store(fail)
		if _, err := tx.ExecContext(t.Context(), stmt); err != nil {
			return fmt.Errorf("insert: %w", err)
		}
		return nil
	})
}

func TestTransactionErrorNamesExtendedCode(t *testing.T) {
	t.Parallel()
	s, fv := openFaulty(t)
	err := insert(t, s, fv, true, "INSERT INTO t VALUES (zeroblob(1 << 16))")
	if !errors.Is(err, sqlite3.IOERR_WRITE) {
		t.Fatalf("err = %v, want IOERR_WRITE", err)
	}
	if !strings.HasSuffix(err.Error(), "(IOERR_WRITE)") {
		t.Errorf("err = %q, want it to name IOERR_WRITE", err)
	}
}

// TestRollbackSQLiteDidIsNotReported covers errors SQLite answers by rolling
// the transaction back itself, after which ROLLBACK fails with "no transaction
// is active": the error says what caused it, and nothing else.
func TestRollbackSQLiteDidIsNotReported(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		fail bool
		stmt string
		want string
	}{
		{"disk", true, "INSERT INTO t VALUES (zeroblob(1 << 16))", "insert: sqlite3: disk I/O error (IOERR_WRITE)"},
		{
			"or rollback", false, "INSERT OR ROLLBACK INTO t VALUES (x'00')",
			"insert: sqlite3: constraint failed: UNIQUE constraint failed: t.x (CONSTRAINT_UNIQUE)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, fv := openFaulty(t)
			err := insert(t, s, fv, tt.fail, tt.stmt)
			if err == nil || err.Error() != tt.want {
				t.Errorf("err = %q, want %q", err, tt.want)
			}
			fv.failing.Store(false)
			var n int
			if err = s.w.QueryRowContext(t.Context(), "SELECT count(*) FROM t").Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 0 {
				t.Errorf("%d rows after the failed transaction, want 0", n)
			}
		})
	}
}

func TestNamed(t *testing.T) {
	t.Parallel()
	errOther := errors.New("not from SQLite")
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"extended", fmt.Errorf("commit: %w", sqlite3.IOERR_FSYNC), "commit: sqlite3: disk I/O error (IOERR_FSYNC)"},
		{"primary", fmt.Errorf("begin: %w", sqlite3.BUSY), "begin: sqlite3: database is locked (BUSY)"},
		{"unknown", sqlite3.ExtendedErrorCode(0x7f0a), "sqlite3: disk I/O error (code 32522)"},
		{"not sqlite", errOther, "not from SQLite"},
		{"once", named(named(sqlite3.IOERR_SHMMAP)), "sqlite3: disk I/O error (IOERR_SHMMAP)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := named(tt.err)
			if got.Error() != tt.want {
				t.Errorf("named(%v) = %q, want %q", tt.err, got, tt.want)
			}
			if !errors.Is(got, tt.err) {
				t.Errorf("named(%v) no longer wraps it", tt.err)
			}
		})
	}
	if named(nil) != nil {
		t.Error("named(nil) != nil")
	}
}
