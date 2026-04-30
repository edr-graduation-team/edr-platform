import React, { useMemo } from 'react';
import { Terminal } from 'lucide-react';
import type { Alert, Agent } from '../../api/client';

interface SystemActionLogProps {
    alerts: Alert[];
    agents: Agent[];
}

export const SystemActionLog = React.memo(function SystemActionLog({
    alerts,
    agents,
}: SystemActionLogProps) {
    const logEntries = useMemo(
        () =>
            [
                ...alerts.map((a) => ({
                    id: `a-${a.id}`,
                    timestamp: new Date(a.timestamp).getTime(),
                    timeStr: new Date(a.timestamp).toLocaleTimeString('en-US', { hour12: false }),
                    type: 'THREAT',
                    message: `[DETECT] ${a.severity.toUpperCase()} on ${a.agent_id?.slice(0, 8)}: ${a.rule_title}`,
                    color:
                        a.severity === 'critical'
                            ? 'text-rose-500 dark:text-rose-400'
                            : 'text-amber-500 dark:text-amber-400',
                })),
                ...agents.map((ag) => ({
                    id: `ag-${ag.id}`,
                    timestamp: new Date(ag.last_seen).getTime(),
                    timeStr: new Date(ag.last_seen).toLocaleTimeString('en-US', { hour12: false }),
                    type: 'SYSTEM',
                    message: `[CHECK-IN] Agent ${ag.hostname} reported status: ${ag.status}`,
                    color: 'text-cyan-500 dark:text-cyan-400',
                })),
            ]
                .sort((a, b) => b.timestamp - a.timestamp)
                .slice(0, 40),
        [alerts, agents]
    );

    return (
        <div className="card border border-slate-200 dark:border-slate-700/60 shadow-lg bg-white dark:bg-slate-800/90 backdrop-blur-sm rounded-xl overflow-hidden flex flex-col h-[260px] font-mono w-full">
            <div className="bg-slate-50 dark:bg-slate-950/50 px-4 py-3 border-b border-slate-200 dark:border-slate-700/60 flex items-center justify-between shrink-0">
                <div className="flex items-center gap-2">
                    <Terminal className="w-4 h-4 text-slate-500 dark:text-slate-400" />
                    <span className="text-[11px] text-slate-600 dark:text-slate-300 font-bold tracking-widest uppercase">
                        System Action Log
                    </span>
                </div>
                <div className="flex gap-1.5">
                    <div className="w-2.5 h-2.5 rounded-full bg-rose-400" />
                    <div className="w-2.5 h-2.5 rounded-full bg-amber-400" />
                    <div className="w-2.5 h-2.5 rounded-full bg-emerald-400" />
                </div>
            </div>
            <div className="p-4 space-y-1 overflow-y-auto text-[11px] leading-relaxed flex-1">
                {logEntries.length === 0 ? (
                    <div className="text-slate-600 italic">Awaiting terminal stream...</div>
                ) : (
                    logEntries.map((entry) => (
                        <div
                            key={entry.id}
                            className="flex gap-3 hover:bg-slate-100 dark:hover:bg-slate-800/80 px-2 py-0.5 -mx-2 rounded transition-colors"
                        >
                            <span className="text-slate-500 shrink-0">{entry.timeStr}</span>
                            <span className={`${entry.color} truncate`} title={entry.message}>
                                {entry.message}
                            </span>
                        </div>
                    ))
                )}
            </div>
        </div>
    );
});
