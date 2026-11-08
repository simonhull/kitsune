-- Each person's shuffle rules. Exclude: a song shuffle never picks. Combine: songs
-- that always play together, in order, as one run. Both are a person's own, and go
-- with that person and with the song. A song is in at most one of a person's runs,
-- so a cycle can't be made, and a run of fewer than two songs is no run: it goes
-- with its last-but-one song.

CREATE TABLE shuffle_excluded (
	user_id     INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	track_id    INTEGER NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
	excluded_at INTEGER NOT NULL,
	PRIMARY KEY (user_id, track_id)
) STRICT, WITHOUT ROWID;

CREATE INDEX shuffle_excluded_track ON shuffle_excluded (track_id);

CREATE TABLE shuffle_runs (
	id         INTEGER PRIMARY KEY,
	ulid       TEXT    NOT NULL UNIQUE CHECK (length(ulid) = 26), -- public ID
	user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL,
	UNIQUE (id, user_id) -- what a run's songs point at, so each of them names its person
) STRICT;

CREATE INDEX shuffle_runs_user ON shuffle_runs (user_id);

CREATE TABLE shuffle_run_tracks (
	run_id   INTEGER NOT NULL,
	user_id  INTEGER NOT NULL,
	position INTEGER NOT NULL, -- the song's place in the run, from 0; gaps are allowed
	track_id INTEGER NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
	PRIMARY KEY (run_id, position),
	UNIQUE (user_id, track_id), -- one run per song, per person
	FOREIGN KEY (run_id, user_id) REFERENCES shuffle_runs (id, user_id) ON DELETE CASCADE
) STRICT, WITHOUT ROWID;

CREATE INDEX shuffle_run_tracks_track ON shuffle_run_tracks (track_id);

CREATE TRIGGER shuffle_runs_need_two AFTER DELETE ON shuffle_run_tracks
WHEN (SELECT count(*) FROM shuffle_run_tracks WHERE run_id = OLD.run_id) < 2
BEGIN
	DELETE FROM shuffle_runs WHERE id = OLD.run_id;
END;
