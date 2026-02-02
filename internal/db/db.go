package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite database for the music library index.
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

// migrate creates tables if they don't exist.
func (db *DB) migrate() error {
	_, err := db.Conn.Exec(schema)
	return err
}

var schema = `
CREATE TABLE IF NOT EXISTS tracks (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	path         TEXT UNIQUE NOT NULL,
	title        TEXT NOT NULL,
	artist       TEXT NOT NULL DEFAULT '',
	album        TEXT NOT NULL DEFAULT '',
	album_artist TEXT NOT NULL DEFAULT '',
	genre        TEXT NOT NULL DEFAULT '',
	year         INTEGER NOT NULL DEFAULT 0,
	track_num    INTEGER NOT NULL DEFAULT 0,
	disc_num     INTEGER NOT NULL DEFAULT 0,
	duration_ms  INTEGER NOT NULL DEFAULT 0,
	format       TEXT NOT NULL DEFAULT '',
	modified_at  INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_tracks_artist ON tracks(artist);
CREATE INDEX IF NOT EXISTS idx_tracks_album ON tracks(album);
CREATE INDEX IF NOT EXISTS idx_tracks_album_artist ON tracks(album_artist);

-- Full-text search across the library.
CREATE VIRTUAL TABLE IF NOT EXISTS tracks_fts USING fts5(
	title, artist, album, album_artist,
	content='tracks',
	content_rowid='id'
);

-- Triggers to keep FTS in sync.
CREATE TRIGGER IF NOT EXISTS tracks_fts_insert AFTER INSERT ON tracks BEGIN
	INSERT INTO tracks_fts(rowid, title, artist, album, album_artist)
	VALUES (new.id, new.title, new.artist, new.album, new.album_artist);
END;

CREATE TRIGGER IF NOT EXISTS tracks_fts_delete AFTER DELETE ON tracks BEGIN
	INSERT INTO tracks_fts(tracks_fts, rowid, title, artist, album, album_artist)
	VALUES ('delete', old.id, old.title, old.artist, old.album, old.album_artist);
END;

CREATE TRIGGER IF NOT EXISTS tracks_fts_update AFTER UPDATE ON tracks BEGIN
	INSERT INTO tracks_fts(tracks_fts, rowid, title, artist, album, album_artist)
	VALUES ('delete', old.id, old.title, old.artist, old.album, old.album_artist);
	INSERT INTO tracks_fts(rowid, title, artist, album, album_artist)
	VALUES (new.id, new.title, new.artist, new.album, new.album_artist);
END;
`
