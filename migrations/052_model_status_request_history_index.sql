CREATE INDEX IF NOT EXISTS model_requests_group_model_created_idx
    ON model_requests (group_id, model_id, created_at DESC, id DESC);
