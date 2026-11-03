-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, created_at, last_seen, expires_at, user_agent)
VALUES (?, ?, ?, ?, ?, ?);

-- name: SessionByTokenHash :one
SELECT s.created_at, s.last_seen, s.expires_at, s.user_agent,
       u.id, u.name, u.role, u.created_at AS user_created_at
FROM sessions AS s
JOIN users AS u ON u.id = s.user_id
WHERE s.token_hash = ?;

-- name: TouchSession :exec
UPDATE sessions SET last_seen = ?, expires_at = ? WHERE token_hash = ?;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = ?;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at <= ?;
