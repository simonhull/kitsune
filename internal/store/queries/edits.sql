-- Tag edits: the rows an edit moves before it writes, so its tracks land on them.

-- name: AlbumGroupKey :one
SELECT group_key FROM albums WHERE id = ?;

-- name: GroupKeyTaken :one
SELECT EXISTS (SELECT 1 FROM albums WHERE group_key = ?);

-- name: SetAlbumGroupKey :exec
UPDATE albums SET group_key = sqlc.arg(group_key) WHERE id = sqlc.arg(id);

-- name: CountAlbumTracks :one
SELECT count(*) FROM tracks WHERE album_id = ?;

-- name: TrackAlbumRow :one
SELECT album_id FROM tracks WHERE id = ?;

-- name: MergeAlbumInto :exec
-- The album a merge lands in keeps its cover, or takes the other's, and was
-- first seen when the older of the two was.
UPDATE albums SET
    artwork_hash = coalesce(albums.artwork_hash, (SELECT f.artwork_hash FROM albums f WHERE f.id = sqlc.arg(from_id))),
    added_at = min(albums.added_at, (SELECT f.added_at FROM albums f WHERE f.id = sqlc.arg(from_id)))
WHERE albums.id = sqlc.arg(into_id);

-- name: MoveAlbumHearts :exec
INSERT INTO album_hearts (user_id, album_id, hearted_at)
SELECT h.user_id, sqlc.arg(into_id), h.hearted_at FROM album_hearts h WHERE h.album_id = sqlc.arg(from_id)
ON CONFLICT (user_id, album_id) DO UPDATE SET hearted_at = min(album_hearts.hearted_at, excluded.hearted_at);
