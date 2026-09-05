CREATE TABLE IF NOT EXISTS announcements (
    id uuid PRIMARY KEY,
    title text NOT NULL,
    content text NOT NULL,
    effective_at timestamptz NOT NULL,
    expires_at timestamptz,
    enabled boolean NOT NULL DEFAULT true,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT announcements_title_length CHECK (char_length(title) BETWEEN 1 AND 200),
    CONSTRAINT announcements_content_length CHECK (char_length(content) BETWEEN 1 AND 50000),
    CONSTRAINT announcements_window_valid CHECK (expires_at IS NULL OR expires_at > effective_at)
);

CREATE INDEX IF NOT EXISTS announcements_active_idx
    ON announcements (enabled, effective_at, expires_at);

CREATE TABLE IF NOT EXISTS announcement_user_states (
    announcement_id uuid NOT NULL REFERENCES announcements(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    delivered_at timestamptz NOT NULL DEFAULT now(),
    read_at timestamptz,
    PRIMARY KEY (announcement_id, user_id)
);

CREATE INDEX IF NOT EXISTS announcement_user_states_user_idx
    ON announcement_user_states (user_id, delivered_at DESC);

CREATE INDEX IF NOT EXISTS announcement_user_states_announcement_idx
    ON announcement_user_states (announcement_id, delivered_at DESC);
