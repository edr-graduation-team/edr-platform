import { Suspense } from 'react';
import { Outlet } from 'react-router-dom';

/**
 * Minimal layout wrapper for /dashboards/* routes.
 * Navigation is already handled by PlatformAppShell's context bar.
 * This layout only provides a Suspense boundary for lazy-loaded child pages.
 */
export default function DashboardsLayout() {
    return (
        <Suspense
            fallback={
                <div className="space-y-4 animate-pulse">
                    <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                        {[...Array(4)].map((_, i) => (
                            <div key={i} className="h-24 rounded-xl bg-slate-200 dark:bg-slate-800" />
                        ))}
                    </div>
                    <div className="h-64 rounded-xl bg-slate-200 dark:bg-slate-800" />
                </div>
            }
        >
            <Outlet />
        </Suspense>
    );
}
