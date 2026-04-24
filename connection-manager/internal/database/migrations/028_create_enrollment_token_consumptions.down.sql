-- Migration: 028_create_enrollment_token_consumptions (down)

DROP INDEX IF EXISTS idx_enrollment_token_consumptions_token_id;
DROP INDEX IF EXISTS idx_enrollment_token_consumptions_token_hw_unique;
DROP TABLE IF EXISTS enrollment_token_consumptions;

