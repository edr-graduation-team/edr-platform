/** Static mock payloads when parity APIs return 404 or are unreachable. */

import type { AppControlPoliciesPayload } from './appControlModel';

export const mockServiceSummary = {
    tenant_id: 'demo-tenant',
    window: { from: '2026-04-15T00:00:00Z', to: '2026-04-16T00:00:00Z' },
    sla_percent: 99.2,
    events_ingested: 842100,
    events_dropped: 12,
    alerts_generated: 1840,
    avg_detection_latency_ms: 380,
    avg_response_latency_ms: 1100,
    pipeline_health: 'healthy' as const,
};

export const mockEndpointSummary = {
    totals: {
        endpoints_total: 128,
        online: 118,
        offline: 8,
        isolated: 2,
    },
    risk_distribution: { critical: 3, high: 14, medium: 42, low: 69 },
    top_risky_endpoints: [
        { agent_id: 'demo-agent-1', hostname: 'FIN-WS-14', risk_score: 91, open_alerts: 7 },
        { agent_id: 'demo-agent-2', hostname: 'DEV-LAP-03', risk_score: 84, open_alerts: 4 },
    ],
    timeline: [
        { ts: '2026-04-16T08:00:00Z', critical: 1, high: 3, medium: 9, low: 22 },
        { ts: '2026-04-16T10:00:00Z', critical: 2, high: 4, medium: 11, low: 19 },
    ],
};

export const mockCloudSummary = {
    accounts_total: 6,
    misconfigurations_total: 48,
    critical_findings: 5,
    high_findings: 12,
    provider_breakdown: [
        { provider: 'aws', findings: 28 },
        { provider: 'azure', findings: 14 },
        { provider: 'gcp', findings: 6 },
    ],
};

export const mockRoi = {
    period_days: 30,
    incidents_prevented: 42,
    estimated_hours_saved: 320,
    estimated_cost_saved_usd: 98500,
    automation_success_rate: 0.86,
};

export const mockVerdictCloud = {
    lookups_total: 32000,
    malicious: 210,
    unknown: 890,
    benign: 30900,
    top_hashes: [
        { sha256: 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855', verdict: 'benign' as const, count: 1200 },
        { sha256: 'deadbeef00000000000000000000000000000000000000000000000000000000', verdict: 'malicious' as const, count: 14 },
    ],
};

export const mockListEnvelope = <T>(rows: T[], total = rows.length) => ({
    data: rows,
    pagination: { total, limit: 50, offset: 0, has_more: false },
    meta: { request_id: 'mock-req', timestamp: new Date().toISOString() },
});

export const mockComplianceEndpointRows = [
    {
        agent_id: 'a1',
        hostname: 'WS-DEMO-01',
        framework: 'cis_windows_11',
        score_percent: 82.5,
        passed_controls: 95,
        failed_controls: 18,
        last_scan_at: '2026-04-16T08:00:00Z',
    },
];

export const mockCtemExposure = {
    exposure_score: 64.0,
    critical_exposures: 9,
    high_exposures: 31,
    open_findings: 112,
    mttr_hours: 41.2,
    trend: [
        { ts: '2026-04-10T00:00:00Z', score: 68.0 },
        { ts: '2026-04-16T00:00:00Z', score: 64.0 },
    ],
};

export const mockCtemFindings = mockListEnvelope([
    {
        id: 'ctem_demo_1',
        title: 'Exposed management port (demo)',
        severity: 'high',
        status: 'open',
        asset_id: 'a1',
        asset_name: 'WS-DEMO-01',
        owner: 'sec-team',
        detected_at: '2026-04-15T10:00:00Z',
        due_at: '2026-04-22T00:00:00Z',
    },
]);

export const mockVulnFindings = mockListEnvelope([
    {
        id: 'v1',
        cve: 'CVE-2024-0001',
        severity: 'high',
        cvss: 8.1,
        asset_id: 'a1',
        hostname: 'WS-DEMO-01',
        status: 'open',
        published_at: '2026-03-01T00:00:00Z',
        detected_at: '2026-04-14T12:00:00Z',
    },
]);

export const mockPatchOverview = {
    compliance_percent: 88,
    pending_reboots: 4,
    last_scan_at: '2026-04-16T06:00:00Z',
};

export const mockPatchMissing = mockListEnvelope([
    {
        agent_id: 'a1',
        hostname: 'WS-DEMO-01',
        kb: 'KB5034123',
        title: 'Security update (demo)',
        severity: 'critical',
        due_at: '2026-04-20T00:00:00Z',
    },
]);

export const mockSecurityPostureEndpoint = {
    posture_score: 78,
    blocked_executions_24h: 12,
    containment_actions_24h: 3,
    policy_violations_open: 5,
};

export const mockSecurityPostureCloud = {
    accounts_at_risk: 2,
    identity_risks: 6,
    misconfig_critical: 4,
};

export const mockSiemConnectors = mockListEnvelope([
    { id: 'c1', type: 'splunk', name: 'Demo Splunk', enabled: false, status: 'disconnected' },
]);

export const mockThreatLabsIocs = mockListEnvelope([
    { id: 'ioc1', type: 'ip', value: '198.51.100.10', verdict: 'malicious', last_seen: '2026-04-16T09:00:00Z' },
]);

export const mockManagedIncidents = mockListEnvelope([
    {
        id: 'inc1',
        title: 'Suspicious lateral movement (demo)',
        severity: 'high',
        status: 'open',
        assignee: 'mss-team',
        opened_at: '2026-04-16T07:00:00Z',
    },
]);

export const mockManagedSla = {
    met_percent: 97.5,
    breaches_30d: 2,
    avg_response_min: 18,
};

export const mockItsmTickets = mockListEnvelope([
    {
        id: 't1',
        title: 'Review alert correlation (demo)',
        priority: 'medium',
        status: 'open',
        source_type: 'alert',
        source_id: 'alert-demo-1',
        created_at: '2026-04-16T08:30:00Z',
    },
]);

export const mockItsmPlaybooks = mockListEnvelope([
    { id: 'pb1', name: 'Isolate host on critical alert', enabled: true, runs_30d: 12 },
]);

export const mockManagementDevices = mockListEnvelope([
    {
        id: 'a1',
        hostname: 'WS-DEMO-01',
        status: 'online',
        os_type: 'windows' as const,
        os_version: '11',
        last_seen: '2026-04-16T10:00:00Z',
        health_score: 92,
        group: 'Finance',
        tags: { tier: '1' },
    },
]);

export const mockManagementProfiles = mockListEnvelope([
    { id: 'p1', name: 'Standard Workstation', enabled: true, endpoints: 84 },
]);

export const mockRmmJobs = mockListEnvelope([
    { id: 'j1', type: 'inventory', status: 'completed', agent_id: 'a1', started_at: '2026-04-16T05:00:00Z' },
]);

export const mockAppControlPolicies: AppControlPoliciesPayload = {
    data: [
        {
            id: 'ac-baseline',
            name: 'Workstation execution baseline',
            description: 'Allow signed Program Files; block untrusted roots and user-writable run paths.',
            scope_type: 'fleet',
            scope_label: 'All enrolled Windows endpoints',
            mode: 'enforce',
            state: 'published',
            priority: 10,
            rule_count: 28,
            coverage_percent: 94,
            endpoints_synced: 120,
            endpoints_lagged: 8,
            last_published_at: '2026-04-20T14:00:00Z',
            updated_at: '2026-04-22T09:15:00Z',
            audit_only_blocks_7d: 0,
            enforce_blocks_7d: 42,
        },
        {
            id: 'ac-dev-audit',
            name: 'Developer overrides (audit)',
            description: 'Broader execution in dev tags; observe only before switching to enforce.',
            scope_type: 'tag',
            scope_label: 'tier=dev',
            mode: 'audit',
            state: 'draft',
            priority: 20,
            rule_count: 11,
            coverage_percent: 18,
            endpoints_synced: 22,
            endpoints_lagged: 1,
            last_published_at: null,
            updated_at: '2026-04-23T11:40:00Z',
            audit_only_blocks_7d: 86,
            enforce_blocks_7d: 0,
        },
        {
            id: 'ac-temp-block',
            name: 'Block execution from user temp',
            description: 'Hash-less path rules for %TEMP% / AppData\\Local\\Temp launchers.',
            scope_type: 'fleet',
            scope_label: 'All enrolled Windows endpoints',
            mode: 'enforce',
            state: 'published',
            priority: 15,
            rule_count: 6,
            coverage_percent: 94,
            endpoints_synced: 118,
            endpoints_lagged: 10,
            last_published_at: '2026-04-18T08:30:00Z',
            updated_at: '2026-04-21T16:05:00Z',
            audit_only_blocks_7d: 0,
            enforce_blocks_7d: 19,
        },
        {
            id: 'ac-field-usb',
            name: 'Field sales — removable media',
            description: 'Deny known risky USB toolchains; allow corporate signed bundles only.',
            scope_type: 'group',
            scope_label: 'Group: Field Sales',
            mode: 'enforce',
            state: 'published',
            priority: 25,
            rule_count: 9,
            coverage_percent: 31,
            endpoints_synced: 38,
            endpoints_lagged: 3,
            last_published_at: '2026-04-10T12:00:00Z',
            updated_at: '2026-04-19T07:50:00Z',
            audit_only_blocks_7d: 0,
            enforce_blocks_7d: 3,
        },
    ],
    pagination: { total: 4, limit: 50, offset: 0, has_more: false },
    meta: { request_id: 'mock-req', timestamp: new Date().toISOString() },
    rollout_preview: [
        {
            hostname: 'FIN-WS-14',
            agent_id: '00000000-0000-4000-8000-000000000101',
            policy_sync: 'ok',
            last_policy_sync_at: '2026-04-23T20:40:00Z',
        },
        {
            hostname: 'DEV-LAP-03',
            agent_id: '00000000-0000-4000-8000-000000000102',
            policy_sync: 'lagging',
            last_policy_sync_at: '2026-04-22T14:10:00Z',
        },
        {
            hostname: 'HR-LT-09',
            agent_id: '00000000-0000-4000-8000-000000000103',
            policy_sync: 'ok',
            last_policy_sync_at: '2026-04-23T19:55:00Z',
        },
        {
            hostname: 'SALES-2-11',
            agent_id: '00000000-0000-4000-8000-000000000104',
            policy_sync: 'lagging',
            last_policy_sync_at: '2026-04-21T09:00:00Z',
        },
        {
            hostname: 'WS-DEMO-01',
            agent_id: 'a1',
            policy_sync: 'unknown',
            last_policy_sync_at: null,
        },
    ],
    audit_summary: {
        would_block_events_7d: 128,
        distinct_binaries_touched: 41,
    },
};

export const mockLicenses = {
    seats_total: 500,
    seats_used: 128,
    expires_at: '2027-01-01T00:00:00Z',
};

export const mockBillingSummary = {
    currency: 'USD',
    current_period_spend: 4200,
    forecast_next_month: 4450,
};
