-- Trash: songs an administrator deleted, whose files wait under the root's
-- .kitsune-trash. A trashed track's missing_since is its trashed_at, so every
-- live read leaves it out; see migrations/0019_trash.sql.

-- name: TrashableTracks :many
SELECT t.id, t.ulid, t.root, t.rel_path, t.title, t.album_id, a.ulid AS album
FROM tracks t
JOIN albums a ON a.id = t.album_id
WHERE t.ulid IN (sqlc.slice(ids)) AND t.missing_since IS NULL
ORDER BY t.id;

-- name: TrashableAlbumTracks :many
SELECT t.id, t.ulid, t.root, t.rel_path, t.title, t.album_id, a.ulid AS album
FROM tracks t
JOIN albums a ON a.id = t.album_id
WHERE a.ulid IN (sqlc.slice(ids)) AND t.missing_since IS NULL
ORDER BY t.id;

-- name: MarkTrashed :execrows
UPDATE tracks SET
    trashed_at = sqlc.arg(at), trashed_by = sqlc.narg(by), missing_since = sqlc.arg(at),
    restore_path = rel_path, rel_path = sqlc.arg(trash_path)
WHERE id = sqlc.arg(id) AND missing_since IS NULL;

-- name: UnmarkTrashed :execrows
UPDATE tracks SET
    rel_path = restore_path, restore_path = NULL, trashed_at = NULL, trashed_by = NULL, missing_since = NULL
WHERE id = ? AND trashed_at IS NOT NULL;

-- name: TrashedTracks :many
SELECT t.id, t.ulid, t.root, t.rel_path, coalesce(t.restore_path, '') AS restore_path, t.title, t.album_id, a.ulid AS album
FROM tracks t
JOIN albums a ON a.id = t.album_id
WHERE t.ulid IN (sqlc.slice(ids)) AND t.trashed_at IS NOT NULL
ORDER BY t.id;

-- name: TrashedBefore :many
SELECT t.id, t.ulid, t.root, t.rel_path, coalesce(t.restore_path, '') AS restore_path, t.title, t.album_id, a.ulid AS album
FROM tracks t INDEXED BY tracks_trashed
JOIN albums a ON a.id = t.album_id
WHERE t.trashed_at < sqlc.arg(cutoff)
ORDER BY t.trashed_at, t.id
LIMIT sqlc.arg(n);

-- name: TrackAtPath :one
SELECT EXISTS (SELECT 1 FROM tracks WHERE root = ? AND rel_path = ?) AS taken;

-- name: ListTrash :many
-- The Trash page: every trashed track with its album, in album order. Go puts
-- the albums newest first.
SELECT t.ulid, t.title, t.size, t.trashed_at, u.name AS trashed_by,
       t.disc_no, t.track_no, t.root, t.rel_path,
       a.ulid AS album, a.name AS album_title, a.artist,
       a.artwork_hash, w.width AS artwork_width, w.height AS artwork_height,
       w.accent AS artwork_accent, w.ground AS artwork_ground
FROM tracks t INDEXED BY tracks_trashed
JOIN albums a ON a.id = t.album_id
LEFT JOIN users u ON u.id = t.trashed_by
LEFT JOIN artwork w ON w.hash = a.artwork_hash
WHERE t.trashed_at IS NOT NULL
ORDER BY a.id, t.disc_no NULLS LAST, t.track_no NULLS LAST, t.title, t.root || '/' || t.rel_path;
