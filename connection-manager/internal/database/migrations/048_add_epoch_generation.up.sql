ALTER TABLE signature_sync_epochs
    ADD COLUMN IF NOT EXISTS generation BIGINT;
