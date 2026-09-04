-- Account-query metadata is operational display data only. It must never be
-- consulted by relay routing, health, billing, or cost estimation.

ALTER TABLE channels
    ADD COLUMN IF NOT EXISTS upstream_account_user_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS upstream_balance_total numeric(40, 18),
    ADD COLUMN IF NOT EXISTS upstream_balance_used numeric(40, 18),
    ADD COLUMN IF NOT EXISTS upstream_account_plan_name text NOT NULL DEFAULT '';

-- The prior implementation used generic response-field matching. Clear old
-- snapshots for the integrations whose parsers are now protocol-specific, so
-- the next scheduled refresh cannot display a value interpreted by old rules.
UPDATE channels
SET upstream_balance = NULL,
    upstream_balance_unit = '',
    upstream_balance_total = NULL,
    upstream_balance_used = NULL,
    upstream_account_plan_name = '',
    upstream_rate_multiplier = NULL,
    upstream_account_sync_error = '',
    upstream_account_synced_at = NULL,
    upstream_account_sync_status = CASE
        WHEN upstream_account_credential_ref <> ''
             AND (
                 upstream_integration = 'sub2api'
                 OR (
                     upstream_integration = 'newapi'
                     AND btrim(upstream_account_user_id) <> ''
                 )
             ) THEN 'pending'
        ELSE 'not_configured'
    END
WHERE upstream_integration IN ('newapi', 'sub2api');
