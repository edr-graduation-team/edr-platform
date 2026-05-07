import { useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import InsightHero from '../components/InsightHero';
import StatCard from '../components/StatCard';
import {
    Activity,
    AlertTriangle,
    BarChart3,
    Bug,
    Crosshair,
    Network,
    ShieldAlert,
    TerminalSquare,
    Zap,
} from 'lucide-react';
import { statsApi, vulnerabilityApi, commandsApi } from '../api/client';

export default function SocTracker() {
    useEffect(() => {
        document.title = 'SOC Tracker \u2014 EDR Platform';
    }, []);

    // Fetch some basic overviews for the hub KPIs
    const alertStats = useQuery({ queryKey: ['soc-alerts'], queryFn: () => statsApi.alerts(), staleTime: 60_000 });
    const vulnStats = useQuery({ queryKey: ['soc-vuln'], queryFn: () => vulnerabilityApi.listFindings({ limit: 1 }), staleTime: 60_000 });
    const cmdStats = useQuery({ queryKey: ['soc-cmds'], queryFn: () => commandsApi.stats(), staleTime: 60_000 });

    const openAlerts = alertStats.data?.by_status?.open ?? 0;
    const totalVulns = vulnStats.data?.pagination?.total ?? 0;
    const pendingCmds = cmdStats.data?.pending ?? 0;

    const socModules = [
        {
            title: 'Statistics',
            desc: 'High-level SOC metrics, agent health, and alert trends over time.',
            icon: BarChart3,
            link: '/stats',
            color: 'text-indigo-500',
            bg: 'bg-indigo-500/10',
            borderColor: 'border-indigo-500/20'
        },
        {
            title: 'Alerts (Triage)',
            desc: 'Review, assign, and respond to incoming security alerts and detections.',
            icon: ShieldAlert,
            link: '/alerts',
            color: 'text-rose-500',
            bg: 'bg-rose-500/10',
            borderColor: 'border-rose-500/20'
        },
        {
            title: 'Telemetry Search',
            desc: 'Deep dive into raw telemetry events, processes, network connections, and files.',
            icon: SearchIcon,
            link: '/events',
            color: 'text-cyan-500',
            bg: 'bg-cyan-500/10',
            borderColor: 'border-cyan-500/20'
        },
        {
            title: 'Endpoint Risk',
            desc: 'Ranked endpoints by risk score, identifying the most vulnerable hosts.',
            icon: AlertTriangle,
            link: '/endpoint-risk',
            color: 'text-amber-500',
            bg: 'bg-amber-500/10',
            borderColor: 'border-amber-500/20'
        },
        {
            title: 'ATT&CK Analytics (Threats)',
            desc: 'Map detections directly to MITRE ATT&CK tactics and techniques.',
            icon: Crosshair,
            link: '/threats',
            color: 'text-violet-500',
            bg: 'bg-violet-500/10',
            borderColor: 'border-violet-500/20'
        },
        {
            title: 'Detection Rules',
            desc: 'Manage, create, and tune Sigma rules generating security alerts.',
            icon: Zap,
            link: '/rules',
            color: 'text-emerald-500',
            bg: 'bg-emerald-500/10',
            borderColor: 'border-emerald-500/20'
        },
        {
            title: 'Command Center',
            desc: 'Track response actions, remote terminal sessions, and host isolation commands.',
            icon: TerminalSquare,
            link: '/responses',
            color: 'text-sky-500',
            bg: 'bg-sky-500/10',
            borderColor: 'border-sky-500/20'
        },
        {
            title: 'Vulnerability Management',
            desc: 'Track CVEs and out-of-date packages across your entire fleet.',
            icon: Bug,
            link: '/soc/vulnerability',
            color: 'text-orange-500',
            bg: 'bg-orange-500/10',
            borderColor: 'border-orange-500/20'
        },
        {
            title: 'Correlation Engine',
            desc: 'Configure advanced correlation policies to detect complex attack patterns.',
            icon: Network,
            link: '/soc/correlation',
            color: 'text-fuchsia-500',
            bg: 'bg-fuchsia-500/10',
            borderColor: 'border-fuchsia-500/20'
        }
    ];

    return (
        <div className="space-y-6 animate-slide-up-fade w-full min-w-0">
            <InsightHero
                accent="cyan"
                icon={Activity}
                eyebrow="Security Operations"
                title="SOC Tracker Overview"
                lead={
                    <>
                        Central hub for tracking all <strong className="text-white">Security Operations Center</strong> modules. Monitor alerts, hunt threats, manage endpoint risks, and coordinate incident response from one unified view.
                    </>
                }
            />

            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                <StatCard
                    title="Open Alerts"
                    value={alertStats.isLoading ? '...' : String(openAlerts)}
                    icon={ShieldAlert}
                    color={openAlerts > 0 ? "red" : "emerald"}
                    subtext="Awaiting triage"
                />
                <StatCard
                    title="Active Vulnerabilities"
                    value={vulnStats.isLoading ? '...' : String(totalVulns)}
                    icon={Bug}
                    color="amber"
                    subtext="Identified CVE findings"
                />
                <StatCard
                    title="Pending Responses"
                    value={cmdStats.isLoading ? '...' : String(pendingCmds)}
                    icon={TerminalSquare}
                    color="cyan"
                    subtext="Commands awaiting execution"
                />
            </div>

            <h3 className="text-sm font-bold text-slate-800 dark:text-slate-200 mt-8 mb-4 flex items-center gap-2 uppercase tracking-wide">
                <Network className="w-4 h-4 text-cyan-500" />
                SOC Modules & Trackers
            </h3>

            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {socModules.map((mod) => (
                    <Link
                        key={mod.link}
                        to={mod.link}
                        className="group flex flex-col p-5 rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/90 dark:bg-slate-800/80 hover:bg-slate-50 dark:hover:bg-slate-800 transition-all shadow-sm hover:shadow-md"
                    >
                        <div className="flex items-start gap-4 mb-3">
                            <div className={`p-2.5 rounded-lg ${mod.bg} ${mod.borderColor} border`}>
                                <mod.icon className={`w-5 h-5 ${mod.color}`} />
                            </div>
                            <div className="flex-1 mt-1">
                                <h4 className="font-semibold text-slate-900 dark:text-white group-hover:text-cyan-600 dark:group-hover:text-cyan-400 transition-colors">
                                    {mod.title}
                                </h4>
                            </div>
                        </div>
                        <p className="text-xs text-slate-500 dark:text-slate-400 leading-relaxed flex-1">
                            {mod.desc}
                        </p>
                    </Link>
                ))}
            </div>
        </div>
    );
}

function SearchIcon(props: any) {
    return (
        <svg
            {...props}
            xmlns="http://www.w3.org/2000/svg"
            width="24"
            height="24"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
        >
            <circle cx="11" cy="11" r="8" />
            <path d="m21 21-4.3-4.3" />
        </svg>
    );
}
