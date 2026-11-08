-- Every listen a person starts, and how it ended. A play is a listen that
-- counted, for play counts and Last.fm, and plays stay as they were. Listens are
-- what shuffle learns from: what came up, how much of it was heard, and whether
-- the person moved on. Only a skip ever counts against a song.

CREATE TABLE listens (
	user_id     INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	track_id    INTEGER NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
	client_id   TEXT    NOT NULL, -- chosen by the client, so a start and its end are one row, however often sent
	shuffle     TEXT    NOT NULL, -- the shuffle session's ID, or '' for a listen outside one
	started_at  INTEGER NOT NULL, -- by the client's clock, never ahead of the server's
	listened_ms INTEGER NOT NULL CHECK (listened_ms >= 0), -- how much of it played
	duration_ms INTEGER NOT NULL, -- the track's, as the listen began
	-- How it ended; NULL while it is under way, or if its end never arrived.
	ended       TEXT    CHECK (ended IN ('completed', 'skipped', 'stopped', 'interrupted')),
	UNIQUE (user_id, client_id)
) STRICT;

CREATE INDEX listens_history ON listens (user_id, track_id);
CREATE INDEX listens_recent ON listens (user_id, started_at);
CREATE INDEX listens_track ON listens (track_id); -- deleting a track scans listens without it
