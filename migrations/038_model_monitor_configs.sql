CREATE TABLE IF NOT EXISTS model_monitor_configs (
    id uuid PRIMARY KEY,
    group_id uuid NOT NULL REFERENCES routing_groups(id) ON DELETE CASCADE,
    name text NOT NULL,
    selection_mode text NOT NULL DEFAULT 'all'
        CHECK (selection_mode IN ('all', 'selected')),
    mode text NOT NULL DEFAULT 'passive'
        CHECK (mode IN ('passive', 'active')),
    probe_interval_seconds integer NOT NULL DEFAULT 300
        CHECK (probe_interval_seconds >= 60 AND probe_interval_seconds <= 86400),
    enabled boolean NOT NULL DEFAULT true,
    probe_started_at timestamptz,
    last_probe_started_at timestamptz,
    last_probe_finished_at timestamptz,
    last_probe_status text NOT NULL DEFAULT ''
        CHECK (last_probe_status IN ('', 'success', 'failed', 'skipped')),
    last_probe_error text NOT NULL DEFAULT '',
    created_by uuid REFERENCES users(id),
    updated_by uuid REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (group_id)
);

CREATE INDEX IF NOT EXISTS model_monitor_configs_due_idx
    ON model_monitor_configs (enabled, mode, last_probe_finished_at, probe_started_at);

CREATE TABLE IF NOT EXISTS model_monitor_config_models (
    config_id uuid NOT NULL REFERENCES model_monitor_configs(id) ON DELETE CASCADE,
    model_id uuid NOT NULL REFERENCES models(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (config_id, model_id)
);

CREATE INDEX IF NOT EXISTS model_monitor_config_models_model_idx
    ON model_monitor_config_models (model_id, config_id);

INSERT INTO platform_permissions (id, resource, action, name)
VALUES
    ('11111111-1111-4111-8111-111111111901', 'operations', 'update', 'operations:update')
ON CONFLICT (resource, action) DO UPDATE SET name = EXCLUDED.name;

INSERT INTO platform_role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM platform_roles r
CROSS JOIN platform_permissions p
WHERE r.code = 'platform_owner'
  AND p.resource = 'operations'
  AND p.action = 'update'
ON CONFLICT DO NOTHING;
