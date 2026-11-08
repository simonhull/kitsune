package store

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// albumRows keeps the index's album rows between library writes. They are most
// of what an index costs to read, since each album's owner is a subquery, and
// they change only when the library does: a heart or a play leaves them alone.
// Library writes go through [Store.inLibraryTx], which counts them.
type albumRows struct {
	writing atomic.Int32  // Library writes begun and not yet ended.
	gen     atomic.Uint64 // Library writes ended.
	cached  atomic.Pointer[cachedAlbums]
}

// cachedAlbums are the album rows as they stood after gen library writes.
type cachedAlbums struct {
	gen  uint64
	rows []sqlcdb.ListIndexAlbumsRow
}

// inLibraryTx is inTx for a write that changes what the index shows.
func (s *Store) inLibraryTx(ctx context.Context, fn func(q *sqlcdb.Queries, tx *sql.Tx) error) error {
	s.albums.writing.Add(1)
	defer func() {
		s.albums.gen.Add(1)
		s.albums.writing.Add(-1)
	}()
	return s.inTx(ctx, fn)
}

// writesMark is where library writes stood at one moment.
type writesMark struct {
	gen     uint64
	settled bool // No write was under way.
}

// before notes where library writes stand before a read transaction takes its
// snapshot.
func (a *albumRows) before() writesMark {
	return writesMark{gen: a.gen.Load(), settled: a.writing.Load() == 0}
}

// readAlbums returns the album rows as q's transaction sees them, from the
// cache when no library write began or ended between mark, taken before the
// transaction's snapshot, and now, after it, which proves the snapshot saw what
// the cache holds. Otherwise it reads them in the transaction, and keeps them
// when that proof holds for the read.
func (a *albumRows) readAlbums(ctx context.Context, q *sqlcdb.Queries, mark writesMark) ([]sqlcdb.ListIndexAlbumsRow, error) {
	gen := mark.gen
	settled := mark.settled && a.writing.Load() == 0 && a.gen.Load() == gen
	if c := a.cached.Load(); settled && c != nil && c.gen == gen {
		return c.rows, nil
	}
	rows, err := q.ListIndexAlbums(ctx)
	if err != nil {
		return nil, fmt.Errorf("albums: %w", err)
	}
	if settled {
		a.cached.Store(&cachedAlbums{gen: gen, rows: rows})
	}
	return rows, nil
}
