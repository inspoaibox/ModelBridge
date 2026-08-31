INSERT INTO platform_permissions (id, resource, action, name)
VALUES
    ('11111111-1111-4111-8111-111111111503', 'token', 'create', 'token:create'),
    ('11111111-1111-4111-8111-111111111504', 'token', 'revoke', 'token:revoke')
ON CONFLICT (resource, action) DO UPDATE SET
    name = EXCLUDED.name;

INSERT INTO platform_role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM platform_roles r
CROSS JOIN platform_permissions p
WHERE r.code = 'platform_owner'
  AND p.resource = 'token'
  AND p.action IN ('create', 'revoke')
ON CONFLICT DO NOTHING;
