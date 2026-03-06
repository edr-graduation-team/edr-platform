-- ============================================================================
-- COMMANDS TABLE
-- Phase 2: Dashboard API - Remote Command Execution
-- ============================================================================

CREATE TABLE IF NOT EXISTS commands (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Target
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    
    -- Command details
    command_type VARCHAR(50) NOT NULL CHECK (command_type IN (
        'kill_process',
        'quarantine_file',
        'collect_logs',
        'update_policy',
        'restart_agent',
        'isolate_network',
        'restore_network',
        'scan_file',
        'scan_memory',
        'custom'
    )),
    parameters JSONB DEFAULT '{}',
    priority INTEGER NOT NULL DEFAULT 5 CHECK (priority BETWEEN 1 AND 10),
    
    -- Status tracking
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN (
        'pending',
        'sent',
        'acknowledged',
        'executing',
        'completed',
        'failed',
        'timeout',
        'cancelled'
    )),
    
    -- Execution tracking
    result JSONB,
    error_message TEXT,
    exit_code INTEGER,
    
    -- Timing
    timeout_seconds INTEGER NOT NULL DEFAULT 300,
    issued_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMP WITH TIME ZONE,
    acknowledged_at TIMESTAMP WITH TIME ZONE,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,
    
    -- Ownership
    issued_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    
    -- Metadata
    metadata JSONB DEFAULT '{}'
);

-- Command queue for prioritized processing
CREATE TABLE IF NOT EXISTS command_queue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    command_id UUID NOT NULL REFERENCES commands(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    
    priority INTEGER NOT NULL,
    scheduled_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Ensure only one queue entry per command
    CONSTRAINT unique_command_queue UNIQUE (command_id)
);

-- Indexes
CREATE INDEX idx_commands_agent_id ON commands(agent_id);
CREATE INDEX idx_commands_status ON commands(status);
CREATE INDEX idx_commands_command_type ON commands(command_type);
CREATE INDEX idx_commands_issued_at ON commands(issued_at DESC);
CREATE INDEX idx_commands_issued_by ON commands(issued_by);
CREATE INDEX idx_commands_pending ON commands(agent_id, priority DESC) WHERE status = 'pending';

CREATE INDEX idx_command_queue_agent_priority ON command_queue(agent_id, priority DESC, scheduled_at);

-- Updated at trigger (using issued_at as reference)
CREATE TRIGGER trigger_commands_set_expires
    BEFORE INSERT ON commands
    FOR EACH ROW
    EXECUTE FUNCTION set_command_expires();

-- Function to set expiration
CREATE OR REPLACE FUNCTION set_command_expires()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.expires_at IS NULL THEN
        NEW.expires_at = NEW.issued_at + (NEW.timeout_seconds || ' seconds')::INTERVAL;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Recreate trigger with function
DROP TRIGGER IF EXISTS trigger_commands_set_expires ON commands;
CREATE TRIGGER trigger_commands_set_expires
    BEFORE INSERT ON commands
    FOR EACH ROW
    EXECUTE FUNCTION set_command_expires();

-- Auto-add to queue on insert
CREATE OR REPLACE FUNCTION add_to_command_queue()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO command_queue (command_id, agent_id, priority, scheduled_at)
    VALUES (NEW.id, NEW.agent_id, NEW.priority, NOW())
    ON CONFLICT (command_id) DO NOTHING;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_commands_queue
    AFTER INSERT ON commands
    FOR EACH ROW
    WHEN (NEW.status = 'pending')
    EXECUTE FUNCTION add_to_command_queue();

-- Comments
COMMENT ON TABLE commands IS 'Remote commands to be executed on agents';
COMMENT ON TABLE command_queue IS 'Priority queue for pending commands';
COMMENT ON COLUMN commands.command_type IS 'Type of command to execute';
COMMENT ON COLUMN commands.parameters IS 'Command-specific parameters as JSON';
COMMENT ON COLUMN commands.priority IS 'Execution priority (1=lowest, 10=highest)';
COMMENT ON COLUMN commands.status IS 'Current execution status';
