-- Drop policy tables and all dependencies
DROP TRIGGER IF EXISTS trigger_policies_version ON policies;
DROP FUNCTION IF EXISTS increment_policy_version();

DROP TABLE IF EXISTS policy_agent_assignments CASCADE;
DROP TABLE IF EXISTS policy_versions CASCADE;
DROP TABLE IF EXISTS policies CASCADE;

-- Drop indexes
DROP INDEX IF EXISTS idx_policies_enabled;
DROP INDEX IF EXISTS idx_policies_priority;
DROP INDEX IF EXISTS idx_policies_name;
DROP INDEX IF EXISTS idx_policies_created_by;
DROP INDEX IF EXISTS idx_policy_versions_policy_id;
DROP INDEX IF EXISTS idx_policy_versions_version;
DROP INDEX IF EXISTS idx_policy_agent_assignments_agent;
