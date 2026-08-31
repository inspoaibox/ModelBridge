ALTER TABLE api_tokens
    ADD COLUMN IF NOT EXISTS allowed_domains_json jsonb NOT NULL DEFAULT '[]'::jsonb;

CREATE INDEX IF NOT EXISTS api_tokens_network_allowlist_idx
    ON api_tokens (status, tenant_id)
    WHERE jsonb_array_length(allowed_ips_json) > 0
       OR jsonb_array_length(allowed_domains_json) > 0;
