import { useState } from 'react';
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
                <div className="space-y-2">
                    {Object.entries(s.by_os_type ?? {})
                        .sort((a, b) => b[1] - a[1])
                        .map(([k, v]) => {
                            const total = Math.max(1, s.total || 1);
                            const pct = Math.round((v / total) * 100);
                            return (
                                <div key={k} className="flex items-center gap-3">
                                    <div className="w-28 text-xs font-mono text-gray-700 dark:text-gray-300 uppercase">{k}</div>
                                    <div className="flex-1 h-2 rounded bg-gray-100 dark:bg-gray-900 overflow-hidden">
                                        <div className="h-2 bg-cyan-500" style={{ width: `${Math.min(100, pct)}%` }} />
                                    </div>
                                    <div className="w-24 text-right text-xs text-gray-500 dark:text-gray-400 font-mono">
                                        {v} ({pct}%)
                                    </div>
                                </div>
                            );
                        })}
                    {Object.keys(s.by_os_type ?? {}).length === 0 && (
                        <div className="text-xs text-gray-500 dark:text-gray-400">No OS data available.</div>
                    )}
                </div>
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

    const downloadCsv = (name: string, rows: Record<string, unknown>[]) => {
        const esc = (v: unknown) => {
            const s = v === null || v === undefined ? '' : String(v);
            const q = s.replaceAll('"', '""');
            return `"${q}"`;
        };
        const headers = Array.from(
            rows.reduce((set, r) => {
                Object.keys(r).forEach((k) => set.add(k));
                return set;
            }, new Set<string>())
        );
        const lines = [headers.join(','), ...rows.map((r) => headers.map((h) => esc(r[h])).join(','))];
        const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' });
        const a = document.createElement('a');
        a.href = URL.createObjectURL(blob);
        a.download = `${name}-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.csv`;
        a.click();
        URL.revokeObjectURL(a.href);
    };

    const fetchAllPaged = async <T,>(
        fetchPage: (offset: number, limit: number) => Promise<{ data: T[]; pagination?: { has_more?: boolean } }>,
        opts?: { limit?: number; max?: number }
    ): Promise<T[]> => {
        const limit = opts?.limit ?? 500;
        const max = opts?.max ?? 5000;
        const out: T[] = [];
        let offset = 0;
        for (let i = 0; i < 200; i++) {
            const r = await fetchPage(offset, limit);
            out.push(...(r.data ?? []));
            if (!r.pagination?.has_more) break;
            offset += limit;
            if (out.length >= max) break;
        }
        return out.slice(0, max);
    };

    const [reportType, setReportType] = useState<'commands' | 'alerts' | 'devices'>('commands');
    const [agentScope, setAgentScope] = useState<string>('all');
    const [from, setFrom] = useState<string>(() => new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString().slice(0, 16));
    const [to, setTo] = useState<string>(() => new Date().toISOString().slice(0, 16));
    const [loading, setLoading] = useState(false);
    const [err, setErr] = useState<string | null>(null);

    const agentsListQ = useQuery({
        queryKey: ['agents', 'list', 'reports'],
        queryFn: async () => {
            const out = await fetchAllPaged((offset, limit) => agentsApi.list({ offset, limit, sort_by: 'hostname', sort_order: 'asc' }), { max: 2000 });
            return out;
        },
        staleTime: 30_000,
        retry: 1,
    });

    const runReport = async (fmt: 'json' | 'csv') => {
        try {
            setLoading(true);
            setErr(null);
            const fromIso = new Date(from).toISOString();
            const toIso = new Date(to).toISOString();

            if (reportType === 'commands') {
                const rows = await fetchAllPaged(
                    (offset, limit) =>
                        commandsApi.list({
                            offset,
                            limit,
                            agent_id: agentScope === 'all' ? undefined : agentScope,
                            sort_by: 'issued_at',
                            sort_order: 'desc',
                        }),
                    { max: 5000 }
                );
                const filtered = rows.filter((c: any) => {
                    const t = new Date(c.issued_at).getTime();
                    return t >= new Date(fromIso).getTime() && t <= new Date(toIso).getTime();
                });
                if (fmt === 'json') downloadJson('report-commands', { from: fromIso, to: toIso, scope: agentScope, rows: filtered });
                else downloadCsv('report-commands', filtered.map((c: any) => ({
                    id: c.id,
                    agent_id: c.agent_id,
                    agent_hostname: c.agent_hostname,
                    command_type: c.command_type,
                    status: c.status,
                    issued_at: c.issued_at,
                    issued_by_user: c.issued_by_user,
                    exit_code: c.exit_code ?? '',
                    error_message: c.error_message ?? '',
                })));
                return;
            }

            if (reportType === 'alerts') {
                // Sigma alerts API supports date filtering server-side.
                const first = await alertsApi.list({
                    limit: 500,
                    offset: 0,
                    agent_id: agentScope === 'all' ? undefined : agentScope,
                    date_from: fromIso,
                    date_to: toIso,
                    sort: 'timestamp',
                    order: 'desc',
                });
                const rows = (first.alerts ?? []).slice(0, 2000);
                if (fmt === 'json') downloadJson('report-alerts', { from: fromIso, to: toIso, scope: agentScope, rows });
                else downloadCsv('report-alerts', rows.map((a: any) => ({
                    id: a.id,
                    timestamp: a.timestamp,
                    agent_id: a.agent_id,
                    rule_title: a.rule_title,
                    severity: a.severity,
                    status: a.status,
                    category: a.category,
                    risk_score: a.risk_score ?? '',
                })));
                return;
            }

            if (reportType === 'devices') {
                const allAgents = agentsListQ.data ?? [];
                const fromT = new Date(fromIso).getTime();
                const toT = new Date(toIso).getTime();
                const rows = allAgents.filter((a: any) => {
                    const t = new Date(a.created_at).getTime();
                    return t >= fromT && t <= toT;
                });
                if (fmt === 'json') downloadJson('report-devices', { from: fromIso, to: toIso, rows });
                else downloadCsv('report-devices', rows.map((a: any) => ({
                    id: a.id,
                    hostname: a.hostname,
                    os_type: a.os_type,
                    os_version: a.os_version,
                    agent_version: a.agent_version,
                    status: a.status,
                    created_at: a.created_at,
                    last_seen: a.last_seen,
                })));
            }
        } catch (e: any) {
            setErr(e?.message || 'Report failed');
        } finally {
            setLoading(false);
        }
    };

    return (
        <div className="space-y-4">
            <div>
                <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Reports</h2>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    Generate filtered reports (JSON/CSV) for commands, alerts, and devices.
                </p>
            </div>

            <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40 p-4 space-y-3">
                <div className="grid grid-cols-1 lg:grid-cols-4 gap-3">
                    <div>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Report type</label>
                        <select
                            value={reportType}
                            onChange={(e) => setReportType(e.target.value as any)}
                            className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-sm text-gray-900 dark:text-white"
                        >
                            <option value="commands">Executed commands</option>
                            <option value="alerts">Alerts</option>
                            <option value="devices">Devices (joined in range)</option>
                        </select>
                    </div>

                    <div>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Scope</label>
                        <select
                            value={agentScope}
                            onChange={(e) => setAgentScope(e.target.value)}
                            className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-sm text-gray-900 dark:text-white"
                        >
                            <option value="all">All endpoints</option>
                            {(agentsListQ.data ?? []).slice(0, 500).map((a: any) => (
                                <option key={a.id} value={a.id}>
                                    {a.hostname} — {a.id}
                                </option>
                            ))}
                        </select>
                        {agentsListQ.isLoading && (
                            <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">Loading endpoints…</div>
                        )}
                    </div>

                    <div>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">From</label>
                        <input
                            type="datetime-local"
                            value={from}
                            onChange={(e) => setFrom(e.target.value)}
                            className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-sm text-gray-900 dark:text-white"
                        />
                    </div>

                    <div>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">To</label>
                        <input
                            type="datetime-local"
                            value={to}
                            onChange={(e) => setTo(e.target.value)}
                            className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-sm text-gray-900 dark:text-white"
                        />
                    </div>
                </div>

                {err && (
                    <div className="rounded-lg border border-rose-200 dark:border-rose-900/50 bg-rose-50/80 dark:bg-rose-950/20 px-4 py-3 text-sm text-rose-900 dark:text-rose-200">
                        {err}
                    </div>
                )}

                <div className="flex flex-wrap gap-2">
                    <button
                        type="button"
                        className="px-3 py-2 rounded-lg text-sm font-semibold bg-cyan-600 hover:bg-cyan-700 text-white disabled:opacity-50"
                        disabled={loading}
                        onClick={() => runReport('json')}
                    >
                        Download JSON
                    </button>
                    <button
                        type="button"
                        className="px-3 py-2 rounded-lg text-sm font-semibold bg-gray-900 hover:bg-black text-white disabled:opacity-50 dark:bg-gray-700 dark:hover:bg-gray-600"
                        disabled={loading}
                        onClick={() => runReport('csv')}
                    >
                        Download CSV
                    </button>
                    {loading && <span className="text-sm text-gray-500 dark:text-gray-400 self-center">Generating…</span>}
                </div>
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

