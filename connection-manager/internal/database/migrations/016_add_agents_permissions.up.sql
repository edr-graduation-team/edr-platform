-- Migration: 016_add_agents_permissions
-- Adds missing agent deployment permissions (agents:read, agents:write)
-- and grants them to appropriate roles.

-- ─── Add missing permissions ─────────────────────────────────────────────────

INSERT INTO permissions (resource, action, description) VALUES
    ('agents',    'read',    'View agent deployment page and build history'),
    ('agents',    'write',   'Build and download agent binaries')
ON CONFLICT (resource, action) DO NOTHING;

-- ─── Grant to roles ──────────────────────────────────────────────────────────

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE (r.name, p.resource, p.action) IN (
    -- Admin: full access to agents
    ('admin',    'agents', 'read'),
    ('admin',    'agents', 'write'),

    -- Security: can deploy agents
    ('security', 'agents', 'read'),
    ('security', 'agents', 'write'),

    -- Operations: can view agent deployment
    ('operations', 'agents', 'read')
)
ON CONFLICT DO NOTHING;
