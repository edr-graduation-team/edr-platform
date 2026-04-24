import { Link } from 'react-router-dom';
import {
    UserCircle,
    Users,
    Shield,
    FileStack,
    Activity,
    Settings2,
    ArrowUpRight,
} from 'lucide-react';
import InsightHero from '../InsightHero';

const RELATED = [
    {
        to: '/system/profile',
        label: 'Your profile',
        desc: 'Display name, email, password for the signed-in account',
        icon: UserCircle,
    },
    {
        to: '/system/access/users',
        label: 'Directory users',
        desc: 'Platform login accounts (create, roles, activation)',
        icon: Users,
    },
    {
        to: '/system/access/roles',
        label: 'Roles & permissions',
        desc: 'RBAC matrix — not stored on this page',
        icon: Shield,
    },
    {
        to: '/management/context-policies',
        label: 'Context policies',
        desc: 'Server-backed policy rules (API: context-policies)',
        icon: FileStack,
    },
    {
        to: '/system/reliability-health',
        label: 'Reliability health',
        desc: 'Live backend / pipeline status',
        icon: Activity,
    },
] as const;

export default function SettingsLayout({
    children,
}: {
    activeTab?: string;
    onChangeTab?: (id: string) => void;
    children: React.ReactNode;
    userRole?: string;
}) {
    return (
        <div className="flex-1 flex flex-col min-h-0 bg-slate-50 dark:bg-slate-950">
            <div className="px-6 md:px-10 py-6 md:py-8 border-b border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 shrink-0">
                <InsightHero
                    variant="light"
                    accent="sky"
                    icon={Settings2}
                    eyebrow="Settings"
                    title="Platform preferences"
                    className="!rounded-xl border-0 shadow-none bg-transparent px-0 py-0"
                    lead={
                        <>
                            This section holds <strong className="font-semibold text-slate-800 dark:text-slate-200">dashboard-only</strong> controls (theme, locale-style options, local
                            notification UI toggles). Identity, authorization, audit, and operational health are separate routes so nothing overlaps or contradicts server-backed data.
                        </>
                    }
                />
            </div>

            <main className="flex-1 min-h-0 bg-slate-50 dark:bg-slate-950/80 p-6 md:p-10 overflow-auto">
                <div className="w-full max-w-none space-y-8 animate-fade-in">
                    <section aria-labelledby="related-settings-heading">
                        <h2 id="related-settings-heading" className="text-sm font-semibold text-slate-700 dark:text-slate-300 mb-3">
                            Related configuration (elsewhere)
                        </h2>
                        <p className="text-[13px] text-slate-500 dark:text-slate-500 mb-4 leading-relaxed max-w-none">
                            The Settings menu in the top bar lists only this hub; use the links below to jump to areas that use real
                            APIs and shared state — without duplicating them on this page.
                        </p>
                        <ul className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 list-none p-0 m-0">
                            {RELATED.map(({ to, label, desc, icon: Icon }) => (
                                <li key={to}>
                                    <Link
                                        to={to}
                                        className="group flex h-full flex-col rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900/60 p-4 shadow-sm transition hover:border-cyan-300 dark:hover:border-cyan-700 hover:shadow-md"
                                    >
                                        <div className="flex items-start justify-between gap-2">
                                            <div className="flex items-start gap-3 min-w-0">
                                                <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-cyan-50 text-cyan-700 dark:bg-cyan-500/15 dark:text-cyan-400">
                                                    <Icon className="h-4 w-4" aria-hidden />
                                                </span>
                                                <span className="font-semibold text-slate-900 dark:text-white group-hover:text-cyan-700 dark:group-hover:text-cyan-300 leading-snug">
                                                    {label}
                                                </span>
                                            </div>
                                            <ArrowUpRight className="h-4 w-4 shrink-0 text-slate-400 group-hover:text-cyan-600 dark:group-hover:text-cyan-400" aria-hidden />
                                        </div>
                                        <p className="mt-2 text-xs text-slate-500 dark:text-slate-400 leading-relaxed pl-0 sm:pl-12">
                                            {desc}
                                        </p>
                                    </Link>
                                </li>
                            ))}
                        </ul>
                    </section>

                    {children}
                </div>
            </main>
        </div>
    );
}
