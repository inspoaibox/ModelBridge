CREATE TABLE IF NOT EXISTS api_endpoints (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    base_url text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0,
    created_by uuid REFERENCES users(id),
    updated_by uuid REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS api_endpoints_base_url_unique_idx
    ON api_endpoints (lower(base_url));

CREATE INDEX IF NOT EXISTS api_endpoints_enabled_order_idx
    ON api_endpoints (enabled, sort_order, name);
