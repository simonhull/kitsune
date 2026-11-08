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
SELECT id, name, role, created_at
FROM users
ORDER BY name, id;
