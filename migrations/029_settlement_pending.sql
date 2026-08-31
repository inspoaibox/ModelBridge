ALTER TABLE billing_reservations
    DROP CONSTRAINT IF EXISTS billing_reservations_status_check;

ALTER TABLE billing_reservations
    ADD CONSTRAINT billing_reservations_status_check
    CHECK (status IN ('held', 'pending', 'settled', 'released', 'expired'));

CREATE INDEX IF NOT EXISTS billing_reservations_pending_idx
    ON billing_reservations (status, updated_at)
    WHERE status = 'pending';
