-- Model status is an optional customer-facing feature. Keep it enabled by
-- default so existing deployments retain the current console experience.
INSERT INTO platform_settings (key, value)
VALUES ('model_status_enabled', 'true')
ON CONFLICT (key) DO NOTHING;
