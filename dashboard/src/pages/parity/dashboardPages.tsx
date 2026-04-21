import { useQuery } from '@tanstack/react-query';
import { Link, Navigate } from 'react-router-dom';
import { parityApi } from '../../api/parity/parityApi';
import * as mocks from '../../api/parity/mocks';
import { GenericParityView } from '../../components/parity/GenericParityView';
import StatCard from '../../components/StatCard';
import {
    Activity, Server, Shield, Terminal, AlertTriangle, Database, Clock,
} from 'lucide-react';
import {
    agentsApi,
    alertsApi,
    commandsApi,
    reliabilityApi,
    statsApi,
} from '../../api/client';

/** Live data from connection-manager + sigma (self-hosted). */
export function DashboardServicePage() {
    const cmdQ = useQuery({ queryKey: ['commands-stats'], queryFn: () => commandsApi.stats(), staleTime: 30_000 });
    const alertQ = useQuery({ queryKey: ['sigma-stats-alerts'], queryFn: () => statsApi.alerts(), staleTime: 30_000 });
    const relQ = useQuery({
        queryKey: ['reliability-health'],
        queryFn: () => reliabilityApi.health(),
        staleTime: 60_000,
        retry: 1,
    });

    if (cmdQ.isLoading || alertQ.isLoading) {
        return <div className="h-40 rounded-xl bg-gray-100 dark:bg-gray-800 animate-pulse" />;
    }

    const cmd = cmdQ.data;
    const al = alertQ.data;

    const relReason = relQ.data?.fallback_store?.reason;
    const relEnabled = relQ.data?.fallback_store?.enabled === true;

    return (
        <div className="space-y-4">
            <div>
                <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Service summary</h2>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    From <code className="text-xs">GET /api/v1/commands/stats</code>,{' '}
                    <code className="text-xs">/api/v1/sigma/stats/alerts</code>, and{' '}
                    <code className="text-xs">/api/v1/reliability</code>.
                </p>
            </div>

            {(cmdQ.isError || alertQ.isError) && (
                <div className="rounded-lg border border-amber-200 dark:border-amber-800 bg-amber-50 dark:bg-amber-950/30 px-4 py-3 text-sm text-amber-900 dark:text-amber-200">
                    Some service metrics could not be loaded. Check API connectivity and roles.
                </div>
            )}

            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
                <StatCard
                    title="Commands (total)"
                    value={cmd ? String(cmd.total) : '—'}
                    icon={Terminal}
                    color="cyan"
                />
                <StatCard
                    title="Commands completed"
                    value={cmd ? String(cmd.completed) : '—'}
                    icon={Activity}
                    color="emerald"
                />
                <StatCard
                    title="Commands failed / timeout"
                    value={cmd ? `${cmd.failed} / ${cmd.timeout}` : '—'}
                    icon={AlertTriangle}
                    color="amber"
                />
                <StatCard
                    title="Sigma alerts (total)"
                    value={al ? String(al.total_alerts) : '—'}
                    icon={Shield}
                />
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
                <StatCard
                    title="Alerts (last 24h)"
                    value={al ? String(al.last_24h) : '—'}
                    icon={Clock}
                    color="cyan"
                />
                <StatCard
                    title="Open (status)"
                    value={al ? String(al.by_status?.open ?? 0) : '—'}
                    icon={AlertTriangle}
                    color="red"
                />
                <StatCard
                    title="Reliability fallback store"
                    value={relQ.isLoading ? '…' : relEnabled ? 'On' : 'Off'}
                    icon={Database}
                    color={relEnabled ? 'amber' : 'emerald'}
                />
            </div>

            {relReason && (
                <p className="text-xs text-gray-500 dark:text-gray-400">{relReason}</p>
            )}

            <div className="flex flex-wrap gap-3 text-sm">
                <Link to="/responses" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                    Action Center →
                </Link>
                <Link to="/stats" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                    Statistics →
                </Link>
            </div>
        </div>
    );
}

/** Live fleet posture from agents stats + endpoint risk. */
export function DashboardEndpointPage() {
    const statsQ = useQuery({ queryKey: ['agents-stats'], queryFn: () => agentsApi.stats(), staleTime: 30_000 });
    const riskQ = useQuery({
        queryKey: ['endpoint-risk'],
        queryFn: () => alertsApi.endpointRisk(),
        staleTime: 60_000,
        retry: 1,
    });

    if (statsQ.isLoading) {
        return <div className="h-40 rounded-xl bg-gray-100 dark:bg-gray-800 animate-pulse" />;
    }

    if (statsQ.isError || !statsQ.data) {
        return (
            <div className="rounded-xl border border-rose-200 dark:border-rose-900/50 bg-rose-50/80 dark:bg-rose-950/20 p-6 text-sm text-rose-900 dark:text-rose-200">
                Could not load agent statistics. Open{' '}
                <Link className="font-semibold underline" to="/management/devices">Device Management</Link> after verifying the connection-manager API.
            </div>
        );
    }

    const s = statsQ.data;
    const riskRows = riskQ.data?.data ?? [];
    const withOpenAlerts = riskRows.filter((r) => r.open_count > 0).length;

    return (
        <div className="space-y-4">
            <div>
                <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Endpoint summary</h2>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    From <code className="text-xs">GET /api/v1/agents/stats</code> and{' '}
                    <code className="text-xs">GET /api/v1/alerts/endpoint-risk</code>.
                </p>
            </div>

            {riskQ.isError && (
                <p className="text-xs text-amber-700 dark:text-amber-300">
                    Endpoint risk rows unavailable (check alerts:read / connection-manager).
                </p>
            )}

            <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                <StatCard title="Endpoints" value={String(s.total)} icon={Server} />
                <StatCard title="Online" value={String(s.online)} icon={Activity} color="emerald" />
                <StatCard title="Offline" value={String(s.offline)} icon={Server} />
                <StatCard title="Degraded" value={String(s.degraded)} icon={AlertTriangle} color="amber" />
            </div>

            <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                <StatCard title="Pending" value={String(s.pending)} icon={Clock} />
                <StatCard title="Suspended" value={String(s.suspended)} icon={Shield} color="red" />
                <StatCard title="Avg health" value={`${Math.round(s.avg_health)}%`} icon={Activity} color="cyan" />
                <StatCard
                    title="Agents w/ open alerts"
                    value={String(withOpenAlerts)}
                    icon={Shield}
                    color="amber"
                />
            </div>

            <div className="rounded-xl border border-gray-200 dark:border-gray-700 p-4 bg-white dark:bg-gray-800/80">
                <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-2">OS distribution</h3>
                <pre className="text-xs font-mono text-gray-600 dark:text-gray-400 overflow-auto max-h-40">
                    {JSON.stringify(s.by_os_type, null, 2)}
                </pre>
            </div>

            {riskRows.length > 0 && (
                <div className="rounded-xl border border-gray-200 dark:border-gray-700 p-4 bg-white dark:bg-gray-800/80">
                    <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-2">Top risk (open alerts)</h3>
                    <ul className="text-xs space-y-1 max-h-48 overflow-y-auto">
                        {[...riskRows]
                            .sort((a, b) => b.open_count - a.open_count)
                            .slice(0, 8)
                            .map((r) => (
                                <li key={r.agent_id} className="flex justify-between gap-2 font-mono">
                                    <Link
                                        className="text-cyan-600 dark:text-cyan-400 hover:underline truncate"
                                        to={`/management/devices/${encodeURIComponent(r.agent_id)}`}
                                    >
                                        {r.agent_id.slice(0, 8)}…
                                    </Link>
                                    <span className="text-gray-500 shrink-0">open: {r.open_count}</span>
                                </li>
                            ))}
                    </ul>
                </div>
            )}

            <div className="flex flex-wrap gap-3 text-sm">
                <Link to="/management/devices" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                    Device Management →
                </Link>
                <Link to="/endpoint-risk" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                    Risk Intelligence →
                </Link>
            </div>
        </div>
    );
}

/** Cloud SaaS dashboards are out of scope for this deployment — use Endpoint summary. */
export function DashboardCloudPage() {
    return <Navigate to="/dashboards/endpoint" replace />;
}

export function DashboardAuditRedirect() {
    return <Navigate to="/audit" replace />;
}

export function DashboardEndpointCompliancePage() {
    return (
        <GenericParityView
            title="Endpoint compliance"
            queryKey={['parity', 'compliance', 'endpoint']}
            fetcher={() => parityApi.getComplianceEndpoint()}
            mock={mocks.mockComplianceEndpointRows}
        />
    );
}

export function DashboardCtemPage() {
    return (
        <div className="space-y-6">
            <GenericParityView
                title="CTEM exposure summary"
                queryKey={['parity', 'ctem', 'exposure']}
                fetcher={() => parityApi.getCtemExposureSummary()}
                mock={mocks.mockCtemExposure}
            />
            <GenericParityView
                title="CTEM findings"
                queryKey={['parity', 'ctem', 'findings']}
                fetcher={() => parityApi.getCtemFindings()}
                mock={mocks.mockCtemFindings.data}
            />
        </div>
    );
}

/** Commercial verdict cloud — not used; fleet view lives under Endpoint. */
export function DashboardVerdictCloudPage() {
    return <Navigate to="/dashboards/endpoint" replace />;
}

export function DashboardRoiPage() {
    return (
        <GenericParityView
            title="ROI dashboard"
            queryKey={['parity', 'dashboard', 'roi']}
            fetcher={() => parityApi.getRoi()}
            mock={mocks.mockRoi}
        />
    );
}

export function DashboardReportsPage() {
    return (
        <div className="rounded-xl border border-dashed border-gray-300 dark:border-gray-600 p-8 text-center text-gray-500 dark:text-gray-400">
            <p className="font-medium text-gray-700 dark:text-gray-300">Reports</p>
            <p className="text-sm mt-2">Scheduled PDF/CSV reports will connect here when the API is available.</p>
            <p className="text-sm mt-4">
                <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/audit">
                    Audit logs
                </Link>
                {' · '}
                <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/stats">
                    Statistics export
                </Link>
            </p>
        </div>
    );
}

export function DashboardNotificationsPage() {
    return (
        <div className="rounded-xl border border-dashed border-gray-300 dark:border-gray-600 p-8 text-center text-gray-500 dark:text-gray-400">
            <p className="font-medium text-gray-700 dark:text-gray-300">Notifications</p>
            <p className="text-sm mt-2">In-app notification center — wiring to `/api/v1/...` when ready.</p>
            <p className="text-sm mt-4 text-left max-w-md mx-auto text-gray-600 dark:text-gray-300">
                For now, monitor <Link className="text-cyan-600 dark:text-cyan-400 font-medium" to="/alerts">Alerts</Link>
                {' '}and <Link className="text-cyan-600 dark:text-cyan-400 font-medium" to="/">Overview</Link> live streams.
            </p>
        </div>
    );
}
