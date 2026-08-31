CREATE TABLE IF NOT EXISTS routing_groups (
    id uuid PRIMARY KEY,
    code text NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'disabled')),
    multiplier numeric(12, 6) NOT NULL DEFAULT 1.000000
        CHECK (multiplier > 0 AND multiplier <= 1000),
    rpm_limit integer NOT NULL DEFAULT 0
        CHECK (rpm_limit >= 0),
    billing_type text NOT NULL DEFAULT 'prepaid'
        CHECK (billing_type IN ('prepaid', 'free')),
    priority integer NOT NULL DEFAULT 100
        CHECK (priority >= 0 AND priority <= 10000),
    created_by uuid REFERENCES users(id),
    updated_by uuid REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS routing_groups_code_unique
    ON routing_groups (lower(code))
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS routing_groups_status_priority_idx
    ON routing_groups (status, priority DESC, code);

CREATE TABLE IF NOT EXISTS routing_group_channels (
    group_id uuid NOT NULL REFERENCES routing_groups(id) ON DELETE CASCADE,
    channel_id uuid NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, channel_id)
);

CREATE INDEX IF NOT EXISTS routing_group_channels_channel_idx
    ON routing_group_channels (channel_id, group_id);

INSERT INTO routing_groups (
    id, code, name, description, status, multiplier, rpm_limit,
    billing_type, priority
) VALUES (
    '11111111-1111-4111-8111-111111111401',
    'default',
    '默认分组',
    '平台默认路由分组',
    'active',
    1.000000,
    0,
    'prepaid',
    100
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO platform_permissions (id, resource, action, name)
VALUES
    ('11111111-1111-4111-8111-111111111401', 'group', 'read', 'group:read'),
    ('11111111-1111-4111-8111-111111111402', 'group', 'update', 'group:update')
ON CONFLICT (resource, action) DO UPDATE SET
    name = EXCLUDED.name;

INSERT INTO platform_role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM platform_roles r
CROSS JOIN platform_permissions p
WHERE r.code = 'platform_owner'
  AND p.resource = 'group'
  AND p.action IN ('read', 'update')
ON CONFLICT DO NOTHING;
