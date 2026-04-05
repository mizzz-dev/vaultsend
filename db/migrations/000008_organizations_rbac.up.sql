CREATE TYPE organization_role AS ENUM ('owner', 'admin', 'member');

CREATE TABLE organizations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name varchar(120) NOT NULL,
    owner_user_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE organization_members (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id uuid NOT NULL,
    role organization_role NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, user_id)
);

CREATE INDEX idx_organization_members_user ON organization_members (user_id, organization_id);

ALTER TABLE shipments
    ADD COLUMN organization_id uuid NULL REFERENCES organizations(id) ON DELETE SET NULL;

CREATE INDEX idx_shipments_organization_created_at ON shipments (organization_id, created_at DESC);

CREATE TRIGGER trg_organizations_set_updated_at
    BEFORE UPDATE ON organizations
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
