package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ncruces/go-sqlite3"
	_ "github.com/ncruces/go-sqlite3/driver" // Registers the "sqlite3" database/sql driver.
	"github.com/ncruces/go-sqlite3/ext/fts5"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// Sentinels callers match with [errors.Is].
var (
	// ErrNotFound means no row matched.
	ErrNotFound = errors.New("not found")
	// ErrNameTaken means a user with that name exists, ignoring ASCII case.
	ErrNameTaken = errors.New("name already taken")
	// ErrNotFirst means a first user was asked for, but a user exists.
	ErrNotFirst = errors.New("a user already exists")
	// ErrStaleSchema means the database came from a development build older than
	// migrations, with a schema they don't start from.
	ErrStaleSchema = errors.New("database predates migrations and doesn't match their baseline, so it can't be upgraded; run the build that made it, or move the file aside to start over")
	// ErrNewerSchema means the database was migrated by a newer binary.
	ErrNewerSchema = errors.New("database is newer than this binary; run a newer kitsune-server, or restore a backup taken before the upgrade")
	// ErrUnversioned means the database has tables but no schema version.
	ErrUnversioned = errors.New("database has tables but no schema version, so it can't be upgraded; restore a backup, or move the file aside to start over")
	// ErrMigrationsPending means the database needs migrating and [Options.Migrate] is off.
	ErrMigrationsPending = errors.New("database needs migrating; start the server once, which does that")
	// ErrNotKitsune means the file is a SQLite database that Kitsune did not create.
	ErrNotKitsune = errors.New("not a Kitsune database")

	// errNegativeReaders is the tail of the error for a negative Options.Readers.
	errNegativeReaders = errors.New("want 0 or more")
)

// Read pool sizing: the default is max(minReaders, min(GOMAXPROCS, maxReaders)).
const (
	minReaders = 2
	maxReaders = 8
)

// FileName is the database's file name inside data_dir.
const FileName = "kitsune.db"

// privateFileMode is the mode of a database file Kitsune creates.
const privateFileMode = 0o600

// busyTimeoutMS is how long a connection waits on a lock before SQLITE_BUSY.
const busyTimeoutMS = 5000

func init() { //nolint:gochecknoinits // FTS5 must be registered before any connection opens; AutoExtension is process-wide.
	sqlite3.AutoExtension(fts5.Register)
}

// Store is Kitsune's database. It is safe for concurrent use.
type Store struct {
	w, r   *sql.DB
	wq, rq *sqlcdb.Queries // Generated queries bound to w and r.
	now    func() time.Time

	names      atomic.Pointer[names] // Never nil once Open returns.
	refreshing sync.Mutex            // Held by a rebuild of names, so an older one can't land last.
	changed    changedAlbums         // What library writes touched, until the writer takes it.
	albums     albumRows             // The index's album rows, kept between library writes.
}

// Options configures [Open]. The zero value is ready to use.
type Options struct {
	// Now returns the current time. Nil means [time.Now].
	Now func() time.Time
	// Readers is the size of the read pool. Zero means max(2, min(GOMAXPROCS, 8)).
	Readers int
	// Migrate applies pending migrations. Only the server sets it: off, a
	// command-line tool can't migrate a database under a running server.
	Migrate bool
	// Logger reports migrations. Nil discards.
	Logger *slog.Logger
}

// logger returns Logger, or one that discards.
func (o *Options) logger() *slog.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return slog.New(slog.DiscardHandler)
}

// Open opens the database at path, creating it if it is empty. A database behind
// this binary is backed up beside itself and migrated when opts.Migrate is set,
// and otherwise refused with [ErrMigrationsPending]. A failed backup stops Open
// with the database untouched. A failed migration stops it with an error naming
// the file; that step is undone and the ones before it stay.
//
// It returns an error wrapping [ErrStaleSchema], [ErrNewerSchema],
// [ErrUnversioned] or [ErrNotKitsune] if the file can't be used, and an error if
// opts.Readers is negative.
func Open(ctx context.Context, path string, opts Options) (*Store, error) {
	s, err := open(ctx, path, opts)
	return s, named(err)
}

// open is [Open] without naming SQLite's result code.
func open(ctx context.Context, path string, opts Options) (*Store, error) {
	if opts.Readers < 0 {
		return nil, fmt.Errorf("store: Options.Readers is %d; %w", opts.Readers, errNegativeReaders)
	}
	readers := opts.Readers
	if readers == 0 {
		readers = max(minReaders, min(runtime.GOMAXPROCS(0), maxReaders))
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	if err := createPrivate(path); err != nil {
		return nil, err
	}
	w, err := openWriter(ctx, path)
	if err != nil {
		return nil, err
	}
	if err = settleSchema(ctx, w, path, newUpgrade(&opts, now)); err != nil {
		return nil, closeAfter(err, w)
	}
	if _, err = w.ExecContext(ctx, "PRAGMA journal_mode = WAL"); err != nil {
		return nil, closeAfter(fmt.Errorf("open %s: enable WAL: %w", path, err), w)
	}

	r, err := openReaders(ctx, path, readers)
	if err != nil {
		return nil, closeAfter(fmt.Errorf("open %s: readers: %w", path, err), w)
	}

	s := &Store{w: w, r: r, wq: sqlcdb.New(w), rq: sqlcdb.New(r), now: now}
	if err = s.RefreshNames(ctx); err != nil {
		return nil, closeAfter(fmt.Errorf("open %s: %w", path, err), w, r)
	}
	return s, nil
}

// Close closes both handles.
func (s *Store) Close() error {
	return errors.Join(s.r.Close(), s.w.Close())
}

// Ping checks that the read pool can reach the database. It pings readers
// only: the writer has one connection, and a long write transaction would
// stall readiness.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.r.PingContext(ctx); err != nil {
		return named(fmt.Errorf("store: ping: %w", err))
	}
	return nil
}

// createPrivate creates the database file and its shared-memory file with
// mode 0600 when they are missing. An existing file, and its mode, are left
// alone.
//
// The WAL gets the database's mode through modeof (see dsn). The VFS creates
// the -shm file with 0666 minus the umask, ignoring modeof, and deletes it when
// the last connection closes. The store holds connections open until Close, so
// a -shm created here lasts as long as the store does.
func createPrivate(path string) error {
	for _, p := range []string{path, path + "-shm"} {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, privateFileMode) //nolint:gosec // path is the operator's configured data_dir.
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("open %s: create: %w", p, err)
		}
		if err = f.Close(); err != nil {
			return fmt.Errorf("open %s: create: %w", p, err)
		}
	}
	return nil
}

// openWriter opens the single-connection writer and checks it connects.
func openWriter(ctx context.Context, path string) (*sql.DB, error) {
	w, err := sql.Open("sqlite3", dsn(path, true))
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	w.SetMaxOpenConns(1)
	w.SetMaxIdleConns(1)
	w.SetConnMaxLifetime(0)
	if err = w.PingContext(ctx); err != nil {
		return nil, closeAfter(fmt.Errorf("open %s: %w", path, err), w)
	}
	return w, nil
}

// openReaders opens the read pool and fills it.
func openReaders(ctx context.Context, path string, readers int) (*sql.DB, error) {
	r, err := sql.Open("sqlite3", dsn(path, false))
	if err != nil {
		return nil, err //nolint:wrapcheck // Open wraps this with the path.
	}
	r.SetMaxOpenConns(readers)
	r.SetMaxIdleConns(readers)
	r.SetConnMaxLifetime(0)
	if err = warm(ctx, r, readers); err != nil {
		return nil, closeAfter(err, r)
	}
	return r, nil
}

// warm opens n connections at once, so the pool starts full and idle.
func warm(ctx context.Context, db *sql.DB, n int) (err error) {
	conns := make([]*sql.Conn, 0, n)
	defer func() {
		for _, c := range conns {
			err = errors.Join(err, c.Close())
		}
	}()
	for range n {
		c, connErr := db.Conn(ctx)
		if connErr != nil {
			return fmt.Errorf("connect: %w", connErr)
		}
		conns = append(conns, c)
		if pingErr := c.PingContext(ctx); pingErr != nil {
			return fmt.Errorf("ping: %w", pingErr)
		}
	}
	return nil
}

// closeAfter joins err with the errors from closing dbs.
func closeAfter(err error, dbs ...*sql.DB) error {
	for _, db := range dbs {
		err = errors.Join(err, db.Close())
	}
	return err
}

// dsn builds the connection string. Pragmas run per connection, in order:
// busy_timeout must come first. journal_mode is deliberately absent; see Open.
func dsn(path string, writer bool) string {
	q := url.Values{}
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busyTimeoutMS))
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "synchronous(NORMAL)")
	if writer {
		q.Set("_txlock", "immediate")
	} else {
		q.Add("_pragma", "query_only(1)")
	}
	// modeof makes the VFS copy the main file's mode onto the WAL it creates;
	// without it the WAL gets 0666 minus the umask. createPrivate covers -shm.
	q.Set("modeof", path)
	u := url.URL{Path: filepath.ToSlash(path)}
	return "file:" + u.EscapedPath() + "?" + q.Encode()
}
