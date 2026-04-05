DROP INDEX IF EXISTS idx_subscriptions_org_id;
DROP INDEX IF EXISTS idx_subscriptions_org_status;

ALTER TABLE subscriptions
    DROP CONSTRAINT IF EXISTS chk_subscriptions_owner_scope;

ALTER TABLE subscriptions
    DROP COLUMN IF EXISTS organization_id;

ALTER TABLE subscriptions
    ALTER COLUMN user_id SET NOT NULL;
