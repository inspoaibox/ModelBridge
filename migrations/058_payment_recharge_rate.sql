ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS credited_amount numeric(30, 12),
    ADD COLUMN IF NOT EXISTS recharge_rate numeric(24, 12),
    ADD COLUMN IF NOT EXISTS checkout_client_secret text;

UPDATE payment_orders
SET credited_amount = amount,
    recharge_rate = 1
WHERE credited_amount IS NULL
   OR recharge_rate IS NULL;

ALTER TABLE payment_orders
    ALTER COLUMN credited_amount SET NOT NULL,
    ALTER COLUMN credited_amount SET DEFAULT 0,
    ALTER COLUMN recharge_rate SET NOT NULL,
    ALTER COLUMN recharge_rate SET DEFAULT 1;

ALTER TABLE payment_orders
    ADD CONSTRAINT payment_orders_credited_amount_positive CHECK (credited_amount > 0),
    ADD CONSTRAINT payment_orders_recharge_rate_positive CHECK (recharge_rate > 0);
