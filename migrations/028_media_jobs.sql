CREATE TABLE IF NOT EXISTS media_jobs (
    id uuid PRIMARY KEY,
    model_request_id uuid NOT NULL UNIQUE REFERENCES model_requests(id) ON DELETE CASCADE,
    reservation_id uuid UNIQUE REFERENCES billing_reservations(id) ON DELETE SET NULL,
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    token_id uuid NOT NULL REFERENCES api_tokens(id),
    group_id uuid REFERENCES routing_groups(id),
    channel_id uuid NOT NULL REFERENCES channels(id),
    provider text NOT NULL,
    model_name text NOT NULL,
    upstream_model_name text NOT NULL,
    upstream_job_id text NOT NULL,
    status text NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'processing', 'completed', 'failed', 'cancelled')),
    output_uri text,
    response_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    estimated_metrics_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    failure_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);

ALTER TABLE media_jobs
    ADD COLUMN IF NOT EXISTS reservation_id uuid UNIQUE REFERENCES billing_reservations(id) ON DELETE SET NULL;

ALTER TABLE media_jobs
    ADD COLUMN IF NOT EXISTS upstream_model_name text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS media_jobs_tenant_created_idx
    ON media_jobs (tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS media_jobs_token_status_idx
    ON media_jobs (token_id, status, updated_at DESC);

CREATE INDEX IF NOT EXISTS media_jobs_pending_idx
    ON media_jobs (status, updated_at)
    WHERE status IN ('queued', 'processing');
