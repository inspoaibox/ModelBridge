UPDATE channels
SET status = 'disabled',
    deleted_at = now(),
    updated_at = now()
WHERE id IN (
    '20000000-0000-4000-8000-000000000001',
    '20000000-0000-4000-8000-000000000002'
)
  AND credential_ref IN ('env:OPENAI_API_KEY', 'env:ANTHROPIC_API_KEY')
  AND deleted_at IS NULL;
