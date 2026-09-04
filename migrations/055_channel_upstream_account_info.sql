-- Upstream account information is an optional, administrator-only snapshot.
-- It is deliberately separate from the channel credential used for relay calls.

ALTER TABLE channels
    ADD COLUMN IF NOT EXISTS upstream_integration text NOT NULL DEFAULT 'official',
    ADD COLUMN IF NOT EXISTS upstream_account_credential_ref text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS upstream_balance numeric(40, 18),
    ADD COLUMN IF NOT EXISTS upstream_balance_unit text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS upstream_rate_multiplier numeric(40, 18),
    ADD COLUMN IF NOT EXISTS upstream_account_sync_status text NOT NULL DEFAULT 'not_configured',
    ADD COLUMN IF NOT EXISTS upstream_account_sync_error text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS upstream_account_synced_at timestamptz,
    ADD COLUMN IF NOT EXISTS upstream_account_last_attempt_at timestamptz;

ALTER TABLE channels
    DROP CONSTRAINT IF EXISTS channels_upstream_integration_check;

ALTER TABLE channels
    ADD CONSTRAINT channels_upstream_integration_check
    CHECK (upstream_integration IN ('official', 'newapi', 'sub2api', 'other'));

ALTER TABLE channels
    DROP CONSTRAINT IF EXISTS channels_upstream_account_sync_status_check;

ALTER TABLE channels
    ADD CONSTRAINT channels_upstream_account_sync_status_check
    CHECK (upstream_account_sync_status IN ('not_configured', 'pending', 'success', 'failed', 'not_supported'));

CREATE TABLE IF NOT EXISTS channel_account_secrets (
    id uuid PRIMARY KEY,
    channel_id uuid NOT NULL REFERENCES channels(id),
    encrypted_secret bytea NOT NULL,
    secret_prefix text NOT NULL DEFAULT '',
    secret_suffix text NOT NULL DEFAULT '',
    created_by uuid REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    rotated_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS channel_account_secrets_active_unique
    ON channel_account_secrets (channel_id)
    WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS channel_account_secrets_channel_idx
    ON channel_account_secrets (channel_id, revoked_at);

-- Existing channels have no independent account-query credential and must
-- remain fully usable without any follow-up configuration.
UPDATE channels
SET upstream_integration = 'official',
    upstream_account_credential_ref = '',
    upstream_account_sync_status = 'not_configured'
WHERE upstream_integration IS NULL OR btrim(upstream_integration) = '';
