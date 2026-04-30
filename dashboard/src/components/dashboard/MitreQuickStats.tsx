import { useMemo } from 'react';
import type { Alert } from '../../api/client';

interface MitreQuickStatsProps {
    alerts: Alert[];
    byTactic?: Record<string, number>;
}

export function MitreQuickStats({ alerts, byTactic }: MitreQuickStatsProps) {
    const tacticCounts = useMemo(() => {
        // Prefer server-side byTactic if available; otherwise compute from liveAlerts
        if (byTactic && Object.keys(byTactic).length > 0) return byTactic;
        const counts: Record<string, number> = {};
        alerts.forEach((a) => {
            a.mitre_tactics?.forEach((t) => {
                counts[t] = (counts[t] || 0) + 1;
            });
        });
        return counts;
    }, [alerts, byTactic]);

    const top5 = useMemo(
        () =>
            Object.entries(tacticCounts)
                .sort(([, a], [, b]) => b - a)
                .slice(0, 5),
        [tacticCounts]
    );

    if (top5.length === 0) {
        return (
            <div className="flex items-center justify-center h-20 text-slate-500 text-xs">
                No MITRE data available
            </div>
        );
    }

    const maxVal = Math.max(...top5.map(([, v]) => v), 1);

    return (
        <div className="space-y-2.5">
            {top5.map(([tactic, count]) => (
                <div key={tactic} className="flex items-center gap-3">
                    <span
                        className="text-[11px] font-semibold text-slate-500 dark:text-slate-400 w-28 shrink-0 truncate"
                        title={tactic}
                    >
                        {tactic}
                    </span>
                    <div className="flex-1 h-2 bg-slate-200 dark:bg-slate-700 rounded-full overflow-hidden">
                        <div
                            className="h-full rounded-full transition-all duration-700"
                            style={{
                                width: `${(count / maxVal) * 100}%`,
                                background: 'linear-gradient(to right, #a855f7, #22d3ee)',
                            }}
                        />
                    </div>
                    <span className="text-[11px] font-bold text-slate-600 dark:text-slate-300 font-mono w-6 text-right shrink-0">
                        {count}
                    </span>
                </div>
            ))}
        </div>
    );
}
