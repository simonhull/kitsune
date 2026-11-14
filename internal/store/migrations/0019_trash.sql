-- Trash: a song an administrator deleted keeps its row, with everything that
-- hangs off it, while its file waits under <root>/.kitsune-trash/<ULID>/.
--
-- A trashed track is out of the library, so its missing_since is set too, to
-- the same time as trashed_at: every read that means "live" (missing_since IS
-- NULL, tracks_live) leaves it out with no change of its own. What reads
-- missing tracks to follow, hold or delete them adds trashed_at IS NULL.
--
-- rel_path always names where the file is, so while trashed it is the path in
-- the trash, and restore_path is where the file goes back to. trashed_by is the
-- administrator who moved it, kept while their account is.
--
-- No row is trashed when this runs, so nothing is backfilled.

ALTER TABLE tracks ADD COLUMN trashed_at INTEGER;
ALTER TABLE tracks ADD COLUMN trashed_by INTEGER REFERENCES users (id) ON DELETE SET NULL;
ALTER TABLE tracks ADD COLUMN restore_path TEXT;

CREATE INDEX tracks_trashed ON tracks (trashed_at) WHERE trashed_at IS NOT NULL;
