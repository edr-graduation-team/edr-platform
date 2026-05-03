-- Migration: 015_create_roles_permissions
-- Granular RBAC: roles, permissions, and role-permission mapping tables.
-- Aligns backend with 5-role frontend model: admin, security, analyst, operations, viewer.

-- ─── Roles ───────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(50)  UNIQUE NOT NULL,
    description TEXT         NOT NULL DEFAULT '',
    is_built_in BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TRIGGER update_roles_updated_at
    BEFORE UPDATE ON roles
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE roles IS 'RBAC roles — both built-in and custom';

-- ─── Permissions ─────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS permissions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource    VARCHAR(50)  NOT NULL,
    action      VARCHAR(50)  NOT NULL,
    description TEXT         NOT NULL DEFAULT '',
    UNIQUE (resource, action)
);

COMMENT ON TABLE permissions IS 'Granular permissions: resource:action pairs';

-- ─── Role ↔ Permission junction ──────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS role_permissions (
    role_id       UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

CREATE INDEX IF NOT EXISTS idx_role_permissions_role ON role_permissions(role_id);
CREATE INDEX IF NOT EXISTS idx_role_permissions_perm ON role_permissions(permission_id);

COMMENT ON TABLE role_permissions IS 'Maps roles to their granted permissions';

-- ═════════════════════════════════════════════════════════════════════════════
-- SEED: Built-in roles
-- ═════════════════════════════════════════════════════════════════════════════

INSERT INTO roles (name, description, is_built_in) VALUES
    ('admin',      'Full platform access — manages users, roles, and all settings',    TRUE),
    ('security',   'Security operations — alert triage, endpoint isolation, audit view', TRUE),
    ('analyst',    'Alert investigation and response execution',                         TRUE),
    ('operations', 'Infrastructure monitoring — endpoint and agent health oversight',    TRUE),
    ('viewer',     'Read-only dashboard access',                                         TRUE)
ON CONFLICT (name) DO NOTHING;

-- ═════════════════════════════════════════════════════════════════════════════
-- SEED: Permissions
-- ═════════════════════════════════════════════════════════════════════════════

INSERT INTO permissions (resource, action, description) VALUES
    -- Alerts
    ('alerts',    'read',    'View alerts and alert details'),
    ('alerts',    'write',   'Update alert status, assign, add notes'),
    ('alerts',    'delete',  'Delete alerts'),
    -- Endpoints
    ('endpoints', 'read',    'View endpoint list and details'),
    ('endpoints', 'manage',  'Update endpoint tags, delete endpoints'),
    ('endpoints', 'isolate', 'Network-isolate or restore endpoints'),
    -- Rules
    ('rules',     'read',    'View Sigma detection rules'),
    ('rules',     'write',   'Create, edit, enable/disable rules'),
    ('rules',     'delete',  'Delete detection rules'),
    -- Responses (Action Center)
    ('responses', 'read',    'View command history and action center'),
    ('responses', 'execute', 'Execute remote commands on endpoints'),
    -- Settings
    ('settings',  'read',    'View platform settings'),
    ('settings',  'write',   'Modify platform configuration'),
    -- Users
    ('users',     'read',    'View user accounts'),
    ('users',     'write',   'Create and update user accounts'),
    ('users',     'delete',  'Deactivate or remove user accounts'),
    -- Roles
    ('roles',     'read',    'View roles and permissions'),
    ('roles',     'write',   'Create, edit, delete custom roles'),
    -- Audit
    ('audit',     'read',    'View audit log entries'),
    -- Enrollment Tokens
    ('tokens',    'read',    'View enrollment tokens'),
    ('tokens',    'write',   'Generate and revoke enrollment tokens'),
    -- Agents (build & package management — admin only)
    ('agents',    'write',   'Build agent installers and create agent packages')
ON CONFLICT (resource, action) DO NOTHING;

-- ═════════════════════════════════════════════════════════════════════════════
-- SEED: Role ↔ Permission mappings
-- ═════════════════════════════════════════════════════════════════════════════

-- Helper: insert by name so the seed is idempotent regardless of UUID generation.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE (r.name, p.resource, p.action) IN (
    -- ── Admin: everything ──
    ('admin', 'alerts',    'read'),
    ('admin', 'alerts',    'write'),
    ('admin', 'alerts',    'delete'),
    ('admin', 'endpoints', 'read'),
    ('admin', 'endpoints', 'manage'),
    ('admin', 'endpoints', 'isolate'),
    ('admin', 'rules',     'read'),
    ('admin', 'rules',     'write'),
    ('admin', 'rules',     'delete'),
    ('admin', 'responses', 'read'),
    ('admin', 'responses', 'execute'),
    ('admin', 'settings',  'read'),
    ('admin', 'settings',  'write'),
    ('admin', 'users',     'read'),
    ('admin', 'users',     'write'),
    ('admin', 'users',     'delete'),
    ('admin', 'roles',     'read'),
    ('admin', 'roles',     'write'),
    ('admin', 'audit',     'read'),
    ('admin', 'tokens',    'read'),
    ('admin', 'tokens',    'write'),
    ('admin', 'agents',    'write'),

    -- ── Security: triage + isolate + audit, no settings/user mgmt ──
    ('security', 'alerts',    'read'),
    ('security', 'alerts',    'write'),
    ('security', 'alerts',    'delete'),
    ('security', 'endpoints', 'read'),
    ('security', 'endpoints', 'manage'),
    ('security', 'endpoints', 'isolate'),
    ('security', 'rules',     'read'),
    ('security', 'rules',     'write'),
    ('security', 'responses', 'read'),
    ('security', 'responses', 'execute'),
    ('security', 'settings',  'read'),
    ('security', 'users',     'read'),
    ('security', 'roles',     'read'),
    ('security', 'audit',     'read'),
    ('security', 'tokens',    'read'),

    -- ── Analyst: investigate + respond, no admin features ──
    ('analyst', 'alerts',    'read'),
    ('analyst', 'alerts',    'write'),
    ('analyst', 'endpoints', 'read'),
    ('analyst', 'rules',     'read'),
    ('analyst', 'responses', 'read'),
    ('analyst', 'responses', 'execute'),
    ('analyst', 'settings',  'read'),
    ('analyst', 'tokens',    'read'),

    -- ── Operations: infrastructure monitoring ──
    ('operations', 'alerts',    'read'),
    ('operations', 'endpoints', 'read'),
    ('operations', 'endpoints', 'manage'),
    ('operations', 'responses', 'read'),
    ('operations', 'settings',  'read'),
    ('operations', 'tokens',    'read'),

    -- ── Viewer: read-only ──
    ('viewer', 'alerts',    'read'),
    ('viewer', 'endpoints', 'read'),
    ('viewer', 'rules',     'read'),
    ('viewer', 'settings',  'read'),
    ('viewer', 'tokens',    'read')
)
ON CONFLICT DO NOTHING;

-- ═════════════════════════════════════════════════════════════════════════════
-- Align existing users.role with new roles table
-- ═════════════════════════════════════════════════════════════════════════════

-- Add the two new role values to any CHECK constraint or validate at app level.
-- The column remains VARCHAR for backward compat; the roles table is source of truth.

COMMENT ON COLUMN users.role IS 'User role: admin, security, analyst, operations, viewer';
