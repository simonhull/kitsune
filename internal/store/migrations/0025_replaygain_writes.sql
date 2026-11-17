-- ReplayGain writes: a run writes Kitsune's own loudness measure into the
-- files of songs that carry no ReplayGain, one album at a time, and journals
-- every write before it makes it, so a server stopped midway is settled at
-- the next start, and a run can be undone.
--
-- gain_runs is one run, or an undo of one (undo_of). Its counts are what it
-- did, kept as it goes; total is how many songs it set out to write, and
-- albums how many albums they are on. A run is 'running' until it ends
-- 'done' or 'stopped', and 'undone' once an undo has taken out everything it
-- wrote. by_user is the administrator who ran it, kept while their account is.
--
-- gain_writes is the journal: one row per song a run means to write. A row is
-- 'planned' before its file is written, then 'written' (with the file's size
-- and mtime after) or 'skipped' with a reason; 'undone' once an undo took the
-- tags out again. album is the song's album's ULID and ord its place in the
-- run, for the run's album list. The four values are what it wrote (album
-- gain and peak NULL when it wrote none); prior is the ReplayGain tags the
-- file held before, as JSON of each key as spelled and its values, which an
-- undo puts back. track_id is set null when a track is deleted for good, so a
-- purge never breaks a journal. Runs past the last ten, older than 90 days,
-- go with their rows.
--
-- Nothing is backfilled: no run has happened when this runs.

CREATE TABLE gain_runs (
	id          INTEGER PRIMARY KEY,
	ulid        TEXT    NOT NULL UNIQUE CHECK (length(ulid) = 26),
	kind        TEXT    NOT NULL CHECK (kind IN ('run', 'undo')),
	by_user     INTEGER REFERENCES users (id) ON DELETE SET NULL,
	started_at  INTEGER NOT NULL,
	finished_at INTEGER,
	state       TEXT    NOT NULL CHECK (state IN ('running', 'done', 'stopped', 'undone')),
	total       INTEGER NOT NULL DEFAULT 0,
	albums      INTEGER NOT NULL DEFAULT 0,
	written     INTEGER NOT NULL DEFAULT 0,
	left_count  INTEGER NOT NULL DEFAULT 0,
	failed      INTEGER NOT NULL DEFAULT 0,
	undo_of     INTEGER REFERENCES gain_runs (id) ON DELETE SET NULL
) STRICT;

CREATE INDEX gain_runs_undo_of ON gain_runs (undo_of) WHERE undo_of IS NOT NULL;

CREATE TABLE gain_writes (
	id          INTEGER PRIMARY KEY,
	run_id      INTEGER NOT NULL REFERENCES gain_runs (id) ON DELETE CASCADE,
	track_id    INTEGER REFERENCES tracks (id) ON DELETE SET NULL,
	album       TEXT    NOT NULL,
	ord         INTEGER NOT NULL,
	root        TEXT    NOT NULL,
	rel_path    TEXT    NOT NULL,
	state       TEXT    NOT NULL CHECK (state IN ('planned', 'written', 'skipped', 'undone')),
	reason      TEXT,
	track_gain  REAL    NOT NULL,
	track_peak  REAL    NOT NULL,
	album_gain  REAL,
	album_peak  REAL,
	prior       TEXT    NOT NULL DEFAULT '{}',
	size_after  INTEGER,
	mtime_after INTEGER,
	undoes      INTEGER REFERENCES gain_writes (id) ON DELETE SET NULL
) STRICT;

CREATE INDEX gain_writes_run ON gain_writes (run_id, state);
CREATE INDEX gain_writes_planned ON gain_writes (run_id) WHERE state = 'planned';
CREATE INDEX gain_writes_track ON gain_writes (track_id, state);
