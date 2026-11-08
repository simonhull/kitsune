-- Each person's shuffles: what was shuffled, how, and how far it has got, so a
-- shuffle goes on where it was, on any of their devices. The state is the
-- shuffle's own (an endless shuffle's cycle and recent picks, or a finite
-- order, and the last pages given) and only the shuffle reads it.

CREATE TABLE shuffle_sessions (
	id         INTEGER PRIMARY KEY,
	ulid       TEXT    NOT NULL UNIQUE CHECK (length(ulid) = 26), -- public ID
	user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	scope      TEXT    NOT NULL, -- JSON: what was shuffled, as the request named it
	mode       TEXT    NOT NULL CHECK (mode IN ('finite', 'endless')),
	seed       INTEGER NOT NULL, -- the 64-bit seed's bits
	position   INTEGER NOT NULL CHECK (position >= 0), -- how many units it has given
	state      BLOB    NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
) STRICT;

CREATE INDEX shuffle_sessions_user ON shuffle_sessions (user_id, updated_at);
