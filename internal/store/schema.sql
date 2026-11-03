CREATE TABLE users (
	id            INTEGER PRIMARY KEY,
	name          TEXT    NOT NULL UNIQUE COLLATE NOCASE,
	password_hash TEXT    NOT NULL,
	role          TEXT    NOT NULL CHECK (role IN ('admin', 'user')),
	created_at    INTEGER NOT NULL
) STRICT;
