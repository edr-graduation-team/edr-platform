-- Migration: 002_create_sigma_rules
-- Description: Creates the sigma_rules table for storing Sigma detection rules
-- Author: Sigma Engine Team
-- Date: 2026-01-09

-- =============================================================================
-- UP: Create sigma_rules table
-- =============================================================================

CREATE TABLE IF NOT EXISTS sigma_rules (
    -- Primary identifier (Sigma rule ID from YAML)
    id VARCHAR(255) PRIMARY KEY,
    
    -- Rule metadata
    title VARCHAR(512) NOT NULL,
    description TEXT,
    author VARCHAR(255),
    
    -- Full rule content (YAML)
    content TEXT NOT NULL,
    
    -- Rule status
    enabled BOOLEAN DEFAULT TRUE,
    status VARCHAR(20) DEFAULT 'stable' CHECK (status IN ('stable', 'test', 'experimental', 'deprecated')),
    
    -- Classification
    product VARCHAR(100) DEFAULT 'windows',
    category VARCHAR(100),
    service VARCHAR(100),
    
    -- Severity
    severity VARCHAR(20) CHECK (severity IN ('critical', 'high', 'medium', 'low', 'informational')),
    
    -- MITRE ATT&CK mapping
    mitre_tactics TEXT[] DEFAULT ARRAY[]::TEXT[],
    mitre_techniques TEXT[] DEFAULT ARRAY[]::TEXT[],
    
    -- Tags and references
    tags TEXT[] DEFAULT ARRAY[]::TEXT[],
    "references" TEXT[] DEFAULT ARRAY[]::TEXT[],
    
    -- Rule versioning
    version INTEGER DEFAULT 1,
    date_created DATE,
    date_modified DATE,
    
    -- Source tracking
    source VARCHAR(100) DEFAULT 'official' CHECK (source IN ('official', 'custom', 'community', 'imported')),
    source_url TEXT,
    
    -- Additional metadata (flexible JSON)
    custom_metadata JSONB DEFAULT '{}'::JSONB,
    
    -- False positive information
    false_positives TEXT[] DEFAULT ARRAY[]::TEXT[],
    
    -- Performance metrics
    avg_match_time_ms DECIMAL(10,3),
    total_matches BIGINT DEFAULT 0,
    last_matched_at TIMESTAMPTZ,
    
    -- Audit timestamps
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- =============================================================================
-- Indexes for fast querying
-- =============================================================================

-- Primary query patterns
CREATE INDEX IF NOT EXISTS idx_sigma_rules_enabled ON sigma_rules(enabled);
CREATE INDEX IF NOT EXISTS idx_sigma_rules_product ON sigma_rules(product);
CREATE INDEX IF NOT EXISTS idx_sigma_rules_category ON sigma_rules(category);
CREATE INDEX IF NOT EXISTS idx_sigma_rules_severity ON sigma_rules(severity);
CREATE INDEX IF NOT EXISTS idx_sigma_rules_status ON sigma_rules(status);
CREATE INDEX IF NOT EXISTS idx_sigma_rules_source ON sigma_rules(source);

-- Composite indexes
CREATE INDEX IF NOT EXISTS idx_sigma_rules_enabled_product ON sigma_rules(enabled, product);
CREATE INDEX IF NOT EXISTS idx_sigma_rules_product_category ON sigma_rules(product, category);

-- MITRE ATT&CK queries (GIN index for array containment)
CREATE INDEX IF NOT EXISTS idx_sigma_rules_mitre_tactics ON sigma_rules USING GIN(mitre_tactics);
CREATE INDEX IF NOT EXISTS idx_sigma_rules_mitre_techniques ON sigma_rules USING GIN(mitre_techniques);
CREATE INDEX IF NOT EXISTS idx_sigma_rules_tags ON sigma_rules USING GIN(tags);

-- Full-text search on title and description
CREATE INDEX IF NOT EXISTS idx_sigma_rules_title_search ON sigma_rules USING GIN(to_tsvector('english', title));
CREATE INDEX IF NOT EXISTS idx_sigma_rules_description_search ON sigma_rules USING GIN(to_tsvector('english', COALESCE(description, '')));

-- =============================================================================
-- Trigger: Auto-update updated_at on modification
-- =============================================================================

CREATE OR REPLACE FUNCTION update_sigma_rules_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trigger_sigma_rules_updated_at') THEN
        CREATE TRIGGER trigger_sigma_rules_updated_at
            BEFORE UPDATE ON sigma_rules
            FOR EACH ROW
            EXECUTE FUNCTION update_sigma_rules_updated_at();
    END IF;
END $$;

-- =============================================================================
-- Comments for documentation
-- =============================================================================

COMMENT ON TABLE sigma_rules IS 'Sigma detection rules stored for dynamic rule management';
COMMENT ON COLUMN sigma_rules.id IS 'Unique rule identifier (from Sigma YAML id field)';
COMMENT ON COLUMN sigma_rules.title IS 'Human-readable rule title';
COMMENT ON COLUMN sigma_rules.content IS 'Full Sigma rule YAML content';
COMMENT ON COLUMN sigma_rules.enabled IS 'Whether the rule is active for detection';
COMMENT ON COLUMN sigma_rules.product IS 'Target product (windows, linux, etc.)';
COMMENT ON COLUMN sigma_rules.category IS 'Event category (process_creation, network_connection, etc.)';
COMMENT ON COLUMN sigma_rules.source IS 'Rule origin: official (SigmaHQ), custom, community, imported';
