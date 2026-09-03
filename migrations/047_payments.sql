-- Payment provider secrets are encrypted JSON values owned by the application.
CREATE TABLE IF NOT EXISTS payment_provider_configs (
    provider text PRIMARY KEY CHECK (provider IN ('wechat', 'alipay', 'stripe', 'paypal')),
    enabled boolean NOT NULL DEFAULT false,
    config_ciphertext bytea NOT NULL DEFAULT '',
    updated_by uuid REFERENCES users(id),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS payment_orders (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    user_id uuid NOT NULL REFERENCES users(id),
    provider text NOT NULL CHECK (provider IN ('wechat', 'alipay', 'stripe', 'paypal')),
    merchant_order_no text NOT NULL UNIQUE,
    provider_order_id text,
    amount numeric(30, 12) NOT NULL CHECK (amount > 0),
    currency char(3) NOT NULL,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'paid', 'failed', 'cancelled', 'expired')),
    checkout_url text,
    qr_code text,
    request_payload_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    response_payload_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    failure_reason text,
    paid_at timestamptz,
    expires_at timestamptz NOT NULL,
    idempotency_key text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS payment_orders_tenant_idx
    ON payment_orders (tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS payment_orders_provider_idx
    ON payment_orders (provider, provider_order_id)
    WHERE provider_order_id IS NOT NULL;

INSERT INTO platform_permissions (id, resource, action, name)
VALUES
    ('11111111-1111-4111-8111-111111113101', 'payment', 'read', 'payment:read'),
    ('11111111-1111-4111-8111-111111113102', 'payment', 'update', 'payment:update')
ON CONFLICT (resource, action) DO UPDATE SET name = EXCLUDED.name;

INSERT INTO platform_role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM platform_roles r
CROSS JOIN platform_permissions p
WHERE r.code = 'platform_owner'
  AND p.resource = 'payment'
  AND p.action IN ('read', 'update')
ON CONFLICT DO NOTHING;
