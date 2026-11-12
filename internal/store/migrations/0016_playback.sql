-- Where each person left off: what was playing, and how far in. One row per
-- person, replaced on every save. queue is JSON internal/playback has checked:
-- {"tracks":[ulid...],"later":[ulid...],"continues":"ulid"}, at most 1,000
-- track ULIDs in all. Nothing here is backfilled.
CREATE TABLE playback (
	user_id       INTEGER PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
	kind          TEXT    NOT NULL CHECK (kind IN ('album', 'songs', 'list', 'shuffle')),
	queue         TEXT    NOT NULL,
	at            INTEGER NOT NULL CHECK (at >= 0), -- the current song's index in tracks
	position_ms   INTEGER NOT NULL CHECK (position_ms >= 0),
	ordered       INTEGER NOT NULL CHECK (ordered IN (0, 1)),
	shuffle_id    TEXT    CHECK (shuffle_id IS NULL OR length(shuffle_id) = 26),
	shuffle_after INTEGER CHECK (shuffle_after IS NULL OR shuffle_after >= 0),
	client_id     TEXT    NOT NULL,
	device        TEXT    NOT NULL CHECK (device IN ('phone', 'tablet', 'computer')),
	updated_at    INTEGER NOT NULL,
	CHECK ((kind = 'shuffle') = (shuffle_id IS NOT NULL AND shuffle_after IS NOT NULL))
) STRICT;
