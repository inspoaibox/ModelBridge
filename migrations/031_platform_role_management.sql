-- Platform roles are managed from the admin console, while customer accounts
-- continue to be created only through public console registration.
INSERT INTO platform_permissions (id, resource, action, name)
VALUES
    ('11111111-1111-4111-8111-111111111901', 'role', 'read', 'role:read'),
    ('11111111-1111-4111-8111-111111111902', 'role', 'update', 'role:update')
ON CONFLICT (resource, action) DO UPDATE SET name = EXCLUDED.name;

INSERT INTO platform_role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM platform_roles r
CROSS JOIN platform_permissions p
WHERE r.code = 'platform_owner'
  AND p.resource = 'role'
  AND p.action IN ('read', 'update')
ON CONFLICT DO NOTHING;
