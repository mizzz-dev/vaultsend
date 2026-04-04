-- name: CreateNotificationEvent :one
INSERT INTO notification_events (shipment_id, recipient_id, event_type, status, queued_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateNotificationEventStatus :exec
UPDATE notification_events
SET status = $2,
    error_message = $3,
    sent_at = COALESCE($4, sent_at),
    failed_at = COALESCE($5, failed_at)
WHERE id = $1;
