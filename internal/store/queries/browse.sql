-- Browse reads. Both list queries have the same SELECT list, so the store
-- converts between their row types, and each pages by keyset on an index:
-- albums_sort and albums_added. The collation of sort_name comes from the
-- column; a COLLATE here would stop the row-value comparison from seeking.

-- name: ListAlbumsByName :many
SELECT
    a.id, a.ulid, a.name, a.sort_name, a.artist, a.year, a.compilation, a.added_at,
    a.artwork_hash, w.width AS artwork_width, w.height AS artwork_height,
    w.accent AS artwork_accent, w.ground AS artwork_ground,
    (SELECT count(*) FROM tracks t WHERE t.album_id = a.id AND t.missing_since IS NULL) AS track_count
FROM albums a
LEFT JOIN artwork w ON w.hash = a.artwork_hash
WHERE (a.sort_name, a.id) > (sqlc.arg(after_name), CAST(sqlc.arg(after_id) AS INTEGER))
  AND EXISTS (SELECT 1 FROM tracks t WHERE t.album_id = a.id AND t.missing_since IS NULL)
ORDER BY a.sort_name, a.id
LIMIT sqlc.arg(page_size);

-- name: ListAlbumsByAdded :many
SELECT
    a.id, a.ulid, a.name, a.sort_name, a.artist, a.year, a.compilation, a.added_at,
    a.artwork_hash, w.width AS artwork_width, w.height AS artwork_height,
    w.accent AS artwork_accent, w.ground AS artwork_ground,
    (SELECT count(*) FROM tracks t WHERE t.album_id = a.id AND t.missing_since IS NULL) AS track_count
FROM albums a
LEFT JOIN artwork w ON w.hash = a.artwork_hash
WHERE (a.added_at, a.id) < (sqlc.arg(before_added), sqlc.arg(before_id))
  AND EXISTS (SELECT 1 FROM tracks t WHERE t.album_id = a.id AND t.missing_since IS NULL)
ORDER BY a.added_at DESC, a.id DESC
LIMIT sqlc.arg(page_size);

-- name: GetAlbum :one
SELECT
    a.id, a.ulid, a.name, a.sort_name, a.artist, a.year, a.compilation, a.added_at,
    a.artwork_hash, w.width AS artwork_width, w.height AS artwork_height,
    w.accent AS artwork_accent, w.ground AS artwork_ground,
    (SELECT count(*) FROM tracks t WHERE t.album_id = a.id AND t.missing_since IS NULL) AS track_count
FROM albums a
LEFT JOIN artwork w ON w.hash = a.artwork_hash
WHERE a.ulid = ?
  AND EXISTS (SELECT 1 FROM tracks t WHERE t.album_id = a.id AND t.missing_since IS NULL);

-- name: ListAlbumTracks :many
SELECT
    id, ulid, title, disc_no, disc_total, track_no, track_total, year, duration_ms, format,
    sample_rate, bit_depth, rg_track_gain, rg_track_peak, rg_album_gain, rg_album_peak,
    size, mtime
FROM tracks
WHERE album_id = ? AND missing_since IS NULL
ORDER BY coalesce(disc_no, 1), track_no NULLS LAST, title, id;

-- name: GetTrackFile :one
SELECT root, rel_path, format
FROM tracks
WHERE ulid = ? AND missing_since IS NULL;

-- name: GetTrackAlbum :one
SELECT a.ulid
FROM tracks t
JOIN albums a ON a.id = t.album_id
WHERE t.ulid = ? AND t.missing_since IS NULL;

-- name: ListAlbumFiles :many
SELECT t.id, t.ulid, t.root, t.rel_path, t.size, t.mtime
FROM tracks t
JOIN albums a ON a.id = t.album_id
WHERE a.ulid = ? AND t.missing_since IS NULL
ORDER BY coalesce(t.disc_no, 1), t.track_no NULLS LAST, t.title, t.id;

-- name: ListAlbumTagValues :many
SELECT t.ulid, tt.value
FROM tracks t
JOIN albums a ON a.id = t.album_id
JOIN track_tags tt ON tt.track_id = t.id
WHERE a.ulid = sqlc.arg(album) AND t.missing_since IS NULL AND tt.key IN (sqlc.slice(tag_keys))
ORDER BY t.id, tt.key, tt.ord;

-- name: ListAlbumTrackArtists :many
SELECT ta.track_id, ar.name
FROM track_artists ta
JOIN artists ar ON ar.id = ta.artist_id
WHERE ta.role = 'artist'
  AND ta.track_id IN (SELECT t.id FROM tracks t WHERE t.album_id = ? AND t.missing_since IS NULL)
ORDER BY ta.track_id, ta.ord;

-- name: CountLiveAlbums :one
SELECT count(*) FROM albums a
WHERE EXISTS (SELECT 1 FROM tracks t WHERE t.album_id = a.id AND t.missing_since IS NULL);

-- INDEXED BY: the WHERE column of a partial index doesn't count as covered, so
-- without it the planner picks a slower path (about 21 ms against 10 ms at 100,000
-- tracks). Renaming tracks_live makes this query fail to prepare; a test checks that.
-- name: LiveTrackFacts :one
SELECT count(*) AS tracks, CAST(coalesce(sum(duration_ms), 0) AS INTEGER) AS duration_ms
FROM tracks INDEXED BY tracks_live
WHERE missing_since IS NULL;

-- name: CountScanIssuesByKind :many
SELECT kind, count(*) AS issues FROM scan_issues GROUP BY kind;

-- name: CountWarnedTracks :one
SELECT count(*) FROM tracks WHERE missing_since IS NULL AND scan_warnings <> '[]';

-- name: ListScanIssues :many
SELECT root, rel_path, kind, message, size, mtime FROM scan_issues ORDER BY root, rel_path;

-- name: ListWarnedTracks :many
SELECT ulid, root, rel_path, scan_warnings
FROM tracks
WHERE missing_since IS NULL AND scan_warnings <> '[]'
ORDER BY root, rel_path;
