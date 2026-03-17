-- Migration: 011_add_agent_telemetry
-- Adds dropped events tracking and IP addresses collection to agents table.
--
-- events_dropped: Cumulative count of events filtered/rate-limited at the agent edge.
--                 A high value relative to events_collected signals potential
--                 blinding attack or overly aggressive QoS policies.
--
-- ip_addresses:   JSONB array of the agent's non-loopback, active network addresses.
--                 Reported on each heartbeat for network topology visibility.

ALTER TABLE agents ADD COLUMN IF NOT EXISTS events_dropped BIGINT DEFAULT 0;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS ip_addresses JSONB DEFAULT '[]';

-- Index for dropped events — enables fast identification of agents with high drop rates
CREATE INDEX IF NOT EXISTS idx_agents_events_dropped ON agents(events_dropped)
    WHERE events_dropped > 0;

COMMENT ON COLUMN agents.events_dropped IS 'Cumulative events filtered/rate-limited at agent edge (potential blinding indicator)';
COMMENT ON COLUMN agents.ip_addresses IS 'JSONB array of agent non-loopback IP addresses from last heartbeat';
