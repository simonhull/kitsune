CREATE TABLE users (
	id            INTEGER PRIMARY KEY,
	name          TEXT    NOT NULL UNIQUE COLLATE NOCASE,
	password_hash TEXT    NOT NULL,
	role          TEXT    NOT NULL CHECK (role IN ('admin', 'user')),
	created_at    INTEGER NOT NULL
) STRICT;

CREATE TABLE sessions (
	token_hash BLOB    PRIMARY KEY CHECK (length(token_hash) = 32),
	user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL,
	last_seen  INTEGER NOT NULL,
	expires_at INTEGER NOT NULL,
	user_agent TEXT    NOT NULL
) STRICT;

CREATE INDEX sessions_user ON sessions (user_id);
CREATE INDEX sessions_expires ON sessions (expires_at);
