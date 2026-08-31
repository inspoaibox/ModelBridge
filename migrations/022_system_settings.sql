INSERT INTO platform_settings (key, value)
VALUES
    ('site_name', 'AI Token Gateway'),
    ('site_logo_url', ''),
    ('site_favicon_url', '')
ON CONFLICT (key) DO NOTHING;
