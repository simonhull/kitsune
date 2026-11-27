-- Choose artwork (plan 072, Phase 4): an administrator's pins, uploaded images,
-- the pictures the picker previews, and Not this artist. Nothing here is a
-- value derived in Go: each table starts empty, on a new database and an old.
--
-- artist_pins holds an administrator's choice for one artist and one kind: an
-- image a source offered, the artist folder's own, or one uploaded. It is its
-- own table rather than a 'pinned' row of artist_images, which 0026 allowed and
-- nothing ever wrote: a pin lies over what the folders and the sources give,
-- and the scan and the pass go on keeping that beneath it, so unpinning shows
-- it again at once, with no scan and no fetch. No scan or pass writes here.
-- source is where the pinned image came from, remote_id the source's own ID for
-- it (fanart.tv's number, a Commons file name), credit what the source asks to
-- be said of it. pinned_by is NULL once that account is gone.
--
-- artist_holds is Not this artist: the MusicBrainz ID an administrator said
-- named someone else. While an artist's mbid is the one held, no source is
-- asked about it; a new mbid (a match, a corrected tag) ends the hold with no
-- write, since the IDs no longer agree. Keyed by the artist's row, and gone with
-- it.
--
-- artwork_previews names the previews the picker shows: an image a source
-- offers, rendered at most 400 px into the artwork cache by the hash of the
-- preview's own bytes, with no artwork row, since nothing shows it once the
-- picker closes. Keyed by the source and its ID for the image, so a second look
-- within a day fetches nothing; older ones are swept, files and rows.
--
-- artwork is rebuilt, in AGENTS.md's order (create new, copy with every column
-- named, drop old, rename new), for its source CHECK alone, which gains
-- 'upload': an image an administrator sent. It is WITHOUT ROWID, keyed by hash,
-- so nothing that hangs off it moves.

CREATE TABLE artwork_new (
	hash   BLOB    PRIMARY KEY CHECK (length(hash) = 32), -- SHA-256 of the source image; names the files
	width  INTEGER NOT NULL,                              -- of the source
	height INTEGER NOT NULL,
	accent TEXT    NOT NULL,                              -- #rrggbb
	ground TEXT    NOT NULL,                              -- #rrggbb
	source TEXT    NOT NULL CHECK (source IN ('embedded', 'folder', 'remote', 'upload')),
	alpha  INTEGER NOT NULL CHECK (alpha IN (0, 1))       -- 1: the renditions are PNG, with transparency
) STRICT, WITHOUT ROWID;

INSERT INTO artwork_new (hash, width, height, accent, ground, source, alpha)
SELECT hash, width, height, accent, ground, source, alpha
FROM artwork;

DROP TABLE artwork;

ALTER TABLE artwork_new RENAME TO artwork;

CREATE TABLE artist_pins (
	artist_id    INTEGER NOT NULL REFERENCES artists (id) ON DELETE CASCADE,
	kind         TEXT    NOT NULL CHECK (kind IN ('thumb', 'background', 'logo', 'banner')),
	artwork_hash BLOB    NOT NULL REFERENCES artwork (hash),
	source       TEXT    NOT NULL CHECK (source IN ('folder', 'fanart', 'wikimedia', 'upload')),
	remote_id    TEXT,                                                -- the source's own ID for the image
	credit       TEXT    CHECK (credit IS NULL OR json_valid(credit)),
	pinned_by    INTEGER REFERENCES users (id) ON DELETE SET NULL,
	pinned_at    INTEGER NOT NULL,
	PRIMARY KEY (artist_id, kind),
	CHECK ((source IN ('fanart', 'wikimedia')) = (remote_id IS NOT NULL))
) STRICT, WITHOUT ROWID;

CREATE INDEX artist_pins_artwork ON artist_pins (artwork_hash);

CREATE TABLE artist_holds (
	artist_id INTEGER PRIMARY KEY REFERENCES artists (id) ON DELETE CASCADE,
	mbid      TEXT    NOT NULL CHECK (length(mbid) = 36),
	held_by   INTEGER REFERENCES users (id) ON DELETE SET NULL,
	held_at   INTEGER NOT NULL
) STRICT;

CREATE TABLE artwork_previews (
	source    TEXT    NOT NULL CHECK (source IN ('fanart', 'wikimedia')),
	remote_id TEXT    NOT NULL,
	hash      BLOB    NOT NULL CHECK (length(hash) = 32),
	width     INTEGER NOT NULL,
	height    INTEGER NOT NULL,
	accent    TEXT    NOT NULL,
	ground    TEXT    NOT NULL,
	alpha     INTEGER NOT NULL CHECK (alpha IN (0, 1)),
	made_at   INTEGER NOT NULL,
	PRIMARY KEY (source, remote_id)
) STRICT, WITHOUT ROWID;

CREATE INDEX artwork_previews_made ON artwork_previews (made_at);
CREATE INDEX artwork_previews_hash ON artwork_previews (hash);
