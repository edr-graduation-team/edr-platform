import { useQuery, useQueryClient } from '@tanstack/react-query';
import React, { useEffect, useState, useMemo, useCallback, useRef } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import {
    AlertTriangle, Activity, Monitor,
    Wifi, WifiOff, ChevronRight, Target, Clock, Terminal, Zap, ShieldAlert,
    X, ExternalLink, Shield, Cpu, BarChart3
} from 'lucide-react';
import {
    PieChart, Pie, Cell, Tooltip, ResponsiveContainer
} from 'recharts';
import {
    statsApi, alertsApi, agentsApi, createAlertStream,
    type Alert, type AgentStats, type Agent, type AlertStats
} from '../api/client';
import { SkeletonKPICards } from '../components';
import StatCard from '../components/StatCard';
import ThreatMeter from '../components/ThreatMeter';
import LiveIndicator from '../components/LiveIndicator';
import InsightHero from '../components/InsightHero';

// ─── Constants ──────────────────────────────────────────────
const STALE_THRESHOLD_MS = 1 * 60 * 1000;



// ─── Threat Score Calculation ────────────────────────────────
function calcThreatScore(stats: AlertStats | undefined): number {
    if (!stats) return 0;
    const s = stats.by_severity || {};
    const raw = (s.critical || 0) * 20 + (s.high || 0) * 8 + (s.medium || 0) * 3 + (s.low || 0);
    return Math.min(100, Math.round((raw / Math.max(raw, 200)) * 100));
}

// ─── OS Donut Chart ──────────────────────────────────────────
const OS_COLORS: Record<string, string> = {
    windows: '#38bdf8',
    linux:   '#a855f7',
    macos:   '#10b981',
};
const OS_FALLBACK = '#64748b';

function OSDonut({ byOsType }: { byOsType: Record<string, number> }) {
    const data = Object.entries(byOsType)
        .filter(([, v]) => v > 0)
        .map(([k, v]) => ({ name: k.charAt(0).toUpperCase() + k.slice(1), value: v, key: k }));

    if (data.length === 0) {
        return (
            <div className="flex items-center justify-center h-32 text-slate-500 text-sm">
                No OS data available
            </div>
        );
    }

    const total = data.reduce((s, d) => s + d.value, 0);

    return (
        <div className="flex flex-col items-center gap-2">
            <ResponsiveContainer width="100%" height={130}>
                <PieChart>
                    <Pie
                        data={data}
                        dataKey="value"
                        nameKey="name"
                        cx="50%"
                        cy="50%"
                        innerRadius={38}
                        outerRadius={58}
                        paddingAngle={3}
                        strokeWidth={0}
                    >
                        {data.map((entry) => (
                            <Cell key={entry.key} fill={OS_COLORS[entry.key] || OS_FALLBACK} />
                        ))}
                    </Pie>
                    <Tooltip
                        contentStyle={{
                            background: 'rgba(15,23,42,0.95)',
                            border: '1px solid rgba(30,48,72,0.8)',
                            borderRadius: '10px',
                            color: 'white',
                            fontSize: '12px',
                            fontFamily: 'Inter, sans-serif',
                        }}
                        formatter={(v: number | undefined) => [`${v ?? 0} agents`, '']}
                    />
                </PieChart>
            </ResponsiveContainer>
            {/* Legend */}
            <div className="flex flex-wrap justify-center gap-3">
                {data.map(d => (
                    <div key={d.key} className="flex items-center gap-1.5 text-[11px] font-medium text-slate-500 dark:text-slate-400">
                        <span className="w-2.5 h-2.5 rounded-full shrink-0" style={{ background: OS_COLORS[d.key] || OS_FALLBACK }} />
                        {d.name} ({Math.round((d.value / total) * 100)}%)
                    </div>
                ))}
            </div>
        </div>
    );
}

// ─── MITRE Quick Stats Bar ───────────────────────────────────
function MitreQuickStats({ alerts, byTactic }: { alerts: Alert[]; byTactic?: Record<string, number> }) {
    const tacticCounts = useMemo(() => {
        // Prefer server-side byTactic if available; otherwise compute from liveAlerts
        if (byTactic && Object.keys(byTactic).length > 0) return byTactic;
        const counts: Record<string, number> = {};
        alerts.forEach(a => {
            a.mitre_tactics?.forEach(t => { counts[t] = (counts[t] || 0) + 1; });
        });
        return counts;
    }, [alerts, byTactic]);

    const top5 = useMemo(() =>
        Object.entries(tacticCounts)
            .sort(([, a], [, b]) => b - a)
            .slice(0, 5)
    , [tacticCounts]);

    if (top5.length === 0) {
        return (
            <div className="flex items-center justify-center h-20 text-slate-500 text-xs">
                No MITRE data available
            </div>
        );
    }

    const maxVal = Math.max(...top5.map(([, v]) => v), 1);

    return (
        <div className="space-y-2.5">
            {top5.map(([tactic, count]) => (
                <div key={tactic} className="flex items-center gap-3">
                    <span className="text-[11px] font-semibold text-slate-500 dark:text-slate-400 w-28 shrink-0 truncate" title={tactic}>
                        {tactic}
                    </span>
                    <div className="flex-1 h-2 bg-slate-200 dark:bg-slate-700 rounded-full overflow-hidden">
                        <div
                            className="h-full rounded-full transition-all duration-700"
                            style={{
                                width: `${(count / maxVal) * 100}%`,
                                background: 'linear-gradient(to right, #a855f7, #22d3ee)',
                            }}
                        />
                    </div>
                    <span className="text-[11px] font-bold text-slate-600 dark:text-slate-300 font-mono w-6 text-right shrink-0">
                        {count}
                    </span>
                </div>
            ))}
        </div>
    );
}

// ─── Alert Detail Drawer ─────────────────────────────────────
function AlertDrawer({ alert, agentMap, onClose }: {
    alert: Alert;
    agentMap: Record<string, string>;
    onClose: () => void;
}) {
    const severityColor: Record<string, string> = {
        critical: '#f43f5e',
        high:     '#f97316',
        medium:   '#f59e0b',
        low:      '#3b82f6',
        informational: '#22d3ee',
    };
    const color = severityColor[alert.severity] || '#64748b';
    const hostname = agentMap[alert.agent_id] || alert.agent_id?.slice(0, 12) + '...';

    return (
        <div
            className="fixed inset-0 z-50 flex"
            onClick={(e) => e.target === e.currentTarget && onClose()}
        >
            {/* Dim backdrop */}
            <div className="flex-1 bg-black/40 backdrop-blur-sm animate-fade-in" onClick={onClose} />

            {/* Drawer panel */}
            <div
                className="
                    w-full max-w-lg bg-white dark:bg-slate-900
                    border-l border-slate-200 dark:border-slate-700
                    shadow-2xl drawer-enter flex flex-col h-full
                "
                onClick={e => e.stopPropagation()}
            >
                {/* Header */}
                <div className="flex items-start justify-between p-5 border-b border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/80">
                    <div className="flex-1 min-w-0 pr-4">
                        <div
                            className="text-[10px] font-bold uppercase tracking-widest mb-2 px-2 py-0.5 rounded-full inline-block"
                            style={{ color, background: `${color}20`, border: `1px solid ${color}40` }}
                        >
                            {alert.severity}
                        </div>
                        <h2 className="text-base font-bold text-slate-900 dark:text-white leading-snug">
                            {alert.rule_title}
                        </h2>
                    </div>
                    <button
                        onClick={onClose}
                        className="p-1.5 rounded-lg text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-700 transition-all"
                    >
                        <X className="w-4 h-4" />
                    </button>
                </div>

                {/* Scrollable body */}
                <div className="flex-1 overflow-y-auto p-5 space-y-5">
                    {/* Meta grid */}
                    <div className="grid grid-cols-2 gap-3">
                        {[
                            { label: 'Endpoint', value: hostname },
                            { label: 'Status', value: alert.status },
                            { label: 'Category', value: alert.category || '—' },
                            { label: 'Events', value: String(alert.event_count || 1) },
                            { label: 'Risk Score', value: alert.risk_score != null ? String(alert.risk_score) : '—' },
                            { label: 'Confidence', value: alert.confidence != null ? `${((alert.confidence as number) * 100).toFixed(0)}%` : '—' },
                        ].map(({ label, value }) => (
                            <div key={label} className="bg-slate-100 dark:bg-slate-800 rounded-lg p-3">
                                <p className="text-[10px] text-slate-400 uppercase tracking-widest mb-0.5">{label}</p>
                                <p className="text-sm font-semibold text-slate-800 dark:text-slate-200 truncate font-mono">{value}</p>
                            </div>
                        ))}
                    </div>

                    {/* MITRE Tactics */}
                    {(alert.mitre_tactics?.length ?? 0) > 0 && (
                        <div>
                            <p className="text-[10px] text-slate-400 uppercase tracking-widest mb-2">MITRE Tactics</p>
                            <div className="flex flex-wrap gap-2">
                                {alert.mitre_tactics!.map(t => (
                                    <span key={t} className="badge badge-mitre">{t}</span>
                                ))}
                            </div>
                        </div>
                    )}

                    {/* MITRE Techniques */}
                    {(alert.mitre_techniques?.length ?? 0) > 0 && (
                        <div>
                            <p className="text-[10px] text-slate-400 uppercase tracking-widest mb-2">MITRE Techniques</p>
                            <div className="flex flex-wrap gap-2">
                                {alert.mitre_techniques!.map(t => (
                                    <span key={t} className="badge badge-mitre">{t}</span>
                                ))}
                            </div>
                        </div>
                    )}

                    {/* Timestamps */}
                    <div>
                        <p className="text-[10px] text-slate-400 uppercase tracking-widest mb-2">Timeline</p>
                        <div className="space-y-1.5">
                            <div className="flex items-center gap-2 text-xs font-mono text-slate-500">
                                <Clock className="w-3 h-3 shrink-0" />
                                <span className="text-slate-400">Detected:</span>
                                {new Date(alert.timestamp).toLocaleString()}
                            </div>
                            <div className="flex items-center gap-2 text-xs font-mono text-slate-500">
                                <Clock className="w-3 h-3 shrink-0" />
                                <span className="text-slate-400">Created:</span>
                                {new Date(alert.created_at).toLocaleString()}
                            </div>
                        </div>
                    </div>
                </div>

                {/* Footer actions */}
                <div className="border-t border-slate-200 dark:border-slate-700 p-4 flex gap-3 bg-slate-50 dark:bg-slate-800/80">
                    <Link
                        to={`/alerts?id=${alert.id}`}
                        className="btn btn-primary flex-1 justify-center text-sm"
                    >
                        <ExternalLink className="w-3.5 h-3.5" />
                        View in Alerts
                    </Link>
                    <button
                        onClick={onClose}
                        className="btn btn-secondary text-sm"
                    >
                        Close
                    </button>
                </div>
            </div>
        </div>
    );
}

// ─── Live Alerts Feed ────────────────────────────────────────
const AlertsFeed = React.memo(function AlertsFeed({
    alerts,
    agentMap,
    onAlertClick,
}: {
    alerts: Alert[];
    agentMap: Record<string, string>;
    onAlertClick: (a: Alert) => void;
}) {
    const severityConfig: Record<string, { color: string; border: string; bg: string; stripe: string }> = {
        critical:      { color: 'text-rose-400', border: 'border-rose-500/50', bg: 'bg-rose-500/10', stripe: 'border-l-4 border-l-rose-500' },
        high:          { color: 'text-orange-400', border: 'border-orange-500/50', bg: 'bg-orange-500/10', stripe: 'border-l-4 border-l-orange-500' },
        medium:        { color: 'text-amber-400', border: 'border-amber-500/50', bg: 'bg-amber-500/10', stripe: 'border-l-4 border-l-amber-500' },
        low:           { color: 'text-indigo-400', border: 'border-indigo-500/50', bg: 'bg-indigo-500/10', stripe: 'border-l-4 border-l-indigo-500' },
        informational: { color: 'text-cyan-400', border: 'border-cyan-500/50', bg: 'bg-cyan-500/10', stripe: 'border-l-4 border-l-cyan-500' },
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
        <div className="card border border-slate-200 dark:border-slate-700/60 shadow-lg bg-white dark:bg-slate-800/90 backdrop-blur-sm rounded-xl p-5 flex flex-col h-full min-h-[450px]">
            <div className="flex items-center justify-between mb-5 border-b border-slate-200 dark:border-slate-700/60 pb-4 shrink-0">
                <h3 className="text-sm font-bold text-slate-800 dark:text-slate-200 uppercase tracking-widest flex items-center gap-2">
                    <Activity className="w-4 h-4 text-cyan-500 dark:text-cyan-400" /> Live Alerts Stream
                </h3>
                <LiveIndicator label="Live" color="emerald" />
            </div>

            <div className="space-y-2 flex-1 overflow-y-auto pr-1">
                {alerts.length === 0 ? (
                    <div className="flex flex-col items-center justify-center h-full text-slate-500 min-h-[200px]">
                        <Zap className="w-6 h-6 opacity-40 mb-3" />
                        <span className="text-sm">Listening for signals...</span>
                    </div>
                ) : (
                    alerts.slice(0, 50).map((alert) => {
                        const style = severityConfig[alert.severity] || severityConfig.informational;
                        const hostname = agentMap[alert.agent_id] || alert.agent_id?.slice(0, 8);
                        return (
                            <div
                                key={alert.id}
                                onClick={() => onAlertClick(alert)}
                                className={`bg-slate-50/50 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-700/40 ${style.stripe} rounded-lg p-3.5 hover:bg-slate-100 dark:hover:bg-slate-700/60 transition-all cursor-pointer flex items-start gap-3 group`}
                            >
                                <div className={`mt-1 flex-shrink-0 w-2 h-2 rounded-full ${style.bg} ${style.border} border shadow-[0_0_6px_currentColor] ${style.color}`} />
                                <div className="flex-1 min-w-0">
                                    <div className="flex justify-between items-start mb-1 gap-2">
                                        <p className="font-semibold text-slate-800 dark:text-slate-200 truncate text-sm group-hover:text-cyan-700 dark:group-hover:text-cyan-400 transition-colors">
                                            {alert.rule_title}
                                        </p>
                                        <span className={`text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border ${style.bg} ${style.color} ${style.border} shrink-0`}>
                                            {alert.severity}
                                        </span>
                                    </div>
                                    <div className="flex items-center gap-3 mt-1.5">
                                        <span className="font-mono text-[10px] text-slate-500 dark:text-slate-400 bg-slate-100 dark:bg-slate-900/50 px-1.5 py-0.5 rounded truncate max-w-[120px]">
                                            {hostname}
                                        </span>
                                        {alert.mitre_tactics?.[0] && (
                                            <span className="text-[10px] text-purple-500 dark:text-purple-400 font-medium truncate max-w-[100px]">
                                                {alert.mitre_tactics[0]}
                                            </span>
                                        )}
                                        <div className="text-[10px] text-slate-500 flex items-center gap-1 shrink-0 ml-auto">
                                            <Clock className="w-3 h-3" /> {formatRel(alert.timestamp)}
                                        </div>
                                    </div>
                                </div>
                                <ChevronRight className="w-4 h-4 text-slate-400 opacity-0 group-hover:opacity-100 transition-opacity shrink-0" />
                            </div>
                        );
                    })
                )}
            </div>
        </div>
    );
});

// ─── Incident Queue ──────────────────────────────────────────
const ActiveIncidentQueue = React.memo(function ActiveIncidentQueue({
    alerts,
    agentMap
}: { alerts: Alert[]; agentMap: Record<string, string> }) {
    const activeThreats = alerts.filter(a =>
        (a.severity === 'critical' || a.severity === 'high') &&
        (a.status === 'open' || a.status === 'in_progress')
    ).slice(0, 8);

    return (
        <div className="card border border-slate-200 dark:border-slate-700/60 shadow-lg bg-white dark:bg-slate-800/90 backdrop-blur-sm rounded-xl p-5 flex flex-col flex-1 min-h-0">
            <div className="flex items-center justify-between mb-4 border-b border-slate-200 dark:border-slate-700/60 pb-3 shrink-0">
                <div className="flex items-center gap-2">
                    <Target className="w-4 h-4 text-rose-500 dark:text-rose-400" />
                    <h3 className="text-sm font-bold text-slate-800 dark:text-slate-200 uppercase tracking-widest">
                        Incident Queue
                    </h3>
                </div>
                <span className="text-[10px] font-bold text-rose-600 dark:text-rose-400 uppercase tracking-widest bg-rose-100 dark:bg-rose-400/10 px-2 py-1 rounded-md border border-rose-200 dark:border-rose-400/20">
                    {activeThreats.length} Active
                </span>
            </div>

            <div className="space-y-2 flex-1 overflow-y-auto pr-1 min-h-[120px]">
                {activeThreats.length === 0 ? (
                    <div className="flex flex-col items-center justify-center p-6 text-slate-500 h-full">
                        <ShieldAlert className="w-6 h-6 opacity-30 mb-2" />
                        <span className="text-xs">All clear — no pending incidents</span>
                    </div>
                ) : (
                    activeThreats.map((alert) => (
                        <div key={alert.id} className="group bg-slate-50/50 dark:bg-slate-800/40 hover:bg-slate-100 dark:hover:bg-slate-700/50 border-l-2 border-rose-500 border border-slate-200 dark:border-slate-700/50 rounded-lg p-3 transition-colors cursor-pointer">
                            <div className="flex justify-between items-start mb-1 gap-2">
                                <span className="font-mono text-[11px] font-semibold text-rose-600 dark:text-rose-400 truncate flex-1" title={alert.rule_title}>
                                    {alert.rule_title}
                                </span>
                                <span className="text-[10px] text-slate-500 shrink-0">
                                    {Math.floor((Date.now() - new Date(alert.timestamp).getTime()) / 60000)}m ago
                                </span>
                            </div>
                            <div className="flex justify-between items-center mt-1.5">
                                <span className="text-[10px] font-mono text-slate-500 dark:text-slate-400 truncate">
                                    {agentMap[alert.agent_id] || alert.agent_id?.slice(0, 10)}
                                </span>
                                <span className="text-xs font-medium text-cyan-600 dark:text-cyan-400 opacity-0 group-hover:opacity-100 transition-opacity">
                                    Triage →
                                </span>
                            </div>
                        </div>
                    ))
                )}
            </div>
        </div>
    );
});

// ─── Endpoints Pulse ─────────────────────────────────────────
const EndpointsPulse = React.memo(function EndpointsPulse({
    stats, agents, onClick
}: { stats: AgentStats | null; agents: Agent[]; onClick: () => void }) {
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
        online = stats.online; offline = stats.offline; degraded = stats.degraded;
    }
    const total = online + offline + degraded;

    return (
        <div
            className="card border border-slate-200 dark:border-slate-700/60 shadow-lg bg-white dark:bg-slate-800/90 backdrop-blur-sm rounded-xl p-5 flex flex-col cursor-pointer transition-all hover:-translate-y-0.5 hover:shadow-xl shrink-0"
            onClick={onClick}
        >
            <div className="flex items-center justify-between mb-4 border-b border-slate-200 dark:border-slate-700/60 pb-3 shrink-0">
                <div className="flex items-center gap-2">
                    <Monitor className="w-4 h-4 text-cyan-500 dark:text-cyan-400" />
                    <h3 className="text-sm font-bold text-slate-800 dark:text-slate-200 uppercase tracking-widest">Agent Pulse</h3>
                </div>
            </div>

            <div className="flex justify-around items-end pb-2">
                <div className="flex flex-col items-center gap-1">
                    <span className="text-2xl font-bold text-emerald-400 font-mono">{online}</span>
                    <span className="text-[10px] font-bold uppercase tracking-widest text-slate-500 flex items-center gap-1"><Wifi className="w-3 h-3" />Online</span>
                </div>
                <div className="flex flex-col items-center gap-1">
                    <span className="text-2xl font-bold text-amber-400 font-mono">{degraded}</span>
                    <span className="text-[10px] font-bold uppercase tracking-widest text-slate-500 flex items-center gap-1"><AlertTriangle className="w-3 h-3" />Warn</span>
                </div>
                <div className="flex flex-col items-center gap-1">
                    <span className="text-2xl font-bold text-slate-500 font-mono">{offline}</span>
                    <span className="text-[10px] font-bold uppercase tracking-widest text-slate-500 flex items-center gap-1"><WifiOff className="w-3 h-3" />Offline</span>
                </div>
            </div>

            <div className="mt-4 pt-4 border-t border-slate-200 dark:border-slate-800 flex flex-col gap-1.5">
                <div className="flex gap-0.5 h-1.5 rounded-full overflow-hidden bg-slate-200 dark:bg-slate-800">
                    <div className="bg-emerald-500 transition-all duration-700" style={{ width: `${total > 0 ? (online / total) * 100 : 0}%` }} />
                    <div className="bg-amber-500 transition-all duration-700" style={{ width: `${total > 0 ? (degraded / total) * 100 : 0}%` }} />
                    <div className="bg-slate-400 dark:bg-slate-600 transition-all duration-700" style={{ width: `${total > 0 ? (offline / total) * 100 : 0}%` }} />
                </div>
                <div className="text-[10px] text-slate-500 dark:text-slate-400 font-mono text-right">{total} TOTAL AGENTS</div>
            </div>
        </div>
    );
});

// ─── System Action Log ───────────────────────────────────────
const SystemActionLog = React.memo(function SystemActionLog({ alerts, agents }: { alerts: Alert[]; agents: Agent[] }) {
    const logEntries = useMemo(() => [
        ...alerts.map(a => ({
            id: `a-${a.id}`,
            timestamp: new Date(a.timestamp).getTime(),
            timeStr: new Date(a.timestamp).toLocaleTimeString('en-US', { hour12: false }),
            type: 'THREAT',
            message: `[DETECT] ${a.severity.toUpperCase()} on ${a.agent_id?.slice(0, 8)}: ${a.rule_title}`,
            color: a.severity === 'critical' ? 'text-rose-500 dark:text-rose-400' : 'text-amber-500 dark:text-amber-400',
        })),
        ...agents.map(ag => ({
            id: `ag-${ag.id}`,
            timestamp: new Date(ag.last_seen).getTime(),
            timeStr: new Date(ag.last_seen).toLocaleTimeString('en-US', { hour12: false }),
            type: 'SYSTEM',
            message: `[CHECK-IN] Agent ${ag.hostname} reported status: ${ag.status}`,
            color: 'text-cyan-500 dark:text-cyan-400',
        })),
    ].sort((a, b) => b.timestamp - a.timestamp).slice(0, 40), [alerts, agents]);

    return (
        <div className="card border border-slate-200 dark:border-slate-700/60 shadow-lg bg-white dark:bg-slate-800/90 backdrop-blur-sm rounded-xl overflow-hidden flex flex-col h-[260px] font-mono w-full">
            <div className="bg-slate-50 dark:bg-slate-950/50 px-4 py-3 border-b border-slate-200 dark:border-slate-700/60 flex items-center justify-between shrink-0">
                <div className="flex items-center gap-2">
                    <Terminal className="w-4 h-4 text-slate-500 dark:text-slate-400" />
                    <span className="text-[11px] text-slate-600 dark:text-slate-300 font-bold tracking-widest uppercase">System Action Log</span>
                </div>
                <div className="flex gap-1.5">
                    <div className="w-2.5 h-2.5 rounded-full bg-rose-400" />
                    <div className="w-2.5 h-2.5 rounded-full bg-amber-400" />
                    <div className="w-2.5 h-2.5 rounded-full bg-emerald-400" />
                </div>
            </div>
            <div className="p-4 space-y-1 overflow-y-auto text-[11px] leading-relaxed flex-1">
                {logEntries.length === 0 ? (
                    <div className="text-slate-600 italic">Awaiting terminal stream...</div>
                ) : (
                    logEntries.map((entry) => (
                        <div key={entry.id} className="flex gap-3 hover:bg-slate-100 dark:hover:bg-slate-800/80 px-2 py-0.5 -mx-2 rounded transition-colors">
                            <span className="text-slate-500 shrink-0">{entry.timeStr}</span>
                            <span className={`${entry.color} truncate`} title={entry.message}>{entry.message}</span>
                        </div>
                    ))
                )}
            </div>
        </div>
    );
});

// ─── Main Dashboard ──────────────────────────────────────────
export default function Dashboard() {
    const navigate = useNavigate();
    const queryClient = useQueryClient();
    const [liveAlerts, setLiveAlerts] = useState<Alert[]>([]);
    const [drawerAlert, setDrawerAlert] = useState<Alert | null>(null);
    const statsInvalidateTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

    const { data: alertStats, isLoading: statsLoading } = useQuery({
        queryKey: ['alertStats'],
        queryFn: statsApi.alerts,
        refetchInterval: 1000,
    });

    const { data: agentStats } = useQuery({
        queryKey: ['agentStats'],
        queryFn: agentsApi.stats,
        retry: false,
        refetchInterval: 5000,
    });

    const { data: agentListData } = useQuery({
        queryKey: ['agents'],
        queryFn: () => agentsApi.list({ limit: 200 }),
        retry: false,
        refetchInterval: 10000,
    });

    const { data: recentAlerts } = useQuery({
        queryKey: ['recentAlerts'],
        queryFn: () => alertsApi.list({ limit: 100 }),
        // Fallback polling in case WebSocket stream is unavailable (proxy/ws misconfig).
        // Keep this reasonably low so the dashboard still feels "near real-time".
        refetchInterval: 1000,
    });

    // ── Sparkline data from timeline (7 points) ──────────────
    const { data: timelineData } = useQuery({
        queryKey: ['dashboardTimeline'],
        queryFn: () => {
            const to = new Date().toISOString();
            const from = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString();
            return statsApi.timeline({ from, to, granularity: '1d' });
        },
        refetchInterval: 60000,
    });

    // ── Agent hostname lookup map ─────────────────────────────
    const agentMap = useMemo<Record<string, string>>(() => {
        const map: Record<string, string> = {};
        (agentListData?.data || []).forEach(a => { map[a.id] = a.hostname; });
        return map;
    }, [agentListData]);

    // ── Threat score ─────────────────────────────────────────
    const threatScore = useMemo(() => calcThreatScore(alertStats), [alertStats]);

    // ── Sparklines (7 data points per KPI) ───────────────────
    const sparklines = useMemo(() => {
        const pts = timelineData?.data || [];
        const critical = pts.map(p => p.critical);
        const high     = pts.map(p => p.high);
        const total    = pts.map(p => p.critical + p.high + p.medium + p.low + p.informational);
        return { critical, high, total };
    }, [timelineData]);

    useEffect(() => {
        if (recentAlerts?.alerts) setLiveAlerts(recentAlerts.alerts);
    }, [recentAlerts]);

    useEffect(() => {
        const stream = createAlertStream((alert) => {
            setLiveAlerts((prev) => [alert, ...prev.slice(0, 99)]);

            // ── Real-time KPI invalidation ─────────────────────────
            // Debounce: batch rapid WebSocket arrivals into a single
            // stats refetch (150ms window) to avoid API hammering.
            if (statsInvalidateTimer.current) {
                clearTimeout(statsInvalidateTimer.current);
            }
            statsInvalidateTimer.current = setTimeout(() => {
                queryClient.invalidateQueries({ queryKey: ['alertStats'] });
                queryClient.invalidateQueries({ queryKey: ['recentAlerts'] });
            }, 150);
        }, { severity: ['critical', 'high', 'medium', 'low'] });
        return () => {
            stream.close();
            if (statsInvalidateTimer.current) {
                clearTimeout(statsInvalidateTimer.current);
            }
        };
    }, [queryClient]);

    const handleAlertClick = useCallback((alert: Alert) => setDrawerAlert(alert), []);
    const handleCloseDrawer = useCallback(() => setDrawerAlert(null), []);

    // Set document title
    useEffect(() => { document.title = 'Security Posture — EDR Platform'; }, []);

    if (statsLoading) {
        return (
            <div className="space-y-6 w-full min-w-0">
                <div className="rounded-2xl border border-slate-200/80 dark:border-slate-700/60 bg-gradient-to-br from-slate-100 to-slate-50 dark:from-slate-800 dark:to-slate-900 h-36 sm:h-40 animate-pulse" aria-hidden />
                <SkeletonKPICards count={4} />
                <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
                    <div className="lg:col-span-2 h-96 bg-slate-200 dark:bg-slate-800 rounded-xl animate-pulse" />
                    <div className="lg:col-span-1 h-96 bg-slate-200 dark:bg-slate-800 rounded-xl animate-pulse" />
                </div>
            </div>
        );
    }

    const byOs = agentStats?.by_os_type || {};

    return (
        <div className="space-y-6 pb-8 w-full min-w-0">
            <InsightHero
                variant="light"
                accent="cyan"
                icon={BarChart3}
                eyebrow="Dashboards"
                title="Security Posture"
                segments={[
                    {
                        heading: 'What this screen is',
                        children: (
                            <>
                                Executive snapshot of <strong className="font-semibold text-slate-800 dark:text-slate-200">alert pressure</strong>,{' '}
                                <strong className="font-semibold text-slate-800 dark:text-slate-200">fleet connectivity</strong>, and{' '}
                                <strong className="font-semibold text-slate-800 dark:text-slate-200">detection confidence</strong> — fed by Sigma statistics and connection-manager agent
                                APIs. Use it for at-a-glance posture before drilling into operational grids.
                            </>
                        ),
                    },
                    {
                        heading: 'Live behaviour',
                        children: (
                            <>
                                Recent alerts update via stream when available, with polling fallback. KPI cards and charts refresh on a short interval — suitable for NOC-style
                                monitoring, not long-form investigation by itself.
                            </>
                        ),
                    },
                    {
                        heading: 'Where to go deeper',
                        children: (
                            <>
                                Full triage: <Link className="text-cyan-600 dark:text-cyan-400 font-semibold hover:underline" to="/alerts">Alerts</Link>
                                {' · '}
                                Fleet ops: <Link className="text-cyan-600 dark:text-cyan-400 font-semibold hover:underline" to="/management/devices">Devices</Link>
                                {' · '}
                                Alternative summary:{' '}
                                <Link className="text-cyan-600 dark:text-cyan-400 font-semibold hover:underline" to="/dashboards/endpoint">
                                    Endpoint Summary
                                </Link>
                                .
                            </>
                        ),
                    },
                ]}
            />

            {/* ── Row 1: KPI Cards ── */}
            <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4 animate-slide-up-fade">
                <StatCard
                    title="Alerts (24h)"
                    value={alertStats?.last_24h || 0}
                    icon={AlertTriangle}
                    color="amber"
                    sparkline={sparklines.total}
                    subtext={`${alertStats?.last_7d || 0} in 7 days · ${alertStats?.by_status?.open || 0} open`}
                    onClick={() => navigate('/alerts')}
                />
                <StatCard
                    title="Critical Threats"
                    value={alertStats?.by_severity?.critical || 0}
                    icon={ShieldAlert}
                    color="red"
                    sparkline={sparklines.critical}
                    subtext="Requires immediate triage"
                    onClick={() => navigate('/alerts?severity=critical')}
                />
                <StatCard
                    title="Active Agents"
                    value={agentStats?.online || 0}
                    icon={Monitor}
                    color="emerald"
                    subtext={`Avg health ${Math.round(agentStats?.avg_health || 0)}%`}
                    onClick={() => navigate('/management/devices')}
                />
                <StatCard
                    title="Detection Engine"
                    value={`${((alertStats?.avg_confidence || 0) * 100).toFixed(1)}%`}
                    icon={Activity}
                    color="cyan"
                    subtext="Average rule confidence"
                />
            </div>

            {/* ── Row 2: Threat Meter + MITRE Bar + OS Donut ── */}
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                {/* Threat Meter */}
                <div className="card border border-slate-200 dark:border-slate-700/60 bg-white dark:bg-slate-800/90 rounded-xl p-5 flex flex-col items-center justify-center gap-2 animate-slide-up-fade animate-delay-100">
                    <h3 className="text-xs font-bold uppercase tracking-widest text-slate-400 mb-2 self-start flex items-center gap-2">
                        <Shield className="w-3.5 h-3.5 text-cyan-500" /> Threat Level
                    </h3>
                    <ThreatMeter score={threatScore} size={150} />
                </div>

                {/* MITRE Quick Stats */}
                <div className="card border border-slate-200 dark:border-slate-700/60 bg-white dark:bg-slate-800/90 rounded-xl p-5 animate-slide-up-fade animate-delay-200">
                    <h3 className="text-xs font-bold uppercase tracking-widest text-slate-400 mb-4 flex items-center gap-2">
                        <Cpu className="w-3.5 h-3.5 text-purple-500" /> MITRE ATT&amp;CK — Top Tactics
                    </h3>
                    <MitreQuickStats alerts={liveAlerts} byTactic={alertStats?.by_tactic} />
                </div>

                {/* OS Distribution */}
                <div className="card border border-slate-200 dark:border-slate-700/60 bg-white dark:bg-slate-800/90 rounded-xl p-5 animate-slide-up-fade animate-delay-300">
                    <h3 className="text-xs font-bold uppercase tracking-widest text-slate-400 mb-4 flex items-center gap-2">
                        <BarChart3 className="w-3.5 h-3.5 text-sky-500" /> OS Distribution
                    </h3>
                    <OSDonut byOsType={byOs} />
                </div>
            </div>

            {/* ── Row 3: Alerts Feed + Incident Queue + Agent Pulse ── */}
            <div className="grid grid-cols-1 xl:grid-cols-12 gap-4 w-full">
                {/* Main: Alerts Feed (spans 8) */}
                <div className="xl:col-span-8 flex flex-col h-[520px] min-h-0">
                    <AlertsFeed alerts={liveAlerts} agentMap={agentMap} onAlertClick={handleAlertClick} />
                </div>

                {/* Side column (spans 4) */}
                <div className="xl:col-span-4 flex flex-col gap-4 h-[520px] min-h-0">
                    <div className="flex-1 flex flex-col min-h-0">
                        <ActiveIncidentQueue alerts={liveAlerts} agentMap={agentMap} />
                    </div>
                    <div className="shrink-0">
                        <EndpointsPulse
                            stats={agentStats || null}
                            agents={agentListData?.data || []}
                            onClick={() => navigate('/management/devices')}
                        />
                    </div>
                </div>
            </div>

            {/* ── Row 4: System Action Log ── */}
            <div className="w-full shrink-0">
                <SystemActionLog alerts={liveAlerts} agents={agentListData?.data || []} />
            </div>

            {/* ── Alert Detail Drawer ── */}
            {drawerAlert && (
                <AlertDrawer
                    alert={drawerAlert}
                    agentMap={agentMap}
                    onClose={handleCloseDrawer}
                />
            )}
        </div>
    );
}

