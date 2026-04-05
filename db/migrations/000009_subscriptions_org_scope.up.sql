ALTER TABLE subscriptions
    ALTER COLUMN user_id DROP NOT NULL,
    ADD COLUMN organization_id uuid NULL REFERENCES organizations(id) ON DELETE CASCADE;

ALTER TABLE subscriptions
    ADD CONSTRAINT chk_subscriptions_owner_scope
    CHECK (
        (user_id IS NOT NULL AND organization_id IS NULL)
        OR (user_id IS NULL AND organization_id IS NOT NULL)
    );

CREATE INDEX idx_subscriptions_org_status ON subscriptions (organization_id, status, updated_at DESC);
CREATE INDEX idx_subscriptions_org_id ON subscriptions (organization_id);
