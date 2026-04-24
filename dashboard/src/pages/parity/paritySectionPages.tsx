import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import React, { useEffect, useMemo, useState } from 'react';
import { GenericParityView } from '../../components/parity/GenericParityView';
import { parityApi } from '../../api/parity/parityApi';
import * as mocks from '../../api/parity/mocks';
import { useParityQuery } from '../../api/parity/withFallback';
import { ParityMockBanner } from '../../components/parity/ParityMockBanner';
import type { AppControlPoliciesPayload, AppControlPolicy } from '../../api/parity/appControlModel';
import StatCard from '../../components/StatCard';
import InsightHero from '../../components/InsightHero';
import {
    Activity,
    AlertTriangle,
    Ban,
    BarChart3,
    BookOpen,
    ChevronLeft,
    ChevronRight,
    Clock,
    Eye,
    Fingerprint,
    KeyRound,
    Layers,
    ListFilter,
    Loader2,
    Lock,
    Plug,
    Radio,
    RefreshCw,
    Search,
    Shield,
    Share2,
    ShieldCheck,
    Terminal,
    Trash2,
    TrendingUp,
    User as UserIcon,
    Wifi,
    WifiOff,
    X,
    Zap,
    Settings,
} from 'lucide-react';
import {
    Bar,
    BarChart,
    CartesianGrid,
    Cell,
    Legend,
    Pie,
    PieChart,
    ResponsiveContainer,
    Tooltip,
    XAxis,
    YAxis,
} from 'recharts';
import {
    agentsApi,
    alertsApi,
    authApi,
    commandsApi,
    incidentApi,
    siemConnectorsApi,
    statsApi,
    vulnerabilityApi,
    type Agent,
    type Alert,
    type User,
    type AlertStats,
    type CommandType,
    type EndpointRiskSummary,
    type SiemConnector,
    type VulnerabilityFinding,
} from '../../api/client';
import { useToast } from '../../components';
import { formatDateTime, formatRelativeTime, getEffectiveStatus, STALE_THRESHOLD_MS } from '../../utils/agentDisplay';

const ZT_CHART_TOOLTIP = {
    backgroundColor: 'rgba(15, 23, 42, 0.92)',
    border: '1px solid rgba(51, 65, 85, 0.85)',
    borderRadius: '12px',
    color: 'white',
};

const HEALTH_BUCKET_COLORS = ['#f43f5e', '#f97316', '#22c55e'];

/** Normalize GET /api/v1/itsm/playbooks payloads into flat object rows for UI */
function extractPlaybookCatalogRows(data: unknown): Record<string, unknown>[] {
    if (data == null) return [];
    if (Array.isArray(data)) {
        return data.filter((x): x is Record<string, unknown> => x != null && typeof x === 'object' && !Array.isArray(x));
    }
    if (typeof data === 'object') {
        const o = data as Record<string, unknown>;
        for (const key of ['playbooks', 'items', 'data', 'catalog', 'results']) {
            const inner = o[key];
            if (Array.isArray(inner)) {
                const nested = extractPlaybookCatalogRows(inner);
                if (nested.length > 0) return nested;
            }
        }
        return [o];
    }
    return [];
}

function playbookRowTitle(row: Record<string, unknown>): string {
    for (const k of ['name', 'title', 'label', 'slug', 'id', 'key'] as const) {
        const v = row[k];
        if (typeof v === 'string' && v.trim()) return v;
        if (typeof v === 'number' && Number.isFinite(v)) return String(v);
    }
    return 'Playbook entry';
}

function playbookRowSubtitle(row: Record<string, unknown>): string | undefined {
    for (const k of ['description', 'summary', 'purpose', 'notes', 'category'] as const) {
        const v = row[k];
        if (typeof v === 'string' && v.trim()) return v.length > 280 ? `${v.slice(0, 277)}…` : v;
    }
    return undefined;
}

/** Short primitive fields for catalog card footer (skip bulky / nested values) */
function playbookRowMetaChips(row: Record<string, unknown>): { key: string; value: string }[] {
    const skip = new Set([
        'name',
        'title',
        'label',
        'slug',
        'description',
        'summary',
        'purpose',
        'notes',
        'category',
    ]);
    const out: { key: string; value: string }[] = [];
    for (const [k, v] of Object.entries(row)) {
        if (skip.has(k)) continue;
        if (v == null) continue;
        if (typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean') {
            const s = String(v);
            if (s.length > 48) continue;
            out.push({ key: k, value: s });
        }
        if (Array.isArray(v) && v.length > 0 && v.length <= 20) {
            out.push({ key: k, value: `${v.length} items` });
        }
    }
    return out.slice(0, 6);
}

function certDaysRemaining(agent: Agent): number | null {
    if (!agent.cert_expires_at) return null;
    const t = new Date(agent.cert_expires_at).getTime();
    if (Number.isNaN(t)) return null;
    return Math.ceil((t - Date.now()) / (24 * 60 * 60 * 1000));
}

/** Higher = worse trust posture for sorting exception queues. */
function zeroTrustConcernScore(agent: Agent): number {
    const eff = getEffectiveStatus(agent);
    let s = 0;
    if ((agent.status === 'online' || agent.status === 'degraded') && eff === 'offline') {
        s += 100;
    }
    if (agent.is_isolated) {
        s += 35;
    }
    if (agent.health_score < 60) {
        s += 45;
    } else if (agent.health_score < 80) {
        s += 15;
    }
    const certDays = certDaysRemaining(agent);
    if (certDays != null && certDays >= 0 && certDays <= 30) {
        s += 30 - certDays;
    }
    if (eff === 'offline' && agent.status === 'offline') {
        s += 8;
    }
    if (agent.status === 'suspended') {
        s += 25;
    }
    return s;
}

/** Commercial / MSP modules outside current self-hosted endpoint scope. */
function SelfHostedOutOfScope({ title }: { title: string }) {
    return (
        <div className="rounded-xl border border-amber-200 dark:border-amber-900/40 bg-amber-50/90 dark:bg-amber-950/25 p-6 space-y-3">
            <p className="font-semibold text-slate-900 dark:text-white">{title}</p>
            <p className="text-sm text-slate-600 dark:text-slate-400">
                Not part of the self-hosted EDR MVP. Use the sections below for real fleet and response workflows.
            </p>
            <div className="flex flex-wrap gap-x-4 gap-y-2 text-sm">
                <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/management/devices">
                    Devices (Fleet)
                </Link>
                <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/responses">
                    Command Center
                </Link>
                <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/system/access/users">
                    Access
                </Link>
            </div>
        </div>
    );
}

export function SecurityEndpointZeroTrustPage() {
    const statsQ = useQuery({ queryKey: ['agents-stats'], queryFn: () => agentsApi.stats(), staleTime: 30_000, refetchInterval: 45_000 });
    const riskQ = useQuery({ queryKey: ['endpoint-risk'], queryFn: () => alertsApi.endpointRisk(), staleTime: 60_000, retry: 1 });
    const agentsQ = useQuery({
        queryKey: ['agents', 'zero-trust', 500],
        queryFn: () => agentsApi.list({ limit: 500, offset: 0, sort_by: 'health_score', sort_order: 'asc' }),
        staleTime: 30_000,
        refetchInterval: 60_000,
    });

    const s = statsQ.data;
    const riskRows: EndpointRiskSummary[] = riskQ.data?.data ?? [];
    const agents = agentsQ.data?.data ?? [];

    const hostnameByAgent = useMemo(() => new Map(agents.map((a) => [a.id, a.hostname] as const)), [agents]);

    const ztDerived = useMemo(() => {
        let staleVerification = 0;
        let isolatedHosts = 0;
        let certExpiring30d = 0;
        let lowHealth = 0;
        let healthPoor = 0;
        let healthFair = 0;
        let healthGood = 0;
        for (const a of agents) {
            const eff = getEffectiveStatus(a);
            if ((a.status === 'online' || a.status === 'degraded') && eff === 'offline') {
                staleVerification += 1;
            }
            if (a.is_isolated) {
                isolatedHosts += 1;
            }
            const cd = certDaysRemaining(a);
            if (cd != null && cd >= 0 && cd <= 30) {
                certExpiring30d += 1;
            }
            if (a.health_score < 60) {
                lowHealth += 1;
                healthPoor += 1;
            } else if (a.health_score < 80) {
                healthFair += 1;
            } else {
                healthGood += 1;
            }
        }
        const withOpenAlerts = riskRows.filter((r) => r.open_count > 0).length;
        return {
            staleVerification,
            isolatedHosts,
            certExpiring30d,
            lowHealth,
            healthPoor,
            healthFair,
            healthGood,
            withOpenAlerts,
        };
    }, [agents, riskRows]);

    const osChartData = useMemo(() => {
        if (!s?.by_os_type) return [];
        return Object.entries(s.by_os_type).map(([name, value]) => ({ name, value }));
    }, [s]);

    const healthBarData = useMemo(
        () => [
            { name: '< 60', count: ztDerived.healthPoor, fill: HEALTH_BUCKET_COLORS[0] },
            { name: '60–79', count: ztDerived.healthFair, fill: HEALTH_BUCKET_COLORS[1] },
            { name: '≥ 80', count: ztDerived.healthGood, fill: HEALTH_BUCKET_COLORS[2] },
        ],
        [ztDerived.healthPoor, ztDerived.healthFair, ztDerived.healthGood]
    );

    const exceptionAgents = useMemo(() => {
        return [...agents]
            .map((a) => ({ agent: a, score: zeroTrustConcernScore(a) }))
            .filter((x) => x.score > 0)
            .sort((a, b) => b.score - a.score)
            .slice(0, 18)
            .map((x) => x.agent);
    }, [agents]);

    const trustDebtRows = useMemo(() => {
        return [...riskRows]
            .filter((r) => r.peak_risk_score > 0 || r.open_count > 0)
            .sort((a, b) => b.peak_risk_score - a.peak_risk_score)
            .slice(0, 10);
    }, [riskRows]);

    const loading = statsQ.isLoading || riskQ.isLoading || agentsQ.isLoading;

    return (
        <div className="space-y-6 animate-slide-up-fade w-full min-w-0">
            <InsightHero
                variant="dark"
                accent="indigo"
                icon={Fingerprint}
                eyebrow="Continuous device trust"
                title="Endpoint Zero Trust"
                lead={
                    <>
                        Interprets <strong className="text-white">live enrollment health</strong>,{' '}
                        <strong className="text-white">certificate continuity</strong>, <strong className="text-white">isolation posture</strong>, and{' '}
                        <strong className="text-white">alert-derived trust debt</strong> — the signals you verify before treating a host as trusted.
                        This view is <strong className="text-white">not</strong> the fleet network table, the full risk leaderboard, or correlation timelines.
                    </>
                }
            />

            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <Radio className="w-4 h-4 text-cyan-500 shrink-0" />
                        vs Fleet connectivity
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/management/network">
                            Fleet connectivity
                        </Link>{' '}
                        focuses on queues, drops, and addresses. Zero Trust here focuses on whether the <em>identity and health</em> of the agent still warrant trust.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <Activity className="w-4 h-4 text-amber-500 shrink-0" />
                        vs Endpoint risk
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/endpoint-risk">
                            Endpoint risk
                        </Link>{' '}
                        ranks every host for triage. Below we only surface <strong>trust debt excerpts</strong> and exception queues derived from the same API.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <BookOpen className="w-4 h-4 text-violet-500 shrink-0" />
                        vs Context policies
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/management/context-policies">
                            Context policies
                        </Link>{' '}
                        adjust alert scoring weights. They do not replace device health, cert, or isolation evidence shown here.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <Shield className="w-4 h-4 text-emerald-500 shrink-0" />
                        vs Correlation
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/soc/correlation">
                            Correlation
                        </Link>{' '}
                        fuses timelines for investigation. Zero Trust answers “should we still trust this device class of signals?” at a glance.
                    </p>
                </div>
            </div>

            {loading && <div className="h-44 rounded-xl bg-slate-100 dark:bg-slate-800 animate-pulse" />}

            {(statsQ.isError || !s) && !loading && (
                <div className="rounded-xl border border-rose-200 dark:border-rose-900/50 bg-rose-50/80 dark:bg-rose-950/20 p-6 text-sm text-rose-900 dark:text-rose-200">
                    Could not load agent stats (<code className="text-xs">GET /api/v1/agents/stats</code>). Verify connection-manager and <code className="text-xs">endpoints:read</code>.
                </div>
            )}

            {agentsQ.isError && !loading && (
                <div className="rounded-xl border border-amber-200 dark:border-amber-900/40 bg-amber-50/80 dark:bg-amber-950/20 p-4 text-sm text-amber-900 dark:text-amber-100">
                    Agent list unavailable — ZT charts that depend on per-host rows are hidden. Stats above may still apply.
                </div>
            )}

            {s && !loading && (
                <>
                    <p className="text-xs text-slate-500 dark:text-slate-400">
                        Registry reference: <strong className="text-slate-700 dark:text-slate-200">{s.total}</strong> endpoints ·{' '}
                        <strong className="text-slate-700 dark:text-slate-200">{s.online}</strong> online registry ·{' '}
                        <strong className="text-slate-700 dark:text-slate-200">{s.offline}</strong> offline · fleet avg health{' '}
                        <strong className="text-slate-700 dark:text-slate-200">{Math.round(s.avg_health)}%</strong> (
                        <Link className="text-cyan-600 dark:text-cyan-400 hover:underline" to="/management/devices">
                            open Devices
                        </Link>
                        ).
                    </p>

                    <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                        <StatCard
                            title="Stale verification"
                            value={String(ztDerived.staleVerification)}
                            icon={WifiOff}
                            color="amber"
                            subtext={`Registry online/degraded but last_seen > ${Math.round(STALE_THRESHOLD_MS / 1000)}s`}
                        />
                        <StatCard title="Isolated hosts" value={String(ztDerived.isolatedHosts)} icon={Lock} color="red" subtext="Network isolation flag" />
                        <StatCard
                            title="Certs ≤ 30d"
                            value={String(ztDerived.certExpiring30d)}
                            icon={KeyRound}
                            color="amber"
                            subtext="Agent TLS material on file"
                        />
                        <StatCard title="Low health (under 60)" value={String(ztDerived.lowHealth)} icon={ShieldCheck} color="red" subtext="Loaded sample (500 hosts)" />
                    </div>

                    <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                        <StatCard title="Hosts w/ open alerts" value={String(ztDerived.withOpenAlerts)} icon={AlertTriangle} color="amber" />
                        <StatCard title="Suspended" value={String(s.suspended)} icon={AlertTriangle} color="red" />
                        <StatCard title="Pending enroll" value={String(s.pending)} icon={Clock} />
                        <StatCard title="Degraded (registry)" value={String(s.degraded)} icon={Activity} color="amber" />
                    </div>

                    {agents.length > 0 && (
                        <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
                            <div className="lg:col-span-1 rounded-xl border border-slate-200/80 dark:border-slate-700/50 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm min-h-[260px] flex flex-col">
                                <h3 className="text-sm font-bold text-slate-900 dark:text-white mb-1">Fleet OS (registry)</h3>
                                <p className="text-xs text-slate-500 dark:text-slate-400 mb-2">From <code className="text-[10px]">agents/stats</code>.</p>
                                <div className="flex-1 min-h-[200px]">
                                    {osChartData.length === 0 ? (
                                        <div className="h-full flex items-center justify-center text-xs text-slate-500">No OS breakdown.</div>
                                    ) : (
                                        <ResponsiveContainer width="100%" height="100%">
                                            <PieChart>
                                                <Pie data={osChartData} dataKey="value" nameKey="name" cx="50%" cy="50%" innerRadius={48} outerRadius={72} paddingAngle={2}>
                                                    {osChartData.map((_, i) => (
                                                        <Cell key={i} fill={['#22d3ee', '#a78bfa', '#34d399', '#fb923c', '#94a3b8'][i % 5]} />
                                                    ))}
                                                </Pie>
                                                <Tooltip contentStyle={ZT_CHART_TOOLTIP} />
                                            </PieChart>
                                        </ResponsiveContainer>
                                    )}
                                </div>
                            </div>
                            <div className="lg:col-span-2 rounded-xl border border-slate-200/80 dark:border-slate-700/50 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm min-h-[260px] flex flex-col">
                                <h3 className="text-sm font-bold text-slate-900 dark:text-white mb-1">Health distribution (loaded agents)</h3>
                                <p className="text-xs text-slate-500 dark:text-slate-400 mb-2">Buckets from live <code className="text-[10px]">health_score</code> on up to 500 rows.</p>
                                <div className="flex-1 min-h-[200px]">
                                    <ResponsiveContainer width="100%" height="100%">
                                        <BarChart data={healthBarData} margin={{ top: 8, right: 8, left: -8, bottom: 0 }}>
                                            <CartesianGrid strokeDasharray="3 3" stroke="#334155" opacity={0.2} vertical={false} />
                                            <XAxis dataKey="name" tick={{ fontSize: 11, fill: '#94a3b8' }} />
                                            <YAxis allowDecimals={false} tick={{ fontSize: 11, fill: '#94a3b8' }} />
                                            <Tooltip contentStyle={ZT_CHART_TOOLTIP} />
                                            <Bar dataKey="count" radius={[6, 6, 0, 0]} name="Hosts">
                                                {healthBarData.map((entry, index) => (
                                                    <Cell key={`cell-${index}`} fill={entry.fill} />
                                                ))}
                                            </Bar>
                                        </BarChart>
                                    </ResponsiveContainer>
                                </div>
                            </div>
                        </div>
                    )}

                    <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
                        <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 shadow-sm overflow-hidden">
                            <div className="px-4 py-3 border-b border-slate-200 dark:border-slate-700/60 bg-slate-50/80 dark:bg-slate-900/40">
                                <h3 className="text-sm font-bold text-slate-900 dark:text-white">Trust verification exceptions</h3>
                                <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
                                    Prioritized queue (stale registry, isolation, weak health, expiring certs). Worst first — max 18 from loaded sample.
                                </p>
                            </div>
                            <div className="overflow-x-auto max-h-[340px] overflow-y-auto">
                                <table className="min-w-full text-left text-xs">
                                    <thead className="sticky top-0 bg-slate-50 dark:bg-slate-800/95 text-slate-500 uppercase z-10">
                                        <tr>
                                            <th className="px-3 py-2">Host</th>
                                            <th className="px-3 py-2">Live</th>
                                            <th className="px-3 py-2">Health</th>
                                            <th className="px-3 py-2 hidden sm:table-cell">Cert</th>
                                            <th className="px-3 py-2">Iso</th>
                                            <th className="px-3 py-2 text-right" />
                                        </tr>
                                    </thead>
                                    <tbody>
                                        {exceptionAgents.length === 0 ? (
                                            <tr>
                                                <td colSpan={6} className="px-3 py-8 text-center text-slate-500 dark:text-slate-400">
                                                    No trust exceptions in the loaded sample — good baseline.
                                                </td>
                                            </tr>
                                        ) : (
                                            exceptionAgents.map((a) => {
                                                const eff = getEffectiveStatus(a);
                                                const cd = certDaysRemaining(a);
                                                return (
                                                    <tr key={a.id} className="border-t border-slate-100 dark:border-slate-800">
                                                        <td className="px-3 py-2 font-medium text-slate-900 dark:text-white">{a.hostname}</td>
                                                        <td className="px-3 py-2 font-mono text-[10px] text-slate-600 dark:text-slate-300">{eff}</td>
                                                        <td className="px-3 py-2 tabular-nums">{Math.round(a.health_score)}%</td>
                                                        <td className="px-3 py-2 hidden sm:table-cell text-slate-500">
                                                            {cd == null ? '—' : cd < 0 ? 'expired' : `${cd}d`}
                                                        </td>
                                                        <td className="px-3 py-2">{a.is_isolated ? 'Yes' : '—'}</td>
                                                        <td className="px-3 py-2 text-right">
                                                            <Link
                                                                className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium"
                                                                to={`/management/devices/${encodeURIComponent(a.id)}`}
                                                            >
                                                                Device
                                                            </Link>
                                                        </td>
                                                    </tr>
                                                );
                                            })
                                        )}
                                    </tbody>
                                </table>
                            </div>
                        </div>

                        <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 shadow-sm overflow-hidden">
                            <div className="px-4 py-3 border-b border-slate-200 dark:border-slate-700/60 bg-slate-50/80 dark:bg-slate-900/40">
                                <h3 className="text-sm font-bold text-slate-900 dark:text-white">Alert-driven trust debt (top)</h3>
                                <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
                                    From <code className="text-[10px]">GET /api/v1/alerts/endpoint-risk</code> — excerpt only; full ranking stays on Endpoint Risk.
                                </p>
                            </div>
                            <div className="overflow-x-auto">
                                <table className="min-w-full text-left text-xs">
                                    <thead className="text-slate-500 uppercase bg-slate-50 dark:bg-slate-800/80">
                                        <tr>
                                            <th className="px-3 py-2">Host</th>
                                            <th className="px-3 py-2 text-right">Peak risk</th>
                                            <th className="px-3 py-2 text-right">Open alerts</th>
                                            <th className="px-3 py-2 text-right" />
                                        </tr>
                                    </thead>
                                    <tbody>
                                        {trustDebtRows.length === 0 ? (
                                            <tr>
                                                <td colSpan={4} className="px-3 py-8 text-center text-slate-500 dark:text-slate-400">
                                                    No risk rows returned.
                                                </td>
                                            </tr>
                                        ) : (
                                            trustDebtRows.map((r) => (
                                                <tr key={r.agent_id} className="border-t border-slate-100 dark:border-slate-800">
                                                    <td className="px-3 py-2 font-medium text-slate-900 dark:text-white">
                                                        {hostnameByAgent.get(r.agent_id) ?? r.agent_id.slice(0, 8) + '…'}
                                                    </td>
                                                    <td className="px-3 py-2 text-right tabular-nums">{Math.round(r.peak_risk_score)}</td>
                                                    <td className="px-3 py-2 text-right tabular-nums">{r.open_count}</td>
                                                    <td className="px-3 py-2 text-right">
                                                        <Link
                                                            className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium"
                                                            to={`/management/devices/${encodeURIComponent(r.agent_id)}`}
                                                        >
                                                            Device
                                                        </Link>
                                                    </td>
                                                </tr>
                                            ))
                                        )}
                                    </tbody>
                                </table>
                            </div>
                        </div>
                    </div>

                    <div className="flex flex-wrap gap-3 text-sm">
                        <Link to="/management/devices" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                            Devices (Fleet) →
                        </Link>
                        <Link to="/endpoint-risk" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                            Endpoint Risk (full board) →
                        </Link>
                        <Link to="/soc/correlation" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                            SOC Correlation →
                        </Link>
                    </div>
                </>
            )}
        </div>
    );
}

export function SecurityCloudZeroTrustPage() {
    return (
        <GenericParityView
            title="Cloud Security — Zero Trust"
            missingApi="true"
            queryKey={['parity', 'security', 'posture', 'cloud']}
            fetcher={() => parityApi.getSecurityPostureCloud()}
            mock={mocks.mockSecurityPostureCloud}
        />
    );
}

const SIEM_TYPE_LABELS: Record<string, string> = {
    splunk_hec: 'Splunk HEC',
    azure_sentinel: 'Azure Sentinel',
    elastic_webhook: 'Elastic webhook',
    generic_webhook: 'Generic HTTPS',
    syslog_tls: 'Syslog (TLS)',
};

const SIEM_CHART_TT = {
    backgroundColor: 'rgba(15, 23, 42, 0.92)',
    border: '1px solid rgba(51, 65, 85, 0.85)',
    borderRadius: '12px',
    color: 'white',
};

export function SecuritySiemPage() {
    const { showToast } = useToast();
    const queryClient = useQueryClient();
    const canWriteSettings = authApi.canWriteSettings();

    const [name, setName] = useState('');
    const [connectorType, setConnectorType] = useState<SiemConnector['connector_type']>('generic_webhook');
    const [endpointUrl, setEndpointUrl] = useState('');
    const [notes, setNotes] = useState('');
    const [enabledDraft, setEnabledDraft] = useState(true);

    const listQ = useQuery({
        queryKey: ['siem-connectors'],
        queryFn: () => siemConnectorsApi.list(),
        staleTime: 15_000,
        refetchInterval: 45_000,
    });

    const createM = useMutation({
        mutationFn: () =>
            siemConnectorsApi.create({
                name: name.trim(),
                connector_type: connectorType,
                endpoint_url: endpointUrl.trim(),
                enabled: enabledDraft,
                notes: notes.trim(),
            }),
        onSuccess: () => {
            showToast('Connector created', 'success');
            setName('');
            setEndpointUrl('');
            setNotes('');
            queryClient.invalidateQueries({ queryKey: ['siem-connectors'] });
        },
        onError: (e: Error) => showToast(e.message || 'Create failed', 'error'),
    });

    const patchM = useMutation({
        mutationFn: ({ id, body }: { id: string; body: Parameters<typeof siemConnectorsApi.patch>[1] }) => siemConnectorsApi.patch(id, body),
        onSuccess: () => {
            showToast('Updated', 'success');
            queryClient.invalidateQueries({ queryKey: ['siem-connectors'] });
        },
        onError: (e: Error) => showToast(e.message || 'Update failed', 'error'),
    });

    const deleteM = useMutation({
        mutationFn: (id: string) => siemConnectorsApi.remove(id),
        onSuccess: () => {
            showToast('Connector removed', 'success');
            queryClient.invalidateQueries({ queryKey: ['siem-connectors'] });
        },
        onError: (e: Error) => showToast(e.message || 'Delete failed', 'error'),
    });

    const rows = listQ.data ?? [];
    const enabledCount = useMemo(() => rows.filter((r) => r.enabled).length, [rows]);
    const typeChartData = useMemo(() => {
        const m = new Map<string, number>();
        for (const r of rows) {
            m.set(r.connector_type, (m.get(r.connector_type) ?? 0) + 1);
        }
        return [...m.entries()].map(([k, count]) => ({ name: SIEM_TYPE_LABELS[k] || k, count }));
    }, [rows]);

    return (
        <div className="space-y-6 animate-slide-up-fade w-full min-w-0">
            <InsightHero
                variant="dark"
                accent="cyan"
                icon={Share2}
                eyebrow="Export & forwarding"
                title="SIEM — X"
                lead={
                    <>
                        Configure <strong className="text-white">server-side destinations</strong> where the platform may forward normalized security data (alerts, audit, telemetry
                        pipelines are roadmap items). This page is <strong className="text-white">not</strong> the in-product alert triage grid, raw event search, or immutable audit
                        log viewer — it only manages outbound integration targets stored in connection-manager.
                    </>
                }
            />

            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <Radio className="w-4 h-4 text-amber-500" />
                        vs Telemetry Search
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/events">
                            Telemetry Search
                        </Link>{' '}
                        queries events inside the platform. SIEM — X defines where copies could be shipped out.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <AlertTriangle className="w-4 h-4 text-orange-500" />
                        vs Alerts
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/alerts">
                            Alerts
                        </Link>{' '}
                        are detections for analysts. Connectors here are infrastructure endpoints, not alert rows.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <BookOpen className="w-4 h-4 text-violet-500" />
                        vs Audit logs
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/system/audit-logs">
                            Audit logs
                        </Link>{' '}
                        are the tamper-evident operator trail. SIEM connectors are separate forwarder configuration.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <Activity className="w-4 h-4 text-emerald-500" />
                        vs Correlation
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/soc/correlation">
                            Correlation
                        </Link>{' '}
                        fuses timelines per host. SIEM — X does not visualize fused streams — it registers export sinks.
                    </p>
                </div>
            </div>

            {listQ.isLoading && <div className="h-36 rounded-xl bg-slate-100 dark:bg-slate-800 animate-pulse" />}

            {listQ.isError && (
                <div className="rounded-xl border border-rose-200 dark:border-rose-900/50 bg-rose-50/80 dark:bg-rose-950/20 p-4 text-sm text-rose-900 dark:text-rose-200">
                    Could not load connectors. Confirm connection-manager is up and your role includes <code className="text-xs">settings:read</code>.
                </div>
            )}

            {!listQ.isLoading && !listQ.isError && (
                <>
                    <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                        <StatCard title="Connectors" value={String(rows.length)} icon={Plug} color="cyan" />
                        <StatCard title="Enabled" value={String(enabledCount)} icon={Share2} color="emerald" />
                        <StatCard title="Disabled" value={String(rows.length - enabledCount)} icon={Plug} />
                        <StatCard title="Mutations" value={canWriteSettings ? 'Allowed' : 'View only'} icon={Shield} color="amber" subtext={canWriteSettings ? 'Admin (settings:write)' : 'Requires admin'} />
                    </div>

                    <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
                        <div className="lg:col-span-2 rounded-xl border border-slate-200/80 dark:border-slate-700/50 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm min-h-[240px] flex flex-col">
                            <h3 className="text-sm font-bold text-slate-900 dark:text-white mb-1">Connectors by type</h3>
                            <p className="text-xs text-slate-500 dark:text-slate-400 mb-2">Live counts from <code className="text-[10px]">GET /api/v1/siem/connectors</code>.</p>
                            <div className="flex-1 min-h-[200px]">
                                {typeChartData.length === 0 ? (
                                    <div className="h-full flex items-center justify-center text-sm text-slate-500">No connectors yet — add one below.</div>
                                ) : (
                                    <ResponsiveContainer width="100%" height="100%">
                                        <BarChart data={typeChartData} margin={{ top: 8, right: 8, left: -12, bottom: 0 }}>
                                            <CartesianGrid strokeDasharray="3 3" stroke="#334155" opacity={0.2} vertical={false} />
                                            <XAxis dataKey="name" tick={{ fontSize: 10, fill: '#94a3b8' }} interval={0} angle={-18} textAnchor="end" height={60} />
                                            <YAxis allowDecimals={false} tick={{ fontSize: 10, fill: '#94a3b8' }} />
                                            <Tooltip contentStyle={SIEM_CHART_TT} />
                                            <Bar dataKey="count" fill="#22d3ee" radius={[6, 6, 0, 0]} name="Connectors" />
                                        </BarChart>
                                    </ResponsiveContainer>
                                )}
                            </div>
                        </div>
                        <div className="rounded-xl border border-slate-200/80 dark:border-slate-700/50 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm min-h-[240px] flex flex-col">
                            <h3 className="text-sm font-bold text-slate-900 dark:text-white mb-1">Enabled share</h3>
                            <div className="flex-1 min-h-[180px]">
                                {rows.length === 0 ? (
                                    <div className="h-full flex items-center justify-center text-xs text-slate-500">—</div>
                                ) : (
                                    <ResponsiveContainer width="100%" height="100%">
                                        <PieChart>
                                            <Pie
                                                data={[
                                                    { name: 'Enabled', value: enabledCount },
                                                    { name: 'Disabled', value: Math.max(0, rows.length - enabledCount) },
                                                ]}
                                                dataKey="value"
                                                nameKey="name"
                                                cx="50%"
                                                cy="50%"
                                                innerRadius={50}
                                                outerRadius={72}
                                                paddingAngle={2}
                                            >
                                                <Cell fill="#34d399" />
                                                <Cell fill="#64748b" />
                                            </Pie>
                                            <Tooltip contentStyle={SIEM_CHART_TT} />
                                            <Legend wrapperStyle={{ fontSize: '11px' }} />
                                        </PieChart>
                                    </ResponsiveContainer>
                                )}
                            </div>
                        </div>
                    </div>

                    {canWriteSettings && (
                        <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 shadow-sm p-4 space-y-3">
                            <h3 className="text-sm font-bold text-slate-900 dark:text-white flex items-center gap-2">
                                <Plug className="w-4 h-4 text-cyan-500" />
                                Add connector
                            </h3>
                            <p className="text-xs text-slate-500 dark:text-slate-400">
                                Stored in PostgreSQL via <code className="text-[10px]">POST /api/v1/siem/connectors</code>. Do not paste live secrets in shared sessions; prefer vault
                                references in <code className="text-[10px]">notes</code> and inject tokens at the proxy.
                            </p>
                            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                                <div>
                                    <label className="block text-xs font-semibold text-slate-500 uppercase mb-1">Name</label>
                                    <input
                                        value={name}
                                        onChange={(e) => setName(e.target.value)}
                                        className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-3 py-2 text-sm"
                                        placeholder="Prod Splunk HEC"
                                    />
                                </div>
                                <div>
                                    <label className="block text-xs font-semibold text-slate-500 uppercase mb-1">Type</label>
                                    <select
                                        value={connectorType}
                                        onChange={(e) => setConnectorType(e.target.value as SiemConnector['connector_type'])}
                                        className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-3 py-2 text-sm"
                                    >
                                        {Object.entries(SIEM_TYPE_LABELS).map(([k, lab]) => (
                                            <option key={k} value={k}>
                                                {lab}
                                            </option>
                                        ))}
                                    </select>
                                </div>
                                <div className="md:col-span-2">
                                    <label className="block text-xs font-semibold text-slate-500 uppercase mb-1">Endpoint URL</label>
                                    <input
                                        value={endpointUrl}
                                        onChange={(e) => setEndpointUrl(e.target.value)}
                                        className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-3 py-2 text-sm font-mono"
                                        placeholder="https://splunk.internal:8088/services/collector/event"
                                    />
                                </div>
                                <div className="md:col-span-2">
                                    <label className="block text-xs font-semibold text-slate-500 uppercase mb-1">Notes</label>
                                    <textarea
                                        value={notes}
                                        onChange={(e) => setNotes(e.target.value)}
                                        rows={2}
                                        className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-3 py-2 text-sm"
                                        placeholder="HEC token in vault: splunk/prod/hec …"
                                    />
                                </div>
                                <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-200 md:col-span-2">
                                    <input type="checkbox" checked={enabledDraft} onChange={(e) => setEnabledDraft(e.target.checked)} className="rounded border-slate-300" />
                                    Enabled (inactive connectors stay disabled server-side)
                                </label>
                            </div>
                            <div className="flex justify-end">
                                <button
                                    type="button"
                                    disabled={createM.isPending || !name.trim() || !endpointUrl.trim()}
                                    onClick={() => createM.mutate()}
                                    className="px-4 py-2 rounded-lg text-sm font-semibold bg-cyan-600 hover:bg-cyan-700 text-white disabled:opacity-50"
                                >
                                    {createM.isPending ? 'Saving…' : 'Create connector'}
                                </button>
                            </div>
                        </div>
                    )}

                    {!canWriteSettings && (
                        <div className="rounded-lg border border-amber-200 dark:border-amber-900/40 bg-amber-50/80 dark:bg-amber-950/20 px-3 py-2 text-xs text-amber-900 dark:text-amber-100">
                            Your role can view connectors. Creating or editing requires <code className="text-[10px]">settings:write</code> (admin).
                        </div>
                    )}

                    <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 shadow-sm overflow-hidden">
                        <div className="px-4 py-3 border-b border-slate-200 dark:border-slate-700/60 bg-slate-50/80 dark:bg-slate-900/40">
                            <h3 className="text-sm font-bold text-slate-900 dark:text-white">Configured destinations</h3>
                            <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">Truth source: connection-manager database.</p>
                        </div>
                        <div className="overflow-x-auto">
                            <table className="min-w-full text-left text-sm">
                                <thead className="text-xs uppercase text-slate-500 bg-slate-50 dark:bg-slate-800/80">
                                    <tr>
                                        <th className="px-3 py-2">Name</th>
                                        <th className="px-3 py-2">Type</th>
                                        <th className="px-3 py-2 hidden lg:table-cell">Endpoint</th>
                                        <th className="px-3 py-2">Status</th>
                                        <th className="px-3 py-2">On</th>
                                        <th className="px-3 py-2 text-right">Actions</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {rows.length === 0 ? (
                                        <tr>
                                            <td colSpan={6} className="px-3 py-10 text-center text-slate-500 dark:text-slate-400">
                                                No connectors. Admins can add destinations above.
                                            </td>
                                        </tr>
                                    ) : (
                                        rows.map((r) => (
                                            <tr key={r.id} className="border-t border-slate-100 dark:border-slate-800">
                                                <td className="px-3 py-2 font-medium text-slate-900 dark:text-white">{r.name}</td>
                                                <td className="px-3 py-2 text-xs">{SIEM_TYPE_LABELS[r.connector_type] || r.connector_type}</td>
                                                <td className="px-3 py-2 text-xs font-mono text-slate-600 dark:text-slate-300 hidden lg:table-cell max-w-md truncate" title={r.endpoint_url}>
                                                    {r.endpoint_url}
                                                </td>
                                                <td className="px-3 py-2 text-xs font-mono">{r.status}</td>
                                                <td className="px-3 py-2">
                                                    {canWriteSettings ? (
                                                        <input
                                                            type="checkbox"
                                                            checked={r.enabled}
                                                            disabled={patchM.isPending}
                                                            onChange={() => patchM.mutate({ id: r.id, body: { enabled: !r.enabled } })}
                                                            className="rounded border-slate-300"
                                                        />
                                                    ) : (
                                                        <span className="text-xs">{r.enabled ? 'Yes' : 'No'}</span>
                                                    )}
                                                </td>
                                                <td className="px-3 py-2 text-right">
                                                    {canWriteSettings && (
                                                        <button
                                                            type="button"
                                                            disabled={deleteM.isPending}
                                                            className="text-xs text-rose-600 dark:text-rose-400 hover:underline font-medium"
                                                            onClick={() => {
                                                                if (window.confirm(`Remove connector “${r.name}”?`)) deleteM.mutate(r.id);
                                                            }}
                                                        >
                                                            Delete
                                                        </button>
                                                    )}
                                                </td>
                                            </tr>
                                        ))
                                    )}
                                </tbody>
                            </table>
                        </div>
                    </div>

                    <div className="flex flex-wrap gap-3 text-sm">
                        <Link to="/alerts" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                            Alerts workspace →
                        </Link>
                        <Link to="/events" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                            Telemetry search →
                        </Link>
                        <Link to="/system/audit-logs" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                            Audit logs →
                        </Link>
                    </div>
                </>
            )}
        </div>
    );
}

export function SecurityThreatLabsPage() {
    return (
        <GenericParityView
            title="Threat Labs — IOC feed"
            missingApi="true"
            queryKey={['parity', 'threat-labs', 'iocs']}
            fetcher={() => parityApi.getThreatLabsIocs()}
            mock={mocks.mockThreatLabsIocs.data}
        />
    );
}

function sumMapKeys(m: Record<string, number> | undefined, keys: string[]): number {
    if (!m) return 0;
    let t = 0;
    for (const k of keys) {
        const v = m[k];
        if (typeof v === 'number') t += v;
    }
    return t;
}

/** Normalize Sigma status keys (DB may use snake_case). */
function statusBucket(st: AlertStats['by_status'] | undefined, key: string): number {
    if (!st) return 0;
    const lower = key.toLowerCase();
    let n = 0;
    for (const [k, v] of Object.entries(st)) {
        if (k.toLowerCase() === lower && typeof v === 'number') n += v;
    }
    return n;
}

export function ManagedSecurityOverviewPage() {
    useEffect(() => {
        document.title = 'MDR — Operations overview | EDR Platform';
    }, []);

    const alertStatsQ = useQuery({
        queryKey: ['sigma-stats-alerts', 'managed-overview'],
        queryFn: () => statsApi.alerts(),
        staleTime: 30_000,
        refetchInterval: 60_000,
        retry: 1,
    });

    const ruleStatsQ = useQuery({
        queryKey: ['sigma-stats-rules', 'managed-overview'],
        queryFn: () => statsApi.rules(),
        staleTime: 60_000,
        refetchInterval: 120_000,
        retry: 1,
    });

    const agentsStatsQ = useQuery({
        queryKey: ['agents-stats', 'managed-overview'],
        queryFn: () => agentsApi.stats(),
        staleTime: 60_000,
        refetchInterval: 120_000,
        retry: 1,
    });

    const cmdStatsQ = useQuery({
        queryKey: ['commands-stats', 'managed-overview'],
        queryFn: () => commandsApi.stats(),
        staleTime: 30_000,
        refetchInterval: 60_000,
        retry: 1,
    });

    const riskQ = useQuery({
        queryKey: ['alerts-endpoint-risk', 'managed-overview'],
        queryFn: () => alertsApi.endpointRisk(),
        staleTime: 45_000,
        refetchInterval: 90_000,
        retry: 0,
    });

    const recentQ = useQuery({
        queryKey: ['sigma-alerts', 'managed-overview-recent'],
        queryFn: () => alertsApi.list({ limit: 12, order: 'desc' }),
        staleTime: 20_000,
        refetchInterval: 45_000,
        retry: 1,
    });

    const sevData = useMemo(() => {
        const s = alertStatsQ.data?.by_severity ?? {};
        const order = ['critical', 'high', 'medium', 'low', 'informational'];
        return order
            .map((k) => {
                let count = s[k] ?? 0;
                if (!count && k.length) {
                    const cap = k.charAt(0).toUpperCase() + k.slice(1);
                    count = s[cap] ?? 0;
                }
                return { name: k, count };
            })
            .filter((d) => d.count > 0);
    }, [alertStatsQ.data]);

    const statusBarData = useMemo(() => {
        const st = alertStatsQ.data?.by_status;
        if (!st) return [];
        return Object.entries(st)
            .map(([name, value]) => ({ name, count: value }))
            .filter((d) => d.count > 0)
            .sort((a, b) => b.count - a.count)
            .slice(0, 8);
    }, [alertStatsQ.data]);

    const riskTop = useMemo(() => {
        const rows = riskQ.data?.data ?? [];
        return [...rows].sort((a, b) => (b.peak_risk_score ?? 0) - (a.peak_risk_score ?? 0)).slice(0, 6);
    }, [riskQ.data]);

    if (alertStatsQ.isLoading) {
        return (
            <div className="space-y-4 animate-pulse">
                <div className="h-28 rounded-2xl bg-slate-200 dark:bg-slate-800" />
                <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                    {[...Array(4)].map((_, i) => (
                        <div key={i} className="h-24 rounded-xl bg-slate-200 dark:bg-slate-800" />
                    ))}
                </div>
                <div className="h-48 rounded-xl bg-slate-200 dark:bg-slate-800" />
            </div>
        );
    }

    if (alertStatsQ.isError || !alertStatsQ.data) {
        return (
            <div className="rounded-xl border border-rose-200 dark:border-rose-900/50 bg-rose-50/80 dark:bg-rose-950/20 p-6 text-sm text-rose-900 dark:text-rose-200 space-y-3">
                <p>Could not load Sigma alert statistics (<code className="text-xs">GET /api/v1/sigma/stats/alerts</code>). Confirm the Sigma engine is reachable and the user has alert read access.</p>
                <div className="flex flex-wrap gap-3 text-sm">
                    <Link to="/alerts" className="text-cyan-700 dark:text-cyan-300 font-medium hover:underline">
                        Alerts workspace →
                    </Link>
                    <Link to="/managed-security/incidents" className="text-cyan-700 dark:text-cyan-300 font-medium hover:underline">
                        MDR incident queue →
                    </Link>
                </div>
            </div>
        );
    }

    const st = alertStatsQ.data;
    const byS = st.by_severity ?? {};
    const openBacklog =
        statusBucket(st.by_status, 'open') +
        statusBucket(st.by_status, 'in_progress') +
        sumMapKeys(st.by_status, ['investigating', 'Investigating']);
    const critHigh = (byS.critical ?? 0) + (byS.high ?? 0);
    const medPlus = critHigh + (byS.medium ?? 0);

    return (
        <div className="space-y-6 animate-slide-up-fade w-full min-w-0">
            <InsightHero
                variant="dark"
                accent="violet"
                icon={Shield}
                eyebrow="Managed detection & response"
                title="Operations overview"
                lead={
                    <>
                        Program-level view for <strong className="text-white">supervised SOC delivery</strong>: Sigma aggregates from{' '}
                        <code className="text-[11px] text-violet-200/95 bg-white/10 px-1 rounded">/sigma/stats/alerts</code> (true totals, not a truncated list),
                        fleet context from connection-manager, response-queue pressure from{' '}
                        <code className="text-[11px] text-violet-200/95 bg-white/10 px-1 rounded">/commands/stats</code>, and a short{' '}
                        <strong className="text-white">endpoint-risk excerpt</strong> for prioritization — without replacing the full Alerts grid or deep incident workflows.
                    </>
                }
            />

            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <Zap className="w-4 h-4 text-amber-500" />
                        vs Alerts workspace
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/alerts">
                            /alerts
                        </Link>{' '}
                        is triage, filters, and bulk actions. Here you only see <strong>aggregates and a 12-row pulse</strong>.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <BarChart3 className="w-4 h-4 text-cyan-500" />
                        vs Service Summary
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/dashboards/service">
                            Service Summary
                        </Link>{' '}
                        broadens to reliability + platform KPIs. This page stays on <strong>threat queue + response + fleet touchpoints</strong>.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <Terminal className="w-4 h-4 text-slate-500" />
                        vs Command Center
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/responses">
                            Command Center
                        </Link>{' '}
                        (<code className="text-[10px]">/responses</code>) is command history and execution. Here we surface <strong>pending/failed command totals</strong> as a backlog signal only.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <TrendingUp className="w-4 h-4 text-rose-500" />
                        vs Endpoint Risk
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/endpoint-risk">
                            Endpoint Risk
                        </Link>{' '}
                        is the full ranked board. Below is a <strong>top-6 managed excerpt</strong> for the same index.
                    </p>
                </div>
            </div>

            <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                <StatCard
                    title="Open backlog"
                    value={String(openBacklog)}
                    icon={Shield}
                    color="amber"
                    subtext="open + in_progress (+ investigating if present)"
                />
                <StatCard title="Critical + High" value={String(critHigh)} icon={AlertTriangle} color="red" subtext="From Sigma by_severity" />
                <StatCard title="New (24h)" value={String(st.last_24h)} icon={Activity} color="cyan" subtext={`7d: ${st.last_7d} · engine total ${st.total_alerts}`} />
                <StatCard
                    title="Avg confidence"
                    value={st.avg_confidence != null ? `${((st.avg_confidence || 0) * 100).toFixed(1)}%` : '—'}
                    icon={Eye}
                    color="emerald"
                    subtext="Sigma aggregate (0–1 scaled to %)"
                />
            </div>

            <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                <StatCard
                    title="Protected endpoints"
                    value={agentsStatsQ.data ? String(agentsStatsQ.data.total) : '—'}
                    icon={Fingerprint}
                    subtext={agentsStatsQ.isError ? 'stats unavailable' : 'Registry total (/agents/stats)'}
                />
                <StatCard
                    title="Rules enabled"
                    value={ruleStatsQ.data ? `${ruleStatsQ.data.enabled_rules} / ${ruleStatsQ.data.total_rules}` : '—'}
                    icon={Layers}
                    subtext={ruleStatsQ.isError ? 'rule stats unavailable' : 'Sigma /sigma/stats/rules'}
                />
                <StatCard
                    title="Commands pending"
                    value={cmdStatsQ.data != null ? String(cmdStatsQ.data.pending) : '—'}
                    icon={Clock}
                    color="amber"
                    subtext={
                        cmdStatsQ.data != null
                            ? `failed ${cmdStatsQ.data.failed} · completed ${cmdStatsQ.data.completed}`
                            : cmdStatsQ.isError
                              ? 'commands:read?'
                              : '…'
                    }
                />
                <StatCard title="Med+ severity (Σ)" value={String(medPlus)} icon={Radio} subtext="critical+high+medium" />
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 p-4 shadow-sm">
                    <h3 className="text-xs font-bold uppercase tracking-widest text-slate-400 mb-3">Severity distribution (engine)</h3>
                    {sevData.length === 0 ? (
                        <div className="h-36 flex items-center justify-center text-sm text-slate-500">No severity buckets</div>
                    ) : (
                        <ResponsiveContainer width="100%" height={200}>
                            <BarChart data={sevData} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                                <CartesianGrid strokeDasharray="3 3" stroke="rgba(100,116,139,0.15)" vertical={false} />
                                <XAxis dataKey="name" tick={{ fontSize: 11, fill: '#94a3b8' }} />
                                <YAxis allowDecimals={false} tick={{ fontSize: 11, fill: '#94a3b8' }} />
                                <Tooltip
                                    contentStyle={ZT_CHART_TOOLTIP}
                                    formatter={(v: number | undefined) => [`${v ?? 0} alerts`, 'Count']}
                                />
                                <Bar dataKey="count" radius={[6, 6, 0, 0]} maxBarSize={48}>
                                    {sevData.map((e, i) => (
                                        <Cell
                                            key={i}
                                            fill={
                                                e.name === 'critical'
                                                    ? '#dc2626'
                                                    : e.name === 'high'
                                                      ? '#f97316'
                                                      : e.name === 'medium'
                                                        ? '#eab308'
                                                        : e.name === 'low'
                                                          ? '#22c55e'
                                                          : '#64748b'
                                            }
                                        />
                                    ))}
                                </Bar>
                            </BarChart>
                        </ResponsiveContainer>
                    )}
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 p-4 shadow-sm">
                    <h3 className="text-xs font-bold uppercase tracking-widest text-slate-400 mb-3">Status distribution (top buckets)</h3>
                    {statusBarData.length === 0 ? (
                        <div className="h-36 flex items-center justify-center text-sm text-slate-500">No status breakdown</div>
                    ) : (
                        <ResponsiveContainer width="100%" height={200}>
                            <BarChart data={statusBarData} layout="vertical" margin={{ left: 8, right: 16, top: 8, bottom: 8 }}>
                                <CartesianGrid strokeDasharray="3 3" stroke="rgba(100,116,139,0.15)" horizontal={false} />
                                <XAxis type="number" allowDecimals={false} tick={{ fontSize: 11, fill: '#94a3b8' }} />
                                <YAxis type="category" dataKey="name" width={100} tick={{ fontSize: 10, fill: '#94a3b8' }} />
                                <Tooltip
                                    contentStyle={ZT_CHART_TOOLTIP}
                                    formatter={(v: number | undefined) => [`${v ?? 0} alerts`, 'Count']}
                                />
                                <Bar dataKey="count" fill="#8b5cf6" radius={[0, 6, 6, 0]} maxBarSize={22} name="Alerts" />
                            </BarChart>
                        </ResponsiveContainer>
                    )}
                </div>
            </div>

            <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 shadow-sm overflow-hidden">
                    <div className="px-4 py-3 border-b border-slate-100 dark:border-slate-800 flex flex-wrap items-center justify-between gap-2">
                        <h3 className="text-sm font-bold text-slate-900 dark:text-white">Recent Sigma detections</h3>
                        <span className="text-[11px] text-slate-500 dark:text-slate-400">
                            Live API · 12 most recent · <code className="text-[10px] font-mono">GET /api/v1/sigma/alerts</code> · full filterable queue in Incident queue
                        </span>
                    </div>
                    {recentQ.isLoading ? (
                        <div className="h-40 animate-pulse bg-slate-100 dark:bg-slate-900/50" />
                    ) : recentQ.isError ? (
                        <div className="p-4 text-xs text-amber-800 dark:text-amber-200">Could not load recent alerts list.</div>
                    ) : (
                        <div className="overflow-x-auto">
                            <table className="min-w-full text-left text-sm">
                                <thead className="bg-slate-50 dark:bg-slate-900/50 text-slate-500 dark:text-slate-400 text-xs uppercase">
                                    <tr>
                                        <th className="px-3 py-2">Time</th>
                                        <th className="px-3 py-2">Severity</th>
                                        <th className="px-3 py-2">Rule</th>
                                        <th className="px-3 py-2">Agent</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {(recentQ.data?.alerts ?? []).map((a) => (
                                        <tr key={a.id} className="border-t border-slate-100 dark:border-slate-800">
                                            <td className="px-3 py-2 text-xs whitespace-nowrap text-slate-600 dark:text-slate-300">
                                                {new Date(a.timestamp || a.created_at).toLocaleString()}
                                            </td>
                                            <td className="px-3 py-2 text-xs font-mono">{a.severity || '—'}</td>
                                            <td className="px-3 py-2 text-slate-800 dark:text-slate-100">{a.rule_title || a.rule_id || 'Alert'}</td>
                                            <td className="px-3 py-2">
                                                {a.agent_id ? (
                                                    <Link
                                                        className="text-cyan-600 dark:text-cyan-400 hover:underline font-mono text-xs"
                                                        to={`/management/devices/${encodeURIComponent(a.agent_id)}?tab=activity`}
                                                    >
                                                        {a.agent_id.slice(0, 8)}…
                                                    </Link>
                                                ) : (
                                                    '—'
                                                )}
                                            </td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    )}
                    <div className="px-4 py-2 border-t border-slate-100 dark:border-slate-800 bg-slate-50/50 dark:bg-slate-900/30">
                        <Link to="/managed-security/incidents" className="text-sm text-cyan-600 dark:text-cyan-400 font-medium hover:underline">
                            Open incident queue →
                        </Link>
                    </div>
                </div>

                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 shadow-sm overflow-hidden">
                    <div className="px-4 py-3 border-b border-slate-100 dark:border-slate-800 flex flex-wrap items-center justify-between gap-2">
                        <h3 className="text-sm font-bold text-slate-900 dark:text-white">Endpoint risk excerpt</h3>
                        <span className="text-[11px] text-slate-500">Top 6 by peak score</span>
                    </div>
                    {riskQ.isLoading ? (
                        <div className="h-40 animate-pulse bg-slate-100 dark:bg-slate-900/50" />
                    ) : riskQ.isError ? (
                        <div className="p-4 text-xs text-amber-800 dark:text-amber-200">
                            Risk index unavailable — confirm <code className="text-[10px]">alerts:read</code> for{' '}
                            <code className="text-[10px]">/api/v1/alerts/endpoint-risk</code>.
                        </div>
                    ) : riskTop.length === 0 ? (
                        <div className="p-8 text-center text-sm text-slate-500">No endpoint risk rows returned.</div>
                    ) : (
                        <div className="overflow-x-auto">
                            <table className="min-w-full text-left text-sm">
                                <thead className="bg-slate-50 dark:bg-slate-900/50 text-slate-500 dark:text-slate-400 text-xs uppercase">
                                    <tr>
                                        <th className="px-3 py-2">Agent</th>
                                        <th className="px-3 py-2">Peak risk</th>
                                        <th className="px-3 py-2">Open</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {riskTop.map((r: EndpointRiskSummary) => (
                                        <tr key={r.agent_id} className="border-t border-slate-100 dark:border-slate-800">
                                            <td className="px-3 py-2">
                                                <Link
                                                    className="text-cyan-600 dark:text-cyan-400 font-mono text-xs hover:underline"
                                                    to={`/management/devices/${encodeURIComponent(r.agent_id)}`}
                                                >
                                                    {r.agent_id.slice(0, 8)}…
                                                </Link>
                                            </td>
                                            <td className="px-3 py-2 font-mono text-xs">{r.peak_risk_score ?? '—'}</td>
                                            <td className="px-3 py-2 font-mono text-xs">{r.open_count ?? '—'}</td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    )}
                    <div className="px-4 py-2 border-t border-slate-100 dark:border-slate-800 bg-slate-50/50 dark:bg-slate-900/30">
                        <Link to="/endpoint-risk" className="text-sm text-cyan-600 dark:text-cyan-400 font-medium hover:underline">
                            Full endpoint risk board →
                        </Link>
                    </div>
                </div>
            </div>

            <div className="flex flex-wrap gap-4 text-sm">
                <Link to="/alerts" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                    Alerts workspace →
                </Link>
                <Link to="/events" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                    Telemetry search →
                </Link>
                <Link to="/responses" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                    Command Center →
                </Link>
            </div>
        </div>
    );
}

function managedIncidentSeverityClass(sev: string | undefined): string {
    switch (sev) {
        case 'critical':
            return 'text-rose-700 dark:text-rose-300 font-bold';
        case 'high':
            return 'text-orange-700 dark:text-orange-300 font-semibold';
        case 'medium':
            return 'text-amber-700 dark:text-amber-300 font-medium';
        case 'low':
            return 'text-emerald-700 dark:text-emerald-300';
        default:
            return 'text-slate-600 dark:text-slate-400';
    }
}

const MANAGED_INCIDENTS_PAGE_SIZE = 25;

export function ManagedSecurityIncidentsPage() {
    useEffect(() => {
        document.title = 'MDR — Incident queue | EDR Platform';
    }, []);

    const queryClient = useQueryClient();
    const [page, setPage] = useState(1);
    const [searchInput, setSearchInput] = useState('');
    const [debouncedSearch, setDebouncedSearch] = useState('');
    const [severityFilter, setSeverityFilter] = useState<string>('');
    const [statusPreset, setStatusPreset] = useState<'all' | 'active' | 'open' | 'closed'>('active');
    const [sortKey, setSortKey] = useState<'risk' | 'time' | 'sev'>('risk');

    useEffect(() => {
        const t = window.setTimeout(() => setDebouncedSearch(searchInput.trim()), 400);
        return () => window.clearTimeout(t);
    }, [searchInput]);

    useEffect(() => {
        setPage(1);
    }, [debouncedSearch, severityFilter, statusPreset, sortKey]);

    const statsQ = useQuery({
        queryKey: ['sigma-stats-alerts', 'managed-incidents-kpi'],
        queryFn: () => statsApi.alerts(),
        staleTime: 30_000,
        refetchInterval: 60_000,
        retry: 1,
    });

    const agentsMapQ = useQuery({
        queryKey: ['agents', 'hostnames', 'managed-incidents'],
        queryFn: () => agentsApi.list({ limit: 500, sort_by: 'hostname', sort_order: 'asc' }),
        staleTime: 60_000,
        retry: 1,
    });

    const listQ = useQuery({
        queryKey: [
            'sigma-alerts',
            'managed-incidents',
            page,
            MANAGED_INCIDENTS_PAGE_SIZE,
            debouncedSearch,
            severityFilter,
            statusPreset,
            sortKey,
        ],
        queryFn: () => {
            let status: string | undefined;
            if (statusPreset === 'active') status = 'open,in_progress';
            else if (statusPreset === 'open') status = 'open';
            else if (statusPreset === 'closed') status = 'resolved,closed,false_positive';
            const sort = sortKey === 'time' ? '-timestamp' : sortKey === 'sev' ? '-severity' : '-risk_score';
            return alertsApi.list({
                limit: MANAGED_INCIDENTS_PAGE_SIZE,
                offset: (page - 1) * MANAGED_INCIDENTS_PAGE_SIZE,
                search: debouncedSearch || undefined,
                severity: severityFilter || undefined,
                status,
                sort,
            });
        },
        staleTime: 10_000,
        refetchInterval: 25_000,
        retry: 1,
    });

    useEffect(() => {
        if (!listQ.isSuccess || listQ.data == null) return;
        const tp = Math.max(1, Math.ceil(Number(listQ.data.total) / MANAGED_INCIDENTS_PAGE_SIZE));
        if (page > tp) setPage(tp);
    }, [listQ.isSuccess, listQ.data?.total, page]);

    const agentHostnameMap = useMemo(() => {
        const m: Record<string, string> = {};
        for (const a of agentsMapQ.data?.data ?? []) {
            m[a.id] = a.hostname;
        }
        return m;
    }, [agentsMapQ.data]);

    const st = statsQ.data;
    const openBacklogKpi = st
        ? statusBucket(st.by_status, 'open') +
          statusBucket(st.by_status, 'in_progress') +
          sumMapKeys(st.by_status, ['investigating', 'Investigating'])
        : null;
    const bySev = st?.by_severity ?? {};
    const critHighKpi = (bySev.critical ?? 0) + (bySev.high ?? 0);

    const total = listQ.data?.total ?? 0;
    const rows = listQ.data?.alerts ?? [];
    const totalPages = Math.max(1, Math.ceil(Number(total) / MANAGED_INCIDENTS_PAGE_SIZE));
    const apiOffset = listQ.data?.offset ?? (page - 1) * MANAGED_INCIDENTS_PAGE_SIZE;
    const displayPage = rows.length ? Math.floor(apiOffset / MANAGED_INCIDENTS_PAGE_SIZE) + 1 : Math.min(page, totalPages);
    const fromIdx = total === 0 || rows.length === 0 ? 0 : apiOffset + 1;
    const toIdx = total === 0 || rows.length === 0 ? 0 : apiOffset + rows.length;

    const refresh = () => {
        void queryClient.invalidateQueries({ queryKey: ['sigma-alerts', 'managed-incidents'] });
        void queryClient.invalidateQueries({ queryKey: ['sigma-stats-alerts', 'managed-incidents-kpi'] });
    };

    return (
        <div className="space-y-6 animate-slide-up-fade w-full min-w-0">
            <InsightHero
                variant="dark"
                accent="fuchsia"
                icon={ListFilter}
                eyebrow="Managed queue"
                title="Incident queue"
                lead={
                    <>
                        Operational <strong className="text-white">Sigma-backed incident queue</strong> with server-side filters,{' '}
                        <code className="text-[11px] text-fuchsia-200/95 bg-white/10 px-1 rounded">COUNT + LIMIT/OFFSET</code> from{' '}
                        <code className="text-[11px] text-fuchsia-200/95 bg-white/10 px-1 rounded">GET /api/v1/sigma/alerts</code>, and hostnames from connection-manager when available.
                        KPIs above the grid use{' '}
                        <code className="text-[11px] text-fuchsia-200/95 bg-white/10 px-1 rounded">/sigma/stats/alerts</code> so headline posture stays accurate even while you page deep into history.
                    </>
                }
            />

            <div className="grid gap-3 md:grid-cols-3">
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <Zap className="w-4 h-4 text-amber-500" />
                        vs Alerts workspace
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/alerts">
                            /alerts
                        </Link>{' '}
                        adds streaming, drawer triage, and bulk status writes. This route is a <strong>managed-service style backlog</strong> with stable paging.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <BarChart3 className="w-4 h-4 text-cyan-500" />
                        vs Operations overview
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/managed-security/overview">
                            Operations overview
                        </Link>{' '}
                        is charts + excerpts. Here you work the <strong>full filterable list</strong> with correct totals.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <Radio className="w-4 h-4 text-violet-500" />
                        vs Endpoint / risk boards
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/endpoint-risk">
                            Endpoint risk
                        </Link>{' '}
                        ranks hosts. This table stays <strong>alert-centric</strong> (one row per detection).
                    </p>
                </div>
            </div>

            <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                <StatCard
                    title="Open backlog (Sigma)"
                    value={statsQ.isLoading ? '…' : openBacklogKpi != null ? String(openBacklogKpi) : '—'}
                    icon={Shield}
                    color="amber"
                    subtext="open + in_progress (+ investigating if present)"
                />
                <StatCard
                    title="Critical + High (engine)"
                    value={statsQ.isLoading ? '…' : String(critHighKpi)}
                    icon={AlertTriangle}
                    color="red"
                    subtext="From /sigma/stats/alerts"
                />
                <StatCard
                    title="Total in engine"
                    value={statsQ.isLoading ? '…' : st ? String(st.total_alerts) : '—'}
                    icon={Activity}
                    color="cyan"
                    subtext="All-time count in Sigma DB"
                />
                <StatCard
                    title="Matching (this view)"
                    value={listQ.isLoading && !listQ.data ? '…' : String(total)}
                    icon={Layers}
                    subtext="Respects filters below · server total"
                />
            </div>

            {agentsMapQ.isError && (
                <div className="rounded-lg border border-amber-200 dark:border-amber-800 bg-amber-50 dark:bg-amber-950/30 px-4 py-2 text-xs text-amber-900 dark:text-amber-200">
                    Hostname enrichment unavailable (agents list). Agent column falls back to short IDs.
                </div>
            )}

            <div className="flex flex-col gap-3 lg:flex-row lg:flex-wrap lg:items-end">
                <div className="relative flex-1 min-w-[200px] max-w-md">
                    <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
                    <input
                        type="search"
                        value={searchInput}
                        onChange={(e) => setSearchInput(e.target.value)}
                        placeholder="Search rule title, rule id, agent id…"
                        className="w-full pl-9 pr-3 py-2 rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-sm text-slate-900 dark:text-white placeholder:text-slate-400"
                    />
                </div>
                <div className="flex flex-wrap gap-2">
                    <select
                        value={statusPreset}
                        onChange={(e) => setStatusPreset(e.target.value as typeof statusPreset)}
                        className="px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-sm text-slate-900 dark:text-white"
                    >
                        <option value="active">Status: Active backlog</option>
                        <option value="open">Status: Open only</option>
                        <option value="closed">Status: Closed / resolved / FP</option>
                        <option value="all">Status: All</option>
                    </select>
                    <select
                        value={severityFilter}
                        onChange={(e) => setSeverityFilter(e.target.value)}
                        className="px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-sm text-slate-900 dark:text-white"
                    >
                        <option value="">Severity: Any</option>
                        <option value="critical">Critical</option>
                        <option value="high">High</option>
                        <option value="critical,high">Critical + High</option>
                        <option value="medium">Medium</option>
                        <option value="low">Low</option>
                    </select>
                    <select
                        value={sortKey}
                        onChange={(e) => setSortKey(e.target.value as typeof sortKey)}
                        className="px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-sm text-slate-900 dark:text-white"
                    >
                        <option value="risk">Sort: Risk score (desc)</option>
                        <option value="time">Sort: Time (newest)</option>
                        <option value="sev">Sort: Severity (desc)</option>
                    </select>
                    <button
                        type="button"
                        onClick={() => refresh()}
                        className="px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-sm font-medium text-slate-700 dark:text-slate-200 hover:bg-slate-50 dark:hover:bg-slate-700/80 transition-colors"
                    >
                        Refresh
                    </button>
                </div>
            </div>

            {listQ.isError && (
                <div className="rounded-xl border border-rose-200 dark:border-rose-900/50 bg-rose-50/80 dark:bg-rose-950/20 p-6 text-sm text-rose-900 dark:text-rose-200">
                    Could not load incidents from Sigma. Confirm <code className="text-xs">/api/v1/sigma/alerts</code> and alert read permissions.
                </div>
            )}

            {!listQ.isError && (
                <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm shadow-sm">
                    <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-2 border-b border-slate-100 dark:border-slate-800 text-xs text-slate-500">
                        <span>
                            {total === 0
                                ? 'No rows for current filters'
                                : `Showing ${fromIdx}–${toIdx} of ${total}`}
                        </span>
                        <span className="font-mono text-[10px] text-slate-400">limit={MANAGED_INCIDENTS_PAGE_SIZE} offset={apiOffset}</span>
                    </div>
                    <table className="min-w-full text-left text-sm">
                        <thead className="bg-slate-50 dark:bg-slate-900/50 text-slate-500 dark:text-slate-400 text-xs uppercase">
                            <tr>
                                <th className="px-3 py-2.5">Time</th>
                                <th className="px-3 py-2.5">Severity</th>
                                <th className="px-3 py-2.5">Status</th>
                                <th className="px-3 py-2.5 text-right">Risk</th>
                                <th className="px-3 py-2.5">Rule</th>
                                <th className="px-3 py-2.5">Category</th>
                                <th className="px-3 py-2.5">Host</th>
                                <th className="px-3 py-2.5">Actions</th>
                            </tr>
                        </thead>
                        <tbody>
                            {listQ.isLoading && !listQ.data ? (
                                <tr>
                                    <td colSpan={8} className="px-3 py-12 text-center text-slate-500">
                                        <Loader2 className="w-6 h-6 animate-spin inline text-slate-400" />
                                    </td>
                                </tr>
                            ) : rows.length === 0 ? (
                                <tr>
                                    <td colSpan={8} className="px-3 py-10 text-center text-slate-500">
                                        No incidents for these filters.
                                    </td>
                                </tr>
                            ) : (
                                rows.map((a: Alert) => {
                                    const host = a.agent_id ? agentHostnameMap[a.agent_id] || `${a.agent_id.slice(0, 8)}…` : '—';
                                    return (
                                        <tr key={a.id} className="border-t border-slate-100 dark:border-slate-800 hover:bg-slate-50/80 dark:hover:bg-slate-800/50">
                                            <td className="px-3 py-2.5 text-xs whitespace-nowrap text-slate-600 dark:text-slate-300">
                                                {new Date(a.timestamp || a.created_at).toLocaleString()}
                                            </td>
                                            <td className={`px-3 py-2.5 text-xs font-mono capitalize ${managedIncidentSeverityClass(a.severity)}`}>
                                                {a.severity || '—'}
                                            </td>
                                            <td className="px-3 py-2.5 text-xs font-mono text-slate-600 dark:text-slate-300">{a.status || '—'}</td>
                                            <td className="px-3 py-2.5 text-xs font-mono text-right text-slate-700 dark:text-slate-200">
                                                {a.risk_score != null ? Math.round(a.risk_score) : '—'}
                                            </td>
                                            <td className="px-3 py-2.5 text-slate-800 dark:text-slate-100 max-w-[220px]">
                                                <div className="truncate font-medium" title={a.rule_title || a.rule_id}>
                                                    {a.rule_title || a.rule_id || 'Alert'}
                                                </div>
                                                <div className="text-[10px] font-mono text-slate-400 truncate">{a.id}</div>
                                            </td>
                                            <td className="px-3 py-2.5 text-xs text-slate-500 max-w-[140px] truncate" title={a.category}>
                                                {a.category || '—'}
                                            </td>
                                            <td className="px-3 py-2.5 text-xs">
                                                {a.agent_id ? (
                                                    <Link
                                                        className="text-cyan-600 dark:text-cyan-400 hover:underline"
                                                        to={`/management/devices/${encodeURIComponent(a.agent_id)}?tab=activity`}
                                                    >
                                                        {host}
                                                    </Link>
                                                ) : (
                                                    '—'
                                                )}
                                            </td>
                                            <td className="px-3 py-2.5 text-xs whitespace-nowrap">
                                                <Link
                                                    to="/alerts"
                                                    className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline"
                                                >
                                                    Triage
                                                </Link>
                                            </td>
                                        </tr>
                                    );
                                })
                            )}
                        </tbody>
                    </table>
                    {totalPages > 1 && (
                        <div className="flex items-center justify-between px-4 py-2 border-t border-slate-100 dark:border-slate-800 text-xs text-slate-500">
                            <span>
                                Page {displayPage} of {totalPages}
                            </span>
                            <div className="flex gap-1">
                                <button
                                    type="button"
                                    disabled={displayPage <= 1}
                                    onClick={() => setPage((p) => Math.max(1, p - 1))}
                                    className="p-1.5 rounded-lg border border-slate-300 dark:border-slate-600 hover:bg-slate-100 dark:hover:bg-slate-800 disabled:opacity-30"
                                >
                                    <ChevronLeft className="w-4 h-4" />
                                </button>
                                <button
                                    type="button"
                                    disabled={displayPage >= totalPages}
                                    onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                                    className="p-1.5 rounded-lg border border-slate-300 dark:border-slate-600 hover:bg-slate-100 dark:hover:bg-slate-800 disabled:opacity-30"
                                >
                                    <ChevronRight className="w-4 h-4" />
                                </button>
                            </div>
                        </div>
                    )}
                </div>
            )}

            <p className="text-xs text-slate-500 dark:text-slate-400">
                For acknowledge/resolve/bulk workflows use the{' '}
                <Link to="/alerts" className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline">
                    Alerts workspace
                </Link>
                . Hostnames resolve from the first 500 agents by hostname (same enrichment pattern as Alerts).
            </p>
        </div>
    );
}

export function ManagedSecuritySlaPage() {
    return (
        <GenericParityView
            title="MDR — SLA"
            missingApi="true"
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
            missingApi="true"
            queryKey={['parity', 'itsm', 'tickets']}
            fetcher={() => parityApi.getItsmTickets()}
            mock={mocks.mockItsmTickets}
        />
    );
}

export function ItsmPlaybooksPage() {
    const queryClient = useQueryClient();
    const { showToast } = useToast();
    const canExec = authApi.canExecuteCommands();
    const canSeeRuns = authApi.canViewResponses();
    const [agentId, setAgentId] = React.useState('');
    const [killName, setKillName] = React.useState('notepad.exe');
    const [domain, setDomain] = React.useState('example.com');
    const [ip, setIp] = React.useState('1.2.3.4');

    const agentTrim = agentId.trim();
    const agentUuidOk = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(agentTrim);

    useEffect(() => {
        document.title = 'Response Playbooks · Orchestration';
    }, []);

    /** Optional Sigma parity catalog — often 404 in self-hosted builds. */
    const catalogQ = useQuery({
        queryKey: ['itsm', 'playbooks', 'catalog'],
        queryFn: () => parityApi.getItsmPlaybooks(),
        retry: 0,
        staleTime: 120_000,
    });

    const runsQ = useQuery({
        queryKey: ['incident', 'playbook-runs', agentTrim],
        queryFn: () => incidentApi.listRuns(agentTrim, 10),
        enabled: canSeeRuns && agentUuidOk,
        staleTime: 10_000,
    });

    const exec = useMutation({
        mutationFn: async (req: { command_type: CommandType; parameters?: Record<string, string>; timeout?: number }) => {
            const aid = agentId.trim();
            if (!aid) throw new Error('agent_id is required');
            return agentsApi.executeCommand(aid, {
                command_type: req.command_type,
                parameters: req.parameters ?? {},
                timeout: req.timeout ?? 300,
            });
        },
        onSuccess: (d) => {
            showToast(`Queued (${d.command_id})`, 'success');
            queryClient.invalidateQueries({ queryKey: ['commands'] });
            queryClient.invalidateQueries({ queryKey: ['agent-commands', agentTrim] });
            queryClient.invalidateQueries({ queryKey: ['incident', 'playbook-runs', agentTrim] });
        },
        onError: (e: Error) => showToast(e.message || 'Failed', 'error'),
    });

    const PlaybookCard = ({
        title,
        description,
        onRun,
        disabled,
        children,
    }: {
        title: string;
        description: string;
        onRun: () => void;
        disabled?: boolean;
        children?: React.ReactNode;
    }) => (
        <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm shadow-sm p-4 space-y-2">
            <div className="font-semibold text-slate-900 dark:text-white">{title}</div>
            <div className="text-xs text-slate-500 dark:text-slate-400">{description}</div>
            {children}
            <div className="flex justify-end">
                <button
                    type="button"
                    disabled={disabled || !canExec || exec.isPending}
                    onClick={onRun}
                    className="px-3 py-2 rounded-lg text-sm font-semibold bg-cyan-600 hover:bg-cyan-700 text-white disabled:opacity-50"
                >
                    {exec.isPending ? 'Running…' : 'Run playbook'}
                </button>
            </div>
        </div>
    );

    const catalogRows = useMemo(() => extractPlaybookCatalogRows(catalogQ.data), [catalogQ.data]);

    return (
        <div className="w-full min-w-0 space-y-6 animate-slide-up-fade">
            <header className="rounded-2xl border border-slate-200/90 dark:border-slate-700/80 bg-gradient-to-br from-slate-50 via-white to-cyan-50/40 dark:from-slate-900/80 dark:via-slate-900/50 dark:to-cyan-950/20 px-5 py-6 sm:px-8 sm:py-8 w-full">
                <div className="flex flex-col lg:flex-row lg:items-start gap-5 lg:gap-8 w-full min-w-0">
                    <div className="flex gap-4 min-w-0 shrink-0">
                        <div className="shrink-0 p-3 rounded-xl border border-cyan-500/25 bg-cyan-500/10 text-cyan-700 dark:text-cyan-300">
                            <BookOpen className="w-7 h-7" aria-hidden />
                        </div>
                        <div className="min-w-0 flex-1">
                            <h1 className="text-2xl sm:text-[1.65rem] font-bold tracking-tight text-slate-900 dark:text-white">
                                Response playbooks
                            </h1>
                            <p className="text-[15px] sm:text-base text-slate-600 dark:text-slate-400 mt-2 leading-relaxed">
                                Guided <strong className="font-semibold text-slate-800 dark:text-slate-200">command sequences</strong> for common
                                incident-response steps. Everything here triggers real work on agents — not simulations.
                            </p>
                        </div>
                    </div>
                </div>

                <div className="mt-6 grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4 w-full">
                    <div className="rounded-xl border border-slate-200/90 dark:border-slate-700 bg-white/75 dark:bg-slate-950/40 px-4 py-4 shadow-sm">
                        <h2 className="text-[11px] font-bold uppercase tracking-wider text-cyan-700 dark:text-cyan-400 mb-2">
                            Command pipeline
                        </h2>
                        <p className="text-sm text-slate-600 dark:text-slate-400 leading-relaxed">
                            Each control issues jobs through{' '}
                            <code className="text-[10px] font-mono px-1 py-0.5 rounded bg-slate-200/90 dark:bg-slate-800">
                                POST /api/v1/agents/:id/commands
                            </code>{' '}
                            on the connection manager — the same pipeline as{' '}
                            <Link className="text-cyan-600 dark:text-cyan-400 font-semibold hover:underline" to="/responses">
                                Command Center
                            </Link>
                            , with opinionated defaults on this page.
                        </p>
                    </div>
                    <div className="rounded-xl border border-slate-200/90 dark:border-slate-700 bg-white/75 dark:bg-slate-950/40 px-4 py-4 shadow-sm">
                        <h2 className="text-[11px] font-bold uppercase tracking-wider text-cyan-700 dark:text-cyan-400 mb-2">
                            Multi-step automation
                        </h2>
                        <p className="text-sm text-slate-600 dark:text-slate-400 leading-relaxed">
                            For ordered chains (for example isolate → collect), use{' '}
                            <Link className="text-cyan-600 dark:text-cyan-400 font-semibold hover:underline" to="/itsm/automations">
                                Response Automations
                            </Link>
                            .
                        </p>
                    </div>
                    <div className="rounded-xl border border-slate-200/90 dark:border-slate-700 bg-white/75 dark:bg-slate-950/40 px-4 py-4 shadow-sm">
                        <h2 className="text-[11px] font-bold uppercase tracking-wider text-cyan-700 dark:text-cyan-400 mb-2">
                            Recorded playbook runs
                        </h2>
                        <p className="text-sm text-slate-600 dark:text-slate-400 leading-relaxed">
                            After you enter an agent UUID below, post-isolation and engine runs stored in the database appear in the table —
                            loaded via{' '}
                            <code className="text-[10px] font-mono px-1 py-0.5 rounded bg-slate-200/90 dark:bg-slate-800">
                                GET …/playbook-runs
                            </code>{' '}
                            (aligned with the Incident tab on the endpoint).
                        </p>
                    </div>
                    <div className="rounded-xl border border-slate-200/90 dark:border-slate-700 bg-white/75 dark:bg-slate-950/40 px-4 py-4 shadow-sm">
                        <h2 className="text-[11px] font-bold uppercase tracking-wider text-slate-500 dark:text-slate-400 mb-2">
                            Scope &amp; fleet
                        </h2>
                        <p className="text-sm text-slate-600 dark:text-slate-400 leading-relaxed">
                            This is <strong className="font-medium text-slate-800 dark:text-slate-200">not</strong> the MDR SLA dashboard — see{' '}
                            <Link to="/managed-security/overview" className="text-cyan-600 dark:text-cyan-400 font-semibold hover:underline">
                                MDR operations overview
                            </Link>
                            . Copy agent UUIDs from{' '}
                            <Link to="/management/devices" className="text-cyan-600 dark:text-cyan-400 font-semibold hover:underline">
                                Fleet
                            </Link>
                            . For arbitrary commands outside these templates, use Command Center.
                        </p>
                    </div>
                </div>
            </header>

            {catalogQ.isPending ? (
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/80 dark:bg-slate-800/40 p-6 animate-pulse">
                    <div className="h-4 w-48 rounded bg-slate-200 dark:bg-slate-700 mb-4" />
                    <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                        {[1, 2, 3].map((i) => (
                            <div key={i} className="h-28 rounded-lg bg-slate-100 dark:bg-slate-800/80" />
                        ))}
                    </div>
                </div>
            ) : null}

            {catalogQ.isSuccess && catalogRows.length > 0 && (
                <section className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 shadow-sm overflow-hidden">
                    <div className="px-5 py-4 border-b border-slate-200 dark:border-slate-700 flex flex-col sm:flex-row sm:items-start sm:justify-between gap-3 bg-slate-50/80 dark:bg-slate-900/40">
                        <div className="min-w-0">
                            <h2 className="text-sm font-semibold text-slate-900 dark:text-white">Sigma playbook catalog</h2>
                            <p className="text-xs text-slate-500 dark:text-slate-400 mt-1 leading-relaxed max-w-[min(100%,42rem)]">
                                Optional definitions returned by{' '}
                                <code className="text-[10px] font-mono px-1 rounded bg-slate-200/80 dark:bg-slate-800">
                                    GET /api/v1/itsm/playbooks
                                </code>
                                . Use them as reference metadata; execution on endpoints still flows through the templates and Command Center
                                below.
                            </p>
                        </div>
                        <button
                            type="button"
                            onClick={() => catalogQ.refetch()}
                            disabled={catalogQ.isFetching}
                            className="inline-flex items-center gap-2 shrink-0 text-xs px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-600 hover:bg-white dark:hover:bg-slate-800 disabled:opacity-60"
                        >
                            <RefreshCw className={`w-3.5 h-3.5 ${catalogQ.isFetching ? 'animate-spin' : ''}`} aria-hidden />
                            Refresh catalog
                        </button>
                    </div>
                    <div className="p-4 grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3">
                        {catalogRows.map((row, idx) => {
                            const title = playbookRowTitle(row);
                            const sub = playbookRowSubtitle(row);
                            const chips = playbookRowMetaChips(row);
                            return (
                                <article
                                    key={`${title}-${idx}`}
                                    className="rounded-lg border border-slate-200/90 dark:border-slate-700/80 bg-white dark:bg-slate-900/50 p-4 flex flex-col gap-2 min-h-[7rem] shadow-sm"
                                >
                                    <div className="font-semibold text-slate-900 dark:text-white leading-snug">{title}</div>
                                    {sub ? (
                                        <p className="text-xs text-slate-600 dark:text-slate-400 leading-relaxed flex-1">{sub}</p>
                                    ) : (
                                        <p className="text-xs text-slate-400 dark:text-slate-500 italic flex-1">
                                            No description field — see attributes below or raw JSON.
                                        </p>
                                    )}
                                    {chips.length > 0 ? (
                                        <dl className="flex flex-wrap gap-1.5 pt-2 mt-auto border-t border-slate-100 dark:border-slate-800">
                                            {chips.map(({ key, value }) => (
                                                <div
                                                    key={key}
                                                    className="inline-flex items-center gap-1 rounded-md bg-slate-100 dark:bg-slate-800/80 px-2 py-0.5 text-[10px] font-mono text-slate-600 dark:text-slate-300"
                                                >
                                                    <span className="text-slate-400 dark:text-slate-500">{key}</span>
                                                    <span className="text-slate-800 dark:text-slate-200 truncate max-w-[9rem]" title={value}>
                                                        {value}
                                                    </span>
                                                </div>
                                            ))}
                                        </dl>
                                    ) : null}
                                </article>
                            );
                        })}
                    </div>
                    <details className="border-t border-slate-200 dark:border-slate-700 bg-slate-50/60 dark:bg-slate-950/30 group">
                        <summary className="px-5 py-3 text-xs font-medium text-slate-600 dark:text-slate-400 cursor-pointer list-none flex items-center gap-2 marker:content-none">
                            <span className="opacity-60 group-open:rotate-90 transition-transform">▸</span>
                            Technical · full JSON payload
                        </summary>
                        <pre className="px-5 pb-4 text-[11px] font-mono text-slate-700 dark:text-slate-300 whitespace-pre-wrap break-words max-h-56 overflow-y-auto border-t border-transparent">
                            {JSON.stringify(catalogQ.data, null, 2)}
                        </pre>
                    </details>
                </section>
            )}

            {catalogQ.isSuccess && catalogRows.length === 0 && !catalogQ.isPending && (
                <div className="rounded-xl border border-slate-200 dark:border-slate-700 bg-white/90 dark:bg-slate-800/60 px-5 py-4">
                    <h2 className="text-sm font-semibold text-slate-900 dark:text-white">Sigma playbook catalog</h2>
                    <p className="text-xs text-slate-500 dark:text-slate-400 mt-1 leading-relaxed">
                        The catalog endpoint responded with an empty list or a shape without playbook objects. Built-in command templates
                        below remain available and call the live agent command API.
                    </p>
                    <details className="mt-3">
                        <summary className="text-xs text-cyan-600 dark:text-cyan-400 cursor-pointer font-medium">Show raw response</summary>
                        <pre className="mt-2 text-[11px] font-mono text-slate-600 dark:text-slate-400 whitespace-pre-wrap break-words max-h-40 overflow-y-auto">
                            {catalogQ.data === undefined || catalogQ.data === null
                                ? '—'
                                : typeof catalogQ.data === 'string'
                                  ? catalogQ.data
                                  : JSON.stringify(catalogQ.data, null, 2)}
                        </pre>
                    </details>
                </div>
            )}

            {catalogQ.isError && (
                <div className="rounded-xl border border-slate-200 dark:border-slate-600 bg-slate-50 dark:bg-slate-800/50 px-5 py-4 text-sm text-slate-600 dark:text-slate-400">
                    <div className="font-semibold text-slate-800 dark:text-slate-200">Catalog unavailable</div>
                    <p className="mt-1 text-xs leading-relaxed">
                        Sigma{' '}
                        <code className="text-[11px] font-mono px-1 rounded bg-slate-200/80 dark:bg-slate-800">/api/v1/itsm/playbooks</code>{' '}
                        is not reachable in this deployment. The command templates below still issue real jobs via the connection manager —
                        they are not mock data.
                    </p>
                </div>
            )}

            <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm shadow-sm p-4 space-y-3">
                <label className="block text-xs font-semibold text-slate-500 uppercase tracking-wide">Target agent ID</label>
                <input
                    className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-3 py-2 text-sm font-mono"
                    value={agentId}
                    onChange={(e) => setAgentId(e.target.value)}
                    placeholder="e.g. agent UUID from Fleet"
                    autoComplete="off"
                />
                {!agentUuidOk && agentTrim.length > 0 ? (
                    <p className="text-xs text-amber-700 dark:text-amber-300">Enter a valid UUID to load playbook run history.</p>
                ) : null}
                {!canExec ? (
                    <p className="text-xs text-amber-700 dark:text-amber-300">Your role cannot execute commands (responses:execute).</p>
                ) : null}
                {agentUuidOk && (
                    <Link
                        to={`/management/devices/${encodeURIComponent(agentTrim)}`}
                        className="inline-flex text-sm text-cyan-600 dark:text-cyan-400 font-medium hover:underline"
                    >
                        Open endpoint detail →
                    </Link>
                )}
            </div>

            {canSeeRuns && agentUuidOk && (
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm shadow-sm overflow-hidden">
                    <div className="px-4 py-3 border-b border-slate-200 dark:border-slate-700 flex items-center justify-between gap-2">
                        <div>
                            <h3 className="text-sm font-semibold text-slate-900 dark:text-white">Recorded playbook runs (connection-manager)</h3>
                            <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
                                Server-tracked automation (e.g. post-isolation). Same source as Incident tab on the endpoint.
                            </p>
                        </div>
                        <button
                            type="button"
                            onClick={() => runsQ.refetch()}
                            className="text-xs px-2 py-1 rounded-lg border border-slate-300 dark:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-800"
                        >
                            Refresh
                        </button>
                    </div>
                    {runsQ.isLoading ? (
                        <div className="px-4 py-8 text-center text-sm text-slate-500">
                            <Loader2 className="inline w-4 h-4 animate-spin mr-2 align-middle" />
                            Loading runs…
                        </div>
                    ) : runsQ.isError ? (
                        <div className="px-4 py-4 text-sm text-red-600 dark:text-red-400">
                            {(runsQ.error as Error)?.message || 'Failed to load playbook runs'}
                        </div>
                    ) : !runsQ.data?.length ? (
                        <div className="px-4 py-6 text-sm text-slate-500 text-center">No playbook runs stored for this agent yet.</div>
                    ) : (
                        <div className="overflow-x-auto">
                            <table className="w-full text-sm">
                                <thead>
                                    <tr className="border-b border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-900/40 text-left text-[11px] uppercase tracking-wider text-slate-500">
                                        <th className="px-4 py-2">Playbook</th>
                                        <th className="px-4 py-2">Trigger</th>
                                        <th className="px-4 py-2">Status</th>
                                        <th className="px-4 py-2">Started</th>
                                    </tr>
                                </thead>
                                <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
                                    {runsQ.data.map((r) => (
                                        <tr key={r.id} className="hover:bg-slate-50 dark:hover:bg-slate-800/50">
                                            <td className="px-4 py-2 font-mono text-xs text-slate-900 dark:text-white">{r.playbook}</td>
                                            <td className="px-4 py-2 text-slate-600 dark:text-slate-400">{r.trigger}</td>
                                            <td className="px-4 py-2">
                                                <span className="rounded-md bg-slate-100 dark:bg-slate-700 px-2 py-0.5 text-xs">{r.status}</span>
                                            </td>
                                            <td className="px-4 py-2 text-slate-600 dark:text-slate-400 whitespace-nowrap">
                                                {formatDateTime(r.started_at)}
                                            </td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    )}
                </div>
            )}
            {canSeeRuns && !agentUuidOk && (
                <p className="text-xs text-slate-500 dark:text-slate-500">
                    Enter a valid agent UUID above to see stored playbook runs from the API.
                </p>
            )}

            <section className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-slate-50/40 dark:bg-slate-900/30 overflow-hidden w-full">
                <div className="px-5 py-5 sm:px-8 sm:py-6 border-b border-slate-200 dark:border-slate-700 bg-white/90 dark:bg-slate-800/60 w-full">
                    <h2 className="text-lg font-semibold text-slate-900 dark:text-white tracking-tight">Built-in command templates</h2>
                    <div className="mt-4 grid grid-cols-1 lg:grid-cols-2 gap-6 lg:gap-10 w-full text-sm text-slate-600 dark:text-slate-400 leading-relaxed">
                        <div className="min-w-0 space-y-2">
                            <p className="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-500">What this is</p>
                            <p>
                                Opinionated <strong className="font-medium text-slate-800 dark:text-slate-200">one-click bundles</strong>{' '}
                                implemented in this UI. Each run issues real agent commands through the connection manager — not placeholders.
                            </p>
                        </div>
                        <div className="min-w-0 space-y-2">
                            <p className="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-500">
                                Before you run &amp; beyond templates
                            </p>
                            <p>
                                Set the <strong className="font-medium text-slate-800 dark:text-slate-200">target agent UUID</strong> in the field
                                above so commands route to the correct endpoint. For ad-hoc control outside these defaults, use{' '}
                                <Link className="text-cyan-600 dark:text-cyan-400 font-semibold hover:underline" to="/responses">
                                    Command Center
                                </Link>
                                .
                            </p>
                        </div>
                    </div>
                </div>
                <div className="p-4 sm:p-6 md:p-8 grid grid-cols-1 lg:grid-cols-2 gap-4 lg:gap-6 w-full">
                <PlaybookCard
                    title="Contain: isolate network"
                    description="Immediate containment. Use Restore Network from Command Center to revert."
                    onRun={() => exec.mutate({ command_type: 'isolate_network', timeout: 300 })}
                />

                <PlaybookCard
                    title="Triage: collect forensics"
                    description="Collect a bounded set of telemetry for investigation."
                    onRun={() =>
                        exec.mutate({
                            command_type: 'collect_forensics',
                            parameters: { event_types: 'process,file,network,dns,registry', max_events: '500' },
                            timeout: 900,
                        })
                    }
                />

                <PlaybookCard
                    title="Stop suspicious process"
                    description="Terminate a process by name (best-effort)."
                    onRun={() =>
                        exec.mutate({
                            command_type: 'kill_process',
                            parameters: { process_name: killName, kill_tree: 'true' },
                            timeout: 300,
                        })
                    }
                >
                    <label className="block text-[10px] font-semibold uppercase tracking-wide text-slate-500 mt-2">process_name</label>
                    <input
                        className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-3 py-2 text-sm font-mono"
                        value={killName}
                        onChange={(e) => setKillName(e.target.value)}
                    />
                </PlaybookCard>

                <PlaybookCard
                    title="Block indicators (IP + domain)"
                    description="Queues block_ip then block_domain on the agent."
                    onRun={async () => {
                        await exec.mutateAsync({ command_type: 'block_ip', parameters: { ip, direction: 'both' }, timeout: 300 });
                        await exec.mutateAsync({ command_type: 'block_domain', parameters: { domain }, timeout: 300 });
                    }}
                    disabled={!ip.trim() || !domain.trim()}
                >
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 mt-2">
                        <div>
                            <label className="block text-[10px] font-semibold uppercase tracking-wide text-slate-500">ip</label>
                            <input
                                className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-3 py-2 text-sm font-mono"
                                value={ip}
                                onChange={(e) => setIp(e.target.value)}
                            />
                        </div>
                        <div>
                            <label className="block text-[10px] font-semibold uppercase tracking-wide text-slate-500">domain</label>
                            <input
                                className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-3 py-2 text-sm font-mono"
                                value={domain}
                                onChange={(e) => setDomain(e.target.value)}
                            />
                        </div>
                    </div>
                </PlaybookCard>
                </div>
            </section>
        </div>
    );
}

export function ItsmAutomationsPage() {
    const queryClient = useQueryClient();
    const { showToast } = useToast();
    const canExec = authApi.canExecuteCommands();
    const canSeeRuns = authApi.canViewResponses();
    const [agentId, setAgentId] = React.useState('');
    const [busy, setBusy] = React.useState(false);
    const [sigUrl, setSigUrl] = React.useState('');

    const agentTrim = agentId.trim();
    const agentUuidOk = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(agentTrim);

    useEffect(() => {
        document.title = 'Response Automations · Orchestration';
    }, []);

    const runsQ = useQuery({
        queryKey: ['incident', 'playbook-runs', agentTrim],
        queryFn: () => incidentApi.listRuns(agentTrim, 10),
        enabled: canSeeRuns && agentUuidOk,
        staleTime: 10_000,
    });

    const run = useMutation({
        mutationFn: async ({ command_type, parameters }: { command_type: CommandType; parameters?: Record<string, string> }) => {
            return agentsApi.executeCommand(agentTrim, { command_type, parameters: parameters ?? {}, timeout: 600 });
        },
        onSuccess: (data) => {
            showToast(`Automation step queued (${data.command_id})`, 'success');
            queryClient.invalidateQueries({ queryKey: ['commands'] });
            queryClient.invalidateQueries({ queryKey: ['agent-commands', agentTrim] });
            queryClient.invalidateQueries({ queryKey: ['incident', 'playbook-runs', agentTrim] });
        },
        onError: (e: Error) => showToast(e.message || 'Automation failed', 'error'),
    });

    const simpleAutomations = useMemo(
        (): { id: string; title: string; steps: { t: CommandType; p?: Record<string, string> }[]; needsSigUrl?: boolean }[] => [
            {
                id: 'isolate-collect',
                title: 'Containment: isolate → collect forensics',
                steps: [
                    { t: 'isolate_network' },
                    { t: 'collect_forensics', p: { event_types: 'process,file,network,dns,registry', max_events: '500' } },
                ],
            },
            {
                id: 'signatures-collect',
                title: 'Triage: update signatures → collect forensics',
                needsSigUrl: true,
                steps: [
                    {
                        t: 'update_signatures',
                        p: {
                            url: sigUrl.trim() || 'https://example.com/signatures.ndjson',
                        },
                    },
                    { t: 'collect_forensics', p: { event_types: 'process,file,network', max_events: '300' } },
                ],
            },
        ],
        [sigUrl]
    );

    const execAutomation = async (
        steps: { t: CommandType; p?: Record<string, string> }[],
        opts?: { needsSigUrl?: boolean }
    ) => {
        if (!agentTrim) {
            showToast('Enter an agent ID first.', 'info');
            return;
        }
        if (!canExec) {
            showToast('Missing responses:execute permission.', 'error');
            return;
        }
        if (opts?.needsSigUrl && !sigUrl.trim()) {
            showToast('Set a signature bundle HTTPS URL before running this automation.', 'info');
            return;
        }
        setBusy(true);
        try {
            for (const s of steps) {
                // eslint-disable-next-line no-await-in-loop
                await run.mutateAsync({ command_type: s.t, parameters: s.p });
            }
        } finally {
            setBusy(false);
        }
    };

    return (
        <div className="w-full min-w-0 space-y-6 animate-slide-up-fade">
            <InsightHero
                variant="light"
                accent="violet"
                icon={Zap}
                title="Response automations"
                segments={[
                    {
                        heading: 'Execution path',
                        children: (
                            <>
                                Chains of <strong className="font-medium text-slate-800 dark:text-slate-200">sequential commands</strong> issued
                                through{' '}
                                <code className="text-[11px] font-mono px-1 rounded bg-slate-200/90 dark:bg-slate-800">
                                    POST /api/v1/agents/:id/commands
                                </code>
                                . Each step is a real command job on the connection manager — not a simulated playbook runner.
                            </>
                        ),
                    },
                    {
                        heading: 'Architecture (self-hosted)',
                        children: (
                            <>
                                There is no separate “ITSM orchestrator” service in this repo — execution is always{' '}
                                <strong className="font-medium text-slate-800 dark:text-slate-200">connection-manager + agent</strong>. There is{' '}
                                <strong className="font-medium text-slate-800 dark:text-slate-200">no</strong> persisted{' '}
                                <code className="text-[11px] font-mono px-1 rounded bg-slate-200/90 dark:bg-slate-800">
                                    POST /api/v1/itsm/automations
                                </code>{' '}
                                workflow engine in the self-hosted connection-manager build.
                            </>
                        ),
                    },
                    {
                        heading: 'Extend',
                        children: (
                            <>
                                Use the chains below for multi-step containment and triage. Optional Sigma-side routes can be added separately
                                without changing how agents execute work.
                            </>
                        ),
                    },
                ]}
            >
                <ul className="grid grid-cols-1 sm:grid-cols-3 gap-3 text-xs text-slate-600 dark:text-slate-400 list-none p-0 m-0">
                    <li className="rounded-xl border border-slate-200/90 dark:border-slate-700 bg-white/70 dark:bg-slate-950/35 px-4 py-3 shadow-sm">
                        <span className="font-semibold text-slate-800 dark:text-slate-200">vs Playbooks</span> —{' '}
                        <Link className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium" to="/itsm/playbooks">
                            Response Playbooks
                        </Link>{' '}
                        focuses on single-step templates; here you run <strong className="font-medium">multi-step</strong> chains.
                    </li>
                    <li className="rounded-xl border border-slate-200/90 dark:border-slate-700 bg-white/70 dark:bg-slate-950/35 px-4 py-3 shadow-sm">
                        <span className="font-semibold text-slate-800 dark:text-slate-200">vs Command Center</span> —{' '}
                        <Link className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium" to="/responses">
                            Command Center
                        </Link>{' '}
                        is manual every time; automations bundle ordered steps with one click.
                    </li>
                    <li className="rounded-xl border border-slate-200/90 dark:border-slate-700 bg-white/70 dark:bg-slate-950/35 px-4 py-3 shadow-sm">
                        <span className="font-semibold text-slate-800 dark:text-slate-200">Fleet</span> — copy agent UUID from{' '}
                        <Link className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium" to="/management/devices">
                            Devices (Fleet)
                        </Link>
                        .
                    </li>
                </ul>
            </InsightHero>

            <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm shadow-sm p-4 space-y-4">
                <label className="block text-xs font-semibold text-slate-500 uppercase tracking-wide">Target agent ID</label>
                <input
                    className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-3 py-2 text-sm font-mono"
                    value={agentId}
                    onChange={(e) => setAgentId(e.target.value)}
                    placeholder="UUID from Fleet"
                    disabled={busy}
                    autoComplete="off"
                />
                {!agentUuidOk && agentTrim.length > 0 ? (
                    <p className="text-xs text-amber-700 dark:text-amber-300">Enter a valid UUID to load playbook run history.</p>
                ) : null}
                {!canExec ? (
                    <p className="text-xs text-amber-700 dark:text-amber-300">Your role cannot execute commands (responses:execute).</p>
                ) : null}

                <div>
                    <label className="block text-xs font-semibold text-slate-500 uppercase tracking-wide">
                        Signature bundle URL (required for “signatures → collect” chain)
                    </label>
                    <input
                        className="mt-1 w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-3 py-2 text-sm"
                        value={sigUrl}
                        onChange={(e) => setSigUrl(e.target.value)}
                        placeholder="https://your-server/signatures.ndjson"
                        disabled={busy}
                    />
                    <p className="text-[11px] text-slate-500 dark:text-slate-500 mt-1">
                        The agent must reach this URL; avoid placeholder hosts. Leave empty and the flow will refuse to run until you set it.
                    </p>
                </div>

                {agentUuidOk && (
                    <Link
                        to={`/management/devices/${encodeURIComponent(agentTrim)}`}
                        className="inline-flex text-sm text-cyan-600 dark:text-cyan-400 font-medium hover:underline"
                    >
                        Open endpoint detail →
                    </Link>
                )}

                <div className="grid grid-cols-1 md:grid-cols-2 gap-3 pt-2">
                    {simpleAutomations.map((a) => (
                        <button
                            key={a.id}
                            type="button"
                            className="rounded-xl border border-slate-200 dark:border-slate-700 p-4 text-left hover:bg-slate-50 dark:hover:bg-slate-800/50 disabled:opacity-50 transition-colors"
                            disabled={busy || run.isPending}
                            onClick={() => execAutomation(a.steps, { needsSigUrl: a.needsSigUrl })}
                        >
                            <div className="font-semibold text-slate-900 dark:text-white">{a.title}</div>
                            <div className="text-xs text-slate-500 dark:text-slate-400 mt-1 font-mono">
                                {a.steps.map((s) => s.t).join(' → ')}
                            </div>
                            {a.needsSigUrl ? (
                                <div className="text-[11px] text-amber-700 dark:text-amber-400 mt-2">Requires signature URL above.</div>
                            ) : null}
                        </button>
                    ))}
                </div>
                <p className="text-xs text-slate-500 dark:text-slate-500">
                    Manual one-off commands:{' '}
                    <Link className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium" to="/responses">
                        Command Center
                    </Link>
                    . Single-step templates:{' '}
                    <Link className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium" to="/itsm/playbooks">
                        Response Playbooks
                    </Link>
                    .
                </p>
            </div>

            {canSeeRuns && agentUuidOk && (
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm shadow-sm overflow-hidden">
                    <div className="px-4 py-3 border-b border-slate-200 dark:border-slate-700 flex items-center justify-between gap-2">
                        <div>
                            <h3 className="text-sm font-semibold text-slate-900 dark:text-white">Related playbook runs (same agent)</h3>
                            <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
                                Post-isolation / engine runs from connection-manager — shared data model with Playbooks page.
                            </p>
                        </div>
                        <button
                            type="button"
                            onClick={() => runsQ.refetch()}
                            className="text-xs px-2 py-1 rounded-lg border border-slate-300 dark:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-800"
                        >
                            Refresh
                        </button>
                    </div>
                    {runsQ.isLoading ? (
                        <div className="px-4 py-8 text-center text-sm text-slate-500">
                            <Loader2 className="inline w-4 h-4 animate-spin mr-2 align-middle" />
                            Loading…
                        </div>
                    ) : runsQ.isError ? (
                        <div className="px-4 py-4 text-sm text-red-600 dark:text-red-400">
                            {(runsQ.error as Error)?.message || 'Failed to load runs'}
                        </div>
                    ) : !runsQ.data?.length ? (
                        <div className="px-4 py-6 text-sm text-slate-500 text-center">No recorded playbook runs for this agent.</div>
                    ) : (
                        <div className="overflow-x-auto">
                            <table className="w-full text-sm">
                                <thead>
                                    <tr className="border-b border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-900/40 text-left text-[11px] uppercase tracking-wider text-slate-500">
                                        <th className="px-4 py-2">Playbook</th>
                                        <th className="px-4 py-2">Trigger</th>
                                        <th className="px-4 py-2">Status</th>
                                        <th className="px-4 py-2">Started</th>
                                    </tr>
                                </thead>
                                <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
                                    {runsQ.data.map((r) => (
                                        <tr key={r.id} className="hover:bg-slate-50 dark:hover:bg-slate-800/50">
                                            <td className="px-4 py-2 font-mono text-xs text-slate-900 dark:text-white">{r.playbook}</td>
                                            <td className="text-slate-600 dark:text-slate-400 px-4 py-2">{r.trigger}</td>
                                            <td className="px-4 py-2">
                                                <span className="rounded-md bg-slate-100 dark:bg-slate-700 px-2 py-0.5 text-xs">{r.status}</span>
                                            </td>
                                            <td className="px-4 py-2 text-slate-600 dark:text-slate-400 whitespace-nowrap">
                                                {formatDateTime(r.started_at)}
                                            </td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    )}
                </div>
            )}
        </div>
    );
}

export function ItsmIntegrationsPage() {
    return (
        <div className="space-y-4 animate-slide-up-fade">
            <div>
                <h2 className="text-lg font-semibold text-slate-900 dark:text-white">ITSM — integrations</h2>
                <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
                    Integrations will connect ticketing/chat/webhooks. For now, configure outbound destinations under{' '}
                    <Link className="text-cyan-600 dark:text-cyan-400 hover:underline" to="/security/siem-x">
                        SIEM connectors
                    </Link>
                    .
                </p>
            </div>
            <div className="rounded-xl border border-dashed border-slate-300 dark:border-slate-700 bg-white/50 dark:bg-slate-900/20 p-6 text-sm text-slate-600 dark:text-slate-400">
                Backend integration endpoints are not exposed yet. Once available, this page will manage credentials + test connections.
            </div>
        </div>
    );
}

export function ManagementDevicesPage() {
    return (
        <GenericParityView
            title="Device management"
            description="Aligned with `/management/devices` — can mirror `/api/v1/agents` later."
            missingApi="true"
            queryKey={['parity', 'management', 'devices']}
            fetcher={() => parityApi.getManagementDevices()}
            mock={mocks.mockManagementDevices}
        />
    );
}

const FleetConnectivityStatusBadge = React.memo(function FleetConnectivityStatusBadge({
    status,
}: {
    status: Agent['status'];
}) {
    const config = {
        online: { label: 'Online', color: 'badge-online', icon: Wifi },
        offline: { label: 'Offline', color: 'badge-offline', icon: WifiOff },
        degraded: { label: 'Degraded', color: 'badge-degraded', icon: AlertTriangle },
        pending: { label: 'Pending', color: 'badge-warning', icon: Clock },
        suspended: { label: 'Suspended', color: 'badge-danger', icon: X },
        pending_uninstall: { label: 'Uninstalling…', color: 'badge-warning', icon: Trash2 },
        uninstalled: { label: 'Uninstalled', color: 'badge-offline', icon: Trash2 },
    } as const;

    const { label, color, icon: Icon } = config[status as keyof typeof config] || config.offline;

    return (
        <span className={`badge ${color} flex items-center gap-1 w-fit`}>
            <Icon className="w-3 h-3" />
            {label}
        </span>
    );
});

function fleetEffectivePriority(eff: Agent['status']): number {
    switch (eff) {
        case 'offline':
            return 0;
        case 'suspended':
            return 1;
        case 'degraded':
            return 2;
        case 'pending':
            return 3;
        case 'pending_uninstall':
            return 4;
        case 'online':
            return 5;
        case 'uninstalled':
            return 99;
        default:
            return 50;
    }
}

function compareFleetWorstFirst(a: Agent, b: Agent): number {
    const ea = getEffectiveStatus(a);
    const eb = getEffectiveStatus(b);
    const pa = fleetEffectivePriority(ea);
    const pb = fleetEffectivePriority(eb);
    if (pa !== pb) return pa - pb;
    const qa = a.queue_depth ?? 0;
    const qb = b.queue_depth ?? 0;
    if (qa !== qb) return qb - qa;
    const da = a.events_dropped ?? 0;
    const db = b.events_dropped ?? 0;
    if (da !== db) return db - da;
    return new Date(a.last_seen).getTime() - new Date(b.last_seen).getTime();
}

function fleetAttentionReasons(a: Agent): string[] {
    const eff = getEffectiveStatus(a);
    const r: string[] = [];
    if ((a.status === 'online' || a.status === 'degraded') && eff === 'offline') {
        r.push('Stale heartbeat');
    }
    if (eff === 'offline' && a.status === 'offline') {
        r.push('Offline');
    }
    if (eff === 'suspended') {
        r.push('Suspended');
    }
    if (eff === 'degraded') {
        r.push('Degraded link');
    }
    if ((a.queue_depth ?? 0) > 0) {
        r.push('Command backlog');
    }
    if ((a.events_dropped ?? 0) > 0) {
        r.push('Dropped events');
    }
    if (a.is_isolated) {
        r.push('Network isolated');
    }
    return r;
}

/** Fleet connectivity: server-wide stats + worst-first agent table from `GET /api/v1/agents` (and `/stats`). */
export function ManagementNetworkPage() {
    const statsQ = useQuery({
        queryKey: ['agents-stats'],
        queryFn: () => agentsApi.stats(),
        staleTime: 30_000,
        refetchInterval: 30_000,
        retry: 1,
    });

    const listQ = useQuery({
        queryKey: ['management-network-fleet', 1000],
        queryFn: () => agentsApi.list({ limit: 1000, offset: 0 }),
        staleTime: 30_000,
        refetchInterval: 30_000,
    });

    const rawRows = listQ.data?.data ?? [];
    const pagination = listQ.data?.pagination;
    const s = statsQ.data;

    const rows = useMemo(() => [...rawRows].sort(compareFleetWorstFirst), [rawRows]);

    const sampleAgg = useMemo(() => {
        let staleHeartbeat = 0;
        let isolated = 0;
        let queueSum = 0;
        let dropsSum = 0;
        for (const a of rawRows) {
            const eff = getEffectiveStatus(a);
            if ((a.status === 'online' || a.status === 'degraded') && eff === 'offline') {
                staleHeartbeat += 1;
            }
            if (a.is_isolated) {
                isolated += 1;
            }
            queueSum += a.queue_depth ?? 0;
            dropsSum += a.events_dropped ?? 0;
        }
        return { staleHeartbeat, isolated, queueSum, dropsSum };
    }, [rawRows]);

    const attention = useMemo(() => {
        return rows
            .map((agent) => ({ agent, reasons: fleetAttentionReasons(agent) }))
            .filter((x) => x.reasons.length > 0)
            .slice(0, 8);
    }, [rows]);

    const staleLabel = `>${Math.round(STALE_THRESHOLD_MS / 1000)}s since last_seen`;

    if (listQ.isLoading) {
        return (
            <div className="flex items-center justify-center py-16 text-slate-500 gap-2">
                <Loader2 className="w-6 h-6 animate-spin" /> Loading fleet…
            </div>
        );
    }

    if (listQ.isError || !listQ.data?.data) {
        return (
            <div className="rounded-lg border border-rose-200 dark:border-rose-900/50 p-4 text-sm text-rose-800 dark:text-rose-200">
                Could not load agents. Check connection-manager and <code className="text-xs">endpoints:read</code>.
            </div>
        );
    }

    return (
        <div className="space-y-4 animate-slide-up-fade w-full min-w-0">
            <InsightHero
                variant="light"
                accent="cyan"
                icon={Wifi}
                eyebrow="Fleet operations"
                title="Fleet connectivity"
                segments={[
                    {
                        heading: 'Data sources',
                        children: (
                            <>
                                Fleet-wide posture from{' '}
                                <code className="text-[11px] font-mono px-1 py-0.5 rounded bg-slate-200/90 dark:bg-slate-800">/api/v1/agents/stats</code>; per-host queue, drops, and
                                addresses come from the paged agent list (same registry as Devices).
                            </>
                        ),
                    },
                    {
                        heading: 'Effective “live” status',
                        children: (
                            <>
                                Registry <strong className="font-semibold text-slate-800 dark:text-slate-200">online/degraded</strong> is shown as{' '}
                                <strong className="font-semibold text-slate-800 dark:text-slate-200">offline</strong> when more than {staleLabel} since{' '}
                                <code className="text-[11px] font-mono px-1 rounded bg-slate-200/90 dark:bg-slate-800">last_seen</code> — identical to the Device Management grid.
                            </>
                        ),
                    },
                    {
                        heading: 'Per-host depth',
                        children: (
                            <>
                                Deep per-host network telemetry (interfaces, routes, DNS context) lives on the endpoint — open a host from{' '}
                                <Link className="text-cyan-600 dark:text-cyan-400 font-semibold hover:underline" to="/management/devices">
                                    Devices (Fleet)
                                </Link>{' '}
                                → <strong className="font-medium text-slate-800 dark:text-slate-200">Network</strong> tab on device detail.
                            </>
                        ),
                    },
                ]}
            />

            {statsQ.isLoading && !s && (
                <div className="h-36 rounded-xl bg-slate-100 dark:bg-slate-800 animate-pulse" aria-hidden />
            )}

            {statsQ.isError && (
                <div className="rounded-lg border border-amber-200 dark:border-amber-900/40 bg-amber-50/80 dark:bg-amber-950/20 px-3 py-2 text-xs text-amber-900 dark:text-amber-200">
                    Fleet totals from <code className="text-[10px]">/agents/stats</code> unavailable — KPI row below uses this page of agents only.
                </div>
            )}

            {s && (
                <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                    <StatCard title="Endpoints (fleet)" value={String(s.total)} icon={Radio} color="cyan" />
                    <StatCard title="Online" value={String(s.online)} icon={Activity} color="emerald" />
                    <StatCard title="Degraded" value={String(s.degraded)} icon={AlertTriangle} color="amber" />
                    <StatCard title="Offline" value={String(s.offline)} icon={WifiOff} color="red" />
                </div>
            )}

            {s && (
                <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                    <StatCard title="Pending" value={String(s.pending)} icon={Clock} />
                    <StatCard title="Suspended" value={String(s.suspended)} icon={Shield} color="amber" />
                    <StatCard title="Avg health" value={`${Math.round(s.avg_health)}%`} icon={Activity} color="cyan" />
                    <StatCard
                        title="Loaded for table"
                        value={String(rawRows.length)}
                        icon={Radio}
                        subtext={
                            pagination?.has_more
                                ? `Has more — worst-first among first ${rawRows.length} loaded`
                                : 'Full list in this response'
                        }
                    />
                </div>
            )}

            {!s && (
                <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                    <StatCard
                        title="Agents (this page)"
                        value={String(rawRows.length)}
                        icon={Radio}
                        color="cyan"
                        subtext={pagination?.has_more ? 'Additional pages not loaded here' : undefined}
                    />
                </div>
            )}

            <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                <StatCard
                    title="Stale heartbeat"
                    value={String(sampleAgg.staleHeartbeat)}
                    icon={WifiOff}
                    color="amber"
                    subtext={`In loaded rows · ${staleLabel}`}
                />
                <StatCard
                    title="Isolated (loaded)"
                    value={String(sampleAgg.isolated)}
                    icon={Shield}
                    color="red"
                />
                <StatCard
                    title="Command backlog"
                    value={String(sampleAgg.queueSum)}
                    icon={Activity}
                    color="amber"
                    subtext="Σ queue_depth in loaded rows"
                />
                <StatCard
                    title="Dropped events"
                    value={String(sampleAgg.dropsSum)}
                    icon={AlertTriangle}
                    color="red"
                    subtext="Σ events_dropped in loaded rows"
                />
            </div>

            {attention.length > 0 && (
                <div className="rounded-xl border border-amber-200/80 dark:border-amber-900/40 bg-amber-50/50 dark:bg-amber-950/20 p-4 space-y-2">
                    <p className="text-sm font-semibold text-slate-900 dark:text-white flex items-center gap-2">
                        <AlertTriangle className="w-4 h-4 text-amber-600 dark:text-amber-400 shrink-0" />
                        Needs attention (top {attention.length})
                    </p>
                    <ul className="space-y-2 text-sm">
                        {attention.map(({ agent, reasons }) => (
                            <li
                                key={agent.id}
                                className="flex flex-wrap items-center gap-x-3 gap-y-1 border-t border-amber-200/60 dark:border-amber-900/30 pt-2 first:border-t-0 first:pt-0"
                            >
                                <Link
                                    className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline shrink-0"
                                    to={`/management/devices/${encodeURIComponent(agent.id)}`}
                                >
                                    {agent.hostname}
                                </Link>
                                <FleetConnectivityStatusBadge status={getEffectiveStatus(agent)} />
                                <span className="text-slate-600 dark:text-slate-400 text-xs">
                                    {reasons.join(' · ')}
                                </span>
                            </li>
                        ))}
                    </ul>
                </div>
            )}

            <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm shadow-sm">
                <table className="min-w-full text-left text-sm">
                    <thead className="bg-slate-50 dark:bg-slate-800/80 text-slate-600 dark:text-slate-400 text-xs uppercase">
                        <tr>
                            <th className="px-3 py-2">Host</th>
                            <th className="px-3 py-2">Live status</th>
                            <th className="px-3 py-2 hidden md:table-cell">Registry</th>
                            <th className="px-3 py-2">Last seen</th>
                            <th className="px-3 py-2 hidden lg:table-cell">IPs</th>
                            <th className="px-3 py-2 text-right">Queue</th>
                            <th className="px-3 py-2 text-right">Dropped</th>
                            <th className="px-3 py-2 text-right hidden xl:table-cell">Collected</th>
                            <th className="px-3 py-2 text-right hidden xl:table-cell">Delivered</th>
                            <th className="px-3 py-2 text-right hidden lg:table-cell">CPU %</th>
                            <th className="px-3 py-2 text-right hidden lg:table-cell">Mem MB</th>
                            <th className="px-3 py-2">Isolated</th>
                        </tr>
                    </thead>
                    <tbody>
                        {rows.map((a) => {
                            const eff = getEffectiveStatus(a);
                            const stale =
                                (a.status === 'online' || a.status === 'degraded') && eff === 'offline';
                            return (
                                <tr key={a.id} className="border-t border-slate-100 dark:border-slate-800">
                                    <td className="px-3 py-2">
                                        <Link
                                            className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline"
                                            to={`/management/devices/${encodeURIComponent(a.id)}`}
                                        >
                                            {a.hostname}
                                        </Link>
                                    </td>
                                    <td className="px-3 py-2">
                                        <div className="flex flex-col gap-1 items-start">
                                            <FleetConnectivityStatusBadge status={eff} />
                                            {stale && (
                                                <span className="text-[10px] uppercase tracking-wide text-amber-600 dark:text-amber-400">
                                                    Stale vs registry
                                                </span>
                                            )}
                                        </div>
                                    </td>
                                    <td className="px-3 py-2 hidden md:table-cell">
                                        <span className="font-mono text-xs text-slate-500 dark:text-slate-400">{a.status}</span>
                                    </td>
                                    <td className="px-3 py-2 text-xs whitespace-nowrap">{formatRelativeTime(a.last_seen)}</td>
                                    <td className="px-3 py-2 text-xs break-all max-w-xs hidden lg:table-cell">
                                        {(a.ip_addresses || []).join(', ') || '—'}
                                    </td>
                                    <td className="px-3 py-2 text-right tabular-nums">{a.queue_depth ?? 0}</td>
                                    <td className="px-3 py-2 text-right tabular-nums">{a.events_dropped ?? 0}</td>
                                    <td className="px-3 py-2 text-right tabular-nums hidden xl:table-cell">
                                        {a.events_collected ?? '—'}
                                    </td>
                                    <td className="px-3 py-2 text-right tabular-nums hidden xl:table-cell">{a.events_delivered}</td>
                                    <td className="px-3 py-2 text-right tabular-nums hidden lg:table-cell">
                                        {a.cpu_usage != null ? `${a.cpu_usage.toFixed(0)}%` : '—'}
                                    </td>
                                    <td className="px-3 py-2 text-right tabular-nums hidden lg:table-cell">
                                        {a.memory_used_mb != null ? Math.round(a.memory_used_mb) : '—'}
                                    </td>
                                    <td className="px-3 py-2">{a.is_isolated ? 'Yes' : 'No'}</td>
                                </tr>
                            );
                        })}
                    </tbody>
                </table>
            </div>

            <p className="text-xs text-slate-500 dark:text-slate-400">
                Sort: worst effective status first, then higher command queue, more dropped events, then oldest last_seen.{' '}
                <Link className="text-cyan-600 dark:text-cyan-400 hover:underline" to="/management/devices">
                    Open Devices
                </Link>{' '}
                for search, tags, and bulk actions.
            </p>
        </div>
    );
}

export function ManagementStaffPage() {
    return <SelfHostedOutOfScope title="Staff / shifts" />;
}

export function ManagementAccountPage() {
    useEffect(() => {
        document.title = 'Account — Management | EDR Platform';
    }, []);

    const meQ = useQuery({
        queryKey: ['auth', 'me'],
        queryFn: async () => {
            const u = await authApi.fetchMe();
            const prev = authApi.getCurrentUser();
            if (prev && u) {
                localStorage.setItem(
                    'user',
                    JSON.stringify({
                        ...prev,
                        ...u,
                        id: u.id || prev.id,
                        role: (u.role || prev.role) as User['role'],
                    })
                );
            }
            return u;
        },
        staleTime: 60_000,
        retry: 1,
    });

    const u = meQ.data;
    const permRows = useMemo(
        () => [
            {
                area: 'Alerts',
                view: authApi.canViewAlerts(),
                act: authApi.canWriteAlerts() ? 'Triage write' : authApi.canViewAlerts() ? 'Read' : '—',
            },
            {
                area: 'Endpoints / devices',
                view: authApi.canViewEndpoints(),
                act: authApi.canIsolateEndpoints()
                    ? 'Manage + isolate'
                    : authApi.canManageEndpoints()
                      ? 'Manage'
                      : authApi.canViewEndpoints()
                        ? 'Read'
                        : '—',
            },
            {
                area: 'Detection rules',
                view: authApi.canViewRules(),
                act: authApi.canWriteRules() ? 'Edit' : authApi.canViewRules() ? 'Read' : '—',
            },
            {
                area: 'Responses / Command Center',
                view: authApi.canViewResponses(),
                act: authApi.canExecuteCommands() ? 'Execute' : authApi.canViewResponses() ? 'Read' : '—',
            },
            {
                area: 'Settings (platform)',
                view: authApi.canViewSettings(),
                act: authApi.canWriteSettings() ? 'Admin write' : authApi.canViewSettings() ? 'Read' : '—',
            },
            {
                area: 'Users & roles',
                view: authApi.canViewUsers() || authApi.canViewRoles(),
                act: authApi.canManageUsers() ? 'Manage users' : authApi.canViewRoles() ? 'View roles' : '—',
            },
            {
                area: 'Audit logs',
                view: authApi.canViewAuditLogs(),
                act: authApi.canViewAuditLogs() ? 'Read' : '—',
            },
            {
                area: 'Enrollment tokens',
                view: authApi.canViewTokens(),
                act: authApi.canManageTokens() ? 'Manage' : authApi.canViewTokens() ? 'Read' : '—',
            },
        ],
        [meQ.dataUpdatedAt]
    );

    if (meQ.isLoading) {
        return (
            <div className="space-y-4 animate-pulse">
                <div className="h-28 rounded-2xl bg-slate-200 dark:bg-slate-800" />
                <div className="grid md:grid-cols-2 gap-4">
                    <div className="h-48 rounded-xl bg-slate-200 dark:bg-slate-800" />
                    <div className="h-48 rounded-xl bg-slate-200 dark:bg-slate-800" />
                </div>
            </div>
        );
    }

    if (meQ.isError || !u) {
        return (
            <div className="rounded-xl border border-rose-200 dark:border-rose-900/50 bg-rose-50/80 dark:bg-rose-950/20 p-6 text-sm text-rose-900 dark:text-rose-200">
                Could not load account from <code className="text-xs">GET /api/v1/auth/me</code>. Sign in again or check connection-manager.
            </div>
        );
    }

    return (
        <div className="w-full min-w-0 space-y-6 animate-slide-up-fade">
            <InsightHero
                variant="dark"
                accent="sky"
                icon={UserIcon}
                eyebrow="Identity"
                title="Account"
                lead={
                    <>
                        Read-only summary of <strong className="text-white">who you are in this tenant</strong> and which capabilities your role unlocks in the UI. Profile edits and
                        password changes live under{' '}
                        <Link to="/system/profile" className="text-sky-300 font-semibold hover:underline">
                            System → Profile
                        </Link>{' '}
                        — not here.
                    </>
                }
            />

            <div className="grid gap-3 md:grid-cols-2">
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <Settings className="w-4 h-4 text-slate-500" />
                        vs System → Profile
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/system/profile">
                            Profile
                        </Link>{' '}
                        is where you <strong>edit name, email, and password</strong>. This page only reflects server state and effective permissions.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <Shield className="w-4 h-4 text-violet-500" />
                        vs Admin users
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        Admins manage <strong>other</strong> accounts under platform user management. This screen is only <strong>your</strong> signed-in principal.
                    </p>
                </div>
            </div>

            <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 shadow-sm overflow-hidden">
                <div className="px-5 py-4 border-b border-slate-100 dark:border-slate-800 flex items-center gap-3">
                    <div className="w-10 h-10 rounded-xl bg-sky-500/15 text-sky-600 dark:text-sky-400 flex items-center justify-center">
                        <Fingerprint className="w-5 h-5" />
                    </div>
                    <div>
                        <h2 className="text-sm font-bold text-slate-900 dark:text-white">Principal (from API)</h2>
                        <p className="text-xs text-slate-500 dark:text-slate-400">Synchronized from GET /api/v1/auth/me · updates local session cache</p>
                    </div>
                </div>
                <dl className="grid sm:grid-cols-2 gap-4 p-5 text-sm">
                    <div>
                        <dt className="text-[11px] font-semibold uppercase text-slate-400">Username</dt>
                        <dd className="mt-1 font-mono text-slate-900 dark:text-slate-100">{u.username}</dd>
                    </div>
                    <div>
                        <dt className="text-[11px] font-semibold uppercase text-slate-400">User ID</dt>
                        <dd className="mt-1 font-mono text-xs text-slate-600 dark:text-slate-300 break-all">{u.id || '—'}</dd>
                    </div>
                    <div>
                        <dt className="text-[11px] font-semibold uppercase text-slate-400">Role</dt>
                        <dd className="mt-1 capitalize text-slate-900 dark:text-white">{u.role}</dd>
                    </div>
                    <div>
                        <dt className="text-[11px] font-semibold uppercase text-slate-400">Status</dt>
                        <dd className="mt-1 capitalize text-slate-900 dark:text-white">{u.status || '—'}</dd>
                    </div>
                    <div>
                        <dt className="text-[11px] font-semibold uppercase text-slate-400">Full name</dt>
                        <dd className="mt-1 text-slate-800 dark:text-slate-100">{u.full_name || '—'}</dd>
                    </div>
                    <div>
                        <dt className="text-[11px] font-semibold uppercase text-slate-400">Email</dt>
                        <dd className="mt-1 text-slate-800 dark:text-slate-100">{u.email || '—'}</dd>
                    </div>
                    <div>
                        <dt className="text-[11px] font-semibold uppercase text-slate-400">Last login</dt>
                        <dd className="mt-1 text-slate-700 dark:text-slate-200">
                            {u.last_login ? formatDateTime(u.last_login) : '—'}
                        </dd>
                    </div>
                    <div>
                        <dt className="text-[11px] font-semibold uppercase text-slate-400">Record updated</dt>
                        <dd className="mt-1 text-slate-700 dark:text-slate-200">{u.updated_at ? formatDateTime(u.updated_at) : '—'}</dd>
                    </div>
                </dl>
            </div>

            <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 shadow-sm overflow-hidden">
                <div className="px-5 py-4 border-b border-slate-100 dark:border-slate-800 flex items-center gap-3">
                    <div className="w-10 h-10 rounded-xl bg-violet-500/15 text-violet-600 dark:text-violet-400 flex items-center justify-center">
                        <Shield className="w-5 h-5" />
                    </div>
                    <div>
                        <h2 className="text-sm font-bold text-slate-900 dark:text-white">Effective UI capabilities</h2>
                        <p className="text-xs text-slate-500 dark:text-slate-400">
                            Derived from your role using the same helpers as navigation (RBAC is still enforced on the API).
                        </p>
                    </div>
                </div>
                <div className="overflow-x-auto">
                    <table className="min-w-full text-left text-sm">
                        <thead className="bg-slate-50 dark:bg-slate-900/50 text-slate-500 dark:text-slate-400 text-xs uppercase">
                            <tr>
                                <th className="px-4 py-2.5">Area</th>
                                <th className="px-4 py-2.5">Visible</th>
                                <th className="px-4 py-2.5">Level</th>
                            </tr>
                        </thead>
                        <tbody>
                            {permRows.map((row) => (
                                <tr key={row.area} className="border-t border-slate-100 dark:border-slate-800">
                                    <td className="px-4 py-2.5 text-slate-800 dark:text-slate-100">{row.area}</td>
                                    <td className="px-4 py-2.5 text-xs">{row.view ? 'Yes' : 'No'}</td>
                                    <td className="px-4 py-2.5 text-xs text-slate-600 dark:text-slate-300">{row.act}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            </div>

            <div className="flex flex-wrap gap-4 text-sm">
                <Link to="/system/profile" className="inline-flex items-center gap-2 text-cyan-600 dark:text-cyan-400 font-medium hover:underline">
                    <Settings className="w-4 h-4" />
                    Edit profile & password →
                </Link>
                <Link to="/system/audit-logs" className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline">
                    Audit logs →
                </Link>
            </div>
        </div>
    );
}

export function ManagementProfilesPage() {
    return (
        <GenericParityView
            title="Profile management"
            missingApi="true"
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
                missingApi="true"
                queryKey={['parity', 'patch', 'overview']}
                fetcher={() => parityApi.getPatchOverview()}
                mock={mocks.mockPatchOverview}
            />
            <GenericParityView
                title="Patch — missing"
                missingApi="true"
                queryKey={['parity', 'patch', 'missing']}
                fetcher={() => parityApi.getPatchMissing()}
                mock={mocks.mockPatchMissing.data}
            />
        </div>
    );
}

const VULN_STATUS_OPTIONS: VulnerabilityFinding['status'][] = ['open', 'acknowledged', 'resolved', 'risk_accepted'];

function vulnSeverityClass(sev: string): string {
    switch (sev) {
        case 'critical':
            return 'bg-rose-100 text-rose-900 dark:bg-rose-900/40 dark:text-rose-100';
        case 'high':
            return 'bg-orange-100 text-orange-900 dark:bg-orange-900/35 dark:text-orange-100';
        case 'medium':
            return 'bg-amber-100 text-amber-900 dark:bg-amber-900/30 dark:text-amber-100';
        case 'low':
            return 'bg-slate-200 text-slate-800 dark:bg-slate-700 dark:text-slate-100';
        default:
            return 'bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-200';
    }
}

export function ManagementVulnPage() {
    const { showToast } = useToast();
    const queryClient = useQueryClient();
    const canManageEndpoints = authApi.canManageEndpoints();

    const [statusFilter, setStatusFilter] = React.useState('');
    const [severityFilter, setSeverityFilter] = React.useState('');
    const [search, setSearch] = React.useState('');

    const findingsQ = useQuery({
        queryKey: ['vuln-findings', { status: statusFilter, severity: severityFilter, search }],
        queryFn: () =>
            vulnerabilityApi.listFindings({
                limit: 500,
                offset: 0,
                status: statusFilter || undefined,
                severity: severityFilter || undefined,
                search: search.trim() || undefined,
            }),
        staleTime: 20_000,
        refetchInterval: 60_000,
    });

    const patchStatus = useMutation({
        mutationFn: ({ id, status }: { id: string; status: string }) => vulnerabilityApi.patchFindingStatus(id, status),
        onSuccess: () => {
            showToast('Finding status updated', 'success');
            queryClient.invalidateQueries({ queryKey: ['vuln-findings'] });
        },
        onError: (e: Error) => showToast(e.message || 'Update failed', 'error'),
    });

    const rows = findingsQ.data?.data ?? [];
    const total = findingsQ.data?.pagination?.total ?? rows.length;

    const stats = useMemo(() => {
        let openCrit = 0;
        let openHigh = 0;
        let open = 0;
        const hosts = new Set<string>();
        for (const r of rows) {
            if (r.status === 'open') {
                open += 1;
                if (r.severity === 'critical') {
                    openCrit += 1;
                }
                if (r.severity === 'high') {
                    openHigh += 1;
                }
                hosts.add(r.agent_id);
            }
        }
        return { openCrit, openHigh, open, hostsAffected: hosts.size };
    }, [rows]);

    return (
        <div className="space-y-5 animate-slide-up-fade w-full min-w-0">
            <InsightHero
                variant="light"
                accent="rose"
                icon={AlertTriangle}
                title="Vulnerability exposure"
                lead={
                    <>
                        This workspace is for <strong className="font-semibold text-slate-800 dark:text-slate-200">software and CVE posture</strong> per host: imported or scanner-sourced rows
                        stored in connection-manager (<code className="text-xs font-mono px-1 rounded bg-slate-200/90 dark:bg-slate-800">vulnerability_findings</code>). It is intentionally
                        separate from alert triage, risk scoring, and patch dashboards so each module keeps a single responsibility.
                    </>
                }
            />

            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">vs Alerts</div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/alerts">
                            Alerts
                        </Link>{' '}
                        are detection outcomes (Sigma / rules). They are not a substitute for a structured CVE inventory.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">vs Endpoint risk</div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/endpoint-risk">
                            Endpoint risk
                        </Link>{' '}
                        ranks hosts using alert-derived scores — complementary to, not the same as, CVE rows below.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">vs Detection rules</div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/rules">
                            Detection rules
                        </Link>{' '}
                        define how events become alerts. Vulnerability rows track missing patches / CVE exposure metadata.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">Responses &amp; devices</div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        Use{' '}
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/responses">
                            Command Center
                        </Link>{' '}
                        for live response. Use{' '}
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/management/devices">
                            Devices
                        </Link>{' '}
                        for host inventory — open a host from a finding row when you need actions beyond status triage.
                    </p>
                </div>
            </div>

            {findingsQ.isLoading && <div className="h-40 rounded-xl bg-slate-100 dark:bg-slate-800 animate-pulse" />}

            {findingsQ.isError && (
                <div className="rounded-xl border border-rose-200 dark:border-rose-900/50 bg-rose-50/80 dark:bg-rose-950/20 p-4 text-sm text-rose-900 dark:text-rose-200">
                    Could not load vulnerability findings. Confirm connection-manager is reachable and your role includes{' '}
                    <code className="text-xs">endpoints:read</code>.
                </div>
            )}

            {!findingsQ.isLoading && !findingsQ.isError && (
                <>
                    <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                        <StatCard title="Total findings (page)" value={String(total)} icon={Shield} color="cyan" />
                        <StatCard title="Open (critical)" value={String(stats.openCrit)} icon={AlertTriangle} color="red" />
                        <StatCard title="Open (high)" value={String(stats.openHigh)} icon={AlertTriangle} color="amber" />
                        <StatCard title="Hosts w/ open findings" value={String(stats.hostsAffected)} icon={Radio} color="emerald" />
                    </div>

                    <div className="flex flex-col lg:flex-row lg:flex-wrap gap-3 items-stretch lg:items-end rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                        <div className="flex-1 min-w-[160px]">
                            <label className="block text-xs font-semibold text-slate-500 uppercase mb-1">Search</label>
                            <input
                                value={search}
                                onChange={(e) => setSearch(e.target.value)}
                                placeholder="CVE, title, package, hostname…"
                                className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-3 py-2 text-sm"
                            />
                        </div>
                        <div className="w-full sm:w-40">
                            <label className="block text-xs font-semibold text-slate-500 uppercase mb-1">Severity</label>
                            <select
                                value={severityFilter}
                                onChange={(e) => setSeverityFilter(e.target.value)}
                                className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-3 py-2 text-sm"
                            >
                                <option value="">All</option>
                                <option value="critical">Critical</option>
                                <option value="high">High</option>
                                <option value="medium">Medium</option>
                                <option value="low">Low</option>
                                <option value="informational">Informational</option>
                            </select>
                        </div>
                        <div className="w-full sm:w-44">
                            <label className="block text-xs font-semibold text-slate-500 uppercase mb-1">Workflow status</label>
                            <select
                                value={statusFilter}
                                onChange={(e) => setStatusFilter(e.target.value)}
                                className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-3 py-2 text-sm"
                            >
                                <option value="">All</option>
                                <option value="open">Open</option>
                                <option value="acknowledged">Acknowledged</option>
                                <option value="resolved">Resolved</option>
                                <option value="risk_accepted">Risk accepted</option>
                            </select>
                        </div>
                    </div>

                    {rows.length === 0 ? (
                        <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-slate-50/80 dark:bg-slate-900/40 p-8 text-center space-y-2">
                            <p className="text-sm font-medium text-slate-800 dark:text-slate-100">No vulnerability rows yet</p>
                            <p className="text-xs text-slate-600 dark:text-slate-400 max-w-lg mx-auto">
                                Data is read live from <code className="text-[11px]">GET /api/v1/vuln/findings</code>. Populate{' '}
                                <code className="text-[11px]">vulnerability_findings</code> via your scanner pipeline or SQL import; when rows exist, they appear here
                                immediately — no mock layer.
                            </p>
                        </div>
                    ) : (
                        <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm shadow-sm">
                            <table className="min-w-full text-left text-sm">
                                <thead className="bg-slate-50 dark:bg-slate-800/80 text-slate-600 dark:text-slate-400 text-xs uppercase">
                                    <tr>
                                        <th className="px-3 py-2">Severity</th>
                                        <th className="px-3 py-2">CVE / title</th>
                                        <th className="px-3 py-2 hidden md:table-cell">Package</th>
                                        <th className="px-3 py-2">Host</th>
                                        <th className="px-3 py-2 text-right hidden lg:table-cell">CVSS</th>
                                        <th className="px-3 py-2">Detected</th>
                                        <th className="px-3 py-2 hidden xl:table-cell">Source</th>
                                        <th className="px-3 py-2">Status</th>
                                        <th className="px-3 py-2 text-right">Device</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {rows.map((f) => (
                                        <tr key={f.id} className="border-t border-slate-100 dark:border-slate-800">
                                            <td className="px-3 py-2">
                                                <span className={`inline-flex rounded-full px-2 py-0.5 text-[11px] font-semibold capitalize ${vulnSeverityClass(f.severity)}`}>
                                                    {f.severity}
                                                </span>
                                            </td>
                                            <td className="px-3 py-2 max-w-xs">
                                                <div className="font-medium text-slate-900 dark:text-white">{f.cve || '—'}</div>
                                                <div className="text-xs text-slate-600 dark:text-slate-300 line-clamp-2">{f.title}</div>
                                            </td>
                                            <td className="px-3 py-2 text-xs hidden md:table-cell">
                                                <div className="text-slate-800 dark:text-slate-100">{f.package_name || '—'}</div>
                                                {f.fixed_version ? (
                                                    <div className="text-slate-500 dark:text-slate-400">Fix: {f.fixed_version}</div>
                                                ) : null}
                                            </td>
                                            <td className="px-3 py-2 text-sm">
                                                <span className="text-slate-800 dark:text-slate-100">{f.hostname}</span>
                                            </td>
                                            <td className="px-3 py-2 text-right tabular-nums hidden lg:table-cell">{f.cvss != null ? f.cvss.toFixed(1) : '—'}</td>
                                            <td className="px-3 py-2 text-xs whitespace-nowrap">{formatRelativeTime(f.detected_at)}</td>
                                            <td className="px-3 py-2 text-xs text-slate-500 hidden xl:table-cell font-mono">{f.source}</td>
                                            <td className="px-3 py-2">
                                                {canManageEndpoints ? (
                                                    <select
                                                        value={f.status}
                                                        disabled={patchStatus.isPending}
                                                        onChange={(e) => patchStatus.mutate({ id: f.id, status: e.target.value })}
                                                        className="max-w-[140px] rounded-md border border-slate-200 dark:border-slate-600 bg-white dark:bg-slate-950 px-2 py-1 text-xs"
                                                    >
                                                        {VULN_STATUS_OPTIONS.map((s) => (
                                                            <option key={s} value={s}>
                                                                {s.replace(/_/g, ' ')}
                                                            </option>
                                                        ))}
                                                    </select>
                                                ) : (
                                                    <span className="text-xs font-mono text-slate-600 dark:text-slate-300">{f.status}</span>
                                                )}
                                            </td>
                                            <td className="px-3 py-2 text-right">
                                                <Link
                                                    className="text-cyan-600 dark:text-cyan-400 text-xs font-medium hover:underline whitespace-nowrap"
                                                    to={`/management/devices/${encodeURIComponent(f.agent_id)}`}
                                                >
                                                    Open →
                                                </Link>
                                            </td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    )}
                </>
            )}

            <p className="text-xs text-slate-500 dark:text-slate-400">
                API: <code className="text-[11px]">GET/PATCH /api/v1/vuln/findings</code> on connection-manager. Ingestion from agents or external scanners can insert into{' '}
                <code className="text-[11px]">vulnerability_findings</code>; UI always reflects database state.
            </p>
        </div>
    );
}

function appControlModeBadge(mode: AppControlPolicy['mode']) {
    if (mode === 'enforce') {
        return (
            <span className="inline-flex items-center gap-1 rounded-full bg-rose-100 text-rose-800 dark:bg-rose-900/35 dark:text-rose-200 px-2 py-0.5 text-[11px] font-semibold">
                <Ban className="w-3 h-3" />
                Enforce
            </span>
        );
    }
    return (
        <span className="inline-flex items-center gap-1 rounded-full bg-amber-100 text-amber-900 dark:bg-amber-900/30 dark:text-amber-100 px-2 py-0.5 text-[11px] font-semibold">
            <Eye className="w-3 h-3" />
            Audit
        </span>
    );
}

function appControlStateBadge(state: AppControlPolicy['state']) {
    return (
        <span
            className={
                state === 'published'
                    ? 'rounded-full bg-emerald-100 text-emerald-800 dark:bg-emerald-900/35 dark:text-emerald-200 px-2 py-0.5 text-[11px] font-semibold'
                    : 'rounded-full bg-slate-200 text-slate-800 dark:bg-slate-700 dark:text-slate-200 px-2 py-0.5 text-[11px] font-semibold'
            }
        >
            {state === 'published' ? 'Published' : 'Draft'}
        </span>
    );
}

export function ManagementAppControlPage() {
    const { showToast } = useToast();
    const queryClient = useQueryClient();
    const canExec = authApi.canExecuteCommands();
    const [agentId, setAgentId] = React.useState('');
    const [policyJson, setPolicyJson] = React.useState(
        JSON.stringify(
            {
                mode: 'audit',
                allow_paths: ['C:\\\\Program Files\\\\'],
                deny_hashes: [],
            },
            null,
            2
        )
    );

    const policiesQ = useParityQuery<AppControlPoliciesPayload>(
        ['management', 'application-control', 'policies'],
        () => parityApi.getAppControlPolicies(),
        mocks.mockAppControlPolicies,
        { staleTime: 30_000, refetchInterval: 60_000 }
    );

    const push = useMutation({
        mutationFn: async () => {
            const aid = agentId.trim();
            if (!aid) throw new Error('agent_id is required');
            JSON.parse(policyJson);
            return agentsApi.executeCommand(aid, {
                command_type: 'update_config',
                parameters: { app_control_policy_json: policyJson },
                timeout: 300,
            });
        },
        onSuccess: (d) => {
            showToast(`Policy payload pushed (Command ID: ${d.command_id})`, 'success');
            queryClient.invalidateQueries({ queryKey: ['agent-commands', agentId.trim()] });
        },
        onError: (e: Error) => showToast(e.message || 'Failed to push policy', 'error'),
    });

    const parityResult = policiesQ.data;
    const acPayload = parityResult?.data;
    const policies = acPayload?.data ?? [];
    const auditSummary = acPayload?.audit_summary;
    const rollout = acPayload?.rollout_preview ?? [];

    const stats = useMemo(() => {
        const enforce = policies.filter((p) => p.mode === 'enforce').length;
        const audit = policies.filter((p) => p.mode === 'audit').length;
        const published = policies.filter((p) => p.state === 'published').length;
        const rules = policies.reduce((s, p) => s + p.rule_count, 0);
        return { enforce, audit, published, rules };
    }, [policies]);

    if (policiesQ.isLoading) {
        return (
            <div className="flex items-center justify-center py-16 text-slate-500 gap-2 animate-slide-up-fade">
                <Loader2 className="w-6 h-6 animate-spin" /> Loading application control…
            </div>
        );
    }

    if (policiesQ.isError) {
        return (
            <div className="rounded-lg border border-rose-200 dark:border-rose-900/50 p-4 text-sm text-rose-800 dark:text-rose-200 animate-slide-up-fade">
                Could not load application control policies.
            </div>
        );
    }

    return (
        <div className="space-y-5 animate-slide-up-fade w-full min-w-0">
            <InsightHero
                variant="light"
                accent="emerald"
                icon={Shield}
                title="Application control policies"
                lead={
                    <>
                        Author and track <strong className="font-semibold text-slate-800 dark:text-slate-200">who may run which code</strong> on endpoints (allowlist / blocklist by path,
                        publisher, hash, and scope). This module owns <strong className="font-semibold text-slate-800 dark:text-slate-200">execution policy</strong> — not threat detection
                        signatures and not telemetry scoring filters.
                    </>
                }
            />

            {parityResult?.isMock && (
                <ParityMockBanner missingApi="GET /api/v1/management/application-control/policies" />
            )}

            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
                        <BookOpen className="w-4 h-4 text-cyan-500 shrink-0" />
                        vs Detection Rules
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/rules">
                            Detection Rules
                        </Link>{' '}
                        manage Sigma and behavioral <em>detections</em> (alerts). They do not grant or deny the right to execute a binary.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
                        <Shield className="w-4 h-4 text-amber-500 shrink-0" />
                        vs Context policies
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/management/context-policies">
                            Context policies
                        </Link>{' '}
                        tune risk context for triage (weights, trusted networks). They are not application execution allow/block lists.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
                        <Activity className="w-4 h-4 text-emerald-500 shrink-0" />
                        vs Telemetry Search
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/events">
                            Telemetry Search
                        </Link>{' '}
                        is for investigating raw events. Use it after policy changes; keep policy inventory and rollout here.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
                        <Radio className="w-4 h-4 text-violet-500 shrink-0" />
                        vs Devices (Fleet)
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/management/devices">
                            Devices
                        </Link>{' '}
                        lists hosts and health. This page summarizes <em>policy</em> coverage and sync; open a device for host-level actions, not rule authoring.
                    </p>
                </div>
            </div>

            <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                <StatCard title="Policies" value={String(policies.length)} icon={Shield} color="cyan" />
                <StatCard title="Published" value={String(stats.published)} icon={Activity} color="emerald" />
                <StatCard title="Enforce / Audit" value={`${stats.enforce} / ${stats.audit}`} icon={Ban} color="amber" />
                <StatCard title="Rule rows (sum)" value={String(stats.rules)} icon={BookOpen} />
            </div>

            {auditSummary && (
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 p-4 shadow-sm">
                    <h3 className="text-sm font-semibold text-slate-900 dark:text-white flex items-center gap-2">
                        <Eye className="w-4 h-4 text-amber-500" />
                        Audit-mode signal (aggregate)
                    </h3>
                    <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                        Counts reflect policies running in <strong>audit</strong> only — they estimate what would have been blocked without replacing alert triage on the Alerts page.
                    </p>
                    <div className="mt-3 flex flex-wrap gap-6 text-sm">
                        <div>
                            <span className="text-slate-500 dark:text-slate-400">Would-block events (7d)</span>
                            <p className="text-lg font-semibold tabular-nums text-slate-900 dark:text-white">{auditSummary.would_block_events_7d}</p>
                        </div>
                        <div>
                            <span className="text-slate-500 dark:text-slate-400">Distinct binaries</span>
                            <p className="text-lg font-semibold tabular-nums text-slate-900 dark:text-white">{auditSummary.distinct_binaries_touched}</p>
                        </div>
                    </div>
                </div>
            )}

            <div>
                <h3 className="text-sm font-semibold text-slate-900 dark:text-white mb-2">Policy inventory</h3>
                <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm shadow-sm">
                    <table className="min-w-full text-left text-sm">
                        <thead className="bg-slate-50 dark:bg-slate-800/80 text-slate-600 dark:text-slate-400 text-xs uppercase">
                            <tr>
                                <th className="px-3 py-2">Policy</th>
                                <th className="px-3 py-2">Scope</th>
                                <th className="px-3 py-2">Mode</th>
                                <th className="px-3 py-2">State</th>
                                <th className="px-3 py-2 text-right">Priority</th>
                                <th className="px-3 py-2 text-right">Rules</th>
                                <th className="px-3 py-2 text-right">Coverage</th>
                                <th className="px-3 py-2 text-right hidden lg:table-cell">Synced / Lag</th>
                                <th className="px-3 py-2 text-right hidden md:table-cell">7d blocks</th>
                                <th className="px-3 py-2 hidden xl:table-cell">Updated</th>
                            </tr>
                        </thead>
                        <tbody>
                            {[...policies]
                                .sort((a, b) => a.priority - b.priority)
                                .map((p) => (
                                    <tr key={p.id} className="border-t border-slate-100 dark:border-slate-800">
                                        <td className="px-3 py-2 align-top">
                                            <div className="font-medium text-slate-900 dark:text-white">{p.name}</div>
                                            {p.description && (
                                                <div className="text-xs text-slate-500 dark:text-slate-400 mt-0.5 max-w-md">{p.description}</div>
                                            )}
                                        </td>
                                        <td className="px-3 py-2 align-top text-xs">
                                            <span className="font-mono text-[10px] uppercase text-slate-500 dark:text-slate-400">{p.scope_type}</span>
                                            <div className="text-slate-700 dark:text-slate-200 mt-0.5">{p.scope_label}</div>
                                        </td>
                                        <td className="px-3 py-2 align-top">{appControlModeBadge(p.mode)}</td>
                                        <td className="px-3 py-2 align-top">{appControlStateBadge(p.state)}</td>
                                        <td className="px-3 py-2 text-right tabular-nums align-top">{p.priority}</td>
                                        <td className="px-3 py-2 text-right tabular-nums align-top">{p.rule_count}</td>
                                        <td className="px-3 py-2 text-right tabular-nums align-top">{p.coverage_percent}%</td>
                                        <td className="px-3 py-2 text-right tabular-nums align-top hidden lg:table-cell">
                                            {p.endpoints_synced} / {p.endpoints_lagged}
                                        </td>
                                        <td className="px-3 py-2 text-right tabular-nums align-top hidden md:table-cell">
                                            {p.mode === 'enforce' ? p.enforce_blocks_7d : p.audit_only_blocks_7d}
                                        </td>
                                        <td className="px-3 py-2 text-xs text-slate-500 dark:text-slate-400 hidden xl:table-cell whitespace-nowrap">
                                            {formatRelativeTime(p.updated_at)}
                                        </td>
                                    </tr>
                                ))}
                        </tbody>
                    </table>
                </div>
                <p className="text-xs text-slate-500 dark:text-slate-400 mt-2">
                    Priority resolves overlaps when multiple policies apply (lower number wins first). Fleet authoring UI is read-only until the management API accepts creates/updates.
                </p>
            </div>

            {rollout.length > 0 && (
                <div>
                    <h3 className="text-sm font-semibold text-slate-900 dark:text-white mb-2">Rollout snapshot (policy sync)</h3>
                    <p className="text-xs text-slate-500 dark:text-slate-400 mb-2 max-w-3xl">
                        Short sample of hosts and policy materialization — not the full fleet grid (see Devices for inventory and search).
                    </p>
                    <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 shadow-sm">
                        <table className="min-w-full text-left text-sm">
                            <thead className="bg-slate-50 dark:bg-slate-800/80 text-slate-600 dark:text-slate-400 text-xs uppercase">
                                <tr>
                                    <th className="px-3 py-2">Host</th>
                                    <th className="px-3 py-2">Sync</th>
                                    <th className="px-3 py-2">Last policy sync</th>
                                    <th className="px-3 py-2" />
                                </tr>
                            </thead>
                            <tbody>
                                {rollout.map((r) => (
                                    <tr key={r.agent_id} className="border-t border-slate-100 dark:border-slate-800">
                                        <td className="px-3 py-2 font-medium text-slate-900 dark:text-white">{r.hostname}</td>
                                        <td className="px-3 py-2">
                                            {r.policy_sync === 'ok' && (
                                                <span className="text-emerald-600 dark:text-emerald-400 text-xs font-semibold">OK</span>
                                            )}
                                            {r.policy_sync === 'lagging' && (
                                                <span className="text-amber-600 dark:text-amber-400 text-xs font-semibold">Lagging</span>
                                            )}
                                            {r.policy_sync === 'unknown' && (
                                                <span className="text-slate-500 dark:text-slate-400 text-xs">Unknown</span>
                                            )}
                                        </td>
                                        <td className="px-3 py-2 text-xs text-slate-600 dark:text-slate-300">
                                            {r.last_policy_sync_at ? formatRelativeTime(r.last_policy_sync_at) : '—'}
                                        </td>
                                        <td className="px-3 py-2 text-right">
                                            <Link
                                                className="text-cyan-600 dark:text-cyan-400 text-xs font-medium hover:underline"
                                                to={`/management/devices/${encodeURIComponent(r.agent_id)}`}
                                            >
                                                Device →
                                            </Link>
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                </div>
            )}

            <details className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-slate-50/80 dark:bg-slate-900/40 shadow-sm group">
                <summary className="cursor-pointer list-none px-4 py-3 flex items-center justify-between gap-2">
                    <span className="text-sm font-semibold text-slate-900 dark:text-white flex items-center gap-2">
                        <AlertTriangle className="w-4 h-4 text-amber-500 shrink-0" />
                        Advanced: targeted config push (single agent)
                    </span>
                    <span className="text-xs text-slate-500 dark:text-slate-400">Operator escape hatch — not fleet CRUD</span>
                </summary>
                <div className="px-4 pb-4 pt-0 border-t border-slate-200 dark:border-slate-700/60 space-y-3">
                    <p className="text-xs text-slate-600 dark:text-slate-400 pt-3">
                        Pushes raw JSON to one agent through <code className="text-[10px]">update_config</code> (connection-manager). Use when no central policy API exists yet; prefer inventory above once
                        writes are supported.
                    </p>
                    <label className="block text-xs font-semibold text-slate-500 uppercase">Target agent_id</label>
                    <input
                        className="w-full max-w-xl rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-3 py-2 text-sm font-mono"
                        value={agentId}
                        onChange={(e) => setAgentId(e.target.value)}
                        placeholder="UUID"
                    />

                    <label className="block text-xs font-semibold text-slate-500 uppercase">Policy JSON</label>
                    <textarea
                        className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-3 py-2 text-xs font-mono min-h-[200px]"
                        value={policyJson}
                        onChange={(e) => setPolicyJson(e.target.value)}
                        spellCheck={false}
                    />

                    <div className="flex justify-end gap-2">
                        <button
                            type="button"
                            disabled={!canExec || push.isPending}
                            onClick={() => push.mutate()}
                            className="px-3 py-2 rounded-lg text-sm font-semibold bg-cyan-600 hover:bg-cyan-700 text-white disabled:opacity-50"
                        >
                            {push.isPending ? 'Pushing…' : 'Push to agent'}
                        </button>
                    </div>
                </div>
            </details>
        </div>
    );
}

export function ManagementLicensesPage() {
    return <SelfHostedOutOfScope title="Licenses" />;
}

export function ManagementBillingPage() {
    return <SelfHostedOutOfScope title="Billing" />;
}

