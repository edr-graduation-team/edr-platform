import React, { useMemo, useState } from 'react';
import {
    Activity, AlertTriangle, Search, Terminal, Cpu,
    ArrowUpDown, ChevronDown, Monitor, RefreshCw,
} from 'lucide-react';
import {
    ResponsiveContainer, BarChart, Bar, XAxis, YAxis,
    CartesianGrid, Tooltip as RechartsTooltip, Cell,
} from 'recharts';
import { useProcessAnalytics } from './useProcessAnalytics';
import { CATEGORY_META, type ProcessCategory } from './types';
import { isHighAttention } from './classifyProcess';

// ─── Category badge ──────────────────────────────────────────────────────────

function CategoryBadge({ category }: { category: ProcessCategory }) {
    const m = CATEGORY_META[category];
    return (
        <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[11px] font-semibold border ${m.color}`}>
            <span className={`w-1.5 h-1.5 rounded-full ${m.dot}`} />
            {m.label}
        </span>
    );
}

// ─── KPI card (local — small variant) ────────────────────────────────────────

function KPICard({ label, value, sub, icon: Icon, accent }: {
    label: string; value: string | number; sub?: string;
    icon: React.ElementType; accent: string;
}) {
    return (
        <div className={`relative overflow-hidden rounded-xl border bg-white/60 dark:bg-slate-900/40 backdrop-blur-md p-5 shadow-sm transition-all hover:shadow-md group ${accent}`}>
            <div className="absolute -top-8 -right-8 w-24 h-24 rounded-full blur-3xl opacity-15 group-hover:opacity-30 transition-opacity pointer-events-none bg-current" />
            <div className="relative z-10 flex items-start gap-4">
                <div className={`p-2.5 rounded-lg shrink-0 ${accent}`}>
                    <Icon className="w-4 h-4" />
                </div>
                <div className="min-w-0">
                    <p className="text-[11px] font-bold uppercase tracking-widest text-slate-500 dark:text-slate-400 mb-1">{label}</p>
                    <p className="text-2xl font-extrabold text-slate-900 dark:text-white leading-none">{value}</p>
                    {sub && <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">{sub}</p>}
                </div>
            </div>
        </div>
    );
}

// ─── Sort helpers ────────────────────────────────────────────────────────────

type SortKey = 'count' | 'name' | 'category' | 'lastSeen';
type SortDir = 'asc' | 'desc';

// ─── Main component ──────────────────────────────────────────────────────────

export default function ProcessAnalyticsTab() {
    const { rows, totalEvents, isLoading, isError, refetch, isFetching } = useProcessAnalytics();

    const [search, setSearch] = useState('');
    const [catFilter, setCatFilter] = useState<ProcessCategory | ''>('');
    const [sortKey, setSortKey] = useState<SortKey>('count');
    const [sortDir, setSortDir] = useState<SortDir>('desc');

    // ── Aggregated KPIs ──
    const kpis = useMemo(() => {
        const uniqueExes = rows.length;
        const scriptingCount = rows.filter(r => r.category === 'scripting').reduce((s, r) => s + r.count, 0);
        const adminCount = rows.filter(r => r.category === 'admin').reduce((s, r) => s + r.count, 0);
        const remoteCount = rows.filter(r => r.category === 'remote_access').reduce((s, r) => s + r.count, 0);
        const highAttention = rows.filter(r => isHighAttention(r.category)).length;
        return { uniqueExes, scriptingCount, adminCount, remoteCount, highAttention };
    }, [rows]);

    // ── Filtered + sorted rows ──
    const displayed = useMemo(() => {
        let list = rows;
        if (catFilter) list = list.filter(r => r.category === catFilter);
        if (search.trim()) {
            const q = search.toLowerCase();
            list = list.filter(r =>
                r.name.includes(q) ||
                r.executable.toLowerCase().includes(q) ||
                CATEGORY_META[r.category].label.toLowerCase().includes(q),
            );
        }
        const sorted = [...list].sort((a, b) => {
            let cmp = 0;
            switch (sortKey) {
                case 'count': cmp = a.count - b.count; break;
                case 'name': cmp = a.name.localeCompare(b.name); break;
                case 'category': cmp = a.category.localeCompare(b.category); break;
                case 'lastSeen': cmp = new Date(a.lastSeen).getTime() - new Date(b.lastSeen).getTime(); break;
            }
            return sortDir === 'desc' ? -cmp : cmp;
        });
        return sorted;
    }, [rows, catFilter, search, sortKey, sortDir]);

    // ── Chart: Top 12 processes ──
    const chartData = useMemo(() => {
        return rows.slice(0, 12).map(r => ({
            name: r.name.length > 18 ? r.name.slice(0, 16) + '…' : r.name,
            fullName: r.name,
            count: r.count,
            fill: isHighAttention(r.category) ? '#f59e0b' : '#06b6d4',
        }));
    }, [rows]);

    // ── Category distribution for pie-like pills ──
    const categoryBreakdown = useMemo(() => {
        const map = new Map<ProcessCategory, number>();
        for (const r of rows) {
            map.set(r.category, (map.get(r.category) ?? 0) + r.count);
        }
        return Array.from(map.entries())
            .map(([cat, count]) => ({ cat, count }))
            .sort((a, b) => b.count - a.count);
    }, [rows]);

    const toggleSort = (key: SortKey) => {
        if (sortKey === key) setSortDir(d => d === 'asc' ? 'desc' : 'asc');
        else { setSortKey(key); setSortDir('desc'); }
    };

    // ── Loading / Error states ──
    if (isLoading) {
        return (
            <div className="flex items-center justify-center py-20 text-slate-500 gap-2">
                <RefreshCw className="w-5 h-5 animate-spin" /> Loading process analytics…
            </div>
        );
    }

    if (isError) {
        return (
            <div className="rounded-xl border border-rose-200 dark:border-rose-900/50 p-6 text-center space-y-2">
                <AlertTriangle className="w-8 h-8 text-rose-400 mx-auto" />
                <p className="text-sm font-semibold text-slate-700 dark:text-slate-300">Failed to load process events</p>
                <p className="text-xs text-slate-500">Ensure the connection-manager events API is reachable.</p>
                <button onClick={refetch} className="mt-2 px-4 py-1.5 text-xs font-semibold rounded-lg bg-slate-800 text-white hover:bg-slate-700 transition-colors">
                    Retry
                </button>
            </div>
        );
    }

    return (
        <div className="space-y-6">
            {/* KPI row */}
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                <KPICard label="Unique Executables" value={kpis.uniqueExes}
                    sub={`From ${totalEvents.toLocaleString()} events (24h)`}
                    icon={Cpu} accent="bg-cyan-500/10 text-cyan-600 dark:text-cyan-400 border-cyan-500/20" />
                <KPICard label="Scripting Engines" value={kpis.scriptingCount}
                    sub="PowerShell, CMD, WSH, Python…"
                    icon={Terminal} accent="bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20" />
                <KPICard label="Admin / Recon Tools" value={kpis.adminCount}
                    sub="regedit, sc, systeminfo, net…"
                    icon={Monitor} accent="bg-orange-500/10 text-orange-600 dark:text-orange-400 border-orange-500/20" />
                <KPICard label="Attention-Worthy" value={kpis.highAttention}
                    sub="Unique exes requiring review"
                    icon={AlertTriangle} accent="bg-rose-500/10 text-rose-600 dark:text-rose-400 border-rose-500/20" />
            </div>

            {/* Chart + Category breakdown */}
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
                {/* Bar chart: top processes */}
                <div className="lg:col-span-2 rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900/60 p-4">
                    <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-300 flex items-center gap-2 mb-4">
                        <Activity className="w-4 h-4" />
                        Top Executed Processes (24h)
                    </h3>
                    {chartData.length === 0 ? (
                        <p className="text-xs text-slate-400 text-center py-12">No process events in the last 24 hours.</p>
                    ) : (
                        <div className="h-56">
                            <ResponsiveContainer width="100%" height="100%">
                                <BarChart data={chartData} layout="vertical" margin={{ left: 10, right: 20 }}>
                                    <CartesianGrid strokeDasharray="3 3" stroke="#e2e8f0" horizontal={false} />
                                    <XAxis type="number" tick={{ fontSize: 10 }} />
                                    <YAxis
                                        type="category"
                                        dataKey="name"
                                        tick={{ fontSize: 10, fontFamily: 'monospace' }}
                                        width={120}
                                    />
                                    <RechartsTooltip
                                        contentStyle={{ background: 'rgba(15, 23, 42, 0.95)', border: 'none', borderRadius: '8px', color: 'white', fontSize: '12px' }}
                                        formatter={(value: number, _name: string, entry: { payload: { fullName: string } }) => [
                                            `${value} executions`,
                                            entry.payload.fullName,
                                        ]}
                                    />
                                    <Bar dataKey="count" radius={[0, 4, 4, 0]} barSize={16}>
                                        {chartData.map((d, i) => (
                                            <Cell key={i} fill={d.fill} />
                                        ))}
                                    </Bar>
                                </BarChart>
                            </ResponsiveContainer>
                        </div>
                    )}
                </div>

                {/* Category distribution */}
                <div className="rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900/60 p-4">
                    <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-300 flex items-center gap-2 mb-4">
                        <ChevronDown className="w-4 h-4" />
                        Category Breakdown
                    </h3>
                    {categoryBreakdown.length === 0 ? (
                        <p className="text-xs text-slate-400 text-center py-8">No data</p>
                    ) : (
                        <ul className="space-y-2.5">
                            {categoryBreakdown.map(({ cat, count }) => {
                                const m = CATEGORY_META[cat];
                                const pct = totalEvents > 0 ? ((count / totalEvents) * 100).toFixed(1) : '0';
                                return (
                                    <li key={cat} className="flex items-center gap-3 text-xs">
                                        <button
                                            onClick={() => setCatFilter(c => c === cat ? '' : cat)}
                                            className={`shrink-0 transition-opacity ${catFilter && catFilter !== cat ? 'opacity-40' : ''}`}
                                        >
                                            <CategoryBadge category={cat} />
                                        </button>
                                        <div className="flex-1 min-w-0">
                                            <div className="h-1.5 rounded-full bg-slate-100 dark:bg-slate-800 overflow-hidden">
                                                <div
                                                    className={`h-full rounded-full ${m.dot}`}
                                                    style={{ width: `${Math.min(100, Number(pct))}%` }}
                                                />
                                            </div>
                                        </div>
                                        <span className="font-mono font-semibold text-slate-600 dark:text-slate-400 shrink-0 tabular-nums">
                                            {count.toLocaleString()} <span className="text-slate-400">({pct}%)</span>
                                        </span>
                                    </li>
                                );
                            })}
                        </ul>
                    )}
                </div>
            </div>

            {/* Filter bar */}
            <div className="flex flex-col sm:flex-row items-start sm:items-center gap-3 flex-wrap">
                <div className="relative w-full sm:w-72">
                    <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-400 pointer-events-none" />
                    <input
                        type="text"
                        placeholder="Search process name or path…"
                        value={search}
                        onChange={e => setSearch(e.target.value)}
                        className="w-full pl-9 pr-4 py-2 text-sm bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 rounded-lg text-slate-900 dark:text-white placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-cyan-500/40"
                    />
                </div>

                {catFilter && (
                    <button
                        onClick={() => setCatFilter('')}
                        className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-semibold rounded-lg border border-cyan-500/40 bg-cyan-500/10 text-cyan-600 dark:text-cyan-400 transition-all hover:bg-cyan-500/20"
                    >
                        {CATEGORY_META[catFilter].label} ×
                    </button>
                )}

                <button
                    onClick={refetch}
                    disabled={isFetching}
                    className="flex items-center gap-2 px-3 py-2 bg-slate-800 dark:bg-slate-700 hover:bg-slate-700 dark:hover:bg-slate-600 disabled:opacity-50 text-white text-xs font-medium rounded-lg transition-colors shadow-sm ml-auto"
                >
                    <RefreshCw className={`w-3.5 h-3.5 ${isFetching ? 'animate-spin' : ''}`} />
                    Refresh
                </button>

                <span className="text-xs text-slate-400 font-medium">
                    {displayed.length} process{displayed.length !== 1 ? 'es' : ''}
                </span>
            </div>

            {/* Table */}
            <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white dark:bg-slate-900/60 overflow-hidden shadow-sm">
                {displayed.length === 0 ? (
                    <div className="flex flex-col items-center justify-center py-16 text-center">
                        <Terminal className="w-10 h-10 text-slate-300 dark:text-slate-600 mb-3" />
                        <p className="text-sm font-semibold text-slate-600 dark:text-slate-400">
                            {rows.length === 0 ? 'No process events in the last 24 hours' : 'No processes match your filter'}
                        </p>
                    </div>
                ) : (
                    <div className="overflow-x-auto">
                        <table className="w-full text-left text-sm border-collapse">
                            <thead>
                                <tr className="border-b border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/60">
                                    {[
                                        { key: 'name' as SortKey, label: 'Process' },
                                        { key: 'category' as SortKey, label: 'Category' },
                                        { key: 'count' as SortKey, label: 'Executions' },
                                        { key: 'lastSeen' as SortKey, label: 'Last Seen' },
                                    ].map(col => (
                                        <th key={col.key} className="px-4 py-3 text-[11px] font-bold uppercase tracking-wider text-slate-500 dark:text-slate-400 whitespace-nowrap">
                                            <button
                                                onClick={() => toggleSort(col.key)}
                                                className="flex items-center gap-1 hover:text-slate-700 dark:hover:text-slate-200 transition-colors"
                                            >
                                                {col.label}
                                                <ArrowUpDown className={`w-3 h-3 ${sortKey === col.key ? 'text-cyan-500' : 'text-slate-300'}`} />
                                            </button>
                                        </th>
                                    ))}
                                    <th className="px-4 py-3 text-[11px] font-bold uppercase tracking-wider text-slate-500 dark:text-slate-400">Hosts</th>
                                    <th className="px-4 py-3 text-[11px] font-bold uppercase tracking-wider text-slate-500 dark:text-slate-400">Path</th>
                                </tr>
                            </thead>
                            <tbody>
                                {displayed.slice(0, 100).map((row, i) => (
                                    <tr
                                        key={row.name}
                                        className={`border-b border-slate-100 dark:border-slate-800/80 transition-colors hover:bg-cyan-500/5 dark:hover:bg-slate-800/60 ${
                                            isHighAttention(row.category) ? 'bg-amber-50/40 dark:bg-amber-950/10' : i % 2 === 1 ? 'bg-slate-50/60 dark:bg-slate-800/30' : ''
                                        }`}
                                    >
                                        <td className="px-4 py-3">
                                            <div className="flex items-center gap-2">
                                                {isHighAttention(row.category) && (
                                                    <AlertTriangle className="w-3.5 h-3.5 text-amber-500 shrink-0" title="Security-relevant process" />
                                                )}
                                                <span className="font-mono text-xs font-semibold text-slate-900 dark:text-white">{row.name}</span>
                                            </div>
                                        </td>
                                        <td className="px-4 py-3 whitespace-nowrap">
                                            <CategoryBadge category={row.category} />
                                        </td>
                                        <td className="px-4 py-3 tabular-nums text-right font-semibold text-slate-700 dark:text-slate-300">
                                            {row.count.toLocaleString()}
                                        </td>
                                        <td className="px-4 py-3 text-xs text-slate-500 whitespace-nowrap">
                                            {new Date(row.lastSeen).toLocaleString()}
                                        </td>
                                        <td className="px-4 py-3 tabular-nums text-slate-600 dark:text-slate-400">{row.agents.size}</td>
                                        <td className="px-4 py-3 text-xs text-slate-400 max-w-[280px] truncate font-mono" title={row.executable}>
                                            {row.executable || '—'}
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                )}
            </div>

            {displayed.length > 100 && (
                <p className="text-xs text-slate-400 text-center">
                    Showing top 100 of {displayed.length} processes. Use search/category filters to narrow results.
                </p>
            )}
        </div>
    );
}
