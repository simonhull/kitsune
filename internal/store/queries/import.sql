-- An import from another server: what it resolves against, and its writes. None
-- of them queues anything for Last.fm.

-- name: ImportTracks :many
SELECT t.ulid, t.root, t.rel_path, a.ulid AS album
FROM tracks AS t
JOIN albums AS a ON a.id = t.album_id
ORDER BY t.id;

-- name: ImportTrackTags :many
SELECT t.ulid, g.key, g.value
FROM track_tags AS g
JOIN tracks AS t ON t.id = g.track_id
WHERE g.key IN (sqlc.slice(keys))
ORDER BY t.id, g.key, g.ord;

-- name: ImportArtists :many
SELECT ulid, match_key, mbid FROM artists ORDER BY id;

-- name: ImportAlbumHeart :execrows
INSERT INTO album_hearts (user_id, album_id, hearted_at) VALUES (?, ?, ?)
ON CONFLICT (user_id, album_id) DO NOTHING;

-- name: ImportArtistHeart :execrows
INSERT INTO artist_hearts (user_id, artist_id, hearted_at) VALUES (?, ?, ?)
ON CONFLICT (user_id, artist_id) DO NOTHING;

-- A heart that was there keeps the earlier time: hearted_at is the first heart.

-- name: EarlierTrackHeart :exec
UPDATE track_hearts SET hearted_at = sqlc.arg(hearted_at)
WHERE user_id = sqlc.arg(user_id) AND track_id = sqlc.arg(track_id) AND hearted_at > sqlc.arg(hearted_at);

-- name: EarlierAlbumHeart :exec
UPDATE album_hearts SET hearted_at = sqlc.arg(hearted_at)
WHERE user_id = sqlc.arg(user_id) AND album_id = sqlc.arg(album_id) AND hearted_at > sqlc.arg(hearted_at);

-- name: EarlierArtistHeart :exec
UPDATE artist_hearts SET hearted_at = sqlc.arg(hearted_at)
WHERE user_id = sqlc.arg(user_id) AND artist_id = sqlc.arg(artist_id) AND hearted_at > sqlc.arg(hearted_at);

-- A rating already there is kept: it was given since the switch, so it is newer.

-- name: ImportTrackRating :execrows
INSERT INTO track_ratings (user_id, track_id, halves, rated_at) VALUES (?, ?, ?, ?)
ON CONFLICT (user_id, track_id) DO NOTHING;

-- name: TrackRatingHalves :one
SELECT halves FROM track_ratings WHERE user_id = ? AND track_id = ?;
