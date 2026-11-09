package store

import (
	"context"
	"database/sql"
	"strings"
	"sync/atomic"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// libraryRows keeps what the whole index reads of the library between library
// writes: the album rows, each with its owner from a subquery, the tracks and
// their artists, and what the song columns read of them. They are most of what an
// index costs to read, and they change only when the library does: a heart or a
// play leaves them alone, so those are read every time. So are loudness measures,
// which the loudness pass writes outside library writes, a file at a time.
// Library writes go through [Store.inLibraryTx], which counts them.
type libraryRows struct {
	writing atomic.Int32  // Library writes begun and not yet ended.
	gen     atomic.Uint64 // Library writes ended.

	albums       kept[sqlcdb.ListIndexAlbumsRow]
	tracks       kept[sqlcdb.ListIndexTracksRow]
	artists      kept[sqlcdb.ListIndexTrackArtistsRow]
	details      kept[sqlcdb.ListIndexTrackDetailsRow]
	albumArtists kept[sqlcdb.ListIndexTrackAlbumArtistsRow]
	tags         kept[sqlcdb.ListIndexTrackTagsRow]
}

// kept holds one query's rows as they stood after some number of library
// writes. Readers share them, so nothing may change them.
type kept[T any] struct {
	rows atomic.Pointer[keptRows[T]]
}

type keptRows[T any] struct {
	gen  uint64
	args string // What the query was asked, for one that takes arguments.
	rows []T
}

// inLibraryTx is inTx for a write that changes what the index shows.
func (s *Store) inLibraryTx(ctx context.Context, fn func(q *sqlcdb.Queries, tx *sql.Tx) error) error {
	s.library.writing.Add(1)
	defer func() {
		s.library.gen.Add(1)
		s.library.writing.Add(-1)
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
func (l *libraryRows) before() writesMark {
	return writesMark{gen: l.gen.Load(), settled: l.writing.Load() == 0}
}

// read returns k's rows as a read transaction sees them: from what k keeps when
// no library write began or ended between mark, taken before the transaction's
// snapshot, and now, after it, which proves the snapshot saw what k holds.
// Otherwise it runs query in the transaction, and keeps the rows when that proof
// holds for the read. args tells apart the questions of a query that takes
// arguments.
func read[T any](l *libraryRows, k *kept[T], mark writesMark, args string, query func() ([]T, error)) ([]T, error) {
	settled := mark.settled && l.writing.Load() == 0 && l.gen.Load() == mark.gen
	if c := k.rows.Load(); settled && c != nil && c.gen == mark.gen && c.args == args {
		return c.rows, nil
	}
	rows, err := query()
	if err != nil {
		return nil, err
	}
	if settled {
		k.rows.Store(&keptRows[T]{gen: mark.gen, args: args, rows: rows})
	}
	return rows, nil
}

// tagArgs is what ListIndexTrackTags was asked, as one string.
func tagArgs(keys []string) string {
	return strings.Join(keys, "\x00")
}
