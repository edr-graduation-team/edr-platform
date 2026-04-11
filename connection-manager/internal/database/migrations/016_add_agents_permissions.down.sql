-- Rollback: 016_add_agents_permissions

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE resource = 'agents'
);

DELETE FROM permissions WHERE resource = 'agents';
