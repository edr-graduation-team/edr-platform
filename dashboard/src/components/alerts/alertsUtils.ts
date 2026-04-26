import { severityColors, statusColors, severityStripe, statusIcons } from './alertsConstants';

// Safe stringify for any field value
export const json_safe = (val: unknown): string => {
    if (val === null || val === undefined) return '—';
    if (typeof val === 'string') return val;
    return JSON.stringify(val);
};

// Risk score style helper
export function getRiskScoreStyle(score: number, riskLevel?: string): { bg: string; text: string; label: string; ring: string; shadow: string } {
    const lvl = (riskLevel || '').toLowerCase();
    if (lvl === 'critical') return { bg: 'bg-rose-500/10 dark:bg-rose-500/20', text: 'text-rose-600 dark:text-rose-400', label: 'CRITICAL', ring: 'border border-rose-500/30', shadow: 'shadow-[0_0_10px_rgba(244,63,94,0.2)]' };
    if (lvl === 'high') return { bg: 'bg-orange-500/10 dark:bg-orange-500/20', text: 'text-orange-600 dark:text-orange-400', label: 'HIGH', ring: 'border border-orange-500/30', shadow: 'shadow-[0_0_10px_rgba(249,115,22,0.2)]' };
    if (lvl === 'medium') return { bg: 'bg-amber-500/10 dark:bg-amber-500/20', text: 'text-amber-600 dark:text-amber-400', label: 'MEDIUM', ring: 'border border-amber-500/30', shadow: 'shadow-[0_0_10px_rgba(245,158,11,0.2)]' };
    if (lvl === 'low') return { bg: 'bg-emerald-500/10 dark:bg-emerald-500/20', text: 'text-emerald-600 dark:text-emerald-400', label: 'LOW', ring: 'border border-emerald-500/30', shadow: 'shadow-[0_0_10px_rgba(16,185,129,0.2)]' };

    // Fallback (older backend responses): derive level from score thresholds.
    if (score >= 90) return { bg: 'bg-rose-500/10 dark:bg-rose-500/20', text: 'text-rose-600 dark:text-rose-400', label: 'CRITICAL', ring: 'border border-rose-500/30', shadow: 'shadow-[0_0_10px_rgba(244,63,94,0.2)]' };
    if (score >= 70) return { bg: 'bg-orange-500/10 dark:bg-orange-500/20', text: 'text-orange-600 dark:text-orange-400', label: 'HIGH', ring: 'border border-orange-500/30', shadow: 'shadow-[0_0_10px_rgba(249,115,22,0.2)]' };
    if (score >= 40) return { bg: 'bg-amber-500/10 dark:bg-amber-500/20', text: 'text-amber-600 dark:text-amber-400', label: 'MEDIUM', ring: 'border border-amber-500/30', shadow: 'shadow-[0_0_10px_rgba(245,158,11,0.2)]' };
    return { bg: 'bg-emerald-500/10 dark:bg-emerald-500/20', text: 'text-emerald-600 dark:text-emerald-400', label: 'LOW', ring: 'border border-emerald-500/30', shadow: 'shadow-[0_0_10px_rgba(16,185,129,0.2)]' };
}

// Re-export constants for convenience
export { severityColors, statusColors, severityStripe, statusIcons };
