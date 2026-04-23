-- Rollback post-isolation pipeline tables (reverse dependency order)
DROP TABLE IF EXISTS ioc_enrichment;
DROP TABLE IF EXISTS triage_snapshots;
DROP TABLE IF EXISTS playbook_steps;
DROP TABLE IF EXISTS playbook_runs;
