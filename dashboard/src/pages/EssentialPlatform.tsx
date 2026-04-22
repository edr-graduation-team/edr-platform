import { Link } from 'react-router-dom';
import { Shield, Server, Activity, Terminal, Search, Lock } from 'lucide-react';

export default function EssentialPlatform() {
    return (
        <div className="relative flex flex-col min-h-[calc(100vh-2rem)] -mx-4 sm:-mx-6 lg:-mx-8 -my-4 sm:-my-6 lg:-my-8 p-4 sm:p-6 lg:p-8 bg-slate-200 dark:bg-gradient-to-br dark:from-slate-900 dark:via-[#0b1120] dark:to-slate-900">
            <div className=" w-full space-y-5">
                <div className="rounded-2xl border border-slate-200 dark:border-slate-700 bg-white/70 dark:bg-slate-900/40 backdrop-blur p-6 sm:p-8">
                    <div className="flex items-start gap-3">
                        <div className="p-2 rounded-xl border border-cyan-500/20 bg-cyan-500/10 text-cyan-700 dark:text-cyan-300">
                            <Shield className="w-6 h-6" />
                        </div>
                        <div className="flex-1">
                            <h1 className="text-2xl sm:text-3xl font-bold text-slate-900 dark:text-white tracking-tight">
                                Essential Platform
                            </h1>
                            <p className="text-sm text-slate-600 dark:text-slate-300 mt-2 leading-relaxed">
                                This deployment is a self-hosted endpoint security platform that combines agent telemetry,
                                central storage, and analyst response workflows. It is designed to help you <strong>observe</strong>,
                                <strong> investigate</strong>, and <strong>respond</strong> to endpoint activity using the APIs and UI in this dashboard.
                            </p>
                            <div className="mt-4 grid grid-cols-1 md:grid-cols-2 gap-3 text-sm text-slate-600 dark:text-slate-300">
                                <div className="rounded-xl border border-slate-200 dark:border-slate-700 bg-white/60 dark:bg-slate-950/20 p-4">
                                    <div className="font-semibold text-slate-900 dark:text-white">What problems it solves</div>
                                    <ul className="mt-2 space-y-1">
                                        <li>- Centralized visibility across endpoints (status, health, alerts).</li>
                                        <li>- Faster investigations with searchable telemetry + raw payload access.</li>
                                        <li>- Consistent response actions via an auditable command pipeline.</li>
                                    </ul>
                                </div>
                                <div className="rounded-xl border border-slate-200 dark:border-slate-700 bg-white/60 dark:bg-slate-950/20 p-4">
                                    <div className="font-semibold text-slate-900 dark:text-white">Key capabilities</div>
                                    <ul className="mt-2 space-y-1">
                                        <li>- Detection & alerting (Sigma rules engine).</li>
                                        <li>- Device management and endpoint profiles.</li>
                                        <li>- Remote response (containment, forensics, blocking).</li>
                                        <li>- Governance (audit logs, roles & permissions, tokens).</li>
                                    </ul>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div className="rounded-2xl border border-slate-200 dark:border-slate-700 bg-white/70 dark:bg-slate-900/40 backdrop-blur p-5 space-y-2">
                        <div className="flex items-center gap-2 text-slate-800 dark:text-slate-100 font-semibold">
                            <Server className="w-4 h-4 text-cyan-500" /> Core services
                        </div>
                        <ul className="text-sm text-slate-600 dark:text-slate-300 space-y-1">
                            <li>- Connection Manager: agents, commands, audit, and event storage/search.</li>
                            <li>- Sigma Engine: alerting/detections (alerts, rules, stats).</li>
                            <li>- Dashboard: analyst UI for fleet, alerts, events, and response actions.</li>
                        </ul>
                    </div>

                    <div className="rounded-2xl border border-slate-200 dark:border-slate-700 bg-white/70 dark:bg-slate-900/40 backdrop-blur p-5 space-y-2">
                        <div className="flex items-center gap-2 text-slate-800 dark:text-slate-100 font-semibold">
                            <Activity className="w-4 h-4 text-cyan-500" /> What you can do
                        </div>
                        <ul className="text-sm text-slate-600 dark:text-slate-300 space-y-1">
                            <li>- Monitor alerts and endpoint status across the fleet.</li>
                            <li>- Search stored events and open raw payloads for investigation.</li>
                            <li>- Execute response commands (isolation, quarantine, blocking, forensics collection).</li>
                            <li>- Push runtime configuration updates and allow exceptions when needed.</li>
                        </ul>
                    </div>
                </div>

                <div className="rounded-2xl border border-slate-200 dark:border-slate-700 bg-white/70 dark:bg-slate-900/40 backdrop-blur p-5 space-y-3">
                    <div className="flex items-center gap-2 text-slate-800 dark:text-slate-100 font-semibold">
                        <Lock className="w-4 h-4 text-cyan-500" /> Permissions and guardrails
                    </div>
                    <p className="text-sm text-slate-600 dark:text-slate-300 leading-relaxed">
                        Many endpoints are guarded by role permissions (e.g. <code className="text-xs">alerts:read</code> for event search/detail,
                        and <code className="text-xs">responses:execute</code> to run commands). The UI follows the same guards as the backend.
                    </p>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                    <Link
                        to="/dashboards/service"
                        className="rounded-2xl border border-slate-200 dark:border-slate-700 bg-white/70 dark:bg-slate-900/40 backdrop-blur p-5 hover:bg-slate-50 dark:hover:bg-slate-800/40 transition-colors"
                    >
                        <div className="flex items-center gap-2 font-semibold text-slate-900 dark:text-white">
                            <Shield className="w-4 h-4 text-cyan-500" /> Security Posture
                        </div>
                        <p className="text-sm text-slate-600 dark:text-slate-300 mt-2">
                            Live threat monitoring and system pulse.
                        </p>
                    </Link>
                    <Link
                        to="/dashboards/endpoint"
                        className="rounded-2xl border border-slate-200 dark:border-slate-700 bg-white/70 dark:bg-slate-900/40 backdrop-blur p-5 hover:bg-slate-50 dark:hover:bg-slate-800/40 transition-colors"
                    >
                        <div className="flex items-center gap-2 font-semibold text-slate-900 dark:text-white">
                            <Activity className="w-4 h-4 text-cyan-500" /> Endpoint Summary
                        </div>
                        <p className="text-sm text-slate-600 dark:text-slate-300 mt-2">
                            Fleet health, risk, and top affected endpoints.
                        </p>
                    </Link>
                    <Link
                        to="/events"
                        className="rounded-2xl border border-slate-200 dark:border-slate-700 bg-white/70 dark:bg-slate-900/40 backdrop-blur p-5 hover:bg-slate-50 dark:hover:bg-slate-800/40 transition-colors"
                    >
                        <div className="flex items-center gap-2 font-semibold text-slate-900 dark:text-white">
                            <Search className="w-4 h-4 text-cyan-500" /> Telemetry Search
                        </div>
                        <p className="text-sm text-slate-600 dark:text-slate-300 mt-2">
                            Search and view raw event payloads.
                        </p>
                    </Link>
                </div>

                <div className="rounded-2xl border border-slate-200 dark:border-slate-700 bg-white/70 dark:bg-slate-900/40 backdrop-blur p-5">
                    <div className="flex items-center justify-between flex-wrap gap-3">
                        <div className="flex items-center gap-2 font-semibold text-slate-900 dark:text-white">
                            <Terminal className="w-4 h-4 text-cyan-500" /> Next steps
                        </div>
                        <div className="flex flex-wrap gap-3 text-sm">
                            <Link className="text-cyan-700 dark:text-cyan-300 hover:underline font-medium" to="/management/devices">
                                Devices (Fleet) →
                            </Link>
                            <Link className="text-cyan-700 dark:text-cyan-300 hover:underline font-medium" to="/responses">
                                Command Center →
                            </Link>
                            <Link className="text-cyan-700 dark:text-cyan-300 hover:underline font-medium" to="/alerts">
                                Alerts (Triage) →
                            </Link>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    );
}


