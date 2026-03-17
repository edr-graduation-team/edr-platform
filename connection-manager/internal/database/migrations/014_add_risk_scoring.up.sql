-- Migration: 014_add_risk_scoring (connection-manager alerts table)
-- Description: Adds Context-Aware Risk Scoring columns to the connection-manager alerts table.
-- This mirrors the sigma_engine's 014_add_risk_scoring migration but targets the
-- connection-manager's own `alerts` table, which is the read-side for the REST API.
-- Date: 2026-03-10

ALTER TABLE alerts
    ADD COLUMN IF NOT EXISTS risk_score         INTEGER     NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS context_snapshot   JSONB       NOT NULL DEFAULT '{}'::JSONB,
    ADD COLUMN IF NOT EXISTS score_breakdown    JSONB       NOT NULL DEFAULT '{}'::JSONB,
    ADD COLUMN IF NOT EXISTS false_positive_risk DECIMAL(4,3) NOT NULL DEFAULT 0.000;

-- Performance indexes
CREATE INDEX IF NOT EXISTS idx_alerts_risk_score
    ON alerts (risk_score DESC);

CREATE INDEX IF NOT EXISTS idx_alerts_agent_risk_score
    ON alerts (agent_id, risk_score DESC);

CREATE INDEX IF NOT EXISTS idx_alerts_context_snapshot
    ON alerts USING GIN (context_snapshot);

COMMENT ON COLUMN alerts.risk_score IS
    'Context-aware risk score (0-100) computed by the sigma-engine RiskScorer.';

COMMENT ON COLUMN alerts.context_snapshot IS
    'Full forensic evidence snapshot: ancestor chain, privilege context, burst count.';

COMMENT ON COLUMN alerts.score_breakdown IS
    'Component-level breakdown of the risk_score formula for SOC analyst transparency.';

COMMENT ON COLUMN alerts.false_positive_risk IS
    'False positive probability estimate (0.000-1.000) based on signature and known-good path signals.';
