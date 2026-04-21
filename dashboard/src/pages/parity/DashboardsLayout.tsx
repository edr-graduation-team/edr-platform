import { Suspense } from 'react';
import { Outlet } from 'react-router-dom';
import { LayoutGrid } from 'lucide-react';

export default function DashboardsLayout() {
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
                    <LayoutGrid className="w-6 h-6" />
                </div>
                <div>
                    <h1 className="text-xl font-bold text-gray-900 dark:text-white">Dashboards</h1>
                    <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
                        Live dashboards displaying real-time connection and statistical data.
                    </p>
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

