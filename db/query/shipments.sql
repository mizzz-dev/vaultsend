-- name: CreateShipment :one
INSERT INTO shipments (
    owner_type, owner_user_id, status, share_mode, title, message, max_downloads, expires_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: GetShipment :one
SELECT *
FROM shipments
WHERE id = $1;
