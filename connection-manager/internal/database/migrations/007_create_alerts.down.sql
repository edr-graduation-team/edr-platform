-- Drop alerts table and all dependencies
DROP TABLE IF EXISTS alerts CASCADE;

-- Drop indexes
DROP INDEX IF EXISTS idx_alerts_agent_id;
DROP INDEX IF EXISTS idx_alerts_status;
DROP INDEX IF EXISTS idx_alerts_severity;
DROP INDEX IF EXISTS idx_alerts_detected_at;
DROP INDEX IF EXISTS idx_alerts_assigned_to;
DROP INDEX IF EXISTS idx_alerts_rule_id;
DROP INDEX IF EXISTS idx_alerts_status_severity;
DROP INDEX IF EXISTS idx_alerts_open_by_severity;
DROP INDEX IF EXISTS idx_alerts_search;
