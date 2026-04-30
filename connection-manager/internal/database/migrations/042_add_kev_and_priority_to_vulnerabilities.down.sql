DROP TABLE IF EXISTS kev_catalog;

DROP INDEX IF EXISTS uq_vuln_findings_agent_cve_pkg;
DROP INDEX IF EXISTS idx_vuln_findings_priority;
DROP INDEX IF EXISTS idx_vuln_findings_kev;

ALTER TABLE vulnerability_findings
    DROP COLUMN IF EXISTS reference_url,
    DROP COLUMN IF EXISTS installed_version,
    DROP COLUMN IF EXISTS priority_score,
    DROP COLUMN IF EXISTS exploit_available,
    DROP COLUMN IF EXISTS kev_due_date,
    DROP COLUMN IF EXISTS kev_added_date,
    DROP COLUMN IF EXISTS kev_listed;
