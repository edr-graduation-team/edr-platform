import { useState, useMemo, useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link, Navigate } from 'react-router-dom';
import { parityApi } from '../../api/parity/parityApi';
import * as mocks from '../../api/parity/mocks';
import { GenericParityView } from '../../components/parity/GenericParityView';
import StatCard from '../../components/StatCard';
import InsightHero from '../../components/InsightHero';
import {
    Activity,
    AlertTriangle,
    BarChart3,
    ChevronDown,
    ChevronLeft,
    ChevronRight,
    ChevronUp,
    ClipboardList,
    Clock,
    Database,
    Fingerprint,
    KeyRound,
    Layers,
    ListChecks,
    Radio,
    Search,
    Server,
    Shield,
    Terminal,
    TrendingUp,
    Wifi,
} from 'lucide-react';
import {
    PieChart,
    Pie,
    Cell,
    Tooltip,
    ResponsiveContainer,
    BarChart,
    Bar,
    XAxis,
    YAxis,
    CartesianGrid,
} from 'recharts';
import {
    agentsApi,
    alertsApi,
    commandsApi,
    reliabilityApi,
    statsApi,
    type Agent,
} from '../../api/client';
import {
    formatDate,
    formatRelativeTime,
    getEffectiveStatus,
    isDecommissioned,
    STALE_THRESHOLD_MS,
} from '../../utils/agentDisplay';

const CHART_TOOLTIP: React.CSSProperties = {
    background: 'rgba(15,23,42,0.95)',
    border: '1px solid rgba(30,48,72,0.8)',
    borderRadius: '10px',
    color: 'white',
    fontSize: '12px',
};
const OS_COLORS: Record<string, string> = { windows: '#38bdf8', linux: '#a855f7', macos: '#10b981' };
const OS_FALLBACK = '#64748b';

const COMPLIANCE_AGENT_PAGE = 1000;
const COMPLIANCE_MAX_PAGES = 100;

/** Full sorted fleet for compliance (paginated GET /api/v1/agents until has_more is false). */
async function fetchEntireAgentFleetForCompliance(): Promise<{ agents: Agent[]; truncated: boolean }> {
    const all: Agent[] = [];
    let offset = 0;
    let truncated = false;
    for (let p = 0; p < COMPLIANCE_MAX_PAGES; p++) {
        const res = await agentsApi.list({
            limit: COMPLIANCE_AGENT_PAGE,
            offset,
            sort_by: 'hostname',
            sort_order: 'asc',
        });
        all.push(...res.data);
        if (!res.pagination?.has_more) {
            return { agents: all, truncated: false };
        }
        offset += COMPLIANCE_AGENT_PAGE;
        if (p === COMPLIANCE_MAX_PAGES - 1) {
            truncated = true;
        }
    }
    return { agents: all, truncated };
}

const CERT_EXPIRING_SOON_MS = 14 * 24 * 60 * 60 * 1000;

type ComplianceReasonCode =
    | 'offline'
    | 'health'
    | 'isolated'
    | 'no_cert'
    | 'cert_expired'
    | 'cert_expiring';

function evalEndpointCompliance(a: Agent): { compliant: boolean; reasons: string[]; codes: ComplianceReasonCode[] } {
    const reasons: string[] = [];
    const codes: ComplianceReasonCode[] = [];
    const eff = getEffectiveStatus(a);
    if (eff !== 'online' && eff !== 'degraded') {
        reasons.push('Agent not online (effective status)');
        codes.push('offline');
    }
    if ((a.health_score ?? 0) < 80) {
        reasons.push('Health score below 80%');
        codes.push('health');
    }
    if (a.is_isolated) {
        reasons.push('Host is network-isolated');
        codes.push('isolated');
    }
    if (!a.current_cert_id) {
        reasons.push('Missing active mTLS certificate');
        codes.push('no_cert');
    }
    const certExpiry = a.cert_expires_at ? new Date(a.cert_expires_at) : null;
    if (certExpiry && !Number.isNaN(certExpiry.getTime())) {
        const t = certExpiry.getTime();
        if (t < Date.now()) {
            reasons.push('mTLS certificate expired');
            codes.push('cert_expired');
        } else if (t < Date.now() + CERT_EXPIRING_SOON_MS) {
            reasons.push('mTLS certificate expires within 14 days');
            codes.push('cert_expiring');
        }
    }
    return { compliant: reasons.length === 0, reasons, codes };
}

/** Live data from connection-manager + sigma (self-hosted). */
export function DashboardServicePage() {
    useEffect(() => { document.title = 'Service Summary \u2014 EDR Platform'; }, []);

    const cmdQ = useQuery({ queryKey: ['commands-stats'], queryFn: () => commandsApi.stats(), staleTime: 30_000 });
    const alertQ = useQuery({ queryKey: ['sigma-stats-alerts'], queryFn: () => statsApi.alerts(), staleTime: 30_000 });
    const relQ = useQuery({
        queryKey: ['reliability-health'],
        queryFn: () => reliabilityApi.health(),
        staleTime: 60_000,
        retry: 1,
    });

    if (cmdQ.isLoading || alertQ.isLoading) {
        return (
            <div className="space-y-4 animate-pulse">
                <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                    {[...Array(4)].map((_, i) => <div key={i} className="h-24 rounded-xl bg-slate-200 dark:bg-slate-800" />)}
                </div>
                <div className="grid grid-cols-3 gap-4">
                    {[...Array(3)].map((_, i) => <div key={i} className="h-24 rounded-xl bg-slate-200 dark:bg-slate-800" />)}
                </div>
            </div>
        );
    }

    const cmd = cmdQ.data;
    const al = alertQ.data;

    const relReason = relQ.data?.fallback_store?.reason;
    const relEnabled = relQ.data?.fallback_store?.enabled === true;

    return (
        <div className="space-y-5 animate-slide-up-fade">
            <div>
                <h2 className="text-lg font-bold text-slate-900 dark:text-white">Service Summary</h2>
                <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
                    Live metrics from command operations, sigma alert engine, and reliability health.
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
                <p className="text-xs text-slate-500 dark:text-slate-400">{relReason}</p>
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

const ENDPOINT_DASHBOARD_AGENT_LIMIT = 500;

/** Executive endpoint fleet dashboard — registry stats + sampled live connectivity + risk excerpt. */
export function DashboardEndpointPage() {
    useEffect(() => { document.title = 'Endpoint Summary — EDR Platform'; }, []);

    const statsQ = useQuery({ queryKey: ['agents-stats'], queryFn: () => agentsApi.stats(), staleTime: 30_000, refetchInterval: 30_000 });
    const riskQ = useQuery({ queryKey: ['endpoint-risk'], queryFn: () => alertsApi.endpointRisk(), staleTime: 60_000, refetchInterval: 60_000, retry: 1 });
    const agentsQ = useQuery({
        queryKey: ['agents', 'endpoint-dashboard', ENDPOINT_DASHBOARD_AGENT_LIMIT],
        queryFn: () => agentsApi.list({ limit: ENDPOINT_DASHBOARD_AGENT_LIMIT, offset: 0, sort_by: 'hostname', sort_order: 'asc' }),
        staleTime: 30_000,
        refetchInterval: 45_000,
    });

    const agentMap = useMemo<Record<string, string>>(() => {
        const m: Record<string, string> = {};
        (agentsQ.data?.data ?? []).forEach((a) => {
            m[a.id] = a.hostname;
        });
        return m;
    }, [agentsQ.data]);

    const riskRows = riskQ.data?.data ?? [];
    const sortedRisk = useMemo(
        () =>
            [...riskRows]
                .sort((a, b) => b.peak_risk_score - a.peak_risk_score || b.open_count - a.open_count || b.total_alerts - a.total_alerts)
                .slice(0, 10),
        [riskRows]
    );

    const sampleAgents = agentsQ.data?.data ?? [];
    const liveSample = useMemo(() => {
        let effOnline = 0;
        let effDegraded = 0;
        let effOffline = 0;
        let stale = 0;
        let isolated = 0;
        let qSum = 0;
        let dropSum = 0;
        for (const a of sampleAgents) {
            const e = getEffectiveStatus(a);
            if (e === 'online') {
                effOnline += 1;
            } else if (e === 'degraded') {
                effDegraded += 1;
            } else {
                effOffline += 1;
            }
            if ((a.status === 'online' || a.status === 'degraded') && e === 'offline') {
                stale += 1;
            }
            if (a.is_isolated) {
                isolated += 1;
            }
            qSum += a.queue_depth ?? 0;
            dropSum += a.events_dropped ?? 0;
        }
        const n = sampleAgents.length || 1;
        return { effOnline, effDegraded, effOffline, stale, isolated, qSum, dropSum, n };
    }, [sampleAgents]);

    const dominantVersion = useMemo(() => {
        const s = statsQ.data;
        if (!s?.by_version) return '';
        const ent = Object.entries(s.by_version).sort((a, b) => b[1] - a[1]);
        return ent[0]?.[0] ?? '';
    }, [statsQ.data]);

    const versionDriftHosts = useMemo(() => {
        if (!dominantVersion) return [];
        return sampleAgents.filter((a) => a.agent_version && a.agent_version !== dominantVersion).slice(0, 8);
    }, [sampleAgents, dominantVersion]);

    if (statsQ.isLoading) {
        return (
            <div className="space-y-4 animate-pulse">
                <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                    {[...Array(4)].map((_, i) => (
                        <div key={i} className="h-28 rounded-xl bg-slate-200 dark:bg-slate-800" />
                    ))}
                </div>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div className="h-56 rounded-xl bg-slate-200 dark:bg-slate-800" />
                    <div className="h-56 rounded-xl bg-slate-200 dark:bg-slate-800" />
                </div>
            </div>
        );
    }

    if (statsQ.isError || !statsQ.data) {
        return (
            <div className="rounded-xl border border-rose-200 dark:border-rose-900/50 bg-rose-50/80 dark:bg-rose-950/20 p-6 text-sm text-rose-900 dark:text-rose-200">
                Could not load agent statistics. Open{' '}
                <Link className="font-semibold underline" to="/management/devices">
                    Device Management
                </Link>{' '}
                after verifying the connection-manager API.
            </div>
        );
    }

    const s = statsQ.data;
    const withOpenAlerts = riskRows.filter((r) => r.open_count > 0).length;

    const osData = Object.entries(s.by_os_type ?? {})
        .filter(([, v]) => v > 0)
        .map(([k, v]) => ({ name: k.charAt(0).toUpperCase() + k.slice(1), value: v, key: k }));
    const osTotal = osData.reduce((a, d) => a + d.value, 0) || 1;

    const versionData = Object.entries(s.by_version ?? {})
        .sort((a, b) => b[1] - a[1])
        .slice(0, 6)
        .map(([k, v]) => ({ version: k.length > 12 ? k.slice(0, 12) + '…' : k, count: v }));

    const connectivityCompare = [
        { name: 'Registry online', value: s.online, fill: '#22d3ee' },
        { name: 'Live online (sample)', value: liveSample.effOnline, fill: '#34d399' },
    ];

    return (
        <div className="space-y-6 animate-slide-up-fade w-full min-w-0">
            <InsightHero
                variant="dark"
                accent="emerald"
                icon={BarChart3}
                eyebrow="Executive fleet"
                title="Endpoint Summary"
                lead={
                    <>
                        One screen for <strong className="text-white">leadership-grade posture</strong>: registry totals from{' '}
                        <code className="text-[11px] text-emerald-200/95 bg-white/10 px-1 rounded">/agents/stats</code>, sampled{' '}
                        <strong className="text-white">live connectivity</strong> using the same last_seen rule as Devices, and a{' '}
                        <strong className="text-white">curated risk excerpt</strong> from endpoint-risk — without replacing the deep grids elsewhere.
                    </>
                }
            />

            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <Layers className="w-4 h-4 text-cyan-500" />
                        vs Service Summary
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/dashboards/service">
                            Service Summary
                        </Link>{' '}
                        tracks commands + Sigma totals + reliability. This page stays on <strong>endpoints only</strong>.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <TrendingUp className="w-4 h-4 text-amber-500" />
                        vs Endpoint Risk
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/endpoint-risk">
                            Endpoint Risk
                        </Link>{' '}
                        is the full ranked board. Here you only see a <strong>top-10 executive excerpt</strong> sorted by peak score.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <Shield className="w-4 h-4 text-violet-500" />
                        vs Compliance
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/dashboards/endpoint-compliance">
                            Endpoint Compliance
                        </Link>{' '}
                        evaluates rule-based pass/fail per host. This dashboard highlights drift and risk signals instead.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <Wifi className="w-4 h-4 text-sky-500" />
                        vs Fleet connectivity
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/management/network">
                            Fleet connectivity
                        </Link>{' '}
                        is the SOC table for queues, drops, and IP inventory. Summary here stays compact and chart-led.
                    </p>
                </div>
            </div>

            {riskQ.isError && (
                <div className="rounded-lg border border-amber-200 dark:border-amber-800 bg-amber-50 dark:bg-amber-950/30 px-4 py-2.5 text-xs text-amber-900 dark:text-amber-200">
                    Endpoint risk excerpt unavailable — confirm <code className="text-[10px]">alerts:read</code> for <code className="text-[10px]">/alerts/endpoint-risk</code>.
                </div>
            )}

            <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                <StatCard title="Total Endpoints" value={String(s.total)} icon={Server} subtext={`${s.pending} pending · ${s.suspended} suspended`} />
                <StatCard title="Registry online" value={String(s.online)} icon={Activity} color="emerald" subtext={`Avg health ${Math.round(s.avg_health)}%`} />
                <StatCard title="Offline / Degraded" value={`${s.offline} / ${s.degraded}`} icon={AlertTriangle} color="amber" />
                <StatCard title="Hosts w/ open alerts" value={String(withOpenAlerts)} icon={Shield} color="red" subtext={`${riskRows.length} in risk index`} />
            </div>

            <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                <StatCard
                    title="Live online (sample)"
                    value={agentsQ.isLoading ? '…' : String(liveSample.effOnline)}
                    icon={Fingerprint}
                    color="emerald"
                    subtext={`First ${liveSample.n} hosts by hostname`}
                />
                <StatCard
                    title="Stale heartbeat"
                    value={agentsQ.isLoading ? '…' : String(liveSample.stale)}
                    icon={Radio}
                    color="amber"
                    subtext={`>${Math.round(STALE_THRESHOLD_MS / 1000)}s vs registry online/degraded`}
                />
                <StatCard title="Isolated (sample)" value={agentsQ.isLoading ? '…' : String(liveSample.isolated)} icon={AlertTriangle} color="red" />
                <StatCard
                    title="Telemetry pressure"
                    value={agentsQ.isLoading ? '…' : `${liveSample.dropSum} drops`}
                    icon={Activity}
                    color="cyan"
                    subtext={`Σ queue ${liveSample.qSum} · sample only`}
                />
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm p-5 shadow-sm lg:col-span-1">
                    <h3 className="text-xs font-bold uppercase tracking-widest text-slate-400 mb-2 flex items-center gap-2">
                        <Radio className="w-3.5 h-3.5 text-cyan-500" />
                        Registry vs live online
                    </h3>
                    <p className="text-[11px] text-slate-500 dark:text-slate-400 mb-3">
                        Compares DB registry online count to effective-online count on the same {liveSample.n}-host slice (sorted A→Z).
                    </p>
                    {agentsQ.isLoading ? (
                        <div className="h-36 rounded-lg bg-slate-100 dark:bg-slate-800 animate-pulse" />
                    ) : (
                        <ResponsiveContainer width="100%" height={160}>
                            <BarChart data={connectivityCompare} margin={{ top: 8, right: 8, left: -16, bottom: 0 }}>
                                <CartesianGrid strokeDasharray="3 3" stroke="rgba(100,116,139,0.15)" vertical={false} />
                                <XAxis dataKey="name" tick={{ fontSize: 9, fill: '#94a3b8' }} interval={0} angle={-12} textAnchor="end" height={52} />
                                <YAxis allowDecimals={false} tick={{ fontSize: 10, fill: '#94a3b8' }} />
                                <Tooltip contentStyle={CHART_TOOLTIP} />
                                <Bar dataKey="value" radius={[6, 6, 0, 0]} maxBarSize={48}>
                                    {connectivityCompare.map((e, i) => (
                                        <Cell key={i} fill={e.fill} />
                                    ))}
                                </Bar>
                            </BarChart>
                        </ResponsiveContainer>
                    )}
                </div>

                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm p-5 shadow-sm lg:col-span-1">
                    <h3 className="text-xs font-bold uppercase tracking-widest text-slate-400 mb-3 flex items-center gap-2">
                        <Server className="w-3.5 h-3.5 text-cyan-500" /> OS Distribution
                    </h3>
                    {osData.length === 0 ? (
                        <div className="flex items-center justify-center h-36 text-slate-500 text-sm">No OS data</div>
                    ) : (
                        <div className="flex flex-col items-center gap-2">
                            <ResponsiveContainer width="100%" height={140}>
                                <PieChart>
                                    <Pie data={osData} dataKey="value" nameKey="name" cx="50%" cy="50%" innerRadius={38} outerRadius={60} paddingAngle={3} strokeWidth={0}>
                                        {osData.map((e) => (
                                            <Cell key={e.key} fill={OS_COLORS[e.key] || OS_FALLBACK} />
                                        ))}
                                    </Pie>
                                    <Tooltip
                                        contentStyle={CHART_TOOLTIP}
                                        formatter={(v: number | undefined) => [`${v ?? 0} agents`, '']}
                                    />
                                </PieChart>
                            </ResponsiveContainer>
                            <div className="flex flex-wrap justify-center gap-3">
                                {osData.map((d) => (
                                    <span key={d.key} className="flex items-center gap-1.5 text-[11px] font-medium text-slate-500 dark:text-slate-400">
                                        <span className="w-2.5 h-2.5 rounded-full shrink-0" style={{ background: OS_COLORS[d.key] || OS_FALLBACK }} />
                                        {d.name} ({Math.round((d.value / osTotal) * 100)}%)
                                    </span>
                                ))}
                            </div>
                        </div>
                    )}
                </div>

                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm p-5 shadow-sm lg:col-span-1">
                    <h3 className="text-xs font-bold uppercase tracking-widest text-slate-400 mb-3 flex items-center gap-2">
                        <Activity className="w-3.5 h-3.5 text-emerald-500" /> Agent Versions
                    </h3>
                    {versionData.length === 0 ? (
                        <div className="flex items-center justify-center h-36 text-slate-500 text-sm">No version data</div>
                    ) : (
                        <ResponsiveContainer width="100%" height={150}>
                            <BarChart data={versionData} layout="vertical" margin={{ left: 0, right: 16, top: 4, bottom: 4 }}>
                                <CartesianGrid strokeDasharray="3 3" stroke="rgba(100,116,139,0.15)" horizontal={false} />
                                <XAxis type="number" tick={{ fontSize: 10, fill: '#94a3b8' }} axisLine={false} tickLine={false} />
                                <YAxis type="category" dataKey="version" tick={{ fontSize: 10, fill: '#94a3b8' }} width={90} axisLine={false} tickLine={false} />
                                <Tooltip contentStyle={CHART_TOOLTIP} />
                                <Bar dataKey="count" fill="#22d3ee" radius={[0, 4, 4, 0]} maxBarSize={18} name="Agents" />
                            </BarChart>
                        </ResponsiveContainer>
                    )}
                </div>
            </div>

            {dominantVersion && versionDriftHosts.length > 0 && (
                <div className="rounded-xl border border-amber-200/80 dark:border-amber-900/40 bg-amber-50/60 dark:bg-amber-950/20 p-4 shadow-sm">
                    <h3 className="text-sm font-bold text-amber-950 dark:text-amber-100">Version drift (sample)</h3>
                    <p className="text-xs text-amber-900/80 dark:text-amber-200/90 mt-1">
                        Dominant fleet version from registry: <code className="text-[11px]">{dominantVersion}</code>. The following hosts in the first {liveSample.n} rows differ — plan upgrades via{' '}
                        <Link className="font-semibold underline" to="/management/agent-deploy">
                            Agent Deployment
                        </Link>
                        .
                    </p>
                    <ul className="mt-2 flex flex-wrap gap-2 text-xs">
                        {versionDriftHosts.map((a) => (
                            <li key={a.id}>
                                <Link className="text-cyan-700 dark:text-cyan-300 hover:underline font-medium" to={`/management/devices/${encodeURIComponent(a.id)}`}>
                                    {a.hostname}
                                </Link>
                                <span className="text-slate-600 dark:text-slate-400"> ({a.agent_version})</span>
                            </li>
                        ))}
                    </ul>
                </div>
            )}

            {sortedRisk.length > 0 && (
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm p-5 shadow-sm">
                    <h3 className="text-xs font-bold uppercase tracking-widest text-slate-400 mb-1 flex items-center gap-2">
                        <AlertTriangle className="w-3.5 h-3.5 text-rose-500" /> Risk excerpt (peak score)
                    </h3>
                    <p className="text-[11px] text-slate-500 dark:text-slate-400 mb-3">Top 10 from live endpoint-risk API — not the full triage grid.</p>
                    <div className="overflow-x-auto">
                        <table className="min-w-full text-sm">
                            <thead>
                                <tr className="text-xs uppercase text-slate-400 border-b border-slate-200 dark:border-slate-700">
                                    <th className="text-left py-2 pr-3 font-semibold">Endpoint</th>
                                    <th className="text-right py-2 px-3 font-semibold">Peak</th>
                                    <th className="text-right py-2 px-3 font-semibold">Open</th>
                                    <th className="text-right py-2 px-3 font-semibold">Avg</th>
                                    <th className="text-right py-2 pl-3 font-semibold">Critical</th>
                                </tr>
                            </thead>
                            <tbody>
                                {sortedRisk.map((r) => (
                                    <tr key={r.agent_id} className="border-b border-slate-100 dark:border-slate-800 hover:bg-slate-50 dark:hover:bg-slate-800/60 transition-colors">
                                        <td className="py-2 pr-3">
                                            <Link
                                                className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline"
                                                to={`/management/devices/${encodeURIComponent(r.agent_id)}`}
                                            >
                                                {agentMap[r.agent_id] || r.agent_id.slice(0, 12) + '…'}
                                            </Link>
                                        </td>
                                        <td className="text-right py-2 px-3 font-mono text-slate-600 dark:text-slate-300">{Math.round(r.peak_risk_score)}</td>
                                        <td className="text-right py-2 px-3 font-mono text-slate-600 dark:text-slate-300">{r.open_count}</td>
                                        <td className="text-right py-2 px-3 font-mono text-slate-600 dark:text-slate-300">{Math.round(r.avg_risk_score)}</td>
                                        <td className="text-right py-2 pl-3">
                                            {r.critical_count > 0 ? (
                                                <span className="text-xs font-bold text-rose-500 bg-rose-500/10 px-1.5 py-0.5 rounded">{r.critical_count}</span>
                                            ) : (
                                                <span className="text-xs text-slate-400">0</span>
                                            )}
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                    <div className="mt-3 flex flex-wrap justify-end gap-3 text-xs">
                        <Link to="/endpoint-risk" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                            Full Endpoint Risk board →
                        </Link>
                        <Link to="/management/devices" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                            Devices →
                        </Link>
                    </div>
                </div>
            )}

            <div className="flex flex-wrap gap-3 text-sm">
                <Link to="/dashboards/service" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                    Service Summary →
                </Link>
                <Link to="/stats" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                    Statistics →
                </Link>
                <Link to="/management/network" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                    Fleet connectivity →
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
    useEffect(() => { document.title = 'Endpoint Compliance — EDR Platform'; }, []);

    const statsQ = useQuery({
        queryKey: ['agents', 'stats', 'compliance-context'],
        queryFn: () => agentsApi.stats(),
        staleTime: 60_000,
        refetchInterval: 120_000,
        retry: 1,
    });

    const fleetQ = useQuery({
        queryKey: ['agents', 'compliance', 'fleet'],
        queryFn: fetchEntireAgentFleetForCompliance,
        staleTime: 60_000,
        refetchInterval: 120_000,
        retry: 1,
    });

    const [searchTerm, setSearchTerm] = useState('');
    const [statusFilter, setStatusFilter] = useState<'all' | 'compliant' | 'non-compliant'>('all');
    const [sortCol, setSortCol] = useState<'hostname' | 'health' | 'status' | 'cert'>('hostname');
    const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc');
    const [page, setPage] = useState(0);
    const PAGE_SIZE = 20;

    const allRows = fleetQ.data?.agents ?? [];
    const truncated = fleetQ.data?.truncated ?? false;
    const decommissioned = useMemo(() => allRows.filter(isDecommissioned), [allRows]);
    const activeRows = useMemo(() => allRows.filter((a) => !isDecommissioned(a)), [allRows]);

    const evaluatedActive = useMemo(
        () =>
            activeRows.map((a) => {
                const eff = getEffectiveStatus(a);
                const { compliant, reasons, codes } = evalEndpointCompliance(a);
                return { agent: a, eff, compliant, reasons, codes };
            }),
        [activeRows]
    );

    const compliantCount = evaluatedActive.filter((e) => e.compliant).length;
    const nonCompliantCount = evaluatedActive.length - compliantCount;
    const isolatedActive = activeRows.filter((a) => a.is_isolated).length;

    const violationBars = useMemo(() => {
        const labels: { code: ComplianceReasonCode; label: string; fill: string }[] = [
            { code: 'offline', label: 'Not online (effective)', fill: '#64748b' },
            { code: 'health', label: 'Health < 80%', fill: '#f59e0b' },
            { code: 'isolated', label: 'Isolated', fill: '#ef4444' },
            { code: 'no_cert', label: 'No mTLS cert', fill: '#a855f7' },
            { code: 'cert_expired', label: 'Cert expired', fill: '#dc2626' },
            { code: 'cert_expiring', label: 'Cert ≤14d', fill: '#f97316' },
        ];
        return labels
            .map((L) => ({
                ...L,
                count: evaluatedActive.filter((e) => !e.compliant && e.codes.includes(L.code)).length,
            }))
            .filter((d) => d.count > 0);
    }, [evaluatedActive]);

    const filtered = useMemo(() => {
        const certTs = (a: Agent) => {
            if (!a.cert_expires_at) return 0;
            const t = new Date(a.cert_expires_at).getTime();
            return Number.isNaN(t) ? 0 : t;
        };
        return evaluatedActive
            .filter((e) => {
                if (statusFilter === 'compliant' && !e.compliant) return false;
                if (statusFilter === 'non-compliant' && e.compliant) return false;
                if (searchTerm && !e.agent.hostname.toLowerCase().includes(searchTerm.toLowerCase())) return false;
                return true;
            })
            .sort((a, b) => {
                let cmp = 0;
                if (sortCol === 'hostname') cmp = a.agent.hostname.localeCompare(b.agent.hostname);
                else if (sortCol === 'health') cmp = (a.agent.health_score ?? 0) - (b.agent.health_score ?? 0);
                else if (sortCol === 'status') cmp = (a.compliant ? 0 : 1) - (b.compliant ? 0 : 1);
                else if (sortCol === 'cert') cmp = certTs(a.agent) - certTs(b.agent);
                return sortDir === 'asc' ? cmp : -cmp;
            });
    }, [evaluatedActive, searchTerm, sortCol, sortDir, statusFilter]);

    const pieData = [
        { name: 'Compliant', value: compliantCount, color: '#10b981' },
        { name: 'Non-compliant', value: nonCompliantCount, color: '#f59e0b' },
    ].filter((d) => d.value > 0);

    const denom = evaluatedActive.length || 1;
    const pctCompliant = Math.round((compliantCount / denom) * 100);

    if (fleetQ.isLoading) {
        return (
            <div className="space-y-4 animate-pulse">
                <div className="h-24 rounded-2xl bg-slate-200 dark:bg-slate-800" />
                <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                    {[...Array(4)].map((_, i) => (
                        <div key={i} className="h-28 rounded-xl bg-slate-200 dark:bg-slate-800" />
                    ))}
                </div>
                <div className="h-64 rounded-xl bg-slate-200 dark:bg-slate-800" />
            </div>
        );
    }

    if (fleetQ.isError || !fleetQ.data) {
        return (
            <div className="rounded-xl border border-rose-200 dark:border-rose-900/50 bg-rose-50/80 dark:bg-rose-950/20 p-6 text-sm text-rose-900 dark:text-rose-200">
                Could not load agents for compliance. Check connection-manager and <code className="text-xs">endpoints:read</code>.
            </div>
        );
    }

    const registryTotal = statsQ.data?.total ?? allRows.length;

    const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
    const safePage = Math.min(page, totalPages - 1);
    const pageRows = filtered.slice(safePage * PAGE_SIZE, (safePage + 1) * PAGE_SIZE);

    const toggleSort = (col: typeof sortCol) => {
        if (sortCol === col) setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'));
        else {
            setSortCol(col);
            setSortDir('asc');
        }
        setPage(0);
    };

    const SortIcon = ({ col }: { col: typeof sortCol }) => {
        if (sortCol !== col) return null;
        return sortDir === 'asc' ? <ChevronUp className="w-3 h-3 inline" /> : <ChevronDown className="w-3 h-3 inline" />;
    };

    const statsMismatch =
        statsQ.data && !truncated && statsQ.data.total !== allRows.length
            ? `Registry reports ${statsQ.data.total} agents; list returned ${allRows.length}.`
            : null;

    return (
        <div className="space-y-6 animate-slide-up-fade w-full min-w-0">
            <InsightHero
                variant="dark"
                accent="indigo"
                icon={ListChecks}
                eyebrow="Policy posture"
                title="Endpoint Compliance"
                lead={
                    <>
                        Pass/fail per <strong className="text-white">active enrolled host</strong> using the same{' '}
                        <code className="text-[11px] text-indigo-200/95 bg-white/10 px-1 rounded">last_seen</code> rule as Device Management, health from the registry, isolation flag,
                        and mTLS material from{' '}
                        <code className="text-[11px] text-indigo-200/95 bg-white/10 px-1 rounded">GET /api/v1/agents</code> (full fleet loaded in pages of {COMPLIANCE_AGENT_PAGE}).
                        Decommissioned agents are listed for audit but <strong className="text-white">excluded from the compliance ratio</strong>.
                    </>
                }
            />

            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <BarChart3 className="w-4 h-4 text-cyan-500" />
                        vs Endpoint Summary
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/dashboards/endpoint">
                            Endpoint Summary
                        </Link>{' '}
                        is charts + risk excerpt. Here the grid is <strong>binary compliance</strong> with explicit failure reasons.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <Server className="w-4 h-4 text-slate-500" />
                        vs Device Management
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/management/devices">
                            Devices
                        </Link>{' '}
                        is the operational console (filters, bulk actions). This page answers <strong>who fails which control</strong>.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <TrendingUp className="w-4 h-4 text-amber-500" />
                        vs Endpoint Risk
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/endpoint-risk">
                            Endpoint Risk
                        </Link>{' '}
                        ranks Sigma-driven exposure. Compliance is <strong>control-state</strong> (connectivity, health, cert, isolation) — orthogonal to alert volume.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <Wifi className="w-4 h-4 text-sky-500" />
                        vs Fleet connectivity
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/management/network">
                            Fleet connectivity
                        </Link>{' '}
                        focuses on queues, drops, and IP inventory. Here we only use connectivity as <strong>one compliance gate</strong>.
                    </p>
                </div>
            </div>

            {truncated && (
                <div className="rounded-lg border border-amber-200 dark:border-amber-800 bg-amber-50 dark:bg-amber-950/30 px-4 py-2.5 text-xs text-amber-900 dark:text-amber-200">
                    Fleet list stopped at {COMPLIANCE_AGENT_PAGE * COMPLIANCE_MAX_PAGES} agents (safety cap). Increase cap or filter server-side if you exceed this bound.
                </div>
            )}
            {statsQ.isError && (
                <div className="rounded-lg border border-amber-200 dark:border-amber-800 bg-amber-50 dark:bg-amber-950/30 px-4 py-2.5 text-xs text-amber-900 dark:text-amber-200">
                    <code className="text-[10px]">GET /api/v1/agents/stats</code> failed — registry total uses loaded list size ({allRows.length}) until stats succeed.
                </div>
            )}
            {statsMismatch && (
                <div className="rounded-lg border border-amber-200 dark:border-amber-800 bg-amber-50 dark:bg-amber-950/30 px-4 py-2.5 text-xs text-amber-900 dark:text-amber-200">
                    {statsMismatch}
                </div>
            )}

            <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 p-4 shadow-sm">
                <h3 className="text-xs font-bold uppercase tracking-widest text-slate-400 mb-3 flex items-center gap-2">
                    <ClipboardList className="w-3.5 h-3.5 text-indigo-500" />
                    In-scope controls (all must pass)
                </h3>
                <ul className="grid sm:grid-cols-2 gap-2 text-sm text-slate-600 dark:text-slate-300">
                    <li className="flex gap-2">
                        <span className="text-emerald-500 font-bold">✓</span>
                        Effective agent status is <code className="text-[11px]">online</code> or <code className="text-[11px]">degraded</code> with{' '}
                        <code className="text-[11px]">last_seen</code> within {Math.round(STALE_THRESHOLD_MS / 1000)}s (same as Devices).
                    </li>
                    <li className="flex gap-2">
                        <span className="text-emerald-500 font-bold">✓</span>
                        Registry <code className="text-[11px]">health_score</code> ≥ 80%.
                    </li>
                    <li className="flex gap-2">
                        <span className="text-emerald-500 font-bold">✓</span>
                        Not network-isolated (<code className="text-[11px]">is_isolated</code> = false) — isolation is treated as out-of-standard posture for this dashboard.
                    </li>
                    <li className="flex gap-2">
                        <span className="text-emerald-500 font-bold">✓</span>
                        Active mTLS enrollment: <code className="text-[11px]">current_cert_id</code> present, certificate not expired, and not expiring within 14 days.
                    </li>
                </ul>
            </div>

            <div className="grid grid-cols-2 lg:grid-cols-5 gap-4">
                <StatCard title="Registry total" value={String(registryTotal)} icon={Database} subtext="From /agents/stats" />
                <StatCard title="Loaded for review" value={String(allRows.length)} icon={Server} subtext={truncated ? 'Truncated' : 'Full list'} />
                <StatCard title="Active (in ratio)" value={String(evaluatedActive.length)} icon={Fingerprint} subtext={`${decommissioned.length} decommissioned excluded`} />
                <StatCard title="Compliant" value={String(compliantCount)} icon={Shield} color="emerald" subtext={`${pctCompliant}% of active`} />
                <StatCard title="Non-compliant" value={String(nonCompliantCount)} icon={AlertTriangle} color="amber" subtext={`Isolated (active): ${isolatedActive}`} />
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm p-4 shadow-sm flex flex-col items-center justify-center">
                    <h3 className="text-xs font-bold uppercase tracking-widest text-slate-400 mb-2 self-start">Active compliance ratio</h3>
                    {pieData.length === 0 ? (
                        <span className="text-sm text-slate-500 py-6">No active hosts</span>
                    ) : (
                        <ResponsiveContainer width="100%" height={130}>
                            <PieChart>
                                <Pie
                                    data={pieData}
                                    dataKey="value"
                                    nameKey="name"
                                    cx="50%"
                                    cy="50%"
                                    innerRadius={36}
                                    outerRadius={56}
                                    paddingAngle={3}
                                    strokeWidth={0}
                                >
                                    {pieData.map((d, i) => (
                                        <Cell key={i} fill={d.color} />
                                    ))}
                                </Pie>
                                <Tooltip
                                    contentStyle={CHART_TOOLTIP}
                                    formatter={(v: number | undefined, name: string | undefined) => [`${v ?? 0} hosts`, name ?? '']}
                                />
                            </PieChart>
                        </ResponsiveContainer>
                    )}
                    <div className="flex flex-wrap gap-3 text-[11px] font-medium text-slate-500 dark:text-slate-400">
                        <span className="flex items-center gap-1">
                            <span className="w-2 h-2 rounded-full bg-emerald-500" />
                            Compliant ({pctCompliant}%)
                        </span>
                        <span className="flex items-center gap-1">
                            <span className="w-2 h-2 rounded-full bg-amber-500" />
                            Non-compliant
                        </span>
                    </div>
                </div>
                <div className="lg:col-span-2 rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm p-4 shadow-sm">
                    <h3 className="text-xs font-bold uppercase tracking-widest text-slate-400 mb-2 flex items-center gap-2">
                        <Activity className="w-3.5 h-3.5 text-cyan-500" />
                        Violation mix (non-compliant hosts; one row may count in multiple bars)
                    </h3>
                    {violationBars.length === 0 ? (
                        <div className="flex items-center justify-center h-32 text-sm text-slate-500">No open violations — fleet is compliant on loaded data.</div>
                    ) : (
                        <ResponsiveContainer width="100%" height={140}>
                            <BarChart data={violationBars} layout="vertical" margin={{ left: 4, right: 16, top: 4, bottom: 4 }}>
                                <CartesianGrid strokeDasharray="3 3" stroke="rgba(100,116,139,0.15)" horizontal={false} />
                                <XAxis type="number" allowDecimals={false} tick={{ fontSize: 10, fill: '#94a3b8' }} />
                                <YAxis type="category" dataKey="label" width={130} tick={{ fontSize: 10, fill: '#94a3b8' }} />
                                <Tooltip
                                    contentStyle={CHART_TOOLTIP}
                                    formatter={(v: number | undefined) => [`${v ?? 0} hosts`, 'Count']}
                                />
                                <Bar dataKey="count" radius={[0, 4, 4, 0]} maxBarSize={22} name="Hosts">
                                    {violationBars.map((e, i) => (
                                        <Cell key={i} fill={e.fill} />
                                    ))}
                                </Bar>
                            </BarChart>
                        </ResponsiveContainer>
                    )}
                </div>
            </div>

            <div className="flex flex-wrap items-center gap-3">
                <div className="relative flex-1 min-w-[200px] max-w-sm">
                    <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
                    <input
                        type="text"
                        placeholder="Search hostname…"
                        value={searchTerm}
                        onChange={(e) => {
                            setSearchTerm(e.target.value);
                            setPage(0);
                        }}
                        className="w-full pl-9 pr-3 py-2 rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-sm text-slate-900 dark:text-white placeholder:text-slate-400"
                    />
                </div>
                <select
                    value={statusFilter}
                    onChange={(e) => {
                        setStatusFilter(e.target.value as 'all' | 'compliant' | 'non-compliant');
                        setPage(0);
                    }}
                    className="px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-sm text-slate-900 dark:text-white"
                >
                    <option value="all">All active</option>
                    <option value="compliant">Compliant</option>
                    <option value="non-compliant">Non-compliant</option>
                </select>
                <span className="text-xs text-slate-500 ml-auto">
                    {filtered.length} result{filtered.length !== 1 ? 's' : ''} (active hosts only)
                </span>
            </div>

            <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm shadow-sm">
                <table className="min-w-full text-left text-sm">
                    <thead className="bg-slate-50 dark:bg-slate-900/50 text-slate-500 dark:text-slate-400 text-xs uppercase">
                        <tr>
                            <th className="px-3 py-2.5 cursor-pointer select-none" onClick={() => toggleSort('hostname')}>
                                Host <SortIcon col="hostname" />
                            </th>
                            <th className="px-3 py-2.5 cursor-pointer select-none" onClick={() => toggleSort('status')}>
                                Compliance <SortIcon col="status" />
                            </th>
                            <th className="px-3 py-2.5">Effective status</th>
                            <th className="px-3 py-2.5 cursor-pointer select-none" onClick={() => toggleSort('health')}>
                                Health <SortIcon col="health" />
                            </th>
                            <th className="px-3 py-2.5 cursor-pointer select-none" onClick={() => toggleSort('cert')}>
                                mTLS expiry <SortIcon col="cert" />
                            </th>
                            <th className="px-3 py-2.5">Last seen</th>
                            <th className="px-3 py-2.5">Reasons</th>
                        </tr>
                    </thead>
                    <tbody>
                        {pageRows.map(({ agent: a, eff, compliant: ok, reasons }) => (
                            <tr
                                key={a.id}
                                className="border-t border-slate-100 dark:border-slate-800 hover:bg-slate-50 dark:hover:bg-slate-800/60 transition-colors"
                            >
                                <td className="px-3 py-2.5">
                                    <Link
                                        className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline"
                                        to={`/management/devices/${encodeURIComponent(a.id)}`}
                                    >
                                        {a.hostname}
                                    </Link>
                                </td>
                                <td className="px-3 py-2.5">
                                    {ok ? (
                                        <span className="text-xs font-bold text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded-full">
                                            Compliant
                                        </span>
                                    ) : (
                                        <span className="text-xs font-bold text-amber-600 dark:text-amber-400 bg-amber-500/10 px-2 py-0.5 rounded-full">
                                            Non-compliant
                                        </span>
                                    )}
                                </td>
                                <td className="px-3 py-2.5 text-xs font-mono text-slate-600 dark:text-slate-300">{eff}</td>
                                <td className="px-3 py-2.5">
                                    <span
                                        className={`text-xs font-mono font-semibold ${
                                            (a.health_score ?? 0) >= 80 ? 'text-emerald-600 dark:text-emerald-400' : 'text-amber-600 dark:text-amber-400'
                                        }`}
                                    >
                                        {Math.round(a.health_score ?? 0)}%
                                    </span>
                                </td>
                                <td className="px-3 py-2.5 text-xs text-slate-600 dark:text-slate-300">
                                    <span className="inline-flex items-center gap-1">
                                        <KeyRound className="w-3 h-3 text-slate-400 shrink-0" />
                                        {a.cert_expires_at ? formatDate(a.cert_expires_at) : '—'}
                                    </span>
                                </td>
                                <td className="px-3 py-2.5 text-xs text-slate-500">{formatRelativeTime(a.last_seen)}</td>
                                <td className="px-3 py-2.5 text-xs text-slate-500 dark:text-slate-400 max-w-md" title={reasons.join(' · ')}>
                                    {reasons.length ? reasons.join(' · ') : '—'}
                                </td>
                            </tr>
                        ))}
                        {pageRows.length === 0 && (
                            <tr>
                                <td colSpan={7} className="px-3 py-8 text-center text-sm text-slate-500">
                                    No matching active endpoints.
                                </td>
                            </tr>
                        )}
                    </tbody>
                </table>
            </div>

            {decommissioned.length > 0 && (
                <details className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-slate-50/80 dark:bg-slate-900/40 px-4 py-3 text-sm">
                    <summary className="cursor-pointer font-medium text-slate-700 dark:text-slate-200">
                        Decommissioned hosts ({decommissioned.length}) — excluded from ratio
                    </summary>
                    <ul className="mt-2 space-y-1 text-xs text-slate-600 dark:text-slate-400 max-h-40 overflow-y-auto">
                        {decommissioned.map((a) => (
                            <li key={a.id}>
                                <Link className="text-cyan-600 dark:text-cyan-400 hover:underline" to={`/management/devices/${encodeURIComponent(a.id)}`}>
                                    {a.hostname}
                                </Link>{' '}
                                <span className="font-mono text-slate-500">({a.status})</span>
                            </li>
                        ))}
                    </ul>
                </details>
            )}

            {totalPages > 1 && (
                <div className="flex items-center justify-between text-xs text-slate-500">
                    <span>
                        Page {safePage + 1} of {totalPages}
                    </span>
                    <div className="flex gap-1">
                        <button
                            disabled={safePage === 0}
                            onClick={() => setPage((p) => p - 1)}
                            className="p-1.5 rounded-lg border border-slate-300 dark:border-slate-600 hover:bg-slate-100 dark:hover:bg-slate-800 disabled:opacity-30 transition-colors"
                        >
                            <ChevronLeft className="w-4 h-4" />
                        </button>
                        <button
                            disabled={safePage >= totalPages - 1}
                            onClick={() => setPage((p) => p + 1)}
                            className="p-1.5 rounded-lg border border-slate-300 dark:border-slate-600 hover:bg-slate-100 dark:hover:bg-slate-800 disabled:opacity-30 transition-colors"
                        >
                            <ChevronRight className="w-4 h-4" />
                        </button>
                    </div>
                </div>
            )}
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
    useEffect(() => { document.title = 'Reports \u2014 EDR Platform'; }, []);

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
            return `"${s.replaceAll('"', '""')}"`;
        };
        const headers = Array.from(rows.reduce((set, r) => { Object.keys(r).forEach(k => set.add(k)); return set; }, new Set<string>()));
        const lines = [headers.join(','), ...rows.map(r => headers.map(h => esc(r[h])).join(','))];
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
        const maxIter = Math.ceil(max / limit) + 1;
        const out: T[] = [];
        let offset = 0;
        for (let i = 0; i < maxIter; i++) {
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
    const [lastCount, setLastCount] = useState<number | null>(null);

    const agentsListQ = useQuery({
        queryKey: ['agents', 'list', 'reports'],
        queryFn: async () => fetchAllPaged((offset, limit) => agentsApi.list({ offset, limit, sort_by: 'hostname', sort_order: 'asc' }), { max: 2000 }),
        staleTime: 30_000, retry: 1,
    });

    const runReport = async (fmt: 'json' | 'csv') => {
        try {
            setLoading(true); setErr(null); setLastCount(null);
            const fromIso = new Date(from).toISOString();
            const toIso = new Date(to).toISOString();

            if (reportType === 'commands') {
                const rows = await fetchAllPaged(
                    (offset, limit) => commandsApi.list({ offset, limit, agent_id: agentScope === 'all' ? undefined : agentScope, sort_by: 'issued_at', sort_order: 'desc' }),
                    { max: 5000 }
                );
                const filtered = rows.filter((c: any) => { const t = new Date(c.issued_at).getTime(); return t >= new Date(fromIso).getTime() && t <= new Date(toIso).getTime(); });
                setLastCount(filtered.length);
                const mapped = filtered.map((c: any) => ({ id: c.id, agent_id: c.agent_id, agent_hostname: c.agent_hostname, command_type: c.command_type, status: c.status, issued_at: c.issued_at, issued_by_user: c.issued_by_user, exit_code: c.exit_code ?? '', error_message: c.error_message ?? '' }));
                fmt === 'json' ? downloadJson('report-commands', { from: fromIso, to: toIso, scope: agentScope, rows: mapped }) : downloadCsv('report-commands', mapped);
                return;
            }
            if (reportType === 'alerts') {
                const allAlerts = await fetchAllPaged(
                    (offset, limit) => alertsApi.list({ limit, offset, agent_id: agentScope === 'all' ? undefined : agentScope, date_from: fromIso, date_to: toIso, sort: 'timestamp', order: 'desc' }).then(r => ({ data: r.alerts ?? [], pagination: { has_more: (r.alerts?.length ?? 0) >= limit } })),
                    { max: 2000 }
                );
                setLastCount(allAlerts.length);
                const mapped = allAlerts.map((a: any) => ({ id: a.id, timestamp: a.timestamp, agent_id: a.agent_id, rule_title: a.rule_title, severity: a.severity, status: a.status, category: a.category, risk_score: a.risk_score ?? '' }));
                fmt === 'json' ? downloadJson('report-alerts', { from: fromIso, to: toIso, scope: agentScope, rows: mapped }) : downloadCsv('report-alerts', mapped);
                return;
            }
            if (reportType === 'devices') {
                const allAgents = agentsListQ.data ?? [];
                const fromT = new Date(fromIso).getTime();
                const toT = new Date(toIso).getTime();
                const filtered = allAgents.filter((a: any) => { const t = new Date(a.created_at).getTime(); return t >= fromT && t <= toT; });
                setLastCount(filtered.length);
                const mapped = filtered.map((a: any) => ({ id: a.id, hostname: a.hostname, os_type: a.os_type, os_version: a.os_version, agent_version: a.agent_version, status: a.status, created_at: a.created_at, last_seen: a.last_seen }));
                fmt === 'json' ? downloadJson('report-devices', { from: fromIso, to: toIso, rows: mapped }) : downloadCsv('report-devices', mapped);
            }
        } catch (e: any) {
            setErr(e?.message || 'Report generation failed');
        } finally { setLoading(false); }
    };

    const inputCls = "w-full px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-sm text-slate-900 dark:text-white";
    const labelCls = "block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1";

    return (
        <div className="space-y-6 animate-slide-up-fade w-full min-w-0">
            <InsightHero
                variant="light"
                accent="violet"
                icon={ClipboardList}
                eyebrow="Dashboards"
                title="Reports"
                segments={[
                    {
                        heading: 'Exports',
                        children: (
                            <>
                                Generate <strong className="font-semibold text-slate-800 dark:text-slate-200">browser-side downloads</strong> (JSON or CSV) from live APIs — executed
                                commands, Sigma alerts, or devices enrolled in the selected time window. Pick report type, optional endpoint scope, and range below.
                            </>
                        ),
                    },
                    {
                        heading: 'Limits & safety',
                        children: (
                            <>
                                Large extracts are <strong className="font-semibold text-slate-800 dark:text-slate-200">paged and capped</strong> so the UI stays responsive. For unconstrained
                                search, streaming triage, or audit trails, use the linked workspaces at the bottom of this page.
                            </>
                        ),
                    },
                    {
                        heading: 'Metric snapshots',
                        children: (
                            <>
                                The three summary tiles below expose read-only <strong className="font-semibold text-slate-800 dark:text-slate-200">aggregate JSON</strong> (fleet, commands,
                                alerts) for quick archival — separate from the filtered row exports above.
                            </>
                        ),
                    },
                ]}
            />

            <div className="rounded-2xl border border-slate-200/90 dark:border-slate-700/80 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm shadow-md overflow-hidden">
                <div className="px-5 py-4 sm:px-6 border-b border-slate-200 dark:border-slate-700 bg-gradient-to-r from-violet-50/80 via-white to-slate-50/80 dark:from-violet-950/30 dark:via-slate-900/80 dark:to-slate-900/60">
                    <h2 className="text-base font-semibold text-slate-900 dark:text-white tracking-tight">Report builder</h2>
                    <p className="text-xs text-slate-500 dark:text-slate-400 mt-1 max-w-none leading-relaxed">
                        Choose data type, scope, and time range — then download. Generation runs in this browser against live connection-manager and Sigma endpoints.
                    </p>
                </div>
                <div className="p-5 sm:p-6 space-y-4">
                <div className="grid grid-cols-1 lg:grid-cols-4 gap-3">
                    <div>
                        <label className={labelCls}>Report type</label>
                        <select value={reportType} onChange={e => setReportType(e.target.value as any)} className={inputCls}>
                            <option value="commands">Executed commands</option>
                            <option value="alerts">Alerts</option>
                            <option value="devices">Devices (joined in range)</option>
                        </select>
                    </div>
                    <div>
                        <label className={labelCls}>Scope</label>
                        <select value={agentScope} onChange={e => setAgentScope(e.target.value)} className={inputCls}>
                            <option value="all">All endpoints</option>
                            {(agentsListQ.data ?? []).slice(0, 500).map((a: any) => (
                                <option key={a.id} value={a.id}>{a.hostname}</option>
                            ))}
                        </select>
                        {agentsListQ.isLoading && <div className="mt-1 text-xs text-slate-500">Loading endpoints\u2026</div>}
                    </div>
                    <div>
                        <label className={labelCls}>From</label>
                        <input type="datetime-local" value={from} onChange={e => setFrom(e.target.value)} className={inputCls} />
                    </div>
                    <div>
                        <label className={labelCls}>To</label>
                        <input type="datetime-local" value={to} onChange={e => setTo(e.target.value)} className={inputCls} />
                    </div>
                </div>
                {err && <div className="rounded-lg border border-rose-200 dark:border-rose-900/50 bg-rose-50/80 dark:bg-rose-950/20 px-4 py-3 text-sm text-rose-900 dark:text-rose-200">{err}</div>}
                <div className="flex flex-wrap items-center gap-2">
                    <button type="button" disabled={loading} onClick={() => runReport('json')}
                        className="px-4 py-2 rounded-lg text-sm font-semibold bg-cyan-600 hover:bg-cyan-700 text-white disabled:opacity-50 transition-colors">
                        {loading ? 'Generating\u2026' : 'Download JSON'}
                    </button>
                    <button type="button" disabled={loading} onClick={() => runReport('csv')}
                        className="px-4 py-2 rounded-lg text-sm font-semibold bg-slate-700 hover:bg-slate-800 text-white disabled:opacity-50 dark:bg-slate-600 dark:hover:bg-slate-500 transition-colors">
                        {loading ? 'Generating\u2026' : 'Download CSV'}
                    </button>
                    {loading && (
                        <div className="flex-1 max-w-xs"><div className="h-1.5 rounded-full bg-slate-200 dark:bg-slate-700 overflow-hidden"><div className="h-full bg-cyan-500 rounded-full animate-pulse" style={{ width: '60%' }} /></div></div>
                    )}
                    {lastCount !== null && !loading && <span className="text-xs text-slate-500 ml-auto">Last report: {lastCount} rows exported</span>}
                </div>
                </div>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                {[
                    {
                        title: 'Fleet snapshot',
                        desc: 'Live aggregate from connection-manager agent metrics.',
                        data: agentsQ.data,
                        name: 'fleet-stats',
                        icon: Server,
                    },
                    {
                        title: 'Command operations',
                        desc: 'Queue and outcome totals from the command API.',
                        data: cmdQ.data,
                        name: 'commands-stats',
                        icon: Terminal,
                    },
                    {
                        title: 'Alert summary',
                        desc: 'Sigma engine alert statistics snapshot.',
                        data: alertQ.data,
                        name: 'sigma-alert-stats',
                        icon: AlertTriangle,
                    },
                ].map((card) => {
                    const Icon = card.icon;
                    return (
                        <div
                            key={card.name}
                            className="rounded-xl border border-slate-200/90 dark:border-slate-700/80 bg-gradient-to-br from-white to-violet-50/30 dark:from-slate-800/95 dark:to-violet-950/25 p-5 shadow-sm flex flex-col gap-4 ring-1 ring-violet-500/[0.06] dark:ring-violet-400/10"
                        >
                            <div className="flex items-start gap-3 min-w-0">
                                <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-violet-500/12 text-violet-600 dark:text-violet-400 border border-violet-500/15">
                                    <Icon className="h-5 w-5" aria-hidden />
                                </span>
                                <div className="min-w-0">
                                    <div className="font-semibold text-slate-900 dark:text-white leading-snug">{card.title}</div>
                                    <div className="text-xs text-slate-500 dark:text-slate-400 mt-1 leading-relaxed">{card.desc}</div>
                                </div>
                            </div>
                            <button
                                type="button"
                                disabled={!card.data}
                                onClick={() => downloadJson(card.name, card.data)}
                                className="mt-auto w-full sm:w-auto self-start px-4 py-2 rounded-lg text-xs font-semibold bg-cyan-600 hover:bg-cyan-700 text-white disabled:opacity-50 transition-colors shadow-sm"
                            >
                                Download JSON
                            </button>
                        </div>
                    );
                })}
            </div>

            <section className="rounded-2xl border border-slate-200/90 dark:border-slate-700/80 bg-slate-50/90 dark:bg-slate-900/50 px-5 py-5 sm:px-6 sm:py-6">
                <h3 className="text-sm font-semibold text-slate-900 dark:text-white">Full workspaces</h3>
                <p className="text-xs text-slate-500 dark:text-slate-400 mt-1 leading-relaxed max-w-3xl">
                    For complete datasets with server-side pagination, saved filters, streaming, or tamper-evident audit trails — use these routes instead of bulk export alone.
                </p>
                <div className="mt-4 flex flex-wrap gap-2">
                    {[
                        { to: '/alerts', label: 'Alerts' },
                        { to: '/responses', label: 'Command Center' },
                        { to: '/events', label: 'Telemetry Search' },
                        { to: '/system/audit-logs', label: 'Audit logs' },
                    ].map((x) => (
                        <Link
                            key={x.to}
                            to={x.to}
                            className="inline-flex items-center rounded-lg border border-slate-200 dark:border-slate-600 bg-white dark:bg-slate-800 px-3 py-2 text-sm font-medium text-cyan-700 dark:text-cyan-300 hover:border-cyan-300 dark:hover:border-cyan-600 hover:bg-cyan-50/80 dark:hover:bg-cyan-950/30 transition-colors"
                        >
                            {x.label}
                        </Link>
                    ))}
                </div>
            </section>
        </div>
    );
}



export function DashboardNotificationsPage() {
    return (
        <div className="rounded-xl border border-dashed border-slate-300 dark:border-slate-600 p-8 text-center text-slate-500 dark:text-slate-400">
            <p className="font-medium text-slate-700 dark:text-slate-300">Notifications</p>
            <p className="text-sm mt-2">In-app notification center — coming soon.</p>
            <p className="text-sm mt-4 text-left max-w-md mx-auto text-slate-600 dark:text-slate-300">
                For now, monitor <Link className="text-cyan-600 dark:text-cyan-400 font-medium" to="/alerts">Alerts</Link>
                {' '}and <Link className="text-cyan-600 dark:text-cyan-400 font-medium" to="/">Overview</Link> live streams.
            </p>
        </div>
    );
}

