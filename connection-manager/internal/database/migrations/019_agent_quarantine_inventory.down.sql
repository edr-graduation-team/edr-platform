DROP TABLE IF EXISTS agent_quarantine_items;

ALTER TABLE commands DROP CONSTRAINT IF EXISTS commands_command_type_check;
ALTER TABLE commands ADD CONSTRAINT commands_command_type_check CHECK (command_type IN (
    'kill_process', 'terminate_process',
    'quarantine_file',
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
