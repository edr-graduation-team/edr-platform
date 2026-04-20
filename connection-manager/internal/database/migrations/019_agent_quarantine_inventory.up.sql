-- Inventory of files in agent quarantine (fed from telemetry + manual quarantine ACKs).
CREATE TABLE IF NOT EXISTS agent_quarantine_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    event_id TEXT,
    original_path TEXT NOT NULL,
    quarantine_path TEXT NOT NULL,
    sha256 TEXT,
    threat_name TEXT,
    source TEXT NOT NULL DEFAULT 'auto_responder',
    state TEXT NOT NULL DEFAULT 'quarantined',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT agent_quarantine_items_source_check CHECK (source IN ('auto_responder', 'manual_c2')),
    CONSTRAINT agent_quarantine_items_state_check CHECK (state IN ('quarantined', 'acknowledged', 'restored', 'deleted')),
    CONSTRAINT agent_quarantine_items_agent_qpath_unique UNIQUE (agent_id, quarantine_path)
);

CREATE INDEX IF NOT EXISTS idx_agent_quarantine_agent_state
    ON agent_quarantine_items (agent_id, state);

-- Allow new C2 command types for restore/delete from quarantine UI.
ALTER TABLE commands DROP CONSTRAINT IF EXISTS commands_command_type_check;
ALTER TABLE commands ADD CONSTRAINT commands_command_type_check CHECK (command_type IN (
    'kill_process', 'terminate_process',
    'quarantine_file', 'restore_quarantine_file', 'delete_quarantine_file',
    'collect_logs', 'collect_forensics',
    'isolate_network', 'isolate',
    'restore_network', 'unisolate_network', 'unisolate',
    'restart_agent', 'restart_service',
    'start_agent', 'start_service',
    'stop_agent', 'stop_service',
    'restart_machine', 'restart',
    'shutdown_machine', 'shutdown',
    'scan_file', 'scan_memory',
    'update_agent',
    'update_policy', 'update_config', 'update_filter_policy',
    'adjust_rate',
    'run_cmd', 'custom',
    'block_ip', 'unblock_ip',
    'block_domain', 'unblock_domain',
    'update_signatures'
));
