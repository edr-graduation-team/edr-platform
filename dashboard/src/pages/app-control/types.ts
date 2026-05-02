// Application Control — shared types
// ────────────────────────────────────────────────────────────────────────────

/** Aggregated process row for the Process Analytics tab. */
export interface ProcessAggRow {
    name: string;
    executable: string;
    count: number;
    /** Category tag (system, scripting, admin, browser, service, user, unknown). */
    category: ProcessCategory;
    lastSeen: string;
    agents: Set<string>;
}

export type ProcessCategory =
    | 'system'
    | 'scripting'
    | 'admin'
    | 'remote_access'
    | 'browser'
    | 'service'
    | 'security'
    | 'user'
    | 'unknown';

/** Category metadata for display. */
export interface CategoryMeta {
    label: string;
    color: string;
    dot: string;
}

export const CATEGORY_META: Record<ProcessCategory, CategoryMeta> = {
    system:        { label: 'System',        color: 'bg-slate-500/10 text-slate-600 dark:text-slate-400 border-slate-500/25',   dot: 'bg-slate-400' },
    scripting:     { label: 'Scripting',     color: 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/25',   dot: 'bg-amber-500' },
    admin:         { label: 'Admin Tool',    color: 'bg-orange-500/10 text-orange-600 dark:text-orange-400 border-orange-500/25', dot: 'bg-orange-500' },
    remote_access: { label: 'Remote Access', color: 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border-rose-500/25',       dot: 'bg-rose-500' },
    browser:       { label: 'Browser',       color: 'bg-sky-500/10 text-sky-600 dark:text-sky-400 border-sky-500/25',           dot: 'bg-sky-500' },
    service:       { label: 'Service',       color: 'bg-indigo-500/10 text-indigo-600 dark:text-indigo-400 border-indigo-500/25', dot: 'bg-indigo-500' },
    security:      { label: 'Security',      color: 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/25', dot: 'bg-emerald-500' },
    user:          { label: 'User App',      color: 'bg-violet-500/10 text-violet-600 dark:text-violet-400 border-violet-500/25', dot: 'bg-violet-500' },
    unknown:       { label: 'Other',         color: 'bg-slate-500/10 text-slate-500 dark:text-slate-400 border-slate-500/25',   dot: 'bg-slate-400' },
};
