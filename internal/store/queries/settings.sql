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
