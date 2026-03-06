-- Rollback: 006_create_csrs

DROP FUNCTION IF EXISTS cleanup_expired_csrs;
DROP INDEX IF EXISTS idx_csrs_expires;
DROP INDEX IF EXISTS idx_csrs_approved;
DROP INDEX IF EXISTS idx_csrs_agent_id;
DROP TABLE IF EXISTS csrs;
