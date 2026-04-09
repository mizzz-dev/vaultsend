ALTER TABLE subscriptions
    ADD COLUMN seat_count bigint NOT NULL DEFAULT 1,
    ADD CONSTRAINT chk_subscriptions_seat_count_positive CHECK (seat_count >= 1);
