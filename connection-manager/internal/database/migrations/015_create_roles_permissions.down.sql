-- Migration: 015_create_roles_permissions (DOWN)
-- Reverse the RBAC schema additions.

DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS permissions;
DROP TABLE IF EXISTS roles;

COMMENT ON COLUMN users.role IS 'User role: admin, analyst, viewer';
