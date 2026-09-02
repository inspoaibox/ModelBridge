ALTER TABLE api_tokens
    ADD COLUMN IF NOT EXISTS deleted_at timestamptz;

ALTER TABLE api_tokens
    DROP CONSTRAINT IF EXISTS api_tokens_status_check;

ALTER TABLE api_tokens
    ADD CONSTRAINT api_tokens_status_check
    CHECK (status IN ('active', 'disabled', 'revoked', 'expired'));

CREATE INDEX IF NOT EXISTS api_tokens_owner_visible_idx
    ON api_tokens (tenant_id, created_by, created_at DESC)
    WHERE deleted_at IS NULL;
