-- Migration: 016_sigma_alert_correlations (down)
-- Roll back persisted alert correlation edges.

DROP TABLE IF EXISTS sigma_alert_correlations;
