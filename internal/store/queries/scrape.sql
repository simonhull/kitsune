-- The artist pass (plan 072, Phase 2): which artists it may ask about, what
-- each source last said, and what it chose of their images and bios.

-- name: ListScrapeArtists :many
-- Every artist with a MusicBrainz ID and a live track.
SELECT ar.id, ar.ulid, ar.name, ar.match_key, ar.mbid
FROM artists ar
WHERE ar.mbid IS NOT NULL
  AND EXISTS (
    SELECT 1 FROM track_artists ta
    JOIN tracks t ON t.id = ta.track_id
    WHERE ta.artist_id = ar.id AND t.missing_since IS NULL);

-- name: GetScrapeArtist :one
-- One artist by its public ID, as ListScrapeArtists has it, its MBID maybe NULL.
SELECT ar.id, ar.ulid, ar.name, ar.match_key, ar.mbid FROM artists ar WHERE ar.ulid = ?;

-- name: ListOwnersByPlays :many
-- Every artist who owns an album, owners as queries/artists.sql has them, with
-- how often everyone played the live tracks of those albums.
SELECT ta.artist_id, CAST(count(p.track_id) AS INTEGER) AS plays
FROM track_artists ta
JOIN tracks t ON t.id = ta.track_id AND t.missing_since IS NULL
JOIN albums a ON a.id = t.album_id
LEFT JOIN plays p ON p.track_id = t.id
WHERE ta.role = 'album_artist'
   OR (ta.ord = 0 AND a.compilation = 0 AND NOT EXISTS (
        SELECT 1 FROM tracks o
        JOIN track_artists oa ON oa.track_id = o.id AND oa.role = 'album_artist'
        WHERE o.album_id = a.id AND o.missing_since IS NULL))
GROUP BY ta.artist_id;

-- name: ListScrapeStates :many
SELECT source, mbid, status, fetched_at, next_at, failures, error
FROM scrape_state WHERE kind = 'artist';

-- name: ListScrapeStatesFor :many
SELECT source, mbid, status, fetched_at, next_at, failures, error
FROM scrape_state WHERE kind = 'artist' AND mbid = ?;

-- name: UpsertScrapeState :exec
INSERT INTO scrape_state (source, kind, mbid, status, fetched_at, next_at, failures, error)
VALUES (?, 'artist', ?, ?, ?, ?, ?, ?)
ON CONFLICT (source, kind, mbid) DO UPDATE SET
    status = excluded.status,
    fetched_at = excluded.fetched_at,
    next_at = excluded.next_at,
    failures = excluded.failures,
    error = excluded.error;

-- name: AskAgain :execrows
-- Brings the next ask of every source about these artists forward to now.
UPDATE scrape_state SET next_at = sqlc.arg(now)
WHERE kind = 'artist' AND next_at > sqlc.arg(now)
  AND source IN (SELECT value FROM json_each(sqlc.arg(sources)))
  AND mbid IN (SELECT value FROM json_each(sqlc.arg(mbids)));

-- name: AskAgainForArtist :exec
-- Brings the next ask of every source about the artist forward to now: its
-- folder gave up an image or a bio a source may have.
UPDATE scrape_state SET next_at = sqlc.arg(now)
WHERE kind = 'artist' AND next_at > sqlc.arg(now)
  AND mbid = (SELECT ar.mbid FROM artists ar WHERE ar.id = sqlc.arg(artist_id));

-- name: GetScrapeFeed :one
SELECT read_at FROM scrape_feeds WHERE source = ?;

-- name: SetScrapeFeed :exec
INSERT INTO scrape_feeds (source, read_at) VALUES (?, ?)
ON CONFLICT (source) DO UPDATE SET read_at = excluded.read_at;

-- name: ListHeldImages :many
-- What an artist holds of each kind: where it came from and the source's ID.
SELECT kind, source, remote_id, artwork_hash FROM artist_images WHERE artist_id = ?;

-- name: GetHeldBio :one
SELECT source, url, lang FROM artist_bios WHERE artist_id = ?;

-- name: UpsertRemoteArtistImage :execrows
-- A source's image replaces another source's, never a folder's or a pin.
INSERT INTO artist_images (artist_id, kind, artwork_hash, source, remote_id, credit, found_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (artist_id, kind) DO UPDATE SET
    artwork_hash = excluded.artwork_hash,
    source = excluded.source,
    remote_id = excluded.remote_id,
    credit = excluded.credit,
    found_at = excluded.found_at
WHERE artist_images.source NOT IN ('folder', 'pinned')
  AND (artist_images.artwork_hash <> excluded.artwork_hash
    OR artist_images.source <> excluded.source
    OR artist_images.remote_id IS NOT excluded.remote_id
    OR artist_images.credit IS NOT excluded.credit);

-- name: DeleteRemoteArtistImage :execrows
DELETE FROM artist_images WHERE artist_id = ? AND kind = ? AND source = ?;

-- name: UpsertRemoteBio :execrows
-- A source's bio replaces another source's, never an artist.nfo's.
INSERT INTO artist_bios (artist_id, source, lang, summary, body, url, license, license_url, fetched_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (artist_id) DO UPDATE SET
    source = excluded.source,
    lang = excluded.lang,
    summary = excluded.summary,
    body = excluded.body,
    url = excluded.url,
    license = excluded.license,
    license_url = excluded.license_url,
    fetched_at = excluded.fetched_at
WHERE artist_bios.source <> 'nfo'
  AND (artist_bios.source <> excluded.source
    OR artist_bios.lang IS NOT excluded.lang
    OR artist_bios.summary <> excluded.summary
    OR artist_bios.body <> excluded.body
    OR artist_bios.url IS NOT excluded.url
    OR artist_bios.license IS NOT excluded.license
    OR artist_bios.license_url IS NOT excluded.license_url);

-- name: DeleteRemoteBio :execrows
DELETE FROM artist_bios WHERE artist_id = ? AND source = ?;

-- name: ListArtedArtists :many
-- Every artist with an image or a bio.
SELECT artist_id FROM artist_images
UNION SELECT artist_id FROM artist_bios;
