-- Reverse migration 044: remove column defaults (values already written stay as-is)
ALTER TABLE agents
    ALTER COLUMN business_unit DROP DEFAULT,
    ALTER COLUMN environment    DROP DEFAULT;
