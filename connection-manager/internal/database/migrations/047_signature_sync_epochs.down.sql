DROP TABLE IF EXISTS signature_sync_epochs;

CREATE SEQUENCE IF NOT EXISTS malware_hashes_version_seq START 1;

ALTER TABLE malware_hashes
    ALTER COLUMN version SET DEFAULT nextval('malware_hashes_version_seq');
