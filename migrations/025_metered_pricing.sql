CREATE TABLE IF NOT EXISTS price_components (
    id uuid PRIMARY KEY,
    price_version_id uuid NOT NULL REFERENCES price_versions(id) ON DELETE CASCADE,
    component_code text NOT NULL,
    unit text NOT NULL,
    price_per_unit numeric(30, 18) NOT NULL CHECK (price_per_unit >= 0),
    tier_json jsonb NOT NULL DEFAULT '[]'::jsonb,
    metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (price_version_id, component_code)
);

CREATE INDEX IF NOT EXISTS price_components_code_idx
    ON price_components (component_code, price_version_id);

CREATE TABLE IF NOT EXISTS official_price_components (
    id uuid PRIMARY KEY,
    official_price_version_id uuid NOT NULL REFERENCES official_model_price_versions(id) ON DELETE CASCADE,
    component_code text NOT NULL,
    unit text NOT NULL,
    price_per_unit numeric(30, 18) NOT NULL CHECK (price_per_unit >= 0),
    tier_json jsonb NOT NULL DEFAULT '[]'::jsonb,
    metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (official_price_version_id, component_code)
);

ALTER TABLE official_model_price_versions
    ALTER COLUMN input_price_per_unit TYPE numeric(30, 18),
    ALTER COLUMN output_price_per_unit TYPE numeric(30, 18),
    ALTER COLUMN cached_input_price_per_unit TYPE numeric(30, 18),
    ALTER COLUMN reasoning_price_per_unit TYPE numeric(30, 18);

CREATE INDEX IF NOT EXISTS official_price_components_code_idx
    ON official_price_components (component_code, official_price_version_id);

ALTER TABLE model_requests
    ADD COLUMN IF NOT EXISTS usage_metrics_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS charge_breakdown_json jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS price_snapshot_json jsonb NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE usage_events
    ADD COLUMN IF NOT EXISTS usage_metrics_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS charge_breakdown_json jsonb NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE billing_reservations
    ADD COLUMN IF NOT EXISTS estimated_metrics_json jsonb NOT NULL DEFAULT '{}'::jsonb;

INSERT INTO price_components (id, price_version_id, component_code, unit, price_per_unit)
SELECT md5(pv.id::text || ':input_tokens')::uuid, pv.id, 'input_tokens', 'token', pv.input_price_per_unit
FROM price_versions pv
WHERE pv.input_price_per_unit > 0
ON CONFLICT (price_version_id, component_code) DO NOTHING;

INSERT INTO price_components (id, price_version_id, component_code, unit, price_per_unit)
SELECT md5(pv.id::text || ':output_tokens')::uuid, pv.id, 'output_tokens', 'token', pv.output_price_per_unit
FROM price_versions pv
WHERE pv.output_price_per_unit > 0
ON CONFLICT (price_version_id, component_code) DO NOTHING;

INSERT INTO price_components (id, price_version_id, component_code, unit, price_per_unit)
SELECT md5(pv.id::text || ':cached_input_tokens')::uuid, pv.id, 'cached_input_tokens', 'token', pv.cached_input_price_per_unit
FROM price_versions pv
WHERE pv.cached_input_price_per_unit > 0
ON CONFLICT (price_version_id, component_code) DO NOTHING;

INSERT INTO price_components (id, price_version_id, component_code, unit, price_per_unit)
SELECT md5(pv.id::text || ':reasoning_tokens')::uuid, pv.id, 'reasoning_tokens', 'token', pv.reasoning_price_per_unit
FROM price_versions pv
WHERE pv.reasoning_price_per_unit > 0
ON CONFLICT (price_version_id, component_code) DO NOTHING;

INSERT INTO official_price_components (id, official_price_version_id, component_code, unit, price_per_unit)
SELECT md5(omp.id::text || ':input_tokens')::uuid, omp.id, 'input_tokens', 'token', omp.input_price_per_unit
FROM official_model_price_versions omp
WHERE omp.input_price_per_unit > 0
ON CONFLICT (official_price_version_id, component_code) DO NOTHING;

INSERT INTO official_price_components (id, official_price_version_id, component_code, unit, price_per_unit)
SELECT md5(omp.id::text || ':output_tokens')::uuid, omp.id, 'output_tokens', 'token', omp.output_price_per_unit
FROM official_model_price_versions omp
WHERE omp.output_price_per_unit > 0
ON CONFLICT (official_price_version_id, component_code) DO NOTHING;

INSERT INTO official_price_components (id, official_price_version_id, component_code, unit, price_per_unit)
SELECT md5(omp.id::text || ':cached_input_tokens')::uuid, omp.id, 'cached_input_tokens', 'token', omp.cached_input_price_per_unit
FROM official_model_price_versions omp
WHERE omp.cached_input_price_per_unit > 0
ON CONFLICT (official_price_version_id, component_code) DO NOTHING;

INSERT INTO official_price_components (id, official_price_version_id, component_code, unit, price_per_unit)
SELECT md5(omp.id::text || ':reasoning_tokens')::uuid, omp.id, 'reasoning_tokens', 'token', omp.reasoning_price_per_unit
FROM official_model_price_versions omp
WHERE omp.reasoning_price_per_unit > 0
ON CONFLICT (official_price_version_id, component_code) DO NOTHING;
