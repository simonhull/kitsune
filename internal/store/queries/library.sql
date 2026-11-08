-- name: UpsertArtist :one
-- A track with no sort name keeps the artist's: a new artist sorts by its name.
INSERT INTO artists (ulid, match_key, name, sort_name, mbid)
VALUES (@ulid, @match_key, @name, coalesce(nullif(CAST(@sort_name AS TEXT), ''), @name), @mbid)
ON CONFLICT (match_key) DO UPDATE SET
    name = excluded.name,
    sort_name = coalesce(nullif(?4, ''), artists.sort_name), -- ?4 is @sort_name: sqlc leaves names in an upsert clause unbound.
    mbid = coalesce(excluded.mbid, artists.mbid)
RETURNING id;

-- name: UpsertArtwork :exec
INSERT INTO artwork (hash, width, height, accent, ground, source)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (hash) DO UPDATE SET
    width = excluded.width,
    height = excluded.height,
    accent = excluded.accent,
    ground = excluded.ground,
    source = excluded.source;

-- name: UpsertAlbum :one
INSERT INTO albums (ulid, group_key, name, sort_name, artist, mbid, year, compilation, artwork_hash, added_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (group_key) DO UPDATE SET
    name = excluded.name,
    sort_name = excluded.sort_name,
    artist = excluded.artist,
    mbid = coalesce(excluded.mbid, albums.mbid),
    year = excluded.year,
    compilation = excluded.compilation,
    artwork_hash = coalesce(excluded.artwork_hash, albums.artwork_hash)
RETURNING id, ulid;

-- name: InsertTrack :one
INSERT INTO tracks (
    ulid, root, rel_path, size, mtime, audio_digest, album_id,
    format, codec, duration_ms, sample_rate, bit_depth, channels, bitrate, lossless,
    title, sort_title, track_no, track_total, disc_no, disc_total, year, original_year,
    compilation, rg_track_gain, rg_track_peak, rg_album_gain, rg_album_peak,
    scan_warnings, added_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?,
    ?, ?
)
RETURNING id;

-- name: UpdateTrack :execrows
UPDATE tracks SET
    root = sqlc.arg(root), rel_path = sqlc.arg(rel_path), size = sqlc.arg(size), mtime = sqlc.arg(mtime),
    -- A write without a fingerprint keeps the stored one only while the file is unchanged.
    audio_digest = CASE
        WHEN sqlc.narg(audio_digest) IS NOT NULL THEN sqlc.narg(audio_digest)
        WHEN tracks.size = sqlc.arg(size) AND tracks.mtime = sqlc.arg(mtime) THEN tracks.audio_digest
    END,
    album_id = sqlc.arg(album_id),
    format = sqlc.arg(format), codec = sqlc.arg(codec), duration_ms = sqlc.arg(duration_ms),
    sample_rate = sqlc.narg(sample_rate), bit_depth = sqlc.narg(bit_depth), channels = sqlc.narg(channels),
    bitrate = sqlc.narg(bitrate), lossless = sqlc.arg(lossless),
    title = sqlc.arg(title), sort_title = sqlc.arg(sort_title),
    track_no = sqlc.narg(track_no), track_total = sqlc.narg(track_total),
    disc_no = sqlc.narg(disc_no), disc_total = sqlc.narg(disc_total),
    year = sqlc.narg(year), original_year = sqlc.narg(original_year),
    compilation = sqlc.arg(compilation),
    rg_track_gain = sqlc.narg(rg_track_gain), rg_track_peak = sqlc.narg(rg_track_peak),
    rg_album_gain = sqlc.narg(rg_album_gain), rg_album_peak = sqlc.narg(rg_album_peak),
    scan_warnings = sqlc.arg(scan_warnings), missing_since = NULL
WHERE id = sqlc.arg(id);

-- name: DeleteTrackTags :exec
DELETE FROM track_tags WHERE track_id = ?;

-- name: DeleteTrackArtists :exec
DELETE FROM track_artists WHERE track_id = ?;

-- name: InsertTrackTag :exec
INSERT INTO track_tags (track_id, key, ord, value) VALUES (?, ?, ?, ?);

-- name: InsertTrackArtist :exec
INSERT INTO track_artists (track_id, role, ord, artist_id) VALUES (?, ?, ?, ?);

-- name: DeleteScanIssue :exec
DELETE FROM scan_issues WHERE root = ? AND rel_path = ?;

-- name: UpsertScanIssue :exec
INSERT INTO scan_issues (root, rel_path, kind, message, size, mtime, seen_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (root, rel_path) DO UPDATE SET
    kind = excluded.kind,
    message = excluded.message,
    size = excluded.size,
    mtime = excluded.mtime,
    seen_at = excluded.seen_at;

-- name: TrackStates :many
SELECT id, rel_path, size, mtime, missing_since IS NOT NULL AS missing
FROM tracks
WHERE root = ?;

-- name: IssueStates :many
SELECT rel_path, size, mtime
FROM scan_issues
WHERE root = ?;

-- name: MissingByDigest :many
SELECT id, root, rel_path
FROM tracks
WHERE missing_since IS NOT NULL AND audio_digest = ?
ORDER BY id;

-- name: AnyMissingWithDigest :one
SELECT EXISTS (SELECT 1 FROM tracks WHERE missing_since IS NOT NULL AND audio_digest IS NOT NULL) AS found;

-- name: Undigested :many
SELECT id, root, rel_path, size, mtime
FROM tracks
WHERE audio_digest IS NULL AND missing_since IS NULL AND (root, rel_path) > (sqlc.arg(root), sqlc.arg(rel_path))
ORDER BY root, rel_path
LIMIT sqlc.arg(n);

-- name: CountUndigested :one
SELECT count(*) FROM tracks INDEXED BY tracks_undigested WHERE audio_digest IS NULL AND missing_since IS NULL;

-- name: SetDigest :execrows
UPDATE tracks SET audio_digest = sqlc.arg(audio_digest)
WHERE id = sqlc.arg(id) AND size = sqlc.arg(size) AND mtime = sqlc.arg(mtime) AND missing_since IS NULL;

-- name: TrackAlbumULID :one
SELECT a.ulid FROM tracks t JOIN albums a ON a.id = t.album_id WHERE t.id = ?;

-- name: MarkMissing :exec
UPDATE tracks SET missing_since = ? WHERE id = ? AND missing_since IS NULL;

-- name: DeleteMissingBefore :execrows
DELETE FROM tracks WHERE missing_since < ?;

-- name: PruneAlbums :execrows
DELETE FROM albums WHERE NOT EXISTS (SELECT 1 FROM tracks WHERE tracks.album_id = albums.id);

-- name: PruneArtwork :many
DELETE FROM artwork WHERE NOT EXISTS (SELECT 1 FROM albums WHERE albums.artwork_hash = artwork.hash)
RETURNING hash;

-- name: PruneArtists :execrows
DELETE FROM artists WHERE NOT EXISTS (SELECT 1 FROM track_artists WHERE track_artists.artist_id = artists.id);

-- name: KnownRoots :many
SELECT root FROM tracks UNION SELECT root FROM scan_issues;
