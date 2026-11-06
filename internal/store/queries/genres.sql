-- Genres live only in the raw tags. The library reads them whole after each scan
-- and edit, and keeps what it derives in memory.

-- name: ListAlbumTagCounts :many
SELECT a.ulid, tt.value, count(*) AS tracks
FROM track_tags tt
JOIN tracks t ON t.id = tt.track_id AND t.missing_since IS NULL
JOIN albums a ON a.id = t.album_id
WHERE tt.key IN (sqlc.slice(tag_keys))
GROUP BY a.id, tt.value;
