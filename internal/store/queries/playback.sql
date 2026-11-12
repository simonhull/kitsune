-- Where each person left off. The queue is JSON internal/playback has checked;
-- the store never reads its shape.

-- name: GetPlayback :one
SELECT kind, queue, at, position_ms, ordered, shuffle_id, shuffle_after, client_id, device, updated_at
FROM playback WHERE user_id = ?;

-- name: UpsertPlayback :exec
INSERT INTO playback (user_id, kind, queue, at, position_ms, ordered, shuffle_id, shuffle_after, client_id, device, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (user_id) DO UPDATE SET
    kind = excluded.kind, queue = excluded.queue, at = excluded.at, position_ms = excluded.position_ms,
    ordered = excluded.ordered, shuffle_id = excluded.shuffle_id, shuffle_after = excluded.shuffle_after,
    client_id = excluded.client_id, device = excluded.device, updated_at = excluded.updated_at;

-- name: ListLiveTracksByULID :many
-- Each of the named tracks that is live, with its album as it is now. ulids is a
-- JSON array, so one prepared statement serves any number of them.
SELECT t.ulid, a.ulid AS album_ulid
FROM tracks t JOIN albums a ON a.id = t.album_id
WHERE t.missing_since IS NULL
  AND t.ulid IN (SELECT value FROM json_each(CAST(sqlc.arg(ulids) AS TEXT)));
