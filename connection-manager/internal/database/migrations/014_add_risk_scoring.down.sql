-- Migration: 014_add_risk_scoring (connection-manager DOWN)
DROP INDEX IF EXISTS idx_alerts_risk_score;
DROP INDEX IF EXISTS idx_alerts_agent_risk_score;
DROP INDEX IF EXISTS idx_alerts_context_snapshot;

ALTER TABLE alerts
    DROP COLUMN IF EXISTS risk_score,
    DROP COLUMN IF EXISTS context_snapshot,
    DROP COLUMN IF EXISTS score_breakdown,
    DROP COLUMN IF EXISTS false_positive_risk;
