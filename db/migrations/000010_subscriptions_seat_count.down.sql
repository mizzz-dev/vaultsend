ALTER TABLE subscriptions
    DROP CONSTRAINT IF EXISTS chk_subscriptions_seat_count_positive;

ALTER TABLE subscriptions
    DROP COLUMN IF EXISTS seat_count;
