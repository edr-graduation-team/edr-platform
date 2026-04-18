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

export const DASHBOARD_MAIN_TABS: ContextTab[] = [
    { to: '/dashboards/service', label: 'Service' },
    { to: '/dashboards/endpoint', label: 'Endpoint' },
    { to: '/dashboards/cloud', label: 'Cloud' },
    { to: '/dashboards/audit', label: 'Audit' },
    { to: '/dashboards/endpoint-compliance', label: 'Endpoint Compliance' },
    { to: '/dashboards/ctem-compliance', label: 'CTEM Compliance' },
    { to: '/dashboards/verdict-cloud', label: 'Xcitium Verdict Cloud' },
];

export const DASHBOARD_MORE_TABS: ContextTab[] = [
    { to: '/dashboards/reports', label: 'Reports' },
    { to: '/dashboards/notifications', label: 'Notifications' },
    { to: '/dashboards/audit-logs', label: 'Audit Logs' },
    { to: '/dashboards/roi', label: 'ROI Dashboard' },
];

export const SECURITY_MODULE_TABS: ContextTab[] = [
    { to: '/security/endpoint-zero-trust', label: 'Endpoint Zero Trust' },
    { to: '/security/cloud-zero-trust', label: 'Cloud — Zero Trust' },
    { to: '/security/siem-x', label: 'SIEM — X' },
    { to: '/security/threat-labs', label: 'Threat Labs' },
];

export const SOC_CONTEXT_TABS: ContextTab[] = [
    { to: '/', label: 'Overview' },
    { to: '/stats', label: 'Statistics' },
    { to: '/alerts', label: 'Alerts' },
    { to: '/endpoints', label: 'Endpoints' },
    { to: '/endpoint-risk', label: 'Risk Intelligence' },
    { to: '/threats', label: 'Threats' },
    { to: '/rules', label: 'Rules' },
    { to: '/responses', label: 'Action Center' },
];

export const MANAGED_SECURITY_TABS: ContextTab[] = [
    { to: '/managed-security/overview', label: 'Overview' },
    { to: '/managed-security/incidents', label: 'Incidents' },
    { to: '/managed-security/sla', label: 'SLA' },
];

export const ITSM_TABS: ContextTab[] = [
    { to: '/itsm/tickets', label: 'Tickets' },
    { to: '/itsm/playbooks', label: 'Playbooks' },
    { to: '/itsm/automations', label: 'Automations' },
    { to: '/itsm/integrations', label: 'Integrations' },
];

export const MANAGEMENT_TABS: ContextTab[] = [
    { to: '/management/devices', label: 'Device Management' },
    { to: '/management/profiles', label: 'Profile Management' },
    { to: '/management/rmm', label: 'RMM' },
    { to: '/management/patch', label: 'Patch Management' },
    { to: '/management/vulnerability', label: 'Vulnerability' },
    { to: '/management/network', label: 'Network' },
    { to: '/management/app-control', label: 'Application Control' },
    { to: '/management/staff', label: 'Staff' },
    { to: '/management/account', label: 'Account' },
    { to: '/management/users', label: 'Users' },
    { to: '/management/licenses', label: 'Licenses' },
    { to: '/management/billing', label: 'Billing' },
];

export function isDashboardMorePath(pathname: string): boolean {
    return DASHBOARD_MORE_TABS.some((t) => pathname === t.to || pathname.startsWith(`${t.to}/`));
}
