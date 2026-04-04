ALTER TYPE share_mode ADD VALUE IF NOT EXISTS 'url_shared';

ALTER TABLE shipments
    ADD COLUMN IF NOT EXISTS password_hash varchar(255) NULL;

ALTER TABLE access_tokens
    ADD COLUMN IF NOT EXISTS used_at timestamptz NULL,
    ADD COLUMN IF NOT EXISTS status varchar(20) NOT NULL DEFAULT 'active',
    ADD CONSTRAINT chk_access_tokens_status CHECK (status IN ('active', 'used', 'revoked', 'expired'));

CREATE INDEX IF NOT EXISTS idx_access_tokens_status_expires_at ON access_tokens (status, expires_at);
