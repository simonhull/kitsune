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
