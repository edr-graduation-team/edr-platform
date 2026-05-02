import React, { useState, useEffect, Suspense, lazy } from 'react';
import { Layers, Activity, ShieldAlert, Package, RefreshCw } from 'lucide-react';
import InsightHero from '../../components/InsightHero';

// ────────────────────────────────────────────────────────────────────────────
// Application Control — main page
//
// Three tabs:
//  1. Process Analytics  — what's running across the fleet (from events API)
//  2. Vulnerability Findings — compact summary of Trivy CVE scan results
//  3. Software Inventory — planned enhancement (roadmap placeholder)
// ────────────────────────────────────────────────────────────────────────────

// Lazy-load tabs for code-splitting
const ProcessAnalyticsTab = lazy(() => import('./ProcessAnalyticsTab'));
const VulnerabilityOverviewTab = lazy(() => import('./VulnerabilityOverviewTab'));
const SoftwareInventoryTab = lazy(() => import('./SoftwareInventoryTab'));

type TabId = 'processes' | 'vulnerabilities' | 'inventory';

const TABS: { id: TabId; label: string; icon: React.ElementType; description: string }[] = [
    {
        id: 'processes',
        label: 'Process Analytics',
        icon: Activity,
        description: 'Execution visibility across fleet endpoints',
    },
    {
        id: 'vulnerabilities',
        label: 'Vulnerability Findings',
        icon: ShieldAlert,
        description: 'CVE exposure from Trivy scans',
    },
    {
        id: 'inventory',
        label: 'Software Inventory',
        icon: Package,
        description: 'Installed applications (planned)',
    },
];

function TabSkeleton() {
    return (
        <div className="flex items-center justify-center py-20 text-slate-500 gap-2">
            <RefreshCw className="w-5 h-5 animate-spin" /> Loading…
        </div>
    );
}

export default function ApplicationControlPage() {
    const [activeTab, setActiveTab] = useState<TabId>('processes');

    useEffect(() => {
        document.title = 'Application Control | EDR Platform';
    }, []);

    return (
        <div className="space-y-6 w-full min-w-0 animate-slide-up-fade">
            {/* Hero */}
            <InsightHero
                icon={Layers}
                accent="violet"
                eyebrow="Management"
                title="Application Control"
                segments={[
                    {
                        heading: 'Process visibility',
                        children: (
                            <>
                                Real-time process execution analytics from{' '}
                                <strong className="font-medium text-slate-800 dark:text-slate-200">ETW kernel telemetry</strong>.
                                Every process creation event is categorised and ranked by frequency to surface
                                scripting engines, admin tools, and remote-access utilities.
                            </>
                        ),
                    },
                    {
                        heading: 'Vulnerability context',
                        children: (
                            <>
                                <strong className="font-medium text-slate-800 dark:text-slate-200">Trivy</strong> scans
                                installed packages every 6 hours and reports CVE findings enriched with CISA KEV data
                                and EDR exploit signals.
                            </>
                        ),
                    },
                    {
                        heading: 'Data sources',
                        children: (
                            <>
                                Process events from{' '}
                                <code className="text-[11px] font-mono px-1 rounded bg-slate-200/90 dark:bg-slate-800">
                                    POST /events/search
                                </code>{' '}
                                and vulnerability findings from{' '}
                                <code className="text-[11px] font-mono px-1 rounded bg-slate-200/90 dark:bg-slate-800">
                                    GET /vuln/findings
                                </code>{' '}
                                — both powered by real backend data, no mocks.
                            </>
                        ),
                    },
                ]}
            />

            {/* Tab bar */}
            <div className="flex items-center gap-1 border-b border-slate-200 dark:border-slate-700 overflow-x-auto">
                {TABS.map(tab => {
                    const isActive = activeTab === tab.id;
                    const Icon = tab.icon;
                    return (
                        <button
                            key={tab.id}
                            onClick={() => setActiveTab(tab.id)}
                            className={`group flex items-center gap-2 px-4 py-3 text-sm font-semibold whitespace-nowrap border-b-2 transition-all ${
                                isActive
                                    ? 'border-violet-500 text-violet-700 dark:text-violet-300'
                                    : 'border-transparent text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 hover:border-slate-300'
                            }`}
                        >
                            <Icon className={`w-4 h-4 ${isActive ? 'text-violet-500' : 'text-slate-400 group-hover:text-slate-500'}`} />
                            {tab.label}
                        </button>
                    );
                })}
            </div>

            {/* Active tab content */}
            <Suspense fallback={<TabSkeleton />}>
                {activeTab === 'processes' && <ProcessAnalyticsTab />}
                {activeTab === 'vulnerabilities' && <VulnerabilityOverviewTab />}
                {activeTab === 'inventory' && <SoftwareInventoryTab />}
            </Suspense>
        </div>
    );
}
