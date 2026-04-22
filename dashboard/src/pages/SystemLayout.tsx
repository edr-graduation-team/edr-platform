import { Suspense, useMemo } from 'react';
import { Outlet, useLocation } from 'react-router-dom';
import { Settings as SettingsIcon } from 'lucide-react';

export default function SystemLayout() {
    const { pathname } = useLocation();

    const meta = useMemo(() => {
        if (pathname.startsWith('/system/access')) {
            return { title: 'System · Access', desc: 'Users, roles, and permissions for platform administration.' };
        }
        if (pathname.startsWith('/system/audit-logs')) {
            return { title: 'System · Audit', desc: 'Track administrative and platform activities with filters and export.' };
        }
        if (pathname.startsWith('/system/reliability-health')) {
            return { title: 'System · Reliability', desc: 'Operational indicators for ingestion durability and backpressure.' };
        }
        if (pathname.startsWith('/system/platform-settings')) {
            return { title: 'System · Platform Settings', desc: 'Core platform configuration settings.' };
        }
        if (pathname.startsWith('/system/profile')) {
            return { title: 'System · Profile', desc: 'Personal preferences and session profile.' };
        }
        return { title: 'System', desc: 'Access, audit, reliability, and platform administration.' };
    }, [pathname]);

    return (
        <div className="space-y-6">
            <div className="flex items-start gap-3">
                <div
                    className="p-2 rounded-xl border"
                    style={{
                        background: 'rgba(34, 211, 238, 0.08)',
                        borderColor: 'rgba(34, 211, 238, 0.25)',
                        color: 'var(--xc-brand-original)',
                    }}
                >
                    <SettingsIcon className="w-6 h-6" />
                </div>
                <div>
                    <h1 className="text-xl font-bold text-gray-900 dark:text-white">{meta.title}</h1>
                    <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">{meta.desc}</p>
                </div>
            </div>

            <Suspense
                fallback={
                    <div className="flex items-center justify-center py-16 text-sm text-gray-500 dark:text-gray-400">
                        Loading…
                    </div>
                }
            >
                <Outlet />
            </Suspense>
        </div>
    );
}

