-- Migration: 015_create_process_baseline
-- Description: Creates a behavioral baseline table for process execution patterns.
-- Used by Sprint 4 to calculate FP discounts for "expected" process behavior
-- based on historical frequency (e.g., svchost.exe runs 200x/hour on this host).
-- Date: 2026-03-10

-- =============================================================================
-- Process Baseline Table
-- =============================================================================
-- Stores the statistical execution profile of each unique (agent, process_name)
-- pair, aggregated by hour-of-day to capture circadian scheduling patterns.
--
-- Data flow:
--   1. sigma-engine writes raw process events to Redis lineage cache (Sprint 1)
--   2. A future offline aggregation job (cron/pg_cron) reads Redis counters and
--      UPSERTS into this table to build the behavioral model
--   3. RiskScorer reads this table (Sprint 4) to compute the fp_discount for
--      processes that are "expected" on this host at this time of day.

CREATE TABLE IF NOT EXISTS process_baselines (
    -- Primary identifier
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Agent and process identity
    agent_id        VARCHAR(255) NOT NULL,
    process_name    VARCHAR(512) NOT NULL,   -- e.g. "svchost.exe"
    process_path    VARCHAR(1024),           -- e.g. "C:\Windows\System32\svchost.exe"

    -- Temporal granularity: 0–23 (hour in local agent timezone, UTC stored)
    hour_of_day     SMALLINT    NOT NULL CHECK (hour_of_day BETWEEN 0 AND 23),

    -- Execution statistics (computed over a 14-day rolling window by default)
    avg_executions_per_hour     DECIMAL(10,4)   NOT NULL DEFAULT 0.0,
    max_executions_per_hour     INTEGER         NOT NULL DEFAULT 0,
    min_executions_per_hour     INTEGER         NOT NULL DEFAULT 0,
    stddev_executions           DECIMAL(10,4)   DEFAULT 0.0,
    observation_days            INTEGER         NOT NULL DEFAULT 0,

    -- Signature and trust context (snapshotted during aggregation)
    typical_signature_status    VARCHAR(50),    -- "microsoft", "trusted", "unsigned"
    typical_integrity_level     VARCHAR(20),    -- "Low","Medium","High","System"
    typically_elevated          BOOLEAN         DEFAULT FALSE,

    -- Parent process patterns (top 3 most common parents as JSON array)
    -- Example: ["services.exe", "svchost.exe"]
    common_parents              JSONB           NOT NULL DEFAULT '[]'::JSONB,

    -- Statistical model metadata
    confidence_score            DECIMAL(3,2)    NOT NULL DEFAULT 0.00,
    -- confidence approaches 1.0 as observation_days increases
    -- Formula: 1 - exp(-observation_days / 7)

    last_observed_at            TIMESTAMPTZ,
    baseline_window_days        INTEGER         NOT NULL DEFAULT 14,

    -- Audit timestamps
    created_at  TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- =============================================================================
-- Indexes
-- =============================================================================

-- Primary lookup: given an event (agent, process_name, current_hour), find baseline
CREATE UNIQUE INDEX IF NOT EXISTS idx_process_baselines_lookup
    ON process_baselines (agent_id, process_name, hour_of_day);

CREATE INDEX IF NOT EXISTS idx_process_baselines_agent
    ON process_baselines (agent_id);

CREATE INDEX IF NOT EXISTS idx_process_baselines_process_name
    ON process_baselines (process_name);

-- GIN index: query baselines by common parent processes
-- Example: WHERE common_parents @> '["winword.exe"]'
CREATE INDEX IF NOT EXISTS idx_process_baselines_common_parents
    ON process_baselines USING GIN (common_parents);

-- =============================================================================
-- Auto-update trigger
-- =============================================================================

CREATE OR REPLACE FUNCTION update_process_baselines_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgname = 'trigger_process_baselines_updated_at'
    ) THEN
        CREATE TRIGGER trigger_process_baselines_updated_at
            BEFORE UPDATE ON process_baselines
            FOR EACH ROW
            EXECUTE FUNCTION update_process_baselines_updated_at();
    END IF;
END $$;

-- =============================================================================
-- Documentation
-- =============================================================================

COMMENT ON TABLE process_baselines IS
    'Behavioral baseline for process execution frequency per agent per hour-of-day. Used by RiskScorer Sprint 4 to compute contextual false-positive discounts for statistically normal behavior.';

COMMENT ON COLUMN process_baselines.hour_of_day IS
    'Hour in UTC (0-23) for circadian behavioral profiling.';

COMMENT ON COLUMN process_baselines.avg_executions_per_hour IS
    'Rolling 14-day average number of times this process starts per hour on this agent.';

COMMENT ON COLUMN process_baselines.confidence_score IS
    'Model confidence [0.00-1.00]. Formula: 1 - exp(-observation_days/7). Reaches ~0.86 after 14 days.';

COMMENT ON COLUMN process_baselines.common_parents IS
    'JSON array of most frequent parent process names observed spawning this process.';
