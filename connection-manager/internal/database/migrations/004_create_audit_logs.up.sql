-- Migration: 004_create_audit_logs
-- Simple non-partitioned audit_logs table for immutable security audit trail.
-- IMPORTANT: ip_address is TEXT (not INET) to accept empty strings.
-- IMPORTANT: resource_id is UUID matching the Go model type.

-- Drop old table if it exists (ensures clean state across restarts)
DROP TABLE IF EXISTS audit_logs CASCADE;

CREATE TABLE audit_logs (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Actor (user_id nullable for system events)
    user_id       UUID         REFERENCES users(id) ON DELETE SET NULL,
    username      VARCHAR(255),

    -- Action
    action        VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100),
    resource_id   UUID,

    -- Change details (JSONB for structured diff data)
    old_value     JSONB,
    new_value     JSONB,

    -- Outcome
    result        VARCHAR(20)  NOT NULL DEFAULT 'success',
    error_message TEXT,

    -- Request context
    ip_address    TEXT         DEFAULT '',   -- TEXT not INET: accepts empty string
    user_agent    TEXT         DEFAULT '',

    -- Timestamp
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Indexes for common query patterns
CREATE INDEX idx_audit_logs_created_at    ON audit_logs(created_at DESC);
CREATE INDEX idx_audit_logs_user_id       ON audit_logs(user_id);
CREATE INDEX idx_audit_logs_action        ON audit_logs(action);
CREATE INDEX idx_audit_logs_resource_type ON audit_logs(resource_type);
CREATE INDEX idx_audit_logs_result        ON audit_logs(result);

COMMENT ON TABLE audit_logs IS 'Immutable security audit trail';
COMMENT ON COLUMN audit_logs.ip_address IS 'Client IP as TEXT (not INET) to accept empty strings';
