-- Server settings, by key. The value is JSON the caller has checked.

-- name: ListSettings :many
SELECT key, value, source, updated_at FROM settings ORDER BY key;

-- name: UpsertSetting :exec
INSERT INTO settings (key, value, source, updated_at) VALUES (?, ?, ?, ?)
ON CONFLICT (key) DO UPDATE SET value = excluded.value, source = excluded.source, updated_at = excluded.updated_at;

-- name: InsertSettingIfAbsent :execrows
INSERT INTO settings (key, value, source, updated_at) VALUES (?, ?, ?, ?)
ON CONFLICT (key) DO NOTHING;

-- name: DeleteSetting :execrows
DELETE FROM settings WHERE key = ?;

-- name: RootStats :many
-- What each library root holds now: its live tracks and their bytes.
SELECT root, COUNT(*) AS tracks, CAST(COALESCE(SUM(size), 0) AS INTEGER) AS bytes
FROM tracks WHERE missing_since IS NULL GROUP BY root ORDER BY root;
