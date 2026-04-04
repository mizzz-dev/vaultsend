CREATE TABLE subscriptions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    stripe_customer_id varchar(255) NULL,
    stripe_subscription_id varchar(255) NOT NULL UNIQUE,
    plan varchar(32) NOT NULL,
    status varchar(32) NOT NULL,
    current_period_end timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_subscriptions_plan CHECK (plan IN ('free','pro')),
    CONSTRAINT chk_subscriptions_status CHECK (status IN ('active','trialing','past_due','canceled','incomplete','incomplete_expired','unpaid'))
);

CREATE INDEX idx_subscriptions_user_status ON subscriptions (user_id, status, updated_at DESC);

CREATE TRIGGER trg_subscriptions_set_updated_at
    BEFORE UPDATE ON subscriptions
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
