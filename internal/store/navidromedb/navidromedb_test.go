package navidromedb_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store/navidromedb"
	"github.com/simonhull/kitsune/internal/store/navidromedb/navidromedbtest"
)

// IDs in fixture.
const (
	user1   = "u1"
	album1  = "al1"
	artist1 = "ar1"
)

// fixture is a small library: one user, two songs, every kind of annotation
// and two scrobbles.
var fixture = []string{ //nolint:gochecknoglobals // Read-only fixture shared by the tests below.
	`INSERT INTO library (id, path) VALUES (1, '/music')`,
	`INSERT INTO "user" (id, user_name) VALUES ('u1', 'Simon')`,
	`INSERT INTO media_file (id, path, album_id, mbz_release_track_id, mbz_recording_id, missing)
		VALUES ('m1', 'A/one.flac', 'al1', 'rt-1', 'rec-1', false), ('m2', 'A/two.flac', 'al1', '', '', true)`,
	`INSERT INTO artist (id, name, mbz_artist_id) VALUES ('ar1', 'Band', 'mb-ar1'), ('ar2', 'Other', '')`,
	`INSERT INTO annotation (user_id, item_id, item_type, play_count, play_date, rating, starred, starred_at)
		VALUES ('u1', 'm1', 'media_file', 3, '2026-01-02 03:04:05.123456789+00:00', 4, true, '2025-03-09 01:50:33+00:00')`,
	`INSERT INTO annotation (user_id, item_id, item_type, play_count, rated_at)
		VALUES ('u1', 'm2', 'media_file', NULL, '2025-03-09 01:36:00.075+00:00')`,
	`INSERT INTO annotation (user_id, item_id, item_type, starred, starred_at) VALUES ('u1', 'al1', 'album', true, '2025-03-09 01:36:00+00:00')`,
	`INSERT INTO annotation (user_id, item_id, item_type, rating) VALUES ('u1', 'ar1', 'artist', 5)`,
	`INSERT INTO scrobbles (id, media_file_id, user_id, submission_time) VALUES (1, 'm1', 'u1', 1767323045), (2, 'm1', 'u1', 1767323000)`,
	`INSERT INTO playlist (id, owner_id) VALUES ('p1', 'u1'), ('p2', 'u2')`,
	`INSERT INTO bookmark (user_id, item_id) VALUES ('u1', 'm1'), ('u1', 'm2')`,
}

func open(t *testing.T, path string) *navidromedb.DB {
	t.Helper()
	db, err := navidromedb.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestReadsLibrariesUsersSongsAndArtists(t *testing.T) {
	t.Parallel()
	db := open(t, navidromedbtest.New(t, fixture...))
	ctx := t.Context()

	libs, err := db.Libraries(ctx)
	if err != nil || !slices.Equal(libs, []navidromedb.Library{{ID: 1, Path: "/music"}}) {
		t.Errorf("Libraries = %v, %v", libs, err)
	}
	users, err := db.Users(ctx)
	if err != nil || !slices.Equal(users, []navidromedb.User{{ID: user1, Name: "Simon"}}) {
		t.Errorf("Users = %v, %v", users, err)
	}
	songs, err := db.Songs(ctx)
	want := []navidromedb.Song{
		{ID: "m1", LibraryID: 1, Path: "A/one.flac", ReleaseTrackMBID: "rt-1", RecordingMBID: "rec-1", AlbumID: album1},
		{ID: "m2", LibraryID: 1, Path: "A/two.flac", Missing: true, AlbumID: album1},
	}
	if err != nil || !slices.Equal(songs, want) {
		t.Errorf("Songs = %+v, %v", songs, err)
	}
	artists, err := db.Artists(ctx, []string{artist1, "nope"})
	if err != nil || !slices.Equal(artists, []navidromedb.Artist{{ID: artist1, Name: "Band", MBID: "mb-ar1"}}) {
		t.Errorf("Artists = %+v, %v", artists, err)
	}
}

func TestReadsAnnotationsAndScrobbles(t *testing.T) {
	t.Parallel()
	db := open(t, navidromedbtest.New(t, fixture...))
	ctx := t.Context()

	notes, err := db.Annotations(ctx, user1)
	wantNotes := []navidromedb.Annotation{
		{ItemID: album1, ItemType: navidromedb.ItemAlbum, Starred: true, StarredAt: time.Date(2025, 3, 9, 1, 36, 0, 0, time.UTC)},
		{ItemID: artist1, ItemType: navidromedb.ItemArtist, Rating: 5},
		{
			ItemID: "m1", ItemType: navidromedb.ItemSong, PlayCount: 3, Rating: 4, Starred: true,
			PlayDate:  time.Date(2026, 1, 2, 3, 4, 5, 123456789, time.UTC),
			StarredAt: time.Date(2025, 3, 9, 1, 50, 33, 0, time.UTC),
		},
		{ItemID: "m2", ItemType: navidromedb.ItemSong, RatedAt: time.Date(2025, 3, 9, 1, 36, 0, 75e6, time.UTC)},
	}
	if err != nil || !slices.Equal(notes, wantNotes) {
		t.Errorf("Annotations = %+v, %v\nwant %+v", notes, err, wantNotes)
	}
	scrobbles, err := db.Scrobbles(ctx, user1)
	wantScrobbles := []navidromedb.Scrobble{
		{ID: 1, SongID: "m1", PlayedAt: time.Unix(1767323045, 0).UTC()},
		{ID: 2, SongID: "m1", PlayedAt: time.Unix(1767323000, 0).UTC()},
	}
	if err != nil || !slices.Equal(scrobbles, wantScrobbles) {
		t.Errorf("Scrobbles = %+v, %v", scrobbles, err)
	}
}

func TestParsesEveryFractionWidth(t *testing.T) {
	t.Parallel()
	for digits := range 10 {
		t.Run(strconv.Itoa(digits), func(t *testing.T) {
			t.Parallel()
			frac := ""
			if digits > 0 {
				frac = "." + strings.Repeat("7", digits)
			}
			path := navidromedbtest.New(t,
				`INSERT INTO "user" (id, user_name) VALUES ('u1', 'Simon')`,
				fmt.Sprintf(`INSERT INTO annotation (user_id, item_id, item_type, play_count, play_date) VALUES ('u1', 'm1', 'media_file', 1, '2025-12-07 00:29:01%s+00:00')`, frac),
			)
			notes, err := open(t, path).Annotations(t.Context(), "u1")
			if err != nil || len(notes) != 1 {
				t.Fatalf("Annotations = %v, %v", notes, err)
			}
			got := notes[0].PlayDate
			if got.Location() != time.UTC || got.Truncate(time.Second) != time.Date(2025, 12, 7, 0, 29, 1, 0, time.UTC) {
				t.Errorf("PlayDate = %v", got)
			}
			if digits > 0 && got.Nanosecond() == 0 {
				t.Errorf("PlayDate %v lost its fraction", got)
			}
		})
	}
}

func TestAnUnreadableTimeNamesTheColumnNotTheValue(t *testing.T) {
	t.Parallel()
	path := navidromedbtest.New(t,
		`INSERT INTO "user" (id, user_name) VALUES ('u1', 'Simon')`,
		`INSERT INTO annotation (user_id, item_id, item_type, starred, starred_at) VALUES ('u1', 'm1', 'media_file', true, 'last tuesday')`,
	)
	_, err := open(t, path).Annotations(t.Context(), "u1")
	if !errors.Is(err, navidromedb.ErrTime) || !strings.Contains(err.Error(), "annotation.starred_at") || strings.Contains(err.Error(), "tuesday") {
		t.Errorf("err = %v", err)
	}
}

func TestOpenCreatesNothingBesideTheFile(t *testing.T) {
	t.Parallel()
	path := navidromedbtest.New(t, fixture...)
	// A WAL database is what Navidrome leaves: opened any other way, it would
	// get -wal and -shm files beside it.
	walPath := navidromedbtest.New(t, append([]string{"PRAGMA journal_mode = WAL"}, fixture...)...)
	for _, p := range []string{path, walPath} {
		before := listDir(t, filepath.Dir(p))
		db := open(t, p)
		if _, err := db.Annotations(t.Context(), "u1"); err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		if after := listDir(t, filepath.Dir(p)); !slices.Equal(before, after) {
			t.Errorf("files beside %s: %v, then %v", filepath.Base(p), before, after)
		}
	}
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

func TestOpenRefusesANonEmptyWAL(t *testing.T) {
	t.Parallel()
	path := navidromedbtest.New(t, fixture...)
	if err := os.WriteFile(path+"-wal", []byte("frames"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := navidromedb.Open(t.Context(), path); !errors.Is(err, navidromedb.ErrWAL) {
		t.Errorf("err = %v, want ErrWAL", err)
	}
	// An empty one holds nothing, so it is no reason to refuse.
	if err := os.WriteFile(path+"-wal", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	open(t, path)
}

func TestOpenNamesWhatIsMissing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, statement, want string
	}{
		{name: "column", statement: "ALTER TABLE annotation DROP COLUMN rated_at", want: "table annotation has no column rated_at"},
		{name: "table", statement: "DROP TABLE artist", want: "no table artist"},
		{name: "optional table's column", statement: "ALTER TABLE scrobbles DROP COLUMN submission_time", want: "table scrobbles has no column submission_time"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := navidromedb.Open(t.Context(), navidromedbtest.New(t, tt.statement))
			if !errors.Is(err, navidromedb.ErrShape) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want ErrShape naming %q", err, tt.want)
			}
		})
	}
}

func TestOpenRefusesWhatIsNotADatabase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	junk := filepath.Join(dir, "junk.db")
	if err := os.WriteFile(junk, []byte("not sqlite at all, not even close, really"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{junk, filepath.Join(dir, "absent.db")} {
		if _, err := navidromedb.Open(t.Context(), p); err == nil {
			t.Errorf("Open(%s) succeeded", filepath.Base(p))
		}
	}
}

func TestNoScrobblesTableIsNoScrobbles(t *testing.T) {
	t.Parallel()
	db := open(t, navidromedbtest.New(t, append(slices.Clone(fixture), "DROP TABLE scrobbles")...))
	got, err := db.Scrobbles(t.Context(), "u1")
	if err != nil || len(got) != 0 {
		t.Errorf("Scrobbles = %v, %v; want none", got, err)
	}
}

func TestLeftCountsWhatStaysBehind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		extra []string
		want  navidromedb.Left
	}{
		{name: "both tables", want: navidromedb.Left{Playlists: 1, Bookmarks: 2}},
		{name: "neither table", extra: []string{"DROP TABLE playlist", "DROP TABLE bookmark"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := open(t, navidromedbtest.New(t, append(slices.Clone(fixture), tt.extra...)...))
			if got, err := db.Left(t.Context(), user1); err != nil || got != tt.want {
				t.Errorf("Left = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

func TestNothingReturnedHoldsASecret(t *testing.T) {
	t.Parallel()
	db := open(t, navidromedbtest.New(t, fixture...))
	ctx := t.Context()
	calls := []struct {
		name string
		call func() (any, error)
	}{
		{"Libraries", func() (any, error) { return db.Libraries(ctx) }},
		{"Users", func() (any, error) { return db.Users(ctx) }},
		{"Songs", func() (any, error) { return db.Songs(ctx) }},
		{"Annotations", func() (any, error) { return db.Annotations(ctx, user1) }},
		{"Scrobbles", func() (any, error) { return db.Scrobbles(ctx, user1) }},
		{"Artists", func() (any, error) { return db.Artists(ctx, []string{artist1, "ar2"}) }},
		{"Left", func() (any, error) { return db.Left(ctx, user1) }},
	}
	for _, c := range calls {
		got, err := c.call()
		text := fmt.Sprintf("%+v %v", got, err)
		for _, s := range navidromedbtest.Secrets() {
			if strings.Contains(text, s) {
				t.Errorf("%s returned the sentinel %q", c.name, s)
			}
		}
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

func TestSchemaMatchesWhatOpenChecks(t *testing.T) {
	t.Parallel()
	// schema.sql is what the queries were generated against: a database made
	// from it alone must pass Open's check.
	b, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "vendored.db")
	navidromedbtest.Write(t, path, string(b))
	open(t, path)
}
