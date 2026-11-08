-- What a person's shuffle is built from: every live song with its album and its
-- own artists, the person's marks and rules on it, and their listening. ids is a
-- JSON array, so one prepared statement serves any number of them.

-- name: ListShuffleTracks :many
SELECT t.id, t.album_id, a.ulid AS album_ulid,
    CAST(coalesce(r.stars, 0) AS INTEGER) AS stars,
    CAST(h.track_id IS NOT NULL AS INTEGER) AS hearted,
    CAST(e.track_id IS NOT NULL AS INTEGER) AS excluded
FROM tracks t
JOIN albums a ON a.id = t.album_id
LEFT JOIN track_ratings r ON r.user_id = sqlc.arg(user_id) AND r.track_id = t.id
LEFT JOIN track_hearts h ON h.user_id = sqlc.arg(user_id) AND h.track_id = t.id
LEFT JOIN shuffle_excluded e ON e.user_id = sqlc.arg(user_id) AND e.track_id = t.id
WHERE t.missing_since IS NULL
ORDER BY t.id;

-- name: ListShuffleTrackArtists :many
-- Each live song's own artists, the primary first. On a compilation they are the
-- song's, not Various Artists.
SELECT ta.track_id, ta.artist_id
FROM track_artists ta
JOIN tracks t ON t.id = ta.track_id AND t.missing_since IS NULL
WHERE ta.role = 'artist'
ORDER BY ta.track_id, ta.ord;

-- name: ListShuffleRuns :many
SELECT run_id, track_id FROM shuffle_run_tracks WHERE user_id = ? ORDER BY run_id, position;

-- name: ListShuffleListens :many
-- What each song's listens say, counted. A song comes up when a listen starts,
-- whether it was heard or skipped. Only completed and skipped listens say how
-- much of a song is heard, and only a skip counts against it: stopped and
-- interrupted ones say nothing.
SELECT track_id,
    CAST(max(started_at) AS INTEGER) AS last_at,
    CAST(total(ended = 'skipped' AND shuffle <> '' AND listened_ms < sqlc.arg(early_ms)
        AND started_at >= sqlc.arg(since)) AS INTEGER) AS early_skips,
    CAST(total(shuffle <> '' AND ended IN ('completed', 'skipped')) AS INTEGER) AS starts,
    CAST(total(shuffle <> '' AND ended = 'skipped') AS INTEGER) AS skips,
    CAST(total(ended IN ('completed', 'skipped')) AS INTEGER) AS listens,
    CAST(total(CASE WHEN ended IN ('completed', 'skipped') AND duration_ms > 0
        THEN min(1.0, CAST(listened_ms AS REAL) / duration_ms) ELSE 0 END) AS REAL) AS heard
FROM listens
WHERE user_id = sqlc.arg(user_id)
GROUP BY track_id;

-- name: ListShuffleLastPlays :many
SELECT track_id, CAST(max(played_at) AS INTEGER) AS last_at
FROM plays
WHERE user_id = ?
GROUP BY track_id;

-- name: ListShuffleSittings :many
-- The person's own choices that played through, outside shuffle, in time order,
-- with each song's primary artist: what co-listening is counted from. Listens
-- in a shuffle are left out, so flow never learns from its own picks.
SELECT l.started_at, ta.artist_id
FROM listens l
JOIN track_artists ta ON ta.track_id = l.track_id AND ta.role = 'artist' AND ta.ord = 0
WHERE l.user_id = ? AND l.shuffle = '' AND l.ended = 'completed' AND l.started_at >= ?
ORDER BY l.started_at;

-- name: ListShuffleHeartedAlbums :many
-- The albums the Library list's Hearted filter keeps whole: hearted ones, and
-- those whose artist is. An album's artist there is its first owner, as the
-- index spells it.
SELECT a.id FROM albums a
WHERE EXISTS (SELECT 1 FROM album_hearts h WHERE h.user_id = sqlc.arg(user_id) AND h.album_id = a.id)
   OR EXISTS (SELECT 1 FROM artist_hearts h WHERE h.user_id = sqlc.arg(user_id) AND h.artist_id = (
        SELECT ta.artist_id
        FROM tracks t
        JOIN track_artists ta ON ta.track_id = t.id
        WHERE t.album_id = a.id AND t.missing_since IS NULL
          AND (ta.role = 'album_artist'
           OR (ta.ord = 0 AND a.compilation = 0 AND NOT EXISTS (
                SELECT 1 FROM tracks o
                JOIN track_artists oa ON oa.track_id = o.id AND oa.role = 'album_artist'
                WHERE o.album_id = a.id AND o.missing_since IS NULL)))
        ORDER BY ta.ord, t.id
        LIMIT 1));

-- name: ListArtistTrackIDs :many
-- The songs an artist is on, as an artist or an album artist.
SELECT DISTINCT ta.track_id
FROM track_artists ta
JOIN artists ar ON ar.id = ta.artist_id
WHERE ar.ulid = ?;

-- name: ListTrackRowIDs :many
SELECT id, ulid FROM tracks
WHERE ulid IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: ListArtistLinksByID :many
SELECT id, ulid, name FROM artists
WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: InsertShuffleSession :exec
INSERT INTO shuffle_sessions (ulid, user_id, scope, mode, seed, position, state, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: PruneShuffleSessions :exec
-- Keeps a person's latest few shuffles.
DELETE FROM shuffle_sessions
WHERE shuffle_sessions.user_id = sqlc.arg(user_id) AND shuffle_sessions.id NOT IN (
    SELECT k.id FROM shuffle_sessions k
    WHERE k.user_id = sqlc.arg(user_id)
    ORDER BY k.updated_at DESC, k.id DESC
    LIMIT sqlc.arg(keep));

-- name: GetShuffleSession :one
SELECT ulid, scope, mode, seed, position, state, created_at, updated_at
FROM shuffle_sessions
WHERE user_id = ? AND ulid = ?;

-- name: UpdateShuffleSession :execrows
UPDATE shuffle_sessions SET position = ?, state = ?, updated_at = ?
WHERE user_id = sqlc.arg(user_id) AND ulid = sqlc.arg(ulid) AND position = sqlc.arg(was);
