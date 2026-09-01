-- SMTP settings use the encrypted platform_settings key/value store. The
-- password is intentionally not seeded; the application writes ciphertext.
INSERT INTO platform_settings (key, value)
VALUES
    ('smtp_addr', ''),
    ('smtp_from', ''),
    ('smtp_username', ''),
    ('public_base_url', '')
ON CONFLICT (key) DO NOTHING;
