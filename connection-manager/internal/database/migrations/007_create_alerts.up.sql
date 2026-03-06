-- ============================================================================
-- ALERTS TABLE
-- Phase 2: Dashboard API - Alert Management
-- ============================================================================

CREATE TABLE IF NOT EXISTS alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Alert identification
    severity VARCHAR(20) NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'low', 'informational')),
    title VARCHAR(500) NOT NULL,
    description TEXT,
    
    -- Source
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    rule_id VARCHAR(100),
    rule_name VARCHAR(255),
    
    -- Status tracking
    status VARCHAR(20) NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in_progress', 'resolved', 'closed', 'false_positive')),
    assigned_to UUID REFERENCES users(id) ON DELETE SET NULL,
    
    -- Resolution
    resolution VARCHAR(50) CHECK (resolution IN ('false_positive', 'remediated', 'escalated', 'accepted_risk', 'duplicate')),
    resolution_notes TEXT,
    
    -- Event correlation
    event_count INTEGER DEFAULT 1,
    first_event_at TIMESTAMP WITH TIME ZONE,
    last_event_at TIMESTAMP WITH TIME ZONE,
    
    -- Timestamps
    detected_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    acknowledged_at TIMESTAMP WITH TIME ZONE,
    resolved_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Metadata
    tags JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    notes TEXT
);

-- Indexes for common queries
CREATE INDEX idx_alerts_agent_id ON alerts(agent_id);
CREATE INDEX idx_alerts_status ON alerts(status);
CREATE INDEX idx_alerts_severity ON alerts(severity);
CREATE INDEX idx_alerts_detected_at ON alerts(detected_at DESC);
CREATE INDEX idx_alerts_assigned_to ON alerts(assigned_to);
CREATE INDEX idx_alerts_rule_id ON alerts(rule_id);
CREATE INDEX idx_alerts_status_severity ON alerts(status, severity);

-- Composite index for dashboard queries
CREATE INDEX idx_alerts_open_by_severity ON alerts(severity) WHERE status = 'open';

-- Full-text search on title and description
CREATE INDEX idx_alerts_search ON alerts USING gin(to_tsvector('english', title || ' ' || COALESCE(description, '')));

-- Updated at trigger
CREATE TRIGGER trigger_alerts_updated_at
    BEFORE UPDATE ON alerts
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Comments
COMMENT ON TABLE alerts IS 'Security alerts generated from event analysis';
COMMENT ON COLUMN alerts.severity IS 'Alert severity: critical, high, medium, low, informational';
COMMENT ON COLUMN alerts.status IS 'Alert status: open, in_progress, resolved, closed, false_positive';
COMMENT ON COLUMN alerts.event_count IS 'Number of correlated events';
