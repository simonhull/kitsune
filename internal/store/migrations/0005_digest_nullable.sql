-- A track may wait for its fingerprint: a scan stores tags first, and a
-- background pass fills audio_digest later. NULL is a track not yet hashed,
-- which no move can match. The rebuild keeps every id, so tags, artist links,
-- plays, marks, the outbox and FTS rows stay on their tracks.

CREATE TABLE tracks_new (
	id            INTEGER PRIMARY KEY,  -- internal; the FTS rowid
	ulid          TEXT    NOT NULL UNIQUE CHECK (length(ulid) = 26),
	root          TEXT    NOT NULL,
	rel_path      TEXT    NOT NULL,
	size          INTEGER NOT NULL,
	mtime         INTEGER NOT NULL,     -- Unix ms
	audio_digest  BLOB    CHECK (audio_digest IS NULL OR length(audio_digest) = 32), -- NULL until the fingerprint pass reads the file
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

INSERT INTO tracks_new (
	id, ulid, root, rel_path, size, mtime, audio_digest, album_id,
	format, codec, duration_ms, sample_rate, bit_depth, channels, bitrate, lossless,
	title, sort_title, track_no, track_total, disc_no, disc_total, year, original_year,
	compilation, rg_track_gain, rg_track_peak, rg_album_gain, rg_album_peak,
	scan_warnings, added_at, missing_since
)
SELECT
	id, ulid, root, rel_path, size, mtime, audio_digest, album_id,
	format, codec, duration_ms, sample_rate, bit_depth, channels, bitrate, lossless,
	title, sort_title, track_no, track_total, disc_no, disc_total, year, original_year,
	compilation, rg_track_gain, rg_track_peak, rg_album_gain, rg_album_peak,
	scan_warnings, added_at, missing_since
FROM tracks
ORDER BY id;

DROP TABLE tracks;
ALTER TABLE tracks_new RENAME TO tracks;

CREATE INDEX tracks_album ON tracks (album_id, disc_no, track_no);
CREATE INDEX tracks_live ON tracks (album_id, duration_ms) WHERE missing_since IS NULL;
CREATE INDEX tracks_missing_digest ON tracks (audio_digest) WHERE missing_since IS NOT NULL;
CREATE INDEX tracks_missing_since ON tracks (missing_since) WHERE missing_since IS NOT NULL;
CREATE INDEX tracks_undigested ON tracks (root, rel_path) WHERE audio_digest IS NULL; -- the fingerprint pass's queue, in path order
