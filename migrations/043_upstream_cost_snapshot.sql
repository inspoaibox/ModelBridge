-- Keep the channel cost factor and the resulting estimate immutable for each
-- request. A later channel configuration change must not rewrite history.

ALTER TABLE model_requests
    ADD COLUMN IF NOT EXISTS upstream_cost_discount numeric(40, 30) NOT NULL DEFAULT 1
        CHECK (upstream_cost_discount >= 0 AND upstream_cost_discount <= 1000),
    ADD COLUMN IF NOT EXISTS estimated_upstream_cost numeric(40, 30) NOT NULL DEFAULT 0
        CHECK (estimated_upstream_cost >= 0),
    ADD COLUMN IF NOT EXISTS upstream_cost numeric(40, 30) NOT NULL DEFAULT 0
        CHECK (upstream_cost >= 0);

CREATE INDEX IF NOT EXISTS model_requests_upstream_cost_idx
    ON model_requests (upstream_cost, created_at DESC);
