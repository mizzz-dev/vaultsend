-- name: CreateShipment :one
INSERT INTO shipments (
    owner_type, owner_user_id, status, share_mode, title, message, max_downloads, expires_at, password_hash
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING *;

-- name: GetShipment :one
SELECT *
FROM shipments
WHERE id = $1;

-- name: ListFilesByIDs :many
SELECT f.*, s.status AS shipment_status
FROM files f
JOIN shipments s ON s.id = f.shipment_id
WHERE f.id = ANY($1::uuid[]);

-- name: UpdateShipmentForSend :one
UPDATE shipments
SET title = $2,
    message = $3,
    share_mode = $4,
    status = $5,
    expires_at = $6,
    max_downloads = $7,
    password_hash = $8,
    sent_at = CASE WHEN $5 = 'sent' THEN now() ELSE sent_at END
WHERE id = $1
RETURNING *;
