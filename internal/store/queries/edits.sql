-- Tag edits: the rows an edit moves before it writes, so its tracks land on them.

-- name: AlbumGroupKey :one
SELECT group_key FROM albums WHERE id = ?;

-- name: GroupKeyTaken :one
SELECT EXISTS (SELECT 1 FROM albums WHERE group_key = ?);

-- name: SetAlbumGroupKey :exec
UPDATE albums SET group_key = sqlc.arg(group_key) WHERE id = sqlc.arg(id);
