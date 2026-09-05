ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS package_id uuid,
    ADD COLUMN IF NOT EXISTS bonus_amount numeric(30, 12) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS valid_until timestamptz;

ALTER TABLE payment_orders
    DROP CONSTRAINT IF EXISTS payment_orders_bonus_amount_nonnegative;

ALTER TABLE payment_orders
    ADD CONSTRAINT payment_orders_bonus_amount_nonnegative CHECK (bonus_amount >= 0);
