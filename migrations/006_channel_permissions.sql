INSERT INTO platform_permissions (id, resource, action, name)
VALUES
    ('11111111-1111-4111-8111-111111111201', 'channel', 'read', 'channel:read'),
    ('11111111-1111-4111-8111-111111111202', 'channel', 'update', 'channel:update'),
    ('11111111-1111-4111-8111-111111111203', 'channel', 'read_secret', 'channel:read_secret')
ON CONFLICT (resource, action) DO UPDATE SET
    name = EXCLUDED.name;

INSERT INTO platform_role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM platform_roles r
CROSS JOIN platform_permissions p
WHERE r.code = 'platform_owner'
  AND p.resource = 'channel'
  AND p.action IN ('read', 'update', 'read_secret')
ON CONFLICT DO NOTHING;
