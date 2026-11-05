CREATE TABLE users (
	id            INTEGER PRIMARY KEY,
	name          TEXT    NOT NULL UNIQUE COLLATE NOCASE,
	password_hash TEXT    NOT NULL,
	role          TEXT    NOT NULL CHECK (role IN ('admin', 'user')),
	created_at    INTEGER NOT NULL
) STRICT;

CREATE TABLE sessions (
	token_hash BLOB    PRIMARY KEY CHECK (length(token_hash) = 32),
	user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL,
	last_seen  INTEGER NOT NULL,
	expires_at INTEGER NOT NULL,
	user_agent TEXT    NOT NULL
) STRICT;

CREATE INDEX sessions_user ON sessions (user_id);
CREATE INDEX sessions_expires ON sessions (expires_at);

CREATE TABLE artists (
	id        INTEGER PRIMARY KEY,
	ulid      TEXT    NOT NULL UNIQUE CHECK (length(ulid) = 26), -- public ID; kept across updates
	match_key TEXT    NOT NULL UNIQUE,  -- caller-normalized name; the identity
	name      TEXT    NOT NULL,
	sort_name TEXT    NOT NULL,
	mbid      TEXT                      -- NULL when unknown
) STRICT;

CREATE TABLE artwork (
	hash   BLOB    PRIMARY KEY CHECK (length(hash) = 32), -- SHA-256 of the source image; names the files
	width  INTEGER NOT NULL,                              -- of the source
	height INTEGER NOT NULL,
	accent TEXT    NOT NULL,                              -- #rrggbb
	ground TEXT    NOT NULL,                              -- #rrggbb
	source TEXT    NOT NULL CHECK (source IN ('embedded', 'folder'))
) STRICT, WITHOUT ROWID;

CREATE TABLE albums (
	id          INTEGER PRIMARY KEY,
	ulid        TEXT    NOT NULL UNIQUE CHECK (length(ulid) = 26), -- public ID; kept across updates
	group_key   TEXT    NOT NULL UNIQUE, -- caller-computed grouping key; the identity
	name        TEXT    NOT NULL,
	sort_name   TEXT    NOT NULL COLLATE NOCASE, -- the collation must be on the column for keyset seeks
	artist      TEXT    NOT NULL,        -- display name
	mbid        TEXT,
	year        INTEGER,
	compilation INTEGER NOT NULL CHECK (compilation IN (0, 1)),
	artwork_hash BLOB REFERENCES artwork (hash), -- NULL when the album has no cover
	added_at    INTEGER NOT NULL                 -- first sight; kept across updates
) STRICT;

CREATE INDEX albums_artwork ON albums (artwork_hash);
CREATE INDEX albums_sort ON albums (sort_name, id);
CREATE INDEX albums_added ON albums (added_at DESC, id DESC);

CREATE TABLE tracks (
	id            INTEGER PRIMARY KEY,  -- internal; the FTS rowid
	ulid          TEXT    NOT NULL UNIQUE CHECK (length(ulid) = 26),
	root          TEXT    NOT NULL,
	rel_path      TEXT    NOT NULL,
	size          INTEGER NOT NULL,
	mtime         INTEGER NOT NULL,     -- Unix ms
	audio_digest  BLOB    NOT NULL CHECK (length(audio_digest) = 32),
	album_id      INTEGER NOT NULL REFERENCES albums (id),
	format        TEXT    NOT NULL,
	codec         TEXT    NOT NULL,
	duration_ms   INTEGER NOT NULL,
	sample_rate   INTEGER,
	bit_depth     INTEGER,
	channels      INTEGER,
	bitrate       INTEGER,
	lossless      INTEGER NOT NULL CHECK (lossless IN (0, 1)),
	title         TEXT    NOT NULL,
	sort_title    TEXT    NOT NULL,
	track_no      INTEGER,
	track_total   INTEGER,
	disc_no       INTEGER,
	disc_total    INTEGER,
	year          INTEGER,
	original_year INTEGER,
	compilation   INTEGER NOT NULL CHECK (compilation IN (0, 1)),
	rg_track_gain REAL,
	rg_track_peak REAL,
	rg_album_gain REAL,
	rg_album_peak REAL,
	scan_warnings TEXT    NOT NULL,     -- JSON array of strings
	added_at      INTEGER NOT NULL,     -- first sight; kept across updates and moves
	missing_since INTEGER,
	UNIQUE (root, rel_path)
) STRICT;

CREATE INDEX tracks_album ON tracks (album_id, disc_no, track_no);
CREATE INDEX tracks_live ON tracks (album_id, duration_ms) WHERE missing_since IS NULL;
CREATE INDEX tracks_missing_digest ON tracks (audio_digest) WHERE missing_since IS NOT NULL;
CREATE INDEX tracks_missing_since ON tracks (missing_since) WHERE missing_since IS NOT NULL;

CREATE TABLE track_tags (
	track_id INTEGER NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
	key      TEXT    NOT NULL,
	ord      INTEGER NOT NULL,
	value    TEXT    NOT NULL,
	PRIMARY KEY (track_id, key, ord)
) STRICT, WITHOUT ROWID;

CREATE TABLE track_artists (
	track_id  INTEGER NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
	role      TEXT    NOT NULL CHECK (role IN ('artist', 'album_artist')),
	ord       INTEGER NOT NULL,
	artist_id INTEGER NOT NULL REFERENCES artists (id),
	PRIMARY KEY (track_id, role, ord)
) STRICT, WITHOUT ROWID;

CREATE INDEX track_artists_artist ON track_artists (artist_id);

CREATE TABLE scan_issues (
	root     TEXT    NOT NULL,
	rel_path TEXT    NOT NULL,
	kind     TEXT    NOT NULL CHECK (kind IN ('unreadable', 'corrupted', 'unsupported')),
	message  TEXT    NOT NULL,
	size     INTEGER NOT NULL,
	mtime    INTEGER NOT NULL,
	seen_at  INTEGER NOT NULL,
	PRIMARY KEY (root, rel_path)
) STRICT, WITHOUT ROWID;

CREATE VIRTUAL TABLE tracks_fts USING fts5 (
	title, artists, album, album_artist,
	content = '', contentless_delete = 1,
	tokenize = 'unicode61 remove_diacritics 2'
);
