-- Migration: 006_create_csrs
-- Create csrs table for pending Certificate Signing Requests

CREATE TABLE IF NOT EXISTS csrs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL UNIQUE,
    csr_data TEXT NOT NULL,
    
    -- Approval tracking
    approved BOOLEAN DEFAULT FALSE,
    approved_by UUID,
    approved_at TIMESTAMPTZ,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours',
    
    CONSTRAINT fk_csrs_agent FOREIGN KEY (agent_id) 
        REFERENCES agents(id) ON DELETE CASCADE,
    CONSTRAINT fk_csrs_approved_by FOREIGN KEY (approved_by) 
        REFERENCES users(id) ON DELETE SET NULL
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_csrs_agent_id ON csrs(agent_id);
CREATE INDEX IF NOT EXISTS idx_csrs_approved ON csrs(approved);
CREATE INDEX IF NOT EXISTS idx_csrs_expires ON csrs(expires_at);

-- Function to clean up expired CSRs
CREATE OR REPLACE FUNCTION cleanup_expired_csrs()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM csrs 
    WHERE expires_at < NOW() AND approved = FALSE;
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

COMMENT ON TABLE csrs IS 'Pending Certificate Signing Requests awaiting approval';
