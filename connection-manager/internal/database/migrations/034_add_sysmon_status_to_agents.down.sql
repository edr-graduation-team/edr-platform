-- Rollback: 034_add_sysmon_status_to_agents
ALTER TABLE agents
    DROP COLUMN IF EXISTS sysmon_installed,
    DROP COLUMN IF EXISTS sysmon_running;
