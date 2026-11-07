-- A person's hearts and stars. A write finds its item by row ID, which the caller
-- looked up by ULID in the same transaction, so an unknown ULID is known apart from
-- a heart that was already there.

-- name: AlbumRowID :one
SELECT id FROM albums WHERE ulid = ?;

-- name: ArtistRowID :one
SELECT id FROM artists WHERE ulid = ?;

-- name: InsertTrackHeart :execrows
INSERT INTO track_hearts (user_id, track_id, hearted_at) VALUES (?, ?, ?)
ON CONFLICT (user_id, track_id) DO NOTHING;

-- name: DeleteTrackHeart :execrows
DELETE FROM track_hearts WHERE user_id = ? AND track_id = ?;

-- name: InsertAlbumHeart :exec
INSERT INTO album_hearts (user_id, album_id, hearted_at) VALUES (?, ?, ?)
ON CONFLICT (user_id, album_id) DO NOTHING;

-- name: DeleteAlbumHeart :exec
DELETE FROM album_hearts WHERE user_id = ? AND album_id = ?;

-- name: InsertArtistHeart :exec
INSERT INTO artist_hearts (user_id, artist_id, hearted_at) VALUES (?, ?, ?)
ON CONFLICT (user_id, artist_id) DO NOTHING;

-- name: DeleteArtistHeart :exec
DELETE FROM artist_hearts WHERE user_id = ? AND artist_id = ?;

-- name: UpsertTrackRating :exec
INSERT INTO track_ratings (user_id, track_id, stars, rated_at) VALUES (?, ?, ?, ?)
ON CONFLICT (user_id, track_id) DO UPDATE SET stars = excluded.stars, rated_at = excluded.rated_at;

-- name: DeleteTrackRating :exec
DELETE FROM track_ratings WHERE user_id = ? AND track_id = ?;

-- Everything a person has marked, by ULID, for the library index.

-- name: ListHeartedTracks :many
SELECT t.ulid FROM track_hearts h JOIN tracks t ON t.id = h.track_id WHERE h.user_id = ?;

-- name: ListHeartedAlbums :many
SELECT a.ulid FROM album_hearts h JOIN albums a ON a.id = h.album_id WHERE h.user_id = ?;

-- name: ListHeartedArtists :many
SELECT a.ulid FROM artist_hearts h JOIN artists a ON a.id = h.artist_id WHERE h.user_id = ?;

-- name: ListTrackRatings :many
SELECT t.ulid, r.stars FROM track_ratings r JOIN tracks t ON t.id = r.track_id WHERE r.user_id = ?;

-- One album's marks, for its page.

-- name: AlbumHearted :one
SELECT EXISTS (
    SELECT 1 FROM album_hearts h JOIN albums a ON a.id = h.album_id
    WHERE h.user_id = sqlc.arg(user_id) AND a.ulid = sqlc.arg(album)
);

-- name: ListAlbumHeartedTracks :many
SELECT t.ulid
FROM track_hearts h
JOIN tracks t ON t.id = h.track_id
JOIN albums a ON a.id = t.album_id
WHERE h.user_id = sqlc.arg(user_id) AND a.ulid = sqlc.arg(album);

-- name: ListAlbumTrackRatings :many
SELECT t.ulid, r.stars
FROM track_ratings r
JOIN tracks t ON t.id = r.track_id
JOIN albums a ON a.id = t.album_id
WHERE r.user_id = sqlc.arg(user_id) AND a.ulid = sqlc.arg(album);
