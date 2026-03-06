-- Migration: 010_create_enrollment_tokens (down)
-- Drop enrollment_tokens table and related objects

DROP TRIGGER IF EXISTS trg_enrollment_tokens_updated_at ON enrollment_tokens;
DROP FUNCTION IF EXISTS update_enrollment_tokens_updated_at();
DROP TABLE IF EXISTS enrollment_tokens;
