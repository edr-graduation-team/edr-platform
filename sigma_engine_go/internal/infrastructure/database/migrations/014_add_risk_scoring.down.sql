-- Migration: 014_add_risk_scoring (DOWN)
-- Reverses 014_add_risk_scoring.up.sql

DROP INDEX IF EXISTS idx_sigma_alerts_risk_score;
DROP INDEX IF EXISTS idx_sigma_alerts_agent_risk_score;
DROP INDEX IF EXISTS idx_sigma_alerts_context_snapshot;
DROP INDEX IF EXISTS idx_sigma_alerts_score_breakdown;
DROP INDEX IF EXISTS idx_sigma_alerts_risk_status;

ALTER TABLE sigma_alerts
    DROP COLUMN IF EXISTS risk_score,
    DROP COLUMN IF EXISTS context_snapshot,
    DROP COLUMN IF EXISTS score_breakdown;
