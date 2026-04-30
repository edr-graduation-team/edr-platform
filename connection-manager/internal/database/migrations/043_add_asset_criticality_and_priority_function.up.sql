-- Phase 3: Asset Criticality + Risk-Adjusted Priority Scoring
-- Adds business-context columns to agents and a SQL function that becomes the single
-- source of truth for vulnerability priority calculation. A trigger keeps priorities
-- in sync when an agent's criticality changes.

-- ── Asset metadata on agents ─────────────────────────────────────────────────
ALTER TABLE agents
    ADD COLUMN IF NOT EXISTS criticality   VARCHAR(16) NOT NULL DEFAULT 'medium',
    ADD COLUMN IF NOT EXISTS business_unit TEXT        NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS environment   VARCHAR(32) NOT NULL DEFAULT '';

-- Constrain criticality to known values.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'agents_criticality_chk'
    ) THEN
        ALTER TABLE agents
            ADD CONSTRAINT agents_criticality_chk
            CHECK (criticality IN ('low', 'medium', 'high', 'critical'));
    END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_agents_criticality ON agents(criticality);

-- ── Priority score function ──────────────────────────────────────────────────
-- Inputs: CVSS, KEV-listed flag, exploit-available flag, agent criticality.
-- Output: 0..100 score. Pure function (immutable) so can be used in indexes.
CREATE OR REPLACE FUNCTION vuln_priority_score(
    p_cvss        DOUBLE PRECISION,
    p_kev         BOOLEAN,
    p_exploit     BOOLEAN,
    p_criticality VARCHAR
) RETURNS DOUBLE PRECISION
LANGUAGE plpgsql
IMMUTABLE
AS $$
DECLARE
    base   DOUBLE PRECISION := COALESCE(p_cvss, 0) * 10;     -- 0..100
    bonus  DOUBLE PRECISION := 0;
    crit   DOUBLE PRECISION := 0;
    final  DOUBLE PRECISION;
BEGIN
    IF p_kev     THEN bonus := bonus + 25; END IF;
    IF p_exploit THEN bonus := bonus + 10; END IF;

    -- Criticality adjustment: weights the asset's business value into the score.
    crit := CASE COALESCE(p_criticality, 'medium')
        WHEN 'low'      THEN -15
        WHEN 'medium'   THEN   0
        WHEN 'high'     THEN  10
        WHEN 'critical' THEN  20
        ELSE                   0
    END;

    final := base + bonus + crit;
    IF final > 100 THEN final := 100; END IF;
    IF final < 0   THEN final := 0;   END IF;
    RETURN final;
END$$;

-- ── Recompute helpers ────────────────────────────────────────────────────────
-- Recomputes priority_score for findings of a specific agent (called by trigger).
CREATE OR REPLACE FUNCTION recompute_vuln_priority_for_agent(p_agent_id UUID)
RETURNS INTEGER
LANGUAGE plpgsql
AS $$
DECLARE
    n INTEGER;
BEGIN
    UPDATE vulnerability_findings vf
    SET priority_score = vuln_priority_score(
            vf.cvss, vf.kev_listed, vf.exploit_available, a.criticality
        ),
        updated_at = NOW()
    FROM agents a
    WHERE vf.agent_id = a.id
      AND vf.agent_id = p_agent_id;
    GET DIAGNOSTICS n = ROW_COUNT;
    RETURN n;
END$$;

-- Recomputes all findings (used after migration and on KEV sync).
CREATE OR REPLACE FUNCTION recompute_all_vuln_priorities()
RETURNS INTEGER
LANGUAGE plpgsql
AS $$
DECLARE
    n INTEGER;
BEGIN
    UPDATE vulnerability_findings vf
    SET priority_score = vuln_priority_score(
            vf.cvss, vf.kev_listed, vf.exploit_available, a.criticality
        ),
        updated_at = NOW()
    FROM agents a
    WHERE vf.agent_id = a.id;
    GET DIAGNOSTICS n = ROW_COUNT;
    RETURN n;
END$$;

-- ── Trigger: recompute on criticality change ─────────────────────────────────
CREATE OR REPLACE FUNCTION trg_agent_criticality_changed()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.criticality IS DISTINCT FROM OLD.criticality THEN
        PERFORM recompute_vuln_priority_for_agent(NEW.id);
    END IF;
    RETURN NEW;
END$$;

DROP TRIGGER IF EXISTS agents_criticality_changed ON agents;
CREATE TRIGGER agents_criticality_changed
    AFTER UPDATE OF criticality ON agents
    FOR EACH ROW
    EXECUTE FUNCTION trg_agent_criticality_changed();

-- ── Backfill existing rows ────────────────────────────────────────────────────
SELECT recompute_all_vuln_priorities();
