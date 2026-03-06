-- Migration: 002_create_certificates
-- Create certificates table for storing agent TLS certificates

CREATE TABLE IF NOT EXISTS certificates (
    id UUID PRIMARY KEY,
    agent_id UUID NOT NULL,
    cert_fingerprint VARCHAR(64) UNIQUE NOT NULL,
    public_key TEXT NOT NULL,
    serial_number VARCHAR(100),
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    
    -- Validity period
    issued_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    
    -- Revocation info
    revoked_at TIMESTAMPTZ,
    revoked_by UUID,
    revoke_reason VARCHAR(255),
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    CONSTRAINT fk_certificates_agent FOREIGN KEY (agent_id) 
        REFERENCES agents(id) ON DELETE CASCADE
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_certificates_agent_id ON certificates(agent_id);
CREATE INDEX IF NOT EXISTS idx_certificates_fingerprint ON certificates(cert_fingerprint);
CREATE INDEX IF NOT EXISTS idx_certificates_status ON certificates(status);
CREATE INDEX IF NOT EXISTS idx_certificates_expires_at ON certificates(expires_at);
CREATE INDEX IF NOT EXISTS idx_certificates_agent_status ON certificates(agent_id, status);

COMMENT ON TABLE certificates IS 'Agent TLS certificates with revocation tracking';
COMMENT ON COLUMN certificates.cert_fingerprint IS 'SHA256 fingerprint of the certificate';
COMMENT ON COLUMN certificates.status IS 'Certificate status: active, expired, revoked, superseded';
