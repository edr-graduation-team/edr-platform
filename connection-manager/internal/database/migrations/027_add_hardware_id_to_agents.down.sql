-- Migration: 027_add_hardware_id_to_agents (down)

DROP INDEX IF EXISTS idx_agents_hardware_id_unique;
ALTER TABLE agents DROP COLUMN IF EXISTS hardware_id;

