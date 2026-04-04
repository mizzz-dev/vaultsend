-- name: CreateUploadSession :one
INSERT INTO upload_sessions (
    shipment_id, file_id, storage_bucket, storage_key, multipart_upload_id,
    part_size_bytes, status, expires_at, file_name, content_type, file_size_bytes,
    checksum_sha256, owner_user_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
)
RETURNING *;

-- name: GetUploadSessionByID :one
SELECT *
FROM upload_sessions
WHERE id = $1;

-- name: MarkUploadSessionCompleted :exec
UPDATE upload_sessions
SET status = 'completed', file_id = $2
WHERE id = $1 AND status <> 'completed';

-- name: CreateFile :one
INSERT INTO files (
    shipment_id, original_name, size_bytes, mime_type, storage_bucket,
    storage_key, checksum_sha256, upload_status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;
