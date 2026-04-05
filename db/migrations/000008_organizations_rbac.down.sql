DROP TRIGGER IF EXISTS trg_organizations_set_updated_at ON organizations;
DROP INDEX IF EXISTS idx_shipments_organization_created_at;
ALTER TABLE shipments DROP COLUMN IF EXISTS organization_id;
DROP INDEX IF EXISTS idx_organization_members_user;
DROP TABLE IF EXISTS organization_members;
DROP TABLE IF EXISTS organizations;
DROP TYPE IF EXISTS organization_role;
