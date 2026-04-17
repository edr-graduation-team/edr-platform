-- Migration: 016_sigma_alert_correlations
-- Persists deduplicated alert-to-alert correlation edges for durability and multi-replica read paths.

CREATE TABLE IF NOT EXISTS sigma_alert_correlations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_low_id TEXT NOT NULL,
    alert_high_id TEXT NOT NULL,
    relation_type VARCHAR(32) NOT NULL,
    correlation_score DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_sigma_alert_correlation_pair UNIQUE (alert_low_id, alert_high_id)
);

CREATE INDEX IF NOT EXISTS idx_sigma_alert_correlations_created
    ON sigma_alert_correlations(created_at DESC);

CREATE INDEX IF NOT EXISTS idx_sigma_alert_correlations_low
    ON sigma_alert_correlations(alert_low_id);

CREATE INDEX IF NOT EXISTS idx_sigma_alert_correlations_high
    ON sigma_alert_correlations(alert_high_id);
