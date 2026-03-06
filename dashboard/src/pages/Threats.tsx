import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
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
function ThreatSummaryCard({
    title,
    value,
    icon: Icon,
    color,
    subtitle
}: {
    title: string;
    value: string | number;
    icon: typeof Shield;
    color: string;
    subtitle?: string;
}) {
    return (
        <div className="card">
            <div className="flex items-start justify-between">
                <div>
                    <p className="text-sm text-gray-500 dark:text-gray-400">{title}</p>
                    <p className="text-2xl font-bold text-gray-900 dark:text-white mt-1">{value}</p>
                    {subtitle && (
                        <p className="text-xs text-gray-500 mt-0.5">{subtitle}</p>
                    )}
                </div>
                <div className={`p-2 rounded-lg ${color}`}>
                    <Icon className="w-5 h-5 text-white" />
                </div>
            </div>
        </div>
    );
}

// MITRE Matrix Heatmap
function MitreMatrixHeatmap({
    tacticCounts,
    onTacticClick
}: {
    tacticCounts: Record<string, number>;
    onTacticClick: (tactic: string) => void;
}) {
    const maxCount = Math.max(...Object.values(tacticCounts), 1);

    return (
        <div className="card">
            <div className="flex items-center justify-between mb-4">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
                    MITRE ATT&CK Matrix
                </h3>
                <a
                    href="https://attack.mitre.org/"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-sm text-primary-600 hover:text-primary-700 flex items-center gap-1"
                >
                    View Full Framework <ExternalLink className="w-3 h-3" />
                </a>
            </div>

            <div className="grid grid-cols-7 gap-2">
                {MITRE_TACTICS.map((tactic) => {
                    const count = tacticCounts[tactic.id] || tacticCounts[tactic.name.toLowerCase()] || 0;
                    const bgColor = getHeatmapColor(count, maxCount);

                    return (
                        <button
                            key={tactic.id}
                            onClick={() => onTacticClick(tactic.name)}
                            className={`relative p-3 rounded-lg border-2 transition-all hover:scale-105 ${count > 0
                                    ? 'border-transparent cursor-pointer hover:shadow-md'
                                    : 'border-gray-200 dark:border-gray-700 cursor-default'
                                }`}
                            style={{ backgroundColor: bgColor || 'rgba(156, 163, 175, 0.1)' }}
                            disabled={count === 0}
                        >
                            <div className="text-center">
                                <p className={`text-xs font-medium ${count > 0 ? 'text-gray-900 dark:text-white' : 'text-gray-400'}`}>
                                    {tactic.shortName}
                                </p>
                                <p className={`text-lg font-bold mt-1 ${count > 0 ? 'text-gray-900 dark:text-white' : 'text-gray-300 dark:text-gray-600'}`}>
                                    {count}
                                </p>
                            </div>
                            {count > maxCount * 0.75 && (
                                <div className="absolute -top-1 -right-1 w-2 h-2 bg-red-500 rounded-full animate-pulse" />
                            )}
                        </button>
                    );
                })}
            </div>

            {/* Legend */}
            <div className="flex items-center justify-center gap-4 mt-4 pt-4 border-t border-gray-200 dark:border-gray-700">
                <div className="flex items-center gap-2 text-xs text-gray-500">
                    <span>Intensity:</span>
                    <div className="flex items-center gap-1">
                        <div className="w-4 h-4 rounded" style={{ backgroundColor: 'rgba(59, 130, 246, 0.3)' }} />
                        <span>Low</span>
                    </div>
                    <div className="flex items-center gap-1">
                        <div className="w-4 h-4 rounded" style={{ backgroundColor: 'rgba(234, 179, 8, 0.5)' }} />
                        <span>Med</span>
                    </div>
                    <div className="flex items-center gap-1">
                        <div className="w-4 h-4 rounded" style={{ backgroundColor: 'rgba(249, 115, 22, 0.7)' }} />
                        <span>High</span>
                    </div>
                    <div className="flex items-center gap-1">
                        <div className="w-4 h-4 rounded" style={{ backgroundColor: 'rgba(239, 68, 68, 0.9)' }} />
                        <span>Critical</span>
                    </div>
                </div>
            </div>
        </div>
    );
}

// Top Techniques Chart
function TopTechniquesChart({ techniques }: { techniques: { name: string; count: number }[] }) {
    const COLORS = ['#ef4444', '#f97316', '#eab308', '#22c55e', '#3b82f6'];

    return (
        <div className="card">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
                Top Techniques Detected
            </h3>
            {techniques.length === 0 ? (
                <div className="text-center py-8 text-gray-500">
                    No technique data available
                </div>
            ) : (
                <div className="h-64">
                    <ResponsiveContainer width="100%" height="100%">
                        <BarChart data={techniques} layout="vertical">
                            <XAxis type="number" tick={{ fontSize: 10, fill: '#9ca3af' }} />
                            <YAxis
                                dataKey="name"
                                type="category"
                                width={180}
                                tick={{ fontSize: 10, fill: '#9ca3af' }}
                                tickFormatter={(value) => value.length > 30 ? value.slice(0, 27) + '...' : value}
                            />
                            <Tooltip
                                contentStyle={{
                                    backgroundColor: '#1f2937',
                                    border: 'none',
                                    borderRadius: '8px',
                                    color: 'white'
                                }}
                            />
                            <Bar dataKey="count" radius={[0, 4, 4, 0]}>
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
}

// Related Alerts List
function RelatedAlertsList({
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
        <div className="card">
            <div className="flex items-center justify-between mb-4">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
                    {selectedTactic ? `Alerts: ${selectedTactic}` : 'Recent Threat Alerts'}
                </h3>
                {selectedTactic && (
                    <span className="badge badge-info">{filteredAlerts.length} matches</span>
                )}
            </div>

            {filteredAlerts.length === 0 ? (
                <div className="text-center py-8 text-gray-500">
                    {selectedTactic ? 'No alerts for this tactic' : 'No threat alerts'}
                </div>
            ) : (
                <div className="space-y-2 max-h-80 overflow-y-auto">
                    {filteredAlerts.map((alert) => (
                        <div
                            key={alert.id}
                            className="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-900/50 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 cursor-pointer transition-colors"
                        >
                            <div className="flex items-center gap-3">
                                <AlertTriangle className={`w-4 h-4 ${alert.severity === 'critical' ? 'text-red-500' :
                                        alert.severity === 'high' ? 'text-orange-500' :
                                            'text-yellow-500'
                                    }`} />
                                <div>
                                    <p className="text-sm font-medium text-gray-900 dark:text-white">
                                        {alert.rule_title}
                                    </p>
                                    <div className="flex items-center gap-2 mt-0.5">
                                        {alert.mitre_techniques?.slice(0, 2).map((tech) => (
                                            <span key={tech} className="text-xs text-primary-600 dark:text-primary-400">
                                                {tech}
                                            </span>
                                        ))}
                                    </div>
                                </div>
                            </div>
                            <ChevronRight className="w-4 h-4 text-gray-400" />
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
}

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
    const tacticCounts: Record<string, number> = {};
    alerts.forEach((alert) => {
        alert.mitre_tactics?.forEach((tactic) => {
            tacticCounts[tactic] = (tacticCounts[tactic] || 0) + 1;
        });
    });

    // Calculate technique counts
    const techniqueCounts: Record<string, number> = {};
    alerts.forEach((alert) => {
        alert.mitre_techniques?.forEach((tech) => {
            techniqueCounts[tech] = (techniqueCounts[tech] || 0) + 1;
        });
    });

    const topTechniques = Object.entries(techniqueCounts)
        .sort(([, a], [, b]) => b - a)
        .slice(0, 8)
        .map(([name, count]) => ({ name, count }));

    // Calculate summary stats
    const totalTactics = Object.keys(tacticCounts).length;
    const mostCommonTactic = Object.entries(tacticCounts)
        .sort(([, a], [, b]) => b - a)[0];
    const totalTechniques = Object.keys(techniqueCounts).length;
    const threatLevel = alerts.filter(a => a.severity === 'critical' || a.severity === 'high').length;

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
        <div className="space-y-6">
            <div className="flex items-center justify-between">
                <h1 className="text-3xl font-bold text-gray-900 dark:text-white">
                    Threat Intelligence
                </h1>
                <a
                    href="https://attack.mitre.org/"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="btn btn-secondary text-sm flex items-center gap-2"
                >
                    <Shield className="w-4 h-4" />
                    MITRE ATT&CK
                    <ExternalLink className="w-3 h-3" />
                </a>
            </div>

            {/* Summary Cards */}
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
                <ThreatSummaryCard
                    title="Tactics Triggered"
                    value={totalTactics}
                    icon={Target}
                    color="bg-red-500"
                    subtitle={`of ${MITRE_TACTICS.length} total tactics`}
                />
                <ThreatSummaryCard
                    title="Most Common Tactic"
                    value={mostCommonTactic ? mostCommonTactic[0] : 'N/A'}
                    icon={Shield}
                    color="bg-orange-500"
                    subtitle={mostCommonTactic ? `${mostCommonTactic[1]} occurrences` : undefined}
                />
                <ThreatSummaryCard
                    title="Techniques Detected"
                    value={totalTechniques}
                    icon={Eye}
                    color="bg-indigo-500"
                />
                <ThreatSummaryCard
                    title="High Risk Alerts"
                    value={threatLevel}
                    icon={TrendingUp}
                    color={threatLevel > 10 ? 'bg-red-600' : 'bg-green-500'}
                    subtitle={threatLevel > 10 ? 'Requires attention' : 'Under control'}
                />
            </div>

            {/* MITRE Matrix */}
            <MitreMatrixHeatmap
                tacticCounts={tacticCounts}
                onTacticClick={(tactic) => setSelectedTactic(tactic === selectedTactic ? null : tactic)}
            />

            {/* Charts and Alerts */}
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                <TopTechniquesChart techniques={topTechniques} />
                <RelatedAlertsList alerts={alerts} selectedTactic={selectedTactic} />
            </div>
        </div>
    );
}
