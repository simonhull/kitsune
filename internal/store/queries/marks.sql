-- A person's stars, and the hearts they kept from before stars were the only like.
-- A write finds its item by row ID, which the caller looked up by ULID in the same
-- transaction, so an unknown ULID is known apart from a mark that was already there.

-- name: AlbumRowID :one
SELECT id FROM albums WHERE ulid = ?;

-- name: ArtistRowID :one
SELECT id FROM artists WHERE ulid = ?;

-- name: InsertTrackHeart :exec
INSERT INTO track_hearts (user_id, track_id, hearted_at) VALUES (?, ?, ?)
ON CONFLICT (user_id, track_id) DO NOTHING;

-- name: InsertAlbumHeart :exec
INSERT INTO album_hearts (user_id, album_id, hearted_at) VALUES (?, ?, ?)
ON CONFLICT (user_id, album_id) DO NOTHING;

-- name: InsertArtistHeart :exec
INSERT INTO artist_hearts (user_id, artist_id, hearted_at) VALUES (?, ?, ?)
ON CONFLICT (user_id, artist_id) DO NOTHING;

-- name: GetTrackRating :one
SELECT halves FROM track_ratings WHERE user_id = ? AND track_id = ?;

-- name: UpsertTrackRating :exec
INSERT INTO track_ratings (user_id, track_id, halves, rated_at) VALUES (?, ?, ?, ?)
ON CONFLICT (user_id, track_id) DO UPDATE SET halves = excluded.halves, rated_at = excluded.rated_at;

-- name: DeleteTrackRating :exec
DELETE FROM track_ratings WHERE user_id = ? AND track_id = ?;

-- Every rating a person has, by ULID, for the library index.

-- name: ListTrackRatings :many
SELECT t.ulid, r.halves FROM track_ratings r JOIN tracks t ON t.id = r.track_id WHERE r.user_id = ?;

-- One album's ratings, for its page.

-- name: ListAlbumTrackRatings :many
SELECT t.ulid, r.halves
FROM track_ratings r
JOIN tracks t ON t.id = r.track_id
JOIN albums a ON a.id = t.album_id
WHERE r.user_id = sqlc.arg(user_id) AND a.ulid = sqlc.arg(album);

-- The hearts a person kept, for the one-time change that turns them into stars.

-- name: CountHeartedTracksByRating :many
SELECT CAST(coalesce(r.halves, 0) AS INTEGER) AS halves, count(*) AS songs
FROM track_hearts h
LEFT JOIN track_ratings r ON r.user_id = h.user_id AND r.track_id = h.track_id
WHERE h.user_id = ?
GROUP BY 1
ORDER BY 1;

-- name: CountAlbumHearts :one
SELECT count(*) FROM album_hearts WHERE user_id = ?;

-- name: CountArtistHearts :one
SELECT count(*) FROM artist_hearts WHERE user_id = ?;

-- name: ListUnratedHeartedTracks :many
SELECT t.id, t.ulid, t.title,
    CAST(coalesce((
        SELECT ar.name FROM track_artists ta JOIN artists ar ON ar.id = ta.artist_id
        WHERE ta.track_id = t.id AND ta.role = 'artist' ORDER BY ta.ord LIMIT 1
    ), '') AS TEXT) AS artist
FROM track_hearts h
JOIN tracks t ON t.id = h.track_id
WHERE h.user_id = sqlc.arg(user_id)
  AND NOT EXISTS (SELECT 1 FROM track_ratings r WHERE r.user_id = h.user_id AND r.track_id = h.track_id)
ORDER BY h.hearted_at DESC, t.id
LIMIT sqlc.arg(limit);

-- name: DeleteTrackHearts :execrows
DELETE FROM track_hearts WHERE user_id = ?;

-- name: DeleteAlbumHearts :execrows
DELETE FROM album_hearts WHERE user_id = ?;

-- name: DeleteArtistHearts :execrows
DELETE FROM artist_hearts WHERE user_id = ?;
