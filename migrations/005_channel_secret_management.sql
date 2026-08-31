ALTER TABLE channels
    ADD COLUMN IF NOT EXISTS created_by uuid REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS updated_by uuid REFERENCES users(id);

CREATE TABLE IF NOT EXISTS channel_secrets (
    id uuid PRIMARY KEY,
    channel_id uuid NOT NULL REFERENCES channels(id),
    secret_kind text NOT NULL DEFAULT 'api_key'
        CHECK (secret_kind IN ('api_key')),
    encrypted_secret bytea NOT NULL,
    secret_prefix text NOT NULL DEFAULT '',
    secret_suffix text NOT NULL DEFAULT '',
    created_by uuid REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    rotated_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS channel_secrets_active_unique
    ON channel_secrets (channel_id, secret_kind)
    WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS channel_secrets_channel_idx
    ON channel_secrets (channel_id, revoked_at);
