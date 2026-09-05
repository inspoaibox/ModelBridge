-- Active monitor probes are request health events, but are not customer
-- requests and must never enter customer billing or token usage totals.
CREATE TABLE IF NOT EXISTS model_probe_requests (
    id uuid PRIMARY KEY,
    group_id uuid NOT NULL REFERENCES routing_groups(id) ON DELETE CASCADE,
    model_id uuid NOT NULL REFERENCES models(id) ON DELETE CASCADE,
    channel_id uuid REFERENCES channels(id) ON DELETE SET NULL,
    status text NOT NULL CHECK (status IN ('settled', 'failed')),
    latency_ms bigint NOT NULL DEFAULT 0 CHECK (latency_ms >= 0),
    status_code integer CHECK (status_code IS NULL OR (status_code >= 100 AND status_code <= 599)),
    failure_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS model_probe_requests_model_time_idx
    ON model_probe_requests (group_id, model_id, created_at DESC, id DESC);
