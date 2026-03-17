import { useQuery } from '@tanstack/react-query';
import React, { useState, useMemo } from 'react';
import { Shield, Target, AlertTriangle, TrendingUp, Eye, ChevronRight, ExternalLink } from 'lucide-react';
import { alertsApi, type Alert } from '../api/client';
import { SkeletonKPICards, SkeletonChart } from '../components';
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Cell } from 'recharts';

// MITRE ATT&CK Tactics
const MITRE_TACTICS = [
    { id: 'TA0043', name: 'Reconnaissance', shortName: 'Recon' },
    { id: 'TA0042', name: 'Resource Development', shortName: 'Resource' },
    { id: 'TA0001', name: 'Initial Access', shortName: 'Initial' },
    { id: 'TA0002', name: 'Execution', shortName: 'Exec' },
    { id: 'TA0003', name: 'Persistence', shortName: 'Persist' },
    { id: 'TA0004', name: 'Privilege Escalation', shortName: 'PrivEsc' },
    { id: 'TA0005', name: 'Defense Evasion', shortName: 'DefEvas' },
    { id: 'TA0006', name: 'Credential Access', shortName: 'Cred' },
    { id: 'TA0007', name: 'Discovery', shortName: 'Discov' },
    { id: 'TA0008', name: 'Lateral Movement', shortName: 'Lateral' },
    { id: 'TA0009', name: 'Collection', shortName: 'Collect' },
    { id: 'TA0011', name: 'Command and Control', shortName: 'C2' },
    { id: 'TA0010', name: 'Exfiltration', shortName: 'Exfil' },
    { id: 'TA0040', name: 'Impact', shortName: 'Impact' },
];

// Color scale for heatmap
const getHeatmapColor = (count: number, maxCount: number) => {
    if (count === 0) return 'transparent';
    const intensity = Math.min(1, count / Math.max(maxCount * 0.8, 1));
    if (intensity < 0.25) return 'rgba(59, 130, 246, 0.3)'; // Light blue
    if (intensity < 0.5) return 'rgba(234, 179, 8, 0.5)'; // Yellow
    if (intensity < 0.75) return 'rgba(249, 115, 22, 0.7)'; // Orange
    return 'rgba(239, 68, 68, 0.9)'; // Red
};

// Threat Summary Card
const ThreatSummaryCard = React.memo(function ThreatSummaryCard({
    title,
    value,
    icon: Icon,
    colorTheme,
    subtitle
}: {
    title: string;
    value: string | number;
    icon: typeof Shield;
    colorTheme: { bg: string; text: string; glow: string };
    subtitle?: string;
}) {
    return (
        <div className="relative overflow-hidden bg-white/60 dark:bg-slate-900/40 backdrop-blur-md rounded-xl border border-slate-200/80 dark:border-slate-700/50 p-5 shadow-sm transition-all hover:shadow-md group">
            <div className={`absolute -top-10 -right-10 w-32 h-32 rounded-full blur-3xl opacity-20 pointer-events-none transition-opacity group-hover:opacity-40 ${colorTheme.glow}`} />
            <div className="flex items-center gap-4 relative z-10">
                <div className={`p-3 rounded-lg ${colorTheme.bg} ${colorTheme.text}`}>
                    <Icon className="w-6 h-6" />
                </div>
                <div>
                    <div className="text-sm font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider mb-1">{title}</div>
                    <div className="text-2xl font-bold text-slate-900 dark:text-white">{value}</div>
                    {subtitle && <div className="text-xs text-slate-500 dark:text-slate-400 mt-1">{subtitle}</div>}
                </div>
            </div>
        </div>
    );
});

// MITRE Matrix Heatmap
const MitreMatrixHeatmap = React.memo(function MitreMatrixHeatmap({
    tacticCounts,
    onTacticClick
}: {
    tacticCounts: Record<string, number>;
    onTacticClick: (tactic: string) => void;
}) {
    const maxCount = Math.max(...Object.values(tacticCounts), 1);

    return (
        <div className="relative bg-white/60 dark:bg-slate-900/40 backdrop-blur-md border border-slate-200/80 dark:border-slate-700/50 rounded-xl p-6 shadow-sm flex flex-col mb-6 mt-6">
            <div className="flex items-center justify-between mb-6">
                <h3 className="text-lg font-bold text-slate-900 dark:text-white">
                    MITRE ATT&CK Matrix
                </h3>
                <a
                    href="https://attack.mitre.org/"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-sm font-medium text-indigo-600 dark:text-indigo-400 hover:text-indigo-700 dark:hover:text-indigo-300 flex items-center gap-1.5 transition-colors bg-indigo-50 dark:bg-indigo-500/10 px-3 py-1.5 rounded-lg border border-indigo-200 dark:border-indigo-500/20"
                >
                    View Framework <ExternalLink className="w-3.5 h-3.5" />
                </a>
            </div>

            <div className="grid grid-cols-2 sm:grid-cols-4 md:grid-cols-7 gap-3">
                {MITRE_TACTICS.map((tactic) => {
                    const matchingKey = Object.keys(tacticCounts).find(k => 
                        k.toLowerCase() === tactic.name.toLowerCase() || 
                        k.toLowerCase() === tactic.id.toLowerCase()
                    );
                    const count = matchingKey ? tacticCounts[matchingKey] : 0;
                    const bgColor = getHeatmapColor(count, maxCount);

                    return (
                        <button
                            key={tactic.id}
                            onClick={() => onTacticClick(tactic.name)}
                            className={`relative p-4 rounded-xl border transition-all duration-300 ${count > 0
                                    ? 'hover:-translate-y-1 hover:shadow-lg cursor-pointer border-transparent'
                                    : 'border-slate-200 dark:border-slate-700/50 cursor-default opacity-60 hover:opacity-100'
                                }`}
                            style={{ backgroundColor: bgColor || 'rgba(148, 163, 184, 0.05)' }}
                            disabled={count === 0}
                        >
                            <div className="text-center">
                                <p className={`text-xs font-semibold uppercase tracking-wider mb-1 ${count > 0 ? 'text-slate-900 dark:text-white' : 'text-slate-500 dark:text-slate-400'}`}>
                                    {tactic.shortName}
                                </p>
                                <p className={`text-2xl font-bold font-mono ${count > 0 ? 'text-slate-900 dark:text-white' : 'text-slate-400 dark:text-slate-600'}`}>
                                    {count}
                                </p>
                            </div>
                            {count > maxCount * 0.75 && (
                                <div className="absolute top-2 right-2 w-2 h-2 bg-rose-500 rounded-full shadow-[0_0_8px_rgba(244,63,94,0.8)] animate-pulse" />
                            )}
                        </button>
                    );
                })}
            </div>

            {/* Legend */}
            <div className="flex items-center justify-center gap-6 mt-8 pt-6 border-t border-slate-200 dark:border-slate-700/50">
                <div className="flex items-center gap-3">
                    <span className="text-xs font-semibold text-slate-500 uppercase tracking-wider">Heatmap Intensity:</span>
                    <div className="flex flex-wrap items-center gap-4 text-xs font-medium text-slate-600 dark:text-slate-400">
                        <div className="flex items-center gap-1.5"><div className="w-4 h-4 rounded-md shadow-sm" style={{ backgroundColor: 'rgba(59, 130, 246, 0.3)' }} /> Low</div>
                        <div className="flex items-center gap-1.5"><div className="w-4 h-4 rounded-md shadow-sm" style={{ backgroundColor: 'rgba(234, 179, 8, 0.5)' }} /> Guarded</div>
                        <div className="flex items-center gap-1.5"><div className="w-4 h-4 rounded-md shadow-sm" style={{ backgroundColor: 'rgba(249, 115, 22, 0.7)' }} /> Elevated</div>
                        <div className="flex items-center gap-1.5"><div className="w-4 h-4 rounded-md shadow-sm" style={{ backgroundColor: 'rgba(239, 68, 68, 0.9)' }} /> Critical</div>
                    </div>
                </div>
            </div>
        </div>
    );
});

// Top Techniques Chart
const TopTechniquesChart = React.memo(function TopTechniquesChart({ techniques }: { techniques: { name: string; count: number }[] }) {
    const COLORS = ['#ef4444', '#f97316', '#eab308', '#22c55e', '#3b82f6'];

    return (
        <div className="relative bg-white/60 dark:bg-slate-900/40 backdrop-blur-md border border-slate-200/80 dark:border-slate-700/50 rounded-xl p-6 shadow-sm flex flex-col h-[400px]">
            <h3 className="text-lg font-bold text-slate-900 dark:text-white mb-6">
                Top Techniques Detected
            </h3>
            {techniques.length === 0 ? (
                <div className="flex-1 flex items-center justify-center text-slate-500 dark:text-slate-400 text-sm">
                    No technique data available
                </div>
            ) : (
                <div className="flex-1 min-h-0">
                    <ResponsiveContainer width="100%" height="100%">
                        <BarChart data={techniques} layout="vertical" margin={{ top: 0, right: 0, left: 10, bottom: 0 }}>
                            <XAxis type="number" tick={{ fontSize: 11, fill: '#94a3b8' }} axisLine={false} tickLine={false} />
                            <YAxis
                                dataKey="name"
                                type="category"
                                width={180}
                                tick={{ fontSize: 11, fill: '#94a3b8', fontWeight: 500 }}
                                axisLine={false}
                                tickLine={false}
                                tickFormatter={(value) => value.length > 25 ? value.slice(0, 22) + '...' : value}
                            />
                            <Tooltip
                                cursor={{ fill: 'rgba(148, 163, 184, 0.1)' }}
                                contentStyle={{
                                    backgroundColor: 'rgba(15, 23, 42, 0.9)',
                                    backdropFilter: 'blur(8px)',
                                    border: '1px solid rgba(51, 65, 85, 0.8)',
                                    borderRadius: '12px',
                                    color: 'white',
                                    boxShadow: '0 10px 15px -3px rgba(0, 0, 0, 0.5)'
                                }}
                            />
                            <Bar dataKey="count" radius={[0, 6, 6, 0]} barSize={24}>
                                {techniques.map((_, index) => (
                                    <Cell key={`cell-${index}`} fill={COLORS[index % COLORS.length]} />
                                ))}
                            </Bar>
                        </BarChart>
                    </ResponsiveContainer>
                </div>
            )}
        </div>
    );
});

// Related Alerts List
const RelatedAlertsList = React.memo(function RelatedAlertsList({
    alerts,
    selectedTactic
}: {
    alerts: Alert[];
    selectedTactic: string | null;
}) {
    const filteredAlerts = selectedTactic
        ? alerts.filter(a => a.mitre_tactics?.some(t => t.toLowerCase().includes(selectedTactic.toLowerCase())))
        : alerts.slice(0, 10);

    return (
        <div className="relative bg-white/60 dark:bg-slate-900/40 backdrop-blur-md border border-slate-200/80 dark:border-slate-700/50 rounded-xl shadow-sm flex flex-col h-[400px] overflow-hidden">
            <div className="p-6 pb-4 border-b border-slate-200 dark:border-slate-700/50 flex items-center justify-between shrink-0 bg-slate-50/50 dark:bg-slate-800/30">
                <h3 className="text-lg font-bold text-slate-900 dark:text-white">
                    {selectedTactic ? `Alerts: ${selectedTactic}` : 'Recent Threat Alerts'}
                </h3>
                {selectedTactic && (
                    <span className="px-3 py-1 bg-indigo-500/10 text-indigo-600 dark:text-indigo-400 border border-indigo-500/20 rounded-full text-xs font-bold uppercase tracking-wider">
                        {filteredAlerts.length} Matches
                    </span>
                )}
            </div>

            <div className="flex-1 overflow-auto custom-scrollbar p-2">
                {filteredAlerts.length === 0 ? (
                    <div className="h-full flex items-center justify-center text-slate-500 dark:text-slate-400 text-sm">
                        {selectedTactic ? 'No alerts for this tactic' : 'No threat alerts'}
                    </div>
                ) : (
                    <div className="space-y-2">
                        {filteredAlerts.map((alert) => (
                            <div
                                key={alert.id}
                                className="group flex items-center justify-between p-4 bg-white dark:bg-slate-800/60 rounded-xl border border-slate-200 dark:border-slate-700 hover:border-slate-300 dark:hover:border-slate-600 hover:shadow-md cursor-pointer transition-all"
                            >
                                <div className="flex items-start gap-4">
                                    <div className={`p-2.5 rounded-lg shrink-0 mt-0.5 ${
                                        alert.severity === 'critical' ? 'bg-rose-500/10 text-rose-500 border border-rose-500/20' :
                                        alert.severity === 'high' ? 'bg-orange-500/10 text-orange-500 border border-orange-500/20' :
                                        'bg-amber-500/10 text-amber-500 border border-amber-500/20'
                                    }`}>
                                        <AlertTriangle className="w-5 h-5" />
                                    </div>
                                    <div>
                                        <p className="text-sm font-semibold text-slate-900 dark:text-slate-100 group-hover:text-indigo-600 dark:group-hover:text-indigo-400 transition-colors">
                                            {alert.rule_title}
                                        </p>
                                        <div className="flex flex-wrap items-center gap-2 mt-2">
                                            {alert.mitre_techniques?.slice(0, 3).map((tech) => (
                                                <span key={tech} className="px-2 py-0.5 rounded-md bg-slate-100 dark:bg-slate-900 text-slate-600 dark:text-slate-400 text-xs font-mono font-medium border border-slate-200 dark:border-slate-700">
                                                    {tech}
                                                </span>
                                            ))}
                                            {(alert.mitre_techniques?.length || 0) > 3 && (
                                                <span className="text-xs text-slate-500 dark:text-slate-400 font-medium">
                                                    +{alert.mitre_techniques!.length - 3} more
                                                </span>
                                            )}
                                        </div>
                                    </div>
                                </div>
                                <div className="shrink-0 p-2 rounded-full group-hover:bg-slate-100 dark:group-hover:bg-slate-700 transition-colors hidden sm:block">
                                    <ChevronRight className="w-4 h-4 text-slate-400 group-hover:text-slate-600 dark:group-hover:text-slate-300" />
                                </div>
                            </div>
                        ))}
                    </div>
                )}
            </div>
        </div>
    );
});

// Main Threats Page
export default function Threats() {
    const [selectedTactic, setSelectedTactic] = useState<string | null>(null);

    // Fetch alerts with MITRE data
    const { data, isLoading } = useQuery({
        queryKey: ['alertsWithMitre'],
        queryFn: () => alertsApi.list({ limit: 100 }),
    });

    const alerts = (data?.alerts || []).filter(a =>
        (a.mitre_tactics?.length ?? 0) > 0 || (a.mitre_techniques?.length ?? 0) > 0
    );

    // Calculate tactic counts
    const tacticCounts = useMemo(() => {
        const counts: Record<string, number> = {};
        alerts.forEach((alert) => {
            alert.mitre_tactics?.forEach((tactic) => {
                counts[tactic] = (counts[tactic] || 0) + 1;
            });
        });
        return counts;
    }, [alerts]);

    // Calculate technique counts
    const { techniqueCounts, topTechniques } = useMemo(() => {
        const tc: Record<string, number> = {};
        alerts.forEach((alert) => {
            alert.mitre_techniques?.forEach((tech) => {
                tc[tech] = (tc[tech] || 0) + 1;
            });
        });
        const top = Object.entries(tc)
            .sort(([, a], [, b]) => b - a)
            .slice(0, 8)
            .map(([name, count]) => ({ name, count }));
        return { techniqueCounts: tc, topTechniques: top };
    }, [alerts]);

    // Calculate summary stats
    const { totalTactics, mostCommonTactic, totalTechniques, threatLevel } = useMemo(() => ({
        totalTactics: Object.keys(tacticCounts).length,
        mostCommonTactic: Object.entries(tacticCounts).sort(([, a], [, b]) => b - a)[0],
        totalTechniques: Object.keys(techniqueCounts).length,
        threatLevel: alerts.filter(a => a.severity === 'critical' || a.severity === 'high').length,
    }), [alerts, tacticCounts, techniqueCounts]);

    if (isLoading) {
        return (
            <div className="space-y-6">
                <div className="h-9 w-64 bg-gray-200 dark:bg-gray-700 rounded animate-pulse" />
                <SkeletonKPICards count={4} />
                <SkeletonChart height={300} />
            </div>
        );
    }

    return (
        <div className="relative flex flex-col min-h-[calc(100vh-5rem)] lg:min-h-[calc(100vh-3.5rem)] -mx-4 sm:-mx-6 lg:-mx-8 -my-4 sm:-my-6 lg:-my-8 p-4 sm:p-6 lg:p-8 bg-slate-50 dark:bg-gradient-to-br dark:from-slate-900 dark:via-[#0b1120] dark:to-slate-900 transition-colors">
            {/* Ambient Glow */}
            <div className="absolute top-0 right-0 w-[600px] h-[600px] pointer-events-none mix-blend-screen" style={{ background: 'radial-gradient(circle, rgba(244,63,94,0.05) 0%, transparent 70%)' }} />
            <div className="absolute bottom-0 left-0 w-[600px] h-[600px] pointer-events-none mix-blend-screen" style={{ background: 'radial-gradient(circle, rgba(99,102,241,0.05) 0%, transparent 70%)' }} />

            <div className="relative max-w-[1600px] mx-auto w-full space-y-6">
                <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                    <div>
                        <h1 className="text-2xl font-bold bg-clip-text text-transparent bg-gradient-to-r from-gray-900 to-gray-600 dark:from-white dark:to-gray-300">
                            Threat Intelligence
                        </h1>
                        <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">Analytics and MITRE ATT&CK visualization</p>
                    </div>
                    
                    <button
                        className="flex items-center gap-2 px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white text-sm font-medium rounded-lg transition-colors shadow-sm shadow-indigo-500/20"
                        onClick={() => window.open('https://attack.mitre.org/', '_blank')}
                    >
                        <Shield className="w-4 h-4" />
                        Explore Framework
                        <ExternalLink className="w-3.5 h-3.5 ml-1 opacity-70" />
                    </button>
                </div>

                {/* Summary Cards */}
                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
                    <ThreatSummaryCard
                        title="Tactics Triggered"
                        value={totalTactics}
                        icon={Target}
                        colorTheme={{ bg: 'bg-rose-500/10 dark:bg-rose-500/20', text: 'text-rose-600 dark:text-rose-400', glow: 'bg-rose-500' }}
                        subtitle={`across ${MITRE_TACTICS.length} stages`}
                    />
                    <ThreatSummaryCard
                        title="Top Attacker Goal"
                        value={mostCommonTactic ? mostCommonTactic[0] : 'None'}
                        icon={Shield}
                        colorTheme={{ bg: 'bg-orange-500/10 dark:bg-orange-500/20', text: 'text-orange-600 dark:text-orange-400', glow: 'bg-orange-500' }}
                        subtitle={mostCommonTactic ? `${mostCommonTactic[1]} alert mappings` : 'No vectors identified'}
                    />
                    <ThreatSummaryCard
                        title="Techniques Matched"
                        value={totalTechniques}
                        icon={Eye}
                        colorTheme={{ bg: 'bg-blue-500/10 dark:bg-blue-500/20', text: 'text-blue-600 dark:text-blue-400', glow: 'bg-blue-500' }}
                        subtitle="Distinct behavioral signatures"
                    />
                    <ThreatSummaryCard
                        title="Critical Posture"
                        value={threatLevel}
                        icon={TrendingUp}
                        colorTheme={
                            threatLevel > 10
                                ? { bg: 'bg-rose-500/10 dark:bg-rose-500/20', text: 'text-rose-600 dark:text-rose-400', glow: 'bg-rose-500' }
                                : { bg: 'bg-emerald-500/10 dark:bg-emerald-500/20', text: 'text-emerald-600 dark:text-emerald-400', glow: 'bg-emerald-500' }
                        }
                        subtitle={threatLevel > 10 ? 'Requires immediate attention' : 'Controlled exposure footprint'}
                    />
                </div>

                {/* MITRE Matrix */}
                <MitreMatrixHeatmap
                    tacticCounts={tacticCounts}
                    onTacticClick={(tactic) => setSelectedTactic(tactic === selectedTactic ? null : tactic)}
                />

                {/* Charts and Alerts */}
                <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 pb-8">
                    <TopTechniquesChart techniques={topTechniques} />
                    <RelatedAlertsList alerts={alerts} selectedTactic={selectedTactic} />
                </div>
            </div>
        </div>
    );
}
