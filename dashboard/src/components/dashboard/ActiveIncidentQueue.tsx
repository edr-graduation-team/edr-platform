import React from 'react';
import { Target, ShieldAlert } from 'lucide-react';
import type { Alert } from '../../api/client';

interface ActiveIncidentQueueProps {
    alerts: Alert[];
    agentMap: Record<string, string>;
}

export const ActiveIncidentQueue = React.memo(function ActiveIncidentQueue({
    alerts,
    agentMap,
}: ActiveIncidentQueueProps) {
    const activeThreats = alerts
        .filter(
            (a) =>
                (a.severity === 'critical' || a.severity === 'high') &&
                (a.status === 'open' || a.status === 'in_progress')
        )
        .slice(0, 8);

    return (
        <div className="card border border-slate-200 dark:border-slate-700/60 shadow-lg bg-white dark:bg-slate-800/90 backdrop-blur-sm rounded-xl p-5 flex flex-col flex-1 min-h-0">
            <div className="flex items-center justify-between mb-4 border-b border-slate-200 dark:border-slate-700/60 pb-3 shrink-0">
                <div className="flex items-center gap-2">
                    <Target className="w-4 h-4 text-rose-500 dark:text-rose-400" />
                    <h3 className="text-sm font-bold text-slate-800 dark:text-slate-200 uppercase tracking-widest">
                        Incident Queue
                    </h3>
                </div>
                <span className="text-[10px] font-bold text-rose-600 dark:text-rose-400 uppercase tracking-widest bg-rose-100 dark:bg-rose-400/10 px-2 py-1 rounded-md border border-rose-200 dark:border-rose-400/20">
                    {activeThreats.length} Active
                </span>
            </div>

            <div className="space-y-2 flex-1 overflow-y-auto pr-1 min-h-[120px]">
                {activeThreats.length === 0 ? (
                    <div className="flex flex-col items-center justify-center p-6 text-slate-500 h-full">
                        <ShieldAlert className="w-6 h-6 opacity-30 mb-2" />
                        <span className="text-xs">All clear — no pending incidents</span>
                    </div>
                ) : (
                    activeThreats.map((alert) => (
                        <div
                            key={alert.id}
                            className="group bg-slate-50/50 dark:bg-slate-800/40 hover:bg-slate-100 dark:hover:bg-slate-700/50 border-l-2 border-rose-500 border border-slate-200 dark:border-slate-700/50 rounded-lg p-3 transition-colors cursor-pointer"
                        >
                            <div className="flex justify-between items-start mb-1 gap-2">
                                <span
                                    className="font-mono text-[11px] font-semibold text-rose-600 dark:text-rose-400 truncate flex-1"
                                    title={alert.rule_title}
                                >
                                    {alert.rule_title}
                                </span>
                                <span className="text-[10px] text-slate-500 shrink-0">
                                    {Math.floor(
                                        (Date.now() - new Date(alert.timestamp).getTime()) / 60000
                                    )}
                                    m ago
                                </span>
                            </div>
                            <div className="flex justify-between items-center mt-1.5">
                                <span className="text-[10px] font-mono text-slate-500 dark:text-slate-400 truncate">
                                    {agentMap[alert.agent_id] || alert.agent_id?.slice(0, 10)}
                                </span>
                                <span className="text-xs font-medium text-cyan-600 dark:text-cyan-400 opacity-0 group-hover:opacity-100 transition-opacity">
                                    Triage →
                                </span>
                            </div>
                        </div>
                    ))
                )}
            </div>
        </div>
    );
});
