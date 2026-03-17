-- Migration: 014_add_risk_scoring
-- Description: Adds Context-Aware Risk Scoring columns to sigma_alerts.
-- Phase 1: Context-Aware Detection & Adaptive Risk Scoring
-- Date: 2026-03-10

-- =============================================================================
-- Add Risk Scoring columns to sigma_alerts
-- =============================================================================

ALTER TABLE sigma_alerts
    ADD COLUMN IF NOT EXISTS risk_score         INTEGER     NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS context_snapshot   JSONB       NOT NULL DEFAULT '{}'::JSONB,
    ADD COLUMN IF NOT EXISTS score_breakdown    JSONB       NOT NULL DEFAULT '{}'::JSONB;

-- Note: false_positive_risk already exists as DECIMAL(3,2) from migration 001.
-- We only need to ensure it accepts values in the 0.00–1.00 range (constraint already present).

-- =============================================================================
-- Indexes
-- =============================================================================

-- Fast filtering/sorting by risk score (primary SOC triage use-case)
CREATE INDEX IF NOT EXISTS idx_sigma_alerts_risk_score
    ON sigma_alerts (risk_score DESC);

-- Composite index: agent + risk_score for per-agent SOC view sorted by risk
CREATE INDEX IF NOT EXISTS idx_sigma_alerts_agent_risk_score
    ON sigma_alerts (agent_id, risk_score DESC);

-- GIN index: allows querying context_snapshot by specific JSON fields
-- Example: WHERE context_snapshot @> '{"lineage_suspicion": "critical"}'
CREATE INDEX IF NOT EXISTS idx_sigma_alerts_context_snapshot
    ON sigma_alerts USING GIN (context_snapshot);

-- GIN index: allows querying score_breakdown to find alerts above a component threshold
-- Example: WHERE score_breakdown @> '{"lineage_bonus": 40}'
CREATE INDEX IF NOT EXISTS idx_sigma_alerts_score_breakdown
    ON sigma_alerts USING GIN (score_breakdown);

-- Composite: risk_score + status — primary SOC queue sort
CREATE INDEX IF NOT EXISTS idx_sigma_alerts_risk_status
    ON sigma_alerts (risk_score DESC, status);

-- =============================================================================
-- Column documentation
-- =============================================================================

COMMENT ON COLUMN sigma_alerts.risk_score IS
    'Context-aware risk score (0-100). Computed by RiskScorer: base(severity) + lineage_bonus + privilege_bonus + burst_bonus - fp_discount.';

COMMENT ON COLUMN sigma_alerts.context_snapshot IS
    'Full forensic evidence snapshot at scoring time. Contains reconstructed ancestor chain, privilege context, burst count, and component score breakdown. Stored as JSONB for flexible querying.';

COMMENT ON COLUMN sigma_alerts.score_breakdown IS
    'Scalar breakdown of the risk_score formula components: base_score, lineage_bonus, privilege_bonus, burst_bonus, fp_discount, raw_score, final_score.';
