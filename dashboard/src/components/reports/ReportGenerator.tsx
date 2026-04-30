/**
 * Report Generator Component
 * Advanced report creation with templates, formats, and data visualization
 */

import { useState, useCallback } from 'react';
import { 
    FileText, FileSpreadsheet, FileCode, Download, RefreshCw, 
    Eye, AlertTriangle, Calendar, Filter,
    BarChart3, Shield, Activity, Terminal, Server, Settings
} from 'lucide-react';
import { useQuery } from '@tanstack/react-query';
import { 
    alertsApi, 
    agentsApi, 
    commandsApi,
} from '../../api/client';
import { ProfessionalReportView } from './ProfessionalReportView';
import { REPORT_TEMPLATES, REPORT_FORMATS, type ReportTemplate, type ReportFormat, type ReportData } from './ReportTemplates';

const TEMPLATE_ICONS: Record<string, any> = {
    BarChart3, Terminal, Shield, Activity, Settings,
};

const FORMAT_ICONS: Record<string, any> = {
    pdf: FileText,
    excel: FileSpreadsheet,
    word: FileText,
    html: FileCode,
    csv: FileSpreadsheet,
    json: FileCode,
};

export function ReportGenerator() {
    // Report configuration
    const [selectedTemplate, setSelectedTemplate] = useState<ReportTemplate>('executive');
    const [selectedFormat, setSelectedFormat] = useState<ReportFormat>('pdf');
    const [reportScope, setReportScope] = useState<'all' | 'specific'>('all');
    const [selectedAgent, setSelectedAgent] = useState<string>('');
    const [dateRange, setDateRange] = useState<{ from: string; to: string }>(() => {
        const to = new Date();
        const from = new Date();
        from.setDate(from.getDate() - 7);
        return {
            from: from.toISOString().slice(0, 16),
            to: to.toISOString().slice(0, 16),
        };
    });
    const [isGenerating, setIsGenerating] = useState(false);
    const [generatedData, setGeneratedData] = useState<ReportData | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [showPreview, setShowPreview] = useState(false);

    // Fetch agents list
    const agentsQuery = useQuery({
        queryKey: ['agents', 'reports-list'],
        queryFn: async () => {
            const res = await agentsApi.list({ limit: 500, sort_by: 'hostname' });
            return res.data;
        },
        staleTime: 60000,
    });

    // Generate report data
    const generateReportData = useCallback(async (): Promise<ReportData> => {
        const fromIso = new Date(dateRange.from).toISOString();
        const toIso = new Date(dateRange.to).toISOString();

        // Fetch all required data in parallel
        const [alertsRes, agentsRes, commandsRes] = await Promise.all([
            alertsApi.list({ 
                limit: 1000, 
                date_from: fromIso, 
                date_to: toIso,
                agent_id: reportScope === 'specific' && selectedAgent ? selectedAgent : undefined,
                sort: 'timestamp',
                order: 'desc'
            }),
            agentsApi.list({ limit: 500 }),
            commandsApi.list({ limit: 500 }),
        ]);

        const alerts = alertsRes.alerts || [];
        const agents = agentsRes.data || [];
        const commands = commandsRes.data || [];

        // Create agent map
        const agentMap = new Map(agents.map((a: { id: string; hostname: string }) => [a.id, a.hostname]));

        // Calculate summary
        const criticalCount = alerts.filter((a: { severity: string }) => a.severity === 'critical').length;
        const highCount = alerts.filter((a: { severity: string }) => a.severity === 'high').length;
        const mediumCount = alerts.filter((a: { severity: string }) => a.severity === 'medium').length;
        const lowCount = alerts.filter((a: { severity: string }) => a.severity === 'low').length;

        // Calculate timeline (last 7 days)
        const timeline: ReportData['charts']['timeline'] = [];
        for (let i = 6; i >= 0; i--) {
            const date = new Date();
            date.setDate(date.getDate() - i);
            const dateStr = date.toISOString().split('T')[0];
            
            const dayAlerts = alerts.filter((a: { timestamp: string }) => a.timestamp.startsWith(dateStr));
            timeline.push({
                date: dateStr,
                critical: dayAlerts.filter(a => a.severity === 'critical').length,
                high: dayAlerts.filter(a => a.severity === 'high').length,
                medium: dayAlerts.filter(a => a.severity === 'medium').length,
                low: dayAlerts.filter(a => a.severity === 'low').length,
            });
        }

        // Severity distribution
        const severityDistribution = [
            { name: 'Critical', value: criticalCount, color: '#ef4444' },
            { name: 'High', value: highCount, color: '#f97316' },
            { name: 'Medium', value: mediumCount, color: '#f59e0b' },
            { name: 'Low', value: lowCount, color: '#3b82f6' },
        ].filter(s => s.value > 0);

        // Top MITRE tactics
        const tacticCounts = new Map<string, number>();
        alerts.forEach((a: { mitre_tactics?: string[] }) => {
            a.mitre_tactics?.forEach((t: string) => {
                tacticCounts.set(t, (tacticCounts.get(t) || 0) + 1);
            });
        });
        const topTactics = Array.from(tacticCounts.entries())
            .sort((a, b) => b[1] - a[1])
            .slice(0, 10)
            .map(([tactic, count]) => ({ tactic, count }));

        // OS distribution
        const osCounts = new Map<string, number>();
        agents.forEach((a: { os_type: string }) => {
            osCounts.set(a.os_type, (osCounts.get(a.os_type) || 0) + 1);
        });
        const osDistribution = Array.from(osCounts.entries())
            .map(([os, count]) => ({ os, count }));

        // Enrich alerts with hostnames
        const enrichedAlerts = alerts.slice(0, 100).map((a: { agent_id: string }) => ({
            ...a,
            agent_hostname: agentMap.get(a.agent_id) || a.agent_id,
        }));

        return {
            generatedAt: new Date().toISOString(),
            period: { from: fromIso, to: toIso },
            filters: {
                scope: reportScope,
                agentId: selectedAgent || undefined,
            },
            summary: {
                totalAlerts: alerts.length,
                totalCommands: commands.length,
                totalDevices: agents.length,
                criticalCount,
                highCount,
                mediumCount,
                lowCount,
            },
            charts: {
                timeline,
                severityDistribution,
                topTactics,
                osDistribution,
            },
            tables: {
                alerts: enrichedAlerts,
                commands: commands.slice(0, 100),
                devices: agents,
                risks: [],
            },
        };
    }, [dateRange, reportScope, selectedAgent]);

    // Handle generate button
    const handleGenerate = async () => {
        setIsGenerating(true);
        setError(null);
        try {
            const data = await generateReportData();
            setGeneratedData(data);
            setShowPreview(true);
        } catch (err: any) {
            setError(err.message || 'Failed to generate report');
        } finally {
            setIsGenerating(false);
        }
    };

    // Handle download
    const handleDownload = async (format: ReportFormat) => {
        if (!generatedData) return;
        
        setIsGenerating(true);
        try {
            // Dynamic import to reduce initial bundle size
            const { exportReport } = await import('./reportExport');
            await exportReport(generatedData, format, selectedTemplate);
        } catch (err: any) {
            setError(err.message || 'Failed to export report');
        } finally {
            setIsGenerating(false);
        }
    };

    return (
        <div className="space-y-6">
            {/* Report Builder Panel */}
            <div className="rounded-2xl border border-slate-200/90 dark:border-slate-700/80 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm shadow-md overflow-hidden">
                <div className="px-6 py-4 border-b border-slate-200 dark:border-slate-700 bg-gradient-to-r from-violet-50/80 via-white to-slate-50/80 dark:from-violet-950/30 dark:via-slate-900/80 dark:to-slate-900/60">
                    <div className="flex items-center gap-3">
                        <div className="w-10 h-10 rounded-xl bg-violet-500/12 text-violet-600 dark:text-violet-400 border border-violet-500/15 flex items-center justify-center">
                            <BarChart3 className="w-5 h-5" />
                        </div>
                        <div>
                            <h2 className="text-lg font-semibold text-slate-900 dark:text-white">Professional Report Generator</h2>
                            <p className="text-sm text-slate-500 dark:text-slate-400">
                                Create beautiful, data-rich reports with charts and visualizations
                            </p>
                        </div>
                    </div>
                </div>

                <div className="p-6 space-y-6">
                    {/* Template Selection */}
                    <div>
                        <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-3">
                            Report Template
                        </label>
                        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-3">
                            {(Object.keys(REPORT_TEMPLATES) as ReportTemplate[]).map((template) => {
                                const config = REPORT_TEMPLATES[template];
                                const Icon = TEMPLATE_ICONS[config.icon] || FileText;
                                const isSelected = selectedTemplate === template;
                                
                                return (
                                    <button
                                        key={template}
                                        onClick={() => setSelectedTemplate(template)}
                                        className={`p-4 rounded-xl border-2 text-left transition-all ${
                                            isSelected 
                                                ? 'border-violet-500 bg-violet-50 dark:bg-violet-950/30' 
                                                : 'border-slate-200 dark:border-slate-700 hover:border-violet-300 dark:hover:border-violet-700'
                                        }`}
                                    >
                                        <Icon className={`w-6 h-6 mb-2 ${isSelected ? 'text-violet-600 dark:text-violet-400' : 'text-slate-500'}`} />
                                        <p className={`font-semibold ${isSelected ? 'text-violet-900 dark:text-violet-100' : 'text-slate-900 dark:text-white'}`}>
                                            {config.name}
                                        </p>
                                        <p className="text-xs text-slate-500 dark:text-slate-400 mt-1 line-clamp-2">
                                            {config.description}
                                        </p>
                                    </button>
                                );
                            })}
                        </div>
                    </div>

                    {/* Format Selection */}
                    <div>
                        <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-3">
                            Export Format
                        </label>
                        <div className="flex flex-wrap gap-2">
                            {REPORT_FORMATS.map((fmt) => {
                                const Icon = FORMAT_ICONS[fmt.id] || FileText;
                                const isSelected = selectedFormat === fmt.id;
                                
                                return (
                                    <button
                                        key={fmt.id}
                                        onClick={() => setSelectedFormat(fmt.id)}
                                        className={`flex items-center gap-2 px-4 py-2 rounded-lg border transition-all ${
                                            isSelected
                                                ? 'border-cyan-500 bg-cyan-50 dark:bg-cyan-950/30 text-cyan-700 dark:text-cyan-300'
                                                : 'border-slate-200 dark:border-slate-700 hover:border-slate-300 dark:hover:border-slate-600'
                                        }`}
                                    >
                                        <Icon className="w-4 h-4" />
                                        <span className="text-sm font-medium">{fmt.name}</span>
                                    </button>
                                );
                            })}
                        </div>
                        <p className="text-xs text-slate-500 dark:text-slate-400 mt-2">
                            {REPORT_FORMATS.find(f => f.id === selectedFormat)?.description}
                        </p>
                    </div>

                    {/* Scope & Date Range */}
                    <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
                        {/* Scope */}
                        <div>
                            <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
                                <Filter className="w-4 h-4 inline mr-1" />
                                Report Scope
                            </label>
                            <select
                                value={reportScope}
                                onChange={(e) => setReportScope(e.target.value as any)}
                                className="w-full px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-sm"
                            >
                                <option value="all">All Endpoints</option>
                                <option value="specific">Specific Endpoint</option>
                            </select>
                        </div>

                        {/* Agent Selection */}
                        {reportScope === 'specific' && (
                            <div>
                                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
                                    <Server className="w-4 h-4 inline mr-1" />
                                    Select Endpoint
                                </label>
                                <select
                                    value={selectedAgent}
                                    onChange={(e) => setSelectedAgent(e.target.value)}
                                    disabled={agentsQuery.isLoading}
                                    className="w-full px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-sm"
                                >
                                    <option value="">Choose an endpoint...</option>
                                    {agentsQuery.data?.map((agent) => (
                                        <option key={agent.id} value={agent.id}>
                                            {agent.hostname} ({agent.os_type})
                                        </option>
                                    ))}
                                </select>
                                {agentsQuery.isLoading && (
                                    <p className="text-xs text-slate-500 mt-1">Loading endpoints...</p>
                                )}
                            </div>
                        )}

                        {/* Date Range */}
                        <div className={reportScope === 'specific' ? '' : 'lg:col-span-2'}>
                            <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
                                <Calendar className="w-4 h-4 inline mr-1" />
                                Time Range
                            </label>
                            <div className="flex gap-2">
                                <input
                                    type="datetime-local"
                                    value={dateRange.from}
                                    onChange={(e) => setDateRange(prev => ({ ...prev, from: e.target.value }))}
                                    className="flex-1 px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-sm"
                                />
                                <span className="text-slate-500 self-center">to</span>
                                <input
                                    type="datetime-local"
                                    value={dateRange.to}
                                    onChange={(e) => setDateRange(prev => ({ ...prev, to: e.target.value }))}
                                    className="flex-1 px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-sm"
                                />
                            </div>
                        </div>
                    </div>

                    {/* Error Message */}
                    {error && (
                        <div className="rounded-lg border border-rose-200 dark:border-rose-900/50 bg-rose-50/80 dark:bg-rose-950/20 px-4 py-3 text-sm text-rose-900 dark:text-rose-200 flex items-center gap-2">
                            <AlertTriangle className="w-4 h-4" />
                            {error}
                        </div>
                    )}

                    {/* Action Buttons */}
                    <div className="flex flex-wrap gap-3 pt-4 border-t border-slate-200 dark:border-slate-700">
                        <button
                            onClick={handleGenerate}
                            disabled={isGenerating || (reportScope === 'specific' && !selectedAgent)}
                            className="flex items-center gap-2 px-6 py-3 rounded-xl bg-gradient-to-r from-violet-600 to-cyan-600 hover:from-violet-700 hover:to-cyan-700 text-white font-semibold shadow-lg shadow-violet-500/20 disabled:opacity-50 disabled:cursor-not-allowed transition-all"
                        >
                            {isGenerating ? (
                                <>
                                    <RefreshCw className="w-5 h-5 animate-spin" />
                                    Generating Report...
                                </>
                            ) : (
                                <>
                                    <Eye className="w-5 h-5" />
                                    Preview Report
                                </>
                            )}
                        </button>

                        {generatedData && (
                            <button
                                onClick={() => handleDownload(selectedFormat)}
                                disabled={isGenerating}
                                className="flex items-center gap-2 px-6 py-3 rounded-xl bg-slate-800 dark:bg-slate-700 hover:bg-slate-900 dark:hover:bg-slate-600 text-white font-semibold disabled:opacity-50 transition-all"
                            >
                                <Download className="w-5 h-5" />
                                Download {selectedFormat.toUpperCase()}
                            </button>
                        )}
                    </div>
                </div>
            </div>

            {/* Report Preview */}
            {showPreview && generatedData && (
                <ProfessionalReportView
                    data={generatedData}
                    template={selectedTemplate}
                    format={selectedFormat}
                    onDownload={handleDownload}
                    isGenerating={isGenerating}
                />
            )}

            {/* Quick Stats */}
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                <div className="p-4 rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800">
                    <div className="flex items-center gap-2 text-slate-500 dark:text-slate-400 mb-1">
                        <Shield className="w-4 h-4" />
                        <span className="text-xs uppercase tracking-wider">Report Types</span>
                    </div>
                    <p className="text-2xl font-bold text-slate-900 dark:text-white">5 Templates</p>
                    <p className="text-xs text-slate-500 mt-1">Executive, Technical, Compliance, Operations, Custom</p>
                </div>

                <div className="p-4 rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800">
                    <div className="flex items-center gap-2 text-slate-500 dark:text-slate-400 mb-1">
                        <FileText className="w-4 h-4" />
                        <span className="text-xs uppercase tracking-wider">Export Formats</span>
                    </div>
                    <p className="text-2xl font-bold text-slate-900 dark:text-white">6 Formats</p>
                    <p className="text-xs text-slate-500 mt-1">PDF, Excel, Word, HTML, CSV, JSON</p>
                </div>

                <div className="p-4 rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800">
                    <div className="flex items-center gap-2 text-slate-500 dark:text-slate-400 mb-1">
                        <BarChart3 className="w-4 h-4" />
                        <span className="text-xs uppercase tracking-wider">Visualizations</span>
                    </div>
                    <p className="text-2xl font-bold text-slate-900 dark:text-white">Interactive</p>
                    <p className="text-xs text-slate-500 mt-1">Charts, tables, and KPI cards</p>
                </div>
            </div>
        </div>
    );
}
