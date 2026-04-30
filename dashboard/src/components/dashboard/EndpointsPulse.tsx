import React from 'react';
import { Monitor, Wifi, WifiOff, AlertTriangle } from 'lucide-react';
import type { AgentStats, Agent } from '../../api/client';
import { STALE_THRESHOLD_MS } from '../../hooks/useDashboard';

interface EndpointsPulseProps {
    stats: AgentStats | null;
    agents: Agent[];
    onClick: () => void;
}

export const EndpointsPulse = React.memo(function EndpointsPulse({
    stats,
    agents,
    onClick,
}: EndpointsPulseProps) {
    if (!stats) return null;

    let online = 0,
        offline = 0,
        degraded = 0;
    if (agents.length > 0) {
        const now = Date.now();
        agents.forEach((a) => {
            const isStale = now - new Date(a.last_seen).getTime() > STALE_THRESHOLD_MS;
            if (a.status === 'online' && !isStale) online++;
            else if (a.status === 'degraded' && !isStale) degraded++;
            else offline++;
        });
    } else {
        online = stats.online;
        offline = stats.offline;
        degraded = stats.degraded;
    }
    const total = online + offline + degraded;

    return (
        <div
            className="card border border-slate-200 dark:border-slate-700/60 shadow-lg bg-white dark:bg-slate-800/90 backdrop-blur-sm rounded-xl p-5 flex flex-col cursor-pointer transition-all hover:-translate-y-0.5 hover:shadow-xl shrink-0"
            onClick={onClick}
        >
            <div className="flex items-center justify-between mb-4 border-b border-slate-200 dark:border-slate-700/60 pb-3 shrink-0">
                <div className="flex items-center gap-2">
                    <Monitor className="w-4 h-4 text-cyan-500 dark:text-cyan-400" />
                    <h3 className="text-sm font-bold text-slate-800 dark:text-slate-200 uppercase tracking-widest">
                        Agent Pulse
                    </h3>
                </div>
            </div>

            <div className="flex justify-around items-end pb-2">
                <div className="flex flex-col items-center gap-1">
                    <span className="text-2xl font-bold text-emerald-400 font-mono">{online}</span>
                    <span className="text-[10px] font-bold uppercase tracking-widest text-slate-500 flex items-center gap-1">
                        <Wifi className="w-3 h-3" />
                        Online
                    </span>
                </div>
                <div className="flex flex-col items-center gap-1">
                    <span className="text-2xl font-bold text-amber-400 font-mono">{degraded}</span>
                    <span className="text-[10px] font-bold uppercase tracking-widest text-slate-500 flex items-center gap-1">
                        <AlertTriangle className="w-3 h-3" />
                        Warn
                    </span>
                </div>
                <div className="flex flex-col items-center gap-1">
                    <span className="text-2xl font-bold text-slate-500 font-mono">{offline}</span>
                    <span className="text-[10px] font-bold uppercase tracking-widest text-slate-500 flex items-center gap-1">
                        <WifiOff className="w-3 h-3" />
                        Offline
                    </span>
                </div>
            </div>

            <div className="mt-4 pt-4 border-t border-slate-200 dark:border-slate-800 flex flex-col gap-1.5">
                <div className="flex gap-0.5 h-1.5 rounded-full overflow-hidden bg-slate-200 dark:bg-slate-800">
                    <div
                        className="bg-emerald-500 transition-all duration-700"
                        style={{ width: `${total > 0 ? (online / total) * 100 : 0}%` }}
                    />
                    <div
                        className="bg-amber-500 transition-all duration-700"
                        style={{ width: `${total > 0 ? (degraded / total) * 100 : 0}%` }}
                    />
                    <div
                        className="bg-slate-400 dark:bg-slate-600 transition-all duration-700"
                        style={{ width: `${total > 0 ? (offline / total) * 100 : 0}%` }}
                    />
                </div>
                <div className="text-[10px] text-slate-500 dark:text-slate-400 font-mono text-right">
                    {total} TOTAL AGENTS
                </div>
            </div>
        </div>
    );
});
