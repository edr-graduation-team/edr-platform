import { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Database, RefreshCw, Send, ShieldCheck, Workflow } from 'lucide-react';
import { authApi, signaturesApi } from '../../api/client';
import { useToast } from '../../components/Toast';
import InsightHero from '../../components/InsightHero';

export default function SignaturesManagement() {
    const queryClient = useQueryClient();
    const { showToast } = useToast();
    const canWrite = authApi.canWriteSettings();
    const [includeOffline, setIncludeOffline] = useState(true);

    useEffect(() => {
        document.title = 'Signatures — System | EDR Platform';
    }, []);

    const statsQuery = useQuery({
        queryKey: ['signatures', 'stats'],
        queryFn: signaturesApi.stats,
        refetchInterval: 10000,
    });

    const listQuery = useQuery({
        queryKey: ['signatures', 'latest-list'],
        queryFn: () => signaturesApi.list({ limit: 25 }),
        refetchInterval: 15000,
    });

    const syncMutation = useMutation({
        mutationFn: signaturesApi.syncNow,
        onSuccess: () => {
            showToast('Signature feed sync queued on server', 'success');
            queryClient.invalidateQueries({ queryKey: ['signatures'] });
        },
        onError: (err: any) => showToast(err?.message || 'Failed to trigger sync', 'error'),
    });

    const pushAllMutation = useMutation({
        mutationFn: () => signaturesApi.pushUpdateAll({ include_offline: includeOffline }),
        onSuccess: (res) => {
            const d = res.data;
            showToast(`Queued for fleet: sent=${d.sent}, queued=${d.queued}, failed=${d.failed}`, 'success');
            queryClient.invalidateQueries({ queryKey: ['signatures'] });
        },
        onError: (err: any) => showToast(err?.message || 'Failed to push update', 'error'),
    });

    const sourceRows = useMemo(() => {
        const src = statsQuery.data?.sources || {};
        return Object.entries(src).sort((a, b) => b[1] - a[1]);
    }, [statsQuery.data]);

    return (
        <div className="space-y-6 md:space-y-8">
            <InsightHero
                accent="emerald"
                icon={ShieldCheck}
                eyebrow="Server-managed signatures"
                title="Central malware hash feed"
                segments={[
                    {
                        heading: 'Single source of truth',
                        children: <>The connection-manager syncs and versions signatures, then all agents pull deltas from server endpoints.</>,
                    },
                    {
                        heading: 'Fleet-wide push',
                        children: <>Use one action to queue `update_signatures` to the whole fleet instead of per-device updates.</>,
                    },
                ]}
            />

            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                <div className="rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 p-4">
                    <div className="text-xs uppercase font-semibold text-slate-500 flex items-center gap-2"><Database className="w-4 h-4" /> Total hashes</div>
                    <div className="mt-2 text-2xl font-bold">{statsQuery.data?.count ?? '—'}</div>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 p-4">
                    <div className="text-xs uppercase font-semibold text-slate-500 flex items-center gap-2"><Workflow className="w-4 h-4" /> Max version</div>
                    <div className="mt-2 text-2xl font-bold">{statsQuery.data?.max_version ?? '—'}</div>
                </div>
                <div className="rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 p-4">
                    <div className="text-xs uppercase font-semibold text-slate-500 flex items-center gap-2"><RefreshCw className="w-4 h-4" /> Sources</div>
                    <div className="mt-2 text-sm space-y-1">
                        {sourceRows.length === 0 ? <div className="text-slate-500">No source data</div> : sourceRows.map(([k, v]) => (
                            <div key={k} className="flex justify-between"><span>{k}</span><span className="font-mono">{v}</span></div>
                        ))}
                    </div>
                </div>
            </div>

            <div className="rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 p-5 space-y-4">
                <h3 className="font-semibold text-slate-900 dark:text-slate-100">Actions</h3>
                <div className="flex flex-wrap gap-3 items-center">
                    <button
                        type="button"
                        disabled={!canWrite || syncMutation.isPending}
                        onClick={() => syncMutation.mutate()}
                        className="px-3 py-2 rounded-lg text-sm font-semibold bg-emerald-600 hover:bg-emerald-700 text-white disabled:opacity-40"
                    >
                        {syncMutation.isPending ? 'Queuing…' : 'Sync signatures now'}
                    </button>
                    <label className="inline-flex items-center gap-2 text-sm">
                        <input type="checkbox" checked={includeOffline} onChange={(e) => setIncludeOffline(e.target.checked)} />
                        Include offline agents (queue for reconnect)
                    </label>
                    <button
                        type="button"
                        disabled={!canWrite || pushAllMutation.isPending}
                        onClick={() => pushAllMutation.mutate()}
                        className="px-3 py-2 rounded-lg text-sm font-semibold bg-cyan-600 hover:bg-cyan-700 text-white disabled:opacity-40 inline-flex items-center gap-2"
                    >
                        <Send className="w-4 h-4" />
                        {pushAllMutation.isPending ? 'Dispatching…' : 'Push update to all agents'}
                    </button>
                </div>
                {!canWrite && (
                    <p className="text-xs text-amber-600 dark:text-amber-400">Read-only role: you can view stats, but only admins can trigger sync or fleet dispatch.</p>
                )}
            </div>

            <div className="rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 p-5">
                <h3 className="font-semibold text-slate-900 dark:text-slate-100 mb-3">Recent signature entries</h3>
                {listQuery.isLoading ? (
                    <p className="text-sm text-slate-500">Loading…</p>
                ) : listQuery.isError ? (
                    <p className="text-sm text-rose-600 dark:text-rose-400">Failed to load signatures list.</p>
                ) : (
                    <div className="space-y-2">
                        {(listQuery.data?.data || []).map((row) => (
                            <div key={`${row.version}-${row.sha256}`} className="rounded-lg border border-slate-200 dark:border-slate-700 px-3 py-2">
                                <div className="text-xs text-slate-500">v{row.version} · {row.source || 'unknown source'}</div>
                                <div className="font-mono text-xs break-all">{row.sha256}</div>
                                {(row.family || row.name) && <div className="text-xs mt-1">{row.family || row.name}</div>}
                            </div>
                        ))}
                    </div>
                )}
            </div>
        </div>
    );
}
