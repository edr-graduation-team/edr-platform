-- Migration: 034_add_sysmon_status_to_agents
-- Description: Adds sysmon_installed and sysmon_running columns to the agents table.
-- These are populated by the agent heartbeat and exposed via GET /api/v1/agents/:id.

ALTER TABLE agents
    ADD COLUMN IF NOT EXISTS sysmon_installed BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS sysmon_running   BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN agents.sysmon_installed IS 'True when the Sysmon64 service is present on the endpoint (binary exists + service registered).';
COMMENT ON COLUMN agents.sysmon_running   IS 'True when the Sysmon64 service is in RUNNING state at last heartbeat.';

CREATE INDEX IF NOT EXISTS idx_agents_sysmon_installed ON agents (sysmon_installed);
CREATE INDEX IF NOT EXISTS idx_agents_sysmon_running   ON agents (sysmon_running);
