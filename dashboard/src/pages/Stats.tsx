import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import {
    BarChart, Bar, LineChart, Line, PieChart, Pie, Cell,
    XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer
} from 'recharts';
import { Download, TrendingUp, Activity, Shield, AlertTriangle } from 'lucide-react';
import { statsApi, type TimelineDataPoint } from '../api/client';

const COLORS = ['#ef4444', '#f97316', '#eab308', '#3b82f6', '#22c55e'];

// Date range options
const DATE_RANGES = [
    { label: 'Last 24 Hours', value: '24h' },
    { label: 'Last 7 Days', value: '7d' },
    { label: 'Last 30 Days', value: '30d' },
    { label: 'Custom', value: 'custom' },
];

export default function Stats() {
    const [dateRange, setDateRange] = useState('7d');
    const [exportFormat, setExportFormat] = useState('csv');

    // Fetch stats
    const { data: alertStats, isLoading: alertsLoading } = useQuery({
        queryKey: ['alertStats'],
        queryFn: statsApi.alerts,
    });

    const { data: ruleStats, isLoading: rulesLoading } = useQuery({
        queryKey: ['ruleStats'],
        queryFn: statsApi.rules,
    });

    const { data: perfStats } = useQuery({
        queryKey: ['performanceStats'],
        queryFn: statsApi.performance,
        refetchInterval: 10000,
    });

    // Compute date range for timeline
    const getDateRange = () => {
        const now = new Date();
        const hours = dateRange === '24h' ? 24 : dateRange === '7d' ? 168 : 720;
        const from = new Date(now.getTime() - hours * 60 * 60 * 1000).toISOString();
        return { from, to: now.toISOString(), granularity: dateRange === '24h' ? '1h' : dateRange === '7d' ? '6h' : '1d' };
    };

    const { data: timelineData } = useQuery({
        queryKey: ['statsTimeline', dateRange],
        queryFn: () => statsApi.timeline(getDateRange()),
    });

    // Transform severity data for pie chart
    const severityData = alertStats?.by_severity
        ? Object.entries(alertStats.by_severity).map(([name, value]) => ({
            name: name.charAt(0).toUpperCase() + name.slice(1),
            value,
        }))
        : [];

    // Trend data from real API
    const trendData = (timelineData?.data || []).map((point: TimelineDataPoint) => ({
        date: new Date(point.timestamp).toLocaleDateString('en-US', {
            weekday: dateRange === '30d' ? undefined : 'short',
            month: dateRange === '30d' ? 'short' : undefined,
            day: dateRange === '30d' ? 'numeric' : undefined,
            hour: dateRange === '24h' ? '2-digit' : undefined,
        }),
        alerts: (point.critical || 0) + (point.high || 0) + (point.medium || 0) + (point.low || 0) + (point.informational || 0),
        events: 0, // Event count not tracked in timeline
    }));

    // Top rules from severity breakdown
    const topRules = ruleStats?.by_severity
        ? Object.entries(ruleStats.by_severity)
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
            <div className="flex justify-between items-center mb-6">
                <h1 className="text-3xl font-bold text-gray-900 dark:text-white">Statistics & Reports</h1>

                {/* Export Controls */}
                <div className="flex gap-3">
                    <select
                        value={dateRange}
                        onChange={(e) => setDateRange(e.target.value)}
                        className="input w-40"
                    >
                        {DATE_RANGES.map((range) => (
                            <option key={range.value} value={range.value}>
                                {range.label}
                            </option>
                        ))}
                    </select>

                    <select
                        value={exportFormat}
                        onChange={(e) => setExportFormat(e.target.value)}
                        className="input w-28"
                    >
                        <option value="csv">CSV</option>
                        <option value="pdf">PDF</option>
                        <option value="json">JSON</option>
                    </select>

                    <button onClick={handleExport} className="btn btn-primary flex items-center gap-2">
                        <Download className="w-4 h-4" />
                        Export
                    </button>
                </div>
            </div>

            {/* Summary Cards */}
            <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
                <div className="card">
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

                <div className="card">
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

                <div className="card">
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

                <div className="card">
                    <div className="flex items-center gap-3">
                        <div className="p-2 bg-purple-100 dark:bg-purple-900 rounded-lg">
                            <TrendingUp className="w-5 h-5 text-purple-600" />
                        </div>
                        <div>
                            <p className="text-sm text-gray-500">Detection Rate</p>
                            <p className="text-2xl font-bold">{((alertStats?.avg_confidence || 0) * 100).toFixed(1)}%</p>
                        </div>
                    </div>
                </div>
            </div>

            {/* Charts Row */}
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
                {/* Alert Trend */}
                <div className="card">
                    <h3 className="text-lg font-semibold mb-4">Alert Trend ({dateRange})</h3>
                    {!timelineData ? (
                        <div className="h-[300px] flex items-center justify-center">
                            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
                        </div>
                    ) : trendData.length === 0 ? (
                        <div className="h-[300px] flex flex-col items-center justify-center text-gray-500">
                            <AlertTriangle className="w-10 h-10 mb-2 text-gray-400" />
                            <p className="font-medium">No trend data available</p>
                            <p className="text-sm mt-1">Timeline data will appear once alerts are generated</p>
                        </div>
                    ) : (
                        <ResponsiveContainer width="100%" height={300}>
                            <LineChart data={trendData}>
                                <CartesianGrid strokeDasharray="3 3" />
                                <XAxis dataKey="date" />
                                <YAxis />
                                <Tooltip />
                                <Legend />
                                <Line type="monotone" dataKey="alerts" stroke="#ef4444" strokeWidth={2} dot={false} />
                                <Line type="monotone" dataKey="events" stroke="#3b82f6" strokeWidth={2} dot={false} />
                            </LineChart>
                        </ResponsiveContainer>
                    )}
                </div>

                {/* Severity Distribution */}
                <div className="card">
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

            {/* Top Rules */}
            <div className="card mb-6">
                <h3 className="text-lg font-semibold mb-4">Top Triggered Rules</h3>
                <ResponsiveContainer width="100%" height={300}>
                    <BarChart data={topRules} layout="vertical">
                        <CartesianGrid strokeDasharray="3 3" />
                        <XAxis type="number" />
                        <YAxis dataKey="name" type="category" width={150} />
                        <Tooltip />
                        <Bar dataKey="count" fill="#0ea5e9" />
                    </BarChart>
                </ResponsiveContainer>
            </div>

            {/* Performance Metrics */}
            <div className="card">
                <h3 className="text-lg font-semibold mb-4">Performance Metrics</h3>
                <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                    <div className="text-center p-4 bg-gray-50 dark:bg-gray-700 rounded-lg">
                        <p className="text-sm text-gray-500">Avg Latency</p>
                        <p className="text-xl font-bold">{perfStats?.avg_event_latency_ms?.toFixed(2) || 0}ms</p>
                    </div>
                    <div className="text-center p-4 bg-gray-50 dark:bg-gray-700 rounded-lg">
                        <p className="text-sm text-gray-500">Rule Match Time</p>
                        <p className="text-xl font-bold">{perfStats?.avg_rule_matching_ms?.toFixed(2) || 0}ms</p>
                    </div>
                    <div className="text-center p-4 bg-gray-50 dark:bg-gray-700 rounded-lg">
                        <p className="text-sm text-gray-500">DB Query Time</p>
                        <p className="text-xl font-bold">{perfStats?.avg_database_query_ms?.toFixed(2) || 0}ms</p>
                    </div>
                    <div className="text-center p-4 bg-gray-50 dark:bg-gray-700 rounded-lg">
                        <p className="text-sm text-gray-500">Error Rate</p>
                        <p className="text-xl font-bold">{((perfStats?.error_rate || 0) * 100).toFixed(3)}%</p>
                    </div>
                </div>
            </div>
        </div>
    );
}
