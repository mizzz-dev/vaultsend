DROP INDEX IF EXISTS idx_upload_sessions_owner_user_id;

ALTER TABLE upload_sessions
    DROP COLUMN IF EXISTS owner_user_id,
    DROP COLUMN IF EXISTS checksum_sha256,
    DROP COLUMN IF EXISTS file_size_bytes,
    DROP COLUMN IF EXISTS content_type,
    DROP COLUMN IF EXISTS file_name;
