ALTER TABLE api_tokens
    ADD COLUMN IF NOT EXISTS group_id uuid REFERENCES routing_groups(id);

CREATE INDEX IF NOT EXISTS api_tokens_group_idx
    ON api_tokens (group_id, status);

UPDATE api_tokens
SET group_id = (
    SELECT id
    FROM routing_groups
    WHERE code = 'default'
      AND deleted_at IS NULL
    LIMIT 1
)
WHERE group_id IS NULL;

INSERT INTO routing_group_channels (group_id, channel_id)
SELECT rg.id, c.id
FROM routing_groups rg
CROSS JOIN channels c
WHERE rg.code = 'default'
  AND rg.deleted_at IS NULL
  AND c.deleted_at IS NULL
ON CONFLICT DO NOTHING;
