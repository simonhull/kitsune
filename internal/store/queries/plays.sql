-- name: TrackRowID :one
SELECT id FROM tracks WHERE ulid = ?;

-- name: InsertPlay :execrows
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

-- name: CountUserListening :one
SELECT
    (SELECT count(*) FROM plays p WHERE p.user_id = sqlc.arg(user_id)) AS plays,
    (SELECT count(*) FROM track_hearts h WHERE h.user_id = sqlc.arg(user_id))
      + (SELECT count(*) FROM album_hearts h WHERE h.user_id = sqlc.arg(user_id))
      + (SELECT count(*) FROM artist_hearts h WHERE h.user_id = sqlc.arg(user_id)) AS hearts,
    (SELECT count(*) FROM track_ratings r WHERE r.user_id = sqlc.arg(user_id)) AS rated;
