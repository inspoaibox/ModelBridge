ALTER TABLE model_requests
    ADD COLUMN IF NOT EXISTS failure_reason text;
