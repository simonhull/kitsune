-- An import from another server: what it resolves against, and its writes. None
-- of them queues anything for Last.fm.

-- name: ImportTracks :many
SELECT t.ulid, t.root, t.rel_path, a.ulid AS album
FROM tracks AS t
JOIN albums AS a ON a.id = t.album_id
WHERE t.trashed_at IS NULL -- a trashed song takes nothing an import brings
ORDER BY t.id;

-- name: ImportTrackTags :many
SELECT t.ulid, g.key, g.value
FROM track_tags AS g
JOIN tracks AS t ON t.id = g.track_id
WHERE g.key IN (sqlc.slice(keys)) AND t.trashed_at IS NULL
ORDER BY t.id, g.key, g.ord;

-- name: ImportArtists :many
SELECT ulid, match_key, mbid FROM artists ORDER BY id;

-- A rating already there is kept: it was given since the switch, so it is newer.

-- name: ImportTrackRating :execrows
INSERT INTO track_ratings (user_id, track_id, halves, rated_at) VALUES (?, ?, ?, ?)
ON CONFLICT (user_id, track_id) DO NOTHING;

