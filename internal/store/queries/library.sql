-- name: UpsertArtist :one
INSERT INTO artists (match_key, name, sort_name, mbid)
VALUES (?, ?, ?, ?)
ON CONFLICT (match_key) DO UPDATE SET
    name = excluded.name,
    sort_name = excluded.sort_name,
    mbid = coalesce(excluded.mbid, artists.mbid)
RETURNING id;

-- name: UpsertAlbum :one
INSERT INTO albums (group_key, name, sort_name, mbid, year, compilation)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (group_key) DO UPDATE SET
    name = excluded.name,
    sort_name = excluded.sort_name,
    mbid = coalesce(excluded.mbid, albums.mbid),
    year = excluded.year,
    compilation = excluded.compilation
RETURNING id;

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
    root = ?, rel_path = ?, size = ?, mtime = ?, audio_digest = ?, album_id = ?,
    format = ?, codec = ?, duration_ms = ?, sample_rate = ?, bit_depth = ?, channels = ?,
    bitrate = ?, lossless = ?,
    title = ?, sort_title = ?, track_no = ?, track_total = ?, disc_no = ?, disc_total = ?,
    year = ?, original_year = ?,
    compilation = ?, rg_track_gain = ?, rg_track_peak = ?, rg_album_gain = ?, rg_album_peak = ?,
    scan_warnings = ?, missing_since = NULL
WHERE id = ?;

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

-- name: MarkMissing :exec
UPDATE tracks SET missing_since = ? WHERE id = ? AND missing_since IS NULL;

-- name: DeleteMissingBefore :execrows
DELETE FROM tracks WHERE missing_since < ?;

-- name: PruneAlbums :execrows
DELETE FROM albums WHERE NOT EXISTS (SELECT 1 FROM tracks WHERE tracks.album_id = albums.id);

-- name: PruneArtists :execrows
DELETE FROM artists WHERE NOT EXISTS (SELECT 1 FROM track_artists WHERE track_artists.artist_id = artists.id);

-- name: KnownRoots :many
SELECT root FROM tracks UNION SELECT root FROM scan_issues;
