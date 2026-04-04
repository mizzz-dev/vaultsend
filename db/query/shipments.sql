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

-- name: ListShipmentsByUser :many
SELECT
    s.id,
    s.title,
    s.share_mode,
    s.status,
    s.created_at,
    s.expires_at,
    s.max_downloads,
    COUNT(DISTINCT f.id)::int4 AS file_count,
    COUNT(de.id) FILTER (WHERE de.result = 'success')::int4 AS download_count
FROM shipments s
LEFT JOIN files f ON f.shipment_id = s.id
LEFT JOIN download_events de ON de.shipment_id = s.id
WHERE s.owner_user_id = $1
GROUP BY s.id
ORDER BY s.created_at DESC, s.id DESC
LIMIT $2 OFFSET $3;

-- name: DeleteShipmentLogical :exec
UPDATE shipments
SET status = 'deleted',
    deleted_at = COALESCE(deleted_at, now())
WHERE id = $1
  AND status NOT IN ('deleted', 'revoked');

-- name: RevokeAccessTokensByShipment :exec
UPDATE access_tokens
SET status = 'revoked',
    revoked_at = COALESCE(revoked_at, now())
WHERE shipment_id = $1
  AND status <> 'revoked';

-- name: ListExpiredShipments :many
SELECT *
FROM shipments
WHERE expires_at < $1
  AND status NOT IN ('deleted', 'expired', 'revoked')
ORDER BY expires_at ASC
LIMIT $2;

-- name: MarkShipmentExpired :exec
UPDATE shipments
SET status = 'expired'
WHERE id = $1
  AND status NOT IN ('deleted', 'expired', 'revoked');

-- name: ListDeletedShipmentsForCleanup :many
SELECT *
FROM shipments
WHERE status = 'deleted'
  AND deleted_at IS NOT NULL
  AND deleted_at < $1
ORDER BY deleted_at ASC
LIMIT $2;

-- name: DeleteShipmentCascade :exec
DELETE FROM shipments
WHERE id = $1;
