CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE owner_type AS ENUM ('anonymous', 'user');
CREATE TYPE shipment_status AS ENUM ('draft', 'uploading', 'ready', 'sent', 'accessed', 'expired', 'deleted', 'revoked');
CREATE TYPE share_mode AS ENUM ('public_link', 'recipient_restricted');
CREATE TYPE file_upload_status AS ENUM ('initiated', 'parts_uploaded', 'completed', 'failed');
CREATE TYPE recipient_status AS ENUM ('pending', 'notified', 'verified', 'downloaded', 'blocked');
CREATE TYPE token_type AS ENUM ('download_access', 'manage', 'otp_verify');
CREATE TYPE download_result AS ENUM ('success', 'expired', 'over_limit', 'invalid_token', 'forbidden');
CREATE TYPE actor_type AS ENUM ('user', 'anonymous', 'system', 'admin');
CREATE TYPE upload_session_status AS ENUM ('initiated', 'uploading', 'completed', 'aborted');

CREATE TABLE shipments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_type owner_type NOT NULL,
    owner_user_id uuid NULL,
    status shipment_status NOT NULL,
    share_mode share_mode NOT NULL,
    title varchar(200) NOT NULL,
    message text NULL,
    max_downloads integer NOT NULL DEFAULT 10 CHECK (max_downloads BETWEEN 1 AND 100),
    current_downloads integer NOT NULL DEFAULT 0,
    expires_at timestamptz NOT NULL,
    sent_at timestamptz NULL,
    revoked_at timestamptz NULL,
    deleted_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_shipments_owner_created_at ON shipments (owner_user_id, created_at DESC);
CREATE INDEX idx_shipments_status_expires_at ON shipments (status, expires_at);
CREATE INDEX idx_shipments_active_deleted_at ON shipments (deleted_at) WHERE deleted_at IS NULL;

CREATE TABLE files (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    shipment_id uuid NOT NULL REFERENCES shipments(id) ON DELETE CASCADE,
    original_name varchar(255) NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes > 0),
    mime_type varchar(120) NOT NULL,
    storage_bucket varchar(63) NOT NULL,
    storage_key varchar(1024) NOT NULL UNIQUE,
    checksum_sha256 char(64) NOT NULL,
    upload_status file_upload_status NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_files_shipment_id ON files (shipment_id);
CREATE INDEX idx_files_upload_status ON files (upload_status);

CREATE TABLE recipients (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    shipment_id uuid NOT NULL REFERENCES shipments(id) ON DELETE CASCADE,
    email varchar(320) NOT NULL,
    email_normalized varchar(320) NOT NULL,
    status recipient_status NOT NULL,
    verify_code_hash varchar(255) NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (shipment_id, email_normalized)
);

CREATE INDEX idx_recipients_shipment_status ON recipients (shipment_id, status);

CREATE TABLE access_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    shipment_id uuid NOT NULL REFERENCES shipments(id) ON DELETE CASCADE,
    recipient_id uuid NULL REFERENCES recipients(id) ON DELETE SET NULL,
    token_type token_type NOT NULL,
    token_hash char(64) NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    max_uses integer NOT NULL DEFAULT 1,
    used_count integer NOT NULL DEFAULT 0,
    revoked_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_access_tokens_shipment_type_expires_at ON access_tokens (shipment_id, token_type, expires_at);
CREATE INDEX idx_access_tokens_recipient_type ON access_tokens (recipient_id, token_type);

CREATE TABLE download_events (
    id bigserial PRIMARY KEY,
    shipment_id uuid NOT NULL REFERENCES shipments(id) ON DELETE CASCADE,
    file_id uuid NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    recipient_id uuid NULL REFERENCES recipients(id) ON DELETE SET NULL,
    result download_result NOT NULL,
    ip_hash char(64) NOT NULL,
    user_agent text NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_download_events_shipment_created_at ON download_events (shipment_id, created_at DESC);
CREATE INDEX idx_download_events_recipient_created_at ON download_events (recipient_id, created_at DESC);

CREATE TABLE audit_logs (
    id bigserial PRIMARY KEY,
    actor_type actor_type NOT NULL,
    actor_id varchar(64) NULL,
    action varchar(64) NOT NULL,
    resource_type varchar(64) NOT NULL,
    resource_id varchar(64) NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_logs_resource_created_at ON audit_logs (resource_type, resource_id, created_at DESC);
CREATE INDEX idx_audit_logs_actor_created_at ON audit_logs (actor_type, actor_id, created_at DESC);

CREATE TABLE upload_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    shipment_id uuid NULL REFERENCES shipments(id) ON DELETE SET NULL,
    file_id uuid NULL REFERENCES files(id) ON DELETE SET NULL,
    storage_bucket varchar(63) NOT NULL,
    storage_key varchar(1024) NOT NULL,
    multipart_upload_id varchar(255) NOT NULL UNIQUE,
    part_size_bytes integer NOT NULL,
    status upload_session_status NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_upload_sessions_status_expires_at ON upload_sessions (status, expires_at);

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_shipments_set_updated_at
    BEFORE UPDATE ON shipments
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_recipients_set_updated_at
    BEFORE UPDATE ON recipients
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
