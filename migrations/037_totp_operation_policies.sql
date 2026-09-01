-- TOTP is an optional platform capability. These settings control which
-- administrator operation categories require an additional one-time code.
--
-- Existing installations that already enforced administrator MFA keep the
-- previous protection level. Fresh installations begin with optional
-- operation policies so administrators can opt in by category.
INSERT INTO platform_settings (key, value)
SELECT
    setting_key,
    CASE
        WHEN EXISTS (
            SELECT 1
            FROM platform_settings
            WHERE key = 'admin_mfa_enabled' AND value = 'true'
        ) THEN 'true'
        ELSE 'false'
    END
FROM (
    VALUES
        ('step_up_channel_model_enabled'),
        ('step_up_group_enabled'),
        ('step_up_token_enabled'),
        ('step_up_user_enabled'),
        ('step_up_role_enabled'),
        ('step_up_billing_enabled'),
        ('step_up_system_enabled')
) AS defaults(setting_key)
ON CONFLICT (key) DO NOTHING;
