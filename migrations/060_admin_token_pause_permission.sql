-- Platform administrators may pause a customer token for operational safety,
-- but token configuration and permanent revocation remain customer-owned.
INSERT INTO platform_permissions (id, resource, action, name)
VALUES ('11111111-1111-4111-8111-111111111809', 'token', 'pause', 'token:pause')
ON CONFLICT (resource, action) DO UPDATE SET name = EXCLUDED.name;

INSERT INTO platform_role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM platform_roles r
CROSS JOIN platform_permissions p
WHERE r.code = 'platform_owner'
  AND p.resource = 'token'
  AND p.action = 'pause'
ON CONFLICT DO NOTHING;

-- Remove legacy administrator-side token mutation grants. The same permission
-- names remain available to tenant users in their own console roles.
DELETE FROM platform_role_permissions rp
USING platform_permissions p
WHERE rp.permission_id = p.id
  AND p.resource = 'token'
  AND p.action IN ('create', 'update', 'revoke');
