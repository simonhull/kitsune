package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/internal/auth"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	simonName     = "simon"
	goodPassword  = "correct horse"
	usersCmd      = "users"
	addCmd        = "add"
	passwordStdin = "-password-stdin"
)

func cheapParams() auth.Params { return auth.Params{Time: 1, MemoryKiB: 64, Threads: 1} }

// useCheapParams makes users add hash cheaply for the rest of the test. The
// tests that call it must not be parallel: authParams is process-wide.
func useCheapParams(t *testing.T) {
	t.Helper()
	old := authParams
	authParams = cheapParams
	t.Cleanup(func() { authParams = old })
}

// userEnv is a config file whose data_dir is a directory that may not exist yet.
type userEnv struct {
	config  string
	dataDir string
}

func newUserEnv(t *testing.T) userEnv {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), "data")
	return userEnv{
		config:  writeFile(t, "server.toml", fmt.Sprintf("[storage]\ndata_dir = %q\n", dataDir)),
		dataDir: dataDir,
	}
}

// cliResult is what one cli invocation produced.
type cliResult struct {
	code           int
	stdout, stderr string
}

func (e userEnv) run(args []string, stdin string) cliResult {
	var out, errOut bytes.Buffer
	code := cli(args, noEnv, strings.NewReader(stdin), &out, &errOut)
	return cliResult{code: code, stdout: out.String(), stderr: errOut.String()}
}

// usersAdd runs "users add -config FILE rest...".
func (e userEnv) usersAdd(stdin string, rest ...string) cliResult {
	return e.run(append([]string{usersCmd, addCmd, "-config", e.config}, rest...), stdin)
}

func (e userEnv) openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(e.dataDir, store.FileName), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestUsersAddAdmin(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	e := newUserEnv(t)
	if r := e.usersAdd(goodPassword+"\n", "-admin", passwordStdin, simonName); r.code != exitOK || r.stdout != "created admin \"simon\"\n" {
		t.Fatalf("result = %+v", r)
	}
	st := e.openStore(t)
	u, err := st.UserByName(t.Context(), simonName)
	if err != nil {
		t.Fatal(err)
	}
	if u.Role != store.RoleAdmin {
		t.Errorf("role = %q, want admin", u.Role)
	}
	svc, err := auth.New(st, auth.Options{Params: cheapParams()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Login(t.Context(), simonName, goodPassword, ""); err != nil {
		t.Errorf("login with the password read from stdin: %v", err)
	}
}

func TestUsersAddUser(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	e := newUserEnv(t)
	if r := e.usersAdd("pw-with-crlf-1\r\n", passwordStdin, "kim"); r.code != exitOK || r.stdout != "created user \"kim\"\n" {
		t.Fatalf("result = %+v", r)
	}
	st := e.openStore(t)
	u, err := st.UserByName(t.Context(), "kim")
	if err != nil {
		t.Fatal(err)
	}
	if u.Role != store.RoleUser {
		t.Errorf("role = %q, want user", u.Role)
	}
	svc, err := auth.New(st, auth.Options{Params: cheapParams()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Login(t.Context(), "kim", "pw-with-crlf-1", ""); err != nil {
		t.Errorf("login with the CRLF-terminated password: %v", err)
	}
}

func TestUsersAddCreatesDataDir(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	e := newUserEnv(t)
	if r := e.usersAdd(goodPassword, passwordStdin, "kim"); r.code != exitOK {
		t.Fatalf("result = %+v", r)
	}
	fi, err := os.Stat(e.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Errorf("data_dir mode = %o, want 700", perm)
	}
}

// markUnmigrated takes the database back to the baseline, with the hash from
// before migrations, so it has migrations pending.
func markUnmigrated(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	const back = "DROP TABLE track_loudness; DROP TABLE album_loudness; DROP INDEX tracks_ungained; DROP TABLE shuffle_sessions; DROP TABLE listens; DROP TABLE shuffle_run_tracks; DROP TABLE shuffle_runs; DROP TABLE shuffle_excluded; DROP TABLE settings; DROP TABLE user_prefs; DROP TABLE lastfm_outbox; DROP TABLE lastfm_accounts; DROP TABLE plays; DROP TABLE track_hearts; DROP TABLE album_hearts; DROP TABLE artist_hearts; DROP TABLE track_ratings; " +
		"ALTER TABLE scan_issues DROP COLUMN reader; " +
		"PRAGMA user_version = 852615662"
	if _, err = db.ExecContext(t.Context(), back); err != nil {
		t.Fatal(err)
	}
}

func TestUsersAddRefusesPendingMigrations(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	e := newUserEnv(t)
	if r := e.usersAdd(goodPassword, passwordStdin, "kim"); r.code != exitOK {
		t.Fatalf("result = %+v", r)
	}
	path := filepath.Join(e.dataDir, store.FileName)
	markUnmigrated(t, path)

	r := e.usersAdd(goodPassword, passwordStdin, "lee")

	if r.code != exitFailure || !strings.Contains(r.stderr, store.ErrMigrationsPending.Error()) {
		t.Fatalf("result = %+v", r)
	}
	st, err := store.Open(t.Context(), path, store.Options{Migrate: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if n, countErr := st.CountUsers(t.Context()); countErr != nil || n != 1 {
		t.Errorf("CountUsers = %d, %v, want 1", n, countErr)
	}
}

// errorCase is one row of TestUsersAddErrors.
type errorCase struct {
	name          string
	args          []string // After "users add -config FILE".
	rawArgs       []string // Replaces the whole command line when set.
	stdin         string
	setup         func(*testing.T, userEnv)
	wantCode      int
	wantStderr    string
	wantNoDataDir bool
}

func usersErrorCases() []errorCase {
	return []errorCase{
		{name: "no name", args: []string{passwordStdin}, stdin: goodPassword, wantCode: exitUsage, wantStderr: "exactly one"},
		{name: "two names", args: []string{passwordStdin, "a", "b"}, stdin: goodPassword, wantCode: exitUsage, wantStderr: "exactly one"},
		{name: "stdin is not a terminal", args: []string{simonName}, stdin: goodPassword, wantCode: exitUsage, wantStderr: "pass -password-stdin", wantNoDataDir: true},
		{name: "empty password", args: []string{passwordStdin, simonName}, stdin: "\n", wantCode: exitFailure, wantStderr: "too short", wantNoDataDir: true},
		{name: "bad name", args: []string{passwordStdin, " simon"}, stdin: goodPassword, wantCode: exitFailure, wantStderr: "invalid user name", wantNoDataDir: true},
		{
			name: "duplicate name", args: []string{passwordStdin, simonName}, stdin: goodPassword,
			wantCode: exitFailure, wantStderr: "already taken",
			setup: func(t *testing.T, e userEnv) {
				t.Helper()
				if r := e.usersAdd(goodPassword, passwordStdin, simonName); r.code != exitOK {
					t.Fatalf("setup: %+v", r)
				}
			},
		},
		{name: "bare users", rawArgs: []string{usersCmd}, wantCode: exitUsage, wantStderr: "needs a subcommand"},
		{name: "unknown subcommand", rawArgs: []string{usersCmd, "frob"}, wantCode: exitUsage, wantStderr: "unknown users command"},
	}
}

func TestUsersAddErrors(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	for _, tt := range usersErrorCases() { //nolint:paralleltest // overrides authParams.
		t.Run(tt.name, tt.check)
	}
}

// invoke runs the case's command line.
func (tt *errorCase) invoke(e userEnv) cliResult {
	if tt.rawArgs != nil {
		return e.run(tt.rawArgs, tt.stdin)
	}
	return e.usersAdd(tt.stdin, tt.args...)
}

// check runs the case and verifies its exit code, message and silence about the password.
func (tt *errorCase) check(t *testing.T) {
	e := newUserEnv(t)
	if tt.setup != nil {
		tt.setup(t, e)
	}
	r := tt.invoke(e)
	if r.code != tt.wantCode || !strings.Contains(r.stderr, tt.wantStderr) {
		t.Errorf("code = %d, stderr = %q; want %d containing %q", r.code, r.stderr, tt.wantCode, tt.wantStderr)
	}
	if tt.wantCode == exitUsage && !strings.Contains(r.stderr, usageHeader) {
		t.Errorf("stderr = %q, want the usage text", r.stderr)
	}
	if strings.Contains(r.stdout, goodPassword) || strings.Contains(r.stderr, goodPassword) {
		t.Error("the password was echoed")
	}
	if _, err := os.Stat(e.dataDir); tt.wantNoDataDir && err == nil {
		t.Error("data_dir was created before validation passed")
	}
}
