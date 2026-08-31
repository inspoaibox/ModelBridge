ALTER TABLE model_requests
    ADD COLUMN IF NOT EXISTS endpoint text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS client_ip text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS request_type text NOT NULL DEFAULT 'sync',
    ADD COLUMN IF NOT EXISTS reasoning_effort text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS latency_ms bigint NOT NULL DEFAULT 0
        CHECK (latency_ms >= 0);

CREATE INDEX IF NOT EXISTS model_requests_created_idx
    ON model_requests (created_at DESC, id DESC);

INSERT INTO platform_permissions (id, resource, action, name)
VALUES
    ('11111111-1111-4111-8111-111111111701', 'usage', 'read', 'usage:read'),
    ('11111111-1111-4111-8111-111111111702', 'finance', 'read', 'finance:read')
ON CONFLICT (resource, action) DO UPDATE SET
    name = EXCLUDED.name;

INSERT INTO platform_role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM platform_roles r
CROSS JOIN platform_permissions p
WHERE r.code = 'platform_owner'
  AND ((p.resource = 'usage' AND p.action = 'read')
    OR (p.resource = 'finance' AND p.action = 'read'))
ON CONFLICT DO NOTHING;
