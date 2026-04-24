import type { LucideIcon } from 'lucide-react';
import type { ReactNode } from 'react';

export type InsightHeroSegment = {
    /** Short label above copy */
    heading: string;
    children: ReactNode;
};

type Accent = 'cyan' | 'violet' | 'indigo' | 'fuchsia' | 'teal' | 'emerald' | 'amber' | 'rose' | 'sky';

const LIGHT_SURFACE: Record<Accent, string> = {
    cyan: 'from-slate-50 via-white to-cyan-50/50 dark:from-slate-900/85 dark:via-slate-900/60 dark:to-cyan-950/25',
    violet: 'from-slate-50 via-white to-violet-50/45 dark:from-slate-900/85 dark:via-slate-900/60 dark:to-violet-950/25',
    indigo: 'from-slate-50 via-white to-indigo-50/40 dark:from-slate-900/85 dark:via-slate-900/60 dark:to-indigo-950/25',
    fuchsia: 'from-slate-50 via-white to-fuchsia-50/35 dark:from-slate-900/85 dark:via-slate-900/60 dark:to-fuchsia-950/20',
    teal: 'from-slate-50 via-white to-teal-50/40 dark:from-slate-900/85 dark:via-slate-900/60 dark:to-teal-950/25',
    emerald: 'from-slate-50 via-white to-emerald-50/40 dark:from-slate-900/85 dark:via-slate-900/60 dark:to-emerald-950/25',
    amber: 'from-slate-50 via-white to-amber-50/35 dark:from-slate-900/85 dark:via-slate-900/60 dark:to-amber-950/20',
    rose: 'from-slate-50 via-white to-rose-50/35 dark:from-slate-900/85 dark:via-slate-900/60 dark:to-rose-950/20',
    sky: 'from-slate-50 via-white to-sky-50/45 dark:from-slate-900/85 dark:via-slate-900/60 dark:to-sky-950/25',
};

const ICON_WRAP: Record<Accent, string> = {
    cyan: 'border-cyan-500/30 bg-cyan-500/10 text-cyan-700 dark:text-cyan-300',
    violet: 'border-violet-500/30 bg-violet-500/10 text-violet-700 dark:text-violet-300',
    indigo: 'border-indigo-500/30 bg-indigo-500/10 text-indigo-700 dark:text-indigo-300',
    fuchsia: 'border-fuchsia-500/30 bg-fuchsia-500/10 text-fuchsia-700 dark:text-fuchsia-300',
    teal: 'border-teal-500/30 bg-teal-500/10 text-teal-700 dark:text-teal-300',
    emerald: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
    amber: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300',
    rose: 'border-rose-500/30 bg-rose-500/10 text-rose-700 dark:text-rose-300',
    sky: 'border-sky-500/30 bg-sky-500/10 text-sky-700 dark:text-sky-300',
};

const SEGMENT_HEAD: Record<Accent, string> = {
    cyan: 'text-cyan-700 dark:text-cyan-400',
    violet: 'text-violet-700 dark:text-violet-400',
    indigo: 'text-indigo-700 dark:text-indigo-400',
    fuchsia: 'text-fuchsia-700 dark:text-fuchsia-400',
    teal: 'text-teal-700 dark:text-teal-400',
    emerald: 'text-emerald-700 dark:text-emerald-400',
    amber: 'text-amber-700 dark:text-amber-400',
    rose: 'text-rose-700 dark:text-rose-400',
    sky: 'text-sky-700 dark:text-sky-400',
};

const DARK_MESH: Record<Accent, string> = {
    cyan: 'bg-[radial-gradient(circle_at_18%_22%,#22d3ee,transparent_42%),radial-gradient(circle_at_82%_72%,#64748b,transparent_40%)]',
    violet:
        'bg-[radial-gradient(circle_at_25%_25%,#a78bfa,transparent_45%),radial-gradient(circle_at_75%_70%,#f472b6,transparent_40%)]',
    indigo:
        'bg-[radial-gradient(circle_at_15%_20%,#6366f1,transparent_45%),radial-gradient(circle_at_85%_70%,#22d3ee,transparent_40%)]',
    fuchsia:
        'bg-[radial-gradient(circle_at_20%_25%,#f472b6,transparent_42%),radial-gradient(circle_at_80%_65%,#a78bfa,transparent_45%)]',
    teal: 'bg-[radial-gradient(circle_at_20%_30%,#2dd4bf,transparent_45%),radial-gradient(circle_at_80%_65%,#64748b,transparent_42%)]',
    emerald:
        'bg-[radial-gradient(circle_at_22%_28%,#34d399,transparent_44%),radial-gradient(circle_at_78%_68%,#64748b,transparent_40%)]',
    amber:
        'bg-[radial-gradient(circle_at_20%_25%,#fbbf24,transparent_42%),radial-gradient(circle_at_80%_70%,#94a3b8,transparent_42%)]',
    rose: 'bg-[radial-gradient(circle_at_22%_30%,#fb7185,transparent_44%),radial-gradient(circle_at_78%_68%,#a78bfa,transparent_40%)]',
    sky: 'bg-[radial-gradient(circle_at_20%_25%,#38bdf8,transparent_44%),radial-gradient(circle_at_80%_70%,#818cf8,transparent_40%)]',
};

const DARK_EYEBROW: Record<Accent, string> = {
    cyan: 'text-cyan-200/95',
    violet: 'text-violet-200/95',
    indigo: 'text-indigo-200/95',
    fuchsia: 'text-fuchsia-200/95',
    teal: 'text-teal-200/95',
    emerald: 'text-emerald-200/95',
    amber: 'text-amber-200/95',
    rose: 'text-rose-200/95',
    sky: 'text-sky-200/95',
};

export type InsightHeroProps = {
    /** Small label above the title */
    eyebrow?: ReactNode;
    title: string;
    /** Primary narrative — spans full content width (no artificial max-width) */
    lead?: ReactNode;
    variant?: 'light' | 'dark';
    accent?: Accent;
    icon?: LucideIcon;
    /** Optional 2–4 cards that break long copy into scannable chunks */
    segments?: InsightHeroSegment[];
    /** Right column on large screens — actions, badges */
    actions?: ReactNode;
    /** Extra rows below segments (e.g. link grids) */
    children?: ReactNode;
    className?: string;
    /** Sets `id` on the `<h1>` for `aria-labelledby` on wrapping regions */
    titleId?: string;
};

/**
 * Full-width page hero for operational / reference copy — consistent with Security Posture–style breadth.
 */
export default function InsightHero({
    eyebrow,
    title,
    lead,
    variant = 'light',
    accent = 'cyan',
    icon: Icon,
    segments,
    actions,
    children,
    className = '',
    titleId,
}: InsightHeroProps) {
    const segCols =
        segments && segments.length >= 4 ? 'md:grid-cols-2 xl:grid-cols-4' : 'md:grid-cols-2 xl:grid-cols-3';

    if (variant === 'dark') {
        return (
            <section
                className={`relative w-full min-w-0 overflow-hidden rounded-2xl border border-slate-200/80 dark:border-slate-700/60 bg-gradient-to-br from-slate-950 via-slate-900 to-slate-950 text-white shadow-lg ${className}`}
            >
                <div className={`pointer-events-none absolute inset-0 opacity-[0.09] ${DARK_MESH[accent]}`} />
                <div className="relative px-6 py-8 sm:px-10 sm:py-10 w-full">
                    <div className="flex flex-col xl:flex-row xl:items-start xl:justify-between gap-8 w-full">
                        <div className="flex gap-5 min-w-0 flex-1">
                            {Icon ? (
                                <div
                                    className={`shrink-0 rounded-xl border p-3 ${ICON_WRAP[accent]} bg-white/5 dark:bg-white/5 backdrop-blur-sm`}
                                >
                                    <Icon className="h-7 w-7" aria-hidden />
                                </div>
                            ) : null}
                            <div className="min-w-0 flex-1 space-y-3">
                                {eyebrow ? (
                                    <div
                                        className={`inline-flex items-center gap-2 text-xs font-semibold uppercase tracking-widest ${DARK_EYEBROW[accent]}`}
                                    >
                                        {eyebrow}
                                    </div>
                                ) : null}
                                <h1 id={titleId} className="text-2xl sm:text-[1.75rem] font-bold tracking-tight text-white">
                                    {title}
                                </h1>
                                {lead ? (
                                    <div className="text-[15px] sm:text-base text-slate-300 leading-relaxed w-full max-w-none [&_code]:text-[11px] [&_code]:px-1 [&_code]:py-0.5 [&_code]:rounded [&_code]:bg-white/10">
                                        {lead}
                                    </div>
                                ) : null}
                            </div>
                        </div>
                        {actions ? <div className="shrink-0 flex flex-wrap gap-2 xl:justify-end">{actions}</div> : null}
                    </div>

                    {segments && segments.length > 0 ? (
                        <div className={`mt-10 grid grid-cols-1 gap-4 w-full ${segCols}`}>
                            {segments.map((s) => (
                                <div
                                    key={s.heading}
                                    className="rounded-xl border border-white/10 bg-white/[0.06] px-4 py-4 backdrop-blur-sm"
                                >
                                    <h2 className={`text-[11px] font-bold uppercase tracking-wider ${DARK_EYEBROW[accent]}`}>
                                        {s.heading}
                                    </h2>
                                    <div className="mt-2 text-sm text-slate-300 leading-relaxed [&_a]:text-cyan-300 [&_a]:hover:underline [&_code]:text-[10px]">
                                        {s.children}
                                    </div>
                                </div>
                            ))}
                        </div>
                    ) : null}

                    {children ? <div className="mt-8 w-full">{children}</div> : null}
                </div>
            </section>
        );
    }

    return (
        <section
            className={`w-full min-w-0 rounded-2xl border border-slate-200/90 dark:border-slate-700/80 bg-gradient-to-br shadow-sm ${LIGHT_SURFACE[accent]} px-6 py-8 sm:px-10 sm:py-10 ${className}`}
        >
            <div className="flex flex-col xl:flex-row xl:items-start xl:justify-between gap-8 w-full">
                <div className="flex gap-5 min-w-0 flex-1">
                    {Icon ? (
                        <div className={`shrink-0 rounded-xl border p-3 ${ICON_WRAP[accent]}`}>
                            <Icon className="h-7 w-7" aria-hidden />
                        </div>
                    ) : null}
                    <div className="min-w-0 flex-1 space-y-3">
                        {eyebrow ? (
                            <div className="inline-flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400">
                                {eyebrow}
                            </div>
                        ) : null}
                        <h1
                            id={titleId}
                            className="text-2xl sm:text-[1.75rem] font-bold tracking-tight text-slate-900 dark:text-white"
                        >
                            {title}
                        </h1>
                        {lead ? (
                            <div className="text-[15px] sm:text-base text-slate-600 dark:text-slate-400 leading-relaxed w-full max-w-none [&_strong]:font-semibold [&_strong]:text-slate-800 dark:[&_strong]:text-slate-200 [&_code]:text-[11px] [&_code]:px-1 [&_code]:py-0.5 [&_code]:rounded [&_code]:bg-slate-200/90 dark:[&_code]:bg-slate-800">
                                {lead}
                            </div>
                        ) : null}
                    </div>
                </div>
                {actions ? <div className="shrink-0 flex flex-wrap gap-2 xl:justify-end">{actions}</div> : null}
            </div>

            {segments && segments.length > 0 ? (
                <div className={`mt-10 grid grid-cols-1 gap-4 w-full ${segCols}`}>
                    {segments.map((s) => (
                        <div
                            key={s.heading}
                            className="rounded-xl border border-slate-200/90 dark:border-slate-700/80 bg-white/85 dark:bg-slate-950/35 px-4 py-4 shadow-sm"
                        >
                            <h2 className={`text-[11px] font-bold uppercase tracking-wider ${SEGMENT_HEAD[accent]}`}>{s.heading}</h2>
                            <div className="mt-2 text-sm text-slate-600 dark:text-slate-400 leading-relaxed [&_a]:text-cyan-600 dark:[&_a]:text-cyan-400 [&_a]:font-semibold [&_a]:hover:underline [&_code]:text-[11px]">
                                {s.children}
                            </div>
                        </div>
                    ))}
                </div>
            ) : null}

            {children ? <div className="mt-8 w-full">{children}</div> : null}
        </section>
    );
}
