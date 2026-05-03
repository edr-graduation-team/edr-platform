/**
 * Professional Report View Component
 * Interactive preview of reports with charts and tables before export
 */

import { useState } from 'react';
import { 
    FileText, Download, BarChart3, TrendingUp, 
    AlertTriangle, CheckCircle, Server, Shield, Activity,
    ChevronDown, ChevronUp
} from 'lucide-react';
import {
    BarChart,
    Bar,
    XAxis,
    YAxis,
    CartesianGrid,
    Tooltip,
    ResponsiveContainer,
    PieChart,
    Pie,
    Cell,
    Legend,
    AreaChart,
    Area,
} from 'recharts';
import type { ReportData, ReportTemplate, ReportFormat } from './ReportTemplates';
import { REPORT_TEMPLATES, REPORT_FORMATS } from './ReportTemplates';

interface ProfessionalReportViewProps {
    data: ReportData | null;
    template: ReportTemplate;
    format: ReportFormat;
    onDownload: (format: ReportFormat) => void;
    isGenerating: boolean;
    customSections?: string[];
    /** When true, hides the internal header/download bar (used by ReportPreviewPage which has its own). */
    hideActionBar?: boolean;
}

const CHART_COLORS = {
    critical: '#ef4444',
    high: '#f97316',
    medium: '#f59e0b',
    low: '#3b82f6',
    informational: '#06b6d4',
};

const SEVERITY_COLORS: Record<string, string> = {
    critical: '#ef4444',
    high: '#f97316',
    medium: '#f59e0b',
    low: '#3b82f6',
    informational: '#06b6d4',
};

export function ProfessionalReportView({ 
    data, 
    template, 
    format, 
    onDownload, 
    isGenerating,
    customSections,
    hideActionBar = false,
}: ProfessionalReportViewProps) {
    const [expandedSections, setExpandedSections] = useState<Set<string>>(new Set(['summary', 'kpis']));
    const config = REPORT_TEMPLATES[template];

    const toggleSection = (sectionId: string) => {
        const newSet = new Set(expandedSections);
        if (newSet.has(sectionId)) {
            newSet.delete(sectionId);
        } else {
            newSet.add(sectionId);
        }
        setExpandedSections(newSet);
    };

    // Helper to check if section should be shown
    const shouldShowSection = (sectionId: string) => {
        if (template !== 'custom') return true; // All sections shown for non-custom templates
        return customSections?.includes(sectionId) ?? true;
    };

    if (!data) {
        return (
            <div className="flex flex-col items-center justify-center p-12 text-slate-500">
                <FileText className="w-16 h-16 mb-4 opacity-30" />
                <p className="text-lg font-medium">No report data available</p>
                <p className="text-sm mt-2">Generate a report to see the preview</p>
            </div>
        );
    }

    const formatInfo = REPORT_FORMATS.find(f => f.id === format);

    return (
        <div className="bg-white dark:bg-slate-900 rounded-xl border border-slate-200 dark:border-slate-700 overflow-hidden print:border-none print:rounded-none">
            {/* Internal header — hidden when parent page has its own action bar */}
            {!hideActionBar && (
                <div className="px-6 py-4 border-b border-slate-200 dark:border-slate-700 bg-gradient-to-r from-slate-50 to-white dark:from-slate-800 dark:to-slate-900">
                    <div className="flex items-center justify-between">
                        <div className="flex items-center gap-3">
                            <div className="w-10 h-10 rounded-lg flex items-center justify-center" 
                                 style={{ backgroundColor: `${config.colorScheme.primary}20`, color: config.colorScheme.primary }}>
                                <BarChart3 className="w-5 h-5" />
                            </div>
                            <div>
                                <h3 className="font-semibold text-slate-900 dark:text-white">{config.name}</h3>
                                <p className="text-xs text-slate-500">
                                    Preview • {new Date(data.generatedAt).toLocaleString()} • {formatInfo?.name}
                                </p>
                            </div>
                        </div>
                        <button
                            onClick={() => onDownload(format)}
                            disabled={isGenerating}
                            className="flex items-center gap-2 px-4 py-2 rounded-lg bg-cyan-600 hover:bg-cyan-700 text-white text-sm font-medium disabled:opacity-50 transition-colors"
                        >
                            {isGenerating ? (
                                <>
                                    <div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                                    Generating...
                                </>
                            ) : (
                                <>
                                    <Download className="w-4 h-4" />
                                    Download {formatInfo?.extension.toUpperCase()}
                                </>
                            )}
                        </button>
                    </div>
                </div>
            )}

            {/* Preview Content — scrollable in modal, full-height in standalone page */}
            <div className={`p-6 space-y-6 ${hideActionBar ? '' : 'max-h-[600px] overflow-y-auto'}`}>
                {/* Executive Summary */}
                {shouldShowSection('summary') && <ReportSection 
                        title="Executive Summary"
                        icon={CheckCircle}
                        color={config.colorScheme.success}
                        isExpanded={expandedSections.has('summary')}
                        onToggle={() => toggleSection('summary')}
                    >
                    <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
                        <KpiCard 
                            title="Total Alerts" 
                            value={data.summary.totalAlerts} 
                            trend={+12} 
                            color={config.colorScheme.primary}
                        />
                        <KpiCard 
                            title="Critical" 
                            value={data.summary.criticalCount} 
                            trend={-5}
                            color={config.colorScheme.danger}
                        />
                        <KpiCard 
                            title="High Severity" 
                            value={data.summary.highCount}
                            color={config.colorScheme.warning}
                        />
                        <KpiCard 
                            title="Response Time" 
                            value={`${data.summary.avgResponseTime || 0}m`}
                            color={config.colorScheme.accent}
                        />
                    </div>
                    <div className="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
                        <p className="text-sm text-slate-700 dark:text-slate-300 leading-relaxed">
                            During the period <strong>{new Date(data.period.from).toLocaleDateString()}</strong> to{' '}
                            <strong>{new Date(data.period.to).toLocaleDateString()}</strong>, the EDR platform detected{' '}
                            <strong>{data.summary.totalAlerts}</strong> security events across{' '}
                            <strong>{data.summary.totalDevices}</strong> monitored endpoints. 
                            Critical attention is required for <strong>{data.summary.criticalCount}</strong> high-risk alerts.
                        </p>
                    </div>
                </ReportSection>}

                {/* KPI Cards */}
                {shouldShowSection('kpis') && <ReportSection
                    title="Key Performance Indicators"
                    icon={Activity}
                    color={config.colorScheme.primary}
                    isExpanded={expandedSections.has('kpis')}
                    onToggle={() => toggleSection('kpis')}
                >
                    <div className="grid grid-cols-2 md:grid-cols-5 gap-4">
                        <StatPill label="Alerts" value={data.summary.totalAlerts} color="bg-blue-500" />
                        <StatPill label="Critical" value={data.summary.criticalCount} color="bg-red-500" />
                        <StatPill label="High" value={data.summary.highCount} color="bg-orange-500" />
                        <StatPill label="Medium" value={data.summary.mediumCount} color="bg-yellow-500" />
                        <StatPill label="Low" value={data.summary.lowCount} color="bg-blue-400" />
                    </div>
                </ReportSection>}

                {/* Charts */}
                {shouldShowSection('trends') && data.charts.timeline.length > 0 && (
                    <ReportSection
                        title="Trend Analysis (7 Days)"
                        icon={TrendingUp}
                        color={config.colorScheme.accent}
                        isExpanded={expandedSections.has('trends')}
                        onToggle={() => toggleSection('trends')}
                    >
                        <div className="h-64">
                            <ResponsiveContainer width="100%" height="100%">
                                <AreaChart data={data.charts.timeline}>
                                    <CartesianGrid strokeDasharray="3 3" stroke="#e2e8f0" />
                                    <XAxis dataKey="date" tick={{ fontSize: 11 }} />
                                    <YAxis tick={{ fontSize: 11 }} />
                                    <Tooltip 
                                        contentStyle={{ 
                                            background: 'rgba(15, 23, 42, 0.95)', 
                                            border: 'none', 
                                            borderRadius: '8px',
                                            color: 'white'
                                        }} 
                                    />
                                    <Legend />
                                    <Area type="monotone" dataKey="critical" stackId="1" stroke={CHART_COLORS.critical} fill={CHART_COLORS.critical} fillOpacity={0.6} />
                                    <Area type="monotone" dataKey="high" stackId="1" stroke={CHART_COLORS.high} fill={CHART_COLORS.high} fillOpacity={0.6} />
                                    <Area type="monotone" dataKey="medium" stackId="1" stroke={CHART_COLORS.medium} fill={CHART_COLORS.medium} fillOpacity={0.6} />
                                </AreaChart>
                            </ResponsiveContainer>
                        </div>
                    </ReportSection>
                )}

                {/* Severity Distribution */}
                {shouldShowSection('severity') && data.charts.severityDistribution.length > 0 && (
                    <ReportSection
                        title="Alert Severity Distribution"
                        icon={PieChart}
                        color={config.colorScheme.secondary}
                        isExpanded={expandedSections.has('severity')}
                        onToggle={() => toggleSection('severity')}
                    >
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                            <div className="h-64">
                                <ResponsiveContainer width="100%" height="100%">
                                    <PieChart>
                                        <Pie
                                            data={data.charts.severityDistribution}
                                            cx="50%"
                                            cy="50%"
                                            innerRadius={60}
                                            outerRadius={80}
                                            paddingAngle={5}
                                            dataKey="value"
                                        >
                                            {data.charts.severityDistribution.map((entry, index) => (
                                                <Cell key={`cell-${index}`} fill={SEVERITY_COLORS[entry.name.toLowerCase()] || '#64748b'} />
                                            ))}
                                        </Pie>
                                        <Tooltip />
                                        <Legend />
                                    </PieChart>
                                </ResponsiveContainer>
                            </div>
                            <div className="space-y-3">
                                {data.charts.severityDistribution.map((item) => (
                                    <div key={item.name} className="flex items-center justify-between p-3 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
                                        <div className="flex items-center gap-2">
                                            <div 
                                                className="w-3 h-3 rounded-full" 
                                                style={{ backgroundColor: SEVERITY_COLORS[item.name.toLowerCase()] }}
                                            />
                                            <span className="text-sm font-medium capitalize">{item.name}</span>
                                        </div>
                                        <span className="text-sm font-bold">{item.value}</span>
                                    </div>
                                ))}
                            </div>
                        </div>
                    </ReportSection>
                )}

                {/* MITRE Tactics */}
                {shouldShowSection('mitre') && data.charts.topTactics.length > 0 && (
                    <ReportSection
                        title="MITRE ATT&CK Tactics"
                        icon={Shield}
                        color={config.colorScheme.warning}
                        isExpanded={expandedSections.has('mitre')}
                        onToggle={() => toggleSection('mitre')}
                    >
                        <div className="h-64">
                            <ResponsiveContainer width="100%" height="100%">
                                <BarChart data={data.charts.topTactics} layout="horizontal">
                                    <CartesianGrid strokeDasharray="3 3" stroke="#e2e8f0" />
                                    <XAxis type="number" tick={{ fontSize: 11 }} />
                                    <YAxis type="category" dataKey="tactic" tick={{ fontSize: 10 }} width={150} />
                                    <Tooltip contentStyle={{ background: 'rgba(15, 23, 42, 0.95)', border: 'none', borderRadius: '8px', color: 'white' }} />
                                    <Bar dataKey="count" fill={config.colorScheme.primary} radius={[0, 4, 4, 0]} />
                                </BarChart>
                            </ResponsiveContainer>
                        </div>
                    </ReportSection>
                )}

                {/* Data Tables Preview */}
                {shouldShowSection('alerts') && data.tables.alerts.length > 0 && (
                    <ReportSection
                        title={`Recent Alerts (${data.tables.alerts.length} shown)`}
                        icon={AlertTriangle}
                        color={config.colorScheme.danger}
                        isExpanded={expandedSections.has('alerts')}
                        onToggle={() => toggleSection('alerts')}
                    >
                        <div className="overflow-x-auto">
                            <table className="min-w-full text-sm">
                                <thead className="bg-slate-100 dark:bg-slate-800">
                                    <tr>
                                        <th className="px-4 py-2 text-left font-semibold">Time</th>
                                        <th className="px-4 py-2 text-left font-semibold">Severity</th>
                                        <th className="px-4 py-2 text-left font-semibold">Rule</th>
                                        <th className="px-4 py-2 text-left font-semibold">Endpoint</th>
                                    </tr>
                                </thead>
                                <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
                                    {data.tables.alerts.slice(0, 5).map((alert, idx) => (
                                        <tr key={idx} className="hover:bg-slate-50 dark:hover:bg-slate-800/50">
                                            <td className="px-4 py-2 text-slate-600 dark:text-slate-400">
                                                {new Date(alert.timestamp).toLocaleString()}
                                            </td>
                                            <td className="px-4 py-2">
                                                <SeverityBadge severity={alert.severity} />
                                            </td>
                                            <td className="px-4 py-2 font-medium">{alert.rule_title}</td>
                                            <td className="px-4 py-2 text-slate-600">{alert.agent_hostname || alert.agent_id?.slice(0, 8)}</td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                            {data.tables.alerts.length > 5 && (
                                <p className="text-center text-xs text-slate-500 py-2">
                                    + {data.tables.alerts.length - 5} more in full report
                                </p>
                            )}
                        </div>
                    </ReportSection>
                )}

                {/* OS Distribution */}
                {shouldShowSection('os') && data.charts.osDistribution.length > 0 && (
                    <ReportSection
                        title="Operating System Distribution"
                        icon={Server}
                        color={config.colorScheme.success}
                        isExpanded={expandedSections.has('os')}
                        onToggle={() => toggleSection('os')}
                    >
                        <div className="flex flex-wrap gap-4">
                            {data.charts.osDistribution.map((os) => (
                                <div key={os.os} className="flex items-center gap-2 px-4 py-2 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
                                    <Server className="w-4 h-4 text-slate-500" />
                                    <span className="font-medium capitalize">{os.os}</span>
                                    <span className="text-slate-500">({os.count})</span>
                                </div>
                            ))}
                        </div>
                    </ReportSection>
                )}
            </div>
        </div>
    );
}

// Sub-components
function ReportSection({ 
    title, 
    icon: Icon, 
    color, 
    isExpanded, 
    onToggle, 
    children 
}: { 
    title: string; 
    icon: any; 
    color: string; 
    isExpanded: boolean; 
    onToggle: () => void; 
    children: React.ReactNode;
}) {
    return (
        <div className="border border-slate-200 dark:border-slate-700 rounded-lg overflow-hidden">
            <button
                onClick={onToggle}
                className="w-full flex items-center justify-between px-4 py-3 bg-slate-50 dark:bg-slate-800/50 hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors"
            >
                <div className="flex items-center gap-2">
                    <Icon className="w-4 h-4" style={{ color }} />
                    <span className="font-semibold text-slate-900 dark:text-white">{title}</span>
                </div>
                <div className="print:hidden">
                    {isExpanded ? <ChevronUp className="w-4 h-4 text-slate-500" /> : <ChevronDown className="w-4 h-4 text-slate-500" />}
                </div>
            </button>
            <div className={`p-4 ${isExpanded ? 'block' : 'hidden print:block'}`}>
                {children}
            </div>
        </div>
    );
}

function KpiCard({ title, value, trend, color }: { title: string; value: number | string; trend?: number; color: string }) {
    return (
        <div className="p-4 rounded-lg bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700">
            <p className="text-xs text-slate-500 uppercase tracking-wider">{title}</p>
            <p className="text-2xl font-bold mt-1" style={{ color }}>{value}</p>
            {trend !== undefined && (
                <p className={`text-xs mt-1 ${trend > 0 ? 'text-red-500' : 'text-green-500'}`}>
                    {trend > 0 ? '↑' : '↓'} {Math.abs(trend)}% vs last period
                </p>
            )}
        </div>
    );
}

function StatPill({ label, value, color }: { label: string; value: number; color: string }) {
    return (
        <div className="flex items-center gap-2 px-3 py-2 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
            <div className={`w-2 h-2 rounded-full ${color}`} />
            <span className="text-sm text-slate-600 dark:text-slate-400">{label}:</span>
            <span className="font-bold">{value}</span>
        </div>
    );
}

function SeverityBadge({ severity }: { severity: string }) {
    const colors: Record<string, string> = {
        critical: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300',
        high: 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-300',
        medium: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300',
        low: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300',
        informational: 'bg-cyan-100 text-cyan-800 dark:bg-cyan-900/30 dark:text-cyan-300',
    };
    return (
        <span className={`px-2 py-1 rounded text-xs font-semibold uppercase ${colors[severity.toLowerCase()] || colors.low}`}>
            {severity}
        </span>
    );
}
