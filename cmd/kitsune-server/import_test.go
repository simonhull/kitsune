package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/store"
	"github.com/simonhull/kitsune/internal/store/navidromedb/navidromedbtest"
)

const (
	importCmd    = "import"
	navidromeCmd = "navidrome"
	dryRun       = "-dry-run"
	musicRoot    = "/music"
	configOption = "-config"
)

// navidromeFixture is a Navidrome library of three songs, the second moved
// since, the third never played: Simon played and marked the first two.
var navidromeFixture = []string{ //nolint:gochecknoglobals // Read-only fixture shared by the tests below.
	`INSERT INTO library (id, path) VALUES (1, '/music')`,
	`INSERT INTO "user" (id, user_name) VALUES ('u1', 'Simon'), ('u2', 'Guest')`,
	`INSERT INTO media_file (id, path, album_id) VALUES ('m1', 'A/one.flac', 'al1'), ('m2', 'A/old two.flac', 'al1'), ('m3', 'A/three.flac', 'al1')`,
	`INSERT INTO annotation (user_id, item_id, item_type, play_count, play_date, rating, starred, starred_at) VALUES
		('u1', 'm1', 'media_file', 3, '2026-01-02 03:04:05.5+00:00', 4, true, '2025-03-09 01:50:33+00:00'),
		('u1', 'm2', 'media_file', 2, '2025-11-01 00:00:00+00:00', 5, false, NULL),
		('u1', 'al1', 'album', 0, NULL, 0, true, '2025-03-09 01:36:00+00:00')`,
	`INSERT INTO scrobbles (id, media_file_id, user_id, submission_time) VALUES (1, 'm1', 'u1', 1767323045)`,
}

// mapMoved maps the fixture's moved song onto its file in Kitsune.
const mapMoved = "A/old two.flac=A/two.flac"

// importEnv is a Kitsune data dir with an account, simon, connected to
// Last.fm with everything on, and a library of three tracks.
func newImportEnv(t *testing.T) userEnv {
	t.Helper()
	useCheapParams(t)
	e := newUserEnv(t)
	if r := e.usersAdd(goodPassword, passwordStdin, simonName); r.code != exitOK {
		t.Fatalf("users add: %+v", r)
	}
	st := e.openStore(t)
	album := store.AlbumWrite{GroupKey: "al", Name: "Album", SortName: "Album", ULID: fmt.Sprintf("B%025d", 1), Artist: "Band"}
	var b store.Batch
	for i, rel := range []string{"A/one.flac", "A/two.flac", "A/three.flac"} {
		b.Tracks = append(b.Tracks, store.TrackWrite{
			ULID: fmt.Sprintf("T%025d", i+1), Root: musicRoot, RelPath: rel, Format: "flac", Codec: "flac",
			Title: rel, SortTitle: rel, Album: album, MTime: time.UnixMilli(1),
		})
	}
	if err := st.WriteBatch(t.Context(), &b); err != nil {
		t.Fatal(err)
	}
	u, err := st.UserByName(t.Context(), simonName)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.ConnectLastfm(t.Context(), u.ID, "simonhull", "sk-test"); err != nil {
		t.Fatal(err)
	}
	return e
}

// importNavidrome runs "import navidrome -config FILE rest...".
func (e userEnv) importNavidrome(rest ...string) cliResult {
	return e.run(append([]string{importCmd, navidromeCmd, configOption, e.config}, rest...), "")
}

// imported is what simon has, and what waits for Last.fm.
type imported struct {
	store.Listening

	queued int
}

// listening counts simon's plays, hearts and ratings, and the Last.fm queue.
func (e userEnv) listening(t *testing.T) imported {
	t.Helper()
	st := e.openStore(t)
	u, err := st.UserByName(t.Context(), simonName)
	if err != nil {
		t.Fatal(err)
	}
	l, err := st.CountListening(t.Context(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	due, err := st.LastfmDue(t.Context(), u.ID, time.Now().Add(time.Hour), 100, library.TrackMBIDKeys(),
		store.OutboxScrobble, store.OutboxLove, store.OutboxUnlove)
	if err != nil {
		t.Fatal(err)
	}
	return imported{Listening: l, queued: len(due)}
}

// checkNoSecrets fails if r printed a sentinel secret or the Last.fm session key.
func checkNoSecrets(t *testing.T, r cliResult) {
	t.Helper()
	for _, s := range append(navidromedbtest.Secrets(), "sk-test") {
		if strings.Contains(r.stdout+r.stderr, s) {
			t.Errorf("printed the secret %q", s)
		}
	}
}

func TestImportNavidrome(t *testing.T) { //nolint:paralleltest // overrides authParams.
	e := newImportEnv(t)
	src := navidromedbtest.New(t, navidromeFixture...)

	dry := e.importNavidrome(dryRun, "-v", "-map", mapMoved, src)
	if dry.code != exitOK || !strings.Contains(dry.stdout, "Dry run: nothing was written.") {
		t.Fatalf("dry run: %+v", dry)
	}
	if got := e.listening(t); got != (imported{}) {
		t.Fatalf("after a dry run: %+v; want nothing", got)
	}

	first := e.importNavidrome("-map", mapMoved, src)
	second := e.importNavidrome("-map", mapMoved, src)

	for i, r := range []cliResult{dry, first, second} {
		if r.code != exitOK {
			t.Fatalf("run %d: %+v", i, r)
		}
		checkNoSecrets(t, r)
	}
	// 3 plays of one.flac (1 scrobble, 2 before it) and 2 of two.flac; one
	// song heart, one album heart; two ratings.
	want := imported{Plays: 5, Hearts: 2, Rated: 2}
	if got := e.listening(t); got != want {
		t.Errorf("after two runs: %+v; want %+v, none queued for Last.fm", got, want)
	}
	for _, line := range []string{
		"Navidrome user Simon → Kitsune user simon\n",
		"  plays                 5        5        5        0          0      5              0     -\n",
		"Navidrome users with no Kitsune account: Guest",
		afterImport,
	} {
		if !strings.Contains(first.stdout, line) {
			t.Errorf("first run's report lacks %q:\n%s", line, first.stdout)
		}
	}
	if !strings.Contains(second.stdout, "  plays                 5        5        5        0          0      0              5     -\n") {
		t.Errorf("second run's report says it added plays:\n%s", second.stdout)
	}
}

func TestImportNavidromeRefuses(t *testing.T) { //nolint:paralleltest // overrides authParams.
	tests := []struct {
		name       string
		args       func(src string) []string
		setup      func(*testing.T, userEnv)
		source     []string
		wantCode   int
		wantStderr string
	}{
		{
			name: "fewer than half by path", wantCode: exitFailure, wantStderr: "fewer than half",
			source: append(slices.Clone(navidromeFixture), "UPDATE library SET path = '/elsewhere'"),
			args:   func(src string) []string { return []string{src} },
		},
		{
			name: "migrations pending", wantCode: exitFailure, wantStderr: store.ErrMigrationsPending.Error(),
			setup: func(t *testing.T, e userEnv) { t.Helper(); markUnmigrated(t, filepath.Join(e.dataDir, store.FileName)) },
			args:  func(src string) []string { return []string{dryRun, src} },
		},
		{
			name: "not a Navidrome database", wantCode: exitFailure, wantStderr: "no table annotation",
			source: []string{"DROP TABLE annotation"},
			args:   func(src string) []string { return []string{dryRun, src} },
		},
		{
			name: "a -map onto nothing", wantCode: exitFailure, wantStderr: "no Kitsune track has that path",
			args: func(src string) []string { return []string{dryRun, "-map", "A/one.flac=A/nope.flac", src} },
		},
		{
			name: "a -map without =", wantCode: exitUsage, wantStderr: "want FROM=TO",
			args: func(src string) []string { return []string{"-map", "A/one.flac", src} },
		},
		{name: "no FILE", wantCode: exitUsage, wantStderr: "exactly one FILE", args: func(string) []string { return nil }},
	}
	for _, tt := range tests { //nolint:paralleltest // overrides authParams.
		t.Run(tt.name, func(t *testing.T) {
			e := newImportEnv(t)
			if tt.setup != nil {
				tt.setup(t, e)
			}
			source := tt.source
			if source == nil {
				source = navidromeFixture
			}
			src := navidromedbtest.New(t, source...)
			r := e.importNavidrome(tt.args(src)...)
			if r.code != tt.wantCode || !strings.Contains(r.stderr, tt.wantStderr) {
				t.Errorf("code %d, stderr %q; want %d with %q", r.code, r.stderr, tt.wantCode, tt.wantStderr)
			}
			if tt.wantCode == exitUsage && !strings.Contains(r.stderr, usageHeader) {
				t.Errorf("stderr = %q, want the usage text", r.stderr)
			}
			if tt.name == "migrations pending" {
				return // Nothing can open it to count.
			}
			if got := e.listening(t); got != (imported{}) {
				t.Errorf("wrote %+v", got)
			}
		})
	}
}

func TestImportNeedsASource(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{importCmd}, {importCmd, "beets"}} {
		var out, errOut strings.Builder
		if code := cli(args, noEnv, strings.NewReader(""), &out, &errOut); code != exitUsage {
			t.Errorf("%v: code %d, stderr %q", args, code, errOut.String())
		}
	}
}
