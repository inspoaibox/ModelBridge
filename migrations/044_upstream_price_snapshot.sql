-- Keep the price basis used for upstream cost estimation separate from the
-- customer-facing charge snapshot.

ALTER TABLE model_requests
    ADD COLUMN IF NOT EXISTS upstream_price_snapshot_json jsonb NOT NULL DEFAULT '{}'::jsonb;
