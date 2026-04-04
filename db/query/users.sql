-- name: CreateUser :one
INSERT INTO users (email, email_normalized, password_hash, display_name, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email_normalized = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;
