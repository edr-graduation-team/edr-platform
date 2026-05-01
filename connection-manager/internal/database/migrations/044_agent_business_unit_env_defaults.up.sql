-- Migration 044: Set default 'unknown' for business_unit and environment
-- This replaces the empty/NULL state with a meaningful sentinel so the UI
-- can display "غير محدد" / "غير معرف" instead of "Not set".

ALTER TABLE agents
    ALTER COLUMN business_unit SET DEFAULT 'unknown',
    ALTER COLUMN environment    SET DEFAULT 'unknown';

-- Normalise existing NULL or empty-string rows to the new sentinel.
UPDATE agents SET business_unit = 'unknown' WHERE business_unit IS NULL OR business_unit = '';
UPDATE agents SET environment   = 'unknown' WHERE environment   IS NULL OR environment   = '';
