-- File playlists: a .m3u or .m3u8 file in the library becomes a playlist that
-- everyone on the server sees and nobody owns, so playlists.user_id becomes NULL
-- for one, and source gains 'file'. A CHECK ties the two: a playlist is a file's
-- exactly when it has no owner. That is a rebuild, in AGENTS.md's order: create
-- new, copy with every column named, drop old, rename new. The smart playlists'
-- trigger names playlists, so it goes first and comes back after the rename.
--
-- playlist_files says which file each file playlist is read from: its root and
-- path under it, the size and mtime that were read, when, by which reader
-- version, how many paths it named, how many lay past the read's limits, and
-- missing, the lines that found no song (a JSON array; never an absolute path
-- past its last two segments). It goes with its playlist.
--
-- A file playlist's entries are playlist_tracks rows, in the file's order with
-- its repeats, each with line, the path as the file wrote it, so a read of a
-- changed file keeps a song that moved by the line that named it. Their
-- position is the line's number in the file. line is NULL for every other
-- playlist's entries.
--
-- Nothing here is backfilled: every playlist so far has an owner.
DROP TRIGGER playlist_tracks_not_smart;

CREATE TABLE playlists_new (
	id         INTEGER PRIMARY KEY,
	ulid       TEXT    NOT NULL UNIQUE CHECK (length(ulid) = 26),
	user_id    INTEGER REFERENCES users (id) ON DELETE CASCADE,
	name       TEXT    NOT NULL CHECK (length(name) BETWEEN 1 AND 200), -- not unique
	source     TEXT    NOT NULL CHECK (source IN ('kitsune', 'navidrome', 'file')),
	import_key TEXT,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	added_at   INTEGER,
	rules      TEXT CHECK (rules IS NULL OR json_valid(rules)),
	seed       INTEGER,
	UNIQUE (user_id, import_key),
	CHECK ((source = 'file') = (user_id IS NULL))
) STRICT;

INSERT INTO playlists_new (id, ulid, user_id, name, source, import_key, created_at, updated_at, added_at, rules, seed)
SELECT id, ulid, user_id, name, source, import_key, created_at, updated_at, added_at, rules, seed
FROM playlists
ORDER BY id;

DROP TABLE playlists;

ALTER TABLE playlists_new RENAME TO playlists;

CREATE TRIGGER playlist_tracks_not_smart
BEFORE INSERT ON playlist_tracks
WHEN (SELECT rules FROM playlists WHERE id = NEW.playlist_id) IS NOT NULL
BEGIN
	SELECT RAISE(ABORT, 'a smart playlist has no entries');
END;

ALTER TABLE playlist_tracks ADD COLUMN line TEXT;

CREATE TABLE playlist_files (
	playlist_id INTEGER PRIMARY KEY REFERENCES playlists (id) ON DELETE CASCADE,
	root        TEXT    NOT NULL,
	rel_path    TEXT    NOT NULL,
	size        INTEGER NOT NULL,
	mtime_ms    INTEGER NOT NULL,
	read_at     INTEGER NOT NULL,
	reader      INTEGER NOT NULL,
	lines       INTEGER NOT NULL,
	past_limit  INTEGER NOT NULL,
	missing     TEXT    NOT NULL CHECK (json_valid(missing)),
	UNIQUE (root, rel_path)
) STRICT;
