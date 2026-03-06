-- Fix installation_tokens for Connection Manager (database: sigma)
-- Cause: Connection Manager expects columns token_value, used, used_at, agent_id (and id UUID), not value/is_active/used_count.
-- Run this in the same DB the Connection Manager uses (e.g. sigma).

-- 1) Drop the table you created (wrong schema)
DROP TABLE IF EXISTS installation_tokens;

-- 2) Create agents table if not exists (required by FK in installation_tokens)
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

-- 3) Create installation_tokens with schema expected by Connection Manager
CREATE TABLE installation_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_value VARCHAR(255) UNIQUE NOT NULL,
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    used BOOLEAN DEFAULT FALSE,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours'
);
CREATE INDEX IF NOT EXISTS idx_installation_tokens_token ON installation_tokens(token_value);
CREATE INDEX IF NOT EXISTS idx_installation_tokens_expires ON installation_tokens(expires_at);
CREATE INDEX IF NOT EXISTS idx_installation_tokens_used ON installation_tokens(used);

-- 4) Insert your token (code looks up by token_value)
INSERT INTO installation_tokens (token_value, expires_at)
VALUES ('EDR-SUPER-SECRET-TOKEN-2026', '2027-01-01 00:00:00+00')
ON CONFLICT (token_value) DO UPDATE SET expires_at = EXCLUDED.expires_at, used = FALSE;
