-- Artist art, bios and album extras (plan 072). Phase 1 reads them from the
-- library's own files; later phases fetch them by MusicBrainz ID. Everything
-- lives in Kitsune's cache: nothing here is ever written to a file.
--
-- artwork is rebuilt, in AGENTS.md's order (create new, copy with every column
-- named, drop old, rename new), for two changes: source gains 'remote', for an
-- image a later phase downloads, and alpha says the renditions keep their
-- transparency as PNG (logos and disc art) instead of being flattened onto the
-- ground as JPEG. Every image so far is a cover, flattened, so alpha is copied
-- as 0. hash stays the key, so albums.artwork_hash and its index are untouched.
--
-- artist_images holds the image chosen for each artist and kind, one row each;
-- album_images the extras chosen for each album, kind and disc (0 is every
-- disc, since a key column can't be NULL). Where an image came from lives on
-- the row that uses it, never on the image: a folder image says which file it
-- was read from (root, path under it, size and mtime), so an unchanged file is
-- a stat; any other source names none. A pinned row is an administrator's
-- choice, which no scan or fetch replaces; nothing writes one yet.
--
-- artist_bios holds the bio chosen for each artist, as plain text, with where
-- it came from and its licence, and, for one read from artist.nfo, its file.
--
-- albums.release_group_mbid is the release group's MusicBrainz ID, which a scan
-- reads from MUSICBRAINZ_RELEASEGROUPID from now on, kept as mbid is. Existing
-- albums get it at their next full scan, or their next edit; it is not
-- backfilled.
--
-- The fetch state of later phases (scrape_state) is not part of this file:
-- reading local files needs none.

CREATE TABLE artwork_new (
	hash   BLOB    PRIMARY KEY CHECK (length(hash) = 32), -- SHA-256 of the source image; names the files
	width  INTEGER NOT NULL,                              -- of the source
	height INTEGER NOT NULL,
	accent TEXT    NOT NULL,                              -- #rrggbb
	ground TEXT    NOT NULL,                              -- #rrggbb
	source TEXT    NOT NULL CHECK (source IN ('embedded', 'folder', 'remote')),
	alpha  INTEGER NOT NULL CHECK (alpha IN (0, 1))       -- 1: the renditions are PNG, with transparency
) STRICT, WITHOUT ROWID;

INSERT INTO artwork_new (hash, width, height, accent, ground, source, alpha)
SELECT hash, width, height, accent, ground, source, 0
FROM artwork;

DROP TABLE artwork;

ALTER TABLE artwork_new RENAME TO artwork;

CREATE TABLE artist_images (
	artist_id    INTEGER NOT NULL REFERENCES artists (id) ON DELETE CASCADE,
	kind         TEXT    NOT NULL CHECK (kind IN ('thumb', 'background', 'logo', 'banner')),
	artwork_hash BLOB    NOT NULL REFERENCES artwork (hash),
	source       TEXT    NOT NULL CHECK (source IN ('folder', 'fanart', 'audiodb', 'wikimedia', 'pinned')),
	remote_id    TEXT,                                             -- the source's own ID for the image
	credit       TEXT    CHECK (credit IS NULL OR json_valid(credit)), -- author, licence, licence URL, page URL
	file_root    TEXT,                                             -- a folder image's file: its root,
	file_path    TEXT,                                             -- its path under the root,
	file_size    INTEGER,                                          -- and the size and mtime read
	file_mtime   INTEGER,
	pinned_by    INTEGER REFERENCES users (id) ON DELETE SET NULL,
	found_at     INTEGER NOT NULL,
	PRIMARY KEY (artist_id, kind),
	CHECK ((source = 'folder') = (file_path IS NOT NULL)),
	CHECK ((file_path IS NULL) = (file_root IS NULL AND file_size IS NULL AND file_mtime IS NULL))
) STRICT, WITHOUT ROWID;

CREATE INDEX artist_images_artwork ON artist_images (artwork_hash);

CREATE TABLE album_images (
	album_id     INTEGER NOT NULL REFERENCES albums (id) ON DELETE CASCADE,
	kind         TEXT    NOT NULL CHECK (kind IN ('disc', 'back', 'cover')),
	disc         INTEGER NOT NULL CHECK (disc >= 0),               -- 0: every disc
	artwork_hash BLOB    NOT NULL REFERENCES artwork (hash),
	source       TEXT    NOT NULL CHECK (source IN ('folder', 'fanart', 'audiodb', 'coverartarchive')),
	credit       TEXT    CHECK (credit IS NULL OR json_valid(credit)),
	file_root    TEXT,
	file_path    TEXT,
	file_size    INTEGER,
	file_mtime   INTEGER,
	found_at     INTEGER NOT NULL,
	PRIMARY KEY (album_id, kind, disc),
	CHECK (kind = 'disc' OR disc = 0),
	CHECK ((source = 'folder') = (file_path IS NOT NULL)),
	CHECK ((file_path IS NULL) = (file_root IS NULL AND file_size IS NULL AND file_mtime IS NULL))
) STRICT, WITHOUT ROWID;

CREATE INDEX album_images_artwork ON album_images (artwork_hash);

CREATE TABLE artist_bios (
	artist_id   INTEGER PRIMARY KEY REFERENCES artists (id) ON DELETE CASCADE,
	source      TEXT    NOT NULL CHECK (source IN ('nfo', 'wikipedia', 'lastfm', 'audiodb')),
	lang        TEXT,                -- a two-letter code, when the source says
	summary     TEXT    NOT NULL,    -- plain text
	body        TEXT    NOT NULL,    -- plain text, paragraphs split by blank lines
	url         TEXT,                -- the page it came from
	license     TEXT,                -- e.g. CC BY-SA 4.0
	license_url TEXT,
	file_root   TEXT,                -- an artist.nfo's file, as artist_images has it
	file_path   TEXT,
	file_size   INTEGER,
	file_mtime  INTEGER,
	fetched_at  INTEGER NOT NULL,
	CHECK ((source = 'nfo') = (file_path IS NOT NULL)),
	CHECK ((file_path IS NULL) = (file_root IS NULL AND file_size IS NULL AND file_mtime IS NULL))
) STRICT;

ALTER TABLE albums ADD COLUMN release_group_mbid TEXT;
