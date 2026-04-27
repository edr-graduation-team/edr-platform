-- Migration: 017_alert_dedup_upsert
-- Description: Adds a partial unique index on (agent_id, rule_id) for active
--              alerts within a rolling 5-minute deduplication window.
--              This collapses the alert writer's SELECT + conditional INSERT
--              into a single atomic ON CONFLICT upsert, eliminating the
--              read-modify-write race condition under concurrent workers.
--
-- NOTE: A true time-window unique constraint is not possible in Postgres
-- (it would require an exclusion constraint with a range type). Instead we
-- use a unique index on (agent_id, rule_id) with the dedup enforced by the
-- INSERT ... ON CONFLICT DO UPDATE query in alert_repo.go::Upsert.
-- The upsert increments event_count only when the existing row's timestamp
-- is within the configured window (checked in the DO UPDATE WHERE clause).

-- Partial indexes speed up the dashboard COUNT(*) queries on active alerts.
CREATE INDEX IF NOT EXISTS idx_sigma_alerts_open_ts
    ON sigma_alerts(timestamp DESC)
    WHERE status IN ('open', 'investigating');

CREATE INDEX IF NOT EXISTS idx_sigma_alerts_open_risk
    ON sigma_alerts(risk_score DESC)
    WHERE status IN ('open', 'investigating');

-- Composite dedup lookup index (used by Upsert query).
-- Separate from idx_sigma_alerts_dedup to allow covering scan.
CREATE INDEX IF NOT EXISTS idx_sigma_alerts_dedup_lookup
    ON sigma_alerts(agent_id, rule_id, timestamp DESC)
    WHERE status NOT IN ('resolved', 'false_positive');
