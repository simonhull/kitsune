-- The album pass and Last.fm bios (plan 072, Phase 3). Nothing here is a value
-- derived in Go: the pass fills what it adds as it goes, so a new database and
-- an old one both start with none of it.
--
-- scrape_state is rebuilt, in AGENTS.md's order (create new, copy with every
-- column named, drop old, rename new), for its source CHECK alone, which gains
-- 'lastfm' (artists' bios), 'coverartarchive' (an album's front and back covers,
-- by its release MBID) and 'musicbrainz' (an album's release group, looked up
-- from its release MBID when the files don't say). It is WITHOUT ROWID: no row
-- ID hangs anything off it. Its index is recreated.
--
-- album_images gains remote_id, the source's own ID for an image (the Cover Art
-- Archive's image ID, fanart.tv's), as artist_images has it, so a refresh that
-- finds the same image downloads nothing.
--
-- albums_cover_from_files keeps a Cover Art Archive cover only while it is the
-- album's: a front the pass fetched is both an album_images row (kind 'cover',
-- which says where it came from) and albums.artwork_hash (which every list,
-- the index and the album page read). The moment anything else sets the
-- album's cover (a scan that finds one in the files, an edit, a merge), the row
-- goes, so the files win at once and the credit goes with it.

CREATE TABLE scrape_state_new (
	source     TEXT    NOT NULL CHECK (source IN ('fanarttv', 'wikimedia', 'lastfm', 'coverartarchive', 'musicbrainz')),
	kind       TEXT    NOT NULL CHECK (kind IN ('artist', 'album')),
	mbid       TEXT    NOT NULL CHECK (length(mbid) = 36),
	status     TEXT    NOT NULL CHECK (status IN ('found', 'none', 'failed')),
	fetched_at INTEGER NOT NULL,
	next_at    INTEGER NOT NULL,
	failures   INTEGER NOT NULL CHECK (failures >= 0),
	error      TEXT,
	PRIMARY KEY (source, kind, mbid),
	CHECK ((status = 'failed') = (failures > 0))
) STRICT, WITHOUT ROWID;

INSERT INTO scrape_state_new (source, kind, mbid, status, fetched_at, next_at, failures, error)
SELECT source, kind, mbid, status, fetched_at, next_at, failures, error
FROM scrape_state;

DROP TABLE scrape_state;

ALTER TABLE scrape_state_new RENAME TO scrape_state;

CREATE INDEX scrape_state_mbid ON scrape_state (kind, mbid);

ALTER TABLE album_images ADD COLUMN remote_id TEXT;

CREATE TRIGGER albums_cover_from_files AFTER UPDATE OF artwork_hash ON albums
WHEN new.artwork_hash IS NOT old.artwork_hash
BEGIN
	DELETE FROM album_images
	WHERE album_id = new.id AND kind = 'cover' AND artwork_hash IS NOT new.artwork_hash;
END;
