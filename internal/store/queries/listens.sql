-- Every listen a person starts, and how it ended.

-- name: TrackForListen :one
SELECT id, duration_ms FROM tracks WHERE ulid = ?;

-- name: UpsertListen :exec
-- A start and its end are one row: the end fills in what the start couldn't
-- know, the shuffle's session ID included, and once a listen has ended nothing
-- changes it.
INSERT INTO listens (user_id, track_id, client_id, shuffle, started_at, listened_ms, duration_ms, ended)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (user_id, client_id) DO UPDATE SET
    listened_ms = CASE WHEN listens.ended IS NULL
        THEN max(listens.listened_ms, excluded.listened_ms) ELSE listens.listened_ms END,
    shuffle = CASE WHEN listens.ended IS NULL AND excluded.shuffle <> ''
        THEN excluded.shuffle ELSE listens.shuffle END,
    ended = coalesce(listens.ended, excluded.ended);

-- name: RecentListens :many
SELECT t.ulid, l.client_id, l.shuffle, l.started_at, l.listened_ms, l.duration_ms, l.ended
FROM listens AS l
JOIN tracks AS t ON t.id = l.track_id
WHERE l.user_id = ?
ORDER BY l.started_at DESC, l.rowid DESC
LIMIT ?;
