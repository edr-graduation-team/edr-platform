import React, { useState, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import {
    Package, Search, RefreshCw, AlertTriangle, ArrowUpDown,
    Building2, Calendar, Monitor, Download, FileSpreadsheet,
} from 'lucide-react';
import { appControlApi, type SoftwareInventoryRow } from '../../api/client';

// ────────────────────────────────────────────────────────────────────────────
// Software Inventory Tab — Live Data from Agent WMI Collector
//
// Fetches aggregated software_inventory events from the backend.
// The agent collects installed software from Windows Registry uninstall keys
// every inventory cycle (default: 1 hour) via wmi.go → collectInstalledSoftware().
// ────────────────────────────────────────────────────────────────────────────

// ─── KPI card ────────────────────────────────────────────────────────────────

function MiniKPI({ label, value, sub, icon: Icon, accent }: {
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

// ─── Export ──────────────────────────────────────────────────────────────────

function exportInventory(rows: SoftwareInventoryRow[], format: 'csv' | 'json') {
    const timestamp = new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-');
    if (format === 'csv') {
        const headers = ['Name', 'Version', 'Publisher', 'Install Date', 'Hosts', 'Last Reported'];
        const csv = [
            headers.join(','),
            ...rows.map(r => [
                `"${(r.name || '').replace(/"/g, '""')}"`,
                `"${r.version}"`,
                `"${(r.publisher || '').replace(/"/g, '""')}"`,
                r.install_date,
                r.agent_count,
                r.last_reported,
            ].join(','))
        ].join('\n');
        const blob = new Blob([csv], { type: 'text/csv' });
        const a = document.createElement('a');
        a.href = URL.createObjectURL(blob);
        a.download = `software-inventory-${timestamp}.csv`;
        a.click();
    } else {
        const blob = new Blob([JSON.stringify(rows, null, 2)], { type: 'application/json' });
        const a = document.createElement('a');
        a.href = URL.createObjectURL(blob);
        a.download = `software-inventory-${timestamp}.json`;
        a.click();
    }
}

// ─── Sort ────────────────────────────────────────────────────────────────────

type SortKey = 'name' | 'version' | 'publisher' | 'agent_count' | 'last_reported';
type SortDir = 'asc' | 'desc';

// ─── Main component ──────────────────────────────────────────────────────────

export default function SoftwareInventoryTab() {
    const { data, isLoading, isError, refetch, isFetching } = useQuery({
        queryKey: ['app-control', 'software-inventory'],
        queryFn: () => appControlApi.getSoftwareInventory(),
        staleTime: 60_000,
        refetchInterval: 300_000, // 5 min
        retry: 1,
    });

    const [search, setSearch] = useState('');
    const [sortKey, setSortKey] = useState<SortKey>('agent_count');
    const [sortDir, setSortDir] = useState<SortDir>('desc');
    const [page, setPage] = useState(1);
    const pageSize = 10;

    const rows = data?.data ?? [];

    // KPIs
    const kpis = useMemo(() => {
        const total = rows.length;
        const publishers = new Set(rows.map(r => r.publisher).filter(Boolean)).size;
        const multiHost = rows.filter(r => r.agent_count > 1).length;
        const noVersion = rows.filter(r => !r.version || r.version === '').length;
        return { total, publishers, multiHost, noVersion };
    }, [rows]);

    // Top publishers
    const topPublishers = useMemo(() => {
        const map = new Map<string, number>();
        for (const r of rows) {
            const pub = r.publisher || 'Unknown';
            map.set(pub, (map.get(pub) ?? 0) + 1);
        }
        return Array.from(map.entries())
            .sort((a, b) => b[1] - a[1])
            .slice(0, 8)
            .map(([name, count]) => ({ name, count }));
    }, [rows]);

    // Filtered + sorted
    const displayed = useMemo(() => {
        let list = rows;
        if (search.trim()) {
            const q = search.toLowerCase();
            list = list.filter(r =>
                r.name.toLowerCase().includes(q) ||
                r.version.toLowerCase().includes(q) ||
                r.publisher.toLowerCase().includes(q),
            );
        }
        return [...list].sort((a, b) => {
            let cmp = 0;
            switch (sortKey) {
                case 'name': cmp = a.name.localeCompare(b.name); break;
                case 'version': cmp = a.version.localeCompare(b.version); break;
                case 'publisher': cmp = a.publisher.localeCompare(b.publisher); break;
                case 'agent_count': cmp = a.agent_count - b.agent_count; break;
                case 'last_reported': cmp = new Date(a.last_reported).getTime() - new Date(b.last_reported).getTime(); break;
            }
            return sortDir === 'desc' ? -cmp : cmp;
        });
    }, [rows, search, sortKey, sortDir]);

    const totalPages = Math.max(1, Math.ceil(displayed.length / pageSize));
    React.useEffect(() => {
        if (page > totalPages) setPage(totalPages);
    }, [page, totalPages]);
    React.useEffect(() => {
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
                <RefreshCw className="w-5 h-5 animate-spin" /> Loading software inventory…
            </div>
        );
    }

    // Error
    if (isError) {
        return (
            <div className="rounded-xl border border-rose-200 dark:border-rose-900/50 p-6 text-center space-y-2">
                <AlertTriangle className="w-8 h-8 text-rose-400 mx-auto" />
                <p className="text-sm font-semibold text-slate-700 dark:text-slate-300">Failed to load software inventory</p>
                <p className="text-xs text-slate-500">
                    Ensure agents are running the latest version with software inventory collection enabled.
                </p>
                <button onClick={() => refetch()} className="mt-2 px-4 py-1.5 text-xs font-semibold rounded-lg bg-slate-800 text-white hover:bg-slate-700 transition-colors">
                    Retry
                </button>
            </div>
        );
    }

    // Empty state (no data yet — agents haven't reported)
    if (rows.length === 0) {
        return (
            <div className="space-y-6">
                <div className="rounded-xl border border-amber-200/60 dark:border-amber-900/40 bg-gradient-to-r from-amber-50/80 to-orange-50/60 dark:from-amber-950/20 dark:to-orange-950/10 p-5 flex items-start gap-4">
                    <div className="p-2.5 rounded-lg bg-amber-500/10 border border-amber-500/20 shrink-0">
                        <Package className="w-5 h-5 text-amber-600 dark:text-amber-400" />
                    </div>
                    <div>
                        <h3 className="text-sm font-bold text-slate-900 dark:text-white">
                            Waiting for Software Inventory Data
                        </h3>
                        <p className="text-xs text-slate-600 dark:text-slate-400 mt-1 leading-relaxed max-w-xl">
                            The agent collects installed software from Windows Registry uninstall keys every inventory cycle (default: 1 hour).
                            Once the first inventory is collected and streamed to the server, applications will appear here automatically.
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
                            <span className="text-[11px] text-slate-400">Auto-refreshes every 5 min</span>
                        </div>
                    </div>
                </div>

                <div className="rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900/60 p-5">
                    <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-300 mb-3">Data Pipeline</h3>
                    <ol className="space-y-2">
                        {[
                            { step: 'Agent WMI Collector', desc: 'Queries Windows Registry uninstall keys every 1h', code: 'wmi.go → collectInstalledSoftware()' },
                            { step: 'Event Stream', desc: 'Emits software_inventory events via gRPC', code: 'EventTypeSoftwareInventory' },
                            { step: 'Connection Manager', desc: 'Ingests and stores in PostgreSQL events table', code: 'event_repo.go → InsertMany()' },
                            { step: 'This Dashboard', desc: 'Aggregates via SQL GROUP BY endpoint', code: 'GET /app-control/software-inventory' },
                        ].map((item, i) => (
                            <li key={item.step} className="flex items-start gap-3 text-xs">
                                <span className="mt-0.5 w-5 h-5 rounded-full bg-slate-100 dark:bg-slate-800 flex items-center justify-center text-[10px] font-bold text-slate-500 border border-slate-200 dark:border-slate-700 shrink-0">
                                    {i + 1}
                                </span>
                                <div>
                                    <p className="font-semibold text-slate-800 dark:text-slate-200">{item.step}</p>
                                    <p className="text-slate-500 mt-0.5">{item.desc}</p>
                                    <code className="text-[10px] font-mono text-violet-600 dark:text-violet-400">{item.code}</code>
                                </div>
                            </li>
                        ))}
                    </ol>
                </div>
            </div>
        );
    }

    // ── Full data view ──
    return (
        <div className="space-y-6">
            {/* KPI row */}
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                <MiniKPI label="Applications" value={kpis.total}
                    sub="Unique installed programs"
                    icon={Package} accent="bg-violet-500/10 text-violet-600 dark:text-violet-400 border-violet-500/20" />
                <MiniKPI label="Publishers" value={kpis.publishers}
                    sub="Unique software vendors"
                    icon={Building2} accent="bg-cyan-500/10 text-cyan-600 dark:text-cyan-400 border-cyan-500/20" />
                <MiniKPI label="Multi-Host" value={kpis.multiHost}
                    sub="Installed on 2+ endpoints"
                    icon={Monitor} accent="bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/20" />
                <MiniKPI label="No Version" value={kpis.noVersion}
                    sub="Missing version metadata"
                    icon={AlertTriangle} accent="bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20" />
            </div>

            {/* Top publishers + export */}
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
                <div className="lg:col-span-2 rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900/60 p-4">
                    <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-300 flex items-center gap-2 mb-4">
                        <Building2 className="w-4 h-4" />
                        Top Publishers
                    </h3>
                    {topPublishers.length === 0 ? (
                        <p className="text-xs text-slate-400 text-center py-8">No publisher data</p>
                    ) : (
                        <ul className="space-y-2">
                            {topPublishers.map((pub, idx) => {
                                const max = topPublishers[0]?.count ?? 1;
                                return (
                                    <li key={pub.name} className="flex items-center gap-3 text-xs">
                                        <span className="w-5 text-slate-400 font-mono">{idx + 1}.</span>
                                        <div className="flex-1 min-w-0">
                                            <div className="flex items-center justify-between mb-0.5">
                                                <p className="font-medium text-slate-700 dark:text-slate-300 truncate">{pub.name}</p>
                                                <span className="font-bold text-violet-600 dark:text-violet-400 ml-2 shrink-0 tabular-nums">{pub.count}</span>
                                            </div>
                                            <div className="h-1.5 rounded-full bg-slate-100 dark:bg-slate-800 overflow-hidden">
                                                <div
                                                    className="h-full rounded-full bg-violet-500"
                                                    style={{ width: `${(pub.count / max) * 100}%` }}
                                                />
                                            </div>
                                        </div>
                                    </li>
                                );
                            })}
                        </ul>
                    )}
                </div>

                {/* Export */}
                <div className="rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900/60 p-4">
                    <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-300 flex items-center gap-2 mb-4">
                        <Download className="w-4 h-4" />
                        Export Inventory
                    </h3>
                    <div className="space-y-2">
                        <button
                            onClick={() => exportInventory(displayed, 'csv')}
                            className="w-full flex items-center gap-2 px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-700 hover:border-violet-400 hover:text-violet-600 transition-all text-xs"
                        >
                            <FileSpreadsheet className="w-4 h-4" />
                            Export as CSV
                        </button>
                        <button
                            onClick={() => exportInventory(displayed, 'json')}
                            className="w-full flex items-center gap-2 px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-700 hover:border-violet-400 hover:text-violet-600 transition-all text-xs"
                        >
                            <Download className="w-4 h-4" />
                            Export as JSON
                        </button>
                    </div>
                    <p className="text-xs text-slate-400 mt-3">
                        {displayed.length} application{displayed.length !== 1 ? 's' : ''} ready for export
                    </p>
                </div>
            </div>

            {/* Filter bar */}
            <div className="flex flex-col sm:flex-row items-start sm:items-center gap-3 flex-wrap">
                <div className="relative w-full sm:w-72">
                    <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-400 pointer-events-none" />
                    <input
                        type="text"
                        placeholder="Search application, version, publisher…"
                        value={search}
                        onChange={e => setSearch(e.target.value)}
                        className="w-full pl-9 pr-4 py-2 text-sm bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 rounded-lg text-slate-900 dark:text-white placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-violet-500/40"
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
                                    { key: 'name' as SortKey, label: 'Application' },
                                    { key: 'version' as SortKey, label: 'Version' },
                                    { key: 'publisher' as SortKey, label: 'Publisher' },
                                    { key: 'agent_count' as SortKey, label: 'Hosts' },
                                    { key: 'last_reported' as SortKey, label: 'Last Reported' },
                                ]).map(col => (
                                    <th key={col.key} className="px-4 py-3 text-[11px] font-bold uppercase tracking-wider text-slate-500 dark:text-slate-400 whitespace-nowrap">
                                        <button
                                            onClick={() => toggleSort(col.key)}
                                            className="flex items-center gap-1 hover:text-slate-700 dark:hover:text-slate-200 transition-colors"
                                        >
                                            {col.label}
                                            <ArrowUpDown className={`w-3 h-3 ${sortKey === col.key ? 'text-violet-500' : 'text-slate-300'}`} />
                                        </button>
                                    </th>
                                ))}
                                <th className="px-4 py-3 text-[11px] font-bold uppercase tracking-wider text-slate-500 dark:text-slate-400">Install Date</th>
                            </tr>
                        </thead>
                        <tbody>
                            {pageRows.map((row, i) => (
                                <tr
                                    key={`${row.name}-${row.version}`}
                                    className={`border-b border-slate-100 dark:border-slate-800/80 transition-colors hover:bg-violet-500/5 dark:hover:bg-slate-800/60 ${
                                        i % 2 === 1 ? 'bg-slate-50/60 dark:bg-slate-800/30' : ''
                                    }`}
                                >
                                    <td className="px-4 py-3">
                                        <div className="flex items-center gap-2">
                                            <Package className="w-3.5 h-3.5 text-violet-400 shrink-0" />
                                            <span className="text-xs font-semibold text-slate-900 dark:text-white truncate max-w-[280px]" title={row.name}>
                                                {row.name}
                                            </span>
                                        </div>
                                    </td>
                                    <td className="px-4 py-3 text-xs font-mono text-slate-600 dark:text-slate-400">
                                        {row.version || <span className="text-slate-300">—</span>}
                                    </td>
                                    <td className="px-4 py-3 text-xs text-slate-600 dark:text-slate-400 truncate max-w-[200px]" title={row.publisher}>
                                        {row.publisher || <span className="text-slate-300">—</span>}
                                    </td>
                                    <td className="px-4 py-3 tabular-nums text-center font-semibold text-slate-700 dark:text-slate-300">
                                        {row.agent_count}
                                    </td>
                                    <td className="px-4 py-3 text-xs text-slate-500 whitespace-nowrap">
                                        {new Date(row.last_reported).toLocaleString()}
                                    </td>
                                    <td className="px-4 py-3 text-xs text-slate-400 font-mono">
                                        {row.install_date ? (
                                            <span className="flex items-center gap-1">
                                                <Calendar className="w-3 h-3" />
                                                {row.install_date}
                                            </span>
                                        ) : '—'}
                                    </td>
                                </tr>
                            ))}
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
