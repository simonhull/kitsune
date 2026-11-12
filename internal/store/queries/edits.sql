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

-- An album's first track is the first the Library lists (store.Place): by disc,
-- none after every disc, then track number, none last, then title, then path;
-- live tracks before missing. The scanner's covers rank tracks the same way.

-- name: SettleAlbum :exec
-- An album's year and compilation flag are its first track's.
UPDATE albums SET
    year = (
        SELECT t.year FROM tracks t WHERE t.album_id = albums.id
        ORDER BY t.missing_since IS NOT NULL, t.disc_no NULLS LAST, t.track_no NULLS LAST, t.title, t.root || '/' || t.rel_path
        LIMIT 1
    ),
    compilation = coalesce((
        SELECT t.compilation FROM tracks t WHERE t.album_id = albums.id
        ORDER BY t.missing_since IS NOT NULL, t.disc_no NULLS LAST, t.track_no NULLS LAST, t.title, t.root || '/' || t.rel_path
        LIMIT 1
    ), albums.compilation)
WHERE albums.id = ?;

-- name: ListAlbumCoverFiles :many
SELECT root, rel_path FROM tracks
WHERE album_id = ? AND missing_since IS NULL
ORDER BY disc_no NULLS LAST, track_no NULLS LAST, title, root || '/' || rel_path;

-- name: ArtistToRename :one
SELECT a.id, a.name, a.sort_name, a.mbid, (SELECT count(*) FROM track_artists ta WHERE ta.artist_id = a.id) AS links
FROM artists a WHERE a.match_key = ?;

-- name: MatchKeyTaken :one
SELECT EXISTS (SELECT 1 FROM artists WHERE match_key = ?);

-- name: RenameArtist :exec
UPDATE artists SET match_key = sqlc.arg(match_key), name = sqlc.arg(name), sort_name = sqlc.arg(sort_name), mbid = sqlc.narg(mbid)
WHERE id = sqlc.arg(id);

-- name: SetAlbumArtwork :exec
UPDATE albums SET artwork_hash = sqlc.arg(hash) WHERE id = sqlc.arg(id);
