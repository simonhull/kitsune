-- Organize: a run moves songs' files to where a path template puts them, one
-- album at a time, and journals every move before it makes it, so a server
-- stopped midway is put right at the next start, and a run can be undone.
--
-- organize_runs is one run, an undo of one (undo_of), or the one song an edit
-- moved (kind 'edit'). Its counts are what it did, kept as it goes; total is
-- how many songs it set out to deal with. A run is 'running' until it ends
-- 'done' or 'stopped', and 'undone' once an undo has put its songs back.
-- by_user is the administrator who ran it, kept while their account is.
--
-- organize_moves is the journal: one row per file a run moves (a song, or a
-- companion that comes along with one), per folder it emptied and removed,
-- and per playlist file it rewrote (to_rel is then the original's copy). A
-- row is 'planned' before its file moves and 'moved' in the same transaction
-- that sets the song's rel_path, or 'skipped' with a reason; 'undone' once an
-- undo put it back. track_id is set null when a track is deleted for good, so
-- a purge never breaks a journal. Runs past the last ten, older than 90 days,
-- go with their rows.
--
-- Nothing is backfilled: no run has happened when this runs.

CREATE TABLE organize_runs (
	id          INTEGER PRIMARY KEY,
	ulid        TEXT    NOT NULL UNIQUE CHECK (length(ulid) = 26),
	kind        TEXT    NOT NULL CHECK (kind IN ('run', 'undo', 'edit')),
	by_user     INTEGER REFERENCES users (id) ON DELETE SET NULL,
	started_at  INTEGER NOT NULL,
	finished_at INTEGER,
	state       TEXT    NOT NULL CHECK (state IN ('running', 'done', 'stopped', 'undone')),
	template    TEXT    NOT NULL,
	total       INTEGER NOT NULL DEFAULT 0,
	moved       INTEGER NOT NULL DEFAULT 0,
	folders     INTEGER NOT NULL DEFAULT 0,
	conflicts   INTEGER NOT NULL DEFAULT 0,
	left_count  INTEGER NOT NULL DEFAULT 0,
	failed      INTEGER NOT NULL DEFAULT 0,
	companions  INTEGER NOT NULL DEFAULT 0,
	playlists   INTEGER NOT NULL DEFAULT 0,
	undo_of     INTEGER REFERENCES organize_runs (id) ON DELETE SET NULL
) STRICT;

CREATE INDEX organize_runs_started ON organize_runs (started_at);
CREATE INDEX organize_runs_undo_of ON organize_runs (undo_of) WHERE undo_of IS NOT NULL;

CREATE TABLE organize_moves (
	id       INTEGER PRIMARY KEY,
	run_id   INTEGER NOT NULL REFERENCES organize_runs (id) ON DELETE CASCADE,
	track_id INTEGER REFERENCES tracks (id) ON DELETE SET NULL,
	root     TEXT    NOT NULL,
	from_rel TEXT    NOT NULL,
	to_rel   TEXT    NOT NULL,
	kind     TEXT    NOT NULL CHECK (kind IN ('song', 'companion', 'folder', 'playlist')),
	state    TEXT    NOT NULL CHECK (state IN ('planned', 'moved', 'skipped', 'undone')),
	reason   TEXT
) STRICT;

CREATE INDEX organize_moves_run ON organize_moves (run_id, kind, state);
CREATE INDEX organize_moves_planned ON organize_moves (run_id) WHERE state = 'planned';
CREATE INDEX organize_moves_track ON organize_moves (track_id);
