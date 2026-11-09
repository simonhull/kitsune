-- Kitsune's own loudness measurements: see migrations/0013_loudness.sql.
-- A track is waiting when its album has a song with no ReplayGain and the
-- track has no row for its file at the measurement version.

-- name: CountLoudnessWaiting :one
SELECT count(*)
FROM tracks t
WHERE t.missing_since IS NULL
  AND t.album_id IN (SELECT u.album_id FROM tracks u INDEXED BY tracks_ungained
                     WHERE u.missing_since IS NULL AND u.rg_track_gain IS NULL AND u.rg_album_gain IS NULL)
  AND NOT EXISTS (SELECT 1 FROM track_loudness l
                  WHERE l.track_id = t.id AND l.size = t.size AND l.mtime = t.mtime AND l.version = sqlc.arg(version));

-- name: LoudnessWaiting :many
SELECT t.id, t.root, t.rel_path, t.size, t.mtime, t.album_id, t.format, t.codec, t.channels
FROM tracks t
WHERE t.missing_since IS NULL
  AND (t.root, t.rel_path) > (sqlc.arg(root), sqlc.arg(rel_path))
  AND t.album_id IN (SELECT u.album_id FROM tracks u INDEXED BY tracks_ungained
                     WHERE u.missing_since IS NULL AND u.rg_track_gain IS NULL AND u.rg_album_gain IS NULL)
  AND NOT EXISTS (SELECT 1 FROM track_loudness l
                  WHERE l.track_id = t.id AND l.size = t.size AND l.mtime = t.mtime AND l.version = sqlc.arg(version))
ORDER BY t.root, t.rel_path
LIMIT sqlc.arg(n);

-- name: SetTrackLoudness :execrows
-- Writes only while the track is live and still the file that was read.
INSERT INTO track_loudness (track_id, size, mtime, version, outcome, integrated, true_peak, sample_peak, blocks, album_pool)
SELECT t.id, t.size, t.mtime, sqlc.arg(version), sqlc.arg(outcome), sqlc.narg(integrated), sqlc.narg(true_peak),
       sqlc.narg(sample_peak), sqlc.narg(blocks), NULL
FROM tracks t
WHERE t.id = sqlc.arg(id) AND t.size = sqlc.arg(size) AND t.mtime = sqlc.arg(mtime) AND t.missing_since IS NULL
ON CONFLICT (track_id) DO UPDATE SET
    size = excluded.size, mtime = excluded.mtime, version = excluded.version, outcome = excluded.outcome,
    integrated = excluded.integrated, true_peak = excluded.true_peak, sample_peak = excluded.sample_peak,
    blocks = excluded.blocks, album_pool = NULL;

-- name: AlbumsToPool :many
-- Albums with a song that has no ReplayGain, every live track measured or
-- found unmeasurable for its file, and no pooled measure of just those tracks.
SELECT DISTINCT u.album_id
FROM tracks u INDEXED BY tracks_ungained
WHERE u.missing_since IS NULL AND u.rg_track_gain IS NULL AND u.rg_album_gain IS NULL
  AND NOT EXISTS (SELECT 1 FROM tracks t
                  LEFT JOIN track_loudness l ON l.track_id = t.id AND l.size = t.size AND l.mtime = t.mtime
                  WHERE t.album_id = u.album_id AND t.missing_since IS NULL AND l.track_id IS NULL)
  AND (NOT EXISTS (SELECT 1 FROM album_loudness a WHERE a.album_id = u.album_id)
       OR EXISTS (SELECT 1 FROM tracks t
                  JOIN album_loudness a ON a.album_id = t.album_id
                  LEFT JOIN track_loudness l ON l.track_id = t.id
                  WHERE t.album_id = u.album_id AND t.missing_since IS NULL AND l.album_pool IS NOT a.id)
       OR (SELECT a.tracks FROM album_loudness a WHERE a.album_id = u.album_id)
          != (SELECT count(*) FROM tracks t WHERE t.album_id = u.album_id AND t.missing_since IS NULL))
ORDER BY u.album_id;

-- name: AlbumLoudnessInputs :many
-- Every live track of an album with its loudness row for its file, if any.
SELECT t.id, l.track_id AS measured_id, l.outcome, l.true_peak, l.blocks
FROM tracks t
LEFT JOIN track_loudness l ON l.track_id = t.id AND l.size = t.size AND l.mtime = t.mtime
WHERE t.album_id = ? AND t.missing_since IS NULL
ORDER BY t.id;

-- name: DeleteAlbumLoudness :exec
DELETE FROM album_loudness WHERE album_id = ?;

-- name: InsertAlbumLoudness :one
INSERT INTO album_loudness (album_id, tracks, integrated, true_peak) VALUES (?, ?, ?, ?)
RETURNING id;

-- name: PoolTrackLoudness :exec
UPDATE track_loudness SET album_pool = sqlc.arg(pool) WHERE track_id = sqlc.arg(track_id);

-- name: LoudnessCounts :one
-- Live songs with no ReplayGain, how many have a measure for their file, and
-- how many can't be measured.
SELECT
    count(*) AS ungained,
    CAST(coalesce(sum(l.outcome = 'measured'), 0) AS INTEGER) AS measured,
    CAST(coalesce(sum(l.outcome IN ('unsupported', 'unreadable')), 0) AS INTEGER) AS unmeasurable
FROM tracks t INDEXED BY tracks_ungained
LEFT JOIN track_loudness l ON l.track_id = t.id AND l.size = t.size AND l.mtime = t.mtime
WHERE t.missing_since IS NULL AND t.rg_track_gain IS NULL AND t.rg_album_gain IS NULL;
