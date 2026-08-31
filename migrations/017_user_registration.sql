ALTER TABLE users
    ADD COLUMN IF NOT EXISTS display_name text NOT NULL DEFAULT '';

INSERT INTO platform_permissions (id, resource, action, name)
VALUES
    ('11111111-1111-4111-8111-111111111601', 'user', 'read', 'user:read'),
    ('11111111-1111-4111-8111-111111111602', 'user', 'update', 'user:update')
ON CONFLICT (resource, action) DO UPDATE SET
    name = EXCLUDED.name;

INSERT INTO platform_role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM platform_roles r
CROSS JOIN platform_permissions p
WHERE r.code = 'platform_owner'
  AND p.resource = 'user'
  AND p.action IN ('read', 'update')
ON CONFLICT DO NOTHING;
