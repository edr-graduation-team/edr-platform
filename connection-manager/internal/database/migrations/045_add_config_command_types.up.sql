-- 045_add_config_command_types.up.sql
-- Add update_vuln_config to the commands.command_type CHECK constraint.
-- Migration 029 introduced the triage types but omitted update_vuln_config,
-- enable_sysmon, and disable_sysmon — this aligns the DB constraint with
-- the API handler (mapCommandType) which already accepts these values.

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
        'update_policy', 'update_config', 'update_filter_policy', 'update_vuln_config',
        'adjust_rate',
        'run_cmd', 'custom',
        'block_ip', 'unblock_ip',
        'block_domain', 'unblock_domain',
        'update_signatures',
        'enable_sysmon', 'disable_sysmon',
        'post_isolation_triage',
        'process_tree_snapshot',
        'persistence_scan',
        'lsass_access_audit',
        'filesystem_timeline',
        'network_last_seen',
        'agent_integrity_check',
        'memory_dump'
    ));
