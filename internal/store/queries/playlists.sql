-- Playlists: each person's own songs in their own order. An entry's position is
-- sparse, so a move writes one row; see migrations/0020_playlists.sql. Every
-- read of what a playlist holds keeps to live tracks, so a trashed song's
-- entries wait, unseen, for Restore.

-- name: InsertPlaylist :one
INSERT INTO playlists (ulid, user_id, name, source, import_key, created_at, updated_at, rules, seed)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: PlaylistRow :one
SELECT id, ulid, name, source, created_at, updated_at, added_at, rules, seed
FROM playlists
WHERE user_id = ? AND ulid = ?;

-- name: PlaylistByImportKey :one
SELECT id FROM playlists WHERE user_id = ? AND import_key = ?;

-- name: ListPlaylists :many
SELECT id, ulid, name, source, created_at, updated_at, added_at, rules, seed
FROM playlists
WHERE user_id = ?
ORDER BY name COLLATE NOCASE, id;

-- name: ListPlaylistSongs :many
-- What every playlist of a person holds that is live, in order: enough to count
-- it, time it, find its mosaic and say which songs it holds.
SELECT e.playlist_id, t.ulid, t.album_id, t.duration_ms
FROM playlist_tracks e
JOIN playlists p ON p.id = e.playlist_id
JOIN tracks t ON t.id = e.track_id
WHERE p.user_id = ? AND t.missing_since IS NULL
ORDER BY e.playlist_id, e.position;

-- name: ListPlaylistSongsOf :many
-- ListPlaylistSongs for one playlist.
SELECT e.playlist_id, t.ulid, t.album_id, t.duration_ms
FROM playlist_tracks e
JOIN tracks t ON t.id = e.track_id
WHERE e.playlist_id = ? AND t.missing_since IS NULL
ORDER BY e.position;

-- name: ListPlaylistAlbums :many
-- The albums a playlist page or a mosaic shows, by row ID.
SELECT
    a.id, a.ulid, a.name, a.artist, a.year, a.compilation,
    a.artwork_hash, w.width AS artwork_width, w.height AS artwork_height,
    w.accent AS artwork_accent, w.ground AS artwork_ground
FROM albums a
LEFT JOIN artwork w ON w.hash = a.artwork_hash
WHERE a.id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: ListPlaylistEntries :many
-- A playlist's live entries in order, each with its song as the index has it.
SELECT e.id, t.id AS track_row, t.ulid, t.album_id, t.title, t.disc_no, t.track_no, t.duration_ms
FROM playlist_tracks e
JOIN tracks t ON t.id = e.track_id
WHERE e.playlist_id = ? AND t.missing_since IS NULL
ORDER BY e.position;

-- name: ListPlaylistTrackArtists :many
-- The artists of every song in a playlist, as ListIndexTrackArtists has them.
SELECT ta.track_id, ar.name
FROM track_artists ta
JOIN artists ar ON ar.id = ta.artist_id
WHERE ta.role = 'artist'
  AND ta.track_id IN (SELECT e.track_id FROM playlist_tracks e WHERE e.playlist_id = ?)
ORDER BY ta.track_id, ta.ord;

-- name: ListPlaylistRatings :many
-- The person's stars for the songs in a playlist.
SELECT r.track_id, r.halves
FROM track_ratings r
WHERE r.user_id = ?
  AND r.track_id IN (SELECT e.track_id FROM playlist_tracks e WHERE e.playlist_id = ?);

-- name: ListPlaylistTrackRows :many
-- Every song a playlist holds, live or not, once: what shuffle's scope keeps
-- and what an add finds already there.
SELECT DISTINCT track_id FROM playlist_tracks WHERE playlist_id = ?;

-- name: CountPlaylistEntries :one
SELECT count(*) FROM playlist_tracks WHERE playlist_id = ?;

-- name: LastPlaylistPosition :one
SELECT CAST(coalesce(max(position), 0) AS INTEGER) FROM playlist_tracks WHERE playlist_id = ?;

-- name: EntryPosition :one
SELECT position FROM playlist_tracks WHERE playlist_id = ? AND id = ?;

-- name: PositionBefore :one
-- The position of the entry just before the given one, or 0 at the start.
SELECT CAST(coalesce(max(position), 0) AS INTEGER) FROM playlist_tracks
WHERE playlist_id = sqlc.arg(playlist_id) AND position < sqlc.arg(position);

-- name: PositionAfter :one
-- The position of the entry just after the given one, or -1 at the end.
SELECT CAST(coalesce(min(position), -1) AS INTEGER) FROM playlist_tracks
WHERE playlist_id = sqlc.arg(playlist_id) AND position > sqlc.arg(position);

-- name: InsertPlaylistEntries :many
-- Puts songs in, from a JSON array of [position, track row] pairs, in one
-- statement however many there are. Entry IDs follow the array's order.
INSERT INTO playlist_tracks (playlist_id, position, track_id, added_at)
SELECT sqlc.arg(playlist_id), json_extract(value, '$[0]'), json_extract(value, '$[1]'), sqlc.arg(added_at)
FROM json_each(CAST(sqlc.arg(entries) AS TEXT))
ORDER BY key
RETURNING id, position, track_id;

-- name: SetEntryPosition :execrows
UPDATE playlist_tracks SET position = ? WHERE playlist_id = ? AND id = ?;

-- name: ListEntryRows :many
-- A playlist's entries in order, to renumber them.
SELECT id FROM playlist_tracks WHERE playlist_id = ? ORDER BY position;

-- name: ParkPositions :exec
-- Moves every position below zero, in order, so a renumber can't collide.
UPDATE playlist_tracks SET position = -1 - position WHERE playlist_id = ?;

-- name: DeletePlaylistEntries :many
DELETE FROM playlist_tracks
WHERE playlist_id = sqlc.arg(playlist_id) AND id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)))
RETURNING id;

-- name: SetPlaylistRules :exec
UPDATE playlists SET rules = ?, seed = ?, updated_at = ? WHERE id = ?;

-- name: SetPlaylistSeed :exec
UPDATE playlists SET seed = ?, updated_at = ? WHERE id = ?;

-- name: ListSmartPlaylistRules :many
-- A person's smart playlists' rules: what a check for a cycle walks.
SELECT ulid, rules FROM playlists WHERE user_id = ? AND rules IS NOT NULL;

-- name: ListSmartRatings :many
-- A person's stars, by track row: what smart rules read of them.
SELECT track_id, halves FROM track_ratings WHERE user_id = ?;

-- name: ListSmartSkips :many
-- How often a person skipped each song: listens that ended skipped.
SELECT track_id, count(*) AS skips
FROM listens
WHERE user_id = ? AND ended = 'skipped'
GROUP BY track_id;

-- name: RenamePlaylist :exec
UPDATE playlists SET name = ?, updated_at = ? WHERE id = ?;

-- name: TouchPlaylist :exec
UPDATE playlists SET updated_at = ? WHERE id = ?;

-- name: TouchPlaylistAdded :exec
UPDATE playlists SET updated_at = sqlc.arg(at), added_at = sqlc.arg(at) WHERE id = sqlc.arg(id);

-- name: DeletePlaylist :execrows
DELETE FROM playlists WHERE user_id = ? AND ulid = ?;

-- name: ListTracksToAdd :many
-- The songs named to go into a playlist, by ULID, and whether each is live.
SELECT id, ulid, CAST(missing_since IS NULL AS INTEGER) AS live
FROM tracks
WHERE ulid IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: ListAlbumTracksToAdd :many
-- An album's live songs in the order the Library lists them, for adding it.
SELECT t.id, t.ulid
FROM tracks t
JOIN albums a ON a.id = t.album_id
WHERE a.ulid = ? AND t.missing_since IS NULL
ORDER BY t.disc_no NULLS LAST, t.track_no NULLS LAST, t.title, t.id;
