-- Rollback: 005_create_tokens

DROP FUNCTION IF EXISTS cleanup_expired_installation_tokens;
DROP INDEX IF EXISTS idx_installation_tokens_used;
DROP INDEX IF EXISTS idx_installation_tokens_expires;
DROP INDEX IF EXISTS idx_installation_tokens_token;
DROP TABLE IF EXISTS installation_tokens;
