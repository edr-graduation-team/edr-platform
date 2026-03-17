-- Add network isolation status tracking to agents table
ALTER TABLE agents ADD COLUMN IF NOT EXISTS is_isolated BOOLEAN NOT NULL DEFAULT false;
COMMENT ON COLUMN agents.is_isolated IS 'Whether the agent network is currently isolated (firewall-blocked except C2)';
