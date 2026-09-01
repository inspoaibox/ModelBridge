ALTER TABLE channel_models
    ADD COLUMN IF NOT EXISTS consecutive_failures integer NOT NULL DEFAULT 0
        CHECK (consecutive_failures >= 0),
    ADD COLUMN IF NOT EXISTS auto_disabled_until timestamptz,
    ADD COLUMN IF NOT EXISTS last_failure_status integer
        CHECK (last_failure_status IS NULL OR last_failure_status BETWEEN 100 AND 599),
    ADD COLUMN IF NOT EXISTS last_failure_at timestamptz,
    ADD COLUMN IF NOT EXISTS last_success_at timestamptz;

CREATE INDEX IF NOT EXISTS channel_models_health_idx
    ON channel_models (channel_id, enabled, auto_disabled_until, consecutive_failures);
