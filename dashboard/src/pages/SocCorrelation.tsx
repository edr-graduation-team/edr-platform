import { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Activity, AlertTriangle, Link as LinkIcon, Terminal } from 'lucide-react';
import { alertsApi, agentsApi, commandsApi, eventsApi, type CmEventSummary } from '../api/client';
import { DateRangePicker, type DateRange } from '../components/DateRangePicker';
import StatCard from '../components/StatCard';
import EmptyState from '../components/EmptyState';

function isoOrNull(d: Date | null): string | null {
    if (!d) return null;
    return d.toISOString();
}

function last24hRange(): DateRange {
    return { from: new Date(Date.now() - 24 * 60 * 60 * 1000), to: new Date() };
}

function safeHostnameLabel(hostname?: string) {
    return hostname && hostname.trim().length > 0 ? hostname.trim() : 'Unknown host';
}

export default function SocCorrelation() {
    const [agentId, setAgentId] = useState<string>('');
    const [range, setRange] = useState<DateRange>(() => last24hRange());

    const fromIso = isoOrNull(range.from);
    const toIso = isoOrNull(range.to);

    const agentsQ = useQuery({
        queryKey: ['agents', 'list', 'correlation'],
        queryFn: async () => {
            const out: Awaited<ReturnType<typeof agentsApi.list>>['data'] = [];
            let offset = 0;
            const limit = 500;
            for (let i = 0; i < 10; i++) {
                const r = await agentsApi.list({ limit, offset, sort_by: 'hostname', sort_order: 'asc' });
                out.push(...(r.data ?? []));
                if (!r.pagination?.has_more) break;
                offset += limit;
            }
            return out;
        },
        staleTime: 30_000,
    });

    const selectedAgent = useMemo(() => {
        if (!agentId) return null;
        return (agentsQ.data ?? []).find((a) => a.id === agentId) ?? null;
    }, [agentId, agentsQ.data]);

    const alertsQ = useQuery({
        queryKey: ['correlation', 'alerts', agentId, fromIso, toIso],
        queryFn: async () => {
            if (!agentId || !fromIso || !toIso) return { alerts: [], total: 0 };
            return alertsApi.list({
                limit: 200,
                offset: 0,
                agent_id: agentId,
                date_from: fromIso,
                date_to: toIso,
                sort: 'timestamp',
                order: 'desc',
            });
        },
        enabled: !!agentId && !!fromIso && !!toIso,
        staleTime: 20_000,
        retry: 1,
    });

    const commandsQ = useQuery({
        queryKey: ['correlation', 'commands', agentId, fromIso, toIso],
        queryFn: async () => {
            if (!agentId) return { data: [], pagination: { total: 0, limit: 0, offset: 0, has_more: false } };
            const r = await commandsApi.list({
                limit: 200,
                offset: 0,
                agent_id: agentId,
                sort_by: 'issued_at',
                sort_order: 'desc',
            });
            const fromT = range.from?.getTime() ?? 0;
            const toT = range.to?.getTime() ?? Date.now();
            const filtered = (r.data ?? []).filter((c) => {
                const t = new Date(c.issued_at).getTime();
                return t >= fromT && t <= toT;
            });
            return { ...r, data: filtered };
        },
        enabled: !!agentId,
        staleTime: 20_000,
        retry: 1,
    });

    const eventsQ = useQuery({
        queryKey: ['correlation', 'events', agentId, fromIso, toIso],
        queryFn: async () => {
            if (!agentId || !fromIso || !toIso) return { rows: [] as CmEventSummary[], total: 0 };
            const r = await eventsApi.search({
                filters: [{ field: 'agent_id', operator: 'eq', value: agentId }],
                logic: 'AND',
                time_range: { from: fromIso, to: toIso },
                limit: 200,
                offset: 0,
            });
            return { rows: r.data ?? [], total: r.pagination?.total ?? (r.data?.length ?? 0) };
        },
        enabled: !!agentId && !!fromIso && !!toIso,
        staleTime: 20_000,
        retry: 1,
    });

    const eventTypeCounts = useMemo(() => {
        const rows = eventsQ.data?.rows ?? [];
        const map = new Map<string, number>();
        for (const r of rows) {
            map.set(r.event_type, (map.get(r.event_type) ?? 0) + 1);
        }
        return [...map.entries()].sort((a, b) => b[1] - a[1]).slice(0, 8);
    }, [eventsQ.data?.rows]);

    const openAlerts = useMemo(() => {
        const rows = alertsQ.data?.alerts ?? [];
        return rows.filter((a) => a.status === 'open').length;
    }, [alertsQ.data]);

    return (
        <div className="space-y-5">
            <div>
                <h1 className="text-xl font-bold text-gray-900 dark:text-white">Correlation</h1>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    Correlate alerts, command activity, and telemetry for a specific endpoint within a time range.
                </p>
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
                <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40 p-4 space-y-2">
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">Endpoint</label>
                    <select
                        value={agentId}
                        onChange={(e) => setAgentId(e.target.value)}
                        className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-sm text-gray-900 dark:text-white"
                    >
                        <option value="">Select endpoint…</option>
                        {(agentsQ.data ?? []).map((a) => (
                            <option key={a.id} value={a.id}>
                                {safeHostnameLabel(a.hostname)} — {a.id}
                            </option>
                        ))}
                    </select>
                    {selectedAgent && (
                        <div className="text-xs text-gray-500 dark:text-gray-400">
                            {selectedAgent.os_type} · {selectedAgent.os_version} · agent {selectedAgent.agent_version}
                        </div>
                    )}
                </div>

                <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40 p-4 space-y-2">
                    <DateRangePicker label="Time range" value={range} onChange={setRange} />
                    <div className="text-xs text-gray-500 dark:text-gray-400">
                        Tip: choose “Last 24h” for fastest correlation.
                    </div>
                </div>

                <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40 p-4 space-y-2">
                    <div className="text-sm font-semibold text-gray-900 dark:text-white">Quick links</div>
                    <div className="flex flex-wrap gap-2 text-sm">
                        <a
                            className="inline-flex items-center gap-1 px-3 py-2 rounded-lg bg-cyan-600 hover:bg-cyan-700 text-white font-semibold"
                            href={agentId ? `/management/devices/${encodeURIComponent(agentId)}` : '#'}
                            onClick={(e) => {
                                if (!agentId) e.preventDefault();
                            }}
                        >
                            <LinkIcon className="w-4 h-4" />
                            Device profile
                        </a>
                    </div>
                    {!agentId && (
                        <div className="text-xs text-gray-500 dark:text-gray-400">Select an endpoint to enable links.</div>
                    )}
                </div>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
                <StatCard title="Alerts (loaded)" value={agentId ? String(alertsQ.data?.alerts?.length ?? 0) : '—'} icon={AlertTriangle} color="amber" />
                <StatCard title="Open alerts" value={agentId ? String(openAlerts) : '—'} icon={AlertTriangle} color="red" />
                <StatCard title="Commands (loaded)" value={agentId ? String(commandsQ.data?.data?.length ?? 0) : '—'} icon={Terminal} color="cyan" />
                <StatCard title="Telemetry (loaded)" value={agentId ? String(eventsQ.data?.rows?.length ?? 0) : '—'} icon={Activity} color="emerald" />
            </div>

            {!agentId ? (
                <EmptyState
                    title="Select an endpoint"
                    description="Choose an endpoint from the dropdown to view correlation results."
                />
            ) : (
                <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                    <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40 p-4">
                        <div className="flex items-center justify-between gap-3 mb-3">
                            <div>
                                <div className="text-sm font-semibold text-gray-900 dark:text-white">Recent alerts</div>
                                <div className="text-xs text-gray-500 dark:text-gray-400">Top 20 (time-filtered).</div>
                            </div>
                        </div>
                        <div className="overflow-x-auto">
                            <table className="min-w-full text-left text-sm">
                                <thead className="text-xs uppercase text-gray-500 dark:text-gray-400">
                                    <tr>
                                        <th className="py-2 pr-3">Time</th>
                                        <th className="py-2 pr-3">Severity</th>
                                        <th className="py-2">Rule</th>
                                    </tr>
                                </thead>
                                <tbody className="text-xs">
                                    {(alertsQ.data?.alerts ?? []).slice(0, 20).map((a) => (
                                        <tr key={a.id} className="border-t border-gray-100 dark:border-gray-800">
                                            <td className="py-2 pr-3 whitespace-nowrap">{new Date(a.timestamp).toLocaleString()}</td>
                                            <td className="py-2 pr-3 font-mono">{a.severity}</td>
                                            <td className="py-2">{a.rule_title}</td>
                                        </tr>
                                    ))}
                                    {(alertsQ.data?.alerts ?? []).length === 0 && (
                                        <tr>
                                            <td colSpan={3} className="py-6 text-center text-gray-500 dark:text-gray-400">
                                                No alerts found in this range.
                                            </td>
                                        </tr>
                                    )}
                                </tbody>
                            </table>
                        </div>
                    </div>

                    <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40 p-4">
                        <div className="mb-3">
                            <div className="text-sm font-semibold text-gray-900 dark:text-white">Telemetry by type</div>
                            <div className="text-xs text-gray-500 dark:text-gray-400">Top event types (loaded page).</div>
                        </div>
                        {eventTypeCounts.length === 0 ? (
                            <div className="py-10 text-center text-sm text-gray-500 dark:text-gray-400">
                                No telemetry found in this range.
                            </div>
                        ) : (
                            <div className="space-y-2">
                                {eventTypeCounts.map(([t, n]) => (
                                    <div key={t} className="flex items-center gap-3">
                                        <div className="w-40 text-xs font-mono text-gray-700 dark:text-gray-300 truncate" title={t}>
                                            {t}
                                        </div>
                                        <div className="flex-1 h-2 rounded bg-gray-100 dark:bg-gray-800 overflow-hidden">
                                            <div
                                                className="h-2 bg-cyan-500"
                                                style={{ width: `${Math.min(100, Math.round((n / (eventTypeCounts[0]?.[1] ?? 1)) * 100))}%` }}
                                            />
                                        </div>
                                        <div className="w-10 text-right text-xs text-gray-500 dark:text-gray-400 font-mono">{n}</div>
                                    </div>
                                ))}
                            </div>
                        )}
                    </div>
                </div>
            )}
        </div>
    );
}

