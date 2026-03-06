-- =============================================================================
-- Reset installation_tokens (and agents) to match Connection Manager Go code
-- =============================================================================
-- Run against the same PostgreSQL database the Connection Manager uses (e.g. sigma).
-- Usage: psql -U sigma -d sigma -f reset_installation_tokens_schema.sql
-- Or:   docker exec -i edr_server-postgres-1 psql -U sigma -d sigma < scripts/reset_installation_tokens_schema.sql
-- =============================================================================

BEGIN;

-- -----------------------------------------------------------------------------
-- 1) Drop objects that depend on installation_tokens (none by default)
--    Then drop installation_tokens. Order matters for FK: tokens reference agents.
-- -----------------------------------------------------------------------------
DROP FUNCTION IF EXISTS cleanup_expired_installation_tokens();
DROP TABLE IF EXISTS installation_tokens;

-- -----------------------------------------------------------------------------
-- 2) Agents table (required by installation_tokens.agent_id FK)
--    Matches: internal/database/migrations/001_create_agents.up.sql
--    and: AgentRepository / pkg/models usage (id UUID, hostname, status, ...)
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS agents (
    id UUID PRIMARY KEY,
    hostname VARCHAR(255) NOT NULL UNIQUE,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    os_type VARCHAR(50),
    os_version VARCHAR(100),
    cpu_count INTEGER,
    memory_mb BIGINT,
    agent_version VARCHAR(50),
    installed_date TIMESTAMPTZ,
    last_seen TIMESTAMPTZ,
    events_collected BIGINT DEFAULT 0,
    events_delivered BIGINT DEFAULT 0,
    queue_depth INTEGER DEFAULT 0,
    cpu_usage FLOAT DEFAULT 0,
    memory_used_mb BIGINT DEFAULT 0,
    health_score FLOAT DEFAULT 100.0,
    current_cert_id UUID,
    cert_expires_at TIMESTAMPTZ,
    tags JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
CREATE INDEX IF NOT EXISTS idx_agents_hostname ON agents(hostname);
CREATE INDEX IF NOT EXISTS idx_agents_last_seen ON agents(last_seen);
CREATE INDEX IF NOT EXISTS idx_agents_cert_expires ON agents(cert_expires_at);
CREATE INDEX IF NOT EXISTS idx_agents_os_type ON agents(os_type);
CREATE INDEX IF NOT EXISTS idx_agents_health_score ON agents(health_score);

-- Trigger for updated_at (PG compatibility: use EXECUTE PROCEDURE)
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS update_agents_updated_at ON agents;
CREATE TRIGGER update_agents_updated_at
    BEFORE UPDATE ON agents
    FOR EACH ROW
    EXECUTE PROCEDURE update_updated_at_column();

-- -----------------------------------------------------------------------------
-- 3) installation_tokens table
--    Matches: internal/database/migrations/005_create_tokens.up.sql
--    and: PostgresInstallationTokenRepository.GetByValue() Scan order:
--        id, token_value, agent_id, used, used_at, created_at, expires_at
--    (pkg/models/certificate.go InstallationToken struct)
-- -----------------------------------------------------------------------------
CREATE TABLE installation_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_value VARCHAR(255) UNIQUE NOT NULL,
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    used BOOLEAN DEFAULT FALSE,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours'
);
CREATE INDEX idx_installation_tokens_token ON installation_tokens(token_value);
CREATE INDEX idx_installation_tokens_expires ON installation_tokens(expires_at);
CREATE INDEX idx_installation_tokens_used ON installation_tokens(used);

CREATE OR REPLACE FUNCTION cleanup_expired_installation_tokens()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM installation_tokens
    WHERE expires_at < NOW() AND used = FALSE;
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- -----------------------------------------------------------------------------
-- 4) Seed test token: exact value the agent sends, valid until 2027, unused
--    GetByValue(ctx, "EDR-SUPER-SECRET-TOKEN-2026") must return this row.
-- -----------------------------------------------------------------------------
INSERT INTO installation_tokens (token_value, expires_at)
VALUES ('EDR-SUPER-SECRET-TOKEN-2026', '2027-01-01 00:00:00+00')
ON CONFLICT (token_value) DO UPDATE SET
    expires_at = EXCLUDED.expires_at,
    used = FALSE,
    used_at = NULL,
    agent_id = NULL;

COMMIT;

-- Verify (optional)
-- SELECT id, token_value, used, expires_at FROM installation_tokens WHERE token_value = 'EDR-SUPER-SECRET-TOKEN-2026';
