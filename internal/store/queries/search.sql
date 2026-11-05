-- Search reads. The hand-written MATCH query in search.go picks the rows; these
-- fetch what a result shows. ids is a JSON array of track IDs, so one prepared
-- statement serves any number of them.

-- name: ListTrackHits :many
SELECT
    t.id, t.ulid, t.title, t.duration_ms, a.ulid AS album_ulid, a.name AS album_name,
    a.artwork_hash, w.width AS artwork_width, w.height AS artwork_height,
    w.accent AS artwork_accent, w.ground AS artwork_ground
FROM tracks t
JOIN albums a ON a.id = t.album_id
LEFT JOIN artwork w ON w.hash = a.artwork_hash
WHERE t.id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: ListTrackHitArtists :many
SELECT ta.track_id, ar.name
FROM track_artists ta
JOIN artists ar ON ar.id = ta.artist_id
WHERE ta.role = 'artist'
  AND ta.track_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)))
ORDER BY ta.track_id, ta.ord;
