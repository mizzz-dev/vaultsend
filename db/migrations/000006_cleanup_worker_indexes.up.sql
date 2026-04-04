CREATE INDEX IF NOT EXISTS idx_shipments_deleted_status_deleted_at
    ON shipments (status, deleted_at)
    WHERE status = 'deleted' AND deleted_at IS NOT NULL;
