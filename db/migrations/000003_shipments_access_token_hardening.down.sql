DROP INDEX IF EXISTS idx_access_tokens_status_expires_at;

ALTER TABLE access_tokens
    DROP CONSTRAINT IF EXISTS chk_access_tokens_status,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS used_at;

ALTER TABLE shipments
    DROP COLUMN IF EXISTS password_hash;
