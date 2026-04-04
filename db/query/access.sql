-- name: GetAccessTokenByHash :one
SELECT *
FROM access_tokens
WHERE token_hash = $1;

-- name: CountDownloadEventsByShipment :one
SELECT COUNT(1)::int
FROM download_events
WHERE shipment_id = $1
  AND result = 'success';

-- name: CreateDownloadEvent :one
INSERT INTO download_events (
  shipment_id,
  file_id,
  recipient_id,
  result,
  ip_hash,
  user_agent
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateAccessTokenUsage :exec
UPDATE access_tokens
SET used_count = used_count + 1,
    used_at = COALESCE(used_at, now()),
    status = CASE WHEN used_count + 1 >= max_uses THEN 'used' ELSE status END
WHERE id = $1;
