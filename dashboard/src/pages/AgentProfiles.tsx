import { useEffect, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import {
    Activity,
    AlertTriangle,
    BarChart3,
    ChevronLeft,
    ChevronRight,
    Database,
    Fingerprint,
    Layers,
    Radio,
    Search,
    Server,
    Shield,
} from 'lucide-react';
import { agentsApi, type Agent } from '../api/client';
import { formatDate, formatRelativeTime, getEffectiveStatus } from '../utils/agentDisplay';
import EmptyState from '../components/EmptyState';
import StatCard from '../components/StatCard';
import InsightHero from '../components/InsightHero';

const PAGE_SIZE = 50;

function osLabel(a: Agent) {
    const os = (a.os_type || '').toLowerCase();
    if (os === 'windows') return 'Windows';
    if (os === 'linux') return 'Linux';
    if (os === 'macos') return 'macOS';
    return a.os_type || 'Unknown';
}

export default function AgentProfiles() {
    useEffect(() => {
        document.title = 'Agent Profiles — Management | EDR Platform';
    }, []);

    const [searchInput, setSearchInput] = useState('');
    const [debouncedQ, setDebouncedQ] = useState('');
    const [page, setPage] = useState(1);

    useEffect(() => {
        const t = window.setTimeout(() => setDebouncedQ(searchInput.trim()), 400);
        return () => window.clearTimeout(t);
    }, [searchInput]);

    useEffect(() => {
        setPage(1);
    }, [debouncedQ]);

    const statsQ = useQuery({
        queryKey: ['agents-stats', 'agent-profiles'],
        queryFn: () => agentsApi.stats(),
        staleTime: 60_000,
        refetchInterval: 120_000,
        retry: 1,
    });

    const agentsQ = useQuery({
        queryKey: ['agents', 'profiles-paged', page, debouncedQ, PAGE_SIZE],
        queryFn: () =>
            agentsApi.list({
                limit: PAGE_SIZE,
                offset: (page - 1) * PAGE_SIZE,
                sort_by: 'hostname',
                sort_order: 'asc',
                search: debouncedQ || undefined,
            }),
        staleTime: 15_000,
        refetchInterval: 45_000,
        retry: 1,
    });

    useEffect(() => {
        if (!agentsQ.isSuccess || agentsQ.data == null) return;
        const total = agentsQ.data.pagination?.total ?? 0;
        const tp = Math.max(1, Math.ceil(total / PAGE_SIZE));
        if (page > tp) setPage(tp);
    }, [agentsQ.isSuccess, agentsQ.data?.pagination?.total, page]);

    const totalMatching = agentsQ.data?.pagination?.total ?? 0;
    const rows = agentsQ.data?.data ?? [];
    const apiOffset = (page - 1) * PAGE_SIZE;
    const totalPages = Math.max(1, Math.ceil(totalMatching / PAGE_SIZE));
    const viewPage = Math.min(page, totalPages);
    const fromIdx = totalMatching === 0 || rows.length === 0 ? 0 : apiOffset + 1;
    const toIdx = totalMatching === 0 || rows.length === 0 ? 0 : apiOffset + rows.length;

    const registry = statsQ.data;

    const osDistSummary = useMemo(() => {
        const b = registry?.by_os_type ?? {};
        const parts = Object.entries(b)
            .filter(([, n]) => n > 0)
            .map(([k, n]) => `${k}: ${n}`);
        return parts.length ? parts.join(' · ') : '—';
    }, [registry]);

    return (
        <div className="space-y-6 animate-slide-up-fade w-full min-w-0">
            <InsightHero
                
                accent="cyan"
                icon={Fingerprint}
                eyebrow="Fleet identity"
                title="Agent profiles"
                lead={
                    <>
                        Read-only <strong className="text-white">identity &amp; posture directory</strong> for enrolled endpoints: registry KPIs from{' '}
                        <code className="text-[11px] text-cyan-200/95 bg-white/10 px-1 rounded">GET /api/v1/agents/stats</code>, and paged rows from{' '}
                        <code className="text-[11px] text-cyan-200/95 bg-white/10 px-1 rounded">GET /api/v1/agents</code> with the same effective-status rule as Device Management. No commands or tags here — open a device for operations.
                    </>
                }
            />

            <div className="grid gap-3 md:grid-cols-3">
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <Server className="w-4 h-4 text-slate-500" />
                        vs Device Management
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/management/devices">
                            /management/devices
                        </Link>{' '}
                        is the <strong>operational console</strong> (filters, isolate, tags, commands). This page is a <strong>profile catalog</strong> only.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <BarChart3 className="w-4 h-4 text-cyan-500" />
                        vs Endpoint Summary
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/dashboards/endpoint">
                            Endpoint Summary
                        </Link>{' '}
                        is executive charts and risk excerpts. Here you browse <strong>tabular identity</strong> with search and paging.
                    </p>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 p-4 shadow-sm">
                    <div className="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <Layers className="w-4 h-4 text-violet-500" />
                        vs Agent deployment
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                        <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/management/agent-deploy">
                            Agent deployment
                        </Link>{' '}
                        builds and distributes packages. Profiles describe <strong>what is already installed</strong>, not installer assets.
                    </p>
                </div>
            </div>

            {statsQ.isError && (
                <div className="rounded-lg border border-amber-200 dark:border-amber-800 bg-amber-50 dark:bg-amber-950/30 px-4 py-2 text-xs text-amber-900 dark:text-amber-200">
                    Registry statistics unavailable — KPI cards below may be incomplete. Confirm <code className="text-[10px]">endpoints:read</code> for{' '}
                    agent statistics endpoints.
                </div>
            )}

            <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                <StatCard
                    title="Fleet total"
                    value={statsQ.isLoading ? '…' : registry ? String(registry.total) : '—'}
                    icon={Database}
                    subtext={registry ? `${registry.pending} pending · ${registry.suspended} suspended` : 'Registry'}
                />
                <StatCard
                    title="Registry online"
                    value={statsQ.isLoading ? '…' : registry ? String(registry.online) : '—'}
                    icon={Activity}
                    color="emerald"
                    subtext={registry ? `Avg health ${Math.round(registry.avg_health)}%` : '—'}
                />
                <StatCard
                    title="Offline / Degraded"
                    value={statsQ.isLoading ? '…' : registry ? `${registry.offline} / ${registry.degraded}` : '—'}
                    icon={AlertTriangle}
                    color="amber"
                />
                <StatCard title="OS mix (registry)" value={statsQ.isLoading ? '…' : osDistSummary} icon={Radio} subtext="From stats.by_os_type" />
            </div>

            <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm shadow-sm p-4 space-y-3">
                <div className="flex flex-col lg:flex-row gap-3 lg:items-center lg:justify-between">
                    <div className="relative flex-1 max-w-lg">
                        <Search className="w-4 h-4 text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" />
                        <input
                            value={searchInput}
                            onChange={(e) => setSearchInput(e.target.value)}
                            placeholder="Search hostname, id… (server-side)"
                            className="w-full pl-9 pr-3 py-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm text-slate-900 dark:text-white"
                        />
                    </div>
                    <p className="text-xs text-slate-500 dark:text-slate-400">
                        Matching rows: <span className="font-semibold text-slate-700 dark:text-slate-200">{agentsQ.isLoading ? '…' : totalMatching}</span>
                        {debouncedQ ? ` · filter “${debouncedQ}”` : ''} · Page size {PAGE_SIZE}
                    </p>
                </div>
            </div>

            {agentsQ.isError && (
                <div className="rounded-xl border border-rose-200 dark:border-rose-900/50 bg-rose-50/80 dark:bg-rose-950/20 p-6 text-sm text-rose-900 dark:text-rose-200">
                    Could not load agents. Check connection-manager and <code className="text-xs">endpoints:read</code>.
                </div>
            )}

            {agentsQ.isLoading && !agentsQ.data ? (
                <div className="h-48 rounded-xl bg-slate-100 dark:bg-slate-800 animate-pulse" />
            ) : agentsQ.isError ? null : totalMatching === 0 ? (
                <EmptyState title="No agents match" description="Try clearing search, or enroll endpoints first." />
            ) : (
                <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm shadow-sm">
                    <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-2 border-b border-slate-100 dark:border-slate-800 text-xs text-slate-500">
                        <span>
                            {totalMatching === 0 ? 'No rows' : `Showing ${fromIdx}–${toIdx} of ${totalMatching}`}
                        </span>
                        <span className="font-mono text-[10px] text-slate-400">
                            offset={apiOffset} limit={PAGE_SIZE}
                        </span>
                    </div>
                    <table className="min-w-full text-left text-sm">
                        <thead className="bg-slate-50 dark:bg-slate-900/50 text-slate-500 dark:text-slate-400 text-xs uppercase">
                            <tr>
                                <th className="px-3 py-2.5">Host</th>
                                <th className="px-3 py-2.5">Effective</th>
                                <th className="px-3 py-2.5">Health</th>
                                <th className="px-3 py-2.5">Isolated</th>
                                <th className="px-3 py-2.5">OS</th>
                                <th className="px-3 py-2.5">Agent ver.</th>
                                <th className="px-3 py-2.5">mTLS expiry</th>
                                <th className="px-3 py-2.5">Last seen</th>
                                <th className="px-3 py-2.5 hidden xl:table-cell">Agent ID</th>
                            </tr>
                        </thead>
                        <tbody>
                            {rows.map((a) => {
                                const eff = getEffectiveStatus(a);
                                return (
                                    <tr key={a.id} className="border-t border-slate-100 dark:border-slate-800 hover:bg-slate-50/80 dark:hover:bg-slate-800/50">
                                        <td className="px-3 py-2.5">
                                            <Link
                                                className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline"
                                                to={`/management/devices/${encodeURIComponent(a.id)}`}
                                            >
                                                {a.hostname || '—'}
                                            </Link>
                                        </td>
                                        <td className="px-3 py-2.5 text-xs font-mono text-slate-700 dark:text-slate-300">{eff}</td>
                                        <td className="px-3 py-2.5">
                                            <span
                                                className={`text-xs font-mono font-semibold ${
                                                    (a.health_score ?? 0) >= 80
                                                        ? 'text-emerald-600 dark:text-emerald-400'
                                                        : 'text-amber-600 dark:text-amber-400'
                                                }`}
                                            >
                                                {Math.round(a.health_score ?? 0)}%
                                            </span>
                                        </td>
                                        <td className="px-3 py-2.5 text-xs">{a.is_isolated ? 'Yes' : 'No'}</td>
                                        <td className="px-3 py-2.5 text-xs text-slate-700 dark:text-slate-200">
                                            {osLabel(a)}
                                            {a.os_version ? ` · ${a.os_version}` : ''}
                                        </td>
                                        <td className="px-3 py-2.5 text-xs font-mono">{a.agent_version || '—'}</td>
                                        <td className="px-3 py-2.5 text-xs text-slate-600 dark:text-slate-300 whitespace-nowrap">
                                            {a.cert_expires_at ? formatDate(a.cert_expires_at) : '—'}
                                        </td>
                                        <td className="px-3 py-2.5 text-xs text-slate-500">{formatRelativeTime(a.last_seen)}</td>
                                        <td className="px-3 py-2.5 text-[10px] font-mono text-slate-400 hidden xl:table-cell max-w-[120px] truncate" title={a.id}>
                                            {a.id}
                                        </td>
                                    </tr>
                                );
                            })}
                        </tbody>
                    </table>
                    {totalPages > 1 && (
                        <div className="flex items-center justify-between px-4 py-2 border-t border-slate-100 dark:border-slate-800 text-xs text-slate-500">
                            <span>
                                Page {viewPage} of {totalPages}
                            </span>
                            <div className="flex gap-1">
                                <button
                                    type="button"
                                    disabled={viewPage <= 1}
                                    onClick={() => setPage((p) => Math.max(1, p - 1))}
                                    className="p-1.5 rounded-lg border border-slate-300 dark:border-slate-600 hover:bg-slate-100 dark:hover:bg-slate-800 disabled:opacity-30"
                                >
                                    <ChevronLeft className="w-4 h-4" />
                                </button>
                                <button
                                    type="button"
                                    disabled={viewPage >= totalPages}
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

            <div className="rounded-xl border border-slate-200 dark:border-slate-700/60 bg-slate-50/80 dark:bg-slate-900/40 px-4 py-3 text-xs text-slate-600 dark:text-slate-400 flex flex-wrap items-center gap-2">
                <Shield className="w-4 h-4 shrink-0 text-slate-500" />
                <span>
                    Human user access:{' '}
                    <Link to="/system/access/users" className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline">
                        Access management
                    </Link>
                    — not endpoint agent profiles.
                </span>
                <span className="inline-flex items-center gap-1 ml-auto text-slate-500">
                    <Server className="w-3.5 h-3.5" /> connection-manager
                </span>
            </div>
        </div>
    );
}
