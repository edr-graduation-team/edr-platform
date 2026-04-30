-- Reverse Phase 3 migration.
DROP TRIGGER IF EXISTS agents_criticality_changed ON agents;
DROP FUNCTION IF EXISTS trg_agent_criticality_changed();
DROP FUNCTION IF EXISTS recompute_all_vuln_priorities();
DROP FUNCTION IF EXISTS recompute_vuln_priority_for_agent(UUID);
DROP FUNCTION IF EXISTS vuln_priority_score(DOUBLE PRECISION, BOOLEAN, BOOLEAN, VARCHAR);

ALTER TABLE agents DROP CONSTRAINT IF EXISTS agents_criticality_chk;
DROP INDEX IF EXISTS idx_agents_criticality;

ALTER TABLE agents
    DROP COLUMN IF EXISTS criticality,
    DROP COLUMN IF EXISTS business_unit,
    DROP COLUMN IF EXISTS environment;
