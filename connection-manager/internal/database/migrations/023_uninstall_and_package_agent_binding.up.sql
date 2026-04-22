-- Uninstall path + per-agent package binding.
--
-- 1) Expand commands.command_type CHECK to permit uninstall_agent, restore/delete_quarantine_file,
--    enable_sysmon/disable_sysmon so the API allowlist stays consistent with the agent side.
-- 2) Bind agent_packages to a specific agent_id; tighten to single-use semantics
--    (consumed_at) so the link dies after the first successful download or on expiry cleanup.

ALTER TABLE commands
    DROP CONSTRAINT IF EXISTS commands_command_type_check;

ALTER TABLE commands
    ADD CONSTRAINT commands_command_type_check CHECK (command_type IN (
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
        'update_agent', 'uninstall_agent',
        'update_policy', 'update_config', 'update_filter_policy',
        'adjust_rate',
        'run_cmd', 'custom',
        'block_ip', 'unblock_ip',
        'block_domain', 'unblock_domain',
        'update_signatures',
        'enable_sysmon', 'disable_sysmon'
    ));

-- Per-agent binding for patch/upgrade packages (nullable for backward compat with
-- pre-existing rows; new rows are required to set it).
ALTER TABLE agent_packages
    ADD COLUMN IF NOT EXISTS agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS consumed_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_agent_packages_agent_id ON agent_packages (agent_id);
