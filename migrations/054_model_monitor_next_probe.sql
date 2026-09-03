ALTER TABLE model_monitor_configs
    ADD COLUMN IF NOT EXISTS next_probe_at timestamptz;

-- Existing active monitors without a schedule should run once immediately.
-- Existing completed monitors keep their current interval on the first run.
UPDATE model_monitor_configs
SET next_probe_at = CASE
    WHEN enabled AND mode = 'active' AND probe_started_at IS NULL THEN
        COALESCE(
            last_probe_finished_at + make_interval(secs => probe_interval_seconds),
            now()
        )
    ELSE NULL
END
WHERE next_probe_at IS NULL;

CREATE INDEX IF NOT EXISTS model_monitor_configs_next_probe_idx
    ON model_monitor_configs (enabled, mode, next_probe_at, probe_started_at);
