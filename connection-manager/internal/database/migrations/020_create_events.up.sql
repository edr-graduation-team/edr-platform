-- ============================================================================
-- EVENTS TABLE
-- Phase 3: Event search UI (dashboard) - durable event storage
-- ============================================================================

CREATE TABLE IF NOT EXISTS events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Source
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    batch_id UUID,

    -- Core searchable fields
    event_type VARCHAR(100) NOT NULL,
    severity VARCHAR(50) NOT NULL DEFAULT 'informational',
    ts TIMESTAMPTZ NOT NULL,

    -- UX-friendly summary (best-effort)
    summary TEXT NOT NULL DEFAULT '',

    -- Raw event body (validated JSON object from agent)
    raw JSONB NOT NULL DEFAULT '{}'::jsonb,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for common investigations
CREATE INDEX IF NOT EXISTS idx_events_agent_ts ON events(agent_id, ts DESC);
CREATE INDEX IF NOT EXISTS idx_events_ts ON events(ts DESC);
CREATE INDEX IF NOT EXISTS idx_events_type_ts ON events(event_type, ts DESC);
CREATE INDEX IF NOT EXISTS idx_events_severity_ts ON events(severity, ts DESC);

-- Optional JSONB index for future advanced queries (kept generic)
CREATE INDEX IF NOT EXISTS idx_events_raw_gin ON events USING gin(raw);

COMMENT ON TABLE events IS 'Durable event telemetry (searchable subset + raw JSONB)';
COMMENT ON COLUMN events.ts IS 'Event timestamp (parsed from event.timestamp)';
COMMENT ON COLUMN events.raw IS 'Validated event JSON object from ingestion stream';

