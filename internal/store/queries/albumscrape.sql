-- The album pass (plan 072, Phase 3): which albums it may ask about, what each
-- source last said of them, and what it chose of their covers and extras.

-- name: ListScrapeAlbums :many
-- Every album with a live track: its IDs, its cover, and the Cover Art
-- Archive's front it holds as that cover, if any.
SELECT a.id, a.ulid, a.name, a.artist, a.mbid, a.release_group_mbid, a.artwork_hash,
       c.artwork_hash AS archive_hash
FROM albums a
LEFT JOIN album_images c ON c.album_id = a.id AND c.kind = 'cover' AND c.disc = 0
WHERE EXISTS (SELECT 1 FROM tracks t WHERE t.album_id = a.id AND t.missing_since IS NULL);

-- name: ListAlbumPlays :many
-- How often everyone played each album's live tracks.
SELECT t.album_id, CAST(count(*) AS INTEGER) AS plays
FROM plays p
JOIN tracks t ON t.id = p.track_id AND t.missing_since IS NULL
GROUP BY t.album_id;

-- name: ListAlbumDiscs :many
-- Each album's discs, by its live tracks: a track with no disc number is on
-- the first.
SELECT DISTINCT album_id, CAST(coalesce(disc_no, 1) AS INTEGER) AS disc
FROM tracks
WHERE missing_since IS NULL;

-- name: ListHeldAlbumImages :many
-- What every album holds of its extras: where each came from and the source's ID.
SELECT album_id, kind, disc, source, remote_id FROM album_images;

-- name: ListAlbumScrapeStates :many
SELECT source, mbid, status, fetched_at, next_at, failures, error
FROM scrape_state WHERE kind = 'album';

-- name: UpsertAlbumScrapeState :exec
INSERT INTO scrape_state (source, kind, mbid, status, fetched_at, next_at, failures, error)
VALUES (?, 'album', ?, ?, ?, ?, ?, ?)
ON CONFLICT (source, kind, mbid) DO UPDATE SET
    status = excluded.status,
    fetched_at = excluded.fetched_at,
    next_at = excluded.next_at,
    failures = excluded.failures,
    error = excluded.error;

-- name: AskAgainForAlbum :exec
-- Brings the next ask of every source about the album forward to now: its
-- folder gave up an extra a source may have.
UPDATE scrape_state SET next_at = sqlc.arg(now)
WHERE kind = 'album' AND next_at > sqlc.arg(now)
  AND (mbid = (SELECT a.mbid FROM albums a WHERE a.id = sqlc.arg(album_id))
    OR mbid = (SELECT a.release_group_mbid FROM albums a WHERE a.id = sqlc.arg(album_id)));

-- name: SetLookedUpReleaseGroup :execrows
-- A release group looked up from the album's release, never over one the files
-- give.
UPDATE albums SET release_group_mbid = sqlc.arg(mbid)
WHERE id = sqlc.arg(id) AND release_group_mbid IS NULL;

-- name: GetAlbumCover :one
-- An album's cover, and the Cover Art Archive's front it holds, if any.
SELECT a.ulid, a.artwork_hash, c.artwork_hash AS archive_hash
FROM albums a
LEFT JOIN album_images c ON c.album_id = a.id AND c.kind = 'cover' AND c.disc = 0
WHERE a.id = ?;

-- name: SetArchiveCover :execrows
-- Makes hash (or none) the album's cover, only while it is still was: a scan
-- that found one in the files since wins.
UPDATE albums SET artwork_hash = sqlc.narg(hash)
WHERE id = sqlc.arg(id) AND artwork_hash IS sqlc.narg(was);

-- name: UpsertRemoteAlbumImage :execrows
-- A source's image replaces another source's, never a folder's.
INSERT INTO album_images (album_id, kind, disc, artwork_hash, source, remote_id, credit, found_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (album_id, kind, disc) DO UPDATE SET
    artwork_hash = excluded.artwork_hash,
    source = excluded.source,
    remote_id = excluded.remote_id,
    credit = excluded.credit,
    found_at = excluded.found_at
WHERE album_images.source <> 'folder'
  AND (album_images.artwork_hash <> excluded.artwork_hash
    OR album_images.source <> excluded.source
    OR album_images.remote_id IS NOT excluded.remote_id
    OR album_images.credit IS NOT excluded.credit);

-- name: DeleteRemoteAlbumImage :execrows
DELETE FROM album_images WHERE album_id = ? AND kind = ? AND disc = ? AND source = ?;

-- name: DeleteRemoteDiscArt :execrows
-- A folder's disc art puts every source's disc art of the album aside.
DELETE FROM album_images WHERE album_id = ? AND kind = 'disc' AND source <> 'folder';

-- name: CountScrapeAlbums :one
-- The library's albums, for Health's card: all with a live track, those with
-- a release MBID, those whose cover is the Cover Art Archive's, and those
-- with no cover at all.
SELECT CAST(count(*) AS INTEGER) AS albums,
       CAST(count(a.mbid) AS INTEGER) AS with_mbid,
       CAST(count(c.album_id) AS INTEGER) AS from_archive,
       CAST(coalesce(sum(a.artwork_hash IS NULL), 0) AS INTEGER) AS without_cover
FROM albums a
LEFT JOIN album_images c ON c.album_id = a.id AND c.kind = 'cover' AND c.disc = 0
WHERE EXISTS (SELECT 1 FROM tracks t WHERE t.album_id = a.id AND t.missing_since IS NULL);
