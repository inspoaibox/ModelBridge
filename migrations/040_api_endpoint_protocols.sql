-- API endpoints are stored as a gateway root. Existing rows ending in /v1
-- are migrated only when the normalized value is unambiguous. Conflicts are
-- intentionally left untouched so this migration never deletes data or
-- violates the existing unique index.
WITH candidates AS (
    SELECT
        id,
        regexp_replace(base_url, '/v1/?$', '', 1, 1, 'i') AS root
    FROM api_endpoints
    WHERE base_url ~* '/v1/?$'
),
eligible AS (
    SELECT c.id, c.root
    FROM candidates c
    WHERE NOT EXISTS (
        SELECT 1
        FROM api_endpoints existing
        WHERE existing.id <> c.id
          AND lower(existing.base_url) = lower(c.root)
    )
      AND NOT EXISTS (
        SELECT 1
        FROM candidates other
        WHERE other.id <> c.id
          AND lower(other.root) = lower(c.root)
    )
)
UPDATE api_endpoints endpoint
SET base_url = eligible.root,
    updated_at = now()
FROM eligible
WHERE endpoint.id = eligible.id;
