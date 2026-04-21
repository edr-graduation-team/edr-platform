import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { GenericParityView } from '../../components/parity/GenericParityView';
import { useParityQuery } from '../../api/parity/withFallback';
import { parityApi } from '../../api/parity/parityApi';
import * as mocks from '../../api/parity/mocks';
import StatCard from '../../components/StatCard';
import { Activity, Shield, Loader2 } from 'lucide-react';
import { ParityMockBanner } from '../../components/parity/ParityMockBanner';
import { agentsApi } from '../../api/client';

/** Commercial / MSP modules outside current self-hosted endpoint scope. */
function SelfHostedOutOfScope({ title }: { title: string }) {
    return (
        <div className="rounded-xl border border-amber-200 dark:border-amber-900/40 bg-amber-50/90 dark:bg-amber-950/25 p-6 space-y-3">
            <p className="font-semibold text-gray-900 dark:text-white">{title}</p>
            <p className="text-sm text-gray-600 dark:text-gray-400">
                Not part of the self-hosted EDR MVP. Use the sections below for real fleet and response workflows.
            </p>
            <div className="flex flex-wrap gap-x-4 gap-y-2 text-sm">
                <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/management/devices">
                    Device Management
                </Link>
                <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/responses">
                    Action Center
                </Link>
                <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/settings/users">
                    Users
                </Link>
            </div>
        </div>
    );
}

function PlaceholderPage({ title, hint }: { title: string; hint: string }) {
    return (
        <div className="rounded-xl border border-dashed border-gray-300 dark:border-gray-600 p-8 text-center text-gray-500 dark:text-gray-400">
            <p className="font-medium text-gray-700 dark:text-gray-300">{title}</p>
            <p className="text-sm mt-2">{hint}</p>
        </div>
    );
}

export function SecurityEndpointZeroTrustPage() {
    return (
        <GenericParityView
            title="Endpoint Zero Trust"
            description="EPP + EDR + Zero Trust posture (parity API)."
            queryKey={['parity', 'security', 'posture', 'endpoint']}
            fetcher={() => parityApi.getSecurityPostureEndpoint()}
            mock={mocks.mockSecurityPostureEndpoint}
        />
    );
}

export function SecurityCloudZeroTrustPage() {
    return (
        <GenericParityView
            title="Cloud Security — Zero Trust"
            queryKey={['parity', 'security', 'posture', 'cloud']}
            fetcher={() => parityApi.getSecurityPostureCloud()}
            mock={mocks.mockSecurityPostureCloud}
        />
    );
}

export function SecuritySiemPage() {
    return (
        <GenericParityView
            title="SIEM connectors"
            queryKey={['parity', 'siem', 'connectors']}
            fetcher={() => parityApi.getSiemConnectors()}
            mock={mocks.mockSiemConnectors.data}
        />
    );
}

export function SecurityThreatLabsPage() {
    return (
        <GenericParityView
            title="Threat Labs — IOC feed"
            queryKey={['parity', 'threat-labs', 'iocs']}
            fetcher={() => parityApi.getThreatLabsIocs()}
            mock={mocks.mockThreatLabsIocs.data}
        />
    );
}

export function ManagedSecurityOverviewPage() {
    const q = useParityQuery(['parity', 'managed', 'sla', 'overview'], () => parityApi.getManagedSla(), mocks.mockManagedSla);

    if (q.isLoading) return <div className="h-40 rounded-xl bg-gray-100 dark:bg-gray-800 animate-pulse" />;
    if (q.isError || !q.data) return null;
    const { data: rawData, isMock } = q.data;
    const data = rawData as {
        met_percent: number;
        breaches_30d: number;
        avg_response_min: number;
        [key: string]: unknown;
    };

    return (
        <div className="space-y-4">
            <div>
                <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Managed Security — overview</h2>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    High-level posture for the managed service desk. Open incidents live under the Incidents tab.
                </p>
            </div>
            {isMock && <ParityMockBanner />}
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
                <StatCard title="SLA met" value={`${data.met_percent}%`} icon={Shield} color="emerald" />
                <StatCard title="Breaches (30d)" value={String(data.breaches_30d)} icon={Activity} color="amber" />
                <StatCard title="Avg response (min)" value={String(data.avg_response_min)} icon={Activity} color="cyan" />
            </div>
            <details className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800/80 p-4">
                <summary className="cursor-pointer text-sm font-medium text-gray-700 dark:text-gray-300">Raw SLA payload</summary>
                <pre className="text-xs mt-3 overflow-auto max-h-64 font-mono text-gray-600 dark:text-gray-400">
                    {JSON.stringify(data, null, 2)}
                </pre>
            </details>
        </div>
    );
}

export function ManagedSecurityIncidentsPage() {
    return (
        <GenericParityView
            title="Managed Security — incidents"
            queryKey={['parity', 'managed', 'incidents']}
            fetcher={() => parityApi.getManagedIncidents()}
            mock={mocks.mockManagedIncidents.data}
        />
    );
}

export function ManagedSecuritySlaPage() {
    return (
        <GenericParityView
            title="Managed Security — SLA"
            queryKey={['parity', 'managed', 'sla']}
            fetcher={() => parityApi.getManagedSla()}
            mock={mocks.mockManagedSla}
        />
    );
}

export function ItsmTicketsPage() {
    return (
        <GenericParityView
            title="ITSM — tickets"
            queryKey={['parity', 'itsm', 'tickets']}
            fetcher={() => parityApi.getItsmTickets()}
            mock={mocks.mockItsmTickets}
        />
    );
}

export function ItsmPlaybooksPage() {
    return (
        <GenericParityView
            title="ITSM — playbooks"
            queryKey={['parity', 'itsm', 'playbooks']}
            fetcher={() => parityApi.getItsmPlaybooks()}
            mock={mocks.mockItsmPlaybooks.data}
        />
    );
}

export function ItsmAutomationsPage() {
    return (
        <PlaceholderPage
            title="ITSM — automations"
            hint="Automation catalog and triggers will map to `/api/v1/itsm/automations` when the backend is ready."
        />
    );
}

export function ItsmIntegrationsPage() {
    return (
        <PlaceholderPage
            title="ITSM — integrations"
            hint="Connectors for ticketing, chat, and webhooks will appear here once APIs are exposed."
        />
    );
}

export function ManagementDevicesPage() {
    return (
        <GenericParityView
            title="Device management"
            description="Aligned with `/management/devices` — can mirror `/api/v1/agents` later."
            queryKey={['parity', 'management', 'devices']}
            fetcher={() => parityApi.getManagementDevices()}
            mock={mocks.mockManagementDevices}
        />
    );
}

/** Fleet addresses + isolation — from live `GET /api/v1/agents` (no separate network topology API yet). */
export function ManagementNetworkPage() {
    const q = useQuery({
        queryKey: ['management-network-fleet'],
        queryFn: () => agentsApi.list({ limit: 200, sort_by: 'hostname', sort_order: 'asc' }),
        staleTime: 30_000,
    });

    if (q.isLoading) {
        return (
            <div className="flex items-center justify-center py-16 text-gray-500 gap-2">
                <Loader2 className="w-6 h-6 animate-spin" /> Loading agents…
            </div>
        );
    }

    if (q.isError || !q.data?.data) {
        return (
            <div className="rounded-lg border border-rose-200 dark:border-rose-900/50 p-4 text-sm text-rose-800 dark:text-rose-200">
                Could not load agents. Check connection-manager and <code className="text-xs">endpoints:read</code>.
            </div>
        );
    }

    const rows = q.data.data;

    return (
        <div className="space-y-4">
            <div>
                <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Fleet connectivity</h2>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    Last-seen addresses reported by enrolled agents. Full network telemetry belongs in{' '}
                    <Link className="text-cyan-600 dark:text-cyan-400 hover:underline" to="/management/devices">
                        device detail → Network
                    </Link>
                    {' '}when event search is wired.
                </p>
            </div>
            <div className="overflow-x-auto rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40">
                <table className="min-w-full text-left text-sm">
                    <thead className="bg-gray-50 dark:bg-gray-800/80 text-gray-600 dark:text-gray-400 text-xs uppercase">
                        <tr>
                            <th className="px-3 py-2">Host</th>
                            <th className="px-3 py-2">Status</th>
                            <th className="px-3 py-2">IPs</th>
                            <th className="px-3 py-2">Isolated</th>
                        </tr>
                    </thead>
                    <tbody>
                        {rows.map((a) => (
                            <tr key={a.id} className="border-t border-gray-100 dark:border-gray-800">
                                <td className="px-3 py-2">
                                    <Link
                                        className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline"
                                        to={`/management/devices/${encodeURIComponent(a.id)}`}
                                    >
                                        {a.hostname}
                                    </Link>
                                </td>
                                <td className="px-3 py-2 font-mono text-xs">{a.status}</td>
                                <td className="px-3 py-2 text-xs break-all max-w-md">
                                    {(a.ip_addresses || []).join(', ') || '—'}
                                </td>
                                <td className="px-3 py-2">{a.is_isolated ? 'Yes' : 'No'}</td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>
        </div>
    );
}

export function ManagementStaffPage() {
    return <SelfHostedOutOfScope title="Staff / shifts" />;
}

export function ManagementAccountPage() {
    return <SelfHostedOutOfScope title="Account / tenant branding" />;
}

export function ManagementProfilesPage() {
    return (
        <GenericParityView
            title="Profile management"
            queryKey={['parity', 'management', 'profiles']}
            fetcher={() => parityApi.getManagementProfiles()}
            mock={mocks.mockManagementProfiles.data}
        />
    );
}

export function ManagementRmmPage() {
    return <SelfHostedOutOfScope title="Remote monitoring & management (RMM)" />;
}

export function ManagementPatchPage() {
    return (
        <div className="space-y-6">
            <GenericParityView
                title="Patch — overview"
                queryKey={['parity', 'patch', 'overview']}
                fetcher={() => parityApi.getPatchOverview()}
                mock={mocks.mockPatchOverview}
            />
            <GenericParityView
                title="Patch — missing"
                queryKey={['parity', 'patch', 'missing']}
                fetcher={() => parityApi.getPatchMissing()}
                mock={mocks.mockPatchMissing.data}
            />
        </div>
    );
}

export function ManagementVulnPage() {
    return (
        <GenericParityView
            title="Vulnerability findings"
            queryKey={['parity', 'vuln', 'findings']}
            fetcher={() => parityApi.getVulnFindings()}
            mock={mocks.mockVulnFindings.data}
        />
    );
}

export function ManagementAppControlPage() {
    return (
        <GenericParityView
            title="Application control policies"
            queryKey={['parity', 'management', 'app-control']}
            fetcher={() => parityApi.getAppControlPolicies()}
            mock={mocks.mockAppControlPolicies.data}
        />
    );
}

export function ManagementLicensesPage() {
    return <SelfHostedOutOfScope title="Licenses" />;
}

export function ManagementBillingPage() {
    return <SelfHostedOutOfScope title="Billing" />;
}
