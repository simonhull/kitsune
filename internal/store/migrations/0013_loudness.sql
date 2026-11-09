-- Loudness Kitsune measures itself, for albums with songs whose files carry no
-- ReplayGain. It is Kitsune's own and never written to a file. A track's row
-- holds for the file of its size and mtime, measured by measurement version
-- `version`; outcome says whether it could be measured. An album's row pools
-- the blocks of every track whose album_pool names it, and its id is new each
-- time, so a track pooled into an older measure of the album no longer counts.

CREATE TABLE album_loudness (
	id         INTEGER PRIMARY KEY AUTOINCREMENT, -- never reused: see above
	album_id   INTEGER NOT NULL UNIQUE REFERENCES albums (id) ON DELETE CASCADE,
	tracks     INTEGER NOT NULL, -- live tracks pooled
	integrated REAL,             -- LUFS; NULL when nothing passed the gates
	true_peak  REAL              -- linear, 1 being full scale; NULL with no audio measured
) STRICT;

CREATE TABLE track_loudness (
	track_id    INTEGER PRIMARY KEY REFERENCES tracks (id) ON DELETE CASCADE,
	size        INTEGER NOT NULL,
	mtime       INTEGER NOT NULL, -- Unix ms
	version     INTEGER NOT NULL,
	outcome     TEXT    NOT NULL CHECK (outcome IN ('measured', 'unsupported', 'unreadable')),
	integrated  REAL,    -- LUFS; NULL when not measured, or nothing passed the gates
	true_peak   REAL,    -- linear, as above
	sample_peak REAL,
	blocks      BLOB,    -- the gated blocks' histogram, for pooling an album
	album_pool  INTEGER REFERENCES album_loudness (id) ON DELETE SET NULL
) STRICT;

CREATE INDEX track_loudness_pool ON track_loudness (album_pool);

-- The albums with a song that has no ReplayGain, which are the ones measured.
CREATE INDEX tracks_ungained ON tracks (album_id)
	WHERE missing_since IS NULL AND rg_track_gain IS NULL AND rg_album_gain IS NULL;
