import React, { useEffect, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import {
    ArrowDownUp, Search, RefreshCw, AlertTriangle, ArrowUpDown,
    Wifi, Server, Activity, Globe, Hash,
} from 'lucide-react';
import {
    ResponsiveContainer, BarChart, Bar, XAxis, YAxis,
    CartesianGrid, Tooltip as RechartsTooltip, Cell,
    PieChart, Pie,
} from 'recharts';
import { appControlApi } from '../../api/client';

// ────────────────────────────────────────────────────────────────────────────
// Network Activity by Application Tab
//
// Maps to WatchGuard "Bandwidth-Consuming Applications" tab.
// Shows connection counts, unique destinations, and unique ports per process.
// If byte counts are available (future agent feature), they are displayed.
// ────────────────────────────────────────────────────────────────────────────

/** Format bytes to human-readable string. */
function fmtBytes(bytes: number): string {
    if (bytes === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(Math.abs(bytes)) / Math.log(1024));
    const val = bytes / Math.pow(1024, i);
    return `${val.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

// ─── KPI card ────────────────────────────────────────────────────────────────

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

type SortKey = 'process_name' | 'connections' | 'unique_destinations' | 'unique_ports' | 'total_bytes' | 'agent_count';
type SortDir = 'asc' | 'desc';

const CHART_COLORS = [
    '#06b6d4', '#8b5cf6', '#f59e0b', '#10b981', '#f43f5e',
    '#3b82f6', '#ec4899', '#14b8a6', '#a855f7', '#64748b',
    '#ef4444', '#22c55e',
];

// ─── Main component ──────────────────────────────────────────────────────────

export default function BandwidthAnalyticsTab() {
    const { data, isLoading, isError, refetch, isFetching } = useQuery({
        queryKey: ['app-control', 'bandwidth-analytics'],
        queryFn: () => appControlApi.getBandwidthAnalytics(24),
        staleTime: 60_000,
        refetchInterval: 120_000,
        retry: 1,
    });

    const [search, setSearch] = useState('');
    const [sortKey, setSortKey] = useState<SortKey>('connections');
    const [sortDir, setSortDir] = useState<SortDir>('desc');
    const [page, setPage] = useState(1);
    const pageSize = 10;

    const rows = data?.data ?? [];

    // Determine if we have real byte data
    const hasByteData = useMemo(() => rows.some(r => r.total_bytes > 0), [rows]);

    // KPIs
    const kpis = useMemo(() => {
        const totalConnections = rows.reduce((s, r) => s + r.connections, 0);
        const totalDestinations = rows.reduce((s, r) => s + (r.unique_destinations || 0), 0);
        const totalApps = rows.length;
        const totalBytes = rows.reduce((s, r) => s + r.total_bytes, 0);
        return { totalConnections, totalDestinations, totalApps, totalBytes };
    }, [rows]);

    // Top 10 for bar chart (connections)
    const chartData = useMemo(() => {
        return rows.slice(0, 10).map((r, i) => ({
            name: r.process_name.length > 18 ? r.process_name.slice(0, 16) + '…' : r.process_name,
            fullName: r.process_name,
            connections: r.connections,
            destinations: r.unique_destinations || 0,
            fill: CHART_COLORS[i % CHART_COLORS.length],
        }));
    }, [rows]);

    // Pie chart: connection distribution for top 8
    const pieData = useMemo(() => {
        return rows.slice(0, 8).map((r, i) => ({
            name: r.process_name.length > 16 ? r.process_name.slice(0, 14) + '…' : r.process_name,
            value: r.connections,
            fill: CHART_COLORS[i % CHART_COLORS.length],
        }));
    }, [rows]);

    // Filtered + sorted
    const displayed = useMemo(() => {
        let list = rows;
        if (search.trim()) {
            const q = search.toLowerCase();
            list = list.filter(r =>
                r.process_name.toLowerCase().includes(q) ||
                r.executable.toLowerCase().includes(q),
            );
        }
        return [...list].sort((a, b) => {
            let cmp = 0;
            switch (sortKey) {
                case 'process_name': cmp = a.process_name.localeCompare(b.process_name); break;
                case 'connections': cmp = a.connections - b.connections; break;
                case 'unique_destinations': cmp = (a.unique_destinations || 0) - (b.unique_destinations || 0); break;
                case 'unique_ports': cmp = (a.unique_ports || 0) - (b.unique_ports || 0); break;
                case 'total_bytes': cmp = a.total_bytes - b.total_bytes; break;
                case 'agent_count': cmp = a.agent_count - b.agent_count; break;
            }
            return sortDir === 'desc' ? -cmp : cmp;
        });
    }, [rows, search, sortKey, sortDir]);

    const totalPages = Math.max(1, Math.ceil(displayed.length / pageSize));
    useEffect(() => {
        if (page > totalPages) setPage(totalPages);
    }, [page, totalPages]);
    useEffect(() => {
        setPage(1);
    }, [search, sortKey, sortDir]);
    const pageRows = useMemo(() => {
        const start = (page - 1) * pageSize;
        return displayed.slice(start, start + pageSize);
    }, [displayed, page]);

    const toggleSort = (key: SortKey) => {
        if (sortKey === key) setSortDir(d => d === 'asc' ? 'desc' : 'asc');
        else { setSortKey(key); setSortDir('desc'); }
    };

    // Loading
    if (isLoading) {
        return (
            <div className="flex items-center justify-center py-20 text-slate-500 gap-2">
                <RefreshCw className="w-5 h-5 animate-spin" /> Loading network activity data…
            </div>
        );
    }

    // Error
    if (isError) {
        return (
            <div className="rounded-xl border border-rose-200 dark:border-rose-900/50 p-6 text-center space-y-2">
                <AlertTriangle className="w-8 h-8 text-rose-400 mx-auto" />
                <p className="text-sm font-semibold text-slate-700 dark:text-slate-300">Failed to load network activity analytics</p>
                <p className="text-xs text-slate-500">Ensure the connection-manager events API is reachable.</p>
                <button onClick={() => refetch()} className="mt-2 px-4 py-1.5 text-xs font-semibold rounded-lg bg-slate-800 text-white hover:bg-slate-700 transition-colors">
                    Retry
                </button>
            </div>
        );
    }

    // Empty state
    if (rows.length === 0) {
        return (
            <div className="space-y-6">
                <div className="rounded-xl border border-amber-200/60 dark:border-amber-900/40 bg-gradient-to-r from-amber-50/80 to-orange-50/60 dark:from-amber-950/20 dark:to-orange-950/10 p-5 flex items-start gap-4">
                    <div className="p-2.5 rounded-lg bg-amber-500/10 border border-amber-500/20 shrink-0">
                        <Wifi className="w-5 h-5 text-amber-600 dark:text-amber-400" />
                    </div>
                    <div>
                        <h3 className="text-sm font-bold text-slate-900 dark:text-white">
                            Waiting for Network Activity Data
                        </h3>
                        <p className="text-xs text-slate-600 dark:text-slate-400 mt-1 leading-relaxed max-w-xl">
                            The agent collects network connection events (TCP/UDP) from ETW and Sysmon telemetry.
                            Data will appear here once agents begin reporting network connections.
                        </p>
                        <div className="mt-3 flex items-center gap-3">
                            <button
                                onClick={() => refetch()}
                                disabled={isFetching}
                                className="flex items-center gap-2 px-3 py-1.5 bg-slate-800 dark:bg-slate-700 hover:bg-slate-700 dark:hover:bg-slate-600 text-white text-xs font-medium rounded-lg transition-colors"
                            >
                                <RefreshCw className={`w-3.5 h-3.5 ${isFetching ? 'animate-spin' : ''}`} />
                                Check Again
                            </button>
                            <span className="text-[11px] text-slate-400">Auto-refreshes every 2 min</span>
                        </div>
                    </div>
                </div>
            </div>
        );
    }

    return (
        <div className="space-y-6">
            {/* KPI row */}
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                <KPICard label="Total Connections" value={kpis.totalConnections.toLocaleString()}
                    sub="Network events (24h)"
                    icon={Activity} accent="bg-cyan-500/10 text-cyan-600 dark:text-cyan-400 border-cyan-500/20" />
                <KPICard label="Unique Destinations" value={kpis.totalDestinations.toLocaleString()}
                    sub="Distinct remote IPs"
                    icon={Globe} accent="bg-violet-500/10 text-violet-600 dark:text-violet-400 border-violet-500/20" />
                <KPICard label="Active Applications" value={kpis.totalApps}
                    sub="With network activity"
                    icon={Server} accent="bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/20" />
                {hasByteData ? (
                    <KPICard label="Total Transferred" value={fmtBytes(kpis.totalBytes)}
                        sub="Bytes sent + received"
                        icon={ArrowDownUp} accent="bg-orange-500/10 text-orange-600 dark:text-orange-400 border-orange-500/20" />
                ) : (
                    <KPICard label="Avg Connections" value={kpis.totalApps > 0 ? Math.round(kpis.totalConnections / kpis.totalApps) : 0}
                        sub="Per application (24h)"
                        icon={Hash} accent="bg-orange-500/10 text-orange-600 dark:text-orange-400 border-orange-500/20" />
                )}
            </div>

            {/* Charts */}
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
                {/* Bar chart: top processes by connections */}
                <div className="lg:col-span-2 rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900/60 p-4">
                    <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-300 flex items-center gap-2 mb-4">
                        <ArrowDownUp className="w-4 h-4" />
                        Top Network Activity by Application (24h)
                    </h3>
                    {chartData.length === 0 ? (
                        <p className="text-xs text-slate-400 text-center py-12">No network data.</p>
                    ) : (
                        <div className="h-64">
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
                                        formatter={((value?: number, name?: string) => {
                                            const v = Number(value ?? 0);
                                            return [v.toLocaleString(), name === 'connections' ? 'Connections' : 'Unique Destinations'];
                                        }) as never}
                                    />
                                    <Bar dataKey="connections" name="Connections" fill="#06b6d4" radius={[0, 4, 4, 0]} barSize={14} />
                                </BarChart>
                            </ResponsiveContainer>
                        </div>
                    )}
                </div>

                {/* Pie chart: connection distribution */}
                <div className="rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900/60 p-4">
                    <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-300 flex items-center gap-2 mb-4">
                        <Wifi className="w-4 h-4" />
                        Connection Distribution
                    </h3>
                    {pieData.length === 0 ? (
                        <p className="text-xs text-slate-400 text-center py-8">No data</p>
                    ) : (
                        <>
                            <div className="h-44">
                                <ResponsiveContainer width="100%" height="100%">
                                    <PieChart>
                                        <Pie
                                            data={pieData}
                                            dataKey="value"
                                            cx="50%" cy="50%"
                                            outerRadius={70}
                                            innerRadius={35}
                                            paddingAngle={2}
                                        >
                                            {pieData.map((d, i) => (
                                                <Cell key={i} fill={d.fill} />
                                            ))}
                                        </Pie>
                                        <RechartsTooltip
                                            contentStyle={{ background: 'rgba(15, 23, 42, 0.95)', border: 'none', borderRadius: '8px', color: 'white', fontSize: '11px' }}
                                            formatter={((value?: number) => [Number(value ?? 0).toLocaleString(), 'Connections']) as never}
                                        />
                                    </PieChart>
                                </ResponsiveContainer>
                            </div>
                            <ul className="space-y-1.5 mt-2">
                                {pieData.map((d) => (
                                    <li key={d.name} className="flex items-center gap-2 text-xs">
                                        <span className="w-2.5 h-2.5 rounded-full shrink-0" style={{ backgroundColor: d.fill }} />
                                        <span className="truncate text-slate-600 dark:text-slate-400 flex-1">{d.name}</span>
                                        <span className="font-mono font-semibold text-slate-700 dark:text-slate-300 tabular-nums">{d.value.toLocaleString()}</span>
                                    </li>
                                ))}
                            </ul>
                        </>
                    )}
                </div>
            </div>

            {/* Filter bar */}
            <div className="flex flex-col sm:flex-row items-start sm:items-center gap-3 flex-wrap">
                <div className="relative w-full sm:w-72">
                    <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-400 pointer-events-none" />
                    <input
                        type="text"
                        placeholder="Search application name or path…"
                        value={search}
                        onChange={e => setSearch(e.target.value)}
                        className="w-full pl-9 pr-4 py-2 text-sm bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 rounded-lg text-slate-900 dark:text-white placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-cyan-500/40"
                    />
                </div>

                <button
                    onClick={() => refetch()}
                    disabled={isFetching}
                    className="flex items-center gap-2 px-3 py-2 bg-slate-800 dark:bg-slate-700 hover:bg-slate-700 dark:hover:bg-slate-600 disabled:opacity-50 text-white text-xs font-medium rounded-lg transition-colors shadow-sm ml-auto"
                >
                    <RefreshCw className={`w-3.5 h-3.5 ${isFetching ? 'animate-spin' : ''}`} />
                    Refresh
                </button>

                <span className="text-xs text-slate-400 font-medium">
                    {displayed.length} application{displayed.length !== 1 ? 's' : ''}
                </span>
            </div>

            {/* Table */}
            <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white dark:bg-slate-900/60 overflow-hidden shadow-sm">
                <div className="overflow-x-auto">
                    <table className="w-full text-left text-sm border-collapse">
                        <thead>
                            <tr className="border-b border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/60">
                                {([
                                    { key: 'process_name' as SortKey, label: 'Application' },
                                    { key: 'connections' as SortKey, label: 'Connections' },
                                    { key: 'unique_destinations' as SortKey, label: 'Destinations' },
                                    { key: 'unique_ports' as SortKey, label: 'Ports' },
                                    ...(hasByteData ? [{ key: 'total_bytes' as SortKey, label: 'Total Bytes' }] : []),
                                    { key: 'agent_count' as SortKey, label: 'Hosts' },
                                ]).map(col => (
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
                                <th className="px-4 py-3 text-[11px] font-bold uppercase tracking-wider text-slate-500 dark:text-slate-400">Last Seen</th>
                                <th className="px-4 py-3 text-[11px] font-bold uppercase tracking-wider text-slate-500 dark:text-slate-400">Path</th>
                            </tr>
                        </thead>
                        <tbody>
                            {pageRows.map((row, i) => {
                                const topConns = rows[0]?.connections || 1;
                                const pct = ((row.connections / topConns) * 100);
                                return (
                                    <tr
                                        key={row.process_name}
                                        className={`border-b border-slate-100 dark:border-slate-800/80 transition-colors hover:bg-cyan-500/5 dark:hover:bg-slate-800/60 ${
                                            i % 2 === 1 ? 'bg-slate-50/60 dark:bg-slate-800/30' : ''
                                        }`}
                                    >
                                        <td className="px-4 py-3">
                                            <div className="flex items-center gap-2">
                                                <Wifi className="w-3.5 h-3.5 text-cyan-400 shrink-0" />
                                                <div className="min-w-0">
                                                    <span className="font-mono text-xs font-semibold text-slate-900 dark:text-white block truncate max-w-[200px]">{row.process_name}</span>
                                                    <div className="h-1 mt-1 rounded-full bg-slate-100 dark:bg-slate-800 overflow-hidden w-24">
                                                        <div
                                                            className="h-full rounded-full bg-gradient-to-r from-cyan-500 to-violet-500"
                                                            style={{ width: `${Math.min(100, pct)}%` }}
                                                        />
                                                    </div>
                                                </div>
                                            </div>
                                        </td>
                                        <td className="px-4 py-3 tabular-nums text-right text-xs font-bold text-slate-700 dark:text-slate-300">
                                            {row.connections.toLocaleString()}
                                        </td>
                                        <td className="px-4 py-3 tabular-nums text-right text-xs font-semibold text-violet-600 dark:text-violet-400">
                                            {(row.unique_destinations || 0).toLocaleString()}
                                        </td>
                                        <td className="px-4 py-3 tabular-nums text-right text-xs text-slate-600 dark:text-slate-400">
                                            {(row.unique_ports || 0).toLocaleString()}
                                        </td>
                                        {hasByteData && (
                                            <td className="px-4 py-3 tabular-nums text-right text-xs font-semibold text-cyan-600 dark:text-cyan-400">
                                                {fmtBytes(row.total_bytes)}
                                            </td>
                                        )}
                                        <td className="px-4 py-3 tabular-nums text-center text-slate-600 dark:text-slate-400">
                                            {row.agent_count}
                                        </td>
                                        <td className="px-4 py-3 text-xs text-slate-500 whitespace-nowrap">
                                            {new Date(row.last_seen).toLocaleString()}
                                        </td>
                                        <td className="px-4 py-3 text-xs text-slate-400 max-w-[250px] truncate font-mono" title={row.executable}>
                                            {row.executable || '—'}
                                        </td>
                                    </tr>
                                );
                            })}
                        </tbody>
                    </table>
                </div>
            </div>

            {displayed.length > 0 && (
                <div className="flex items-center justify-between text-xs text-slate-500">
                    <span>
                        Page {page} / {totalPages} · Showing {pageRows.length} of {displayed.length}
                    </span>
                    <div className="flex items-center gap-2">
                        <button
                            type="button"
                            onClick={() => setPage((p) => Math.max(1, p - 1))}
                            disabled={page <= 1}
                            className="px-3 py-1.5 rounded-lg border border-slate-200 dark:border-slate-600 disabled:opacity-50"
                        >
                            Prev
                        </button>
                        <button
                            type="button"
                            onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                            disabled={page >= totalPages}
                            className="px-3 py-1.5 rounded-lg border border-slate-200 dark:border-slate-600 disabled:opacity-50"
                        >
                            Next
                        </button>
                    </div>
                </div>
            )}
        </div>
    );
}
