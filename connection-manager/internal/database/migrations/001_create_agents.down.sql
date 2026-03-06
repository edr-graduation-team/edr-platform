-- Rollback: 001_create_agents

DROP TRIGGER IF EXISTS update_agents_updated_at ON agents;
DROP FUNCTION IF EXISTS update_updated_at_column();
DROP INDEX IF EXISTS idx_agents_health_score;
DROP INDEX IF EXISTS idx_agents_os_type;
DROP INDEX IF EXISTS idx_agents_cert_expires;
DROP INDEX IF EXISTS idx_agents_last_seen;
DROP INDEX IF EXISTS idx_agents_hostname;
DROP INDEX IF EXISTS idx_agents_status;
DROP TABLE IF EXISTS agents;
