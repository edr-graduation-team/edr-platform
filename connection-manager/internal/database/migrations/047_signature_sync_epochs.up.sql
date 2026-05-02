-- Migration 047: Replace per-row sequence versioning with per-sync-epoch versioning.
--
-- BEFORE: each inserted row consumed one value from malware_hashes_version_seq,
--         so ON CONFLICT DO NOTHING still advanced the sequence, causing
--         max_version >> count (e.g. version=4377 with only 807 rows).
--
-- AFTER:  one epoch row is created per successful sync run.  All hashes inserted
--         in that run share the same version = epoch.id.
--         max_version = number of syncs that actually inserted new hashes.
--         Agents at version 0 (built-in BBolt hashes) pull everything > 0.

CREATE TABLE IF NOT EXISTS signature_sync_epochs (
    id              BIGSERIAL    PRIMARY KEY,
    synced_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    hashes_inserted BIGINT       NOT NULL DEFAULT 0
);

-- Consolidate all pre-existing rows into epoch 1 so delta pulls still work.
DO $$
DECLARE
    existing_count BIGINT;
BEGIN
    SELECT COUNT(*) INTO existing_count FROM malware_hashes;
    IF existing_count > 0 THEN
        INSERT INTO signature_sync_epochs (hashes_inserted) VALUES (existing_count);
        UPDATE malware_hashes SET version = 1;
    END IF;
END $$;

-- Remove the per-row sequence default; version is now set explicitly.
ALTER TABLE malware_hashes ALTER COLUMN version DROP DEFAULT;
DROP SEQUENCE IF EXISTS malware_hashes_version_seq;
