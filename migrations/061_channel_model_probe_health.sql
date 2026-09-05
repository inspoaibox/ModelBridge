ALTER TABLE channel_models
    ADD COLUMN IF NOT EXISTS probe_health text NOT NULL DEFAULT 'unknown'
        CHECK (probe_health IN ('unknown', 'normal', 'degraded', 'unavailable')),
    ADD COLUMN IF NOT EXISTS probe_consecutive_failures integer NOT NULL DEFAULT 0
        CHECK (probe_consecutive_failures >= 0),
    ADD COLUMN IF NOT EXISTS probe_last_failure_status integer
        CHECK (probe_last_failure_status IS NULL OR probe_last_failure_status BETWEEN 100 AND 599),
    ADD COLUMN IF NOT EXISTS probe_last_failure_at timestamptz,
    ADD COLUMN IF NOT EXISTS probe_last_success_at timestamptz;

CREATE INDEX IF NOT EXISTS channel_models_probe_health_idx
    ON channel_models (channel_id, probe_health, probe_consecutive_failures);
