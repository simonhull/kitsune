-- A person's marks: hearts on tracks, albums and artists, and stars on tracks.
-- Each is keyed by who and what, and goes with either, like plays.

CREATE TABLE track_hearts (
	user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	track_id   INTEGER NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
	hearted_at INTEGER NOT NULL, -- the first heart: hearting again keeps it
	PRIMARY KEY (user_id, track_id)
) STRICT, WITHOUT ROWID;

CREATE INDEX track_hearts_track ON track_hearts (track_id); -- deleting a track scans hearts without it

CREATE TABLE album_hearts (
	user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	album_id   INTEGER NOT NULL REFERENCES albums (id) ON DELETE CASCADE,
	hearted_at INTEGER NOT NULL,
	PRIMARY KEY (user_id, album_id)
) STRICT, WITHOUT ROWID;

CREATE INDEX album_hearts_album ON album_hearts (album_id);

CREATE TABLE artist_hearts (
	user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	artist_id  INTEGER NOT NULL REFERENCES artists (id) ON DELETE CASCADE,
	hearted_at INTEGER NOT NULL,
	PRIMARY KEY (user_id, artist_id)
) STRICT, WITHOUT ROWID;

CREATE INDEX artist_hearts_artist ON artist_hearts (artist_id);

CREATE TABLE track_ratings (
	user_id  INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	track_id INTEGER NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
	stars    INTEGER NOT NULL CHECK (stars BETWEEN 1 AND 5),
	rated_at INTEGER NOT NULL, -- the latest rating
	PRIMARY KEY (user_id, track_id)
) STRICT, WITHOUT ROWID;

CREATE INDEX track_ratings_track ON track_ratings (track_id);
