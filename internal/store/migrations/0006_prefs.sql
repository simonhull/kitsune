-- A person's preferences, such as the Library list's columns: one JSON value per
-- key, and each goes with its person. Which keys there are, and what each may
-- hold, is decided in Go; the store only keeps the text.

CREATE TABLE user_prefs (
	user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	key        TEXT    NOT NULL,
	value      TEXT    NOT NULL, -- JSON, checked by the key's rule before it is written
	updated_at INTEGER NOT NULL,
	PRIMARY KEY (user_id, key)
) STRICT, WITHOUT ROWID;
