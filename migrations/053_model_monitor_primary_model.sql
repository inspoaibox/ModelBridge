ALTER TABLE model_monitor_configs
    ADD COLUMN IF NOT EXISTS primary_model_id uuid REFERENCES models(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS model_monitor_configs_primary_model_idx
    ON model_monitor_configs (primary_model_id);
