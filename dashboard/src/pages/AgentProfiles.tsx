import { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { Search, Server, Shield } from 'lucide-react';
import { agentsApi, type Agent } from '../api/client';
import { formatRelativeTime, getEffectiveStatus } from '../utils/agentDisplay';
import EmptyState from '../components/EmptyState';

function osLabel(a: Agent) {
    const os = (a.os_type || '').toLowerCase();
    if (os === 'windows') return 'Windows';
    if (os === 'linux') return 'Linux';
    if (os === 'macos') return 'macOS';
    return a.os_type || 'Unknown';
}

export default function AgentProfiles() {
    const [q, setQ] = useState('');

    const agentsQ = useQuery({
        queryKey: ['agents', 'profiles', q],
        queryFn: async () => {
            const out: Agent[] = [];
            let offset = 0;
            const limit = 200;
            for (let i = 0; i < 20; i++) {
                const r = await agentsApi.list({ limit, offset, sort_by: 'hostname', sort_order: 'asc', search: q || undefined });
                out.push(...(r.data ?? []));
                if (!r.pagination?.has_more) break;
                offset += limit;
            }
            return out;
        },
        staleTime: 15_000,
        retry: 1,
    });

    const counts = useMemo(() => {
        const rows = agentsQ.data ?? [];
        const online = rows.filter((a) => getEffectiveStatus(a) === 'online').length;
        const degraded = rows.filter((a) => getEffectiveStatus(a) === 'degraded').length;
        const offline = rows.filter((a) => getEffectiveStatus(a) === 'offline').length;
        return { total: rows.length, online, degraded, offline };
    }, [agentsQ.data]);

    return (
        <div className="space-y-4">
            <div>
                <h1 className="text-xl font-bold text-gray-900 dark:text-white">Agents Profiles</h1>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    Device profiles for enrolled endpoints (status, OS, version, last seen, and identity).
                </p>
            </div>

            <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40 p-4 space-y-3">
                <div className="flex flex-col sm:flex-row gap-2 sm:items-center">
                    <div className="relative flex-1">
                        <Search className="w-4 h-4 text-gray-400 absolute left-3 top-1/2 -translate-y-1/2" />
                        <input
                            value={q}
                            onChange={(e) => setQ(e.target.value)}
                            placeholder="Search by hostname / id…"
                            className="w-full pl-9 pr-3 py-2 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-950 text-sm"
                        />
                    </div>
                    <div className="text-xs text-gray-500 dark:text-gray-400">
                        Total: <span className="font-semibold">{counts.total}</span> · Online:{' '}
                        <span className="font-semibold">{counts.online}</span> · Degraded:{' '}
                        <span className="font-semibold">{counts.degraded}</span> · Offline:{' '}
                        <span className="font-semibold">{counts.offline}</span>
                    </div>
                </div>
            </div>

            {agentsQ.isLoading ? (
                <div className="h-40 rounded-xl bg-gray-100 dark:bg-gray-800 animate-pulse" />
            ) : (agentsQ.data ?? []).length === 0 ? (
                <EmptyState title="No agents found" description="Try a different search term, or enroll endpoints first." />
            ) : (
                <div className="overflow-x-auto rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40">
                    <table className="min-w-full text-left text-sm">
                        <thead className="bg-gray-50 dark:bg-gray-800/80 text-gray-600 dark:text-gray-400 text-xs uppercase">
                            <tr>
                                <th className="px-3 py-2">Host</th>
                                <th className="px-3 py-2">Status</th>
                                <th className="px-3 py-2">OS</th>
                                <th className="px-3 py-2">Agent</th>
                                <th className="px-3 py-2">Last seen</th>
                                <th className="px-3 py-2">ID</th>
                            </tr>
                        </thead>
                        <tbody>
                            {(agentsQ.data ?? []).map((a) => {
                                const eff = getEffectiveStatus(a);
                                return (
                                    <tr key={a.id} className="border-t border-gray-100 dark:border-gray-800">
                                        <td className="px-3 py-2">
                                            <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to={`/management/devices/${encodeURIComponent(a.id)}`}>
                                                {a.hostname || '—'}
                                            </Link>
                                        </td>
                                        <td className="px-3 py-2 text-xs font-mono">{eff}</td>
                                        <td className="px-3 py-2 text-xs">
                                            {osLabel(a)} {a.os_version ? `· ${a.os_version}` : ''}
                                        </td>
                                        <td className="px-3 py-2 text-xs font-mono">{a.agent_version || '—'}</td>
                                        <td className="px-3 py-2 text-xs">{formatRelativeTime(a.last_seen)}</td>
                                        <td className="px-3 py-2 text-xs font-mono text-gray-500 dark:text-gray-400">
                                            {a.id}
                                        </td>
                                    </tr>
                                );
                            })}
                        </tbody>
                    </table>
                </div>
            )}

            <div className="rounded-xl border border-dashed border-gray-300 dark:border-gray-700 bg-white/50 dark:bg-gray-900/20 p-4 text-sm text-gray-600 dark:text-gray-400 flex items-center gap-2">
                <Shield className="w-4 h-4" />
                This page is for endpoint profiles. For user access management, use System → Access.
                <span className="ml-auto inline-flex items-center gap-1 text-xs text-gray-500">
                    <Server className="w-3.5 h-3.5" /> connection-manager
                </span>
            </div>
        </div>
    );
}

