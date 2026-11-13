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

-- name: AlbumsOnRepeat :many
-- Albums by how many plays of their live tracks began at or after since, most
-- first, ties to the latest play: Home's On repeat row. Undated plays don't
-- count, since their time isn't when they happened. Plays are counted by track
-- first, so each track is looked up once.
WITH counted AS (
    SELECT p.track_id, count(*) AS n, max(p.played_at) AS latest
    FROM plays AS p
    WHERE p.user_id = sqlc.arg(user_id) AND p.played_at >= sqlc.arg(since)
      AND p.source <> sqlc.arg(undated)
    GROUP BY p.track_id
)
SELECT a.ulid, CAST(sum(c.n) AS INTEGER) AS plays
FROM counted AS c
JOIN tracks AS t ON t.id = c.track_id
JOIN albums AS a ON a.id = t.album_id
WHERE t.missing_since IS NULL
GROUP BY a.id
ORDER BY plays DESC, max(c.latest) DESC, a.id
LIMIT sqlc.arg(lim);
