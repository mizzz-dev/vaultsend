-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, expires_at, user_agent, ip_hash)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetSessionByHash :one
SELECT * FROM sessions WHERE token_hash = $1;

-- name: RevokeSession :exec
UPDATE sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: UpdateSessionLastUsed :exec
UPDATE sessions SET last_used_at = $2 WHERE token_hash = $1;
