-- Choose artwork (plan 072, Phase 4): an administrator's pins, Not this
-- artist's holds, and the previews the picker shows.

-- name: UpsertArtistPin :exec
INSERT INTO artist_pins (artist_id, kind, artwork_hash, source, remote_id, credit, pinned_by, pinned_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (artist_id, kind) DO UPDATE SET
    artwork_hash = excluded.artwork_hash,
    source = excluded.source,
    remote_id = excluded.remote_id,
    credit = excluded.credit,
    pinned_by = excluded.pinned_by,
    pinned_at = excluded.pinned_at;

-- name: DeleteArtistPin :execrows
DELETE FROM artist_pins WHERE artist_id = ? AND kind = ?;

-- name: GetArtistPin :one
-- The artist's pin of a kind, with who pinned it while their account stands.
SELECT p.artwork_hash, p.source, p.remote_id, p.credit, p.pinned_at, u.name AS pinned_by,
    w.width, w.height, w.accent, w.ground, w.alpha
FROM artist_pins p
JOIN artwork w ON w.hash = p.artwork_hash
LEFT JOIN users u ON u.id = p.pinned_by
WHERE p.artist_id = ? AND p.kind = ?;

-- name: GetArtistImageBeneath :one
-- What the folders or the sources give the artist of a kind, beneath any pin.
SELECT ai.artwork_hash, ai.source, ai.remote_id, ai.credit, w.width, w.height, w.accent, w.ground, w.alpha
FROM artist_images ai
JOIN artwork w ON w.hash = ai.artwork_hash
WHERE ai.artist_id = ? AND ai.kind = ?;

-- name: GetArtistHold :one
SELECT mbid FROM artist_holds WHERE artist_id = ?;

-- name: UpsertArtistHold :exec
INSERT INTO artist_holds (artist_id, mbid, held_by, held_at) VALUES (?, ?, ?, ?)
ON CONFLICT (artist_id) DO UPDATE SET
    mbid = excluded.mbid,
    held_by = excluded.held_by,
    held_at = excluded.held_at;

-- name: DeleteFetchedArtistImages :execrows
-- What the sources gave the artist: never a folder's.
DELETE FROM artist_images WHERE artist_id = ? AND source NOT IN ('folder', 'pinned');

-- name: DeleteFetchedArtistPins :execrows
-- The artist's pins of what a source gave: never an upload or a folder's.
DELETE FROM artist_pins WHERE artist_id = ? AND source IN ('fanart', 'wikimedia');

-- name: DeleteFetchedBio :execrows
-- The bio a source gave the artist: never an artist.nfo's.
DELETE FROM artist_bios WHERE artist_id = ? AND source <> 'nfo';

-- name: GetArtworkPreview :one
SELECT hash, width, height, accent, ground, alpha
FROM artwork_previews
WHERE source = ? AND remote_id = ? AND made_at >= sqlc.arg(not_before);

-- name: UpsertArtworkPreview :exec
INSERT INTO artwork_previews (source, remote_id, hash, width, height, accent, ground, alpha, made_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (source, remote_id) DO UPDATE SET
    hash = excluded.hash,
    width = excluded.width,
    height = excluded.height,
    accent = excluded.accent,
    ground = excluded.ground,
    alpha = excluded.alpha,
    made_at = excluded.made_at;

-- name: DeleteOldPreviews :many
-- Previews made before a time; the caller removes the files of those hashes
-- nothing else uses (HashInUse).
DELETE FROM artwork_previews WHERE made_at < ? RETURNING hash;

-- name: HashInUse :one
-- Whether anything still uses an image's hash: an artwork row, or a preview.
SELECT CAST(EXISTS (SELECT 1 FROM artwork w WHERE w.hash = sqlc.arg(image))
    OR EXISTS (SELECT 1 FROM artwork_previews pv WHERE pv.hash = sqlc.arg(image)) AS INTEGER) AS used;
