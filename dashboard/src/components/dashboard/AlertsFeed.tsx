import React from 'react';
import { Activity, Clock, ChevronRight, Zap } from 'lucide-react';
import type { Alert } from '../../api/client';
import LiveIndicator from '../LiveIndicator';

interface AlertsFeedProps {
    alerts: Alert[];
    agentMap: Record<string, string>;
    onAlertClick: (alert: Alert) => void;
}

export const AlertsFeed = React.memo(function AlertsFeed({
    alerts,
    agentMap,
    onAlertClick,
}: AlertsFeedProps) {
    const severityConfig: Record<string, { color: string; border: string; bg: string; stripe: string }> = {
        critical: {
            color: 'text-rose-400',
            border: 'border-rose-500/50',
            bg: 'bg-rose-500/10',
            stripe: 'border-l-4 border-l-rose-500',
        },
        high: {
            color: 'text-orange-400',
            border: 'border-orange-500/50',
            bg: 'bg-orange-500/10',
            stripe: 'border-l-4 border-l-orange-500',
        },
        medium: {
            color: 'text-amber-400',
            border: 'border-amber-500/50',
            bg: 'bg-amber-500/10',
            stripe: 'border-l-4 border-l-amber-500',
        },
        low: {
            color: 'text-indigo-400',
            border: 'border-indigo-500/50',
            bg: 'bg-indigo-500/10',
            stripe: 'border-l-4 border-l-indigo-500',
        },
        informational: {
            color: 'text-cyan-400',
            border: 'border-cyan-500/50',
            bg: 'bg-cyan-500/10',
            stripe: 'border-l-4 border-l-cyan-500',
        },
    };

    const formatRel = (ts: string) => {
        const diff = Date.now() - new Date(ts).getTime();
        const mins = Math.floor(diff / 60000);
        if (mins < 1) return 'Just now';
        if (mins < 60) return `${mins}m ago`;
        const hrs = Math.floor(mins / 60);
        if (hrs < 24) return `${hrs}h ago`;
        return `${Math.floor(hrs / 24)}d ago`;
    };

    return (
        <div className="card border border-slate-200 dark:border-slate-700/60 shadow-lg bg-white dark:bg-slate-800/90 backdrop-blur-sm rounded-xl p-5 flex flex-col h-full min-h-[450px]">
            <div className="flex items-center justify-between mb-5 border-b border-slate-200 dark:border-slate-700/60 pb-4 shrink-0">
                <h3 className="text-sm font-bold text-slate-800 dark:text-slate-200 uppercase tracking-widest flex items-center gap-2">
                    <Activity className="w-4 h-4 text-cyan-500 dark:text-cyan-400" /> Live Alerts Stream
                </h3>
                <LiveIndicator label="Live" color="emerald" />
            </div>

            <div className="space-y-2 flex-1 overflow-y-auto pr-1">
                {alerts.length === 0 ? (
                    <div className="flex flex-col items-center justify-center h-full text-slate-500 min-h-[200px]">
                        <Zap className="w-6 h-6 opacity-40 mb-3" />
                        <span className="text-sm">Listening for signals...</span>
                    </div>
                ) : (
                    alerts.slice(0, 50).map((alert) => {
                        const style = severityConfig[alert.severity] || severityConfig.informational;
                        const hostname = agentMap[alert.agent_id] || alert.agent_id?.slice(0, 8);
                        return (
                            <div
                                key={alert.id}
                                onClick={() => onAlertClick(alert)}
                                className={`bg-slate-50/50 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-700/40 ${style.stripe} rounded-lg p-3.5 hover:bg-slate-100 dark:hover:bg-slate-700/60 transition-all cursor-pointer flex items-start gap-3 group`}
                            >
                                <div
                                    className={`mt-1 flex-shrink-0 w-2 h-2 rounded-full ${style.bg} ${style.border} border shadow-[0_0_6px_currentColor] ${style.color}`}
                                />
                                <div className="flex-1 min-w-0">
                                    <div className="flex justify-between items-start mb-1 gap-2">
                                        <p className="font-semibold text-slate-800 dark:text-slate-200 truncate text-sm group-hover:text-cyan-700 dark:group-hover:text-cyan-400 transition-colors">
                                            {alert.rule_title}
                                        </p>
                                        <span
                                            className={`text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border ${style.bg} ${style.color} ${style.border} shrink-0`}
                                        >
                                            {alert.severity}
                                        </span>
                                    </div>
                                    <div className="flex items-center gap-3 mt-1.5">
                                        <span className="font-mono text-[10px] text-slate-500 dark:text-slate-400 bg-slate-100 dark:bg-slate-900/50 px-1.5 py-0.5 rounded truncate max-w-[120px]">
                                            {hostname}
                                        </span>
                                        {alert.mitre_tactics?.[0] && (
                                            <span className="text-[10px] text-purple-500 dark:text-purple-400 font-medium truncate max-w-[100px]">
                                                {alert.mitre_tactics[0]}
                                            </span>
                                        )}
                                        <div className="text-[10px] text-slate-500 flex items-center gap-1 shrink-0 ml-auto">
                                            <Clock className="w-3 h-3" /> {formatRel(alert.timestamp)}
                                        </div>
                                    </div>
                                </div>
                                <ChevronRight className="w-4 h-4 text-slate-400 opacity-0 group-hover:opacity-100 transition-opacity shrink-0" />
                            </div>
                        );
                    })
                )}
            </div>
        </div>
    );
});
