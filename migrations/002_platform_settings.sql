CREATE TABLE IF NOT EXISTS platform_settings (
    key text PRIMARY KEY,
    value text NOT NULL,
    updated_by uuid REFERENCES users(id),
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO platform_settings (key, value)
VALUES ('admin_mfa_enabled', 'false')
ON CONFLICT (key) DO NOTHING;
