-- Runtime billing fields. Prices remain versioned and immutable; account balance
-- is only a cached available balance used inside the reservation transaction.

ALTER TABLE ledger_accounts
    ALTER COLUMN tenant_id DROP NOT NULL;

ALTER TABLE ledger_accounts
    ADD COLUMN IF NOT EXISTS balance numeric(24, 12) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS account_code text,
    ADD COLUMN IF NOT EXISTS is_system boolean NOT NULL DEFAULT false;

CREATE UNIQUE INDEX IF NOT EXISTS ledger_accounts_code_unique
    ON ledger_accounts (account_code)
    WHERE account_code IS NOT NULL;

ALTER TABLE model_requests
    ADD COLUMN IF NOT EXISTS idempotency_key text,
    ADD COLUMN IF NOT EXISTS price_version_id uuid REFERENCES price_versions(id),
    ADD COLUMN IF NOT EXISTS failure_reason text;

CREATE UNIQUE INDEX IF NOT EXISTS model_requests_idempotency_unique
    ON model_requests (idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX IF NOT EXISTS model_requests_token_created_idx
    ON model_requests (token_id, created_at);

CREATE INDEX IF NOT EXISTS billing_reservations_expiry_idx
    ON billing_reservations (status, expires_at);
