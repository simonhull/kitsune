-- name: ListOrganizeTrackFiles :many
-- What the organizer reads of every live track beside the index's rows: where
-- its file is, and its original year. Kept between library writes.
SELECT t.id, t.root, t.rel_path, t.original_year, t.size, t.mtime
FROM tracks t
WHERE t.missing_since IS NULL
ORDER BY t.id;

-- name: ListOrganizeMissing :many
-- Tracks out of the library but not in the Trash: their rows hold their paths,
-- so a song filed there would collide with one.
SELECT t.id, t.root, t.rel_path
FROM tracks t
WHERE t.missing_since IS NOT NULL AND t.trashed_at IS NULL
ORDER BY t.id;

-- name: OrganizeTrackStates :many
-- The tracks a run is about to move, as they are now.
SELECT t.id, t.ulid, t.root, t.rel_path, t.size, t.mtime, t.missing_since, t.trashed_at, t.title, a.ulid AS album
FROM tracks t
JOIN albums a ON a.id = t.album_id
WHERE t.id IN (sqlc.slice(ids));

-- name: OrganizeClaimants :many
-- The songs of a conflict, with what tells them apart, plays and stars the
-- person's own.
SELECT t.id, t.ulid, t.title, a.name AS album, a.ulid AS album_ulid, t.root, t.rel_path, t.format, t.bitrate,
       t.duration_ms, t.added_at,
       (SELECT COUNT(*) FROM plays p WHERE p.track_id = t.id AND p.user_id = sqlc.arg(user_id)) AS plays,
       CAST(COALESCE((SELECT r.halves FROM track_ratings r WHERE r.track_id = t.id AND r.user_id = sqlc.arg(user_id)), 0) AS INTEGER) AS halves
FROM tracks t
JOIN albums a ON a.id = t.album_id
WHERE t.id IN (sqlc.slice(ids));

-- name: CreateOrganizeRun :one
INSERT INTO organize_runs (ulid, kind, by_user, started_at, state, template, total, conflicts, undo_of)
VALUES (?, ?, ?, ?, 'running', ?, ?, ?, ?)
RETURNING id;

-- name: PlanOrganizeMove :one
INSERT INTO organize_moves (run_id, track_id, root, from_rel, to_rel, kind, state)
VALUES (?, ?, ?, ?, ?, ?, 'planned')
RETURNING id;

-- name: AddOrganizeMove :exec
-- A move already made: a folder the run emptied and removed, or a playlist file
-- it rewrote.
INSERT INTO organize_moves (run_id, track_id, root, from_rel, to_rel, kind, state)
VALUES (?, NULL, ?, ?, ?, ?, 'moved');

-- name: SetOrganizeMove :exec
UPDATE organize_moves SET state = ?, reason = ? WHERE id = ?;

-- name: MoveTrackPath :execrows
-- A song's file moved: its row follows, only from where the run found it.
UPDATE tracks SET rel_path = sqlc.arg(to_rel)
WHERE id = sqlc.arg(id) AND root = sqlc.arg(root) AND rel_path = sqlc.arg(from_rel);

-- name: CountOrganizeRun :exec
UPDATE organize_runs
SET moved = moved + sqlc.arg(moved), left_count = left_count + sqlc.arg(left_count),
    failed = failed + sqlc.arg(failed), companions = companions + sqlc.arg(companions),
    playlists = playlists + sqlc.arg(playlists)
WHERE id = sqlc.arg(id);

-- name: FinishOrganizeRun :exec
UPDATE organize_runs SET state = ?, finished_at = ?, folders = ? WHERE id = ?;

-- name: SetOrganizeRunState :exec
UPDATE organize_runs SET state = ? WHERE id = ?;

-- name: ListOrganizeRuns :many
-- The latest runs, a running one first. undoable says a song the run moved is
-- still where it put it.
SELECT r.id, r.ulid, r.kind, u.name AS by_name, r.started_at, r.finished_at, r.state, r.template, r.total,
       r.moved, r.folders, r.conflicts, r.left_count, r.failed, r.companions, r.playlists,
       o.ulid AS undo_of,
       CAST(COALESCE((SELECT x.ulid FROM organize_runs x WHERE x.undo_of = r.id ORDER BY x.id DESC LIMIT 1), '') AS TEXT) AS undone_by,
       EXISTS (SELECT 1 FROM organize_moves m JOIN tracks t ON t.id = m.track_id
               WHERE m.run_id = r.id AND m.kind = 'song' AND m.state = 'moved'
                 AND t.root = m.root AND t.rel_path = m.to_rel AND t.trashed_at IS NULL) AS undoable
FROM organize_runs r
LEFT JOIN users u ON u.id = r.by_user
LEFT JOIN organize_runs o ON o.id = r.undo_of
ORDER BY r.state = 'running' DESC, r.id DESC
LIMIT ?;

-- name: OrganizeRun :one
SELECT r.id, r.ulid, r.kind, u.name AS by_name, r.started_at, r.finished_at, r.state, r.template, r.total,
       r.moved, r.folders, r.conflicts, r.left_count, r.failed, r.companions, r.playlists,
       o.ulid AS undo_of,
       CAST(COALESCE((SELECT x.ulid FROM organize_runs x WHERE x.undo_of = r.id ORDER BY x.id DESC LIMIT 1), '') AS TEXT) AS undone_by,
       EXISTS (SELECT 1 FROM organize_moves m JOIN tracks t ON t.id = m.track_id
               WHERE m.run_id = r.id AND m.kind = 'song' AND m.state = 'moved'
                 AND t.root = m.root AND t.rel_path = m.to_rel AND t.trashed_at IS NULL) AS undoable
FROM organize_runs r
LEFT JOIN users u ON u.id = r.by_user
LEFT JOIN organize_runs o ON o.id = r.undo_of
WHERE r.ulid = ?;

-- name: PlannedOrganizeMoves :many
-- Moves a run journaled and never settled: what a server stopped midway left.
SELECT id, run_id, track_id, root, from_rel, to_rel, kind
FROM organize_moves
WHERE state = 'planned'
ORDER BY id;

-- name: RunningOrganizeRuns :many
SELECT id FROM organize_runs WHERE state = 'running' ORDER BY id;

-- name: MovedOrganizeMoves :many
-- What a run moved, for its undo, with where each song is now.
SELECT m.id, m.track_id, m.root, m.from_rel, m.to_rel, m.kind,
       t.ulid AS track_ulid, t.title, a.ulid AS album, t.root AS track_root, t.rel_path AS track_path,
       t.trashed_at, t.missing_since
FROM organize_moves m
LEFT JOIN tracks t ON t.id = m.track_id
LEFT JOIN albums a ON a.id = t.album_id
WHERE m.run_id = ? AND m.state = 'moved'
ORDER BY m.id;

-- name: LeftOrganizeMoves :many
-- The songs a run left where they were, and why.
SELECT t.ulid AS track_ulid, t.title, m.root, m.from_rel, m.to_rel, m.reason
FROM organize_moves m
JOIN tracks t ON t.id = m.track_id
WHERE m.run_id = ? AND m.kind = 'song' AND m.state = 'skipped'
ORDER BY m.id
LIMIT ?;

-- name: PruneOrganizeRuns :execrows
-- Runs past the last kept, and older than the cutoff, go with their journals.
DELETE FROM organize_runs
WHERE organize_runs.state <> 'running' AND organize_runs.started_at < sqlc.arg(cutoff)
  AND id NOT IN (SELECT k.id FROM organize_runs k ORDER BY k.id DESC LIMIT sqlc.arg(keep));

-- name: StopRunningOrganizeRuns :execrows
-- Runs a stopped server left running: stopped, as Stop leaves them.
UPDATE organize_runs SET state = 'stopped', finished_at = ? WHERE state = 'running';
