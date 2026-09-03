ALTER TABLE model_monitor_configs
    ADD COLUMN IF NOT EXISTS recent_request_limit integer NOT NULL DEFAULT 60;

ALTER TABLE model_monitor_configs
    DROP CONSTRAINT IF EXISTS model_monitor_configs_recent_request_limit_check;

ALTER TABLE model_monitor_configs
    ADD CONSTRAINT model_monitor_configs_recent_request_limit_check
    CHECK (recent_request_limit IN (30, 60, 120));
