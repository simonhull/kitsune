-- Server settings, such as the library folders and the Last.fm key: one JSON
-- value per key, with where it came from. server.toml, the environment and flags
-- only fill a key that has no row; after that the row is the truth. Which keys
-- there are, and what each may hold, is decided in Go; the store only keeps the
-- text. A secret is stored as written, because the server signs with it.

CREATE TABLE settings (
	key        TEXT    PRIMARY KEY,
	value      TEXT    NOT NULL, -- JSON, checked by the key's rule before it is written
	source     TEXT    NOT NULL CHECK (source IN ('web', 'server.toml', 'env', 'flag')),
	updated_at INTEGER NOT NULL
) STRICT, WITHOUT ROWID;
