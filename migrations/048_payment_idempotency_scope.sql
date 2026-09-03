-- Idempotency keys are client-provided and must be isolated per tenant.
-- A global unique constraint would let one tenant block another tenant from
-- using the same harmless key.
ALTER TABLE payment_orders
    DROP CONSTRAINT IF EXISTS payment_orders_idempotency_key_key;

CREATE UNIQUE INDEX IF NOT EXISTS payment_orders_tenant_idempotency_key_idx
    ON payment_orders (tenant_id, idempotency_key);
