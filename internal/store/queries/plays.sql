-- name: TrackRowID :one
SELECT id FROM tracks WHERE ulid = ?;

-- name: InsertPlay :exec
INSERT INTO plays (user_id, track_id, played_at, source, client_id)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (user_id, client_id) DO NOTHING;

-- name: RecentPlays :many
SELECT t.ulid, p.played_at, p.source
FROM plays AS p
JOIN tracks AS t ON t.id = p.track_id
WHERE p.user_id = ?
ORDER BY p.played_at DESC
LIMIT ?;
