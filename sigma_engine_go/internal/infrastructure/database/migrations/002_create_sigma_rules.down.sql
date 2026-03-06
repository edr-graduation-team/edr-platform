-- Migration: 002_create_sigma_rules (DOWN)
-- Description: Rollback sigma_rules table creation

-- Drop trigger first
DROP TRIGGER IF EXISTS trigger_sigma_rules_updated_at ON sigma_rules;

-- Drop function
DROP FUNCTION IF EXISTS update_sigma_rules_updated_at();

-- Drop table (cascades indexes)
DROP TABLE IF EXISTS sigma_rules CASCADE;
