// Package navidromedbtest writes small Navidrome databases for tests. No test
// ever reads a real one: the schema here is the part the importer reads, plus
// the columns and table that hold secrets, filled with sentinels a test can
// look for in everything the importer returns.
package navidromedbtest

import (
	"database/sql"
	_ "embed"
	"path/filepath"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver" // Registers the "sqlite3" database/sql driver.
)

// The sentinels New writes where Navidrome keeps secrets. None may appear in
// anything the importer returns, prints or logs.
const (
	SecretPassword = "sentinel-password-hash"
	SecretEmail    = "sentinel@example.invalid"
	SecretProp     = "sentinel-lastfm-session-key"
)

// Secrets lists the sentinels.
func Secrets() []string { return []string{SecretPassword, SecretEmail, SecretProp} }

//go:embed schema.sql
var schema string

// New writes a Navidrome database to a new file under t.TempDir and returns
// its path. After statements run, every user gets the sentinel password and
// email, and a Last.fm session key in user_props. The file is closed, with
// nothing beside it.
func New(tb testing.TB, statements ...string) string {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "navidrome.db")
	all := append([]string{schema}, statements...)
	all = append(all,
		`UPDATE "user" SET password = '`+SecretPassword+`', email = '`+SecretEmail+`'`,
		`INSERT OR REPLACE INTO user_props (user_id, key, value) SELECT id, 'LastFMSessionKey', '`+SecretProp+`' FROM "user"`,
	)
	Write(tb, path, all...)
	return path
}

// Write runs statements on the database at path, creating it, and closes it.
func Write(tb testing.TB, path string, statements ...string) {
	tb.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path))
	if err != nil {
		tb.Fatal(err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			tb.Fatal(closeErr)
		}
	}()
	for _, s := range statements {
		if _, err = db.ExecContext(tb.Context(), s); err != nil {
			tb.Fatalf("%s: %v", s, err)
		}
	}
}
