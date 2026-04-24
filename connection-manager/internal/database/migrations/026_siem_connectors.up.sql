-- SIEM / external analytics destinations (forwarder config). Distinct from in-app Alerts/Events UIs.

CREATE TABLE IF NOT EXISTS siem_connectors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    connector_type VARCHAR(64) NOT NULL,
    endpoint_url TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(32) NOT NULL DEFAULT 'never_tested',
    last_test_at TIMESTAMPTZ,
    last_error TEXT,
    notes TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT siem_connectors_type_chk CHECK (connector_type IN (
        'splunk_hec', 'azure_sentinel', 'elastic_webhook', 'generic_webhook', 'syslog_tls'
    )),
    CONSTRAINT siem_connectors_status_chk CHECK (status IN ('never_tested', 'ok', 'degraded', 'error', 'disabled'))
);

CREATE INDEX IF NOT EXISTS idx_siem_connectors_enabled ON siem_connectors(enabled);
CREATE INDEX IF NOT EXISTS idx_siem_connectors_type ON siem_connectors(connector_type);
