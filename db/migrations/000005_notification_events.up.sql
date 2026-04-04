CREATE TABLE IF NOT EXISTS notification_events (
    id bigserial PRIMARY KEY,
    shipment_id uuid NOT NULL REFERENCES shipments(id) ON DELETE CASCADE,
    recipient_id uuid NOT NULL REFERENCES recipients(id) ON DELETE CASCADE,
    event_type varchar(20) NOT NULL CHECK (event_type IN ('initial_send', 'resend')),
    status varchar(20) NOT NULL CHECK (status IN ('queued', 'sent', 'failed')),
    error_message text NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    queued_at timestamptz NULL,
    sent_at timestamptz NULL,
    failed_at timestamptz NULL
);

CREATE INDEX IF NOT EXISTS idx_notification_events_shipment_created_at ON notification_events (shipment_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notification_events_recipient_created_at ON notification_events (recipient_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notification_events_status_created_at ON notification_events (status, created_at DESC);
