-- Metadata matching: answers kept for a day, and the queue's state.

-- name: GetLookup :one
SELECT body FROM lookups WHERE source = ? AND key = ? AND fetched_at >= ?;

-- name: PutLookup :exec
INSERT INTO lookups (source, key, body, fetched_at) VALUES (?, ?, ?, ?)
ON CONFLICT (source, key) DO UPDATE SET body = excluded.body, fetched_at = excluded.fetched_at;

-- name: PruneLookups :exec
DELETE FROM lookups WHERE fetched_at < ?;

-- MatchQueue lists the albums the queue holds: every one with live tracks and
-- no MBID, and every one matched here, with its state and its live songs.
-- name: MatchQueue :many
SELECT a.ulid, a.name, a.artist, s.state, count(*) AS tracks
FROM albums a
JOIN tracks t INDEXED BY tracks_live ON t.album_id = a.id AND t.missing_since IS NULL
LEFT JOIN match_states s ON s.album_id = a.id
WHERE a.mbid IS NULL OR s.state = 'matched'
GROUP BY a.id
ORDER BY a.sort_name, a.id;

-- name: SetMatchState :execrows
INSERT INTO match_states (album_id, state, at)
SELECT id, sqlc.arg(state), sqlc.arg(at) FROM albums WHERE ulid = sqlc.arg(ulid)
ON CONFLICT (album_id) DO UPDATE SET state = excluded.state, at = excluded.at;

-- name: ClearMatchState :exec
DELETE FROM match_states WHERE album_id = (SELECT id FROM albums WHERE ulid = ?);

-- name: AlbumExists :one
SELECT EXISTS (SELECT 1 FROM albums WHERE ulid = ?);
