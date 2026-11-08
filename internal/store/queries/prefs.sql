-- A person's preferences, by key. The value is JSON the caller has checked.

-- name: GetPref :one
SELECT value FROM user_prefs WHERE user_id = ? AND key = ?;

-- name: UpsertPref :exec
INSERT INTO user_prefs (user_id, key, value, updated_at) VALUES (?, ?, ?, ?)
ON CONFLICT (user_id, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at;
