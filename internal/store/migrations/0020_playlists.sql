-- Playlists: each person's own songs in their own order, made by hand or
-- brought over from Navidrome.
--
-- import_key names where an imported playlist came from (navidrome:<its id>),
-- so a re-run of the import finds it and adds nothing; it is NULL for one made
-- here. added_at is when songs were last added, which the Add to playlist menu
-- sorts by, and NULL until then.
--
-- An entry's id is the public entry ID, never reused (AUTOINCREMENT), so a
-- client holding an old one can't take out a newer entry. position is sparse:
-- entries are spaced by a gap, and a move writes one row, at the midpoint of
-- its new neighbours; a playlist is renumbered only when a gap runs out. A
-- song may be in a playlist more than once, each time its own entry.
--
-- Entries go with their track only when the track is truly deleted: a trashed
-- track keeps its row, so its entries stay for Restore to bring back, and live
-- reads leave them out with the track.
--
-- Nothing here is backfilled.
CREATE TABLE playlists (
	id         INTEGER PRIMARY KEY,
	ulid       TEXT    NOT NULL UNIQUE CHECK (length(ulid) = 26),
	user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	name       TEXT    NOT NULL CHECK (length(name) BETWEEN 1 AND 200), -- not unique
	source     TEXT    NOT NULL CHECK (source IN ('kitsune', 'navidrome')),
	import_key TEXT,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	added_at   INTEGER,
	UNIQUE (user_id, import_key)
) STRICT;

CREATE TABLE playlist_tracks (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	playlist_id INTEGER NOT NULL REFERENCES playlists (id) ON DELETE CASCADE,
	position    INTEGER NOT NULL,
	track_id    INTEGER NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
	added_at    INTEGER NOT NULL,
	UNIQUE (playlist_id, position)
) STRICT;

CREATE INDEX playlist_tracks_track ON playlist_tracks (track_id);
