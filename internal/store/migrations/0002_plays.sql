CREATE TABLE plays (
	user_id   INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	track_id  INTEGER NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
	played_at INTEGER NOT NULL, -- when the listen started, by the client's clock, never ahead of the server's
	source    TEXT    NOT NULL, -- the client's name
	client_id TEXT    NOT NULL, -- chosen by the client, so a retry counts once
	UNIQUE (user_id, client_id)
) STRICT;

CREATE INDEX plays_recent ON plays (user_id, played_at DESC, track_id);
CREATE INDEX plays_track ON plays (track_id); -- deleting a track scans plays without it
