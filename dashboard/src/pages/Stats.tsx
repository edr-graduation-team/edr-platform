import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { useState, useMemo } from 'react';
import {
    PieChart, Pie, Cell,
    Tooltip, ResponsiveContainer
} from 'recharts';
import { Download, TrendingUp, Activity, Shield, AlertTriangle, Target, ChevronDown } from 'lucide-react';
import { statsApi, authApi, contextPoliciesApi, alertsApi, type TimelineDataPoint } from '../api/client';

const COLORS = ['#ef4444', '#f97316', '#eab308', '#3b82f6', '#22c55e'];

const STATUS_COLORS = {
    open: '#ef4444',
    in_progress: '#f59e0b',
    acknowledged: '#f59e0b',
    resolved: '#22c55e',
};

const DATE_RANGES = [
    { label: 'Last 24 Hours', value: '24h' },
    { label: 'Last 7 Days', value: '7d' },
    { label: 'Last 30 Days', value: '30d' },
    { label: 'Custom', value: 'custom' },
];

function AlertStatusOverview({ data }: { data: Record<string, number> }) {
    const statusItems = [
        { key: 'open', label: 'Open', color: STATUS_COLORS.open },
        { key: 'in_progress', label: 'In Progress', color: STATUS_COLORS.in_progress },
        { key: 'resolved', label: 'Resolved', color: STATUS_COLORS.resolved },
    ];

    const total = Object.values(data).reduce((a, b) => a + b, 0);

    return (
        <div className="card border border-slate-200 dark:border-slate-700/60 shadow-lg dark:shadow-cyan-900/10 bg-white dark:bg-slate-800/90 backdrop-blur-sm rounded-xl h-full flex flex-col">
            <div className="flex items-center justify-between mb-6 relative z-10">
                <h3 className="text-lg font-bold text-slate-800 dark:text-white flex items-center gap-2">
                    <Activity className="w-5 h-5 text-cyan-400 drop-shadow-[0_0_5px_rgba(34,211,238,0.5)]" />
                    Alert Status Overview
                </h3>
            </div>
            
            <div className="flex-1 flex flex-col justify-center gap-8 border border-slate-200 dark:border-slate-700/40 rounded-xl p-6 bg-slate-50/30 dark:bg-slate-900/40">
                <div className="grid grid-cols-3 gap-4 text-center">
                    {statusItems.map((item) => {
                        const count = data[item.key] || 0;
                        return (
                            <div key={item.key} className="flex flex-col items-center">
                                <p className="text-4xl font-bold font-mono" style={{ color: item.color, textShadow: `0 0 10px ${item.color}40` }}>
                                    {count}
                                </p>
                                <p className="text-[11px] font-bold uppercase tracking-wider text-slate-500 mt-2">{item.label}</p>
                                <p className="text-[10px] text-slate-400 mt-0.5">
                                    {total > 0 ? ((count / total) * 100).toFixed(0) : 0}%
                                </p>
                            </div>
                        );
                    })}
                </div>

                <div className="w-full h-3 bg-slate-200 dark:bg-slate-700/50 rounded-full overflow-hidden flex shadow-inner">
                    {statusItems.map((item) => {
                        const count = data[item.key] || 0;
                        const percentage = total > 0 ? (count / total) * 100 : 0;
                        return (
                            <div
                                key={item.key}
                                className="h-full transition-all duration-1000 ease-out"
                                style={{ width: `${percentage}%`, backgroundColor: item.color }}
                            >
                                <div className="w-full h-full bg-white/20"></div>
                            </div>
                        );
                    })}
                </div>
            </div>
        </div>
    );
}

export default function Stats() {
    const navigate = useNavigate();
    const canExport = authApi.canExportStats();
    const [dateRange, setDateRange] = useState('7d');
    const [customFrom, setCustomFrom] = useState('');
    const [customTo, setCustomTo] = useState('');
    const [exportFormat, setExportFormat] = useState('csv');

    // Fetch stats
    const { data: alertStats, isLoading: alertsLoading } = useQuery({
        queryKey: ['alertStats'],
        queryFn: statsApi.alerts,
        refetchInterval: 10000,
    });

    const { data: ruleStats, isLoading: rulesLoading } = useQuery({
        queryKey: ['ruleStats'],
        queryFn: statsApi.rules,
        refetchInterval: 30000,
    });

    const { data: perfStats } = useQuery({
        queryKey: ['performanceStats'],
        queryFn: statsApi.performance,
        refetchInterval: 10000,
    });

    // Compute date range for timeline
    const getDateRange = () => {
        const now = new Date();
        if (dateRange === 'custom') {
            const from = customFrom ? new Date(customFrom).toISOString() : new Date(now.getTime() - 720 * 60 * 60 * 1000).toISOString();
            const to = customTo ? new Date(customTo).toISOString() : now.toISOString();
            const diffHours = (new Date(to).getTime() - new Date(from).getTime()) / (1000 * 60 * 60);
            return { from, to, granularity: diffHours <= 48 ? '1h' : '1d' };
        }
        
        const hours = dateRange === '24h' ? 24 : dateRange === '7d' ? 168 : 720;
        const from = new Date(now.getTime() - hours * 60 * 60 * 1000).toISOString();
        return { from, to: now.toISOString(), granularity: dateRange === '24h' ? '1h' : '1d' };
    };

    const { data: timelineData } = useQuery({
        queryKey: ['statsTimeline', dateRange, customFrom, customTo],
        queryFn: () => statsApi.timeline(getDateRange()),
        refetchInterval: 15000,
    });

    const { data: contextPolicies } = useQuery({
        queryKey: ['contextPoliciesCount'],
        queryFn: contextPoliciesApi.list,
        refetchInterval: 30000,
    });

    const { data: contextAlertsSample } = useQuery({
        queryKey: ['contextScoringSample'],
        queryFn: () => alertsApi.list({ limit: 300, sort: 'timestamp', order: 'desc' }),
        refetchInterval: 30000,
    });

    const contextDistribution = useMemo(() => {
        const alerts = contextAlertsSample?.alerts || [];
        const buckets = { elevated: 0, neutral: 0, discounted: 0 };
        for (const a of alerts) {
            const m = a.score_breakdown?.context_multiplier;
            if (typeof m !== 'number') continue;
            if (m > 1.05) buckets.elevated++;
            else if (m < 0.95) buckets.discounted++;
            else buckets.neutral++;
        }
        return buckets;
    }, [contextAlertsSample]);

    // Transform severity data for pie chart
    const severityData = alertStats?.by_severity
        ? Object.entries(alertStats.by_severity).map(([name, value]) => ({
            name: name.charAt(0).toUpperCase() + name.slice(1),
            value,
        }))
        : [];

    // Trend data with Skeleton padding (Professional Activity Matrix)
    const trendData = useMemo(() => {
        const raw = timelineData?.data || [];
        const skeleton: { date: string; alerts: number }[] = [];
        const now = new Date();
        
        // 1. Build chronological timeline skeleton to ensure no empty graphs
        if (dateRange === '24h') {
            for (let i = 23; i >= 0; i--) {
                const d = new Date(now.getTime() - i * 60 * 60 * 1000);
                skeleton.push({ date: d.toLocaleDateString('en-US', { hour: '2-digit' }), alerts: 0 });
            }
        } else if (dateRange === '7d') {
            for (let i = 6; i >= 0; i--) {
                const d = new Date(now.getTime() - i * 24 * 60 * 60 * 1000);
                skeleton.push({ date: d.toLocaleDateString('en-US', { weekday: 'short' }), alerts: 0 });
            }
        } else if (dateRange === 'custom') {
            const endDate = customTo ? new Date(customTo) : now;
            const startDate = customFrom ? new Date(customFrom) : new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000);
            const diffDays = Math.max(1, Math.ceil((endDate.getTime() - startDate.getTime()) / (1000 * 60 * 60 * 24)));
            const diffHours = (endDate.getTime() - startDate.getTime()) / (1000 * 60 * 60);
            
            if (diffHours <= 48) {
                const hours = Math.max(1, Math.ceil(diffHours));
                for (let i = hours - 1; i >= 0; i--) {
                    const d = new Date(endDate.getTime() - i * 60 * 60 * 1000);
                    skeleton.push({ date: d.toLocaleDateString('en-US', { hour: '2-digit' }), alerts: 0 });
                }
            } else {
                // Ensure we don't render too many columns (max 60 to prevent extreme UI breakage)
                const limitDays = Math.min(diffDays, 60);
                for (let i = limitDays - 1; i >= 0; i--) {
                    const d = new Date(endDate.getTime() - i * 24 * 60 * 60 * 1000);
                    skeleton.push({ date: d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' }), alerts: 0 });
                }
            }
        } else {
            for (let i = 29; i >= 0; i--) {
                const d = new Date(now.getTime() - i * 24 * 60 * 60 * 1000);
                skeleton.push({ date: d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' }), alerts: 0 });
            }
        }

        // 2. Overlay API data
        raw.forEach((point: TimelineDataPoint) => {
            const pointDate = new Date(point.timestamp);
            const total = (point.critical || 0) + (point.high || 0) + (point.medium || 0) + (point.low || 0) + (point.informational || 0);

            let formattedPointDate = '';
            if (dateRange === '24h' || (dateRange === 'custom' && (new Date(getDateRange().to).getTime() - new Date(getDateRange().from).getTime()) / (1000 * 60 * 60) <= 48)) {
                formattedPointDate = pointDate.toLocaleDateString('en-US', { hour: '2-digit' });
            } else if (dateRange === '7d') {
                formattedPointDate = pointDate.toLocaleDateString('en-US', { weekday: 'short' });
            } else {
                formattedPointDate = pointDate.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
            }

            const bucket = skeleton.find(s => s.date === formattedPointDate);
            if (bucket) {
                bucket.alerts += total;
            }
        });

        return skeleton;
    }, [timelineData, dateRange]);

    // Top rules from alert frequency mapping
    const topRules = alertStats?.by_rule
        ? Object.entries(alertStats.by_rule)
            .map(([name, count]) => ({ name: name.charAt(0).toUpperCase() + name.slice(1), count: count as number }))
            .sort((a, b) => b.count - a.count)
            .slice(0, 5)
        : [];

    const handleExport = () => {
        // Export feature — future implementation
        console.log(`Export requested: format=${exportFormat}, range=${dateRange}`);
    };

    if (alertsLoading || rulesLoading) {
        return (
            <div className="flex items-center justify-center h-64">
                <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600"></div>
            </div>
        );
    }

    return (
        <div>
            <div className="flex justify-between items-center mb-8">
                <div className="flex flex-col">
                    <h1 className="text-3xl font-bold text-gray-900 dark:text-white">Statistics & Reports</h1>
                    <p className="text-sm font-medium text-slate-500 dark:text-slate-400 mt-1">Historical Data & Threat Analysis</p>
                </div>

                {/* Export Controls */}
                <div className="flex items-center gap-4">
                    <div className="relative">
                        <select
                            value={dateRange}
                            onChange={(e) => setDateRange(e.target.value)}
                            className="appearance-none bg-slate-900/60 border border-slate-700 text-slate-200 rounded-lg pl-4 pr-10 py-2 hover:bg-slate-800 focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500 focus:outline-none transition-all cursor-pointer font-medium text-sm w-40"
                        >
                            {DATE_RANGES.map((range) => (
                                <option key={range.value} value={range.value}>
                                    {range.label}
                                </option>
                            ))}
                        </select>
                        <div className="pointer-events-none absolute inset-y-0 right-0 flex items-center px-3 text-slate-400">
                            <ChevronDown className="w-4 h-4" />
                        </div>
                    </div>

                    {dateRange === 'custom' && (
                        <div className="flex items-center gap-2">
                            <input 
                                type="date" 
                                value={customFrom}
                                onChange={(e) => setCustomFrom(e.target.value)}
                                className="bg-slate-900/60 border border-slate-700 text-slate-200 rounded-lg px-3 py-2 hover:bg-slate-800 focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500 focus:outline-none transition-all text-sm w-36"
                            />
                            <span className="text-slate-500">to</span>
                            <input 
                                type="date" 
                                value={customTo}
                                onChange={(e) => setCustomTo(e.target.value)}
                                className="bg-slate-900/60 border border-slate-700 text-slate-200 rounded-lg px-3 py-2 hover:bg-slate-800 focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500 focus:outline-none transition-all text-sm w-36"
                            />
                        </div>
                    )}

                    {canExport && (
                        <>
                            <div className="relative">
                                <select
                                    value={exportFormat}
                                    onChange={(e) => setExportFormat(e.target.value)}
                                    className="appearance-none bg-slate-900/60 border border-slate-700 text-slate-200 rounded-lg pl-4 pr-10 py-2 hover:bg-slate-800 focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500 focus:outline-none transition-all cursor-pointer font-medium text-sm w-28"
                                >
                                    <option value="csv">CSV</option>
                                    <option value="pdf">PDF</option>
                                    <option value="json">JSON</option>
                                </select>
                                <div className="pointer-events-none absolute inset-y-0 right-0 flex items-center px-3 text-slate-400">
                                    <ChevronDown className="w-4 h-4" />
                                </div>
                            </div>

                            <button onClick={handleExport} className="bg-gradient-to-r from-cyan-600 to-blue-600 hover:from-cyan-500 hover:to-blue-500 text-white shadow-md hover:shadow-cyan-500/25 transition-all rounded-lg px-5 py-2 text-sm font-semibold flex items-center gap-2">
                                <Download className="w-4 h-4" />
                                Export
                            </button>
                        </>
                    )}
                </div>
            </div>

            {/* Summary Cards */}
            <div className="grid grid-cols-1 md:grid-cols-5 gap-4 mb-6">
                <div className="card border border-slate-200 dark:border-slate-700/60 shadow-sm dark:shadow-slate-900/20 rounded-xl">
                    <div className="flex items-center gap-3">
                        <div className="p-2 bg-red-100 dark:bg-red-900 rounded-lg">
                            <AlertTriangle className="w-5 h-5 text-red-600" />
                        </div>
                        <div>
                            <p className="text-sm text-gray-500">Total Alerts</p>
                            <p className="text-2xl font-bold">{alertStats?.total_alerts || 0}</p>
                        </div>
                    </div>
                </div>

                <div className="card border border-slate-200 dark:border-slate-700/60 shadow-sm dark:shadow-slate-900/20 rounded-xl">
                    <div className="flex items-center gap-3">
                        <div className="p-2 bg-blue-100 dark:bg-blue-900 rounded-lg">
                            <Shield className="w-5 h-5 text-blue-600" />
                        </div>
                        <div>
                            <p className="text-sm text-gray-500">Active Rules</p>
                            <p className="text-2xl font-bold">{ruleStats?.enabled_rules || 0}</p>
                        </div>
                    </div>
                </div>

                <div className="card border border-slate-200 dark:border-slate-700/60 shadow-sm dark:shadow-slate-900/20 rounded-xl">
                    <div className="flex items-center gap-3">
                        <div className="p-2 bg-green-100 dark:bg-green-900 rounded-lg">
                            <Activity className="w-5 h-5 text-green-600" />
                        </div>
                        <div>
                            <p className="text-sm text-gray-500">Events/Sec</p>
                            <p className="text-2xl font-bold">{perfStats?.events_per_second?.toFixed(1) || 0}</p>
                        </div>
                    </div>
                </div>

                <div className="card border border-slate-200 dark:border-slate-700/60 shadow-sm dark:shadow-slate-900/20 rounded-xl">
                    <div className="flex items-center gap-3">
                        <div className="p-2 bg-purple-100 dark:bg-purple-900 rounded-lg">
                            <TrendingUp className="w-5 h-5 text-purple-600" />
                        </div>
                        <div>
                            <p className="text-sm text-gray-500">Avg Risk (normalized)</p>
                            <p className="text-2xl font-bold">{((alertStats?.avg_confidence || 0) * 100).toFixed(1)}%</p>
                        </div>
                    </div>
                </div>

                <div className="card border border-slate-200 dark:border-slate-700/60 shadow-sm dark:shadow-slate-900/20 rounded-xl">
                    <div className="flex items-center gap-3">
                        <div className="p-2 bg-cyan-100 dark:bg-cyan-900 rounded-lg">
                            <Target className="w-5 h-5 text-cyan-600" />
                        </div>
                        <div>
                            <p className="text-sm text-gray-500">Active Context Policies</p>
                            <p className="text-2xl font-bold">{(contextPolicies?.data || []).filter(p => p.enabled).length}</p>
                        </div>
                    </div>
                </div>

                <div className="card border border-slate-200 dark:border-slate-700/60 shadow-sm dark:shadow-slate-900/20 rounded-xl">
                    <div className="flex items-center gap-3">
                        <div className="p-2 bg-indigo-100 dark:bg-indigo-900 rounded-lg">
                            <Shield className="w-5 h-5 text-indigo-600" />
                        </div>
                        <div>
                            <p className="text-sm text-gray-500">Context Scoring (sample)</p>
                            <p className="text-sm font-bold">
                                +{contextDistribution.elevated} / ={contextDistribution.neutral} / -{contextDistribution.discounted}
                            </p>
                        </div>
                    </div>
                </div>
            </div>

            {/* Charts Row */}
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
                {/* Alert Trend - Modern Activity Matrix Redesign */}
                <div className="card min-w-0 border border-slate-200 dark:border-slate-700/60 shadow-lg dark:shadow-cyan-900/10 bg-white dark:bg-slate-800/90 backdrop-blur-sm rounded-xl">
                    <div className="flex justify-between items-center mb-6">
                        <h3 className="text-lg font-bold text-slate-800 dark:text-white flex items-center gap-2">
                            <Activity className="w-5 h-5 text-cyan-400 drop-shadow-[0_0_5px_rgba(34,211,238,0.5)]" />
                            Activity Trend <span className="text-sm font-normal text-slate-500">({dateRange})</span>
                        </h3>
                         <div className="flex gap-2 items-center">
                            <div className="flex items-center gap-1.5"><div className="w-2.5 h-2.5 rounded-full bg-slate-200 dark:bg-slate-700"></div><span className="text-xs text-slate-500">0</span></div>
                            <div className="flex items-center gap-1.5"><div className="w-2.5 h-2.5 rounded-full bg-cyan-900/40"></div></div>
                            <div className="flex items-center gap-1.5"><div className="w-2.5 h-2.5 rounded-full bg-cyan-600"></div></div>
                            <div className="flex items-center gap-1.5"><div className="w-2.5 h-2.5 rounded-full bg-cyan-400 shadow-[0_0_8px_rgba(34,211,238,0.8)]"></div><span className="text-xs text-slate-500">High</span></div>
                        </div>
                    </div>

                    {!timelineData ? (
                        <div className="h-[260px] flex items-center justify-center">
                            <div className="w-10 h-10 rounded-full border-2 border-cyan-500/20 border-t-cyan-400 animate-spin"></div>
                        </div>
                    ) : (
                        <div className="border border-slate-200 dark:border-slate-700/40 rounded-xl p-4 sm:p-6 bg-slate-50/30 dark:bg-slate-900/40 mt-2 flex flex-col justify-end">
                            <div className="flex items-end justify-between w-full h-[180px] gap-1 sm:gap-1.5 min-w-0">
                                {trendData.map((data, idx) => {
                                    // Calculate relative intensity for visualization (0.1 to 1.0)
                                    const maxAlerts = Math.max(...trendData.map(d => d.alerts), 1);
                                    const intensity = data.alerts === 0 ? 0 : Math.max(0.15, data.alerts / maxAlerts);
                                    
                                    // Generate highly dynamic column heights and glow
                                    // Minimum 0 height if no alerts.
                                    const heightPct = data.alerts === 0 ? 0 : 10 + (intensity * 90);
                                    
                                    // Stylistic variables
                                    const isZero = data.alerts === 0;
                                    const glowIntensity = intensity > 0.7 ? 'drop-shadow-[0_0_8px_rgba(34,211,238,0.8)]' : (intensity > 0.3 ? 'drop-shadow-[0_0_4px_rgba(6,182,212,0.5)]' : '');
                                    const bgClass = 'bg-gradient-to-t from-blue-700/80 to-cyan-400 border-cyan-400/50';

                                    return (
                                        <div key={idx} className="relative flex flex-col items-center group flex-1 h-full min-w-0">
                                            {/* Tooltip Hover Overlay */}
                                            <div className="absolute bottom-[calc(100%+12px)] opacity-0 group-hover:opacity-100 transition-opacity duration-200 pointer-events-none z-20 flex flex-col items-center">
                                                <div className="bg-slate-800 text-white text-xs font-bold px-3 py-2 rounded-lg shadow-xl border border-slate-600/50 whitespace-nowrap flex items-center">
                                                    <span className="text-cyan-400 mr-2 text-sm">{data.alerts}</span>Alerts
                                                    <span className="text-[10px] text-slate-400 font-normal ml-2 pl-2 border-l border-slate-600/50">{data.date}</span>
                                                </div>
                                                {/* Tooltip Arrow */}
                                                <div className="w-2.5 h-2.5 bg-slate-800 border-r border-b border-slate-600/50 transform rotate-45 -mt-1.5"></div>
                                            </div>

                                            {/* Container Tube (Socket) - Always visible providing structural grid aesthetic */}
                                            <div className="relative w-full max-w-[48px] h-full bg-slate-200/50 dark:bg-slate-800/40 rounded-md border border-slate-300/50 dark:border-slate-700/50 overflow-hidden flex items-end shadow-inner transition-colors group-hover:dark:bg-slate-700/40">
                                                {/* Active Fill Bar */}
                                                <div 
                                                    className={`w-full transition-all duration-1000 ease-out border-t-2 relative ${bgClass} ${glowIntensity}`}
                                                    style={{ height: `${heightPct}%`, opacity: isZero ? 0 : 0.8 + (intensity * 0.2) }}
                                                >
                                                    {!isZero && <div className="absolute top-0 left-0 right-0 h-1 bg-white/60 blur-[1px]"></div>}
                                                    {!isZero && <div className="absolute inset-0 bg-gradient-to-b from-white/20 to-transparent"></div>}
                                                </div>
                                            </div>
                                            
                                            {/* Date Label under bar */}
                                            <span className="text-[9px] sm:text-[10px] font-medium text-slate-500 dark:text-slate-400 mt-3 truncate w-full text-center group-hover:text-cyan-400 transition-colors">
                                                {dateRange === '24h' || (dateRange === 'custom' && (new Date(getDateRange().to).getTime() - new Date(getDateRange().from).getTime()) / (1000 * 60 * 60) <= 48) 
                                                    ? data.date 
                                                    : (dateRange === '7d' ? data.date.split(' ')[0] : data.date.split(' ')[1] || data.date)}
                                            </span>
                                        </div>
                                    );
                                })}
                            </div>
                        </div>
                    )}
                </div>

                {/* Severity Distribution */}
                <div className="card border border-slate-200 dark:border-slate-700/60 shadow-sm dark:shadow-slate-900/20 rounded-xl">
                    <h3 className="text-lg font-semibold mb-4">Severity Distribution</h3>
                    <ResponsiveContainer width="100%" height={300}>
                        <PieChart>
                            <Pie
                                data={severityData}
                                cx="50%"
                                cy="50%"
                                labelLine={false}
                                label={({ name, percent }) => `${name} ${((percent ?? 0) * 100).toFixed(0)}%`}
                                outerRadius={100}
                                dataKey="value"
                            >
                                {severityData.map((_, index) => (
                                    <Cell key={`cell-${index}`} fill={COLORS[index % COLORS.length]} />
                                ))}
                            </Pie>
                            <Tooltip />
                        </PieChart>
                    </ResponsiveContainer>
                </div>
            </div>

            {/* Main Data Row */}
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 mb-6">
                {/* Top Rules - Modern SaaS Redesign */}
                <div className="lg:col-span-2 card relative overflow-hidden border border-slate-200 dark:border-slate-700/60 shadow-lg dark:shadow-cyan-900/10 bg-white dark:bg-slate-800/90 backdrop-blur-sm rounded-xl h-full flex flex-col">
                    {/* Subtle background glow */}
                    <div className="absolute top-0 right-0 w-64 h-64 -z-10 mix-blend-screen pointer-events-none" style={{ background: 'radial-gradient(circle, rgba(6,182,212,0.05) 0%, transparent 70%)' }}></div>

                    <div className="flex items-center justify-between mb-6 relative z-10 shrink-0">
                        <h3 className="text-lg font-bold text-slate-800 dark:text-white flex items-center gap-2">
                            <Target className="w-5 h-5 text-cyan-400 drop-shadow-[0_0_5px_rgba(34,211,238,0.5)]" />
                            Top Triggered Rules
                        </h3>
                    </div>
                    
                    <div className="flex-1 border border-slate-200 dark:border-slate-700/40 rounded-xl p-6 bg-slate-50/30 dark:bg-slate-900/40 relative z-10 box-border flex flex-col justify-center">
                        <div className="space-y-6">
                            {topRules.length === 0 ? (
                                <div className="flex items-center justify-center h-40 text-slate-500">No rule data available.</div>
                            ) : (
                                topRules.map((rule, idx) => {
                                    const maxCount = Math.max(...topRules.map(r => r.count), 1);
                                    const percentage = (rule.count / maxCount) * 100;
                                    return (
                                        <div key={idx} className="relative group">
                                            <div className="flex justify-between items-end mb-2">
                                                <div className="flex items-center gap-2">
                                                    <span className="text-sm font-semibold text-slate-700 dark:text-slate-200 group-hover:text-cyan-400 transition-colors drop-shadow-sm">
                                                        {rule.name}
                                                    </span>
                                                </div>
                                                <span className="text-sm font-bold text-slate-900 dark:text-white flex items-center gap-1.5 drop-shadow-sm">
                                                    {rule.count}
                                                    <span className="text-xs font-normal text-slate-500 dark:text-slate-400">alerts</span>
                                                </span>
                                            </div>
                                            <div className="h-2.5 w-full bg-slate-200 dark:bg-slate-800/80 rounded-full overflow-hidden shadow-inner flex border border-slate-300 dark:border-slate-700/50 relative">
                                                <div 
                                                    className="h-full bg-gradient-to-r from-blue-600 to-cyan-400 rounded-full relative transition-all duration-1000 ease-out flex items-center justify-end"
                                                    style={{ width: `${percentage}%` }}
                                                >
                                                    {/* Glowing continuous Hover Pulse */}
                                                    <div className="absolute top-0 bottom-0 left-0 right-0 bg-white/20 opacity-0 group-hover:opacity-100 transition-opacity duration-300"></div>
                                                    
                                                    {/* Glowing Leading Edge */}
                                                    <div className="w-4 h-full bg-white opacity-40 blur-[2px] rounded-full mr-0.5 shadow-[0_0_8px_rgba(255,255,255,0.8)]"></div>
                                                </div>
                                            </div>
                                        </div>
                                    );
                                })
                            )}
                        </div>
                    </div>
                </div>

                <div className="lg:col-span-1 h-full">
                    <AlertStatusOverview data={alertStats?.by_status || {}} />
                </div>
            </div>

            {/* Performance Metrics */}
            <div className="card border border-slate-200 dark:border-slate-700/60 shadow-sm dark:shadow-slate-900/20 bg-white dark:bg-slate-800 rounded-xl">
                <div className="flex items-center justify-between mb-4">
                    <h3 className="text-lg font-bold text-slate-800 dark:text-white">Performance & Reliability</h3>
                    <div className="flex items-center gap-3">
                        <span className="text-[11px] font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider">
                            Sigma Engine
                        </span>
                        <button
                            onClick={() => navigate('/settings?tab=reliability')}
                            className="text-[11px] font-semibold px-2.5 py-1 rounded-md border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-900/40 transition-colors"
                        >
                            View Ingestion Reliability
                        </button>
                    </div>
                </div>

                <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                    <div className="flex flex-col items-center justify-center p-6 bg-slate-50 dark:bg-slate-900/50 rounded-xl border border-slate-100 dark:border-slate-700/60 transition-all hover:shadow-md hover:border-blue-500/30">
                        <p className="text-sm font-medium text-slate-500 dark:text-slate-400 mb-1">Avg Latency</p>
                        <p className="text-2xl font-bold text-slate-900 dark:text-white">{perfStats?.avg_event_latency_ms?.toFixed(2) || 0}<span className="text-sm font-medium text-slate-400 ml-1">ms</span></p>
                    </div>
                    <div className="flex flex-col items-center justify-center p-6 bg-slate-50 dark:bg-slate-900/50 rounded-xl border border-slate-100 dark:border-slate-700/60 transition-all hover:shadow-md hover:border-blue-500/30">
                        <p className="text-sm font-medium text-slate-500 dark:text-slate-400 mb-1">Rule Match Time</p>
                        <p className="text-2xl font-bold text-slate-900 dark:text-white">{perfStats?.avg_rule_matching_ms?.toFixed(2) || 0}<span className="text-sm font-medium text-slate-400 ml-1">ms</span></p>
                    </div>
                    <div className="flex flex-col items-center justify-center p-6 bg-slate-50 dark:bg-slate-900/50 rounded-xl border border-slate-100 dark:border-slate-700/60 transition-all hover:shadow-md hover:border-blue-500/30">
                        <p className="text-sm font-medium text-slate-500 dark:text-slate-400 mb-1">DB Query Time</p>
                        <p className="text-2xl font-bold text-slate-900 dark:text-white">{perfStats?.avg_database_query_ms?.toFixed(2) || 0}<span className="text-sm font-medium text-slate-400 ml-1">ms</span></p>
                    </div>
                    <div className="flex flex-col items-center justify-center p-6 bg-slate-50 dark:bg-slate-900/50 rounded-xl border border-slate-100 dark:border-slate-700/60 transition-all hover:shadow-md hover:border-rose-500/30">
                        <p className="text-sm font-medium text-slate-500 dark:text-slate-400 mb-1">Error Rate</p>
                        <p className="text-2xl font-bold text-slate-900 dark:text-white">{((perfStats?.error_rate || 0) * 100).toFixed(3)}<span className="text-sm font-medium text-slate-400 ml-1">%</span></p>
                    </div>

                    <div className="flex flex-col items-center justify-center p-6 bg-slate-50 dark:bg-slate-900/50 rounded-xl border border-slate-100 dark:border-slate-700/60 transition-all hover:shadow-md hover:border-amber-500/30">
                        <p className="text-sm font-medium text-slate-500 dark:text-slate-400 mb-1">Alert Fallback Used</p>
                        <p className="text-2xl font-bold text-slate-900 dark:text-white font-mono">
                            {perfStats?.alert_fallback_used ?? 0}
                        </p>
                        <p className="text-[11px] text-slate-400 mt-1 text-center">
                            Fallback delivery when internal channel saturated
                        </p>
                    </div>

                    <div className="flex flex-col items-center justify-center p-6 bg-slate-50 dark:bg-slate-900/50 rounded-xl border border-slate-100 dark:border-slate-700/60 transition-all hover:shadow-md hover:border-red-500/30">
                        <p className="text-sm font-medium text-slate-500 dark:text-slate-400 mb-1">Alerts Dropped</p>
                        <p className="text-2xl font-bold text-slate-900 dark:text-white font-mono">
                            {perfStats?.alerts_dropped ?? 0}
                        </p>
                        <p className="text-[11px] text-slate-400 mt-1 text-center">
                            Non‑zero indicates potential alert data loss
                        </p>
                    </div>
                </div>
            </div>
        </div>
    );
}
