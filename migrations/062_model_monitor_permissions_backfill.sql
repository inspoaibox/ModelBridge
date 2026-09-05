-- Model monitor writes require the same operations permissions as the
-- administrator model-status page. Keep bootstrap-created and pre-existing
-- platform owners equivalent even when migration 038 ran before the role was
-- created.
INSERT INTO platform_permissions (id, resource, action, name)
VALUES
    ('11111111-1111-4111-8111-111111111808', 'operations', 'read', 'operations:read'),
    ('11111111-1111-4111-8111-111111111903', 'operations', 'update', 'operations:update')
ON CONFLICT (resource, action) DO UPDATE SET name = EXCLUDED.name;

INSERT INTO platform_role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM platform_roles r
CROSS JOIN platform_permissions p
WHERE r.code = 'platform_owner'
  AND p.resource = 'operations'
  AND p.action IN ('read', 'update')
ON CONFLICT DO NOTHING;
