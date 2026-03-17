-- Migration: 010_create_enrollment_tokens
-- Create enrollment_tokens table for dynamic, reusable agent enrollment tokens
-- managed via the Dashboard. Replaces static bootstrap tokens.

CREATE TABLE IF NOT EXISTS enrollment_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Cryptographically-secure random token (hex-encoded, 32 bytes = 64 chars)
    token VARCHAR(128) UNIQUE NOT NULL,

    -- Human-readable label (e.g. 'HR Dept Deployment', 'Lab Test Batch')
    description VARCHAR(255) NOT NULL DEFAULT '',

    -- Status: active tokens can be used for enrollment; revoked tokens cannot
    is_active BOOLEAN NOT NULL DEFAULT TRUE,

    -- Optional expiration: NULL = never expires
    expires_at TIMESTAMPTZ,

    -- Usage tracking
    use_count INTEGER NOT NULL DEFAULT 0,
    max_uses INTEGER,  -- NULL = unlimited

    -- Metadata
    created_by VARCHAR(255) NOT NULL DEFAULT 'system',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_enrollment_tokens_token ON enrollment_tokens(token);
CREATE INDEX IF NOT EXISTS idx_enrollment_tokens_active ON enrollment_tokens(is_active);
CREATE INDEX IF NOT EXISTS idx_enrollment_tokens_expires ON enrollment_tokens(expires_at);

-- Auto-update updated_at on row changes
CREATE OR REPLACE FUNCTION update_enrollment_tokens_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_enrollment_tokens_updated_at
    BEFORE UPDATE ON enrollment_tokens
    FOR EACH ROW
    EXECUTE FUNCTION update_enrollment_tokens_updated_at();

COMMENT ON TABLE enrollment_tokens IS 'Dynamic enrollment tokens for agent registration, managed via Dashboard';
COMMENT ON COLUMN enrollment_tokens.token IS 'Cryptographically-secure random token string (hex, 64 chars)';
COMMENT ON COLUMN enrollment_tokens.is_active IS 'FALSE = revoked; enrollment requests using this token will be rejected';
COMMENT ON COLUMN enrollment_tokens.max_uses IS 'NULL = unlimited uses; otherwise token is auto-deactivated after max_uses enrollments';
