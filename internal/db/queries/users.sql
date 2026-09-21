-- name: CreateUser :one
INSERT INTO users (email, username, password_hash)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- Addresses are compared case-insensitively, matching the unique index.
-- name: GetUserByEmail :one
SELECT * FROM users WHERE lower(email) = lower($1);

-- name: GetUserByUsername :one
SELECT * FROM users WHERE lower(username) = lower($1);

-- name: MarkUserVerified :one
UPDATE users SET verified = true WHERE id = $1 RETURNING *;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = $2 WHERE id = $1;

-- Used to tell a duplicate email from a duplicate username *internally*, without
-- the response revealing which one collided.
-- name: CountUsersByEmailOrUsername :one
SELECT
    count(*) FILTER (WHERE lower(email) = lower($1))    AS email_count,
    count(*) FILTER (WHERE lower(username) = lower($2)) AS username_count
FROM users;
