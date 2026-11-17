-- ReplayGain writes: see migrations/0025_replaygain_writes.sql.

-- name: GainCandidates :many
-- Every live song with no ReplayGain, with its measure of its file and its
-- album's pooled measure, whether that pool is current (it pools exactly the
-- album's live songs, each measured for its file now), and whether a run
-- wrote this very file already (a gain that rounds to 0.00 dB reads as none).
-- In the Library's album order is the caller's: rows come by album, then disc
-- and track.
SELECT t.id, t.ulid, t.title, t.root, t.rel_path, t.size, t.mtime, t.format,
       al.id AS album_id, al.ulid AS album, al.name AS album_title, al.artist AS album_artist, al.year AS album_year,
       al.artwork_hash, w.width AS artwork_width, w.height AS artwork_height, w.accent AS artwork_accent, w.ground AS artwork_ground,
       l.version AS measure_version, l.outcome AS measure_outcome, l.integrated AS measure_integrated, l.true_peak AS measure_peak,
       a.integrated AS pool_integrated, a.true_peak AS pool_peak,
       CAST(a.id IS NOT NULL AND l.album_pool IS a.id
            AND a.tracks = (SELECT count(*) FROM tracks x WHERE x.album_id = t.album_id AND x.missing_since IS NULL)
            AND NOT EXISTS (SELECT 1 FROM tracks x
                            LEFT JOIN track_loudness y ON y.track_id = x.id AND y.size = x.size AND y.mtime = x.mtime
                            WHERE x.album_id = t.album_id AND x.missing_since IS NULL AND y.album_pool IS NOT a.id)
            AS INTEGER) AS pool_current,
       CAST(EXISTS (SELECT 1 FROM gain_writes g INDEXED BY gain_writes_track
                    JOIN gain_runs r ON r.id = g.run_id AND r.kind = 'run'
                    WHERE g.track_id = t.id AND g.state = 'written' AND g.size_after = t.size AND g.mtime_after = t.mtime)
            AS INTEGER) AS written
FROM tracks t INDEXED BY tracks_ungained
JOIN albums al ON al.id = t.album_id
LEFT JOIN artwork w ON w.hash = al.artwork_hash
LEFT JOIN track_loudness l ON l.track_id = t.id AND l.size = t.size AND l.mtime = t.mtime
LEFT JOIN album_loudness a ON a.album_id = t.album_id
WHERE t.missing_since IS NULL AND t.rg_track_gain IS NULL AND t.rg_album_gain IS NULL
ORDER BY t.album_id, t.disc_no NULLS LAST, t.track_no NULLS LAST, t.title, t.id;

-- name: GainTrackStates :many
-- The songs a batch is about to write, or an undo to put back, as they are
-- now: where, which file, whether they are live and still carry no
-- ReplayGain, and their measure and their album's pool, current or not.
SELECT t.id, t.root, t.rel_path, t.size, t.mtime, t.missing_since, t.trashed_at,
       CAST(t.rg_track_gain IS NULL AND t.rg_album_gain IS NULL AS INTEGER) AS ungained,
       l.version AS measure_version, l.outcome AS measure_outcome, l.integrated AS measure_integrated,
       l.true_peak AS measure_peak, a.integrated AS pool_integrated, a.true_peak AS pool_peak,
       CAST(a.id IS NOT NULL AND l.album_pool IS a.id
            AND a.tracks = (SELECT count(*) FROM tracks x WHERE x.album_id = t.album_id AND x.missing_since IS NULL)
            AND NOT EXISTS (SELECT 1 FROM tracks x
                            LEFT JOIN track_loudness y ON y.track_id = x.id AND y.size = x.size AND y.mtime = x.mtime
                            WHERE x.album_id = t.album_id AND x.missing_since IS NULL AND y.album_pool IS NOT a.id)
            AS INTEGER) AS pool_current
FROM tracks t
LEFT JOIN track_loudness l ON l.track_id = t.id AND l.size = t.size AND l.mtime = t.mtime
LEFT JOIN album_loudness a ON a.album_id = t.album_id
WHERE t.id IN (sqlc.slice(ids));

-- name: CreateGainRun :one
INSERT INTO gain_runs (ulid, kind, by_user, started_at, state, total, albums, undo_of)
VALUES (?, ?, ?, ?, 'running', ?, ?, ?)
RETURNING id;

-- name: PlanGainWrite :one
-- Planned, or skipped at once with its reason: a song the batch leaves.
INSERT INTO gain_writes (run_id, track_id, album, ord, root, rel_path, state, reason, track_gain, track_peak, album_gain, album_peak, prior, undoes)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: SettleGainWrite :exec
UPDATE gain_writes SET state = ?, reason = ?, size_after = ?, mtime_after = ? WHERE id = ?;

-- name: SetGainWriteState :exec
UPDATE gain_writes SET state = ? WHERE id = ?;

-- name: CountGainRun :exec
UPDATE gain_runs
SET written = written + sqlc.arg(written), left_count = left_count + sqlc.arg(left_count),
    failed = failed + sqlc.arg(failed)
WHERE id = sqlc.arg(id);

-- name: FinishGainRun :exec
UPDATE gain_runs SET state = ?, finished_at = ? WHERE id = ?;

-- name: SetGainRunState :exec
UPDATE gain_runs SET state = ? WHERE id = ?;

-- name: ListGainRuns :many
-- The latest runs, a running one first. undoable says a file the run wrote is
-- still as it left it.
SELECT r.id, r.ulid, r.kind, u.name AS by_name, r.started_at, r.finished_at, r.state, r.total, r.albums,
       r.written, r.left_count, r.failed, o.ulid AS undo_of,
       CAST(COALESCE((SELECT x.ulid FROM gain_runs x WHERE x.undo_of = r.id ORDER BY x.id DESC LIMIT 1), '') AS TEXT) AS undone_by,
       EXISTS (SELECT 1 FROM gain_writes g WHERE g.run_id = r.id AND g.state = 'written' AND g.track_id IS NOT NULL) AS undoable,
       (SELECT count(DISTINCT g.album) FROM gain_writes g WHERE g.run_id = r.id AND g.state <> 'planned') AS albums_done
FROM gain_runs r
LEFT JOIN users u ON u.id = r.by_user
LEFT JOIN gain_runs o ON o.id = r.undo_of
ORDER BY r.state = 'running' DESC, r.id DESC
LIMIT ?;

-- name: GainRun :one
SELECT r.id, r.ulid, r.kind, u.name AS by_name, r.started_at, r.finished_at, r.state, r.total, r.albums,
       r.written, r.left_count, r.failed, o.ulid AS undo_of,
       CAST(COALESCE((SELECT x.ulid FROM gain_runs x WHERE x.undo_of = r.id ORDER BY x.id DESC LIMIT 1), '') AS TEXT) AS undone_by,
       EXISTS (SELECT 1 FROM gain_writes g WHERE g.run_id = r.id AND g.state = 'written' AND g.track_id IS NOT NULL) AS undoable,
       (SELECT count(DISTINCT g.album) FROM gain_writes g WHERE g.run_id = r.id AND g.state <> 'planned') AS albums_done
FROM gain_runs r
LEFT JOIN users u ON u.id = r.by_user
LEFT JOIN gain_runs o ON o.id = r.undo_of
WHERE r.ulid = ?;

-- name: PlannedGainWrites :many
-- Writes a run journaled and never settled: what a server stopped midway left.
SELECT g.id, g.run_id, g.track_id, g.album, g.ord, g.root, g.rel_path, g.track_gain, g.track_peak,
       g.album_gain, g.album_peak, g.prior, g.undoes
FROM gain_writes g
WHERE g.state = 'planned'
ORDER BY g.id;

-- name: WrittenGainWrites :many
-- What a run wrote, for its undo, with where each song is now.
SELECT g.id, g.track_id, g.album, g.ord, g.root, g.rel_path, g.track_gain, g.track_peak, g.album_gain, g.album_peak,
       g.prior, t.title
FROM gain_writes g
LEFT JOIN tracks t ON t.id = g.track_id
WHERE g.run_id = ? AND g.state = 'written'
ORDER BY g.ord, g.id;

-- name: GainRunAlbums :many
-- A run's albums in the order it took them, with what it did to each.
SELECT g.album, CAST(MIN(g.ord) AS INTEGER) AS first_ord,
       CAST(SUM(g.state IN ('written', 'undone')) AS INTEGER) AS written,
       CAST(SUM(g.state = 'skipped') AS INTEGER) AS left_count,
       CAST(SUM(g.state = 'planned') AS INTEGER) AS planned,
       COUNT(*) AS songs
FROM gain_writes g
WHERE g.run_id = ?
GROUP BY g.album
ORDER BY first_ord;

-- name: GainAlbumFacts :many
-- What a run's album list shows of albums, by ULID, and the formats of their
-- live songs.
SELECT al.ulid, al.name, al.artist, al.year, al.artwork_hash, w.width AS artwork_width, w.height AS artwork_height,
       w.accent AS artwork_accent, w.ground AS artwork_ground,
       CAST(COALESCE((SELECT group_concat(DISTINCT t.format) FROM tracks t WHERE t.album_id = al.id AND t.missing_since IS NULL), '') AS TEXT) AS formats
FROM albums al
LEFT JOIN artwork w ON w.hash = al.artwork_hash
WHERE al.ulid IN (sqlc.slice(ulids));

-- name: LeftGainWrites :many
-- The songs a run left as they were, and why.
SELECT COALESCE(t.ulid, '') AS track_ulid, COALESCE(t.title, '') AS title, g.root, g.rel_path, g.reason
FROM gain_writes g
LEFT JOIN tracks t ON t.id = g.track_id
WHERE g.run_id = ? AND g.state = 'skipped'
ORDER BY g.ord, g.id
LIMIT ?;

-- name: PruneGainRuns :execrows
-- Runs past the last kept, and older than the cutoff, go with their journals.
DELETE FROM gain_runs
WHERE gain_runs.state <> 'running' AND gain_runs.started_at < sqlc.arg(cutoff)
  AND id NOT IN (SELECT k.id FROM gain_runs k ORDER BY k.id DESC LIMIT sqlc.arg(keep));

-- name: StopRunningGainRuns :execrows
-- Runs a stopped server left running: stopped, as Stop leaves them.
UPDATE gain_runs SET state = 'stopped', finished_at = ? WHERE state = 'running';

-- name: CarryTrackLoudness :exec
-- A file whose tags alone were rewritten keeps its measure: the audio is the
-- same, as audiometa verified. Only a measure of the file before moves along.
UPDATE track_loudness
SET size = sqlc.arg(size), mtime = sqlc.arg(mtime)
WHERE track_id = sqlc.arg(id)
  AND size = (SELECT t.size FROM tracks t WHERE t.id = sqlc.arg(id))
  AND mtime = (SELECT t.mtime FROM tracks t WHERE t.id = sqlc.arg(id));
