ALTER TABLE upload_sessions
    ADD COLUMN file_name varchar(255) NOT NULL DEFAULT '(unknown)',
    ADD COLUMN content_type varchar(120) NOT NULL DEFAULT 'application/octet-stream',
    ADD COLUMN file_size_bytes bigint NOT NULL DEFAULT 1,
    ADD COLUMN checksum_sha256 char(64) NOT NULL DEFAULT repeat('0', 64),
    ADD COLUMN owner_user_id uuid NULL;

ALTER TABLE upload_sessions
    ALTER COLUMN file_name DROP DEFAULT,
    ALTER COLUMN content_type DROP DEFAULT,
    ALTER COLUMN file_size_bytes DROP DEFAULT,
    ALTER COLUMN checksum_sha256 DROP DEFAULT;

CREATE INDEX idx_upload_sessions_owner_user_id ON upload_sessions (owner_user_id);
