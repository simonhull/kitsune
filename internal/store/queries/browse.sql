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

-- An album's tracks go by disc, then track number, then title. Those with no
-- disc number come after every disc: on an album with no disc numbers at all,
-- that is all of them, in the same order a single disc has. Every query that
-- lists an album's tracks repeats this order, and store.Place states it in Go;
-- the album's first track by it gives the album its cover, year and compilation
-- flag (SettleAlbum in edits.sql).

-- name: ListAlbumTracks :many
-- With Kitsune's own loudness measure of each file, and of the album it was
-- pooled into: see migrations/0013_loudness.sql.
SELECT
    t.id, t.ulid, t.title, t.disc_no, t.disc_total, t.disc_subtitle, t.track_no, t.track_total, t.year,
    t.duration_ms, t.format, t.sample_rate, t.bit_depth,
    t.rg_track_gain, t.rg_track_peak, t.rg_album_gain, t.rg_album_peak,
    t.size, t.mtime,
    l.integrated AS measured_integrated, l.true_peak AS measured_peak,
    a.integrated AS measured_album_integrated, a.true_peak AS measured_album_peak
FROM tracks t
LEFT JOIN track_loudness l ON l.track_id = t.id AND l.size = t.size AND l.mtime = t.mtime AND l.outcome = 'measured'
LEFT JOIN album_loudness a ON a.id = l.album_pool
WHERE t.album_id = ? AND t.missing_since IS NULL
ORDER BY t.disc_no NULLS LAST, t.track_no NULLS LAST, t.title, t.id;

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
SELECT t.id, t.ulid, t.root, t.rel_path, t.size, t.mtime, t.album_id, a.group_key
FROM tracks t
JOIN albums a ON a.id = t.album_id
WHERE a.ulid = ? AND t.missing_since IS NULL
ORDER BY t.disc_no NULLS LAST, t.track_no NULLS LAST, t.title, t.id;

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

-- The library index, read whole: three queries for the whole library, never one per
-- album. An album's owner is its first one, by the test in artists.sql, repeated
-- here because a query can't share it. The order is orderIndex's business.

-- name: ListIndexAlbums :many
SELECT
    a.id, a.ulid, a.name, a.sort_name, a.artist, a.year, a.compilation,
    a.artwork_hash, w.width AS artwork_width, w.height AS artwork_height,
    w.accent AS artwork_accent, w.ground AS artwork_ground,
    ar.ulid AS owner_ulid, ar.name AS owner_name, ar.sort_name AS owner_sort_name,
    ar.match_key AS owner_match_key, ar.mbid AS owner_mbid
FROM albums a
LEFT JOIN artwork w ON w.hash = a.artwork_hash
LEFT JOIN artists ar ON ar.id = (
    SELECT ta.artist_id
    FROM tracks t
    JOIN track_artists ta ON ta.track_id = t.id
    WHERE t.album_id = a.id AND t.missing_since IS NULL
      AND (ta.role = 'album_artist'
       OR (ta.ord = 0 AND a.compilation = 0 AND NOT EXISTS (
            SELECT 1 FROM tracks o
            JOIN track_artists oa ON oa.track_id = o.id AND oa.role = 'album_artist'
            WHERE o.album_id = a.id AND o.missing_since IS NULL)))
    ORDER BY ta.ord, t.id
    LIMIT 1)
WHERE EXISTS (SELECT 1 FROM tracks t WHERE t.album_id = a.id AND t.missing_since IS NULL);

-- name: ListIndexTracks :many
SELECT id, album_id, ulid, title, disc_no, disc_subtitle, track_no, duration_ms
FROM tracks
WHERE missing_since IS NULL
ORDER BY album_id, disc_no NULLS LAST, track_no NULLS LAST, title, id;

-- name: ListIndexTrackArtists :many
SELECT ta.track_id, ar.name
FROM track_artists ta
JOIN artists ar ON ar.id = ta.artist_id
JOIN tracks t ON t.id = ta.track_id AND t.missing_since IS NULL
WHERE ta.role = 'artist'
ORDER BY ta.track_id, ta.ord;

-- What the song columns ask for, read only when they do: an index without them
-- stays as fast as it was.

-- name: ListIndexTrackDetails :many
SELECT id, year, format, sample_rate, bit_depth, bitrate, added_at, rg_track_gain
FROM tracks
WHERE missing_since IS NULL;

-- Kitsune's own measure of each live track with no gain tagged, as album pages
-- read it: of the file as it is now. Measures land while the server runs, by
-- the loudness pass, outside library writes, so this is read every time and
-- never kept with the library's rows.
-- name: ListIndexTrackMeasures :many
SELECT t.id, l.integrated
FROM track_loudness l
JOIN tracks t ON t.id = l.track_id AND t.size = l.size AND t.mtime = l.mtime
WHERE l.outcome = 'measured' AND l.integrated IS NOT NULL
  AND t.missing_since IS NULL AND t.rg_track_gain IS NULL AND t.rg_album_gain IS NULL;

-- name: ListIndexTrackAlbumArtists :many
SELECT ta.track_id, ar.name
FROM track_artists ta
JOIN artists ar ON ar.id = ta.artist_id
JOIN tracks t ON t.id = ta.track_id AND t.missing_since IS NULL
WHERE ta.role = 'album_artist'
ORDER BY ta.track_id, ta.ord;

-- name: ListIndexTrackTags :many
SELECT tt.track_id, tt.key, tt.value
FROM track_tags tt
JOIN tracks t ON t.id = tt.track_id AND t.missing_since IS NULL
WHERE tt.key IN (sqlc.slice(tag_keys))
ORDER BY tt.track_id, tt.key, tt.ord;

-- plays_recent leads with user_id, so this reads only the person's plays.
-- name: ListIndexPlays :many
SELECT track_id, count(*) AS plays, CAST(max(played_at) AS INTEGER) AS last_played
FROM plays
WHERE user_id = ?
GROUP BY track_id;


-- The same, for some albums only, by public ID: what a change to them needs.
-- Each has the columns of its whole-library twin above, in the same order. CROSS
-- JOIN fixes SQLite's join order from the albums out, so each reads the rows of
-- those few albums by index; left to choose, the planner starts from a person's
-- plays or a tag key and reads thousands.

-- name: ListIndexTracksOf :many
SELECT t.id, t.album_id, t.ulid, t.title, t.disc_no, t.disc_subtitle, t.track_no, t.duration_ms
FROM albums a
CROSS JOIN tracks t
WHERE a.ulid IN (sqlc.slice(albums)) AND t.album_id = a.id AND t.missing_since IS NULL
ORDER BY t.album_id, t.disc_no NULLS LAST, t.track_no NULLS LAST, t.title, t.id;

-- name: ListIndexTrackArtistsOf :many
SELECT ta.track_id, ar.name
FROM albums a
CROSS JOIN tracks t
CROSS JOIN track_artists ta
CROSS JOIN artists ar
WHERE a.ulid IN (sqlc.slice(albums)) AND t.album_id = a.id AND t.missing_since IS NULL
  AND ta.track_id = t.id AND ta.role = 'artist' AND ar.id = ta.artist_id
ORDER BY ta.track_id, ta.ord;

-- name: ListIndexTrackDetailsOf :many
SELECT t.id, t.year, t.format, t.sample_rate, t.bit_depth, t.bitrate, t.added_at, t.rg_track_gain
FROM albums a
CROSS JOIN tracks t
WHERE a.ulid IN (sqlc.slice(albums)) AND t.album_id = a.id AND t.missing_since IS NULL;

-- name: ListIndexTrackMeasuresOf :many
SELECT t.id, l.integrated
FROM albums a
CROSS JOIN tracks t
CROSS JOIN track_loudness l
WHERE a.ulid IN (sqlc.slice(albums)) AND t.album_id = a.id AND t.missing_since IS NULL
  AND t.rg_track_gain IS NULL AND t.rg_album_gain IS NULL
  AND l.track_id = t.id AND l.size = t.size AND l.mtime = t.mtime
  AND l.outcome = 'measured' AND l.integrated IS NOT NULL;

-- name: ListIndexTrackAlbumArtistsOf :many
SELECT ta.track_id, ar.name
FROM albums a
CROSS JOIN tracks t
CROSS JOIN track_artists ta
CROSS JOIN artists ar
WHERE a.ulid IN (sqlc.slice(albums)) AND t.album_id = a.id AND t.missing_since IS NULL
  AND ta.track_id = t.id AND ta.role = 'album_artist' AND ar.id = ta.artist_id
ORDER BY ta.track_id, ta.ord;

-- name: ListIndexTrackTagsOf :many
SELECT tt.track_id, tt.key, tt.value
FROM albums a
CROSS JOIN tracks t
CROSS JOIN track_tags tt
WHERE a.ulid IN (sqlc.slice(albums)) AND t.album_id = a.id AND t.missing_since IS NULL
  AND tt.track_id = t.id AND tt.key IN (sqlc.slice(tag_keys))
ORDER BY tt.track_id, tt.key, tt.ord;

-- name: ListIndexPlaysOf :many
SELECT p.track_id, count(*) AS plays, CAST(max(p.played_at) AS INTEGER) AS last_played
FROM albums a
CROSS JOIN tracks t
CROSS JOIN plays p INDEXED BY plays_track -- Else plays_recent: every play of the person, per track.
-- user_id comes first: sqlc numbers a named argument, which a slice before it would shift.
WHERE p.user_id = sqlc.arg(user_id) AND a.ulid IN (sqlc.slice(albums)) AND t.album_id = a.id AND p.track_id = t.id
GROUP BY p.track_id;

-- The same, for the songs one artist is on, as an artist or an album artist, by
-- the artist's row ID: the artist page's list, the artist shuffle scope's songs.
-- Each has the columns of its whole-library twin above, in the same order. The
-- songs come from track_artists_artist, and each track by its row ID.

-- name: ListIndexTracksByArtist :many
SELECT t.id, t.album_id, t.ulid, t.title, t.disc_no, t.disc_subtitle, t.track_no, t.duration_ms
FROM tracks t
WHERE t.id IN (SELECT s.track_id FROM track_artists s WHERE s.artist_id = sqlc.arg(artist_id))
  AND t.missing_since IS NULL
ORDER BY t.album_id, t.disc_no NULLS LAST, t.track_no NULLS LAST, t.title, t.id;

-- name: ListIndexTrackArtistsByArtist :many
SELECT ta.track_id, ar.name
FROM tracks t
CROSS JOIN track_artists ta
CROSS JOIN artists ar
WHERE t.id IN (SELECT s.track_id FROM track_artists s WHERE s.artist_id = sqlc.arg(artist_id))
  AND t.missing_since IS NULL AND ta.track_id = t.id AND ta.role = 'artist' AND ar.id = ta.artist_id
ORDER BY ta.track_id, ta.ord;

-- name: ListIndexTrackDetailsByArtist :many
SELECT t.id, t.year, t.format, t.sample_rate, t.bit_depth, t.bitrate, t.added_at, t.rg_track_gain
FROM tracks t
WHERE t.id IN (SELECT s.track_id FROM track_artists s WHERE s.artist_id = sqlc.arg(artist_id))
  AND t.missing_since IS NULL;

-- name: ListIndexTrackMeasuresByArtist :many
SELECT t.id, l.integrated
FROM tracks t
CROSS JOIN track_loudness l
WHERE t.id IN (SELECT s.track_id FROM track_artists s WHERE s.artist_id = sqlc.arg(artist_id))
  AND t.missing_since IS NULL AND t.rg_track_gain IS NULL AND t.rg_album_gain IS NULL
  AND l.track_id = t.id AND l.size = t.size AND l.mtime = t.mtime
  AND l.outcome = 'measured' AND l.integrated IS NOT NULL;

-- name: ListIndexTrackAlbumArtistsByArtist :many
SELECT ta.track_id, ar.name
FROM tracks t
CROSS JOIN track_artists ta
CROSS JOIN artists ar
WHERE t.id IN (SELECT s.track_id FROM track_artists s WHERE s.artist_id = sqlc.arg(artist_id))
  AND t.missing_since IS NULL AND ta.track_id = t.id AND ta.role = 'album_artist' AND ar.id = ta.artist_id
ORDER BY ta.track_id, ta.ord;

-- name: ListIndexTrackTagsByArtist :many
SELECT tt.track_id, tt.key, tt.value
FROM tracks t
CROSS JOIN track_tags tt
-- artist_id comes first: sqlc numbers a named argument, which a slice before it would shift.
WHERE t.id IN (SELECT s.track_id FROM track_artists s WHERE s.artist_id = sqlc.arg(artist_id))
  AND t.missing_since IS NULL AND tt.track_id = t.id AND tt.key IN (sqlc.slice(tag_keys))
ORDER BY tt.track_id, tt.key, tt.ord;

-- name: ListIndexPlaysByArtist :many
SELECT p.track_id, count(*) AS plays, CAST(max(p.played_at) AS INTEGER) AS last_played
FROM tracks t
CROSS JOIN plays p INDEXED BY plays_track -- Else plays_recent: every play of the person, per track.
WHERE p.user_id = sqlc.arg(user_id)
  AND t.id IN (SELECT s.track_id FROM track_artists s WHERE s.artist_id = sqlc.arg(artist_id))
  AND p.track_id = t.id
GROUP BY p.track_id;

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
