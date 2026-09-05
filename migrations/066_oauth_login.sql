CREATE TABLE IF NOT EXISTS oauth_identities (
    provider text NOT NULL,
    subject text NOT NULL,
    user_id uuid NOT NULL REFERENCES users(id),
    email text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, subject)
);

CREATE INDEX IF NOT EXISTS oauth_identities_user_idx ON oauth_identities (user_id);

CREATE TABLE IF NOT EXISTS oauth_states (
    state text PRIMARY KEY,
    provider text NOT NULL,
    redirect_uri text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS oauth_states_expiry_idx ON oauth_states (expires_at);
