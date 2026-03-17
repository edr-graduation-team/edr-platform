-- Migration: 015_create_process_baseline (DOWN)
-- Reverses 015_create_process_baseline.up.sql

DROP TRIGGER IF EXISTS trigger_process_baselines_updated_at ON process_baselines;
DROP FUNCTION IF EXISTS update_process_baselines_updated_at();

DROP INDEX IF EXISTS idx_process_baselines_lookup;
DROP INDEX IF EXISTS idx_process_baselines_agent;
DROP INDEX IF EXISTS idx_process_baselines_process_name;
DROP INDEX IF EXISTS idx_process_baselines_common_parents;

DROP TABLE IF EXISTS process_baselines;
