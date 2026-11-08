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
	if _, err = w.ExecContext(t.Context(), "CREATE TABLE t (x BLOB)"); err != nil {
		t.Fatal(err)
	}
	return &Store{w: w, wq: sqlcdb.New(w), now: time.Now}, fv
}

// failWrite runs a write in inTx that the disk refuses.
func failWrite(t *testing.T) error {
	t.Helper()
	s, fv := openFaulty(t)
	return s.inTx(t.Context(), func(_ *sqlcdb.Queries, tx *sql.Tx) error {
		fv.failing.Store(true)
		_, err := tx.ExecContext(t.Context(), "INSERT INTO t VALUES (zeroblob(1 << 16))")
		if err != nil {
			return fmt.Errorf("insert: %w", err)
		}
		return nil
	})
}

func TestTransactionErrorNamesExtendedCode(t *testing.T) {
	t.Parallel()
	err := failWrite(t)
	if !errors.Is(err, sqlite3.IOERR_WRITE) {
		t.Fatalf("err = %v, want IOERR_WRITE", err)
	}
	if !strings.Contains(err.Error(), "(IOERR_WRITE)") {
		t.Errorf("err = %q, want it to name IOERR_WRITE", err)
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
