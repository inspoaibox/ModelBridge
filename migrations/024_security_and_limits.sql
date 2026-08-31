-- Keep bootstrap-admin and already provisioned platform owners equivalent.
INSERT INTO platform_permissions (id, resource, action, name)
VALUES
    ('11111111-1111-4111-8111-111111111801', 'group', 'read', 'group:read'),
    ('11111111-1111-4111-8111-111111111802', 'group', 'update', 'group:update'),
    ('11111111-1111-4111-8111-111111111803', 'token', 'read', 'token:read'),
    ('11111111-1111-4111-8111-111111111804', 'token', 'update', 'token:update'),
    ('11111111-1111-4111-8111-111111111805', 'price', 'read', 'price:read'),
    ('11111111-1111-4111-8111-111111111806', 'billing', 'update', 'billing:update'),
    ('11111111-1111-4111-8111-111111111807', 'finance', 'read', 'finance:read'),
    ('11111111-1111-4111-8111-111111111808', 'operations', 'read', 'operations:read')
ON CONFLICT (resource, action) DO UPDATE SET name = EXCLUDED.name;

INSERT INTO platform_role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM platform_roles r
CROSS JOIN platform_permissions p
WHERE r.code = 'platform_owner'
  AND p.resource || ':' || p.action IN (
      'group:read', 'group:update', 'token:read', 'token:update',
      'price:read', 'billing:update', 'finance:read', 'operations:read'
  )
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS api_token_rate_windows (
    token_id uuid NOT NULL REFERENCES api_tokens(id) ON DELETE CASCADE,
    window_start timestamptz NOT NULL,
    request_count bigint NOT NULL DEFAULT 0 CHECK (request_count >= 0),
    token_count bigint NOT NULL DEFAULT 0 CHECK (token_count >= 0),
    PRIMARY KEY (token_id, window_start)
);

CREATE INDEX IF NOT EXISTS api_token_rate_windows_cleanup_idx
    ON api_token_rate_windows (window_start);

CREATE TABLE IF NOT EXISTS api_token_concurrency (
    token_id uuid PRIMARY KEY REFERENCES api_tokens(id) ON DELETE CASCADE,
    active_count integer NOT NULL DEFAULT 0 CHECK (active_count >= 0)
);
