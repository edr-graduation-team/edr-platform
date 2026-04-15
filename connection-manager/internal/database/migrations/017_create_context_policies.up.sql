CREATE TABLE IF NOT EXISTS context_policies (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    scope_type TEXT NOT NULL CHECK (scope_type IN ('global', 'agent', 'user')),
    scope_value TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    user_role_weight DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    device_criticality_weight DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    network_anomaly_factor DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    trusted_networks JSONB NOT NULL DEFAULT '[]'::jsonb,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_context_policies_scope
    ON context_policies(scope_type, scope_value);

CREATE INDEX IF NOT EXISTS idx_context_policies_enabled_scope
    ON context_policies(enabled, scope_type);

INSERT INTO context_policies (
    name, scope_type, scope_value, enabled,
    user_role_weight, device_criticality_weight, network_anomaly_factor,
    trusted_networks, notes
)
VALUES (
    'Global Default Context',
    'global',
    '*',
    TRUE,
    1.0, 1.0, 1.0,
    '[]'::jsonb,
    'Default global context-aware weights'
)
ON CONFLICT (scope_type, scope_value) DO NOTHING;

