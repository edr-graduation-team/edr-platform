-- Post-Isolation Pipeline: playbook_runs, playbook_steps, triage_snapshots, ioc_enrichment

-- Tracks each automatic post-isolation playbook execution.
CREATE TABLE IF NOT EXISTS playbook_runs (
    id           BIGSERIAL PRIMARY KEY,
    agent_id     UUID        NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    playbook     TEXT        NOT NULL DEFAULT 'default_post_isolation',
    trigger      TEXT        NOT NULL DEFAULT 'isolation.succeeded',
    status       TEXT        NOT NULL DEFAULT 'running',  -- running | completed | partial | failed
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at  TIMESTAMPTZ,
    summary      JSONB       NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_playbook_runs_agent ON playbook_runs(agent_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_playbook_runs_status ON playbook_runs(status) WHERE status = 'running';

-- Tracks each step (command) within a playbook run.
CREATE TABLE IF NOT EXISTS playbook_steps (
    id           BIGSERIAL PRIMARY KEY,
    run_id       BIGINT      NOT NULL REFERENCES playbook_runs(id) ON DELETE CASCADE,
    step_name    TEXT        NOT NULL,
    command_type TEXT        NOT NULL,
    status       TEXT        NOT NULL DEFAULT 'pending',  -- pending | running | success | failed | skipped
    command_id   UUID,
    started_at   TIMESTAMPTZ,
    finished_at  TIMESTAMPTZ,
    error        TEXT
);
CREATE INDEX IF NOT EXISTS idx_playbook_steps_run ON playbook_steps(run_id);
CREATE INDEX IF NOT EXISTS idx_playbook_steps_command ON playbook_steps(command_id) WHERE command_id IS NOT NULL;

-- Stores raw snapshots collected by the triage commands.
-- kind: process_tree | persistence | lsass_access | fs_timeline | network_last_seen | integrity | triage
CREATE TABLE IF NOT EXISTS triage_snapshots (
    id         BIGSERIAL PRIMARY KEY,
    agent_id   UUID        NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    run_id     BIGINT      REFERENCES playbook_runs(id) ON DELETE SET NULL,
    kind       TEXT        NOT NULL,
    payload    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_triage_snapshots_agent_kind ON triage_snapshots(agent_id, kind, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_triage_snapshots_run ON triage_snapshots(run_id) WHERE run_id IS NOT NULL;

-- Stores TI enrichment results per IOC (hash/ip/domain).
CREATE TABLE IF NOT EXISTS ioc_enrichment (
    id          BIGSERIAL PRIMARY KEY,
    agent_id    UUID        REFERENCES agents(id) ON DELETE SET NULL,
    run_id      BIGINT      REFERENCES playbook_runs(id) ON DELETE SET NULL,
    ioc_type    TEXT        NOT NULL,   -- hash | ip | domain
    ioc_value   TEXT        NOT NULL,
    provider    TEXT        NOT NULL,   -- virustotal | abuseipdb | otx
    verdict     TEXT,                  -- clean | suspicious | malicious | unknown
    score       INT,
    raw         JSONB,
    fetched_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (ioc_type, ioc_value, provider)
);
CREATE INDEX IF NOT EXISTS idx_ioc_enrichment_agent ON ioc_enrichment(agent_id, fetched_at DESC);
CREATE INDEX IF NOT EXISTS idx_ioc_enrichment_value ON ioc_enrichment(ioc_type, ioc_value);
