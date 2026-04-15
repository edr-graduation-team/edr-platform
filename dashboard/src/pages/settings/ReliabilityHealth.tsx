import { useQuery } from '@tanstack/react-query';
import { Activity, AlertTriangle, Database, HardDrive, RefreshCw } from 'lucide-react';
import { reliabilityApi } from '../../api/client';
import axios from 'axios';

function MetricCard({
    title,
    value,
    subtitle,
    tone = 'neutral',
    icon: Icon,
}: {
    title: string;
    value: string | number;
    subtitle?: string;
    tone?: 'neutral' | 'good' | 'warn' | 'bad';
    icon: React.ElementType;
}) {
    const toneStyles: Record<string, string> = {
        neutral: 'border-gray-200 dark:border-gray-700',
        good: 'border-emerald-200 dark:border-emerald-500/20',
        warn: 'border-amber-200 dark:border-amber-500/20',
        bad: 'border-red-200 dark:border-red-500/20',
    };

    const iconBg: Record<string, string> = {
        neutral: 'bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300',
        good: 'bg-emerald-50 dark:bg-emerald-500/10 text-emerald-700 dark:text-emerald-400',
        warn: 'bg-amber-50 dark:bg-amber-500/10 text-amber-700 dark:text-amber-400',
        bad: 'bg-red-50 dark:bg-red-500/10 text-red-700 dark:text-red-400',
    };

    return (
        <div className={`bg-white dark:bg-gray-800 border ${toneStyles[tone]} rounded-xl p-5 shadow-sm`}>
            <div className="flex items-start justify-between gap-3">
                <div>
                    <p className="text-[11px] font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        {title}
                    </p>
                    <p className="mt-2 text-3xl font-bold text-gray-900 dark:text-white font-mono">
                        {value}
                    </p>
                    {subtitle && (
                        <p className="mt-2 text-sm text-gray-500 dark:text-gray-400">
                            {subtitle}
                        </p>
                    )}
                </div>
                <div className={`w-10 h-10 rounded-lg flex items-center justify-center shrink-0 ${iconBg[tone]}`}>
                    <Icon size={18} />
                </div>
            </div>
        </div>
    );
}

export default function ReliabilityHealth() {
    const { data, isLoading, isFetching, refetch, error, dataUpdatedAt } = useQuery({
        queryKey: ['reliabilityHealth'],
        queryFn: reliabilityApi.health,
        refetchInterval: 5000,
    });
    const errorHint = (() => {
        if (!error || !axios.isAxiosError(error)) return null;
        if (error.response?.status === 403) {
            return 'Access denied for reliability endpoint (403).';
        }
        if (error.response?.status === 404) {
            return 'Reliability endpoint not routed (404). Check dashboard proxy.';
        }
        if (error.code === 'ECONNABORTED') {
            return 'Reliability request timed out.';
        }
        return null;
    })();

    const fb = data?.fallback_store;
    const hasFB = !!fb?.enabled && !!fb?.stats && typeof fb.stats.channel_cap === 'number';
    const stats = fb?.stats;

    const channelUsagePct =
        hasFB && stats!.channel_cap > 0 ? Math.round((stats!.channel_len / stats!.channel_cap) * 100) : 0;

    const drops = hasFB ? stats!.sync_write_failed_drop : 0;
    const channelFull = hasFB ? stats!.channel_full : 0;
    const dbWriteFailed = hasFB ? stats!.db_write_failed : 0;

    const headlineTone: 'good' | 'warn' | 'bad' | 'neutral' =
        !hasFB ? 'neutral' : drops > 0 ? 'bad' : channelFull > 0 || dbWriteFailed > 0 ? 'warn' : 'good';

    return (
        <div className="space-y-5">
            <div className="flex items-start justify-between gap-4">
                <div>
                    <h2 className="text-xl font-bold text-gray-900 dark:text-white flex items-center gap-2">
                        <Activity className="w-5 h-5 text-cyan-500" />
                        Reliability Health
                    </h2>
                    <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                        Operational indicators for ingestion durability (backpressure, fallback usage, and drops).
                    </p>
                </div>

                <button
                    onClick={() => refetch()}
                    className="flex items-center gap-2 px-3.5 py-2 text-sm rounded-lg font-medium transition-colors bg-gray-900 text-white hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-100"
                >
                    <RefreshCw size={14} className={isFetching ? 'animate-spin' : ''} />
                    Refresh
                </button>
            </div>

            <div className={`rounded-xl border p-4 text-sm ${
                headlineTone === 'good'
                    ? 'bg-emerald-50 border-emerald-200 text-emerald-800 dark:bg-emerald-500/10 dark:border-emerald-500/20 dark:text-emerald-300'
                    : headlineTone === 'bad'
                        ? 'bg-red-50 border-red-200 text-red-800 dark:bg-red-500/10 dark:border-red-500/20 dark:text-red-300'
                        : headlineTone === 'warn'
                            ? 'bg-amber-50 border-amber-200 text-amber-800 dark:bg-amber-500/10 dark:border-amber-500/20 dark:text-amber-300'
                            : 'bg-slate-100 border-slate-300 text-slate-700 dark:bg-slate-800/60 dark:border-slate-700 dark:text-slate-300'
            }`}>
                <div className="flex items-start gap-3">
                    <AlertTriangle className="w-5 h-5 shrink-0 mt-0.5" />
                    <div>
                        <div className="font-semibold">
                            {headlineTone === 'good'
                                ? 'Healthy'
                                : headlineTone === 'bad'
                                    ? 'Data loss risk detected'
                                    : headlineTone === 'warn'
                                        ? 'Degraded — fallback pressure observed'
                                        : 'Telemetry unavailable'}
                        </div>
                        <div className="mt-1 opacity-90">
                            {hasFB
                                ? `Fallback channel ${stats!.channel_len}/${stats!.channel_cap} (${channelUsagePct}%). Drops=${stats!.sync_write_failed_drop}, ChannelFull=${stats!.channel_full}, DBWriteFailed=${stats!.db_write_failed}.`
                                : (fb?.reason || 'Fallback store stats not available.')}
                        </div>
                    </div>
                </div>
            </div>

            {isLoading ? (
                <div className="flex items-center justify-center py-16 text-sm text-gray-500 dark:text-gray-400">
                    <div className="w-10 h-10 rounded-full border-2 border-cyan-500/20 border-t-cyan-400 animate-spin mr-3"></div>
                    Loading reliability health…
                </div>
            ) : error ? (
                <div className="rounded-xl border border-red-200 bg-red-50 text-red-800 dark:bg-red-500/10 dark:border-red-500/20 dark:text-red-300 p-4 text-sm">
                    Failed to load reliability health. Ensure your session is valid and the backend is reachable.
                    {errorHint ? <div className="mt-2 opacity-90">{errorHint}</div> : null}
                </div>
            ) : (
                <>
                    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
                        <MetricCard
                            title="Fallback Queue Usage"
                            value={hasFB ? `${stats!.channel_len}/${stats!.channel_cap}` : '—'}
                            subtitle={hasFB ? `${channelUsagePct}% used` : 'DB fallback disabled / unavailable'}
                            tone={hasFB && channelUsagePct >= 90 ? 'bad' : hasFB && channelUsagePct >= 60 ? 'warn' : 'neutral'}
                            icon={HardDrive}
                        />
                        <MetricCard
                            title="Channel Full Events"
                            value={hasFB ? stats!.channel_full : '—'}
                            subtitle="Async queue saturated (sync write attempted)"
                            tone={hasFB && stats!.channel_full > 0 ? 'warn' : 'neutral'}
                            icon={Activity}
                        />
                        <MetricCard
                            title="Sync Fallback Used"
                            value={hasFB ? stats!.sync_write_used : '—'}
                            subtitle="Batches persisted synchronously (bounded timeout)"
                            tone={hasFB && stats!.sync_write_used > 0 ? 'warn' : 'neutral'}
                            icon={Database}
                        />
                        <MetricCard
                            title="Drops (Sync Write Failed)"
                            value={hasFB ? stats!.sync_write_failed_drop : '—'}
                            subtitle="Definitive data loss in fallback path"
                            tone={hasFB && stats!.sync_write_failed_drop > 0 ? 'bad' : 'neutral'}
                            icon={AlertTriangle}
                        />
                    </div>

                    <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                        <MetricCard
                            title="Async Enqueued"
                            value={hasFB ? stats!.enqueued_async : '—'}
                            subtitle="Batches accepted into async fallback writer"
                            tone="neutral"
                            icon={HardDrive}
                        />
                        <MetricCard
                            title="DB Write Failed"
                            value={hasFB ? stats!.db_write_failed : '—'}
                            subtitle="INSERT failures (async or sync)"
                            tone={hasFB && stats!.db_write_failed > 0 ? 'warn' : 'neutral'}
                            icon={Database}
                        />
                        <MetricCard
                            title="Metadata Marshal Failed"
                            value={hasFB ? stats!.marshal_failed : '—'}
                            subtitle="Payload metadata serialization failures"
                            tone={hasFB && stats!.marshal_failed > 0 ? 'warn' : 'neutral'}
                            icon={Activity}
                        />
                    </div>
                </>
            )}

            <div className="text-[12px] text-gray-500 dark:text-gray-400">
                Last update: {dataUpdatedAt ? new Date(dataUpdatedAt).toLocaleTimeString() : '—'} · Counters are cumulative since service start.
            </div>
            <div className="text-[12px] text-gray-500 dark:text-gray-400">
                Tip: spikes in <span className="font-semibold">Channel Full</span> usually indicate Kafka outages or ingestion bursts. Any non-zero{' '}
                <span className="font-semibold">Drops</span> should be investigated immediately.
            </div>
        </div>
    );
}

