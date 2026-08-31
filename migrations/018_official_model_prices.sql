CREATE TABLE IF NOT EXISTS official_model_price_versions (
    id uuid PRIMARY KEY,
    model_id uuid NOT NULL REFERENCES models(id),
    source text NOT NULL,
    source_url text NOT NULL,
    source_model_key text NOT NULL,
    currency char(3) NOT NULL DEFAULT 'USD',
    input_price_per_unit numeric(24, 12) NOT NULL CHECK (input_price_per_unit >= 0),
    output_price_per_unit numeric(24, 12) NOT NULL CHECK (output_price_per_unit >= 0),
    cached_input_price_per_unit numeric(24, 12) NOT NULL DEFAULT 0 CHECK (cached_input_price_per_unit >= 0),
    reasoning_price_per_unit numeric(24, 12) NOT NULL DEFAULT 0 CHECK (reasoning_price_per_unit >= 0),
    effective_from timestamptz NOT NULL DEFAULT now(),
    effective_to timestamptz,
    fetched_at timestamptz NOT NULL DEFAULT now(),
    metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS official_model_prices_lookup_idx
    ON official_model_price_versions (model_id, source, effective_from DESC);

CREATE UNIQUE INDEX IF NOT EXISTS official_model_prices_current_unique
    ON official_model_price_versions (model_id, source)
    WHERE effective_to IS NULL;
