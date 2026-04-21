/** Static mock payloads when parity APIs return 404 or are unreachable. */

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

export const mockAppControlPolicies = mockListEnvelope([
    { id: 'ac1', name: 'Block untrusted USB apps', mode: 'enforce', violations_7d: 0 },
]);

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
