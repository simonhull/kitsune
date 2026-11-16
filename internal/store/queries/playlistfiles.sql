-- File playlists: playlists read from .m3u and .m3u8 files in the library,
-- which everyone sees and nobody owns. See migrations/0022_file_playlists.sql.

-- name: FilePlaylistStates :many
-- What the store knows of every playlist file under a root: what a scan
-- compares the walk with.
SELECT f.playlist_id, p.ulid, f.rel_path, f.size, f.mtime_ms, f.reader,
    CAST(json_array_length(f.missing) + f.past_limit AS INTEGER) AS not_found
FROM playlist_files f
JOIN playlists p ON p.id = f.playlist_id
WHERE f.root = ?;

-- name: FilePlaylistRoots :many
SELECT DISTINCT root FROM playlist_files ORDER BY root;

-- name: TrackPaths :many
-- Every track's path, where its file goes back to while it is in the Trash:
-- what a playlist file's lines are matched against.
SELECT id, root,
    CAST(CASE WHEN trashed_at IS NOT NULL THEN coalesce(restore_path, rel_path) ELSE rel_path END AS TEXT) AS rel_path,
    CAST(missing_since IS NULL AS INTEGER) AS live
FROM tracks;

-- name: FilePlaylistAt :one
SELECT f.playlist_id, p.ulid, p.name, f.size, f.mtime_ms
FROM playlist_files f
JOIN playlists p ON p.id = f.playlist_id
WHERE f.root = ? AND f.rel_path = ?;

-- name: InsertFilePlaylist :one
INSERT INTO playlists (ulid, user_id, name, source, created_at, updated_at)
VALUES (sqlc.arg(ulid), NULL, sqlc.arg(name), 'file', sqlc.arg(at), sqlc.arg(at))
RETURNING id;

-- name: SetFilePlaylistName :exec
UPDATE playlists SET name = ?, updated_at = ? WHERE id = ?;

-- name: UpsertPlaylistFile :exec
INSERT INTO playlist_files (playlist_id, root, rel_path, size, mtime_ms, read_at, reader, lines, past_limit, missing)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (playlist_id) DO UPDATE SET
    size = excluded.size, mtime_ms = excluded.mtime_ms, read_at = excluded.read_at, reader = excluded.reader,
    lines = excluded.lines, past_limit = excluded.past_limit, missing = excluded.missing;

-- name: ClearPlaylistEntries :exec
DELETE FROM playlist_tracks WHERE playlist_id = ?;

-- name: InsertFileEntries :exec
-- A file playlist's entries, from a JSON array of [line number, track row,
-- line] triples, in one statement: the line's number is the position.
INSERT INTO playlist_tracks (playlist_id, position, track_id, added_at, line)
SELECT sqlc.arg(playlist_id), json_extract(value, '$[0]'), json_extract(value, '$[1]'), sqlc.arg(added_at),
    json_extract(value, '$[2]')
FROM json_each(CAST(sqlc.arg(entries) AS TEXT))
ORDER BY key;

-- name: ListFileEntryLines :many
-- The lines of a file playlist's last read that found a song still live: what
-- keeps a moved song when the file is read again.
SELECT CAST(e.line AS TEXT) AS line, e.track_id
FROM playlist_tracks e
JOIN tracks t ON t.id = e.track_id
WHERE e.playlist_id = ? AND e.line IS NOT NULL AND t.missing_since IS NULL
ORDER BY e.position;

-- name: DeleteFilePlaylist :one
DELETE FROM playlists
WHERE source = 'file' AND id = (SELECT f.playlist_id FROM playlist_files f WHERE f.root = ? AND f.rel_path = ?)
RETURNING ulid;

-- name: PlaylistFileOf :one
SELECT rel_path, mtime_ms, read_at, past_limit, missing
FROM playlist_files
WHERE playlist_id = ?;

-- name: ListPlaylistFiles :many
-- Every file playlist's file, with its lines that found no song when it was
-- read, and its entries whose song has left the library since.
SELECT f.playlist_id, f.rel_path, f.mtime_ms, f.read_at, f.past_limit,
    CAST(json_array_length(f.missing) AS INTEGER) AS missing,
    CAST((SELECT count(*) FROM playlist_tracks e JOIN tracks t ON t.id = e.track_id
        WHERE e.playlist_id = f.playlist_id AND t.missing_since IS NOT NULL) AS INTEGER) AS gone
FROM playlist_files f;

-- name: ListExportTracks :many
-- The live songs an export writes, by row: where each is and what its
-- #EXTINF says.
SELECT id, root, rel_path, duration_ms, title
FROM tracks
WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) AND missing_since IS NULL;

-- name: ListExportArtists :many
SELECT ta.track_id, ar.name
FROM track_artists ta
JOIN artists ar ON ar.id = ta.artist_id
WHERE ta.role = 'artist' AND ta.track_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)))
ORDER BY ta.track_id, ta.ord;

-- name: ListGoneFileEntries :many
-- A file playlist's entries whose song is no longer live: in the Trash, or
-- missing since the file was read.
SELECT e.position, CAST(coalesce(e.line, '') AS TEXT) AS line, CAST(t.trashed_at IS NOT NULL AS INTEGER) AS trashed
FROM playlist_tracks e
JOIN tracks t ON t.id = e.track_id
WHERE e.playlist_id = ? AND t.missing_since IS NOT NULL
ORDER BY e.position;
