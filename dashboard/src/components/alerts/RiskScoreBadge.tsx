import React from 'react';
import { getRiskScoreStyle } from './alertsUtils';

interface RiskScoreBadgeProps {
    score?: number | null;
    riskLevel?: string;
}

export const RiskScoreBadge = React.memo(function RiskScoreBadge({ score, riskLevel }: RiskScoreBadgeProps) {
    if (score === undefined || score === null) {
        return <span className="text-xs text-slate-400 font-mono">—</span>;
    }
    const style = getRiskScoreStyle(score, riskLevel);
    return (
        <div className="flex items-center gap-1.5">
            <span
                className={`inline-flex items-center justify-center w-9 h-9 rounded-full text-sm font-bold ${style.bg} ${style.text} ${style.ring} ${style.shadow}`}
                title={`Risk Score: ${score}/100 (${style.label})`}
            >
                {score}
            </span>
        </div>
    );
});

export default RiskScoreBadge;
