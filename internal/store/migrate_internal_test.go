package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ncruces/go-sqlite3"
	sqlitedriver "github.com/ncruces/go-sqlite3/driver"
)

const (
	tracksTable = "tracks"
	movedAside  = "move the file aside"
)

// seedWords are the tracks seedLibrary writes, one each, ULIDs ascending.
var seedWords = []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot"} //nolint:gochecknoglobals // Read-only fixture shared by the tests below.

// scratchDB is an empty database behind a writer, as Open would have it.
type scratchDB struct {
	w    *sql.DB
	path string
}

func newScratchDB(t *testing.T) scratchDB {
	t.Helper()
	path := filepath.Join(t.TempDir(), FileName)
	w, err := openWriter(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return scratchDB{w: w, path: path}
}

// settle migrates the database to the end of steps.
func (d scratchDB) settle(ctx context.Context, steps []migration) error {
	return settleSchemaWith(ctx, d.w, d.path, steps, newUpgrade(&Options{Migrate: true}, time.Now))
}

// mustSettle is settle for a step that has to work.
func (d scratchDB) mustSettle(t *testing.T, steps []migration) {
	t.Helper()
	if err := d.settle(t.Context(), steps); err != nil {
		t.Fatal(err)
	}
}

func shippedSteps(t *testing.T) []migration {
	t.Helper()
	steps, err := parseMigrations(migrationFiles)
	if err != nil {
		t.Fatal(err)
	}
	return steps
}

// withStep returns steps with one more migration on the end.
func withStep(steps []migration, label, body string) []migration {
	next := int64(len(steps) + 1)
	return append(slices.Clone(steps), migration{version: next, name: fmt.Sprintf("%04d_%s.sql", next, label), sql: body})
}

// tableColumns lists table's columns.
func tableColumns(t *testing.T, db *sql.DB, table string) []string {
	t.Helper()
	return rawRows(t, db, "SELECT name FROM pragma_table_info(?)", table)
}

// tableSource is the statements behind a table.
type tableSource struct {
	create  string
	indexes []string
}

// tableDDL returns the statements that made table and its indexes, with the
// quotes a rename leaves on a table's name taken off.
func tableDDL(t *testing.T, db *sql.DB, table string) tableSource {
	t.Helper()
	var src tableSource
	for _, o := range schemaObjects(t, db) {
		switch {
		case o.kind == "table" && o.name == table:
			src.create = normalizeDDL(o.ddl)
		case o.kind == "index" && strings.Contains(o.ddl, " ON "+table+" "):
			src.indexes = append(src.indexes, o.ddl)
		}
	}
	return src
}

// rebuildSQL is a correct rebuild of table, the way AGENTS.md says to write
// one. keep picks the indexes it recreates.
func rebuildSQL(t *testing.T, db *sql.DB, table string, keep func(string) bool) string {
	t.Helper()
	src := tableDDL(t, db, table)
	columns := strings.Join(tableColumns(t, db, table), ", ")
	fresh := table + "_new"
	var b strings.Builder
	fmt.Fprintf(&b, "%s;\n", strings.Replace(src.create, "CREATE TABLE "+table, "CREATE TABLE "+fresh, 1))
	fmt.Fprintf(&b, "INSERT INTO %s (%s) SELECT %s FROM %s;\n", fresh, columns, columns, table)
	fmt.Fprintf(&b, "DROP TABLE %s;\nALTER TABLE %s RENAME TO %s;\n", table, fresh, table)
	for _, ddl := range src.indexes {
		if keep(ddl) {
			fmt.Fprintf(&b, "%s;\n", ddl)
		}
	}
	return b.String()
}

func keepAll(string) bool { return true }

// provingSteps is the shipped migrations plus real rebuilds of the two tables
// that lose data most easily. They change nothing, so the schema and the data
// must come out the same. They prove the runner without shipping a migration.
func provingSteps(t *testing.T) []migration {
	t.Helper()
	d := newScratchDB(t)
	steps := shippedSteps(t)
	d.mustSettle(t, steps)
	steps = withStep(steps, "rebuild_tracks", rebuildSQL(t, d.w, tracksTable, keepAll))
	return withStep(steps, "rebuild_track_tags", rebuildSQL(t, d.w, "track_tags", keepAll))
}

// seedLibrary writes a user, a session and a small library, with every table a
// migration could hurt filled and foreign keys on.
func seedLibrary(t *testing.T, db *sql.DB) {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(t.Context(), query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	digest := func(n int) []byte { return []byte(fmt.Sprintf("%032d", n)) }
	exec("INSERT INTO users (id, name, password_hash, role, created_at) VALUES (1, 'simon', 'h', 'admin', 1)")
	exec("INSERT INTO sessions (token_hash, user_id, created_at, last_seen, expires_at, user_agent) VALUES (?, 1, 1, 1, 2, 'test')", digest(0))
	exec("INSERT INTO artists (id, ulid, match_key, name, sort_name) VALUES (1, ?, 'band', 'Band', 'Band')", fmt.Sprintf("A%025d", 1))
	exec("INSERT INTO artwork (hash, width, height, accent, ground, source) VALUES (?, 1, 1, '#000000', '#ffffff', 'folder')", digest(0))
	exec(`INSERT INTO albums (id, ulid, group_key, name, sort_name, artist, compilation, artwork_hash, added_at)
		VALUES (1, ?, 'g', 'Album', 'Album', 'Band', 0, ?, 1)`, fmt.Sprintf("B%025d", 1), digest(0))
	exec("INSERT INTO scan_issues (root, rel_path, kind, message, size, mtime, seen_at) VALUES ('/m', 'bad.flac', 'corrupted', 'x', 1, 1, 1)")
	for i, word := range seedWords {
		n := i + 1
		title := "Hørizøn " + word
		exec(`INSERT INTO tracks (id, ulid, root, rel_path, size, mtime, audio_digest, album_id, format, codec, duration_ms,
				lossless, title, sort_title, compilation, scan_warnings, added_at)
			VALUES (?, ?, '/m', ?, 1, 1, ?, 1, 'flac', 'flac', 1000, 1, ?, ?, 0, '[]', 1)`,
			n, fmt.Sprintf("T%025d", n), word+".flac", digest(n), title, title)
		exec("INSERT INTO track_tags (track_id, key, ord, value) VALUES (?, 'TITLE', 0, ?)", n, title)
		exec("INSERT INTO track_artists (track_id, role, ord, artist_id) VALUES (?, 'artist', 0, 1)", n)
		exec("INSERT INTO tracks_fts (rowid, title, artists, album, album_artist) VALUES (?, ?, 'Band', 'Album', 'Band')", n, foldSearch(title))
		if hasTable(t, db, "plays") {
			exec("INSERT INTO plays (user_id, track_id, played_at, source, client_id) VALUES (1, ?, ?, 'web', ?)", n, n, word)
		}
		if hasTable(t, db, "track_hearts") && n%2 == 1 {
			exec("INSERT INTO track_hearts (user_id, track_id, hearted_at) VALUES (1, ?, ?)", n, n)
		}
		if hasTable(t, db, "track_ratings") && n <= MaxStars {
			exec("INSERT INTO track_ratings (user_id, track_id, stars, rated_at) VALUES (1, ?, ?, ?)", n, n, n)
		}
	}
	if hasTable(t, db, "album_hearts") {
		exec("INSERT INTO album_hearts (user_id, album_id, hearted_at) VALUES (1, 1, 1)")
	}
	if hasTable(t, db, "artist_hearts") {
		exec("INSERT INTO artist_hearts (user_id, artist_id, hearted_at) VALUES (1, 1, 1)")
	}
	seedLastfm(t, db)
}

// seedLastfm writes a Last.fm account for seedLibrary's user, with a scrobble and
// a love waiting, once the database has the tables.
func seedLastfm(t *testing.T, db *sql.DB) {
	t.Helper()
	if !hasTable(t, db, "lastfm_accounts") {
		return
	}
	for _, query := range []string{
		`INSERT INTO lastfm_accounts (user_id, name, session_key, scrobble, now_playing, loves, state, connected_at, last_sent_at)
			VALUES (1, 'simonhull', 'sk', 1, 1, 0, 'reconnect', 1, 2)`,
		"INSERT INTO lastfm_outbox (id, user_id, kind, track_id, at, attempts, next_at) VALUES (1, 1, 'scrobble', 1, 1, 2, 3)",
		"INSERT INTO lastfm_outbox (id, user_id, kind, track_id, at, attempts, next_at) VALUES (2, 1, 'love', 2, 2, 0, 3)",
	} {
		if _, err := db.ExecContext(t.Context(), query); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
}

// hasTable reports whether the database has reached the version with table.
func hasTable(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	return len(rawRows(t, db, "SELECT name FROM sqlite_schema WHERE type = 'table' AND name = ?", table)) == 1
}

// markFacts lists, in public IDs, every mark a table holds, for the tables the
// database has: a database seeded before them says the same on both sides.
var markFacts = []struct{ table, query string }{ //nolint:gochecknoglobals // Read-only fixture shared by the tests below.
	{"track_hearts", "SELECT 'heart', t.ulid, u.name, h.hearted_at FROM track_hearts h JOIN tracks t ON t.id = h.track_id JOIN users u ON u.id = h.user_id ORDER BY 2"},
	{"album_hearts", "SELECT 'heart', a.ulid, u.name, h.hearted_at FROM album_hearts h JOIN albums a ON a.id = h.album_id JOIN users u ON u.id = h.user_id ORDER BY 2"},
	{"artist_hearts", "SELECT 'heart', a.ulid, u.name, h.hearted_at FROM artist_hearts h JOIN artists a ON a.id = h.artist_id JOIN users u ON u.id = h.user_id ORDER BY 2"},
	{"track_ratings", "SELECT 'stars', t.ulid, u.name, r.stars, r.rated_at FROM track_ratings r JOIN tracks t ON t.id = r.track_id JOIN users u ON u.id = r.user_id ORDER BY 2"},
}

// lastfmFacts lists, in public IDs, every Last.fm account and outbox item, as
// markFacts does marks.
var lastfmFacts = []struct{ table, query string }{ //nolint:gochecknoglobals // Read-only fixture shared by the tests below.
	{"lastfm_accounts", "SELECT 'lastfm', u.name, a.name, a.session_key, a.scrobble, a.now_playing, a.loves, a.state, a.connected_at, a.last_sent_at FROM lastfm_accounts a JOIN users u ON u.id = a.user_id ORDER BY 2"},
	{"lastfm_outbox", "SELECT 'outbox', o.id, u.name, o.kind, t.ulid, o.at, o.attempts, o.next_at FROM lastfm_outbox o JOIN tracks t ON t.id = o.track_id JOIN users u ON u.id = o.user_id ORDER BY 2"},
}

// rawRows returns a query's rows, each row as one line.
func rawRows(t *testing.T, db *sql.DB, query string, args ...any) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	values := make([]any, len(columns))
	targets := make([]any, len(columns))
	for i := range values {
		targets[i] = &values[i]
	}
	var lines []string
	for rows.Next() {
		if err = rows.Scan(targets...); err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		for i, v := range values {
			if i > 0 {
				b.WriteByte(' ')
			}
			if blob, isBlob := v.([]byte); isBlob {
				v = string(blob)
			}
			fmt.Fprint(&b, v)
		}
		lines = append(lines, b.String())
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}

// libraryFacts is what must survive a migration, said in public IDs: row counts
// per table, then who owns what, then which ULID each FTS word finds. Plays and
// marks are listed where their tables exist, so a database seeded before them
// says the same on both sides: nothing.
func libraryFacts(t *testing.T, db *sql.DB) []string {
	t.Helper()
	facts := make([]string, 0, 32)
	for table := range strings.FieldsSeq("users sessions artists artwork albums tracks track_tags track_artists scan_issues") {
		facts = append(facts, rawRows(t, db, "SELECT '"+table+"', count(*) FROM "+table)...)
	}
	for _, query := range []string{
		"SELECT s.token_hash, u.name FROM sessions s JOIN users u ON u.id = s.user_id ORDER BY 1",
		"SELECT t.ulid, t.rel_path, a.ulid, a.artwork_hash FROM tracks t JOIN albums a ON a.id = t.album_id ORDER BY 1",
		"SELECT t.ulid, g.key, g.ord, g.value FROM track_tags g JOIN tracks t ON t.id = g.track_id ORDER BY 1, 2, 3",
		"SELECT t.ulid, l.role, l.ord, a.ulid FROM track_artists l JOIN tracks t ON t.id = l.track_id JOIN artists a ON a.id = l.artist_id ORDER BY 1, 2, 3",
	} {
		facts = append(facts, rawRows(t, db, query)...)
	}
	if hasTable(t, db, "plays") {
		facts = append(facts, rawRows(t, db, `SELECT t.ulid, u.name, p.played_at, p.source, p.client_id
			FROM plays p JOIN tracks t ON t.id = p.track_id JOIN users u ON u.id = p.user_id ORDER BY 1`)...)
	}
	for _, m := range slices.Concat(markFacts, lastfmFacts) {
		if hasTable(t, db, m.table) {
			facts = append(facts, rawRows(t, db, m.query)...)
		}
	}
	for _, word := range seedWords {
		hits := rawRows(t, db, `SELECT t.ulid FROM tracks_fts CROSS JOIN tracks t ON t.id = tracks_fts.rowid
			WHERE tracks_fts MATCH ?`, word)
		facts = append(facts, word+" -> "+strings.Join(hits, ","))
	}
	return facts
}

// expectFacts fails unless the facts are what they were.
func expectFacts(t *testing.T, db *sql.DB, want []string) {
	t.Helper()
	if got := libraryFacts(t, db); !slices.Equal(got, want) {
		t.Errorf("the data changed.\n%s", firstDiff(strings.Join(got, "\n"), strings.Join(want, "\n")))
	}
}

func TestFactsNameEveryTrack(t *testing.T) {
	t.Parallel()
	d := newScratchDB(t)
	d.mustSettle(t, shippedSteps(t))
	seedLibrary(t, d.w)
	facts := strings.Join(libraryFacts(t, d.w), "\n")
	for i, word := range seedWords {
		if want := fmt.Sprintf("%s -> T%025d", word, i+1); !strings.Contains(facts, want) {
			t.Errorf("facts lack %q:\n%s", want, facts)
		}
	}
}

// TestPrefixes starts from every version, with rows written at that version.
func TestPrefixes(t *testing.T) {
	t.Parallel()
	steps := provingSteps(t)
	for k := 1; k <= len(steps); k++ {
		t.Run(steps[k-1].name, func(t *testing.T) {
			t.Parallel()
			d := newScratchDB(t)
			d.mustSettle(t, steps[:k])
			seedLibrary(t, d.w)
			before := libraryFacts(t, d.w)

			d.mustSettle(t, steps)

			expectFacts(t, d.w, before)
			if got := rawInt(t, d.w, "PRAGMA user_version"); got != int64(len(steps)) {
				t.Errorf("user_version = %d, want %d", got, len(steps))
			}
			if got := schemaSnapshot(t, d.w); got != committedSnapshot(t) {
				t.Errorf("a rebuild that changes nothing changed the snapshot.\n%s", firstDiff(got, committedSnapshot(t)))
			}
		})
	}
}

func TestRebuildForgettingAnIndexFailsTheSnapshot(t *testing.T) {
	t.Parallel()
	d := newScratchDB(t)
	shipped := shippedSteps(t)
	d.mustSettle(t, shipped)
	forgetful := rebuildSQL(t, d.w, "albums", func(ddl string) bool { return !strings.Contains(ddl, "albums_sort") })

	d.mustSettle(t, withStep(shipped, "rebuild_albums", forgetful)) // Open passes: that is the danger.

	got := schemaSnapshot(t, d.w)
	if got == committedSnapshot(t) {
		t.Fatal("a forgotten index passed the snapshot")
	}
	if strings.Contains(got, "albums_sort") {
		t.Fatal("the mutant kept albums_sort, so it proves nothing")
	}
}

// expectRolledBack runs a bad migration over seeded rows and checks that it
// failed naming its file, with the data and the version as they were.
func expectRolledBack(t *testing.T, d scratchDB, steps []migration, want error) {
	t.Helper()
	before := libraryFacts(t, d.w)
	version := rawInt(t, d.w, "PRAGMA user_version")
	err := d.settle(t.Context(), steps)
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if name := steps[len(steps)-1].name; !strings.Contains(err.Error(), name) {
		t.Errorf("error %q does not name %s", err, name)
	}
	if got := rawInt(t, d.w, "PRAGMA user_version"); got != version {
		t.Errorf("user_version = %d, want %d", got, version)
	}
	expectFacts(t, d.w, before)
	if got := rawInt(t, d.w, "PRAGMA foreign_keys"); got != 1 {
		t.Errorf("foreign_keys = %d after a failed migration, want 1", got)
	}
}

// seededDB is a database at the shipped version with rows in it.
func seededDB(t *testing.T) (scratchDB, []migration) {
	t.Helper()
	d := newScratchDB(t)
	steps := shippedSteps(t)
	d.mustSettle(t, steps)
	seedLibrary(t, d.w)
	return d, steps
}

func TestWipingMigrationRollsBack(t *testing.T) {
	t.Parallel()
	d, steps := seededDB(t)
	expectRolledBack(t, d, withStep(steps, "wipe_tracks", "DELETE FROM tracks;"), errDanglingRows)
}

func TestRenameFirstRebuildRollsBack(t *testing.T) {
	t.Parallel()
	d, steps := seededDB(t)
	src := tableDDL(t, d.w, tracksTable)
	columns := strings.Join(tableColumns(t, d.w, tracksTable), ", ")
	body := "ALTER TABLE tracks RENAME TO tracks_old;\n" + src.create + ";\n" +
		"INSERT INTO tracks (" + columns + ") SELECT " + columns + " FROM tracks_old;\n" +
		"DROP TABLE tracks_old;\n" + strings.Join(src.indexes, ";\n") + ";\n"
	expectRolledBack(t, d, withStep(steps, "rename_first", body), errDanglingRows)
}

// TestRebuildDroppingIDsIsCaught is the mutant foreign_key_check can't see: tags
// and FTS rows land on the wrong tracks with every key still valid. Only the
// prefix test's facts catch it, so this proves they do.
func TestRebuildDroppingIDsIsCaught(t *testing.T) {
	t.Parallel()
	d, steps := seededDB(t)
	before := libraryFacts(t, d.w)
	src := tableDDL(t, d.w, tracksTable)
	columns := strings.Join(slices.DeleteFunc(tableColumns(t, d.w, tracksTable), func(c string) bool { return c == "id" }), ", ")
	body := strings.Replace(src.create, "CREATE TABLE tracks", "CREATE TABLE tracks_new", 1) + ";\n" +
		"INSERT INTO tracks_new (" + columns + ") SELECT " + columns + " FROM tracks ORDER BY ulid DESC;\n" +
		"DROP TABLE tracks;\nALTER TABLE tracks_new RENAME TO tracks;\n" + strings.Join(src.indexes, ";\n") + ";\n"

	d.mustSettle(t, withStep(steps, "drop_ids", body))

	if slices.Equal(libraryFacts(t, d.w), before) {
		t.Error("tags moved to the wrong tracks and the facts did not notice")
	}
}

func TestMidFileCommitFailsLoudly(t *testing.T) {
	t.Parallel()
	d, steps := seededDB(t)
	mutant := withStep(steps, "commit_midway", "CREATE TABLE half (x INTEGER) STRICT;\nCOMMIT;\nCREATE TABLE rest (x INTEGER) STRICT;\n")

	err := d.settle(t.Context(), mutant)

	if !errors.Is(err, errTransactionLost) {
		t.Fatalf("err = %v, want errTransactionLost", err)
	}
	if !strings.Contains(err.Error(), mutant[len(mutant)-1].name) {
		t.Errorf("error %q does not name the file", err)
	}
	if got := rawInt(t, d.w, "PRAGMA user_version"); got != int64(len(steps)) {
		t.Errorf("user_version = %d, want %d", got, len(steps))
	}
	if got := rawInt(t, d.w, "PRAGMA foreign_keys"); got != 1 {
		t.Errorf("foreign_keys = %d, want 1", got)
	}
}

// rejectControl runs body with transaction control and pragmas denied. SQLite's
// own parser decides, so a BEGIN in a trigger or a COMMIT in a string is fine.
func rejectControl(ctx context.Context, conn *sql.Conn, body string) error {
	denied := []sqlite3.AuthorizerActionCode{
		sqlite3.AUTH_TRANSACTION, sqlite3.AUTH_SAVEPOINT, sqlite3.AUTH_PRAGMA, sqlite3.AUTH_ATTACH, sqlite3.AUTH_DETACH,
	}
	deny := func(action sqlite3.AuthorizerActionCode, _, _, _, _ string) sqlite3.AuthorizerReturnCode {
		if slices.Contains(denied, action) {
			return sqlite3.AUTH_DENY
		}
		return sqlite3.AUTH_OK
	}
	authorize := func(cb func(sqlite3.AuthorizerActionCode, string, string, string, string) sqlite3.AuthorizerReturnCode) error {
		return conn.Raw(func(raw any) error {
			c, ok := raw.(sqlitedriver.Conn)
			if !ok {
				return errNotSQLite
			}
			return c.Raw().SetAuthorizer(cb)
		})
	}
	if err := authorize(deny); err != nil {
		return err
	}
	_, err := conn.ExecContext(ctx, body)
	return errors.Join(err, authorize(nil))
}

func pinned(t *testing.T, db *sql.DB) *sql.Conn {
	t.Helper()
	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestShippedMigrationsLeaveTransactionsAlone(t *testing.T) {
	t.Parallel()
	conn := pinned(t, newScratchDB(t).w)
	for _, step := range shippedSteps(t) {
		if err := rejectControl(t.Context(), conn, step.sql); err != nil {
			t.Errorf("%s: %v: transaction control and PRAGMA belong to the runner", step.name, err)
		}
	}
}

func TestRejectControl(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, body string
		ok         bool
	}{
		{"commit", "CREATE TABLE a (x INTEGER);\nCOMMIT;\nCREATE TABLE b (x INTEGER);", false},
		{"end", "END;", false},
		{"rollback", "ROLLBACK;", false},
		{"begin", "BEGIN;", false},
		{"savepoint", "SAVEPOINT s;", false},
		{"pragma", "PRAGMA foreign_keys = ON;", false},
		{"attach", "ATTACH ':memory:' AS m;", false},
		{"words in a comment and a string", "-- COMMIT;\nCREATE TABLE a (x TEXT DEFAULT 'COMMIT;');", true},
		{"a trigger's BEGIN and END", `CREATE TABLE a (x INTEGER);
			CREATE TRIGGER t AFTER INSERT ON a BEGIN
				DELETE FROM a WHERE x < 0;
			END;`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := rejectControl(t.Context(), pinned(t, newScratchDB(t).w), tt.body)
			if (err == nil) != tt.ok {
				t.Errorf("err = %v, want ok = %t", err, tt.ok)
			}
		})
	}
}

func TestFailedStepLeavesEarlierStepsDone(t *testing.T) {
	t.Parallel()
	d := newScratchDB(t)
	shipped := shippedSteps(t)
	good := withStep(shipped, "good", "CREATE TABLE kept (x INTEGER) STRICT;")
	bad := withStep(good, "bad", "CREATE TABLE undone (x INTEGER) STRICT;\nINSERT INTO nowhere VALUES (1);")

	err := d.settle(t.Context(), bad)

	if err == nil || !strings.Contains(err.Error(), bad[len(bad)-1].name) {
		t.Fatalf("err = %v, want one naming the failed file", err)
	}
	if got := rawInt(t, d.w, "PRAGMA user_version"); got != int64(len(good)) {
		t.Errorf("user_version = %d, want %d", got, len(good))
	}
	if got := rawInt(t, d.w, "SELECT count(*) FROM sqlite_schema WHERE name = 'kept'"); got != 1 {
		t.Errorf("kept tables = %d, want 1", got)
	}
	if got := rawInt(t, d.w, "SELECT count(*) FROM sqlite_schema WHERE name = 'undone'"); got != 0 {
		t.Errorf("undone tables = %d, want 0", got)
	}
}

func TestForeignKeysRestored(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		fail bool
	}{
		{"after success", "CREATE TABLE fine (x INTEGER) STRICT;", false},
		{"after bad SQL", "INSERT INTO nowhere VALUES (1);", true},
		{"after a failed check", "DELETE FROM tracks;", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d, steps := seededDB(t)
			if err := d.settle(t.Context(), withStep(steps, "step", tt.body)); (err != nil) != tt.fail {
				t.Fatalf("err = %v, want failure = %t", err, tt.fail)
			}
			if got := rawInt(t, d.w, "PRAGMA foreign_keys"); got != 1 {
				t.Errorf("foreign_keys = %d, want 1", got)
			}
		})
	}
}

func TestForeignKeysRestoredAfterCancel(t *testing.T) {
	t.Parallel()
	d, steps := seededDB(t)
	endless := `CREATE TABLE big (x INTEGER) STRICT;
		WITH RECURSIVE c (x) AS (SELECT 1 UNION ALL SELECT x + 1 FROM c) INSERT INTO big SELECT x FROM c;`
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	time.AfterFunc(50*time.Millisecond, cancel)

	err := d.settle(ctx, withStep(steps, "endless", endless))

	if err == nil {
		t.Fatal("a canceled migration succeeded")
	}
	if got := rawInt(t, d.w, "PRAGMA foreign_keys"); got != 1 {
		t.Errorf("foreign_keys = %d, want 1", got)
	}
	if got := rawInt(t, d.w, "SELECT count(*) FROM sqlite_schema WHERE name = 'big'"); got != 0 {
		t.Errorf("big tables = %d, want 0", got)
	}
}

// TestForeignKeysAreOffInsideSteps proves the rebuild's safety comes from the
// runner: a step that needs them off sees them off, and can't turn them on.
func TestForeignKeysAreOffInsideSteps(t *testing.T) {
	t.Parallel()
	d, steps := seededDB(t)
	before := libraryFacts(t, d.w)
	d.mustSettle(t, withStep(steps, "rebuild", "PRAGMA foreign_keys = ON;\n"+rebuildSQL(t, d.w, tracksTable, keepAll)))
	expectFacts(t, d.w, before)
}

func TestConcurrentStartersRunAStepOnce(t *testing.T) {
	t.Parallel()
	const starters = 4
	for round := range 5 {
		t.Run(strconv.Itoa(round), func(t *testing.T) {
			t.Parallel()
			d := newScratchDB(t)
			base := withStep(shippedSteps(t), "tally", "CREATE TABLE tally (n INTEGER NOT NULL) STRICT;\nINSERT INTO tally VALUES (0);")
			d.mustSettle(t, base)
			steps := withStep(base, "count", "UPDATE tally SET n = n + 1;")

			errs := make([]error, starters)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range starters {
				w, err := openWriter(t.Context(), d.path)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = w.Close() })
				wg.Go(func() {
					<-start
					errs[i] = scratchDB{w: w, path: d.path}.settle(t.Context(), steps)
				})
			}
			close(start)
			wg.Wait()

			if err := errors.Join(errs...); err != nil {
				t.Fatal(err)
			}
			if got := rawInt(t, d.w, "SELECT n FROM tally"); got != 1 {
				t.Errorf("the step ran %d times, want 1", got)
			}
		})
	}
}

// setVersion writes user_version through a handle it closes again.
func setVersion(t *testing.T, path string, version int64) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err = db.ExecContext(t.Context(), fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		t.Fatal(err)
	}
}

// closedStore is a database at the baseline with a user in it, closed: what a
// binary from before migrations left behind, but for the version.
func closedStore(t *testing.T) string {
	t.Helper()
	d := newScratchDB(t)
	d.mustSettle(t, shippedSteps(t)[:1])
	if _, err := d.w.ExecContext(t.Context(), "INSERT INTO users (name, password_hash, role, created_at) VALUES ('simon', 'h', 'admin', 1)"); err != nil {
		t.Fatal(err)
	}
	if err := d.w.Close(); err != nil {
		t.Fatal(err)
	}
	return d.path
}

func TestAdoption(t *testing.T) {
	t.Parallel()
	path := closedStore(t)
	setVersion(t, path, adoptHash)

	s, err := Open(t.Context(), path, Options{Migrate: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if got := rawInt(t, s.w, "PRAGMA user_version"); got != int64(len(shippedSteps(t))) {
		t.Errorf("user_version = %d, want %d", got, len(shippedSteps(t)))
	}
	if n, countErr := s.CountUsers(t.Context()); countErr != nil || n != 1 {
		t.Errorf("CountUsers = %d, %v, want 1", n, countErr)
	}
	backups, err := listBackups(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 || backups[0].from != 1 {
		t.Errorf("listBackups = %v, want one from version 1: the steps after adoption are real", backups)
	}
}

func TestAdoptionAloneNeedsNoBackup(t *testing.T) {
	t.Parallel()
	path := closedStore(t)
	setVersion(t, path, adoptHash)
	w, err := openWriter(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })

	scratchDB{w: w, path: path}.mustSettle(t, shippedSteps(t)[:1])

	if got := rawInt(t, w, "PRAGMA user_version"); got != 1 {
		t.Errorf("user_version = %d, want 1", got)
	}
	if backups, listErr := listBackups(path); listErr != nil || len(backups) != 0 {
		t.Errorf("listBackups = %v, %v, want none: adoption applies nothing", backups, listErr)
	}
}

func TestPendingMigrationsAreRefusedWithoutMigrate(t *testing.T) {
	t.Parallel()
	path := closedStore(t)
	setVersion(t, path, adoptHash)

	_, err := Open(t.Context(), path, Options{})

	if !errors.Is(err, ErrMigrationsPending) {
		t.Fatalf("err = %v, want ErrMigrationsPending", err)
	}
	if !strings.Contains(err.Error(), strconv.Itoa(adoptHash)) {
		t.Errorf("message %q doesn't report the version in the file", err)
	}
	if got := rawInt(t, rawDB(t, path), "PRAGMA user_version"); got != adoptHash {
		t.Errorf("user_version = %d, want it untouched", got)
	}
}

func TestPendingStepIsRefusedWithoutMigrate(t *testing.T) {
	t.Parallel()
	d, steps := seededDB(t)
	err := settleSchemaWith(t.Context(), d.w, d.path, withStep(steps, "next", "CREATE TABLE next (x INTEGER) STRICT;"), newUpgrade(&Options{}, time.Now))
	if !errors.Is(err, ErrMigrationsPending) {
		t.Fatalf("err = %v, want ErrMigrationsPending", err)
	}
	if got := rawInt(t, d.w, "SELECT count(*) FROM sqlite_schema WHERE name = 'next'"); got != 0 {
		t.Error("a refused migration ran")
	}
}

func TestOpenRefuses(t *testing.T) {
	t.Parallel()
	latest := int64(len(shippedSteps(t)))
	tests := []struct {
		name    string
		version int64
		want    error
		says    string
	}{
		{"no version with rows", 0, ErrUnversioned, "restore a backup"},
		{"one past latest", latest + 1, ErrNewerSchema, "newer"},
		{"just under the hashes", hashFloor - 1, ErrNewerSchema, "newer"},
		{"at the hashes", hashFloor, ErrStaleSchema, movedAside},
		{"a historic hash", 142091602, ErrStaleSchema, movedAside},
		{"negative", -5, ErrStaleSchema, movedAside},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := closedStore(t)
			setVersion(t, path, tt.version)

			_, err := Open(t.Context(), path, Options{Migrate: true})

			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if msg := err.Error(); !strings.Contains(msg, path) || !strings.Contains(msg, tt.says) {
				t.Errorf("message %q lacks the path or %q", msg, tt.says)
			}
			if got := rawInt(t, rawDB(t, path), "PRAGMA user_version"); got != tt.version {
				t.Errorf("user_version = %d, want it untouched", got)
			}
		})
	}
}

func TestEmptyFileWithAVersionIsRefused(t *testing.T) {
	t.Parallel()
	d := newScratchDB(t)
	if _, err := d.w.ExecContext(t.Context(), "PRAGMA user_version = 1"); err != nil {
		t.Fatal(err)
	}
	if err := d.settle(t.Context(), shippedSteps(t)); !errors.Is(err, ErrStaleSchema) {
		t.Fatalf("err = %v, want ErrStaleSchema", err)
	}
}

// TestDigestNullableKeepsTracks runs 0005 over a v4 library: every track keeps
// its id and fingerprint, every FTS row its rowid, and a track can then be
// stored without a fingerprint.
func TestDigestNullableKeepsTracks(t *testing.T) {
	t.Parallel()
	steps := shippedSteps(t)
	at := slices.IndexFunc(steps, func(m migration) bool { return m.name == "0005_digest_nullable.sql" })
	if at < 0 {
		t.Fatal("0005_digest_nullable.sql is not shipped")
	}
	d := newScratchDB(t)
	d.mustSettle(t, steps[:at])
	seedLibrary(t, d.w)
	const tracksQuery = "SELECT id, ulid, hex(audio_digest) FROM tracks ORDER BY id"
	const ftsQuery = "SELECT rowid FROM tracks_fts ORDER BY rowid"
	tracks, fts, facts := rawRows(t, d.w, tracksQuery), rawRows(t, d.w, ftsQuery), libraryFacts(t, d.w)

	d.mustSettle(t, steps[:at+1])

	if got := rawRows(t, d.w, tracksQuery); !slices.Equal(got, tracks) {
		t.Errorf("tracks = %q, want %q", got, tracks)
	}
	if got := rawRows(t, d.w, ftsQuery); !slices.Equal(got, fts) {
		t.Errorf("FTS rowids = %q, want %q", got, fts)
	}
	expectFacts(t, d.w, facts)
	if got := rawRows(t, d.w, "PRAGMA foreign_key_check"); len(got) != 0 {
		t.Errorf("foreign_key_check = %q, want none", got)
	}
	if _, err := d.w.ExecContext(t.Context(), "UPDATE tracks SET audio_digest = NULL WHERE id = 1"); err != nil {
		t.Errorf("clearing a fingerprint: %v", err)
	}
	if _, err := d.w.ExecContext(t.Context(), "UPDATE tracks SET audio_digest = x'00' WHERE id = 2"); err == nil {
		t.Error("a 1-byte fingerprint was stored")
	}
}
