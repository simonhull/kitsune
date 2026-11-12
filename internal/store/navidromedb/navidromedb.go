// Package navidromedb reads a Navidrome database for the importer, read-only.
//
// It opens the file immutable, so nothing is ever written beside it, and it
// selects only the columns in schema.sql: a Navidrome database also holds
// password hashes and Last.fm session keys, and no query here can reach them.
// Errors name tables and columns, never a value.
package navidromedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver" // Registers the "sqlite3" database/sql driver.

	"github.com/simonhull/kitsune/internal/store/navidromedb/ndsql"
)

// Sentinels callers match with [errors.Is].
var (
	// ErrWAL means a non-empty -wal file lies beside the database: it holds
	// writes an immutable open would miss.
	ErrWAL = errors.New("a non-empty -wal file lies beside it; open the database once in Navidrome or sqlite3 to checkpoint it, or copy it with Navidrome stopped")
	// ErrShape means the file lacks a table or column the import reads.
	ErrShape = errors.New("not a Navidrome database this importer reads")
	// ErrTime means a datetime column holds text in no shape Navidrome writes.
	ErrTime = errors.New("unreadable datetime")
)

// Item types in annotation.item_type.
const (
	ItemSong   = "media_file"
	ItemAlbum  = "album"
	ItemArtist = "artist"
)

// scrobblesTable is the one table the import can do without.
const scrobblesTable = "scrobbles"

// required is every table and column the queries read, as in schema.sql.
// TestSchemaMatchesRequired holds the two together.
var required = []struct { //nolint:gochecknoglobals // Read-only table of what Open checks.
	table    string
	columns  []string
	optional bool
}{
	{table: "library", columns: []string{"id", "path"}},
	{table: "media_file", columns: []string{"id", "path", "album_id", "mbz_recording_id", "mbz_release_track_id", "library_id", "missing"}},
	{table: "user", columns: []string{"id", "user_name"}},
	{table: "annotation", columns: []string{"user_id", "item_id", "item_type", "play_count", "play_date", "rating", "starred", "starred_at", "rated_at"}},
	{table: scrobblesTable, columns: []string{"id", "media_file_id", "user_id", "submission_time"}, optional: true},
	{table: "artist", columns: []string{"id", "name", "mbz_artist_id"}},
}

// DB is an open Navidrome database. It is safe for concurrent use.
type DB struct {
	db           *sql.DB
	q            *ndsql.Queries
	hasScrobbles bool
}

// Library is one of Navidrome's music folders.
type Library struct {
	ID   int64
	Path string // Absolute, as Navidrome saw it.
}

// User is a Navidrome account: its ID and name, and nothing else.
type User struct {
	ID   string
	Name string
}

// Song is a Navidrome media_file.
type Song struct {
	ID        string
	LibraryID int64
	Path      string // Relative to its library's path.
	Missing   bool   // Navidrome had lost the file.
	// ReleaseTrackMBID and RecordingMBID are MusicBrainz IDs; empty when untagged.
	ReleaseTrackMBID, RecordingMBID string
	AlbumID                         string
}

// Annotation is one person's marks and plays on one item.
type Annotation struct {
	ItemID    string
	ItemType  string // [ItemSong], [ItemAlbum] or [ItemArtist].
	PlayCount int64
	PlayDate  time.Time // The last play; zero when unknown.
	Rating    int64     // 0 to 5; 0 is none.
	Starred   bool
	StarredAt time.Time // Zero when unknown.
	RatedAt   time.Time // Zero when unknown: Navidrome added it late.
}

// Scrobble is one play Navidrome recorded with its time.
type Scrobble struct {
	ID       int64
	SongID   string
	PlayedAt time.Time
}

// Artist is a Navidrome artist.
type Artist struct {
	ID   string
	Name string
	MBID string // Empty when unknown.
}

// Open opens the Navidrome database at path read-only and immutable, so
// nothing is created beside it, and checks that it has every table and column
// the import reads. It returns an error wrapping [ErrWAL] when a non-empty
// -wal file lies beside it, and [ErrShape] naming what is missing.
func Open(ctx context.Context, path string) (*DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("open navidrome database: %w", err)
	}
	if fi, err := os.Stat(path + "-wal"); err == nil && fi.Size() > 0 {
		return nil, fmt.Errorf("open navidrome database: %w", ErrWAL)
	}
	u := url.URL{Path: filepath.ToSlash(path), RawQuery: "mode=ro&immutable=1"}
	db, err := sql.Open("sqlite3", "file:"+u.EscapedPath()+"?"+u.RawQuery)
	if err != nil {
		return nil, fmt.Errorf("open navidrome database: %w", err)
	}
	d := &DB{db: db, q: ndsql.New(db)}
	if err = d.checkShape(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("open navidrome database: %w", err), db.Close())
	}
	return d, nil
}

// Close closes the database.
func (d *DB) Close() error {
	if err := d.db.Close(); err != nil {
		return fmt.Errorf("close navidrome database: %w", err)
	}
	return nil
}

// Libraries returns every music folder, by ID.
func (d *DB) Libraries(ctx context.Context) ([]Library, error) {
	rows, err := d.q.Libraries(ctx)
	if err != nil {
		return nil, fmt.Errorf("read library: %w", err)
	}
	out := make([]Library, len(rows))
	for i, r := range rows {
		out[i] = Library(r)
	}
	return out, nil
}

// Users returns every account's ID and name, by ID.
func (d *DB) Users(ctx context.Context) ([]User, error) {
	rows, err := d.q.Users(ctx)
	if err != nil {
		return nil, fmt.Errorf("read user: %w", err)
	}
	out := make([]User, len(rows))
	for i, r := range rows {
		out[i] = User{ID: r.ID, Name: r.UserName}
	}
	return out, nil
}

// Songs returns every media_file, missing ones included, by ID.
func (d *DB) Songs(ctx context.Context) ([]Song, error) {
	rows, err := d.q.Songs(ctx)
	if err != nil {
		return nil, fmt.Errorf("read media_file: %w", err)
	}
	out := make([]Song, len(rows))
	for i, r := range rows {
		out[i] = Song{
			ID: r.ID, LibraryID: r.LibraryID, Path: r.Path, Missing: r.Missing,
			ReleaseTrackMBID: r.MbzReleaseTrackID, RecordingMBID: r.MbzRecordingID, AlbumID: r.AlbumID,
		}
	}
	return out, nil
}

// Annotations returns the user's annotations, by item type and ID.
func (d *DB) Annotations(ctx context.Context, userID string) ([]Annotation, error) {
	rows, err := d.q.Annotations(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("read annotation: %w", err)
	}
	out := make([]Annotation, len(rows))
	for i, r := range rows {
		a := Annotation{ItemID: r.ItemID, ItemType: r.ItemType, PlayCount: r.PlayCount, Rating: r.Rating, Starred: r.Starred}
		var errs [3]error
		a.PlayDate, errs[0] = parseTime("annotation.play_date", r.PlayDate)
		a.StarredAt, errs[1] = parseTime("annotation.starred_at", r.StarredAt)
		a.RatedAt, errs[2] = parseTime("annotation.rated_at", r.RatedAt)
		if joined := errors.Join(errs[:]...); joined != nil {
			return nil, joined
		}
		out[i] = a
	}
	return out, nil
}

// Scrobbles returns the user's scrobbles, by ID; none when the database has
// no scrobbles table.
func (d *DB) Scrobbles(ctx context.Context, userID string) ([]Scrobble, error) {
	if !d.hasScrobbles {
		return nil, nil
	}
	rows, err := d.q.Scrobbles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("read scrobbles: %w", err)
	}
	out := make([]Scrobble, len(rows))
	for i, r := range rows {
		out[i] = Scrobble{ID: r.ID, SongID: r.MediaFileID, PlayedAt: time.Unix(r.SubmissionTime, 0).UTC()}
	}
	return out, nil
}

// Artists returns the artists with the given IDs, by ID.
func (d *DB) Artists(ctx context.Context, ids []string) ([]Artist, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := d.q.Artists(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("read artist: %w", err)
	}
	out := make([]Artist, len(rows))
	for i, r := range rows {
		out[i] = Artist{ID: r.ID, Name: r.Name, MBID: r.MbzArtistID}
	}
	return out, nil
}

// checkShape checks the tables and columns in required, noting whether the
// optional scrobbles table is there.
func (d *DB) checkShape(ctx context.Context) error {
	var missing []error
	for _, t := range required {
		have, err := d.columns(ctx, t.table)
		if err != nil {
			return err
		}
		if len(have) == 0 {
			if !t.optional {
				missing = append(missing, fmt.Errorf("%w: no table %s", ErrShape, t.table))
			}
			continue
		}
		for _, c := range t.columns {
			if !have[c] {
				missing = append(missing, fmt.Errorf("%w: table %s has no column %s", ErrShape, t.table, c))
			}
		}
		if t.table == scrobblesTable {
			d.hasScrobbles = true
		}
	}
	return errors.Join(missing...)
}

// columns returns the names of table's columns; none when there is no table.
func (d *DB) columns(ctx context.Context, table string) (map[string]bool, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return nil, fmt.Errorf("read the columns of %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }() // Only read: a close error changes nothing.
	have := map[string]bool{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("read the columns of %s: %w", table, err)
		}
		have[name] = true
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("read the columns of %s: %w", table, err)
	}
	return have, nil
}

// timeLayout is how Navidrome writes a datetime. It has 0 to 9 fraction
// digits, which parsing accepts after the seconds without the layout naming
// them.
const timeLayout = "2006-01-02 15:04:05-07:00"

// parseTime reads a datetime as Navidrome writes it; empty is the zero time.
// The error names column, never the value.
func parseTime(column, s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(timeLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w in %s", ErrTime, column)
	}
	return t.UTC(), nil
}
