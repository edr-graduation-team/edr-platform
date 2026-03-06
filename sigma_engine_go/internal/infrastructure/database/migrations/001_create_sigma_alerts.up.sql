-- Migration: 001_create_sigma_alerts
-- Description: Creates the sigma_alerts table for storing Sigma rule detection alerts
-- Author: Sigma Engine Team
-- Date: 2026-01-09

-- =============================================================================
-- UP: Create sigma_alerts table
-- =============================================================================

CREATE TABLE IF NOT EXISTS sigma_alerts (
    -- Primary identifier
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Timestamp of when the alert was triggered
    timestamp TIMESTAMPTZ NOT NULL,
    
    -- Agent information
    agent_id VARCHAR(255) NOT NULL DEFAULT '',
    
    -- Rule information
    rule_id VARCHAR(255) NOT NULL,
    rule_title VARCHAR(512),
    
    -- Alert classification
    severity VARCHAR(20) NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'low', 'informational')),
    category VARCHAR(100),
    
    -- Event aggregation
    event_count INTEGER DEFAULT 1,
    event_ids TEXT[] DEFAULT ARRAY[]::TEXT[],
    
    -- MITRE ATT&CK mapping
    mitre_tactics TEXT[] DEFAULT ARRAY[]::TEXT[],
    mitre_techniques TEXT[] DEFAULT ARRAY[]::TEXT[],
    
    -- Detection details (JSONB for flexibility)
    matched_fields JSONB DEFAULT '{}'::JSONB,
    matched_selections TEXT[] DEFAULT ARRAY[]::TEXT[],
    context_data JSONB DEFAULT '{}'::JSONB,
    
    -- Alert status and workflow
    status VARCHAR(20) DEFAULT 'open' CHECK (status IN ('open', 'acknowledged', 'investigating', 'resolved', 'false_positive', 'suppressed')),
    assigned_to VARCHAR(255),
    resolution_notes TEXT,
    
    -- Confidence and false positive risk
    confidence DECIMAL(3,2) DEFAULT 0.80,
    false_positive_risk DECIMAL(3,2) DEFAULT 0.00,
    
    -- Multi-rule aggregation (when single event matches multiple rules)
    match_count INTEGER DEFAULT 1,
    related_rules TEXT[] DEFAULT ARRAY[]::TEXT[],
    combined_confidence DECIMAL(3,2),
    severity_promoted BOOLEAN DEFAULT FALSE,
    original_severity VARCHAR(20),
    
    -- Audit timestamps
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- Fix agent_id column type for databases created before the VARCHAR migration
-- This is safe to run multiple times and handles the UUID → VARCHAR transition
ALTER TABLE sigma_alerts ALTER COLUMN agent_id TYPE VARCHAR(255);
ALTER TABLE sigma_alerts ALTER COLUMN agent_id SET DEFAULT '';

-- =============================================================================
-- Indexes for fast querying
-- =============================================================================

-- Primary query patterns
CREATE INDEX IF NOT EXISTS idx_sigma_alerts_timestamp ON sigma_alerts(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sigma_alerts_agent_id ON sigma_alerts(agent_id);
CREATE INDEX IF NOT EXISTS idx_sigma_alerts_rule_id ON sigma_alerts(rule_id);
CREATE INDEX IF NOT EXISTS idx_sigma_alerts_severity ON sigma_alerts(severity);
CREATE INDEX IF NOT EXISTS idx_sigma_alerts_status ON sigma_alerts(status);

-- Composite indexes for common queries
CREATE INDEX IF NOT EXISTS idx_sigma_alerts_agent_timestamp ON sigma_alerts(agent_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sigma_alerts_rule_timestamp ON sigma_alerts(rule_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sigma_alerts_severity_timestamp ON sigma_alerts(severity, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sigma_alerts_status_timestamp ON sigma_alerts(status, timestamp DESC);

-- Deduplication lookups (used by alert writer)
CREATE INDEX IF NOT EXISTS idx_sigma_alerts_dedup ON sigma_alerts(agent_id, rule_id, timestamp DESC);

-- MITRE ATT&CK queries (GIN index for array containment)
CREATE INDEX IF NOT EXISTS idx_sigma_alerts_mitre_tactics ON sigma_alerts USING GIN(mitre_tactics);
CREATE INDEX IF NOT EXISTS idx_sigma_alerts_mitre_techniques ON sigma_alerts USING GIN(mitre_techniques);

-- Full-text search on rule title
CREATE INDEX IF NOT EXISTS idx_sigma_alerts_rule_title_search ON sigma_alerts USING GIN(to_tsvector('english', rule_title));

-- =============================================================================
-- Trigger: Auto-update updated_at on modification
-- =============================================================================

CREATE OR REPLACE FUNCTION update_sigma_alerts_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trigger_sigma_alerts_updated_at') THEN
        CREATE TRIGGER trigger_sigma_alerts_updated_at
            BEFORE UPDATE ON sigma_alerts
            FOR EACH ROW
            EXECUTE FUNCTION update_sigma_alerts_updated_at();
    END IF;
END $$;

-- =============================================================================
-- Comments for documentation
-- =============================================================================

COMMENT ON TABLE sigma_alerts IS 'Security alerts generated by Sigma rule engine. 30-day retention policy.';
COMMENT ON COLUMN sigma_alerts.id IS 'Unique alert identifier (UUID)';
COMMENT ON COLUMN sigma_alerts.timestamp IS 'When the alert was triggered';
COMMENT ON COLUMN sigma_alerts.agent_id IS 'UUID of the EDR agent that generated the event';
COMMENT ON COLUMN sigma_alerts.rule_id IS 'Sigma rule ID that matched';
COMMENT ON COLUMN sigma_alerts.severity IS 'Alert severity: critical, high, medium, low, informational';
COMMENT ON COLUMN sigma_alerts.event_count IS 'Number of events aggregated into this alert';
COMMENT ON COLUMN sigma_alerts.matched_fields IS 'Fields that matched the Sigma rule (JSON)';
COMMENT ON COLUMN sigma_alerts.status IS 'Alert workflow status';
COMMENT ON COLUMN sigma_alerts.confidence IS 'Detection confidence score (0.00-1.00)';
