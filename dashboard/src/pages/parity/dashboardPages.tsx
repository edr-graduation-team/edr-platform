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
import { formatRelativeTime, getEffectiveStatus } from '../../utils/agentDisplay';

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
                    From command metrics,{' '}
                    sigma alert metrics, and{' '}
                    reliability data.
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
                    Command Center →
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
                    From agent metrics and{' '}
                    endpoint risk scoring.
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
                    Devices (Fleet) →
                </Link>
                <Link to="/endpoint-risk" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                    Endpoint Risk →
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
    const agentsQ = useQuery({
        queryKey: ['agents', 'compliance'],
        queryFn: () => agentsApi.list({ limit: 500, sort_by: 'hostname', sort_order: 'asc' }),
        staleTime: 30_000,
        retry: 1,
    });

    if (agentsQ.isLoading) return <div className="h-40 rounded-xl bg-gray-100 dark:bg-gray-800 animate-pulse" />;
    if (agentsQ.isError || !agentsQ.data?.data) {
        return (
            <div className="rounded-xl border border-rose-200 dark:border-rose-900/50 bg-rose-50/80 dark:bg-rose-950/20 p-6 text-sm text-rose-900 dark:text-rose-200">
                Could not load agents for compliance. Check connection-manager and <code className="text-xs">endpoints:read</code>.
            </div>
        );
    }

    const rows = agentsQ.data.data;

    const evalCompliance = (a: (typeof rows)[number]) => {
        const reasons: string[] = [];
        const eff = getEffectiveStatus(a);
        if (eff !== 'online' && eff !== 'degraded') reasons.push('Agent not online');
        if ((a.health_score ?? 0) < 80) reasons.push('Health < 80%');
        if (a.is_isolated) reasons.push('Network isolated');
        if (!a.current_cert_id) reasons.push('Missing mTLS cert');
        const certExpiry = a.cert_expires_at ? new Date(a.cert_expires_at) : null;
        if (certExpiry && certExpiry.getTime() < Date.now()) reasons.push('mTLS cert expired');
        return { compliant: reasons.length === 0, reasons };
    };

    const compliantCount = rows.filter((a) => evalCompliance(a).compliant).length;

    return (
        <div className="space-y-4">
            <div>
                <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Endpoint compliance</h2>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    Self-hosted compliance view built from enrolled agent posture (status, health, isolation, mTLS). This is not a patch/CVE scanner.
                </p>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
                <StatCard title="Endpoints" value={String(rows.length)} icon={Server} />
                <StatCard title="Compliant" value={String(compliantCount)} icon={Shield} color="emerald" />
                <StatCard title="Non-compliant" value={String(rows.length - compliantCount)} icon={AlertTriangle} color="amber" />
                <StatCard title="Isolated" value={String(rows.filter((a) => a.is_isolated).length)} icon={AlertTriangle} color="red" />
            </div>

            <div className="overflow-x-auto rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40">
                <table className="min-w-full text-left text-sm">
                    <thead className="bg-gray-50 dark:bg-gray-800/80 text-gray-600 dark:text-gray-400 text-xs uppercase">
                        <tr>
                            <th className="px-3 py-2">Host</th>
                            <th className="px-3 py-2">Status</th>
                            <th className="px-3 py-2">Health</th>
                            <th className="px-3 py-2">Last seen</th>
                            <th className="px-3 py-2">Compliance</th>
                            <th className="px-3 py-2">Reasons</th>
                        </tr>
                    </thead>
                    <tbody>
                        {rows.map((a) => {
                            const eff = getEffectiveStatus(a);
                            const c = evalCompliance(a);
                            return (
                                <tr key={a.id} className="border-t border-gray-100 dark:border-gray-800">
                                    <td className="px-3 py-2">
                                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to={`/management/devices/${encodeURIComponent(a.id)}`}>
                                            {a.hostname}
                                        </Link>
                                    </td>
                                    <td className="px-3 py-2 text-xs font-mono">{eff}</td>
                                    <td className="px-3 py-2 text-xs font-mono">{Math.round(a.health_score ?? 0)}%</td>
                                    <td className="px-3 py-2 text-xs">{formatRelativeTime(a.last_seen)}</td>
                                    <td className="px-3 py-2 text-xs font-semibold">{c.compliant ? 'Compliant' : 'Non-compliant'}</td>
                                    <td className="px-3 py-2 text-xs text-gray-500 max-w-md">
                                        {c.reasons.length ? c.reasons.join(' · ') : '—'}
                                    </td>
                                </tr>
                            );
                        })}
                    </tbody>
                </table>
            </div>
        </div>
    );
}

export function DashboardCtemPage() {
    return (
        <div className="space-y-6">
            <GenericParityView
                title="CTEM exposure summary"
                missingApi="true"
                queryKey={['parity', 'ctem', 'exposure']}
                fetcher={() => parityApi.getCtemExposureSummary()}
                mock={mocks.mockCtemExposure}
            />
            <GenericParityView
                title="CTEM findings"
                missingApi="true"
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
            missingApi="true"
            queryKey={['parity', 'dashboard', 'roi']}
            fetcher={() => parityApi.getRoi()}
            mock={mocks.mockRoi}
        />
    );
}

export function DashboardReportsPage() {
    const agentsQ = useQuery({ queryKey: ['agents-stats'], queryFn: () => agentsApi.stats(), staleTime: 30_000 });
    const cmdQ = useQuery({ queryKey: ['commands-stats'], queryFn: () => commandsApi.stats(), staleTime: 30_000 });
    const alertQ = useQuery({ queryKey: ['sigma-stats-alerts'], queryFn: () => statsApi.alerts(), staleTime: 30_000 });

    const downloadJson = (name: string, payload: unknown) => {
        const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' });
        const a = document.createElement('a');
        a.href = URL.createObjectURL(blob);
        a.download = `${name}-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.json`;
        a.click();
        URL.revokeObjectURL(a.href);
    };

    return (
        <div className="space-y-4">
            <div>
                <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Reports</h2>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    MVP report exports (JSON snapshots) using live APIs. PDF/CSV scheduling can be added later.
                </p>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
                <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40 p-4 space-y-2">
                    <div className="font-semibold text-gray-900 dark:text-white">Fleet snapshot</div>
                    <div className="text-xs text-gray-500">From agent metrics.</div>
                    <button
                        type="button"
                        className="px-3 py-2 rounded-lg text-sm font-semibold bg-cyan-600 hover:bg-cyan-700 text-white disabled:opacity-50"
                        disabled={!agentsQ.data}
                        onClick={() => downloadJson('fleet-stats', agentsQ.data)}
                    >
                        Download JSON
                    </button>
                </div>

                <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40 p-4 space-y-2">
                    <div className="font-semibold text-gray-900 dark:text-white">Command operations</div>
                    <div className="text-xs text-gray-500">From command metrics.</div>
                    <button
                        type="button"
                        className="px-3 py-2 rounded-lg text-sm font-semibold bg-cyan-600 hover:bg-cyan-700 text-white disabled:opacity-50"
                        disabled={!cmdQ.data}
                        onClick={() => downloadJson('commands-stats', cmdQ.data)}
                    >
                        Download JSON
                    </button>
                </div>

                <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40 p-4 space-y-2">
                    <div className="font-semibold text-gray-900 dark:text-white">Alert summary</div>
                    <div className="text-xs text-gray-500">From alert metrics.</div>
                    <button
                        type="button"
                        className="px-3 py-2 rounded-lg text-sm font-semibold bg-cyan-600 hover:bg-cyan-700 text-white disabled:opacity-50"
                        disabled={!alertQ.data}
                        onClick={() => downloadJson('sigma-alert-stats', alertQ.data)}
                    >
                        Download JSON
                    </button>
                </div>
            </div>

            <div className="rounded-xl border border-dashed border-gray-300 dark:border-gray-700 bg-white/50 dark:bg-gray-900/20 p-4 text-sm text-gray-600 dark:text-gray-400">
                For full datasets, use the primary pages with pagination/filters:
                <div className="mt-2 flex flex-wrap gap-3 text-sm">
                    <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/alerts">
                        Alerts
                    </Link>
                    <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/responses">
                        Command Center
                    </Link>
                    <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/events">
                        Telemetry Search
                    </Link>
                    <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/audit">
                        Audit logs
                    </Link>
                </div>
            </div>
        </div>
    );
}

export function DashboardNotificationsPage() {
    return (
        <div className="rounded-xl border border-dashed border-gray-300 dark:border-gray-600 p-8 text-center text-gray-500 dark:text-gray-400">
            <p className="font-medium text-gray-700 dark:text-gray-300">Notifications</p>
            <p className="text-sm mt-2">In-app notification center — coming soon.</p>
            <p className="text-sm mt-4 text-left max-w-md mx-auto text-gray-600 dark:text-gray-300">
                For now, monitor <Link className="text-cyan-600 dark:text-cyan-400 font-medium" to="/alerts">Alerts</Link>
                {' '}and <Link className="text-cyan-600 dark:text-cyan-400 font-medium" to="/">Overview</Link> live streams.
            </p>
        </div>
    );
}

