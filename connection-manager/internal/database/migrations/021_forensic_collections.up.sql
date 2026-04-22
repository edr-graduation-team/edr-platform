-- Forensic log collections captured from collect_logs / collect_forensics command results.

CREATE TABLE IF NOT EXISTS forensic_collections (
    command_id      uuid PRIMARY KEY,
    agent_id        uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    command_type    varchar(64) NOT NULL,
    issued_at       timestamptz NOT NULL,
    completed_at    timestamptz,
    time_range      text,
    log_types       text,
    summary         jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_forensic_collections_agent_time
    ON forensic_collections(agent_id, issued_at DESC);

CREATE TABLE IF NOT EXISTS forensic_events (
    id          bigserial PRIMARY KEY,
    command_id  uuid NOT NULL REFERENCES forensic_collections(command_id) ON DELETE CASCADE,
    agent_id    uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    log_type    text NOT NULL,
    ts          timestamptz,
    event_id    text,
    level       text,
    provider    text,
    message     text,
    raw         jsonb,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_forensic_events_command_type_id
    ON forensic_events(command_id, log_type, id);

