-- Agent packages (for in-place upgrades)
CREATE TABLE IF NOT EXISTS agent_packages (
    id UUID PRIMARY KEY,
    sha256 TEXT NOT NULL,
    filename TEXT NOT NULL DEFAULT 'edr-agent.exe',
    storage_path TEXT NOT NULL,
    build_params JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_agent_packages_expires_at ON agent_packages (expires_at);

-- Keep a per-agent last-used patch profile for UI prefill.
CREATE TABLE IF NOT EXISTS agent_patch_profiles (
    agent_id UUID PRIMARY KEY REFERENCES agents(id) ON DELETE CASCADE,
    profile JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
