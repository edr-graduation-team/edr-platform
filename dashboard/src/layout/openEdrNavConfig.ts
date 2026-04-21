/** Context (second-row) tabs + path helpers for OpenEDR-style shell. */

export type ContextTab = { to: string; label: string; end?: boolean };

export const SOC_PATHS = [
    '/alerts',
    '/endpoints',
    '/endpoint-risk',
    '/threats',
    '/rules',
    '/responses',
] as const;

export function isSocPath(pathname: string): boolean {
    return SOC_PATHS.some((p) => pathname === p || pathname.startsWith(`${p}/`));
}

/** Hub tabs — Cloud / Verdict SaaS views omitted for self-hosted endpoint focus (routes may still exist). */
export const DASHBOARD_MAIN_TABS: ContextTab[] = [
    { to: '/dashboards/service', label: 'Security Posture' },
    { to: '/dashboards/endpoint', label: 'Endpoint Summary' },
    { to: '/dashboards/audit', label: 'Audit Logs' },
    { to: '/dashboards/endpoint-compliance', label: 'Endpoint Compliance' },
    { to: '/dashboards/reports', label: 'Reports' },
    { to: '/dashboards/notifications', label: 'Notifications' },
];

export const DASHBOARD_MORE_TABS: ContextTab[] = [
    // Intentionally empty: “More” menu removed; tabs are shown directly.
];

export const SECURITY_MODULE_TABS: ContextTab[] = [
    { to: '/security/endpoint-zero-trust', label: 'Endpoint Zero Trust' },
    { to: '/security/siem-x', label: 'SIEM — X' },
];

export const SOC_CONTEXT_TABS: ContextTab[] = [
    { to: '/', label: 'Essential Platform' },
    { to: '/stats', label: 'Reports & Statistics' },
    { to: '/alerts', label: 'Alerts (Triage)' },
    { to: '/events', label: 'Telemetry Search' },
    { to: '/endpoints', label: 'Devices' },
    { to: '/endpoint-risk', label: 'Endpoint Risk' },
    { to: '/threats', label: 'ATT&CK Analytics' },
    { to: '/rules', label: 'Detection Rules' },
    { to: '/responses', label: 'Command Center' },
];

export const MANAGED_SECURITY_TABS: ContextTab[] = [
    { to: '/managed-security/overview', label: 'Managed Overview' },
    { to: '/managed-security/incidents', label: 'Incidents (Alerts)' },
];

export const ITSM_TABS: ContextTab[] = [
    { to: '/itsm/tickets', label: 'Tickets' },
    { to: '/itsm/playbooks', label: 'Response Playbooks' },
    { to: '/itsm/automations', label: 'Response Automations' },
    { to: '/itsm/integrations', label: 'Integrations' },
];

/** RMM / licenses / billing / staff removed from nav — not in self-hosted MVP scope (URLs unchanged for bookmarks). */
export const MANAGEMENT_TABS: ContextTab[] = [
    { to: '/management/devices', label: 'Devices (Fleet)' },
    { to: '/management/vulnerability', label: 'Vulnerability Triage' },
    { to: '/management/network', label: 'Fleet Connectivity' },
    { to: '/management/app-control', label: 'Application Control' },
    { to: '/management/account', label: 'Account (Out of scope)' },
    { to: '/management/users', label: 'Users' },
];

export function isDashboardMorePath(_pathname: string): boolean {
    return false;
}
