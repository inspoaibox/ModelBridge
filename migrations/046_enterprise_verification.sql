-- Enterprise qualification submissions are tenant-scoped. Business license
-- bytes and bank accounts are encrypted application values, never public files.
CREATE TABLE IF NOT EXISTS enterprise_verifications (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    submitted_by uuid NOT NULL REFERENCES users(id),
    enterprise_name text NOT NULL,
    unified_credit_code text NOT NULL,
    license_filename text NOT NULL,
    license_content_type text NOT NULL,
    license_size bigint NOT NULL CHECK (license_size > 0 AND license_size <= 10485760),
    license_sha256 text NOT NULL,
    license_ciphertext bytea NOT NULL,
    bank_account_name text NOT NULL,
    bank_name text NOT NULL,
    bank_account_ciphertext text NOT NULL,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'rejected')),
    rejection_reason text,
    reviewed_by uuid REFERENCES users(id),
    reviewed_at timestamptz,
    submitted_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS enterprise_verifications_status_idx
    ON enterprise_verifications (status, submitted_at DESC);

CREATE INDEX IF NOT EXISTS enterprise_verifications_tenant_idx
    ON enterprise_verifications (tenant_id, submitted_at DESC);

INSERT INTO platform_permissions (id, resource, action, name)
VALUES
    ('11111111-1111-4111-8111-111111113001', 'enterprise', 'read', 'enterprise:read'),
    ('11111111-1111-4111-8111-111111113002', 'enterprise', 'update', 'enterprise:update')
ON CONFLICT (resource, action) DO UPDATE SET name = EXCLUDED.name;

INSERT INTO platform_role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM platform_roles r
CROSS JOIN platform_permissions p
WHERE r.code = 'platform_owner'
  AND p.resource = 'enterprise'
  AND p.action IN ('read', 'update')
ON CONFLICT DO NOTHING;
