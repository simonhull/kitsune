-- Smart playlists: a playlist made of rules, not entries. rules is the rules'
-- JSON (internal/smartlist's form), NULL for a regular playlist; a smart one is
-- worked out each time it is read and never has playlist_tracks rows, which the
-- trigger refuses as the store does. seed orders a smart playlist's random
-- choices and shuffle, so its page reads the same each time; Pick again draws
-- a new one. It is NULL for a regular playlist.
--
-- Nothing here is backfilled: every existing playlist is a regular one.
ALTER TABLE playlists ADD COLUMN rules TEXT CHECK (rules IS NULL OR json_valid(rules));

ALTER TABLE playlists ADD COLUMN seed INTEGER;

CREATE TRIGGER playlist_tracks_not_smart
BEFORE INSERT ON playlist_tracks
WHEN (SELECT rules FROM playlists WHERE id = NEW.playlist_id) IS NOT NULL
BEGIN
	SELECT RAISE(ABORT, 'a smart playlist has no entries');
END;
