ALTER TABLE model_requests
    ADD COLUMN IF NOT EXISTS group_id uuid REFERENCES routing_groups(id),
    ADD COLUMN IF NOT EXISTS group_multiplier numeric(12, 6) NOT NULL DEFAULT 1.000000
        CHECK (group_multiplier > 0 AND group_multiplier <= 1000);

CREATE INDEX IF NOT EXISTS model_requests_group_created_idx
    ON model_requests (group_id, created_at);

CREATE TABLE IF NOT EXISTS routing_group_rpm_windows (
    group_id uuid NOT NULL REFERENCES routing_groups(id) ON DELETE CASCADE,
    window_start timestamptz NOT NULL,
    request_count bigint NOT NULL DEFAULT 0 CHECK (request_count >= 0),
    PRIMARY KEY (group_id, window_start)
);

CREATE INDEX IF NOT EXISTS routing_group_rpm_windows_cleanup_idx
    ON routing_group_rpm_windows (window_start);
