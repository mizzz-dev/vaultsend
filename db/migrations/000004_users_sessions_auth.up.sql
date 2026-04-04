CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email varchar(320) NOT NULL,
    email_normalized varchar(320) NOT NULL UNIQUE,
    password_hash varchar(255) NOT NULL,
    display_name varchar(80) NULL,
    status varchar(20) NOT NULL DEFAULT 'active',
    email_verified_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_users_status CHECK (status IN ('active','disabled'))
);

CREATE UNIQUE INDEX idx_users_email_unique ON users (email);

CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash char(64) NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz NULL,
    revoked_at timestamptz NULL,
    user_agent text NULL,
    ip_hash char(64) NULL
);

CREATE INDEX idx_sessions_user_id_created_at ON sessions (user_id, created_at DESC);
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);

CREATE TRIGGER trg_users_set_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
