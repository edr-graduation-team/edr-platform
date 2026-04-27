-- Rollback: 017_alert_dedup_upsert
DROP INDEX IF EXISTS idx_sigma_alerts_open_ts;
DROP INDEX IF EXISTS idx_sigma_alerts_open_risk;
DROP INDEX IF EXISTS idx_sigma_alerts_dedup_lookup;
