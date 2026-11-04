package store

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// The size of the benchmark library: the real one is about 1,000 albums and
// 7,500 tracks, so this is an order of magnitude more.
// Index names the plans and the schema test refer to.
const (
	indexAlbumsSort  = "albums_sort"
	indexAlbumsAdded = "albums_added"
)

const (
	seedAlbums         = 10_000
	seedTracksPerAlbum = 10
	laterPageAlbums    = 3_000 // Roughly where "a later page" starts in the by-name order.
)

// seedBrowse fills s with seedAlbums albums of seedTracksPerAlbum tracks each, by
// direct SQL: WriteBatch is far too slow for this. Every 100th album has no live
// track, and every 50th track is missing. Sort names tie in groups of two, and
// added times in groups of five.
func seedBrowse(tb testing.TB, s *Store) {
	tb.Helper()
	const (
		artworkSQL = `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < %d)
INSERT INTO artwork (hash, width, height, accent, ground, source)
SELECT CAST(printf('%%032d', i) AS BLOB), 600, 600, '#aabbcc', '#112233', 'embedded' FROM n`
		albumsSQL = `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < %d)
INSERT INTO albums (id, ulid, group_key, name, sort_name, artist, year, compilation, artwork_hash, added_at)
SELECT i, printf('%%026d', i), 'g' || i, 'Album ' || i, printf('Album %%05d', i / 2), 'Artist ' || (i %% 500),
       1970 + i %% 50, 0, CAST(printf('%%032d', i) AS BLOB), 1700000000000 + (i / 5) * 1000
FROM n`
		tracksSQL = `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < %d)
INSERT INTO tracks (id, ulid, root, rel_path, size, mtime, audio_digest, album_id, format, codec, duration_ms,
                    lossless, title, sort_title, track_no, disc_no, compilation, scan_warnings, added_at, missing_since)
SELECT i, printf('%%026d', i), '/m', 'a' || (i / %d) || '/' || i || '.flac', 1, 1, randomblob(32),
       (i - 1) / %d + 1, 'flac', 'flac', 200000, 1, 'Track ' || i, 'Track ' || i, (i - 1) %% %d + 1, 1, 0, '[]', 1,
       CASE WHEN ((i - 1) / %d + 1) %% 100 = 0 OR i %% 50 = 0 THEN 1 END
FROM n`
	)
	ctx := context.Background()
	for _, q := range []string{
		fmt.Sprintf(artworkSQL, seedAlbums),
		fmt.Sprintf(albumsSQL, seedAlbums),
		fmt.Sprintf(tracksSQL, seedAlbums*seedTracksPerAlbum, seedTracksPerAlbum, seedTracksPerAlbum, seedTracksPerAlbum, seedTracksPerAlbum),
	} {
		if _, err := s.w.ExecContext(ctx, q); err != nil {
			tb.Fatalf("seed: %v", err)
		}
	}
	if _, err := s.w.ExecContext(ctx, "ANALYZE"); err != nil {
		tb.Fatalf("analyze: %v", err)
	}
}

// laterCursors returns a cursor for each sort that starts a later page.
// cursorPair holds one later-page cursor per sort.
type cursorPair struct{ name, added string }

func laterCursors(tb testing.TB, s *Store, limit int) cursorPair {
	tb.Helper()
	var pair cursorPair
	for _, sort := range []AlbumSort{SortName, SortAdded} {
		cursor := ""
		for range laterPageAlbums / limit {
			p, err := s.Albums(tb.Context(), sort, limit, cursor)
			if err != nil || p.Next == "" {
				tb.Fatalf("%s: %+v, %v", sort, p, err)
			}
			cursor = p.Next
		}
		if sort == SortName {
			pair.name = cursor
		} else {
			pair.added = cursor
		}
	}
	return pair
}

// planner is a [sqlcdb.DBTX] that records the query plan of each query it runs.
type planner struct {
	db    *sql.DB
	plans [][]string
}

func (p *planner) ExecContext(ctx context.Context, q string, a ...any) (sql.Result, error) {
	return p.db.ExecContext(ctx, q, a...) //nolint:wrapcheck // Test helper.
}

func (p *planner) PrepareContext(ctx context.Context, q string) (*sql.Stmt, error) {
	return p.db.PrepareContext(ctx, q) //nolint:wrapcheck // Test helper.
}

func (p *planner) QueryContext(ctx context.Context, q string, a ...any) (*sql.Rows, error) {
	if err := p.explain(ctx, q, a); err != nil {
		return nil, err
	}
	return p.db.QueryContext(ctx, q, a...) //nolint:wrapcheck // Test helper.
}

func (p *planner) QueryRowContext(ctx context.Context, q string, a ...any) *sql.Row {
	_ = p.explain(ctx, q, a)
	return p.db.QueryRowContext(ctx, q, a...)
}

func (p *planner) explain(ctx context.Context, query string, args []any) error {
	rows, err := p.db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		return err //nolint:wrapcheck // Test helper.
	}
	defer func() { _ = rows.Close() }()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if scanErr := rows.Scan(&id, &parent, &unused, &detail); scanErr != nil {
			return scanErr //nolint:wrapcheck // Test helper.
		}
		plan = append(plan, detail)
	}
	p.plans = append(p.plans, plan)
	return rows.Err() //nolint:wrapcheck // Test helper.
}

// TestListPlans checks that a list page after the first seeks an index and
// sorts nothing: no scan of the albums, no temporary B-tree.
func TestListPlans(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	seedBrowse(t, s)
	later := laterCursors(t, s, DefaultAlbumLimit)
	afterName, err := decodeNameCursor(later.name)
	if err != nil {
		t.Fatal(err)
	}
	before, err := decodeAddedCursor(later.added)
	if err != nil {
		t.Fatal(err)
	}
	byName := func(after nameKey) func(context.Context, *sqlcdb.Queries) error {
		return func(ctx context.Context, q *sqlcdb.Queries) error {
			_, qErr := q.ListAlbumsByName(ctx, sqlcdb.ListAlbumsByNameParams{AfterName: after.name, AfterID: after.id, PageSize: DefaultAlbumLimit + 1})
			return qErr //nolint:wrapcheck // Test helper.
		}
	}
	byAdded := func(before addedKey) func(context.Context, *sqlcdb.Queries) error {
		return func(ctx context.Context, q *sqlcdb.Queries) error {
			_, qErr := q.ListAlbumsByAdded(ctx, sqlcdb.ListAlbumsByAddedParams{BeforeAdded: before.added, BeforeID: before.id, PageSize: DefaultAlbumLimit + 1})
			return qErr //nolint:wrapcheck // Test helper.
		}
	}

	tests := []struct {
		name  string
		index string
		run   func(context.Context, *sqlcdb.Queries) error
	}{
		{"by name", indexAlbumsSort, byName(afterName)},
		{"by added", indexAlbumsAdded, byAdded(before)},
		{"first page by name", indexAlbumsSort, byName(nameKey{})},
		{"first page by added", indexAlbumsAdded, byAdded(addedKey{added: math.MaxInt64, id: math.MaxInt64})},
	}
	for _, tc := range tests {
		pl := &planner{db: s.r}
		if runErr := tc.run(t.Context(), sqlcdb.New(pl)); runErr != nil {
			t.Fatalf("%s: %v", tc.name, runErr)
		}
		plan := strings.Join(pl.plans[0], "\n")
		t.Logf("%s:\n%s", tc.name, plan)
		if !strings.Contains(plan, "SEARCH a USING INDEX "+tc.index) {
			t.Errorf("%s: plan lacks SEARCH a USING INDEX %s:\n%s", tc.name, tc.index, plan)
		}
		if strings.Contains(plan, "TEMP B-TREE") || strings.Contains(plan, "SCAN a") {
			t.Errorf("%s: plan sorts or scans albums:\n%s", tc.name, plan)
		}
	}
}

func TestSeedCounts(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	seedBrowse(t, s)
	f, err := s.Facts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// 100 albums have no live track; of the other 9,900, every 5th album's
	// last track (i % 50 == 0) is missing.
	if f.Albums != seedAlbums-seedAlbums/100 {
		t.Errorf("live albums = %d, want %d", f.Albums, seedAlbums-seedAlbums/100)
	}
	if f.Tracks == 0 || f.Tracks >= seedAlbums*seedTracksPerAlbum {
		t.Errorf("live tracks = %d, want some but not all of %d", f.Tracks, seedAlbums*seedTracksPerAlbum)
	}
}

func BenchmarkListAlbums(b *testing.B) {
	s := openBench(b)
	seedBrowse(b, s)
	for _, limit := range []int{DefaultAlbumLimit, MaxAlbumLimit} {
		later := laterCursors(b, s, limit)
		for _, c := range []struct {
			sort   AlbumSort
			cursor string
			label  string
		}{
			{SortName, "", "name/first"},
			{SortName, later.name, "name/later"},
			{SortAdded, "", "added/first"},
			{SortAdded, later.added, "added/later"},
		} {
			b.Run(fmt.Sprintf("%s/limit=%d", c.label, limit), func(b *testing.B) {
				for b.Loop() {
					p, err := s.Albums(b.Context(), c.sort, limit, c.cursor)
					if err != nil || len(p.Albums) != limit {
						b.Fatalf("%d albums, %v", len(p.Albums), err)
					}
				}
			})
		}
	}
}

func BenchmarkAlbumDetail(b *testing.B) {
	s := openBench(b)
	seedBrowse(b, s)
	id := fmt.Sprintf("%026d", seedAlbums/2+1)
	for b.Loop() {
		d, err := s.Album(b.Context(), id)
		if err != nil || len(d.Tracks) != seedTracksPerAlbum {
			b.Fatalf("%d tracks, %v", len(d.Tracks), err)
		}
	}
}

func BenchmarkLibraryFacts(b *testing.B) {
	s := openBench(b)
	seedBrowse(b, s)
	for b.Loop() {
		if _, err := s.Facts(b.Context()); err != nil {
			b.Fatal(err)
		}
	}
}

// TestFactsQueryUsesPartialIndex makes the query's INDEXED BY fail loudly: it
// must still prepare, and run on tracks_live, so renaming the index breaks it here.
func TestFactsQueryUsesPartialIndex(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	pl := &planner{db: s.r}
	if _, err := sqlcdb.New(pl).LiveTrackFacts(t.Context()); err != nil {
		t.Fatalf("LiveTrackFacts: %v", err)
	}
	if plan := strings.Join(pl.plans[0], "\n"); !strings.Contains(plan, "tracks_live") {
		t.Errorf("plan does not use tracks_live:\n%s", plan)
	}
}
