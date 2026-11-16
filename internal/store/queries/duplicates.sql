-- Duplicates: what finding copies of one recording reads beside the index's
-- rows, the groups an administrator said are not duplicates, and what a merge
-- moves from the copies it trashes onto the one it keeps.

-- name: ListTrackDigests :many
-- Every live track the fingerprint pass has read, with its fingerprint.
SELECT t.id, t.audio_digest
FROM tracks t
WHERE t.missing_since IS NULL AND t.audio_digest IS NOT NULL
ORDER BY t.id;

-- name: ListDuplicateDismissals :many
SELECT group_key, tracks FROM duplicate_dismissals ORDER BY dismissed_at, group_key;

-- name: DismissDuplicates :exec
INSERT INTO duplicate_dismissals (group_key, tracks, by_user, dismissed_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (group_key) DO UPDATE SET tracks = excluded.tracks, by_user = excluded.by_user, dismissed_at = excluded.dismissed_at;

-- name: BringBackDuplicates :execrows
DELETE FROM duplicate_dismissals WHERE group_key = ?;

-- name: DuplicateCopies :many
-- The copies of a group as the page compares them: everyone's plays, and the
-- person's own stars beside how many people rated each.
SELECT t.id, t.ulid, t.title, t.root, t.rel_path, t.format, t.bitrate, t.sample_rate, t.bit_depth, t.lossless,
       t.duration_ms, t.added_at, t.disc_no, t.track_no,
       a.ulid AS album_ulid, a.name AS album, a.year AS album_year,
       a.artwork_hash, w.width AS artwork_width, w.height AS artwork_height,
       w.accent AS artwork_accent, w.ground AS artwork_ground,
       (SELECT COUNT(*) FROM plays p INDEXED BY plays_track WHERE p.track_id = t.id) AS plays,
       (SELECT COUNT(*) FROM track_ratings r INDEXED BY track_ratings_track WHERE r.track_id = t.id) AS raters,
       CAST(COALESCE((SELECT r.halves FROM track_ratings r WHERE r.track_id = t.id AND r.user_id = sqlc.arg(user_id)), 0) AS INTEGER) AS halves,
       (SELECT COUNT(DISTINCT pt.playlist_id) FROM playlist_tracks pt WHERE pt.track_id = t.id) AS playlists
FROM tracks t
JOIN albums a ON a.id = t.album_id
LEFT JOIN artwork w ON w.hash = a.artwork_hash
WHERE t.id IN (sqlc.slice(ids)) AND t.missing_since IS NULL;

-- What a merge moves, read for the copies it keeps and trashes.

-- name: MergePlays :many
SELECT track_id, user_id, COUNT(*) AS n FROM plays WHERE track_id IN (sqlc.slice(ids)) GROUP BY track_id, user_id;

-- name: MergeListens :many
SELECT track_id, user_id, COUNT(*) AS n FROM listens WHERE track_id IN (sqlc.slice(ids)) GROUP BY track_id, user_id;

-- name: MergeRatings :many
SELECT r.user_id, r.track_id, r.halves, r.rated_at,
       a.love_from, CAST(COALESCE(a.loves, 0) AS INTEGER) AS loves
FROM track_ratings r
LEFT JOIN lastfm_accounts a ON a.user_id = r.user_id
WHERE r.track_id IN (sqlc.slice(ids))
ORDER BY r.user_id, r.track_id;

-- name: MergeHearts :many
SELECT user_id, track_id, hearted_at FROM track_hearts WHERE track_id IN (sqlc.slice(ids)) ORDER BY user_id, track_id;

-- name: MergeEntries :many
-- Every playlist entry of the copies, with whose playlist it is: NULL for a
-- file playlist's, which everyone sees.
SELECT pt.id, pt.track_id, p.ulid AS playlist, p.name, p.user_id, p.source
FROM playlist_tracks pt
JOIN playlists p ON p.id = pt.playlist_id
WHERE pt.track_id IN (sqlc.slice(ids))
ORDER BY p.id, pt.position;

-- name: MergeExcluded :many
SELECT user_id, track_id FROM shuffle_excluded WHERE track_id IN (sqlc.slice(ids)) ORDER BY user_id, track_id;

-- name: MergeRunTracks :many
SELECT run_id, user_id, position, track_id FROM shuffle_run_tracks WHERE track_id IN (sqlc.slice(ids)) ORDER BY user_id, track_id;

-- name: MergeQueues :many
-- Where people left off, when their queue names any of the songs: each named
-- as a JSON string, so a ULID is found only whole.
SELECT p.user_id, p.queue
FROM playback p
WHERE EXISTS (
    SELECT 1 FROM json_each(CAST(sqlc.arg(ulids) AS TEXT)) j
    WHERE instr(p.queue, '"' || j.value || '"') > 0
)
ORDER BY p.user_id;

-- name: RepointPlays :exec
UPDATE plays SET track_id = sqlc.arg(keep) WHERE track_id = sqlc.arg(trash);

-- name: RepointListens :exec
UPDATE listens SET track_id = sqlc.arg(keep) WHERE track_id = sqlc.arg(trash);

-- name: DeleteTrackRatings :exec
DELETE FROM track_ratings WHERE track_id = ?;

-- name: MoveTrackHearts :exec
-- A heart kept from before goes to the keeper unless one is there; the
-- earlier stays.
INSERT INTO track_hearts (user_id, track_id, hearted_at)
SELECT h.user_id, sqlc.arg(keep), h.hearted_at FROM track_hearts h WHERE h.track_id = sqlc.arg(trash)
ON CONFLICT (user_id, track_id) DO UPDATE SET hearted_at = min(track_hearts.hearted_at, excluded.hearted_at);

-- name: DeleteHeartsOfTrack :exec
DELETE FROM track_hearts WHERE track_id = ?;

-- name: RepointEntries :exec
-- Entries keep their IDs and places: a playlist holding both copies holds the
-- keeper twice.
UPDATE playlist_tracks SET track_id = sqlc.arg(keep) WHERE track_id = sqlc.arg(trash);

-- name: MoveExcluded :exec
INSERT INTO shuffle_excluded (user_id, track_id, excluded_at)
SELECT e.user_id, sqlc.arg(keep), e.excluded_at FROM shuffle_excluded e WHERE e.track_id = sqlc.arg(trash)
ON CONFLICT (user_id, track_id) DO NOTHING;

-- name: DeleteExcludedOfTrack :exec
DELETE FROM shuffle_excluded WHERE track_id = ?;

-- name: RepointRunTrack :exec
UPDATE shuffle_run_tracks SET track_id = sqlc.arg(keep) WHERE run_id = sqlc.arg(run_id) AND position = sqlc.arg(position);

-- name: DropRunTrack :exec
-- shuffle_runs_need_two tidies a run left with one song.
DELETE FROM shuffle_run_tracks WHERE run_id = ? AND position = ?;

-- name: RepointQueues :exec
UPDATE playback SET queue = replace(queue, sqlc.arg(trash), sqlc.arg(keep))
WHERE instr(queue, sqlc.arg(trash)) > 0;
