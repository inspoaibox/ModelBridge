-- Keep customer-facing billing type (prepaid/free) separate from the unit
-- used to measure a successful request.
ALTER TABLE routing_groups
    ADD COLUMN IF NOT EXISTS metering_mode text NOT NULL DEFAULT 'token';

UPDATE routing_groups
SET metering_mode = 'token'
WHERE metering_mode IS NULL OR btrim(metering_mode) = '';

ALTER TABLE routing_groups
    DROP CONSTRAINT IF EXISTS routing_groups_metering_mode_check;

ALTER TABLE routing_groups
    ADD CONSTRAINT routing_groups_metering_mode_check
    CHECK (metering_mode IN ('token', 'image_count', 'video_seconds', 'video_request'));

ALTER TABLE model_requests
    ADD COLUMN IF NOT EXISTS group_metering_mode text NOT NULL DEFAULT 'token';

UPDATE model_requests
SET group_metering_mode = 'token'
WHERE group_metering_mode IS NULL OR btrim(group_metering_mode) = '';

ALTER TABLE model_requests
    DROP CONSTRAINT IF EXISTS model_requests_group_metering_mode_check;

ALTER TABLE model_requests
    ADD CONSTRAINT model_requests_group_metering_mode_check
    CHECK (group_metering_mode IN ('token', 'image_count', 'video_seconds', 'video_request'));

ALTER TABLE billing_reservations
    ADD COLUMN IF NOT EXISTS estimated_upstream_metrics_json jsonb NOT NULL DEFAULT '{}'::jsonb;
