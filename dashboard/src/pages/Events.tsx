import { useQuery } from '@tanstack/react-query';
import { useEffect, useMemo, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { Activity, AlertTriangle, ChevronLeft, ChevronRight, Loader2, Search } from 'lucide-react';
import { authApi, eventsApi, type CmEventSummary } from '../api/client';
import { EventDetailModal } from '../components/EventDetailModal';
import { useDebounce } from '../hooks/useDebounce';

const DEFAULT_LIMIT = 50;

function isoDaysAgo(days: number) {
    return new Date(Date.now() - days * 24 * 60 * 60 * 1000).toISOString();
}

function Pagination({
    page,
    totalPages,
    onPage,
}: {
    page: number;
    totalPages: number;
    onPage: (p: number) => void;
}) {
    if (totalPages <= 1) return null;
    return (
        <div className="flex items-center justify-between text-sm">
            <button
                type="button"
                className="px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-white/60 dark:bg-slate-900/50 text-slate-700 dark:text-slate-200 disabled:opacity-50"
                onClick={() => onPage(Math.max(1, page - 1))}
                disabled={page <= 1}
            >
                <span className="inline-flex items-center gap-2">
                    <ChevronLeft className="w-4 h-4" /> Prev
                </span>
            </button>
            <div className="text-slate-500 dark:text-slate-400">
                Page <span className="font-semibold text-slate-700 dark:text-slate-200">{page}</span> / {totalPages}
            </div>
            <button
                type="button"
                className="px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-white/60 dark:bg-slate-900/50 text-slate-700 dark:text-slate-200 disabled:opacity-50"
                onClick={() => onPage(Math.min(totalPages, page + 1))}
                disabled={page >= totalPages}
            >
                <span className="inline-flex items-center gap-2">
                    Next <ChevronRight className="w-4 h-4" />
                </span>
            </button>
        </div>
    );
}

export default function Events() {
    const canView = authApi.canViewAlerts(); // server guards events/search with alerts:read
    const [sp, setSp] = useSearchParams();

    const [agentId, setAgentId] = useState(() => sp.get('agent_id') || '');
    const [eventType, setEventType] = useState(() => sp.get('event_type') || '');
    const [from, setFrom] = useState(() => sp.get('from') || isoDaysAgo(7));
    const [to, setTo] = useState(() => sp.get('to') || new Date().toISOString());
    const [page, setPage] = useState(() => Math.max(1, parseInt(sp.get('page') || '1', 10) || 1));
    const [detailId, setDetailId] = useState<string | null>(null);

    useEffect(() => {
        // keep URL in sync (bookmarkable)
        setSp((prev) => {
            const n = new URLSearchParams(prev);
            const setOrDel = (k: string, v: string) => (v ? n.set(k, v) : n.delete(k));
            setOrDel('agent_id', agentId.trim());
            setOrDel('event_type', eventType.trim());
            setOrDel('from', from);
            setOrDel('to', to);
            n.set('page', String(page));
            return n;
        });
    }, [agentId, eventType, from, to, page, setSp]);

    const debouncedAgentId = useDebounce(agentId.trim(), 250);
    const debouncedEventType = useDebounce(eventType.trim(), 250);

    const offset = (page - 1) * DEFAULT_LIMIT;

    const queryBody = useMemo(() => {
        const filters: { field: string; operator: string; value: unknown }[] = [];
        if (debouncedAgentId) filters.push({ field: 'agent_id', operator: 'equals', value: debouncedAgentId });
        if (debouncedEventType) filters.push({ field: 'event_type', operator: 'equals', value: debouncedEventType });
        return {
            filters,
            logic: 'AND' as const,
            time_range: { from, to },
            limit: DEFAULT_LIMIT,
            offset,
        };
    }, [debouncedAgentId, debouncedEventType, from, to, offset]);

    const q = useQuery({
        queryKey: ['events-search', queryBody],
        queryFn: () => eventsApi.search(queryBody),
        enabled: canView,
        staleTime: 15_000,
        retry: 1,
    });

    const rows: CmEventSummary[] = q.data?.data ?? [];
    const total = q.data?.pagination?.total ?? 0;
    const totalPages = Math.max(1, Math.ceil(total / DEFAULT_LIMIT));

    useEffect(() => {
        if (page > totalPages) setPage(totalPages);
    }, [page, totalPages]);

    if (!canView) {
        return (
            <div className="p-8 text-center text-slate-500">
                You do not have permission to view Events. This endpoint is guarded by <code className="text-xs">alerts:read</code>.
            </div>
        );
    }

    return (
        <div className="relative flex flex-col min-h-[calc(100vh-2rem)] -mx-4 sm:-mx-6 lg:-mx-8 -my-4 sm:-my-6 lg:-my-8 p-4 sm:p-6 lg:p-8 bg-slate-50 dark:bg-gradient-to-br dark:from-slate-900 dark:via-[#0b1120] dark:to-slate-900">
            <div className="max-w-[1600px] mx-auto w-full space-y-4">
                <div className="flex items-start gap-3">
                    <div className="p-2 rounded-xl border border-cyan-500/20 bg-cyan-500/10 text-cyan-700 dark:text-cyan-300">
                        <Activity className="w-6 h-6" />
                    </div>
                    <div className="flex-1">
                        <h1 className="text-2xl font-bold text-slate-900 dark:text-white">Telemetry Search</h1>
                        <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
                            Search stored telemetry via <code className="text-xs">POST /api/v1/events/search</code>. Click a row for{' '}
                            <code className="text-xs">GET /api/v1/events/:id</code> raw payload.
                        </p>
                    </div>
                    <Link
                        to="/alerts"
                        className="px-3 py-2 text-sm font-semibold rounded-lg border border-slate-200 dark:border-slate-700 bg-white/60 dark:bg-slate-900/50 text-slate-700 dark:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800"
                    >
                        Alerts (Triage)
                    </Link>
                </div>

                <div className="rounded-xl border border-slate-200 dark:border-slate-700 bg-white/70 dark:bg-slate-900/40 backdrop-blur p-4 sm:p-5 space-y-4">
                    <div className="grid grid-cols-1 md:grid-cols-4 gap-3">
                        <div className="md:col-span-2">
                            <label className="block text-xs font-semibold text-slate-500 uppercase mb-1">Agent ID</label>
                            <div className="relative">
                                <Search className="w-4 h-4 text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" />
                                <input
                                    className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-9 py-2 text-sm"
                                    value={agentId}
                                    onChange={(e) => { setAgentId(e.target.value); setPage(1); }}
                                    placeholder="UUID (optional)"
                                />
                            </div>
                        </div>
                        <div>
                            <label className="block text-xs font-semibold text-slate-500 uppercase mb-1">Event type</label>
                            <input
                                className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-3 py-2 text-sm"
                                value={eventType}
                                onChange={(e) => { setEventType(e.target.value); setPage(1); }}
                                placeholder="e.g. network, process, file"
                            />
                        </div>
                        <div className="flex items-end">
                            <button
                                type="button"
                                className="w-full px-3 py-2 text-sm font-semibold rounded-lg border border-slate-200 dark:border-slate-700 bg-white/70 dark:bg-slate-900/50 hover:bg-slate-100 dark:hover:bg-slate-800"
                                onClick={() => {
                                    setAgentId('');
                                    setEventType('');
                                    setFrom(isoDaysAgo(7));
                                    setTo(new Date().toISOString());
                                    setPage(1);
                                }}
                            >
                                Reset
                            </button>
                        </div>
                    </div>

                    <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                        <div>
                            <label className="block text-xs font-semibold text-slate-500 uppercase mb-1">From (ISO)</label>
                            <input
                                className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-3 py-2 text-sm font-mono"
                                value={from}
                                onChange={(e) => { setFrom(e.target.value); setPage(1); }}
                            />
                        </div>
                        <div>
                            <label className="block text-xs font-semibold text-slate-500 uppercase mb-1">To (ISO)</label>
                            <input
                                className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-3 py-2 text-sm font-mono"
                                value={to}
                                onChange={(e) => { setTo(e.target.value); setPage(1); }}
                            />
                        </div>
                    </div>

                    <div className="flex items-center justify-between text-xs text-slate-500 dark:text-slate-400">
                        <div>
                            Showing <span className="font-semibold text-slate-700 dark:text-slate-200">{rows.length}</span> rows
                            {total ? <> / total {total}</> : null}
                        </div>
                        <div className="font-mono">
                            limit={DEFAULT_LIMIT} offset={offset}
                        </div>
                    </div>
                </div>

                {q.isLoading ? (
                    <div className="flex items-center justify-center py-16">
                        <Loader2 className="w-10 h-10 animate-spin text-cyan-500" />
                    </div>
                ) : q.isError ? (
                    <div className="rounded-xl border border-rose-200 dark:border-rose-900/50 bg-rose-50/80 dark:bg-rose-950/20 p-5 text-sm text-rose-900 dark:text-rose-200 flex items-start gap-3">
                        <AlertTriangle className="w-5 h-5 mt-0.5 shrink-0" />
                        <div>
                            Failed to search events. If the status is 405, the reverse proxy must forward{' '}
                            <code className="text-xs">/api/v1/events/</code> to connection-manager (see dashboard{' '}
                            <code className="text-xs">nginx.conf</code>).
                        </div>
                    </div>
                ) : rows.length === 0 ? (
                    <div className="rounded-xl border border-dashed border-slate-300 dark:border-slate-700 p-8 text-center text-slate-500 dark:text-slate-400">
                        No events returned for this query. If you expect data, the backend <code className="text-xs">SearchEvents</code> handler may still be stubbed.
                    </div>
                ) : (
                    <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-700 bg-white/70 dark:bg-slate-900/40 backdrop-blur">
                        <table className="w-full text-left text-sm">
                            <thead className="bg-slate-100 dark:bg-slate-800/80 text-slate-600 dark:text-slate-300 text-xs uppercase">
                                <tr>
                                    <th className="p-3">Time</th>
                                    <th className="p-3">Agent</th>
                                    <th className="p-3">Type</th>
                                    <th className="p-3">Summary</th>
                                </tr>
                            </thead>
                            <tbody>
                                {rows.map((e) => (
                                    <tr
                                        key={e.id}
                                        role="button"
                                        tabIndex={0}
                                        className="border-t border-slate-100 dark:border-slate-800 cursor-pointer hover:bg-slate-50/80 dark:hover:bg-slate-800/40"
                                        onClick={() => setDetailId(e.id)}
                                        onKeyDown={(ev) => {
                                            if (ev.key === 'Enter' || ev.key === ' ') {
                                                ev.preventDefault();
                                                setDetailId(e.id);
                                            }
                                        }}
                                    >
                                        <td className="p-3 whitespace-nowrap text-xs font-mono text-slate-500">
                                            {new Date(e.timestamp).toLocaleString()}
                                        </td>
                                        <td className="p-3">
                                            <Link
                                                className="text-cyan-700 dark:text-cyan-300 hover:underline font-mono text-xs"
                                                to={`/management/devices/${encodeURIComponent(e.agent_id)}?tab=activity`}
                                                onClick={(ev) => ev.stopPropagation()}
                                            >
                                                {e.agent_id.slice(0, 8)}…
                                            </Link>
                                        </td>
                                        <td className="p-3 font-mono text-xs">{e.event_type}</td>
                                        <td className="p-3">{e.summary}</td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                )}

                <Pagination page={page} totalPages={totalPages} onPage={setPage} />

                <EventDetailModal eventId={detailId} onClose={() => setDetailId(null)} fetchEnabled={canView} />
            </div>
        </div>
    );
}

