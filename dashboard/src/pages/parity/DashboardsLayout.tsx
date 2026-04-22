import { Suspense, useMemo } from 'react';
import { Outlet, useLocation } from 'react-router-dom';
import { LayoutGrid } from 'lucide-react';

export default function DashboardsLayout() {
    const { pathname } = useLocation();

    const meta = useMemo(() => {
        const p = pathname;
        if (p.startsWith('/dashboards/service')) {
            return {
                title: 'Security Posture',
                desc: 'High-level service posture built from command metrics, alerts, and reliability health.',
            };
        }
        if (p.startsWith('/dashboards/endpoint')) {
            return {
                title: 'Endpoint Summary',
                desc: 'Fleet overview using agent metrics, OS distribution, and top risk signals.',
            };
        }
        if (p.startsWith('/dashboards/endpoint-compliance')) {
            return {
                title: 'Endpoint Compliance',
                desc: 'Compliance posture based on enrollment health, online status, isolation, and mTLS.',
            };
        }
        if (p.startsWith('/dashboards/reports')) {
            return {
                title: 'Reports',
                desc: 'Generate downloadable reports with filters for time range, scope, and dataset type.',
            };
        }
        return {
            title: 'Dashboards',
            desc: 'Live dashboards displaying real-time connection and statistical data.',
        };
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
                    <LayoutGrid className="w-6 h-6" />
                </div>
                <div>
                    <h1 className="text-xl font-bold text-gray-900 dark:text-white">{meta.title}</h1>
                    <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
                        {meta.desc}
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

