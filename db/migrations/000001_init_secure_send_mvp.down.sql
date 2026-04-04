DROP TRIGGER IF EXISTS trg_recipients_set_updated_at ON recipients;
DROP TRIGGER IF EXISTS trg_shipments_set_updated_at ON shipments;
DROP FUNCTION IF EXISTS set_updated_at();

DROP TABLE IF EXISTS upload_sessions;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS download_events;
DROP TABLE IF EXISTS access_tokens;
DROP TABLE IF EXISTS recipients;
DROP TABLE IF EXISTS files;
DROP TABLE IF EXISTS shipments;

DROP TYPE IF EXISTS upload_session_status;
DROP TYPE IF EXISTS actor_type;
DROP TYPE IF EXISTS download_result;
DROP TYPE IF EXISTS token_type;
DROP TYPE IF EXISTS recipient_status;
DROP TYPE IF EXISTS file_upload_status;
DROP TYPE IF EXISTS share_mode;
DROP TYPE IF EXISTS shipment_status;
DROP TYPE IF EXISTS owner_type;
