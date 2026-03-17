-- Migration: 011_add_agent_telemetry (rollback)
-- Removes dropped events tracking and IP addresses from agents table.

ALTER TABLE agents DROP COLUMN IF EXISTS events_dropped;
ALTER TABLE agents DROP COLUMN IF EXISTS ip_addresses;
