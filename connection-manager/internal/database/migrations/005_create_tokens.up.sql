-- Migration: 005_create_tokens
-- Create installation_tokens table for one-time agent registration tokens

CREATE TABLE IF NOT EXISTS installation_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_value VARCHAR(255) UNIQUE NOT NULL,
    agent_id UUID,
    
    -- Usage tracking
    used BOOLEAN DEFAULT FALSE,
    used_at TIMESTAMPTZ,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours',
    
    CONSTRAINT fk_installation_tokens_agent FOREIGN KEY (agent_id) 
        REFERENCES agents(id) ON DELETE SET NULL
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_installation_tokens_token ON installation_tokens(token_value);
CREATE INDEX IF NOT EXISTS idx_installation_tokens_expires ON installation_tokens(expires_at);
CREATE INDEX IF NOT EXISTS idx_installation_tokens_used ON installation_tokens(used);

-- Function to clean up expired tokens (run via cron job)
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

COMMENT ON TABLE installation_tokens IS 'One-time tokens for agent registration (24h validity)';
COMMENT ON FUNCTION cleanup_expired_installation_tokens IS 'Cleanup function for expired unused tokens';
