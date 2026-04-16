import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Search, Activity, Database, Clock, Monitor, ChevronDown, X } from 'lucide-react';
import { connectionApi } from '../api/client';

// ── Types ────────────────────────────────────────────────────────────────────

interface EventSummary {
    id: string;
    agent_id: string;
    event_type: string;
    timestamp: string;
    summary: string;
}

interface EventStats {
    total: number;
    by_type: Record<string, number>;
    by_agent: Record<string, number>;
}

interface EventDetailData {
    id: string;
    agent_id: string;
    event_type: string;
    timestamp: string;
    raw_data?: Record<string, unknown>;
    summary?: string;
}

// ── Colour map for event types ────────────────────────────────────────────────

const EVENT_TYPE_COLORS: Record<string, string> = {
    process:        'bg-blue-500/10 text-blue-600 dark:text-blue-400 border-blue-500/20',
    network:        'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/20',
    file:           'bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20',
    registry:       'bg-purple-500/10 text-purple-600 dark:text-purple-400 border-purple-500/20',
    dns:            'bg-cyan-500/10 text-cyan-600 dark:text-cyan-400 border-cyan-500/20',
    authentication: 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border-rose-500/20',
    module:         'bg-indigo-500/10 text-indigo-600 dark:text-indigo-400 border-indigo-500/20',
};

function eventTypeBadge(type: string): string {
    return EVENT_TYPE_COLORS[type.toLowerCase()] || 'bg-slate-500/10 text-slate-600 dark:text-slate-400 border-slate-500/20';
}

// ── Event detail slide panel ──────────────────────────────────────────────────

function EventDetailPanel({ id, onClose }: { id: string; onClose: () => void }) {
    const { data, isLoading } = useQuery({
        queryKey: ['event', id],
        queryFn: async () => {
            const r = await connectionApi.get<{ data: EventDetailData }>(`/api/v1/events/${id}`);
            return r.data.data;
        },
        enabled: !!id,
    });

    return (
        <div
            className="hidden lg:flex flex-col w-[45%] xl:w-[40%] bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700/50 rounded-2xl shadow-xl shrink-0"
            style={{ animation: 'slideInRight 0.2s ease-out', height: '100%', minHeight: 0 }}
        >
            <div className="flex items-center justify-between px-5 py-4 border-b border-slate-200 dark:border-slate-700/50 shrink-0 bg-slate-50/80 dark:bg-slate-900/60">
                <div className="flex items-center gap-2 min-w-0">
                    <Activity className="w-4 h-4 text-indigo-500 shrink-0" />
                    <span className="text-sm font-bold text-slate-800 dark:text-white truncate">Event Detail</span>
                </div>
                <button onClick={onClose} className="p-1.5 rounded-lg hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-400 hover:text-slate-600 transition-colors shrink-0 ml-3">
                    <X className="w-4 h-4" />
                </button>
            </div>
            <div className="flex-1 overflow-y-auto custom-scrollbar p-5 space-y-5">
                {isLoading ? (
                    <div className="space-y-3">
                        {[...Array(5)].map((_, i) => (
                            <div key={i} className="h-8 bg-slate-100 dark:bg-slate-700 rounded-lg animate-pulse" />
                        ))}
                    </div>
                ) : data ? (
                    <>
                        <div className="grid grid-cols-2 gap-x-6 gap-y-3">
                            <div className="col-span-2">
                                <label className="text-[10px] text-gray-400 uppercase tracking-wider font-bold">Event ID</label>
                                <p className="font-mono text-xs text-gray-600 dark:text-gray-300 mt-0.5 break-all">{data.id}</p>
                            </div>
                            <div>
                                <label className="text-[10px] text-gray-400 uppercase tracking-wider font-bold">Type</label>
                                <div className="mt-1">
                                    <span className={`inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-bold border ${eventTypeBadge(data.event_type)}`}>
                                        {data.event_type}
                                    </span>
                                </div>
                            </div>
                            <div>
                                <label className="text-[10px] text-gray-400 uppercase tracking-wider font-bold">Timestamp</label>
                                <p className="text-gray-700 dark:text-gray-300 text-xs mt-0.5">{new Date(data.timestamp).toLocaleString()}</p>
                            </div>
                            <div className="col-span-2">
                                <label className="text-[10px] text-gray-400 uppercase tracking-wider font-bold">Agent ID</label>
                                <p className="font-mono text-xs text-gray-600 dark:text-gray-300 mt-0.5 break-all">{data.agent_id}</p>
                            </div>
                            {data.summary && (
                                <div className="col-span-2">
                                    <label className="text-[10px] text-gray-400 uppercase tracking-wider font-bold">Summary</label>
                                    <p className="text-gray-700 dark:text-gray-300 text-sm mt-0.5">{data.summary}</p>
                                </div>
                            )}
                        </div>

                        {data.raw_data && Object.keys(data.raw_data).length > 0 && (
                            <div>
                                <label className="text-[10px] text-gray-400 uppercase tracking-wider font-bold block mb-2">Raw Event Data</label>
                                <pre className="p-3 bg-gray-100 dark:bg-gray-900 rounded-lg overflow-auto max-h-[55vh] text-[11px] font-mono text-gray-700 dark:text-gray-300 whitespace-pre-wrap break-all">
                                    {JSON.stringify(data.raw_data, null, 2)}
                                </pre>
                            </div>
                        )}
                    </>
                ) : (
                    <p className="text-sm text-gray-400 text-center py-12">No event data found.</p>
                )}
            </div>
        </div>
    );
}

// ── Main Events Page ──────────────────────────────────────────────────────────

const EVENT_TYPE_OPTIONS = [
    'process', 'network', 'file', 'registry', 'dns', 'authentication', 'module',
];

export default function EventsExplorer() {
    const [search, setSearch] = useState('');
    const [selectedType, setSelectedType] = useState('');
    const [fromDate, setFromDate] = useState(() => new Date(Date.now() - 6 * 3600 * 1000).toISOString().slice(0, 16));
    const [toDate, setToDate] = useState(() => new Date().toISOString().slice(0, 16));
    const [limit, setLimit] = useState(100);
    const [selectedEventId, setSelectedEventId] = useState<string | null>(null);

    // Stats
    const { data: statsData } = useQuery({
        queryKey: ['eventStats'],
        queryFn: async () => {
            const r = await connectionApi.get<{ data: EventStats }>('/api/v1/events/stats');
            return r.data.data;
        },
        refetchInterval: 30000,
    });

    // Search / list events
    const { data: eventsData, isLoading } = useQuery({
        queryKey: ['events', search, selectedType, fromDate, toDate, limit],
        queryFn: async () => {
            const filters = [];
            if (search) filters.push({ field: 'summary', operator: 'contains', value: search });
            if (selectedType) filters.push({ field: 'event_type', operator: 'equals', value: selectedType });

            const r = await connectionApi.post<{ data: EventSummary[]; pagination: { total: number } }>('/api/v1/events/search', {
                filters,
                logic: 'AND',
                time_range: {
                    from: new Date(fromDate).toISOString(),
                    to: new Date(toDate).toISOString(),
                },
                limit,
                offset: 0,
            });
            return r.data;
        },
        refetchInterval: 60000,
    });

    const events = eventsData?.data || [];
    const total = eventsData?.pagination?.total || 0;
    const byType: Record<string, number> = statsData?.by_type || {};
    const topTypes = Object.entries(byType).sort((a, b) => b[1] - a[1]).slice(0, 6);

    return (
        <div className="relative flex flex-col min-h-[calc(100vh-2rem)] -mx-4 sm:-mx-6 lg:-mx-8 -my-4 sm:-my-6 lg:-my-8 p-4 sm:p-6 lg:p-8 bg-slate-50 dark:bg-gradient-to-br dark:from-slate-900 dark:via-[#0b1120] dark:to-slate-900 overflow-hidden">
            {/* Ambient */}
            <div className="absolute top-0 right-0 w-[600px] h-[400px] pointer-events-none" style={{ background: 'radial-gradient(circle, rgba(99,102,241,0.07) 0%, transparent 70%)' }} />

            <div className="relative flex-1 flex flex-col min-h-0 space-y-5 max-w-[1700px] mx-auto w-full">

                {/* Header */}
                <div className="flex items-center justify-between shrink-0">
                    <div>
                        <h1 className="text-3xl font-bold text-slate-900 dark:text-white tracking-tight">Event Explorer</h1>
                        <p className="text-sm text-slate-500 mt-1">Search and inspect raw telemetry events from all endpoints</p>
                    </div>
                    <div className="flex items-center gap-2 text-sm bg-white/60 dark:bg-slate-800/60 backdrop-blur-md px-4 py-2 rounded-xl border border-slate-200 dark:border-slate-700/50 shadow-sm">
                        <Database className="w-4 h-4 text-indigo-500" />
                        <span className="text-slate-600 dark:text-slate-300">
                            <span className="font-bold text-slate-800 dark:text-white">{statsData?.total?.toLocaleString() ?? '—'}</span> total events indexed
                        </span>
                    </div>
                </div>

                {/* Type breakdown mini cards */}
                {topTypes.length > 0 && (
                    <div className="grid grid-cols-3 md:grid-cols-6 gap-3 shrink-0">
                        {topTypes.map(([type, count]) => (
                            <button
                                key={type}
                                onClick={() => setSelectedType(selectedType === type ? '' : type)}
                                className={`flex flex-col items-center py-3 px-2 rounded-xl border text-center transition-all cursor-pointer ${
                                    selectedType === type
                                        ? 'bg-indigo-600 text-white border-indigo-600 shadow-md shadow-indigo-500/20'
                                        : 'bg-white dark:bg-slate-800/70 border-slate-200 dark:border-slate-700/60 hover:border-indigo-400 dark:hover:border-indigo-500'
                                }`}
                            >
                                <span className={`text-xl font-extrabold ${selectedType === type ? 'text-white' : 'text-slate-800 dark:text-white'}`}>
                                    {count.toLocaleString()}
                                </span>
                                <span className={`text-[10px] uppercase font-bold mt-0.5 ${selectedType === type ? 'text-indigo-200' : 'text-slate-400'}`}>
                                    {type}
                                </span>
                            </button>
                        ))}
                    </div>
                )}

                {/* Filter bar */}
                <div className="shrink-0 bg-white/70 dark:bg-slate-900/50 backdrop-blur-md border border-slate-200/80 dark:border-slate-700/50 rounded-xl p-4 shadow-sm">
                    <div className="flex flex-wrap gap-3 items-end">
                        {/* Search */}
                        <div className="flex-1 min-w-[200px]">
                            <label className="block text-[10px] font-bold text-slate-500 uppercase tracking-wider mb-1.5">Search</label>
                            <div className="relative">
                                <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-400" />
                                <input
                                    type="text"
                                    placeholder="Event summary keyword…"
                                    value={search}
                                    onChange={e => setSearch(e.target.value)}
                                    className="w-full bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-200 rounded-lg pl-9 pr-3 py-2 text-sm focus:ring-2 focus:ring-indigo-500/50 focus:border-indigo-500 transition-all"
                                />
                            </div>
                        </div>

                        {/* Type */}
                        <div className="w-40">
                            <label className="block text-[10px] font-bold text-slate-500 uppercase tracking-wider mb-1.5">Event Type</label>
                            <div className="relative">
                                <select
                                    value={selectedType}
                                    onChange={e => setSelectedType(e.target.value)}
                                    className="appearance-none w-full bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-200 rounded-lg pl-3 pr-8 py-2 text-sm focus:ring-2 focus:ring-indigo-500/50 transition-all cursor-pointer"
                                >
                                    <option value="">All Types</option>
                                    {EVENT_TYPE_OPTIONS.map(t => <option key={t} value={t}>{t}</option>)}
                                </select>
                                <ChevronDown className="absolute right-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-400 pointer-events-none" />
                            </div>
                        </div>

                        {/* From */}
                        <div>
                            <label className="block text-[10px] font-bold text-slate-500 uppercase tracking-wider mb-1.5">From</label>
                            <input type="datetime-local" value={fromDate} onChange={e => setFromDate(e.target.value)}
                                className="bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-indigo-500/50 transition-all" />
                        </div>

                        {/* To */}
                        <div>
                            <label className="block text-[10px] font-bold text-slate-500 uppercase tracking-wider mb-1.5">To</label>
                            <input type="datetime-local" value={toDate} onChange={e => setToDate(e.target.value)}
                                className="bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-indigo-500/50 transition-all" />
                        </div>

                        {/* Limit */}
                        <div className="w-24">
                            <label className="block text-[10px] font-bold text-slate-500 uppercase tracking-wider mb-1.5">Limit</label>
                            <div className="relative">
                                <select
                                    value={limit}
                                    onChange={e => setLimit(Number(e.target.value))}
                                    className="appearance-none w-full bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-200 rounded-lg pl-3 pr-7 py-2 text-sm focus:ring-2 focus:ring-indigo-500/50 transition-all cursor-pointer"
                                >
                                    {[50, 100, 250, 500].map(n => <option key={n} value={n}>{n}</option>)}
                                </select>
                                <ChevronDown className="absolute right-2 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-slate-400 pointer-events-none" />
                            </div>
                        </div>
                    </div>
                </div>

                {/* Split pane */}
                <div className="relative flex-1 flex min-h-0 gap-4 overflow-hidden">

                    {/* Table */}
                    <div className={`flex flex-col min-h-0 bg-white dark:bg-slate-800/70 rounded-2xl border border-slate-200 dark:border-slate-700/60 shadow-sm overflow-hidden transition-all duration-300 ${selectedEventId ? 'w-full lg:w-[58%]' : 'w-full'}`}>
                        <div className="flex-1 overflow-auto custom-scrollbar">
                            {isLoading ? (
                                <div className="p-5 space-y-2">
                                    {[...Array(8)].map((_, i) => (
                                        <div key={i} className="h-12 bg-slate-100 dark:bg-slate-700 rounded-lg animate-pulse" style={{ opacity: 1 - i * 0.1 }} />
                                    ))}
                                </div>
                            ) : events.length === 0 ? (
                                <div className="flex flex-col items-center justify-center py-20 text-center">
                                    <Activity className="w-12 h-12 text-slate-300 dark:text-slate-600 mb-4" />
                                    <h3 className="text-base font-semibold text-slate-700 dark:text-slate-300">No events found</h3>
                                    <p className="text-sm text-slate-500 mt-1">Try adjusting your filters or time range.</p>
                                </div>
                            ) : (
                                <table className="w-full text-left border-collapse text-sm">
                                    <thead className="sticky top-0 z-10 bg-slate-100/90 dark:bg-slate-800 border-b-2 border-slate-200 dark:border-slate-700/80 backdrop-blur-sm">
                                        <tr>
                                            <th className="px-4 py-3 text-[10px] uppercase tracking-wider text-slate-500 dark:text-slate-400 font-bold">Type</th>
                                            <th className="px-4 py-3 text-[10px] uppercase tracking-wider text-slate-500 dark:text-slate-400 font-bold">Timestamp</th>
                                            <th className="px-4 py-3 text-[10px] uppercase tracking-wider text-slate-500 dark:text-slate-400 font-bold">Summary</th>
                                            <th className="px-4 py-3 text-[10px] uppercase tracking-wider text-slate-500 dark:text-slate-400 font-bold">Agent ID</th>
                                        </tr>
                                    </thead>
                                    <tbody className="divide-y divide-slate-100 dark:divide-slate-800/50">
                                        {events.map(evt => (
                                            <tr
                                                key={evt.id}
                                                onClick={() => setSelectedEventId(selectedEventId === evt.id ? null : evt.id)}
                                                className={`cursor-pointer transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/40 ${selectedEventId === evt.id ? 'bg-indigo-50 dark:bg-indigo-900/20 ring-1 ring-inset ring-indigo-400/30' : ''}`}
                                            >
                                                <td className="px-4 py-3">
                                                    <span className={`inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-bold border ${eventTypeBadge(evt.event_type)}`}>
                                                        {evt.event_type}
                                                    </span>
                                                </td>
                                                <td className="px-4 py-3 text-xs text-slate-500 dark:text-slate-400 whitespace-nowrap">
                                                    <span className="flex items-center gap-1.5">
                                                        <Clock className="w-3 h-3 shrink-0" />
                                                        {new Date(evt.timestamp).toLocaleString()}
                                                    </span>
                                                </td>
                                                <td className="px-4 py-3 text-sm text-slate-700 dark:text-slate-300 max-w-xs truncate" title={evt.summary}>
                                                    {evt.summary || <span className="italic text-slate-400">No summary</span>}
                                                </td>
                                                <td className="px-4 py-3">
                                                    <span className="flex items-center gap-1.5 text-xs text-slate-500 font-mono">
                                                        <Monitor className="w-3 h-3 shrink-0" />
                                                        {evt.agent_id.length > 16 ? evt.agent_id.slice(0, 16) + '…' : evt.agent_id}
                                                    </span>
                                                </td>
                                            </tr>
                                        ))}
                                    </tbody>
                                </table>
                            )}
                        </div>

                        {/* Footer */}
                        <div className="shrink-0 px-5 py-3 bg-slate-50/60 dark:bg-slate-900/40 border-t border-slate-200 dark:border-slate-800/60 text-xs text-slate-500 dark:text-slate-400 flex justify-between items-center">
                            <span>Showing <span className="font-semibold text-slate-700 dark:text-slate-200">{events.length}</span> of <span className="font-semibold text-slate-700 dark:text-slate-200">{total.toLocaleString()}</span> events</span>
                            {selectedType && (
                                <button onClick={() => setSelectedType('')} className="flex items-center gap-1 text-indigo-500 hover:text-indigo-700 font-medium">
                                    <X className="w-3 h-3" /> Clear type filter
                                </button>
                            )}
                        </div>
                    </div>

                    {/* Detail panel */}
                    {selectedEventId && (
                        <EventDetailPanel id={selectedEventId} onClose={() => setSelectedEventId(null)} />
                    )}
                </div>
            </div>
        </div>
    );
}
