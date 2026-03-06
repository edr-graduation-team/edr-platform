-- ============================================================================
-- POLICIES TABLE
-- Phase 2: Dashboard API - Policy Management
-- ============================================================================

CREATE TABLE IF NOT EXISTS policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Policy identification
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    
    -- Policy rules (JSON-based for flexibility)
    rules JSONB NOT NULL DEFAULT '[]',
    
    -- Targeting
    targets JSONB DEFAULT '{"apply_to_all": false, "agents": [], "groups": []}',
    
    -- Status
    enabled BOOLEAN NOT NULL DEFAULT true,
    priority INTEGER NOT NULL DEFAULT 100,
    version INTEGER NOT NULL DEFAULT 1,
    
    -- Ownership
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    
    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Metadata
    tags JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}'
);

-- Policy history for versioning
CREATE TABLE IF NOT EXISTS policy_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    policy_id UUID NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    
    -- Snapshot of policy at this version
    name VARCHAR(255) NOT NULL,
    description TEXT,
    rules JSONB NOT NULL,
    targets JSONB,
    enabled BOOLEAN NOT NULL,
    priority INTEGER NOT NULL,
    
    -- Change tracking
    changed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    change_reason TEXT,
    
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    CONSTRAINT unique_policy_version UNIQUE (policy_id, version)
);

-- Policy-Agent assignments (for targeted policies)
CREATE TABLE IF NOT EXISTS policy_agent_assignments (
    policy_id UUID NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    
    assigned_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    assigned_by UUID REFERENCES users(id) ON DELETE SET NULL,
    
    PRIMARY KEY (policy_id, agent_id)
);

-- Indexes
CREATE INDEX idx_policies_enabled ON policies(enabled);
CREATE INDEX idx_policies_priority ON policies(priority DESC);
CREATE INDEX idx_policies_name ON policies(name);
CREATE INDEX idx_policies_created_by ON policies(created_by);

CREATE INDEX idx_policy_versions_policy_id ON policy_versions(policy_id);
CREATE INDEX idx_policy_versions_version ON policy_versions(policy_id, version DESC);

CREATE INDEX idx_policy_agent_assignments_agent ON policy_agent_assignments(agent_id);

-- Updated at trigger
CREATE TRIGGER trigger_policies_updated_at
    BEFORE UPDATE ON policies
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Auto-increment version on update
CREATE OR REPLACE FUNCTION increment_policy_version()
RETURNS TRIGGER AS $$
BEGIN
    NEW.version = OLD.version + 1;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_policies_version
    BEFORE UPDATE ON policies
    FOR EACH ROW
    WHEN (OLD.rules IS DISTINCT FROM NEW.rules OR OLD.targets IS DISTINCT FROM NEW.targets)
    EXECUTE FUNCTION increment_policy_version();

-- Comments
COMMENT ON TABLE policies IS 'Security policies for agent configuration';
COMMENT ON TABLE policy_versions IS 'Historical versions of policies for audit';
COMMENT ON TABLE policy_agent_assignments IS 'Policy to agent assignments';
COMMENT ON COLUMN policies.rules IS 'JSON array of policy rules';
COMMENT ON COLUMN policies.targets IS 'JSON object defining target agents/groups';
