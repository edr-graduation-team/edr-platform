import React from 'react';
import { Zap, CheckCircle, Info } from 'lucide-react';

interface UEBASignalBadgeProps {
    signal: string;
}

export const UEBASignalBadge = React.memo(function UEBASignalBadge({ signal }: UEBASignalBadgeProps) {
    if (signal === 'anomaly') {
        return (
            <span className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs font-semibold bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-300 ring-1 ring-red-300 dark:ring-red-700">
                <Zap className="w-3 h-3" />
                Baseline Anomaly
            </span>
        );
    }
    if (signal === 'normal') {
        return (
            <span className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs font-semibold bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300 ring-1 ring-green-300 dark:ring-green-700">
                <CheckCircle className="w-3 h-3" />
                Normalcy Discount
            </span>
        );
    }
    return (
        <span className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs font-semibold bg-slate-100 text-slate-600 dark:bg-slate-700 dark:text-slate-400">
            <Info className="w-3 h-3" />
            No UEBA Signal
        </span>
    );
});

export default UEBASignalBadge;
