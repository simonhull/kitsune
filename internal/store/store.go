package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ncruces/go-sqlite3"
	_ "github.com/ncruces/go-sqlite3/driver" // Registers the "sqlite3" database/sql driver.
	"github.com/ncruces/go-sqlite3/ext/fts5"
)

// Sentinels callers match with [errors.Is].
var (
	// ErrNotFound means no row matched.
	ErrNotFound = errors.New("not found")
	// ErrNameTaken means a user with that name exists, ignoring ASCII case.
	ErrNameTaken = errors.New("name already taken")
	// ErrStaleSchema means the database was created from a different schema.sql.
	ErrStaleSchema = errors.New("database was created from a different schema.sql; delete it (Kitsune has no migrations before its first release)")
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

// busyTimeoutMS is how long a connection waits on a lock before SQLITE_BUSY.
const busyTimeoutMS = 5000

func init() { //nolint:gochecknoinits // FTS5 must be registered before any connection opens; AutoExtension is process-wide.
	sqlite3.AutoExtension(fts5.Register)
}

// Store is Kitsune's database. It is safe for concurrent use.
type Store struct {
	w, r *sql.DB
	now  func() time.Time
}

// Options configures [Open]. The zero value is ready to use.
type Options struct {
	// Now returns the current time. Nil means [time.Now].
	Now func() time.Time
	// Readers is the size of the read pool. Zero means max(2, min(GOMAXPROCS, 8)).
	Readers int
}

// Open opens or creates the database at path. It returns an error wrapping
// [ErrStaleSchema] or [ErrNotKitsune] if the file can't be used, and an error
// if opts.Readers is negative.
func Open(ctx context.Context, path string, opts Options) (*Store, error) {
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

	w, err := openWriter(ctx, path)
	if err != nil {
		return nil, err
	}
	if err = settleSchema(ctx, w, path); err != nil {
		return nil, closeAfter(err, w)
	}
	if _, err = w.ExecContext(ctx, "PRAGMA journal_mode = WAL"); err != nil {
		return nil, closeAfter(fmt.Errorf("open %s: enable WAL: %w", path, err), w)
	}

	r, err := sql.Open("sqlite3", dsn(path, false))
	if err != nil {
		return nil, closeAfter(fmt.Errorf("open %s: readers: %w", path, err), w)
	}
	r.SetMaxOpenConns(readers)
	r.SetMaxIdleConns(readers)
	r.SetConnMaxLifetime(0)
	if err = warm(ctx, r, readers); err != nil {
		return nil, closeAfter(fmt.Errorf("open %s: readers: %w", path, err), w, r)
	}

	return &Store{w: w, r: r, now: now}, nil
}

// Close closes both handles.
func (s *Store) Close() error {
	return errors.Join(s.r.Close(), s.w.Close())
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
	u := url.URL{Path: filepath.ToSlash(path)}
	return "file:" + u.EscapedPath() + "?" + q.Encode()
}
