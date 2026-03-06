import { useQuery } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { AlertTriangle, Shield, Activity, TrendingUp, Monitor, Wifi, WifiOff, ChevronRight } from 'lucide-react';
import {
    PieChart, Pie, Cell, ResponsiveContainer, Tooltip,
    AreaChart, Area, XAxis, YAxis, CartesianGrid,
    BarChart, Bar
} from 'recharts';
import { statsApi, alertsApi, agentsApi, createAlertStream, type Alert, type AgentStats, type Agent, type TimelineDataPoint } from '../api/client';
import { SkeletonKPICards, SkeletonChart } from '../components';

// STALE THRESHOLD: 5 minutes in milliseconds (matches backend sweeper)
const STALE_THRESHOLD_MS = 5 * 60 * 1000;

// Severity colors
const SEVERITY_COLORS = {
    critical: '#ef4444',
    high: '#f97316',
    medium: '#eab308',
    low: '#6366f1',
    informational: '#3b82f6',
};

const STATUS_COLORS = {
    open: '#ef4444',
    in_progress: '#f59e0b',
    acknowledged: '#f59e0b',
    resolved: '#22c55e',
};

// KPI Card Component
function KPICard({
    title,
    value,
    icon: Icon,
    color = 'primary',
    trend,
    subValue,
    onClick
}: {
    title: string;
    value: string | number;
    icon: React.ComponentType<{ className?: string }>;
    color?: 'primary' | 'danger' | 'warning' | 'success';
    trend?: number;
    subValue?: string;
    onClick?: () => void;
}) {
    const colorClasses = {
        primary: 'bg-primary-50 text-primary-600 dark:bg-primary-900/50 dark:text-primary-400',
        danger: 'bg-red-50 text-red-600 dark:bg-red-900/50 dark:text-red-400',
        warning: 'bg-amber-50 text-amber-600 dark:bg-amber-900/50 dark:text-amber-400',
        success: 'bg-green-50 text-green-600 dark:bg-green-900/50 dark:text-green-400',
    };

    return (
        <div
            className={`card transition-all duration-200 ${onClick ? 'cursor-pointer hover:shadow-lg hover:-translate-y-0.5' : ''}`}
            onClick={onClick}
        >
            <div className="flex items-center justify-between">
                <div className="flex-1">
                    <p className="text-sm font-medium text-gray-500 dark:text-gray-400">{title}</p>
                    <p className="text-3xl font-bold text-gray-900 dark:text-white mt-1">{value}</p>
                    {subValue && (
                        <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">{subValue}</p>
                    )}
                    {trend !== undefined && (
                        <p className={`text-sm mt-1 flex items-center gap-1 ${trend > 0 ? 'text-red-500' : 'text-green-500'}`}>
                            <TrendingUp className={`w-3 h-3 ${trend < 0 ? 'rotate-180' : ''}`} />
                            {Math.abs(trend)}% vs last week
                        </p>
                    )}
                </div>
                <div className={`p-3 rounded-xl ${colorClasses[color]}`}>
                    <Icon className="w-6 h-6" />
                </div>
            </div>
            {onClick && (
                <div className="mt-3 pt-3 border-t border-gray-100 dark:border-gray-700 flex items-center text-sm text-primary-600 dark:text-primary-400">
                    View details <ChevronRight className="w-4 h-4 ml-1" />
                </div>
            )}
        </div>
    );
}

// Alert Feed Component
function AlertsFeed({ alerts }: { alerts: Alert[] }) {
    const severityColors: Record<string, string> = {
        critical: 'border-l-red-500 bg-red-50/50 dark:bg-red-900/20',
        high: 'border-l-orange-500 bg-orange-50/50 dark:bg-orange-900/20',
        medium: 'border-l-yellow-500 bg-yellow-50/50 dark:bg-yellow-900/20',
        low: 'border-l-indigo-500 bg-indigo-50/50 dark:bg-indigo-900/20',
        informational: 'border-l-blue-500 bg-blue-50/50 dark:bg-blue-900/20',
    };

    const formatRelativeTime = (timestamp: string) => {
        const diff = Date.now() - new Date(timestamp).getTime();
        const minutes = Math.floor(diff / 60000);
        if (minutes < 1) return 'Just now';
        if (minutes < 60) return `${minutes}m ago`;
        const hours = Math.floor(minutes / 60);
        if (hours < 24) return `${hours}h ago`;
        return `${Math.floor(hours / 24)}d ago`;
    };

    return (
        <div className="card">
            <div className="flex items-center justify-between mb-4">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
                    Live Alerts Feed
                </h3>
                <span className="flex items-center gap-2 text-xs text-gray-500">
                    <span className="w-2 h-2 rounded-full bg-green-500 animate-pulse" />
                    Real-time
                </span>
            </div>
            <div className="space-y-2 max-h-[400px] overflow-y-auto">
                {alerts.length === 0 ? (
                    <p className="text-gray-500 text-center py-8">No recent alerts</p>
                ) : (
                    alerts.slice(0, 10).map((alert, index) => (
                        <div
                            key={alert.id}
                            className={`border-l-4 rounded-r-lg p-3 transition-all animate-slide-up ${severityColors[alert.severity] || severityColors.low}`}
                            style={{ animationDelay: `${index * 50}ms` }}
                        >
                            <div className="flex justify-between items-start gap-2">
                                <div className="min-w-0 flex-1">
                                    <p className="font-medium text-gray-900 dark:text-white truncate">
                                        {alert.rule_title}
                                    </p>
                                    <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                                        Agent: {alert.agent_id?.slice(0, 8)}...
                                    </p>
                                </div>
                                <div className="flex flex-col items-end gap-1">
                                    <span className={`badge badge-${alert.severity}`}>
                                        {alert.severity.toUpperCase()}
                                    </span>
                                    <span className="text-xs text-gray-400">
                                        {formatRelativeTime(alert.timestamp)}
                                    </span>
                                </div>
                            </div>
                        </div>
                    ))
                )}
            </div>
        </div>
    );
}

// Severity Distribution Donut Chart
function SeverityDonutChart({ data }: { data: Record<string, number> }) {
    const chartData = Object.entries(data).map(([name, value]) => ({
        name: name.charAt(0).toUpperCase() + name.slice(1),
        value,
        color: SEVERITY_COLORS[name as keyof typeof SEVERITY_COLORS] || '#6b7280',
    }));

    const total = chartData.reduce((acc, item) => acc + item.value, 0);

    if (total === 0) {
        return (
            <div className="card">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
                    Alerts by Severity
                </h3>
                <div className="h-48 flex items-center justify-center text-gray-500">
                    No alert data available
                </div>
            </div>
        );
    }

    return (
        <div className="card">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
                Alerts by Severity
            </h3>
            <div className="flex items-center gap-4">
                <div className="w-48 h-48">
                    <ResponsiveContainer width="100%" height="100%">
                        <PieChart>
                            <Pie
                                data={chartData}
                                cx="50%"
                                cy="50%"
                                innerRadius={50}
                                outerRadius={80}
                                paddingAngle={2}
                                dataKey="value"
                            >
                                {chartData.map((entry, index) => (
                                    <Cell key={`cell-${index}`} fill={entry.color} />
                                ))}
                            </Pie>
                            <Tooltip
                                formatter={(value) => [`${value} (${(((Number(value) || 0) / total) * 100).toFixed(1)}%)`, 'Count']}
                                contentStyle={{
                                    backgroundColor: 'var(--tooltip-bg, #1f2937)',
                                    border: 'none',
                                    borderRadius: '8px',
                                    color: 'white'
                                }}
                            />
                        </PieChart>
                    </ResponsiveContainer>
                </div>
                <div className="flex-1 space-y-2">
                    {chartData.map((item) => (
                        <div key={item.name} className="flex items-center justify-between">
                            <div className="flex items-center gap-2">
                                <div
                                    className="w-3 h-3 rounded-full"
                                    style={{ backgroundColor: item.color }}
                                />
                                <span className="text-sm text-gray-700 dark:text-gray-300">{item.name}</span>
                            </div>
                            <div className="flex items-center gap-2">
                                <span className="font-medium text-gray-900 dark:text-white">{item.value}</span>
                                <span className="text-xs text-gray-500">
                                    ({total > 0 ? ((item.value / total) * 100).toFixed(0) : 0}%)
                                </span>
                            </div>
                        </div>
                    ))}
                </div>
            </div>
        </div>
    );
}

// Endpoints Status Card
function EndpointsStatusCard({ stats, agents, onClick }: { stats: AgentStats | null; agents: Agent[]; onClick: () => void }) {
    if (!stats) return null;

    // Recompute online/offline/degraded using last_seen threshold
    let online = 0, offline = 0, degraded = 0;
    if (agents.length > 0) {
        const now = Date.now();
        agents.forEach((a) => {
            const elapsed = now - new Date(a.last_seen).getTime();
            const isStale = elapsed > STALE_THRESHOLD_MS;
            if (a.status === 'online' && !isStale) online++;
            else if (a.status === 'degraded' && !isStale) degraded++;
            else offline++;
        });
    } else {
        // Fallback to server-side stats if agent list not loaded
        online = stats.online;
        offline = stats.offline;
        degraded = stats.degraded;
    }
    const total = online + offline + degraded;

    const statusItems = [
        { label: 'Online', count: online, color: 'text-green-500', icon: Wifi },
        { label: 'Offline', count: offline, color: 'text-gray-500', icon: WifiOff },
        { label: 'Degraded', count: degraded, color: 'text-amber-500', icon: AlertTriangle },
    ];

    return (
        <div
            className="card cursor-pointer hover:shadow-lg transition-all"
            onClick={onClick}
        >
            <div className="flex items-center justify-between mb-4">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
                    Endpoint Status
                </h3>
                <Monitor className="w-5 h-5 text-gray-400" />
            </div>

            <div className="space-y-3">
                {statusItems.map((item) => (
                    <div key={item.label} className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                            <item.icon className={`w-4 h-4 ${item.color}`} />
                            <span className="text-sm text-gray-600 dark:text-gray-300">{item.label}</span>
                        </div>
                        <div className="flex items-center gap-2">
                            <span className={`font-semibold ${item.color}`}>{item.count}</span>
                            <span className="text-xs text-gray-400">
                                ({total > 0 ? ((item.count / total) * 100).toFixed(0) : 0}%)
                            </span>
                        </div>
                    </div>
                ))}
            </div>

            <div className="mt-4 pt-4 border-t border-gray-100 dark:border-gray-700">
                <div className="flex items-center justify-between text-sm">
                    <span className="text-gray-500">Avg Health</span>
                    <div className="flex items-center gap-2">
                        <div className="w-24 h-2 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
                            <div
                                className="h-full bg-green-500 rounded-full transition-all"
                                style={{ width: `${stats.avg_health}%` }}
                            />
                        </div>
                        <span className="font-medium text-gray-900 dark:text-white">
                            {stats.avg_health?.toFixed(1)}%
                        </span>
                    </div>
                </div>
            </div>

            <div className="mt-3 pt-3 border-t border-gray-100 dark:border-gray-700 flex items-center text-sm text-primary-600 dark:text-primary-400">
                View all endpoints <ChevronRight className="w-4 h-4 ml-1" />
            </div>
        </div>
    );
}

// Alert Status Overview
function AlertStatusOverview({ data }: { data: Record<string, number> }) {
    const statusItems = [
        { key: 'open', label: 'Open', color: STATUS_COLORS.open },
        { key: 'in_progress', label: 'In Progress', color: STATUS_COLORS.in_progress },
        { key: 'resolved', label: 'Resolved', color: STATUS_COLORS.resolved },
    ];

    const total = Object.values(data).reduce((a, b) => a + b, 0);

    return (
        <div className="card">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
                Alert Status Overview
            </h3>
            <div className="grid grid-cols-3 gap-4 text-center">
                {statusItems.map((item) => {
                    const count = data[item.key] || 0;
                    return (
                        <div key={item.key}>
                            <p className="text-3xl font-bold" style={{ color: item.color }}>
                                {count}
                            </p>
                            <p className="text-sm text-gray-500 dark:text-gray-400">{item.label}</p>
                            <p className="text-xs text-gray-400">
                                {total > 0 ? ((count / total) * 100).toFixed(0) : 0}%
                            </p>
                        </div>
                    );
                })}
            </div>

            {/* Progress bar */}
            <div className="mt-4 h-2 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden flex">
                {statusItems.map((item) => {
                    const count = data[item.key] || 0;
                    const percentage = total > 0 ? (count / total) * 100 : 0;
                    return (
                        <div
                            key={item.key}
                            className="h-full transition-all"
                            style={{ width: `${percentage}%`, backgroundColor: item.color }}
                        />
                    );
                })}
            </div>
        </div>
    );
}

// Timeline Chart (real data from statsApi.timeline)
function AlertTimelineChart() {
    const now = new Date();
    const from = new Date(now.getTime() - 24 * 60 * 60 * 1000).toISOString();
    const to = now.toISOString();

    const { data: timelineData, isLoading } = useQuery({
        queryKey: ['alertTimeline', '24h'],
        queryFn: () => statsApi.timeline({ from, to, granularity: '1h' }),
        refetchInterval: 60000,
    });

    const chartData = (timelineData?.data || []).map((point: TimelineDataPoint) => ({
        time: new Date(point.timestamp).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' }),
        critical: point.critical || 0,
        high: point.high || 0,
        medium: point.medium || 0,
        low: point.low || 0,
    }));

    return (
        <div className="card">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
                Alert Timeline (24h)
            </h3>
            {isLoading ? (
                <div className="h-64 flex items-center justify-center">
                    <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
                </div>
            ) : chartData.length === 0 ? (
                <div className="h-64 flex items-center justify-center text-gray-500">
                    No timeline data available
                </div>
            ) : (
                <div className="h-64">
                    <ResponsiveContainer width="100%" height="100%">
                        <AreaChart data={chartData}>
                            <CartesianGrid strokeDasharray="3 3" stroke="#374151" opacity={0.3} />
                            <XAxis
                                dataKey="time"
                                tick={{ fontSize: 10, fill: '#9ca3af' }}
                                interval="preserveStartEnd"
                            />
                            <YAxis tick={{ fontSize: 10, fill: '#9ca3af' }} />
                            <Tooltip
                                contentStyle={{
                                    backgroundColor: '#1f2937',
                                    border: 'none',
                                    borderRadius: '8px',
                                    color: 'white'
                                }}
                            />
                            <Area type="monotone" dataKey="critical" stackId="1" stroke={SEVERITY_COLORS.critical} fill={SEVERITY_COLORS.critical} fillOpacity={0.8} />
                            <Area type="monotone" dataKey="high" stackId="1" stroke={SEVERITY_COLORS.high} fill={SEVERITY_COLORS.high} fillOpacity={0.8} />
                            <Area type="monotone" dataKey="medium" stackId="1" stroke={SEVERITY_COLORS.medium} fill={SEVERITY_COLORS.medium} fillOpacity={0.8} />
                            <Area type="monotone" dataKey="low" stackId="1" stroke={SEVERITY_COLORS.low} fill={SEVERITY_COLORS.low} fillOpacity={0.8} />
                        </AreaChart>
                    </ResponsiveContainer>
                </div>
            )}
            <div className="flex justify-center gap-4 mt-4">
                {Object.entries(SEVERITY_COLORS).slice(0, 4).map(([name, color]) => (
                    <div key={name} className="flex items-center gap-1.5 text-xs">
                        <div className="w-2.5 h-2.5 rounded" style={{ backgroundColor: color }} />
                        <span className="capitalize text-gray-600 dark:text-gray-400">{name}</span>
                    </div>
                ))}
            </div>
        </div>
    );
}

// Top Detection Rules
function TopDetectionRules({ alerts }: { alerts: Alert[] }) {
    // Aggregate by rule
    const ruleCounts = alerts.reduce((acc, alert) => {
        const key = alert.rule_title || alert.rule_id;
        acc[key] = (acc[key] || 0) + 1;
        return acc;
    }, {} as Record<string, number>);

    const topRules = Object.entries(ruleCounts)
        .sort(([, a], [, b]) => b - a)
        .slice(0, 5)
        .map(([name, count]) => ({ name, count }));



    return (
        <div className="card">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
                Top Detection Rules
            </h3>
            {topRules.length === 0 ? (
                <p className="text-gray-500 text-center py-4">No data available</p>
            ) : (
                <div className="h-48">
                    <ResponsiveContainer width="100%" height="100%">
                        <BarChart data={topRules} layout="vertical">
                            <CartesianGrid strokeDasharray="3 3" stroke="#374151" opacity={0.3} horizontal={false} />
                            <XAxis type="number" tick={{ fontSize: 10, fill: '#9ca3af' }} />
                            <YAxis
                                dataKey="name"
                                type="category"
                                width={150}
                                tick={{ fontSize: 10, fill: '#9ca3af' }}
                                tickFormatter={(value) => value.length > 25 ? value.slice(0, 25) + '...' : value}
                            />
                            <Tooltip
                                contentStyle={{
                                    backgroundColor: '#1f2937',
                                    border: 'none',
                                    borderRadius: '8px',
                                    color: 'white'
                                }}
                            />
                            <Bar
                                dataKey="count"
                                fill={SEVERITY_COLORS.high}
                                radius={[0, 4, 4, 0]}
                            />
                        </BarChart>
                    </ResponsiveContainer>
                </div>
            )}
        </div>
    );
}

// Main Dashboard Page
export default function Dashboard() {
    const navigate = useNavigate();
    const [liveAlerts, setLiveAlerts] = useState<Alert[]>([]);

    // Fetch stats
    const { data: alertStats, isLoading: statsLoading } = useQuery({
        queryKey: ['alertStats'],
        queryFn: statsApi.alerts,
        refetchInterval: 30000,
    });

    const { data: ruleStats } = useQuery({
        queryKey: ['ruleStats'],
        queryFn: statsApi.rules,
    });

    const { data: agentStats } = useQuery({
        queryKey: ['agentStats'],
        queryFn: agentsApi.stats,
        retry: false, // Don't retry if connection manager is not available
    });

    // Fetch agent list so we can recompute status with last_seen override
    const { data: agentListData } = useQuery({
        queryKey: ['agents'],
        queryFn: () => agentsApi.list({ limit: 200 }),
        retry: false,
    });

    // Fetch recent alerts for feed
    const { data: recentAlerts } = useQuery({
        queryKey: ['recentAlerts'],
        queryFn: () => alertsApi.list({ limit: 50 }),
    });

    // Initialize live alerts from recent alerts
    useEffect(() => {
        if (recentAlerts?.alerts) {
            setLiveAlerts(recentAlerts.alerts);
        }
    }, [recentAlerts]);

    // WebSocket for live alerts
    useEffect(() => {
        const stream = createAlertStream((alert) => {
            setLiveAlerts((prev) => [alert, ...prev.slice(0, 49)]);
        }, { severity: ['critical', 'high', 'medium'] });

        return () => stream.close();
    }, []);

    if (statsLoading) {
        return (
            <div className="space-y-6">
                <div className="h-9 w-64 bg-gray-200 dark:bg-gray-700 rounded animate-pulse" />
                <SkeletonKPICards count={4} />
                <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                    <SkeletonChart />
                    <SkeletonChart />
                </div>
            </div>
        );
    }

    return (
        <div className="space-y-6">
            <div className="flex items-center justify-between">
                <h1 className="text-3xl font-bold text-gray-900 dark:text-white">
                    Security Dashboard
                </h1>
                <div className="flex items-center gap-2 text-sm text-gray-500">
                    <span className="w-2 h-2 rounded-full bg-green-500 animate-pulse" />
                    Live
                </div>
            </div>

            {/* KPI Cards */}
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
                <KPICard
                    title="Total Alerts (24h)"
                    value={alertStats?.alerts_24h || 0}
                    icon={AlertTriangle}
                    color="danger"
                    onClick={() => navigate('/alerts')}
                />
                <KPICard
                    title="Critical Alerts"
                    value={alertStats?.by_severity?.critical || 0}
                    icon={Shield}
                    color="danger"
                    subValue="Requires immediate action"
                    onClick={() => navigate('/alerts?severity=critical')}
                />
                <KPICard
                    title="Active Rules"
                    value={ruleStats?.enabled_rules || 0}
                    icon={Activity}
                    color="primary"
                    subValue={`of ${ruleStats?.total_rules || 0} total`}
                    onClick={() => navigate('/rules')}
                />
                <KPICard
                    title="Avg Confidence"
                    value={`${((alertStats?.avg_confidence || 0) * 100).toFixed(1)}%`}
                    icon={TrendingUp}
                    color="success"
                />
            </div>

            {/* Main Charts Row */}
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                <AlertsFeed alerts={liveAlerts} />
                <SeverityDonutChart data={alertStats?.by_severity || {}} />
            </div>

            {/* Timeline */}
            <AlertTimelineChart />

            {/* Bottom Row */}
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
                <AlertStatusOverview data={alertStats?.by_status || {}} />
                <EndpointsStatusCard
                    stats={agentStats || null}
                    agents={agentListData?.data || []}
                    onClick={() => navigate('/endpoints')}
                />
                <TopDetectionRules alerts={liveAlerts} />
            </div>
        </div>
    );
}
