-- Rollback: 002_create_certificates

DROP INDEX IF EXISTS idx_certificates_agent_status;
DROP INDEX IF EXISTS idx_certificates_expires_at;
DROP INDEX IF EXISTS idx_certificates_status;
DROP INDEX IF EXISTS idx_certificates_fingerprint;
DROP INDEX IF EXISTS idx_certificates_agent_id;
DROP TABLE IF EXISTS certificates;
