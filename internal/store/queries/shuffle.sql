-- Each person's shuffle rules: songs left out, and runs of songs played together.

-- name: InsertExcluded :exec
INSERT INTO shuffle_excluded (user_id, track_id, excluded_at) VALUES (?, ?, ?)
ON CONFLICT (user_id, track_id) DO NOTHING;

-- name: DeleteExcluded :exec
DELETE FROM shuffle_excluded WHERE user_id = ? AND track_id = ?;

-- name: ListExcludedOf :many
SELECT t.ulid FROM shuffle_excluded e
JOIN tracks t ON t.id = e.track_id
WHERE e.user_id = sqlc.arg(user_id) AND t.ulid IN (sqlc.slice(tracks))
ORDER BY t.ulid;

-- name: RunOfTrack :one
SELECT r.id, r.ulid FROM shuffle_run_tracks rt
JOIN shuffle_runs r ON r.id = rt.run_id
WHERE rt.user_id = ? AND rt.track_id = ?;

-- name: RunExists :one
SELECT EXISTS (SELECT 1 FROM shuffle_runs WHERE id = ?);

-- name: LeaveRun :exec
DELETE FROM shuffle_run_tracks WHERE user_id = ? AND track_id = ?;

-- name: InsertRun :one
INSERT INTO shuffle_runs (ulid, user_id, created_at) VALUES (?, ?, ?)
RETURNING id;

-- name: InsertRunTrack :exec
INSERT INTO shuffle_run_tracks (run_id, user_id, position, track_id) VALUES (?, ?, ?, ?);

-- name: DeleteRun :execrows
DELETE FROM shuffle_runs WHERE user_id = ? AND ulid = ?;

-- name: ListRunsOf :many
-- Every run that holds one of the songs, whole, each song in order.
SELECT r.ulid AS run, t.ulid AS track
FROM shuffle_runs r
JOIN shuffle_run_tracks rt ON rt.run_id = r.id
JOIN tracks t ON t.id = rt.track_id
WHERE r.user_id = sqlc.arg(user_id) AND r.id IN (
    SELECT rt2.run_id FROM shuffle_run_tracks rt2
    JOIN tracks t2 ON t2.id = rt2.track_id
    WHERE rt2.user_id = sqlc.arg(user_id) AND t2.ulid IN (sqlc.slice(tracks))
)
ORDER BY r.id, rt.position;

-- name: ListIndexShuffle :many
-- Every song in a person's runs, with its run and place, and every song they left out.
SELECT rt.track_id, r.ulid AS run, rt.position, 0 AS excluded
FROM shuffle_run_tracks rt
JOIN shuffle_runs r ON r.id = rt.run_id
WHERE rt.user_id = sqlc.arg(user_id)
UNION ALL
SELECT e.track_id, '' AS run, 0 AS position, 1 AS excluded
FROM shuffle_excluded e
WHERE e.user_id = sqlc.arg(user_id);
