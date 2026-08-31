INSERT INTO models (
    id, provider, model_name, protocol_family, capabilities_json, status
) VALUES
    ('10000000-0000-4000-8000-000000000001', 'openai', 'gpt-5', 'openai_chat_completions', '{"modalities":["text"],"official_sdk":true}'::jsonb, 'active'),
    ('10000000-0000-4000-8000-000000000002', 'openai', 'gpt-5-mini', 'openai_chat_completions', '{"modalities":["text"],"official_sdk":true}'::jsonb, 'active'),
    ('10000000-0000-4000-8000-000000000101', 'anthropic', 'claude-sonnet-5', 'anthropic_messages', '{"modalities":["text"],"official_sdk":true}'::jsonb, 'active'),
    ('10000000-0000-4000-8000-000000000102', 'anthropic', 'claude-opus-5', 'anthropic_messages', '{"modalities":["text"],"official_sdk":true}'::jsonb, 'active')
ON CONFLICT (provider, model_name) DO UPDATE SET
    protocol_family = EXCLUDED.protocol_family,
    capabilities_json = EXCLUDED.capabilities_json,
    status = 'active',
    updated_at = now();
