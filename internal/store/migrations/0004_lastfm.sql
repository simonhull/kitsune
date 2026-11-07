-- Last.fm: a person's connected account, and what waits to be sent to it.
-- Deleting the account, or the person, takes the outbox with it.

CREATE TABLE lastfm_accounts (
	user_id      INTEGER PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
	name         TEXT    NOT NULL, -- the Last.fm user name
	session_key  TEXT    NOT NULL, -- as Last.fm gave it: as sensitive as a password, it never leaves the server
	scrobble     INTEGER NOT NULL CHECK (scrobble IN (0, 1)),
	now_playing  INTEGER NOT NULL CHECK (now_playing IN (0, 1)),
	loves        INTEGER NOT NULL CHECK (loves IN (0, 1)),
	state        TEXT    NOT NULL CHECK (state IN ('ok', 'reconnect')),
	connected_at INTEGER NOT NULL,
	last_sent_at INTEGER -- the latest send Last.fm took; NULL before the first
) STRICT;

CREATE TABLE lastfm_outbox (
	id       INTEGER PRIMARY KEY, -- the order things happened in, and are sent in
	user_id  INTEGER NOT NULL REFERENCES lastfm_accounts (user_id) ON DELETE CASCADE,
	kind     TEXT    NOT NULL CHECK (kind IN ('scrobble', 'love', 'unlove')),
	track_id INTEGER NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
	at       INTEGER NOT NULL, -- when the play started, or when the heart changed
	attempts INTEGER NOT NULL, -- sends that failed for a reason that passes
	next_at  INTEGER NOT NULL  -- not sent before; never less than an earlier row's of the same user
) STRICT;

CREATE INDEX lastfm_outbox_due ON lastfm_outbox (user_id, next_at, id);
CREATE INDEX lastfm_outbox_track ON lastfm_outbox (track_id); -- deleting a track scans the outbox without it
