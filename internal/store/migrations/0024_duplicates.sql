-- Duplicates: an administrator said a group of songs that look like one
-- recording are not duplicates. A group is named by its key, a hash of its
-- songs' public IDs (internal/duplicates), and tracks holds those IDs as a JSON
-- array, so a third copy turning up gives the group another key and raises it
-- again. An album pair is kept the same way, by its key and its two albums'
-- public IDs. by_user is the administrator who said so, kept while their
-- account is; Bring back deletes the row.
--
-- Nothing is backfilled: no group was dismissed before this runs.

CREATE TABLE duplicate_dismissals (
	group_key    TEXT    PRIMARY KEY,
	tracks       TEXT    NOT NULL CHECK (json_valid(tracks) AND json_type(tracks) = 'array'),
	by_user      INTEGER REFERENCES users (id) ON DELETE SET NULL,
	dismissed_at INTEGER NOT NULL
) STRICT, WITHOUT ROWID;
