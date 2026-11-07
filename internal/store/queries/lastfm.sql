-- Last.fm accounts and their outbox. An outbox row's next_at never falls below an
-- earlier row's of the same user: a new row starts at the latest next_at there is,
-- and a deferral moves them all. Sending by (next_at, id) is then sending in order.

-- name: GetLastfmAccount :one
SELECT
    a.name, a.session_key, a.scrobble, a.now_playing, a.loves, a.state, a.connected_at, a.last_sent_at,
    (SELECT count(*) FROM lastfm_outbox o WHERE o.user_id = a.user_id AND o.kind = 'scrobble') AS waiting,
    CAST(coalesce((SELECT max(o.attempts) FROM lastfm_outbox o WHERE o.user_id = a.user_id), 0) AS INTEGER) AS attempts,
    CAST(coalesce((SELECT min(o.next_at) FROM lastfm_outbox o WHERE o.user_id = a.user_id), 0) AS INTEGER) AS next_at
FROM lastfm_accounts a
WHERE a.user_id = ?;

-- name: UpsertLastfmAccount :exec
INSERT INTO lastfm_accounts (user_id, name, session_key, scrobble, now_playing, loves, state, connected_at)
VALUES (?, ?, ?, 1, 1, 1, 'ok', ?)
ON CONFLICT (user_id) DO UPDATE SET
    name = excluded.name, session_key = excluded.session_key, state = 'ok', connected_at = excluded.connected_at;

-- name: ResetLastfmOutbox :exec
UPDATE lastfm_outbox SET attempts = 0, next_at = ? WHERE user_id = ?;

-- name: UpdateLastfmSwitches :execrows
UPDATE lastfm_accounts
SET scrobble = sqlc.arg(scrobble), now_playing = sqlc.arg(now_playing), loves = sqlc.arg(loves)
WHERE user_id = sqlc.arg(user_id);

-- name: SetLastfmState :exec
UPDATE lastfm_accounts SET state = ? WHERE user_id = ?;

-- name: DeleteLastfmAccount :exec
DELETE FROM lastfm_accounts WHERE user_id = ?;

-- name: EnqueueLastfm :exec
INSERT INTO lastfm_outbox (user_id, kind, track_id, at, attempts, next_at)
SELECT a.user_id, sqlc.arg(kind), sqlc.arg(track_id), sqlc.arg(at), 0,
    max(CAST(sqlc.arg(now) AS INTEGER), coalesce((SELECT max(o.next_at) FROM lastfm_outbox o WHERE o.user_id = a.user_id), 0))
FROM lastfm_accounts a
WHERE a.user_id = sqlc.arg(user_id)
  AND (CASE WHEN sqlc.arg(kind) = 'scrobble' THEN a.scrobble ELSE a.loves END) = 1;

-- name: ListLastfmDueUsers :many
SELECT DISTINCT o.user_id
FROM lastfm_outbox o
JOIN lastfm_accounts a ON a.user_id = o.user_id
WHERE a.state = 'ok' AND o.next_at <= ?
ORDER BY o.user_id;

-- Plain ? only beside a slice: sqlc numbers named arguments, which a slice
-- of other than one key would shift.
-- name: ListLastfmDue :many
SELECT
    o.id, o.kind, o.at, o.attempts,
    t.title, t.duration_ms, t.track_no, t.id AS track_row,
    al.name AS album, al.artist AS album_artist,
    CAST(coalesce((SELECT tt.value FROM track_tags tt
     WHERE tt.track_id = t.id AND tt.key IN (sqlc.slice(mbid_keys))
     ORDER BY tt.ord LIMIT 1), '') AS TEXT) AS mbid
FROM lastfm_outbox o
JOIN tracks t ON t.id = o.track_id
JOIN albums al ON al.id = t.album_id
WHERE o.user_id = ? AND o.next_at <= ?
  AND o.kind IN (sqlc.slice(kinds))
ORDER BY o.next_at, o.id
LIMIT ?;

-- name: ListTrackArtistNames :many
SELECT ta.track_id, ar.name
FROM track_artists ta
JOIN artists ar ON ar.id = ta.artist_id
WHERE ta.role = 'artist' AND ta.track_id IN (sqlc.slice(track_ids))
ORDER BY ta.track_id, ta.ord;

-- name: DeleteLastfmItem :exec
DELETE FROM lastfm_outbox WHERE id = ? AND user_id = ?;

-- name: MarkLastfmSent :exec
UPDATE lastfm_accounts SET last_sent_at = ? WHERE user_id = ?;

-- name: DeferLastfm :exec
UPDATE lastfm_outbox SET attempts = attempts + 1, next_at = ? WHERE user_id = ?;

-- name: DropOldLastfmScrobbles :execrows
DELETE FROM lastfm_outbox WHERE user_id = ? AND kind = 'scrobble' AND at < ?;

-- name: GetLastfmTrack :one
SELECT
    t.id AS track_row, t.title, t.duration_ms, t.track_no,
    al.name AS album, al.artist AS album_artist,
    CAST(coalesce((SELECT tt.value FROM track_tags tt
     WHERE tt.track_id = t.id AND tt.key IN (sqlc.slice(mbid_keys))
     ORDER BY tt.ord LIMIT 1), '') AS TEXT) AS mbid
FROM tracks t
JOIN albums al ON al.id = t.album_id
WHERE t.ulid = ?;
