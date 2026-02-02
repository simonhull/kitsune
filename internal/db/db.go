package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite database for the music library cache.
type DB struct {
	Conn   *sql.DB
	logger *slog.Logger
}

// Open opens or creates the library database with WAL mode enabled.
func Open(logger *slog.Logger) (*DB, error) {
	if logger == nil {
		logger = slog.Default()
	}

	dataDir := DataDir()
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "library.db")
	conn, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	db := &DB{
		Conn:   conn,
		logger: logger.With("component", "db"),
	}

	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	db.logger.Debug("database opened", "path", dbPath)
	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.Conn.Close()
}

// DataDir returns the Kitsune data directory, respecting XDG.
func DataDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "kitsune")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "kitsune")
}

// TrackCount returns the total number of tracks in the library.
func (db *DB) TrackCount() int {
	var count int
	db.Conn.QueryRow("SELECT COUNT(*) FROM tracks").Scan(&count)
	return count
}

// ArtistCount returns the total number of artists in the library.
func (db *DB) ArtistCount() int {
	var count int
	db.Conn.QueryRow("SELECT COUNT(*) FROM artists").Scan(&count)
	return count
}

// AlbumCount returns the total number of albums in the library.
func (db *DB) AlbumCount() int {
	var count int
	db.Conn.QueryRow("SELECT COUNT(*) FROM albums").Scan(&count)
	return count
}

const currentVersion = 2

// migrate runs schema migrations using PRAGMA user_version.
func (db *DB) migrate() error {
	var version int
	db.Conn.QueryRow("PRAGMA user_version").Scan(&version)

	if version < currentVersion {
		db.logger.Info("migrating database", "from", version, "to", currentVersion)

		// Drop old v1 schema (local-only tracks table).
		if _, err := db.Conn.Exec(dropV1); err != nil {
			return fmt.Errorf("dropping v1 schema: %w", err)
		}

		// Create v2 schema (Subsonic-first with artists/albums/tracks).
		if _, err := db.Conn.Exec(schemaV2); err != nil {
			return fmt.Errorf("creating v2 schema: %w", err)
		}

		if _, err := db.Conn.Exec(fmt.Sprintf("PRAGMA user_version = %d", currentVersion)); err != nil {
			return fmt.Errorf("setting schema version: %w", err)
		}
	}

	return nil
}

var dropV1 = `
DROP TRIGGER IF EXISTS tracks_fts_insert;
DROP TRIGGER IF EXISTS tracks_fts_delete;
DROP TRIGGER IF EXISTS tracks_fts_update;
DROP TABLE IF EXISTS tracks_fts;
DROP TABLE IF EXISTS tracks;
`

var schemaV2 = `
CREATE TABLE IF NOT EXISTS artists (
	id        TEXT PRIMARY KEY,
	name      TEXT NOT NULL,
	album_count INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS albums (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	artist_id   TEXT NOT NULL,
	artist_name TEXT NOT NULL DEFAULT '',
	year        INTEGER NOT NULL DEFAULT 0,
	song_count  INTEGER NOT NULL DEFAULT 0,
	duration_ms INTEGER NOT NULL DEFAULT 0,
	cover_art   TEXT NOT NULL DEFAULT '',
	FOREIGN KEY (artist_id) REFERENCES artists(id)
);

CREATE INDEX IF NOT EXISTS idx_albums_artist ON albums(artist_id);
CREATE INDEX IF NOT EXISTS idx_albums_year ON albums(year);

CREATE TABLE IF NOT EXISTS tracks (
	id              TEXT PRIMARY KEY,
	title           TEXT NOT NULL,
	artist          TEXT NOT NULL DEFAULT '',
	album           TEXT NOT NULL DEFAULT '',
	album_id        TEXT NOT NULL,
	artist_id       TEXT NOT NULL DEFAULT '',
	track_num       INTEGER NOT NULL DEFAULT 0,
	disc_num        INTEGER NOT NULL DEFAULT 0,
	duration_ms     INTEGER NOT NULL DEFAULT 0,
	genre           TEXT NOT NULL DEFAULT '',
	year            INTEGER NOT NULL DEFAULT 0,
	bitrate         INTEGER NOT NULL DEFAULT 0,
	format          TEXT NOT NULL DEFAULT '',
	cover_art       TEXT NOT NULL DEFAULT '',
	-- kitsune-specific metadata (preserved across syncs)
	shuffle_exclude INTEGER NOT NULL DEFAULT 0,
	linked_next_id  TEXT,
	FOREIGN KEY (album_id) REFERENCES albums(id),
	FOREIGN KEY (linked_next_id) REFERENCES tracks(id)
);

CREATE INDEX IF NOT EXISTS idx_tracks_album ON tracks(album_id);
CREATE INDEX IF NOT EXISTS idx_tracks_artist ON tracks(artist_id);

-- Full-text search across the library.
CREATE VIRTUAL TABLE IF NOT EXISTS tracks_fts USING fts5(
	title, artist, album,
	content='tracks',
	content_rowid='rowid'
);

-- Triggers to keep FTS in sync.
CREATE TRIGGER IF NOT EXISTS tracks_fts_insert AFTER INSERT ON tracks BEGIN
	INSERT INTO tracks_fts(rowid, title, artist, album)
	VALUES (new.rowid, new.title, new.artist, new.album);
END;

CREATE TRIGGER IF NOT EXISTS tracks_fts_delete AFTER DELETE ON tracks BEGIN
	INSERT INTO tracks_fts(tracks_fts, rowid, title, artist, album)
	VALUES ('delete', old.rowid, old.title, old.artist, old.album);
END;

CREATE TRIGGER IF NOT EXISTS tracks_fts_update AFTER UPDATE ON tracks BEGIN
	INSERT INTO tracks_fts(tracks_fts, rowid, title, artist, album)
	VALUES ('delete', old.rowid, old.title, old.artist, old.album);
	INSERT INTO tracks_fts(rowid, title, artist, album)
	VALUES (new.rowid, new.title, new.artist, new.album);
END;
`
