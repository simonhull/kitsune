-- An album's owners are its album artists. An album with none belongs to the
-- first artist of its tracks, the same on all of them because grouping uses it,
-- unless it is a compilation, which belongs to nobody. Only live tracks count.
-- This test is repeated wherever owners matter: a query can't share it.

-- name: ListOwningArtists :many
SELECT ar.id, ar.ulid, ar.name, ar.match_key, ar.mbid, count(DISTINCT t.album_id) AS album_count,
    CAST(max(a.added_at) AS INTEGER) AS added_at
FROM track_artists ta
JOIN tracks t ON t.id = ta.track_id AND t.missing_since IS NULL
JOIN albums a ON a.id = t.album_id
JOIN artists ar ON ar.id = ta.artist_id
WHERE ta.role = 'album_artist'
   OR (ta.ord = 0 AND a.compilation = 0 AND NOT EXISTS (
        SELECT 1 FROM tracks o
        JOIN track_artists oa ON oa.track_id = o.id AND oa.role = 'album_artist'
        WHERE o.album_id = a.id AND o.missing_since IS NULL))
GROUP BY ar.id
ORDER BY ar.sort_name COLLATE NOCASE, ar.id;

-- name: ListOwningArtistPlays :many
-- How often the person played the live tracks of the albums each artist owns,
-- owners as ListOwningArtists has them. An artist with none isn't listed.
SELECT ta.artist_id, CAST(sum(p.plays) AS INTEGER) AS plays
FROM (SELECT track_id, count(*) AS plays FROM plays WHERE user_id = ? GROUP BY track_id) AS p
JOIN tracks t ON t.id = p.track_id AND t.missing_since IS NULL
JOIN albums a ON a.id = t.album_id
JOIN track_artists ta ON ta.track_id = t.id
WHERE ta.role = 'album_artist'
   OR (ta.ord = 0 AND a.compilation = 0 AND NOT EXISTS (
        SELECT 1 FROM tracks o
        JOIN track_artists oa ON oa.track_id = o.id AND oa.role = 'album_artist'
        WHERE o.album_id = a.id AND o.missing_since IS NULL))
GROUP BY ta.artist_id;

-- name: GetArtist :one
SELECT ar.id, ar.ulid, ar.name, ar.sort_name, ar.match_key, ar.mbid
FROM artists ar
WHERE ar.ulid = ?
  AND EXISTS (
    SELECT 1 FROM track_artists ta
    JOIN tracks t ON t.id = ta.track_id
    WHERE ta.artist_id = ar.id AND t.missing_since IS NULL
);

-- name: CountArtistTracks :one
-- The live songs an artist is on, as an artist or an album artist.
SELECT count(*) FROM tracks t
WHERE t.id IN (SELECT s.track_id FROM track_artists s WHERE s.artist_id = ?)
  AND t.missing_since IS NULL;

-- name: ListArtistAlbums :many
SELECT
    a.id, a.ulid, a.name, a.sort_name, a.artist, a.year, a.compilation, a.added_at,
    a.artwork_hash, w.width AS artwork_width, w.height AS artwork_height,
    w.accent AS artwork_accent, w.ground AS artwork_ground,
    (SELECT count(*) FROM tracks l WHERE l.album_id = a.id AND l.missing_since IS NULL) AS track_count,
    CAST(max(ta.role = 'album_artist' OR (ta.ord = 0 AND a.compilation = 0 AND NOT EXISTS (
        SELECT 1 FROM tracks o
        JOIN track_artists oa ON oa.track_id = o.id AND oa.role = 'album_artist'
        WHERE o.album_id = a.id AND o.missing_since IS NULL))) AS INTEGER) AS owned
FROM track_artists ta
JOIN tracks t ON t.id = ta.track_id AND t.missing_since IS NULL
JOIN albums a ON a.id = t.album_id
LEFT JOIN artwork w ON w.hash = a.artwork_hash
WHERE ta.artist_id = ?
GROUP BY a.id
ORDER BY a.compilation, a.year DESC NULLS LAST, a.sort_name, a.id;

-- name: ListAlbumOwners :many
SELECT ar.ulid, ar.name, ar.match_key, ar.mbid
FROM tracks t
JOIN track_artists ta ON ta.track_id = t.id
JOIN albums a ON a.id = t.album_id
JOIN artists ar ON ar.id = ta.artist_id
WHERE t.album_id = ? AND t.missing_since IS NULL
  AND (ta.role = 'album_artist'
   OR (ta.ord = 0 AND a.compilation = 0 AND NOT EXISTS (
        SELECT 1 FROM tracks o
        JOIN track_artists oa ON oa.track_id = o.id AND oa.role = 'album_artist'
        WHERE o.album_id = a.id AND o.missing_since IS NULL)))
GROUP BY ar.id
ORDER BY min(ta.ord), min(t.id);

-- ids is a JSON array of album ULIDs. An album's sort artist is its first owner's
-- sort name; an album nobody owns sorts by the artist it displays.

-- name: ListAlbumSortArtists :many
SELECT a.ulid, CAST(coalesce((
    SELECT ar.sort_name
    FROM tracks t
    JOIN track_artists ta ON ta.track_id = t.id
    JOIN artists ar ON ar.id = ta.artist_id
    WHERE t.album_id = a.id AND t.missing_since IS NULL
      AND (ta.role = 'album_artist'
       OR (ta.ord = 0 AND a.compilation = 0 AND NOT EXISTS (
            SELECT 1 FROM tracks o
            JOIN track_artists oa ON oa.track_id = o.id AND oa.role = 'album_artist'
            WHERE o.album_id = a.id AND o.missing_since IS NULL)))
    ORDER BY ta.ord, t.id
    LIMIT 1), a.artist) AS TEXT) AS sort_artist
FROM albums a
WHERE a.ulid IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));
