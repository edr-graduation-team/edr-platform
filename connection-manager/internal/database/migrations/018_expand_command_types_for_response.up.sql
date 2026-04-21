-- Expand commands.command_type CHECK constraint to include response extensions.
-- This keeps existing installs compatible with new API/agent command mappings.

ALTER TABLE commands
    DROP CONSTRAINT IF EXISTS commands_command_type_check;

ALTER TABLE commands
    ADD CONSTRAINT commands_command_type_check CHECK (command_type IN (
        -- Process actions
        'kill_process', 'terminate_process',
        -- File actions
        'quarantine_file',
        -- Log / forensics collection
        'collect_logs', 'collect_forensics',
        -- Network isolation
        'isolate_network', 'isolate',
        'restore_network', 'unisolate_network', 'unisolate',
        -- System control (agent)
        'restart_agent', 'restart_service',
        'start_agent', 'start_service',
        'stop_agent', 'stop_service',
        -- System control (OS-level)
        'restart_machine', 'restart',
        'shutdown_machine', 'shutdown',
        -- Scanning
        'scan_file', 'scan_memory',
        -- Agent management
        'update_agent',
        -- Policy / config
        'update_policy', 'update_config', 'update_filter_policy',
        -- Rate adjustment
        'adjust_rate',
        -- Generic / diagnostic
        'run_cmd', 'custom',
        -- Response extensions
        'block_ip', 'unblock_ip',
        'block_domain', 'unblock_domain',
        'update_signatures'
    ));
