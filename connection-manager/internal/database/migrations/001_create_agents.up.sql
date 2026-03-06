-- Migration: 001_create_agents
-- Create agents table for storing registered EDR agents

CREATE TABLE IF NOT EXISTS agents (
    id UUID PRIMARY KEY,
    hostname VARCHAR(255) NOT NULL UNIQUE,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    
    -- Device information
    os_type VARCHAR(50),
    os_version VARCHAR(100),
    cpu_count INTEGER,
    memory_mb BIGINT,
    
    -- Agent metadata
    agent_version VARCHAR(50),
    installed_date TIMESTAMPTZ,
    last_seen TIMESTAMPTZ,
    
    -- Metrics
    events_collected BIGINT DEFAULT 0,
    events_delivered BIGINT DEFAULT 0,
    queue_depth INTEGER DEFAULT 0,
    cpu_usage FLOAT DEFAULT 0,
    memory_used_mb BIGINT DEFAULT 0,
    health_score FLOAT DEFAULT 100.0,
    
    -- Certificate reference
    current_cert_id UUID,
    cert_expires_at TIMESTAMPTZ,
    
    -- Metadata (JSONB)
    tags JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
CREATE INDEX IF NOT EXISTS idx_agents_hostname ON agents(hostname);
CREATE INDEX IF NOT EXISTS idx_agents_last_seen ON agents(last_seen);
CREATE INDEX IF NOT EXISTS idx_agents_cert_expires ON agents(cert_expires_at);
CREATE INDEX IF NOT EXISTS idx_agents_os_type ON agents(os_type);
CREATE INDEX IF NOT EXISTS idx_agents_health_score ON agents(health_score);

-- Trigger to update updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_agents_updated_at
    BEFORE UPDATE ON agents
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE agents IS 'Registered EDR agents with their status and metrics';
COMMENT ON COLUMN agents.id IS 'Unique agent identifier (UUID)';
COMMENT ON COLUMN agents.hostname IS 'Agent hostname (must be unique)';
COMMENT ON COLUMN agents.status IS 'Agent status: pending, online, offline, degraded, suspended';
COMMENT ON COLUMN agents.health_score IS 'Calculated health score (0-100)';
