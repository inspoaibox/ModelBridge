CREATE UNIQUE INDEX IF NOT EXISTS mfa_credentials_one_active_totp
    ON mfa_credentials (user_id)
    WHERE type = 'totp' AND status = 'active' AND revoked_at IS NULL;
