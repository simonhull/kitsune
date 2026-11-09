-- name: CreateUser :one
INSERT INTO users (name, password_hash, role, created_at)
VALUES (?, ?, ?, ?)
RETURNING id;

-- name: UserByName :one
SELECT id, name, password_hash, role, created_at
FROM users
WHERE name = ?;

-- name: CountUsers :one
SELECT count(*) FROM users;

-- name: SetPasswordHash :execrows
UPDATE users SET password_hash = ? WHERE id = ?;

-- name: CreateFirstUser :execresult
INSERT INTO users (name, password_hash, role, created_at)
SELECT sqlc.arg(name), sqlc.arg(password_hash), sqlc.arg(role), sqlc.arg(created_at)
WHERE NOT EXISTS (SELECT 1 FROM users);

-- name: ListUsers :many
SELECT u.id, u.name, u.role, u.created_at,
       CAST(coalesce(max(s.last_seen), 0) AS INTEGER) AS last_seen,
       count(s.token_hash) AS sessions
FROM users AS u
LEFT JOIN sessions AS s ON s.user_id = u.id AND s.expires_at > sqlc.arg(now)
GROUP BY u.id
ORDER BY u.name, u.id;
