import { Package, Wrench, Database, ArrowRight } from 'lucide-react';

// ────────────────────────────────────────────────────────────────────────────
// Software Inventory Tab — Planned Enhancement
//
// This tab will display installed software per endpoint once the agent's
// WMI collector is extended to query Win32_Product / registry uninstall keys.
// For now, it shows a clear roadmap of what data will be available.
// ────────────────────────────────────────────────────────────────────────────

const ROADMAP_ITEMS = [
    {
        icon: Database,
        title: 'Software Inventory Collection',
        description: 'Extend agent WMI collector to query installed programs from the Windows Registry uninstall keys (HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Uninstall).',
        status: 'planned' as const,
    },
    {
        icon: Package,
        title: 'Application Whitelisting',
        description: 'Define approved software policies per device group. Flag unauthorized or shadow-IT installations automatically.',
        status: 'planned' as const,
    },
    {
        icon: Wrench,
        title: 'Version Drift Detection',
        description: 'Compare installed versions across endpoints to detect outdated deployments and enforce patching baselines.',
        status: 'planned' as const,
    },
];

export default function SoftwareInventoryTab() {
    return (
        <div className="space-y-6">
            {/* Status banner */}
            <div className="rounded-xl border border-violet-200/60 dark:border-violet-900/40 bg-gradient-to-r from-violet-50/80 to-indigo-50/60 dark:from-violet-950/20 dark:to-indigo-950/10 p-5 flex items-start gap-4">
                <div className="p-2.5 rounded-lg bg-violet-500/10 border border-violet-500/20 shrink-0">
                    <Package className="w-5 h-5 text-violet-600 dark:text-violet-400" />
                </div>
                <div>
                    <h3 className="text-sm font-bold text-slate-900 dark:text-white">
                        Software Inventory — Planned Enhancement
                    </h3>
                    <p className="text-xs text-slate-600 dark:text-slate-400 mt-1 leading-relaxed max-w-xl">
                        The EDR agent currently collects process execution events and vulnerability scans, 
                        but does not yet inventory installed applications. This capability requires extending the 
                        agent&apos;s WMI collector (<code className="text-[10px] font-mono px-1 rounded bg-slate-200/60 dark:bg-slate-800">wmi.go</code>) 
                        to query <code className="text-[10px] font-mono px-1 rounded bg-slate-200/60 dark:bg-slate-800">Win32_Product</code> or 
                        the registry uninstall keys.
                    </p>
                </div>
            </div>

            {/* Roadmap cards */}
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                {ROADMAP_ITEMS.map((item) => (
                    <div
                        key={item.title}
                        className="rounded-xl border border-slate-200 dark:border-slate-700 bg-white/60 dark:bg-slate-900/40 p-5 space-y-3 transition-all hover:shadow-md hover:border-violet-300 dark:hover:border-violet-700"
                    >
                        <div className="flex items-center justify-between">
                            <div className="p-2 rounded-lg bg-slate-100 dark:bg-slate-800">
                                <item.icon className="w-4 h-4 text-slate-600 dark:text-slate-400" />
                            </div>
                            <span className="text-[10px] font-bold uppercase tracking-wider text-violet-600 dark:text-violet-400 bg-violet-500/10 px-2 py-0.5 rounded-full border border-violet-500/20">
                                {item.status}
                            </span>
                        </div>
                        <h4 className="text-sm font-semibold text-slate-900 dark:text-white">{item.title}</h4>
                        <p className="text-xs text-slate-500 dark:text-slate-400 leading-relaxed">{item.description}</p>
                    </div>
                ))}
            </div>

            {/* What's available now */}
            <div className="rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900/60 p-5">
                <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-300 mb-3">
                    What&apos;s Available Now
                </h3>
                <ul className="space-y-2">
                    {[
                        { label: 'Process Execution Visibility', desc: 'See all executables running on endpoints via ETW telemetry', done: true },
                        { label: 'Vulnerability Scanning', desc: 'Trivy scans installed packages and reports CVE findings', done: true },
                        { label: 'Auto Process Termination', desc: 'Block known-bad processes via prevention rules', done: true },
                        { label: 'Installed Software List', desc: 'Query and display installed applications per device', done: false },
                        { label: 'Application Policies', desc: 'Whitelist/blacklist enforcement engine', done: false },
                    ].map(item => (
                        <li key={item.label} className="flex items-start gap-3 text-xs">
                            <span className={`mt-0.5 shrink-0 w-5 h-5 rounded-full flex items-center justify-center text-[10px] font-bold ${
                                item.done
                                    ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20'
                                    : 'bg-slate-100 dark:bg-slate-800 text-slate-400 border border-slate-200 dark:border-slate-700'
                            }`}>
                                {item.done ? '✓' : '—'}
                            </span>
                            <div>
                                <p className={`font-semibold ${item.done ? 'text-slate-800 dark:text-slate-200' : 'text-slate-500 dark:text-slate-400'}`}>
                                    {item.label}
                                </p>
                                <p className="text-slate-500 dark:text-slate-500 mt-0.5">{item.desc}</p>
                            </div>
                        </li>
                    ))}
                </ul>
            </div>

            {/* Implementation hint */}
            <div className="rounded-xl border border-dashed border-slate-300 dark:border-slate-700 bg-slate-50/50 dark:bg-slate-900/20 p-4 text-xs text-slate-500 dark:text-slate-400 flex items-center gap-2">
                <ArrowRight className="w-3.5 h-3.5 shrink-0" />
                <span>
                    To implement: extend <code className="font-mono px-1 rounded bg-slate-200/60 dark:bg-slate-800">win_edrAgent/internal/collectors/wmi.go</code> with 
                    a <code className="font-mono px-1 rounded bg-slate-200/60 dark:bg-slate-800">collectInstalledSoftware()</code> function 
                    and add a new event type to the connection-manager ingestion pipeline.
                </span>
            </div>
        </div>
    );
}
