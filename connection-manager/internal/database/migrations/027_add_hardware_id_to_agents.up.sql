-- Migration: 027_add_hardware_id_to_agents
-- Add stable hardware fingerprint for idempotent enrollment.

ALTER TABLE agents
ADD COLUMN IF NOT EXISTS hardware_id VARCHAR(128);

-- Enforce uniqueness when present (NULLs allowed).
CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_hardware_id_unique
ON agents(hardware_id)
WHERE hardware_id IS NOT NULL AND hardware_id <> '';

