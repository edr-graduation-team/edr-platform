-- Migration: 001_create_sigma_alerts (DOWN)
-- Description: Rollback sigma_alerts table creation

-- Drop trigger first
DROP TRIGGER IF EXISTS trigger_sigma_alerts_updated_at ON sigma_alerts;

-- Drop function
DROP FUNCTION IF EXISTS update_sigma_alerts_updated_at();

-- Drop table (cascades indexes)
DROP TABLE IF EXISTS sigma_alerts CASCADE;
