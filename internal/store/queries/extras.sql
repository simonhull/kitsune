-- Artist images, bios and album extras: what a scan reads from the library's
-- folders, and what the artist and album pages show of them.

-- name: LayoutTracks :many
-- Every live track's file, by album: where a scan looks for album and artist
-- folders.
SELECT t.album_id, a.ulid AS album_ulid, t.root, t.rel_path
FROM tracks t
JOIN albums a ON a.id = t.album_id
WHERE t.missing_since IS NULL
ORDER BY t.album_id, t.root, t.rel_path;

-- name: LayoutOwners :many
-- Every album's owners, as queries/artists.sql spells them.
SELECT DISTINCT t.album_id, ar.id AS artist_id, ar.ulid, ar.match_key, ar.mbid
FROM tracks t
JOIN track_artists ta ON ta.track_id = t.id
JOIN albums a ON a.id = t.album_id
JOIN artists ar ON ar.id = ta.artist_id
WHERE t.missing_since IS NULL
  AND (ta.role = 'album_artist'
   OR (ta.ord = 0 AND a.compilation = 0 AND NOT EXISTS (
        SELECT 1 FROM tracks o
        JOIN track_artists oa ON oa.track_id = o.id AND oa.role = 'album_artist'
        WHERE o.album_id = a.id AND o.missing_since IS NULL)))
ORDER BY t.album_id, ar.id;

-- name: StoredArtistImages :many
SELECT artist_id, kind, artwork_hash, source, file_root, file_path, file_size, file_mtime FROM artist_images;

-- name: StoredAlbumImages :many
SELECT album_id, kind, disc, artwork_hash, source, file_root, file_path, file_size, file_mtime FROM album_images;

-- name: StoredBios :many
SELECT artist_id, source, file_root, file_path, file_size, file_mtime FROM artist_bios;

-- name: UpsertFolderArtistImage :execrows
-- A folder image replaces whatever the artist had of the kind, but a pin.
INSERT INTO artist_images (artist_id, kind, artwork_hash, source, file_root, file_path, file_size, file_mtime, found_at)
VALUES (?, ?, ?, 'folder', ?, ?, ?, ?, ?)
ON CONFLICT (artist_id, kind) DO UPDATE SET
    artwork_hash = excluded.artwork_hash,
    source = 'folder',
    remote_id = NULL,
    credit = NULL,
    file_root = excluded.file_root,
    file_path = excluded.file_path,
    file_size = excluded.file_size,
    file_mtime = excluded.file_mtime,
    found_at = excluded.found_at
WHERE artist_images.source <> 'pinned';

-- name: DeleteFolderArtistImage :execrows
DELETE FROM artist_images WHERE artist_id = ? AND kind = ? AND source = 'folder';

-- name: UpsertFolderAlbumImage :execrows
INSERT INTO album_images (album_id, kind, disc, artwork_hash, source, file_root, file_path, file_size, file_mtime, found_at)
VALUES (?, ?, ?, ?, 'folder', ?, ?, ?, ?, ?)
ON CONFLICT (album_id, kind, disc) DO UPDATE SET
    artwork_hash = excluded.artwork_hash,
    source = 'folder',
    credit = NULL,
    file_root = excluded.file_root,
    file_path = excluded.file_path,
    file_size = excluded.file_size,
    file_mtime = excluded.file_mtime,
    found_at = excluded.found_at;

-- name: DeleteFolderAlbumImage :execrows
DELETE FROM album_images WHERE album_id = ? AND kind = ? AND disc = ? AND source = 'folder';

-- name: UpsertNfoBio :execrows
-- An artist.nfo's bio replaces whatever the artist had: it is the person's own.
INSERT INTO artist_bios (artist_id, source, summary, body, file_root, file_path, file_size, file_mtime, fetched_at)
VALUES (?, 'nfo', ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (artist_id) DO UPDATE SET
    source = 'nfo',
    lang = NULL,
    summary = excluded.summary,
    body = excluded.body,
    url = NULL,
    license = NULL,
    license_url = NULL,
    file_root = excluded.file_root,
    file_path = excluded.file_path,
    file_size = excluded.file_size,
    file_mtime = excluded.file_mtime,
    fetched_at = excluded.fetched_at;

-- name: DeleteNfoBio :execrows
DELETE FROM artist_bios WHERE artist_id = ? AND source = 'nfo';

-- name: ListArtistImages :many
SELECT ai.kind, ai.source, ai.credit, w.hash, w.width, w.height, w.accent, w.ground, w.alpha
FROM artist_images ai
JOIN artwork w ON w.hash = ai.artwork_hash
WHERE ai.artist_id = ?
ORDER BY ai.kind;

-- name: GetArtistBio :one
SELECT source, lang, summary, body, url, license, license_url FROM artist_bios WHERE artist_id = ?;

-- name: ListArtistThumbs :many
SELECT ai.artist_id, w.hash, w.width, w.height, w.accent, w.ground, w.alpha
FROM artist_images ai
JOIN artwork w ON w.hash = ai.artwork_hash
WHERE ai.kind = 'thumb';

-- name: ListAlbumImages :many
SELECT ai.kind, ai.disc, ai.source, ai.credit, w.hash, w.width, w.height, w.accent, w.ground, w.alpha
FROM album_images ai
JOIN artwork w ON w.hash = ai.artwork_hash
WHERE ai.album_id = ?
ORDER BY ai.kind, ai.disc;

-- name: ListExtraFiles :many
-- Every file a folder's image or an artist.nfo's bio was read from.
SELECT file_root, file_path FROM artist_images WHERE file_path IS NOT NULL
UNION SELECT file_root, file_path FROM album_images WHERE file_path IS NOT NULL
UNION SELECT file_root, file_path FROM artist_bios WHERE file_path IS NOT NULL;
