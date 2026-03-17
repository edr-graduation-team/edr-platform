import { useQuery } from '@tanstack/react-query';
import React, { useEffect, useState, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import {
    AlertTriangle, Activity, Monitor,
    Wifi, WifiOff, ChevronRight, Target, Clock, Terminal, Zap, ShieldAlert
} from 'lucide-react';
import { statsApi, alertsApi, agentsApi, createAlertStream, type Alert, type AgentStats, type Agent } from '../api/client';
import { SkeletonKPICards } from '../components';

const STALE_THRESHOLD_MS = 5 * 60 * 1000;

function KPICard({ title, value, icon: Icon, color = 'primary', subValue, onClick }: any) {
    const colorClasses: Record<string, string> = {
        primary: 'text-cyan-400 bg-cyan-400/10 ring-1 ring-cyan-400/20',
        danger: 'text-rose-400 bg-rose-400/10 ring-1 ring-rose-400/20',
        warning: 'text-amber-400 bg-amber-400/10 ring-1 ring-amber-400/20',
        success: 'text-emerald-400 bg-emerald-400/10 ring-1 ring-emerald-400/20',
    };

    return (
        <div
            onClick={onClick}
            className={`card border border-slate-200 dark:border-slate-700/60 shadow-sm dark:shadow-slate-900/20 bg-white dark:bg-slate-800/90 backdrop-blur-sm rounded-xl p-5 flex flex-col justify-between transition-all duration-200 ${onClick ? 'cursor-pointer hover:shadow-lg hover:-translate-y-0.5' : ''}`}
        >
            <div className="flex items-start justify-between">
                <div>
                    <p className="text-sm font-medium text-slate-400 mb-1.5">{title}</p>
                    <p className="text-3xl font-bold text-slate-800 dark:text-white tracking-tight">{value}</p>
                </div>
                <div className={`p-2.5 rounded-lg ${colorClasses[color] || colorClasses.primary}`}>
                    <Icon className="w-5 h-5" />
                </div>
            </div>
            {subValue && (
                <div className="mt-4 pt-4 border-t border-slate-800/80 flex items-center justify-between">
                    <p className="text-xs text-slate-500 font-medium">{subValue}</p>
                    {onClick && <ChevronRight className="w-4 h-4 text-slate-600" />}
                </div>
            )}
        </div>
    );
}

const AlertsFeed = React.memo(function AlertsFeed({ alerts }: { alerts: Alert[] }) {
    const severityConfig: Record<string, { color: string, border: string, bg: string }> = {
        critical: { color: 'text-rose-400', border: 'border-rose-500/50', bg: 'bg-rose-500/10' },
        high: { color: 'text-orange-400', border: 'border-orange-500/50', bg: 'bg-orange-500/10' },
        medium: { color: 'text-amber-400', border: 'border-amber-500/50', bg: 'bg-amber-500/10' },
        low: { color: 'text-indigo-400', border: 'border-indigo-500/50', bg: 'bg-indigo-500/10' },
        informational: { color: 'text-cyan-400', border: 'border-cyan-500/50', bg: 'bg-cyan-500/10' },
    };

    const formatRel = (ts: string) => {
        const diff = Date.now() - new Date(ts).getTime();
        const mins = Math.floor(diff / 60000);
        if (mins < 1) return 'Just now';
        if (mins < 60) return `${mins}m ago`;
        const hrs = Math.floor(mins / 60);
        if (hrs < 24) return `${hrs}h ago`;
        return `${Math.floor(hrs / 24)}d ago`;
    };

    return (
        <div className="card border border-slate-200 dark:border-slate-700/60 shadow-lg dark:shadow-cyan-900/10 bg-white dark:bg-slate-800/90 backdrop-blur-sm rounded-xl p-5 flex flex-col h-full min-h-[450px]">
            <div className="flex items-center justify-between mb-5 border-b border-slate-200 dark:border-slate-700/60 pb-4 shrink-0">
                <h3 className="text-sm font-bold text-slate-800 dark:text-slate-200 uppercase tracking-widest flex items-center gap-2">
                    <Activity className="w-4 h-4 text-cyan-500 dark:text-cyan-400" /> Live Alerts Stream
                </h3>
                <span className="flex items-center gap-2 text-[10px] font-bold tracking-widest text-emerald-400 uppercase">
                    <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" /> Live
                </span>
            </div>
            
            <div className="space-y-3 flex-1 overflow-y-auto pr-2 custom-scrollbar">
                {alerts.length === 0 ? (
                    <div className="flex flex-col items-center justify-center h-full text-slate-500 min-h-[200px]">
                        <Zap className="w-6 h-6 opacity-40 mb-3" />
                        <span className="text-sm">Listening for signals...</span>
                    </div>
                ) : (
                    alerts.slice(0, 50).map((alert) => {
                        const style = severityConfig[alert.severity] || severityConfig.informational;
                        return (
                            <div key={alert.id} className="bg-slate-50/50 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-700/40 rounded-lg p-4 hover:bg-slate-100 dark:hover:bg-slate-700/60 transition-colors flex items-start gap-4">
                                <div className={`mt-1 flex-shrink-0 w-2 h-2 rounded-full ${style.bg} ${style.border} border shadow-[0_0_8px_currentColor] ${style.color}`} />
                                <div className="flex-1 min-w-0">
                                    <div className="flex justify-between items-start mb-1">
                                        <p className="font-semibold text-slate-800 dark:text-slate-200 truncate pr-4 text-sm">{alert.rule_title}</p>
                                        <span className={`text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded-full border ${style.bg} ${style.color} ${style.border}`}>
                                            {alert.severity}
                                        </span>
                                    </div>
                                    <div className="flex items-center gap-4 mt-2">
                                        <div className="font-mono text-[10px] text-slate-500 dark:text-slate-400 bg-slate-100 dark:bg-slate-900/50 px-2 py-1 rounded">
                                            SRC: {alert.agent_id?.slice(0, 8)}
                                        </div>
                                        <div className="text-[10px] text-slate-500 flex items-center gap-1 shrink-0">
                                            <Clock className="w-3 h-3" /> {formatRel(alert.timestamp)}
                                        </div>
                                    </div>
                                </div>
                            </div>
                        );
                    })
                )}
            </div>
        </div>
    );
});


const ActiveIncidentQueue = React.memo(function ActiveIncidentQueue({ alerts }: { alerts: Alert[] }) {
    const activeThreats = alerts.filter(a => 
        (a.severity === 'critical' || a.severity === 'high') &&
        (a.status === 'open' || a.status === 'in_progress')
    ).slice(0, 10);

    return (
        <div className="card border border-slate-200 dark:border-slate-700/60 shadow-lg dark:shadow-cyan-900/10 bg-white dark:bg-slate-800/90 backdrop-blur-sm rounded-xl p-5 flex flex-col flex-1 min-h-0">
            <div className="flex items-center justify-between mb-4 border-b border-slate-200 dark:border-slate-700/60 pb-3 shrink-0">
                <div className="flex items-center gap-2">
                    <Target className="w-4 h-4 text-rose-500 dark:text-rose-400" />
                    <h3 className="text-sm font-bold text-slate-800 dark:text-slate-200 uppercase tracking-widest">
                        Incident Queue
                    </h3>
                </div>
                <span className="text-[10px] font-bold text-rose-600 dark:text-rose-400 uppercase tracking-widest bg-rose-100 dark:bg-rose-400/10 px-2 py-1 rounded-md border border-rose-200 dark:border-rose-400/20">
                    Priority
                </span>
            </div>
            
            <div className="space-y-3 flex-1 overflow-y-auto pr-1 custom-scrollbar min-h-[150px]">
                {activeThreats.length === 0 ? (
                    <div className="flex flex-col items-center justify-center p-4 text-slate-500 h-full">
                        <ShieldAlert className="w-6 h-6 opacity-30 mb-2" />
                        <span className="text-xs">No pending items.</span>
                    </div>
                ) : (
                    activeThreats.map((alert) => (
                        <div key={alert.id} className="group bg-slate-50/50 dark:bg-slate-800/40 hover:bg-slate-100 dark:hover:bg-slate-700/50 border border-slate-200 dark:border-slate-700/50 rounded-lg p-3 transition-colors cursor-pointer">
                            <div className="flex justify-between items-start mb-2 gap-2">
                                <span className="font-mono text-[11px] font-semibold text-rose-600 dark:text-rose-400 truncate flex-1" title={alert.rule_title}>
                                    {alert.rule_title}
                                </span>
                                <span className="text-[10px] text-slate-500 shrink-0">
                                    {Math.floor((Date.now() - new Date(alert.timestamp).getTime()) / 60000)}m
                                </span>
                            </div>
                            <div className="flex justify-between items-center mt-2 pt-2 border-t border-slate-200 dark:border-slate-700/50">
                                <span className="text-[10px] font-mono text-slate-500 dark:text-slate-400 truncate">TGT: {alert.agent_id?.slice(0, 8)}</span>
                                <span className="text-xs font-medium text-slate-600 dark:text-slate-300 group-hover:text-cyan-600 dark:group-hover:text-cyan-400 transition-colors">Triage &rarr;</span>
                            </div>
                        </div>
                    ))
                )}
            </div>
        </div>
    );
});


const EndpointsPulse = React.memo(function EndpointsPulse({ stats, agents, onClick }: { stats: AgentStats | null; agents: Agent[]; onClick: () => void }) {
    if (!stats) return null;

    let online = 0, offline = 0, degraded = 0;
    if (agents.length > 0) {
        const now = Date.now();
        agents.forEach((a) => {
            const isStale = (now - new Date(a.last_seen).getTime()) > STALE_THRESHOLD_MS;
            if (a.status === 'online' && !isStale) online++;
            else if (a.status === 'degraded' && !isStale) degraded++;
            else offline++;
        });
    } else {
        online = stats.online;
        offline = stats.offline;
        degraded = stats.degraded;
    }
    const total = online + offline + degraded;

    return (
        <div className="card border border-slate-200 dark:border-slate-700/60 shadow-lg dark:shadow-cyan-900/10 bg-white dark:bg-slate-800/90 backdrop-blur-sm rounded-xl p-5 flex flex-col cursor-pointer transition-transform hover:-translate-y-0.5 hover:shadow-xl shrink-0" onClick={onClick}>
            <div className="flex items-center justify-between mb-4 border-b border-slate-200 dark:border-slate-700/60 pb-3 shrink-0">
                <div className="flex items-center gap-2">
                    <Monitor className="w-4 h-4 text-cyan-500 dark:text-cyan-400" />
                    <h3 className="text-sm font-bold text-slate-800 dark:text-slate-200 uppercase tracking-widest">
                        Agent Pulse
                    </h3>
                </div>
            </div>

            <div className="flex justify-between items-end pb-2">
                <div className="flex flex-col items-center">
                    <span className="text-2xl font-bold text-emerald-400">{online}</span>
                    <span className="text-[10px] font-bold uppercase tracking-widest text-slate-500 mt-1 flex items-center gap-1"><Wifi className="w-3 h-3"/> Online</span>
                </div>
                <div className="flex flex-col items-center">
                    <span className="text-2xl font-bold text-amber-400">{degraded}</span>
                    <span className="text-[10px] font-bold uppercase tracking-widest text-slate-500 mt-1 flex items-center gap-1"><AlertTriangle className="w-3 h-3"/> Warn</span>
                </div>
                <div className="flex flex-col items-center">
                    <span className="text-2xl font-bold text-slate-500">{offline}</span>
                    <span className="text-[10px] font-bold uppercase tracking-widest text-slate-500 mt-1 flex items-center gap-1"><WifiOff className="w-3 h-3"/> Offline</span>
                </div>
            </div>

            <div className="mt-4 pt-4 border-t border-slate-200 dark:border-slate-800 flex flex-col gap-2">
                <div className="flex gap-1 h-1.5 rounded-full overflow-hidden bg-slate-200 dark:bg-slate-800 w-full">
                    <div className="bg-emerald-500 transition-all duration-500" style={{ width: `${total > 0 ? (online/total)*100 : 0}%` }} />
                    <div className="bg-amber-500 transition-all duration-500" style={{ width: `${total > 0 ? (degraded/total)*100 : 0}%` }} />
                    <div className="bg-slate-400 dark:bg-slate-500 transition-all duration-500" style={{ width: `${total > 0 ? (offline/total)*100 : 0}%` }} />
                </div>
                <div className="text-[10px] text-slate-500 dark:text-slate-400 font-mono text-right">{total} TOTAL AGENTS</div>
            </div>
        </div>
    );
});


const SystemActionLog = React.memo(function SystemActionLog({ alerts, agents }: { alerts: Alert[]; agents: Agent[] }) {
    const logEntries = useMemo(() => [
        ...alerts.map(a => ({
            id: `a-${a.id}`,
            timestamp: new Date(a.timestamp).getTime(),
            timeStr: new Date(a.timestamp).toLocaleTimeString('en-US', { hour12: false }),
            type: 'THREAT',
            message: `[DETECT] ${a.severity.toUpperCase()} risk on node ${a.agent_id?.slice(0,8)}: ${a.rule_title}`,
            color: a.severity === 'critical' ? 'text-rose-600 dark:text-rose-400' : 'text-amber-600 dark:text-amber-400'
        })),
        ...agents.map(ag => ({
            id: `ag-${ag.id}`,
            timestamp: new Date(ag.last_seen).getTime(),
            timeStr: new Date(ag.last_seen).toLocaleTimeString('en-US', { hour12: false }),
            type: 'SYSTEM',
            message: `[CHECK-IN] Agent ${ag.hostname} reported healthy status.`,
            color: 'text-cyan-600 dark:text-cyan-400'
        }))
    ].sort((a, b) => b.timestamp - a.timestamp).slice(0, 40), [alerts, agents]);

    return (
        <div className="card border border-slate-200 dark:border-slate-700/60 shadow-lg dark:shadow-cyan-900/10 bg-white dark:bg-slate-800/90 backdrop-blur-sm rounded-xl overflow-hidden flex flex-col h-[300px] font-mono w-full">
            <div className="bg-slate-50 dark:bg-slate-950/50 px-4 py-3 border-b border-slate-200 dark:border-slate-700/60 flex items-center justify-between shrink-0">
                <div className="flex items-center gap-2">
                    <Terminal className="w-4 h-4 text-slate-500 dark:text-slate-400" />
                    <span className="text-[11px] text-slate-600 dark:text-slate-300 font-bold tracking-widest uppercase">System Action Log</span>
                </div>
                <div className="flex gap-1.5">
                    <div className="w-2.5 h-2.5 rounded-full bg-slate-300 dark:bg-slate-700" />
                    <div className="w-2.5 h-2.5 rounded-full bg-slate-300 dark:bg-slate-700" />
                    <div className="w-2.5 h-2.5 rounded-full bg-slate-300 dark:bg-slate-700" />
                </div>
            </div>
            
            <div className="p-4 space-y-1.5 overflow-y-auto text-[11px] leading-relaxed flex-1 custom-scrollbar">
                {logEntries.length === 0 ? (
                    <div className="text-slate-600 italic">Awaiting terminal stream...</div>
                ) : (
                    logEntries.map((entry) => (
                        <div key={entry.id} className="flex gap-3 hover:bg-slate-100 dark:hover:bg-slate-800/80 px-2 py-0.5 -mx-2 rounded transition-colors group">
                            <span className="text-slate-500 shrink-0">{entry.timeStr}</span>
                            <span className={`${entry.color} truncate`} title={entry.message}>
                                {entry.message}
                            </span>
                        </div>
                    ))
                )}
            </div>
        </div>
    );
});


export default function Dashboard() {
    const navigate = useNavigate();
    const [liveAlerts, setLiveAlerts] = useState<Alert[]>([]);

    const { data: alertStats, isLoading: statsLoading } = useQuery({
        queryKey: ['alertStats'],
        queryFn: statsApi.alerts,
        refetchInterval: 30000,
    });

    const { data: agentStats } = useQuery({
        queryKey: ['agentStats'],
        queryFn: agentsApi.stats,
        retry: false,
    });

    const { data: agentListData } = useQuery({
        queryKey: ['agents'],
        queryFn: () => agentsApi.list({ limit: 200 }),
        retry: false,
    });

    const { data: recentAlerts } = useQuery({
        queryKey: ['recentAlerts'],
        queryFn: () => alertsApi.list({ limit: 100 }),
    });

    useEffect(() => {
        if (recentAlerts?.alerts) {
            setLiveAlerts(recentAlerts.alerts);
        }
    }, [recentAlerts]);

    useEffect(() => {
        const stream = createAlertStream((alert) => {
            setLiveAlerts((prev) => [alert, ...prev.slice(0, 99)]);
        }, { severity: ['critical', 'high', 'medium', 'low'] });

        return () => stream.close();
    }, []);

    if (statsLoading) {
        return (
            <div className="space-y-6">
                <div className="h-9 w-64 bg-slate-800 rounded animate-pulse" />
                <SkeletonKPICards count={4} />
                <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
                    <div className="lg:col-span-2 h-96 bg-slate-800 rounded-xl animate-pulse" />
                    <div className="lg:col-span-1 h-96 bg-slate-800 rounded-xl animate-pulse" />
                </div>
            </div>
        );
    }

    return (
        <div className="space-y-8 pb-8">
            <div className="flex items-center justify-between">
                <div>
                    <h1 className="text-2xl font-bold text-slate-900 dark:text-white tracking-tight">
                        Security Posture
                    </h1>
                    <p className="text-sm text-slate-400 mt-1">Live Threat Monitoring & System Pulse</p>
                </div>
            </div>

            {/* KPI Header Row - 4 Cards */}
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-6">
                <KPICard
                    title="Total Alerts (24h)"
                    value={alertStats?.last_24h || 0}
                    icon={AlertTriangle}
                    color="warning"
                    subValue="Volume over 24 hours"
                    onClick={() => navigate('/alerts')}
                />
                <KPICard
                    title="Critical Threats"
                    value={alertStats?.by_severity?.critical || 0}
                    icon={ShieldAlert}
                    color="danger"
                    subValue="Requires immediate triage"
                    onClick={() => navigate('/alerts?severity=critical')}
                />
                <KPICard
                    title="Active Agents"
                    value={agentStats?.online || 0}
                    icon={Monitor}
                    color="success"
                    subValue="Currently reporting OK"
                    onClick={() => navigate('/endpoints')}
                />
                <KPICard
                    title="Detection Engine"
                    value={`${((alertStats?.avg_confidence || 0) * 100).toFixed(1)}%`}
                    icon={Activity}
                    color="primary"
                    subValue="Average rule confidence"
                />
            </div>

            {/* Strict Grid Layout */}
            <div className="grid grid-cols-1 xl:grid-cols-12 gap-6 w-full">
                {/* Main Column - Span 8 */}
                <div className="xl:col-span-8 flex flex-col h-[550px] min-h-0">
                    <AlertsFeed alerts={liveAlerts} />
                </div>

                {/* Side Column - Span 4 */}
                <div className="xl:col-span-4 flex flex-col gap-6 h-[550px] min-h-0">
                    <div className="flex-1 flex flex-col min-h-0">
                        <ActiveIncidentQueue alerts={liveAlerts} />
                    </div>
                    <div className="shrink-0">
                        <EndpointsPulse 
                            stats={agentStats || null} 
                            agents={agentListData?.data || []} 
                            onClick={() => navigate('/endpoints')} 
                        />
                    </div>
                </div>
            </div>

            {/* Bottom Row - Full Width */}
            <div className="w-full shrink-0">
                <SystemActionLog alerts={liveAlerts} agents={agentListData?.data || []} />
            </div>
        </div>
    );
}
