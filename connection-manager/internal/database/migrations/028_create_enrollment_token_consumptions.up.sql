-- Migration: 028_create_enrollment_token_consumptions
-- Track per-device consumption of enrollment tokens for idempotency.

CREATE TABLE IF NOT EXISTS enrollment_token_consumptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_id UUID NOT NULL REFERENCES enrollment_tokens(id) ON DELETE CASCADE,
    hardware_id VARCHAR(128) NOT NULL,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Each hardware_id should only consume a given token once.
CREATE UNIQUE INDEX IF NOT EXISTS idx_enrollment_token_consumptions_token_hw_unique
ON enrollment_token_consumptions(token_id, hardware_id);

CREATE INDEX IF NOT EXISTS idx_enrollment_token_consumptions_token_id
ON enrollment_token_consumptions(token_id);

