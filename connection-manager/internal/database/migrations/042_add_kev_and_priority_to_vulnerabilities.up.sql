-- Add KEV (CISA Known Exploited Vulnerabilities) tracking + risk fields
-- Enriches vulnerability_findings with exploit intelligence and bulk import support.

ALTER TABLE vulnerability_findings
    ADD COLUMN IF NOT EXISTS kev_listed       BOOLEAN          NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS kev_added_date   TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS kev_due_date     TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS exploit_available BOOLEAN          NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS priority_score   DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS installed_version TEXT            NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS reference_url    TEXT             NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_vuln_findings_kev ON vulnerability_findings(kev_listed) WHERE kev_listed = true;
CREATE INDEX IF NOT EXISTS idx_vuln_findings_priority ON vulnerability_findings(priority_score DESC);

-- Unique key for upsert: (agent_id, cve, package_name) — one finding per CVE+package per host.
-- Pre-step: collapse pre-existing duplicates (keep newest by updated_at) so the unique index can build.
DELETE FROM vulnerability_findings a
USING vulnerability_findings b
WHERE a.cve <> ''
  AND a.agent_id     = b.agent_id
  AND a.cve          = b.cve
  AND a.package_name = b.package_name
  AND (a.updated_at, a.id) < (b.updated_at, b.id);

-- Partial unique index allowing same CVE on different packages or hosts; skips empty CVEs.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_indexes
        WHERE schemaname = 'public'
          AND indexname = 'uq_vuln_findings_agent_cve_pkg'
    ) THEN
        CREATE UNIQUE INDEX uq_vuln_findings_agent_cve_pkg
            ON vulnerability_findings (agent_id, cve, package_name)
            WHERE cve <> '';
    END IF;
END$$;

-- CISA KEV catalog cache (full catalog for matching against future findings).
CREATE TABLE IF NOT EXISTS kev_catalog (
    cve              VARCHAR(64) PRIMARY KEY,
    vendor_project   TEXT NOT NULL DEFAULT '',
    product          TEXT NOT NULL DEFAULT '',
    vulnerability_name TEXT NOT NULL DEFAULT '',
    date_added       TIMESTAMPTZ,
    short_description TEXT NOT NULL DEFAULT '',
    required_action  TEXT NOT NULL DEFAULT '',
    due_date         TIMESTAMPTZ,
    known_ransomware TEXT NOT NULL DEFAULT '',
    notes            TEXT NOT NULL DEFAULT '',
    synced_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kev_catalog_synced_at ON kev_catalog(synced_at DESC);
