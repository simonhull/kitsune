-- Metadata matching. lookups keeps what an outside source answered, by the
-- request, for a day: MusicBrainz asks for one request a second, and a person
-- going back and forth between releases shouldn't wait twice. source names who
-- answered ('musicbrainz'), and key is the request's path and query.
CREATE TABLE lookups (
	source     TEXT    NOT NULL,
	key        TEXT    NOT NULL,
	body       BLOB    NOT NULL,
	fetched_at INTEGER NOT NULL,
	PRIMARY KEY (source, key)
) STRICT;

CREATE INDEX lookups_fetched ON lookups (fetched_at);

-- What the matching queue knows of an album beyond its MBID: an administrator
-- skipped it for now, or matched it. An album with neither and no MBID is to do.
CREATE TABLE match_states (
	album_id INTEGER PRIMARY KEY REFERENCES albums (id) ON DELETE CASCADE,
	state    TEXT    NOT NULL CHECK (state IN ('skipped', 'matched')),
	at       INTEGER NOT NULL
) STRICT;
