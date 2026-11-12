-- Reads of a Navidrome database. Datetimes are cast to text, so the driver
-- hands back what Navidrome wrote, and parseTime reads it.

-- name: Libraries :many
SELECT id, path FROM library ORDER BY id;

-- name: Users :many
SELECT id, user_name FROM "user" ORDER BY id;

-- name: Songs :many
SELECT id, library_id, path, missing, mbz_release_track_id, mbz_recording_id, album_id
FROM media_file
ORDER BY id;

-- name: Annotations :many
SELECT
    item_id,
    item_type,
    CAST(coalesce(play_count, 0) AS INTEGER) AS play_count,
    CAST(coalesce(play_date, '') AS TEXT) AS play_date,
    CAST(coalesce(rating, 0) AS INTEGER) AS rating,
    starred,
    CAST(coalesce(starred_at, '') AS TEXT) AS starred_at,
    CAST(coalesce(rated_at, '') AS TEXT) AS rated_at
FROM annotation
WHERE user_id = ?
ORDER BY item_type, item_id;

-- name: Scrobbles :many
SELECT id, media_file_id, submission_time FROM scrobbles WHERE user_id = ? ORDER BY id;

-- name: Artists :many
SELECT id, name, mbz_artist_id FROM artist WHERE id IN (sqlc.slice(ids)) ORDER BY id;
