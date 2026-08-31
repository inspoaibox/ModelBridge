ALTER TABLE price_versions
    ALTER COLUMN scope_id DROP NOT NULL;

UPDATE price_versions
SET scope_id = NULL
WHERE scope_type = 'platform_default';

INSERT INTO platform_permissions (id, resource, action, name)
VALUES
    ('11111111-1111-4111-8111-111111111301', 'price', 'read', 'price:read'),
    ('11111111-1111-4111-8111-111111111302', 'price', 'publish', 'price:publish'),
    ('11111111-1111-4111-8111-111111111303', 'billing', 'read', 'billing:read'),
    ('11111111-1111-4111-8111-111111111304', 'billing', 'update', 'billing:update')
ON CONFLICT (resource, action) DO UPDATE SET
    name = EXCLUDED.name;

INSERT INTO platform_role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM platform_roles r
CROSS JOIN platform_permissions p
WHERE r.code = 'platform_owner'
  AND (
      (p.resource = 'price' AND p.action IN ('read', 'publish'))
      OR
      (p.resource = 'billing' AND p.action IN ('read', 'update'))
  )
ON CONFLICT DO NOTHING;
