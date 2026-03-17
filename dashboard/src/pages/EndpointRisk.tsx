import { useQuery } from '@tanstack/react-query';
import React, { useState, useMemo } from 'react';
import {
    Cpu, AlertTriangle, TrendingUp, Shield, Search,
    ChevronRight, Activity, Clock, Zap,
    Monitor, RefreshCw
} from 'lucide-react';
import { alertsApi, agentsApi, type EndpointRiskSummary, type Agent } from '../api/client';
import { SkeletonKPICards, SkeletonChart } from '../components';

// =========================================================================
// Risk score helpers
// =========================================================================

function getRiskTier(score: number): { label: string; color: string; bg: string; glow: string; barColor: string } {
    if (score >= 90) return {
        label: 'CRITICAL',
        color: 'text-rose-400',
        bg: 'bg-rose-500/10 border-rose-500/30',
        glow: 'shadow-[0_0_20px_rgba(244,63,94,0.25)]',
        barColor: 'bg-rose-500',
    };
    if (score >= 70) return {
        label: 'HIGH',
        color: 'text-orange-400',
        bg: 'bg-orange-500/10 border-orange-500/30',
        glow: 'shadow-[0_0_16px_rgba(249,115,22,0.2)]',
        barColor: 'bg-orange-500',
    };
    if (score >= 40) return {
        label: 'MEDIUM',
        color: 'text-amber-400',
        bg: 'bg-amber-500/10 border-amber-500/30',
        glow: '',
        barColor: 'bg-amber-500',
    };
    return {
        label: 'LOW',
        color: 'text-emerald-400',
        bg: 'bg-emerald-500/10 border-emerald-500/30',
        glow: '',
        barColor: 'bg-emerald-500',
    };
}

function formatRelativeTime(iso: string): string {
    const diff = Date.now() - new Date(iso).getTime();
    const mins = Math.floor(diff / 60000);
    if (mins < 1) return 'just now';
    if (mins < 60) return `${mins}m ago`;
    const hrs = Math.floor(mins / 60);
    if (hrs < 24) return `${hrs}h ago`;
    return `${Math.floor(hrs / 24)}d ago`;
}

// =========================================================================
// KPI Card
// =========================================================================

const KPICard = React.memo(function KPICard({
    label, value, sub, icon: Icon, accent
}: {
    label: string; value: string | number; sub?: string;
    icon: typeof Shield; accent: string;
}) {
    return (
        <div className={`relative overflow-hidden rounded-xl border bg-white/60 dark:bg-slate-900/40 backdrop-blur-md p-5 shadow-sm transition-all hover:shadow-md group ${accent}`}>
            <div className="absolute -top-8 -right-8 w-28 h-28 rounded-full blur-3xl opacity-20 group-hover:opacity-35 transition-opacity pointer-events-none bg-current" />
            <div className="relative z-10 flex items-start gap-4">
                <div className={`p-3 rounded-lg ${accent} shrink-0`}>
                    <Icon className="w-5 h-5" />
                </div>
                <div>
                    <p className="text-xs font-bold text-slate-500 dark:text-slate-400 uppercase tracking-widest mb-1">{label}</p>
                    <p className="text-2xl font-bold text-slate-900 dark:text-white">{value}</p>
                    {sub && <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{sub}</p>}
                </div>
            </div>
        </div>
    );
});

// =========================================================================
// Risk Score Arc (mini radial visual)
// =========================================================================

const RiskArc = React.memo(function RiskArc({ score }: { score: number }) {
    const tier = getRiskTier(score);
    const r = 22;
    const circ = 2 * Math.PI * r;
    const filled = (score / 100) * circ;

    return (
        <div className="relative flex items-center justify-center w-16 h-16 shrink-0">
            <svg className="absolute inset-0 w-16 h-16 -rotate-90" viewBox="0 0 56 56">
                <circle cx="28" cy="28" r={r} stroke="rgba(100,116,139,0.15)" strokeWidth="5" fill="none" />
                <circle
                    cx="28" cy="28" r={r}
                    stroke={score >= 90 ? '#f43f5e' : score >= 70 ? '#f97316' : score >= 40 ? '#f59e0b' : '#10b981'}
                    strokeWidth="5" fill="none"
                    strokeDasharray={`${filled} ${circ}`}
                    strokeLinecap="round"
                    style={{ transition: 'stroke-dasharray 0.6s ease' }}
                />
            </svg>
            <div className="flex flex-col items-center">
                <span className={`text-sm font-bold leading-none ${tier.color}`}>{score}</span>
                <span className="text-[9px] text-slate-500 font-semibold leading-none mt-0.5">/ 100</span>
            </div>
        </div>
    );
});

// =========================================================================
// Endpoint Risk Card
// =========================================================================

interface MergedEndpoint extends EndpointRiskSummary {
    hostname?: string;
    os_type?: string;
    agent_status?: string;
}

const EndpointRiskCard = React.memo(function EndpointRiskCard({ ep, rank }: { ep: MergedEndpoint; rank: number }) {
    const tier = getRiskTier(ep.peak_risk_score);

    return (
        <div className={`group relative flex items-center gap-4 p-4 rounded-xl border bg-white dark:bg-slate-800/60 hover:shadow-lg transition-all duration-300 cursor-default ${tier.glow} border-slate-200 dark:border-slate-700 hover:border-slate-300 dark:hover:border-slate-600`}>
            {/* Rank badge */}
            <div className="shrink-0 w-8 h-8 rounded-full bg-slate-100 dark:bg-slate-900/60 flex items-center justify-center">
                <span className="text-xs font-bold text-slate-500 dark:text-slate-400">{rank}</span>
            </div>

            {/* Risk arc */}
            <RiskArc score={ep.peak_risk_score} />

            {/* Main info */}
            <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 mb-1">
                    <Monitor className="w-3.5 h-3.5 text-slate-400 shrink-0" />
                    <span className="text-sm font-bold text-slate-900 dark:text-slate-100 truncate">
                        {ep.hostname || ep.agent_id.slice(0, 8) + '…'}
                    </span>
                    <span className={`px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wider rounded border ${tier.bg} ${tier.color}`}>
                        {tier.label}
                    </span>
                    {ep.agent_status && (
                        <span className={`ml-auto px-1.5 py-0.5 text-[9px] font-semibold rounded-full ${ep.agent_status === 'online'
                            ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
                            : 'bg-slate-500/10 text-slate-400 border border-slate-500/20'
                            }`}>
                            {ep.agent_status}
                        </span>
                    )}
                </div>

                {/* Stat pills */}
                <div className="flex flex-wrap items-center gap-2 text-xs">
                    {ep.critical_count > 0 && (
                        <span className="flex items-center gap-1 px-2 py-0.5 bg-rose-500/10 text-rose-400 rounded-full border border-rose-500/20 font-semibold">
                            <span className="w-1.5 h-1.5 bg-rose-400 rounded-full animate-pulse" />
                            {ep.critical_count} critical
                        </span>
                    )}
                    {ep.high_count > 0 && (
                        <span className="flex items-center gap-1 px-2 py-0.5 bg-orange-500/10 text-orange-400 rounded-full border border-orange-500/20 font-semibold">
                            {ep.high_count} high
                        </span>
                    )}
                    <span className="flex items-center gap-1 px-2 py-0.5 bg-slate-100 dark:bg-slate-900/60 text-slate-500 rounded-full border border-slate-200 dark:border-slate-700">
                        <AlertTriangle className="w-3 h-3" />
                        {ep.open_count} open
                    </span>
                    <span className="flex items-center gap-1 px-2 py-0.5 bg-slate-100 dark:bg-slate-900/60 text-slate-500 rounded-full border border-slate-200 dark:border-slate-700">
                        <Clock className="w-3 h-3" />
                        {formatRelativeTime(ep.last_alert_at)}
                    </span>
                </div>
            </div>

            {/* Risk bar column */}
            <div className="shrink-0 flex flex-col items-end gap-1.5 w-24">
                <div className="flex items-baseline gap-1">
                    <span className={`text-xs font-bold ${tier.color}`}>avg</span>
                    <span className={`text-sm font-bold ${tier.color}`}>{ep.avg_risk_score.toFixed(0)}</span>
                </div>
                <div className="w-24 h-1.5 bg-slate-200 dark:bg-slate-700 rounded-full overflow-hidden">
                    <div
                        className={`h-full rounded-full transition-all duration-700 ${tier.barColor}`}
                        style={{ width: `${ep.avg_risk_score}%` }}
                    />
                </div>
                <span className="text-[10px] text-slate-400">{ep.total_alerts} alerts total</span>
            </div>

            <ChevronRight className="w-4 h-4 text-slate-400 group-hover:text-slate-600 dark:group-hover:text-slate-300 shrink-0 transition-colors" />
        </div>
    );
});

// =========================================================================
// Distribution bar chart (inline, no recharts dep)
// =========================================================================

function RiskDistributionBar({ summaries }: { summaries: EndpointRiskSummary[] }) {
    const counts = useMemo(() => {
        const c = { critical: 0, high: 0, medium: 0, low: 0 };
        summaries.forEach(s => {
            if (s.peak_risk_score >= 90) c.critical++;
            else if (s.peak_risk_score >= 70) c.high++;
            else if (s.peak_risk_score >= 40) c.medium++;
            else c.low++;
        });
        return c;
    }, [summaries]);

    const total = summaries.length || 1;
    const bars = [
        { label: 'Critical', count: counts.critical, color: 'bg-rose-500', text: 'text-rose-400' },
        { label: 'High', count: counts.high, color: 'bg-orange-500', text: 'text-orange-400' },
        { label: 'Medium', count: counts.medium, color: 'bg-amber-500', text: 'text-amber-400' },
        { label: 'Low / Clean', count: counts.low, color: 'bg-emerald-500', text: 'text-emerald-400' },
    ];

    return (
        <div className="relative bg-white/60 dark:bg-slate-900/40 backdrop-blur-md border border-slate-200/80 dark:border-slate-700/50 rounded-xl p-6 shadow-sm">
            <h3 className="text-sm font-bold text-slate-500 dark:text-slate-400 uppercase tracking-widest mb-4">Risk Distribution</h3>
            <div className="flex h-2 rounded-full overflow-hidden gap-0.5 mb-4">
                {bars.map(b => b.count > 0 && (
                    <div
                        key={b.label}
                        className={`${b.color} rounded-full transition-all duration-700`}
                        style={{ width: `${(b.count / total) * 100}%` }}
                        title={`${b.label}: ${b.count}`}
                    />
                ))}
            </div>
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
                {bars.map(b => (
                    <div key={b.label} className="text-center">
                        <div className={`text-2xl font-bold ${b.text}`}>{b.count}</div>
                        <div className="text-xs text-slate-500 mt-0.5">{b.label}</div>
                    </div>
                ))}
            </div>
        </div>
    );
}

// =========================================================================
// Empty state
// =========================================================================

function EmptyState() {
    return (
        <div className="flex flex-col items-center justify-center py-24 text-center">
            <div className="w-20 h-20 rounded-full bg-slate-100 dark:bg-slate-800 flex items-center justify-center mb-6 shadow-inner">
                <Shield className="w-10 h-10 text-slate-400" />
            </div>
            <h3 className="text-lg font-bold text-slate-900 dark:text-white mb-2">No Risk Data Yet</h3>
            <p className="text-sm text-slate-500 dark:text-slate-400 max-w-sm">
                Once the Sigma engine detects and scores alerts, each endpoint's risk posture will appear here ranked by peak risk score.
            </p>
        </div>
    );
}

// =========================================================================
// Main Page
// =========================================================================

export default function EndpointRisk() {
    const [search, setSearch] = useState('');
    const [tierFilter, setTierFilter] = useState<'all' | 'critical' | 'high' | 'medium' | 'low'>('all');

    const { data: riskData, isLoading: riskLoading, refetch, isFetching } = useQuery({
        queryKey: ['endpointRisk'],
        queryFn: () => alertsApi.endpointRisk(),
        refetchInterval: 30000,
    });

    const { data: agentsData } = useQuery({
        queryKey: ['agents', 'all'],
        queryFn: () => agentsApi.list({ limit: 500 }),
    });

    // Merge risk data with agent metadata
    const merged: MergedEndpoint[] = useMemo(() => {
        const summaries = riskData?.data ?? [];
        const agentMap = new Map<string, Agent>();
        agentsData?.data?.forEach(a => agentMap.set(a.id, a));

        return summaries.map(s => ({
            ...s,
            hostname: agentMap.get(s.agent_id)?.hostname,
            os_type: agentMap.get(s.agent_id)?.os_type,
            agent_status: agentMap.get(s.agent_id)?.status,
        }));
    }, [riskData, agentsData]);

    // Filter
    const filtered = useMemo(() => {
        return merged.filter(ep => {
            const q = search.toLowerCase();
            const matchesSearch = !q ||
                ep.hostname?.toLowerCase().includes(q) ||
                ep.agent_id.toLowerCase().includes(q);
            const score = ep.peak_risk_score;
            const matchesTier =
                tierFilter === 'all' ? true :
                    tierFilter === 'critical' ? score >= 90 :
                        tierFilter === 'high' ? score >= 70 && score < 90 :
                            tierFilter === 'medium' ? score >= 40 && score < 70 :
                                score < 40;
            return matchesSearch && matchesTier;
        });
    }, [merged, search, tierFilter]);

    // KPI summary
    const kpis = useMemo(() => {
        const data = riskData?.data ?? [];
        return {
            total: data.length,
            critical: data.filter(d => d.peak_risk_score >= 90).length,
            highRisk: data.filter(d => d.peak_risk_score >= 70).length,
            totalOpen: data.reduce((acc, d) => acc + d.open_count, 0),
        };
    }, [riskData]);

    if (riskLoading) {
        return (
            <div className="space-y-6">
                <div className="h-9 w-72 bg-gray-200 dark:bg-gray-700 rounded animate-pulse" />
                <SkeletonKPICards count={4} />
                <SkeletonChart height={200} />
            </div>
        );
    }

    const tierButtons = [
        { key: 'all', label: 'All' },
        { key: 'critical', label: 'Critical', color: 'text-rose-500' },
        { key: 'high', label: 'High', color: 'text-orange-500' },
        { key: 'medium', label: 'Medium', color: 'text-amber-500' },
        { key: 'low', label: 'Low / Clean', color: 'text-emerald-500' },
    ] as const;

    return (
        <div className="relative flex flex-col min-h-[calc(100vh-5rem)] lg:min-h-[calc(100vh-3.5rem)] -mx-4 sm:-mx-6 lg:-mx-8 -my-4 sm:-my-6 lg:-my-8 p-4 sm:p-6 lg:p-8 bg-slate-50 dark:bg-gradient-to-br dark:from-slate-900 dark:via-[#0b1120] dark:to-slate-900 transition-colors">
            {/* Ambient glows */}
            <div className="absolute top-0 right-0 w-[500px] h-[500px] pointer-events-none" style={{ background: 'radial-gradient(circle, rgba(244,63,94,0.05) 0%, transparent 70%)' }} />
            <div className="absolute bottom-0 left-0 w-[500px] h-[500px] pointer-events-none" style={{ background: 'radial-gradient(circle, rgba(99,102,241,0.05) 0%, transparent 70%)' }} />

            <div className="relative max-w-[1600px] mx-auto w-full space-y-6">

                {/* Header */}
                <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                    <div>
                        <h1 className="text-2xl font-bold bg-clip-text text-transparent bg-gradient-to-r from-gray-900 to-gray-600 dark:from-white dark:to-gray-300">
                            Endpoint Risk Intelligence
                        </h1>
                        <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                            Live risk posture ranked by context-aware scoring · refreshes every 30s
                        </p>
                    </div>
                    <button
                        onClick={() => refetch()}
                        disabled={isFetching}
                        className="flex items-center gap-2 px-4 py-2 bg-slate-800 hover:bg-slate-700 disabled:opacity-50 text-white text-sm font-medium rounded-lg transition-colors shadow-sm"
                    >
                        <RefreshCw className={`w-4 h-4 ${isFetching ? 'animate-spin' : ''}`} />
                        Refresh
                    </button>
                </div>

                {/* KPI Cards */}
                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
                    <KPICard
                        label="Active Endpoints"
                        value={kpis.total}
                        sub="with open alert activity"
                        icon={Monitor}
                        accent="bg-indigo-500/10 text-indigo-600 dark:text-indigo-400 border-indigo-500/20"
                    />
                    <KPICard
                        label="Critical Risk"
                        value={kpis.critical}
                        sub="peak score ≥ 90"
                        icon={Zap}
                        accent="bg-rose-500/10 text-rose-600 dark:text-rose-400 border-rose-500/20"
                    />
                    <KPICard
                        label="High Risk or Above"
                        value={kpis.highRisk}
                        sub="peak score ≥ 70"
                        icon={TrendingUp}
                        accent="bg-orange-500/10 text-orange-600 dark:text-orange-400 border-orange-500/20"
                    />
                    <KPICard
                        label="Total Open Alerts"
                        value={kpis.totalOpen}
                        sub="across all endpoints"
                        icon={Activity}
                        accent="bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20"
                    />
                </div>

                {/* Distribution chart */}
                {merged.length > 0 && <RiskDistributionBar summaries={merged} />}

                {/* Filters */}
                <div className="flex flex-col sm:flex-row items-start sm:items-center gap-3">
                    {/* Search */}
                    <div className="relative w-full sm:w-72">
                        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-400" />
                        <input
                            type="text"
                            placeholder="Search by hostname or agent ID…"
                            value={search}
                            onChange={e => setSearch(e.target.value)}
                            className="w-full pl-9 pr-4 py-2.5 text-sm bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 rounded-lg text-slate-900 dark:text-white placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-cyan-500/50"
                        />
                    </div>

                    {/* Tier filter chips */}
                    <div className="flex items-center gap-2 flex-wrap">
                        {tierButtons.map(btn => (
                            <button
                                key={btn.key}
                                onClick={() => setTierFilter(btn.key)}
                                className={`px-3 py-1.5 text-xs font-semibold rounded-lg border transition-all ${tierFilter === btn.key
                                    ? 'bg-slate-900 dark:bg-slate-100 text-white dark:text-slate-900 border-transparent'
                                    : 'bg-white dark:bg-slate-800/60 text-slate-600 dark:text-slate-300 border-slate-200 dark:border-slate-700 hover:border-slate-300'
                                    }`}
                            >
                                {'color' in btn && <span className={`${btn.color} mr-1`}>●</span>}{btn.label}
                            </button>
                        ))}
                    </div>

                    <div className="ml-auto text-xs text-slate-400 font-medium">
                        {filtered.length} endpoint{filtered.length !== 1 ? 's' : ''}
                    </div>
                </div>

                {/* Ranked list */}
                {filtered.length === 0 ? (
                    merged.length === 0 ? <EmptyState /> : (
                        <div className="text-center py-16 text-slate-500 text-sm">No endpoints match your current filters.</div>
                    )
                ) : (
                    <div className="space-y-2 pb-8">
                        {filtered.map((ep, i) => (
                            <EndpointRiskCard key={ep.agent_id} ep={ep} rank={i + 1} />
                        ))}
                    </div>
                )}

                {/* Legend */}
                {merged.length > 0 && (
                    <div className="flex items-center gap-6 justify-center pb-6 text-xs text-slate-500 flex-wrap">
                        <span className="font-semibold uppercase tracking-wider">Score tiers:</span>
                        <span className="flex items-center gap-1.5"><span className="w-3 h-3 rounded-full bg-rose-500" />Critical ≥90</span>
                        <span className="flex items-center gap-1.5"><span className="w-3 h-3 rounded-full bg-orange-500" />High 70–89</span>
                        <span className="flex items-center gap-1.5"><span className="w-3 h-3 rounded-full bg-amber-500" />Medium 40–69</span>
                        <span className="flex items-center gap-1.5"><span className="w-3 h-3 rounded-full bg-emerald-500" />Low &lt;40</span>
                        <span className="flex items-center gap-2">
                            <Cpu className="w-3.5 h-3.5" />Scores computed by RiskScorer at alert time — see Context tab in Alerts for full breakdown
                        </span>
                    </div>
                )}
            </div>
        </div>
    );
}
