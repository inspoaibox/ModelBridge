-- Keep the immutable official price version used by a paid request.
-- price_version_id remains reserved for manually published platform, tenant,
-- project, or token prices because it references price_versions.

ALTER TABLE model_requests
    ADD COLUMN IF NOT EXISTS official_price_version_id uuid
        REFERENCES official_model_price_versions(id);

CREATE INDEX IF NOT EXISTS model_requests_official_price_version_idx
    ON model_requests (official_price_version_id)
    WHERE official_price_version_id IS NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'model_requests'::regclass
          AND conname = 'model_requests_one_price_source_chk'
    ) THEN
        ALTER TABLE model_requests
            ADD CONSTRAINT model_requests_one_price_source_chk
            CHECK (price_version_id IS NULL OR official_price_version_id IS NULL);
    END IF;
END $$;
