import { useQuery } from '@tanstack/react-query';
import { useState, useMemo } from 'react';
import {
    Search, Download, Eye, User, Activity,
    Check, X, RefreshCw, Shield, AlertTriangle, Settings, ChevronDown
} from 'lucide-react';
import { auditApi, authApi, type AuditLog } from '../api/client';
import { Modal, DateRangePicker, type DateRange, SkeletonTable } from '../components';
import { useDebounce } from '../hooks/useDebounce';

// Action icons mapping
const ACTION_ICONS: Record<string, { icon: typeof Activity; color: string }> = {
    login: { icon: User, color: 'text-green-500' },
    logout: { icon: User, color: 'text-gray-500' },
    login_failed: { icon: X, color: 'text-red-500' },
    create: { icon: Check, color: 'text-green-500' },
    update: { icon: RefreshCw, color: 'text-blue-500' },
    delete: { icon: X, color: 'text-red-500' },
    execute_command: { icon: Activity, color: 'text-orange-500' },
    acknowledge_alert: { icon: Check, color: 'text-yellow-500' },
    resolve_alert: { icon: Shield, color: 'text-green-500' },
    update_policy: { icon: Settings, color: 'text-indigo-500' },
    isolate_network: { icon: AlertTriangle, color: 'text-red-600' },
    restore_network: { icon: Check, color: 'text-green-600' },
    register: { icon: Check, color: 'text-green-500' },
    deploy_policy: { icon: Activity, color: 'text-blue-500' },
    revoke_token: { icon: X, color: 'text-red-500' },
    change_settings: { icon: Settings, color: 'text-gray-500' },
};

// Resource type badges
const RESOURCE_BADGES: Record<string, { label: string; color: string }> = {
    user: { label: 'User', color: 'bg-blue-500/10 text-blue-500 border border-blue-500/20' },
    agent: { label: 'Agent', color: 'bg-green-500/10 text-green-500 border border-green-500/20' },
    alert: { label: 'Alert', color: 'bg-orange-500/10 text-orange-500 border border-orange-500/20' },
    rule: { label: 'Rule', color: 'bg-rose-500/10 text-rose-500 border border-rose-500/20' },
    policy: { label: 'Policy', color: 'bg-indigo-500/10 text-indigo-500 border border-indigo-500/20' },
    command: { label: 'Command', color: 'bg-cyan-500/10 text-cyan-500 border border-cyan-500/20' },
    session: { label: 'Session', color: 'bg-purple-500/10 text-purple-500 border border-purple-500/20' },
    system: { label: 'System', color: 'bg-slate-500/10 text-slate-500 border border-slate-500/20' },
    dashboard: { label: 'Dashboard', color: 'bg-teal-500/10 text-teal-500 border border-teal-500/20' },
    settings: { label: 'Settings', color: 'bg-gray-500/10 text-gray-400 border border-gray-500/20' },
    token: { label: 'Token', color: 'bg-yellow-500/10 text-yellow-500 border border-yellow-500/20' },
};

// Format action name for display
function formatAction(action: string): string {
    return action
        .split('_')
        .map(word => word.charAt(0).toUpperCase() + word.slice(1))
        .join(' ');
}

// Audit Detail Modal
function AuditDetailModal({ log, isOpen, onClose }: { log: AuditLog | null; isOpen: boolean; onClose: () => void }) {
    if (!log) return null;

    const iconConfig = ACTION_ICONS[log.action] || { icon: Activity, color: 'text-gray-500' };
    const Icon = iconConfig.icon;

    return (
        <Modal isOpen={isOpen} onClose={onClose} title="Audit Log Details" size="lg">
            <div className="space-y-4">
                {/* Header */}
                <div className="flex items-center gap-4 p-4 bg-gray-50 dark:bg-gray-900/50 rounded-lg">
                    <div className={`p-3 rounded-lg ${log.result === 'success' ? 'bg-green-100 dark:bg-green-900/30' : 'bg-red-100 dark:bg-red-900/30'}`}>
                        <Icon className={`w-6 h-6 ${iconConfig.color}`} />
                    </div>
                    <div className="flex-1">
                        <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
                            {formatAction(log.action)}
                        </h3>
                        <p className="text-sm text-gray-500">
                            {new Date(log.timestamp).toLocaleString()}
                        </p>
                    </div>
                    <span className={`px-2.5 py-1 text-xs font-semibold rounded-md border ${log.result === 'success' ? 'bg-green-500/10 text-green-500 border-green-500/20' : 'bg-rose-500/10 text-rose-500 border-rose-500/20'}`}>
                        {log.result}
                    </span>
                </div>

                {/* Details Grid */}
                <div className="grid grid-cols-2 gap-4">
                    <div>
                        <label className="text-xs text-gray-500 uppercase tracking-wider">User</label>
                        <p className="font-medium text-gray-900 dark:text-white">{log.username}</p>
                        <p className="text-xs text-gray-500">{log.user_id}</p>
                    </div>
                    <div>
                        <label className="text-xs text-gray-500 uppercase tracking-wider">Resource</label>
                        <p className="font-medium text-gray-900 dark:text-white">{log.resource_type}</p>
                        <p className="text-xs text-gray-500 font-mono">{log.resource_id || 'N/A'}</p>
                    </div>
                    <div>
                        <label className="text-xs text-gray-500 uppercase tracking-wider">IP Address</label>
                        <p className="font-mono text-gray-900 dark:text-white">{log.ip_address || 'N/A'}</p>
                    </div>
                    <div>
                        <label className="text-xs text-gray-500 uppercase tracking-wider">Log ID</label>
                        <p className="font-mono text-xs text-gray-600 dark:text-gray-400">{log.id}</p>
                    </div>
                </div>

                {/* User Agent */}
                {log.user_agent && (
                    <div>
                        <label className="text-xs text-gray-500 uppercase tracking-wider">User Agent</label>
                        <p className="text-sm text-gray-600 dark:text-gray-400 font-mono break-all">
                            {log.user_agent}
                        </p>
                    </div>
                )}

                {/* Error Message */}
                {log.error_message && (
                    <div className="p-3 bg-red-50 dark:bg-red-900/20 rounded-lg border border-red-200 dark:border-red-800">
                        <label className="text-xs text-red-600 uppercase tracking-wider">Error</label>
                        <p className="text-sm text-red-700 dark:text-red-300">{log.error_message}</p>
                    </div>
                )}

                {/* Details */}
                {log.details && (
                    <div>
                        <label className="text-xs text-gray-500 uppercase tracking-wider">Details</label>
                        <p className="text-sm text-gray-700 dark:text-gray-300">{log.details}</p>
                    </div>
                )}

                {/* Old/New Value Diff */}
                {(log.old_value || log.new_value) && (
                    <div className="grid grid-cols-2 gap-4">
                        {log.old_value && (
                            <div>
                                <label className="text-xs text-gray-500 uppercase tracking-wider">Old Value</label>
                                <pre className="mt-1 p-3 text-xs bg-red-50 dark:bg-red-900/20 rounded-lg overflow-auto max-h-40 text-gray-800 dark:text-gray-200">
                                    {JSON.stringify(log.old_value, null, 2)}
                                </pre>
                            </div>
                        )}
                        {log.new_value && (
                            <div>
                                <label className="text-xs text-gray-500 uppercase tracking-wider">New Value</label>
                                <pre className="mt-1 p-3 text-xs bg-green-50 dark:bg-green-900/20 rounded-lg overflow-auto max-h-40 text-gray-800 dark:text-gray-200">
                                    {JSON.stringify(log.new_value, null, 2)}
                                </pre>
                            </div>
                        )}
                    </div>
                )}
            </div>
        </Modal>
    );
}

// Export functionality
function exportToCSV(logs: AuditLog[]) {
    const headers = ['Timestamp', 'User', 'Action', 'Resource Type', 'Resource ID', 'Result', 'IP Address'];
    const rows = logs.map(log => [
        new Date(log.timestamp).toISOString(),
        log.username,
        log.action,
        log.resource_type,
        log.resource_id || '',
        log.result,
        log.ip_address || ''
    ]);

    const csv = [headers.join(','), ...rows.map(row => row.map(cell => `"${cell}"`).join(','))].join('\n');
    const blob = new Blob([csv], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);

    const a = document.createElement('a');
    a.href = url;
    a.download = `audit-logs-${new Date().toISOString().split('T')[0]}.csv`;
    a.click();
    URL.revokeObjectURL(url);
}

// Main Audit Logs Page
export default function AuditLogs() {
    const [selectedLog, setSelectedLog] = useState<AuditLog | null>(null);
    const [dateRange, setDateRange] = useState<DateRange>({
        from: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000),
        to: new Date(),
    });
    const [filters, setFilters] = useState({
        action: '',
        resource_type: '',
        user_id: '',
        search: '',
    });

    const debouncedSearch = useDebounce(filters.search, 300);

    // Check permissions
    const canViewAudit = authApi.canViewAuditLogs();

    // Fetch audit logs
    const { data, isLoading, error } = useQuery({
        queryKey: ['auditLogs', filters.action, filters.resource_type, filters.user_id, debouncedSearch, dateRange],
        queryFn: () => auditApi.list({
            limit: 100,
            action: filters.action || undefined,
            resource_type: filters.resource_type || undefined,
            user_id: filters.user_id || undefined,
            from: dateRange.from?.toISOString(),
            to: dateRange.to?.toISOString(),
        }),
        enabled: canViewAudit,
    });

    const logs = data?.data || [];

    const filteredLogs = useMemo(() => {
        if (!debouncedSearch) return logs;
        const search = debouncedSearch.toLowerCase();
        return logs.filter((log) => 
            log.username?.toLowerCase().includes(search) ||
            log.action?.toLowerCase().includes(search) ||
            log.resource_type?.toLowerCase().includes(search) ||
            log.resource_id?.toLowerCase().includes(search) ||
            (log.ip_address || '').toLowerCase().includes(search)
        );
    }, [logs, debouncedSearch]);

    // Access denied
    if (!canViewAudit) {
        return (
            <div className="card text-center py-12">
                <Shield className="w-12 h-12 text-gray-400 mx-auto mb-4" />
                <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">
                    Access Denied
                </h3>
                <p className="text-gray-500">
                    You don't have permission to view audit logs.
                </p>
                <p className="text-sm text-gray-400 mt-2">
                    Required role: Admin or Security
                </p>
            </div>
        );
    }

    if (error) {
        return (
            <div className="card text-center py-12">
                <AlertTriangle className="w-12 h-12 text-red-400 mx-auto mb-4" />
                <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">
                    Failed to Load Audit Logs
                </h3>
                <p className="text-gray-500">Please try again later.</p>
            </div>
        );
    }

    return (
        <div className="relative flex flex-col min-h-[calc(100vh-2rem)] lg:min-h-[calc(100vh-1rem)] h-full -mx-4 sm:-mx-6 lg:-mx-8 -my-4 sm:-my-6 lg:-my-8 p-4 sm:p-6 lg:p-8 bg-slate-50 dark:bg-gradient-to-br dark:from-slate-900 dark:via-[#0b1120] dark:to-slate-900 transition-colors overflow-hidden">
            {/* Background ambient glow matching Alerts/Endpoints */}
            <div className="absolute top-0 right-0 w-[500px] h-[500px] pointer-events-none mix-blend-screen" style={{ background: 'radial-gradient(circle, rgba(6,182,212,0.08) 0%, transparent 70%)' }} />

            <div className="relative flex-1 flex flex-col min-h-0 space-y-4 lg:space-y-6 max-w-[1600px] mx-auto w-full">
                
                <div className="flex items-center justify-between shrink-0">
                    <div>
                        <h1 className="text-2xl font-bold bg-clip-text text-transparent bg-gradient-to-r from-gray-900 to-gray-600 dark:from-white dark:to-gray-300">
                            Audit Logs
                        </h1>
                        <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">Platform Activity & Identity Tracking</p>
                    </div>
                    <button
                        onClick={() => exportToCSV(logs)}
                        disabled={logs.length === 0}
                        className="btn bg-white/60 dark:bg-slate-800/60 backdrop-blur border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-gray-300 hover:bg-slate-100 dark:hover:bg-slate-700 transition-all flex items-center gap-2 disabled:opacity-50"
                    >
                        <Download className="w-4 h-4" />
                        Export CSV
                    </button>
                </div>

                {/* Filters */}
                <div className="relative z-20 shrink-0 bg-white dark:bg-slate-900/50 border border-slate-200/80 dark:border-slate-700/50 rounded-xl p-4 shadow-sm">
                    <div className="flex flex-wrap gap-4 items-end">
                        <DateRangePicker
                            value={dateRange}
                            onChange={setDateRange}
                            label="Date Range"
                        />

                        <div className="relative">
                            <label className="block text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider mb-2">
                                Action
                            </label>
                            <div className="relative">
                                <select
                                    value={filters.action}
                                    onChange={(e) => setFilters({ ...filters, action: e.target.value })}
                                    className="appearance-none w-48 bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-700/80 text-slate-700 dark:text-slate-200 rounded-lg pl-3 pr-8 py-2 text-sm focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500 transition-all hover:bg-white dark:hover:bg-slate-800 cursor-pointer"
                                >
                                    <option value="">All Actions</option>
                                    <option value="login">Login</option>
                                    <option value="logout">Logout</option>
                                    <option value="create">Create</option>
                                    <option value="update">Update</option>
                                    <option value="delete">Delete</option>
                                    <option value="execute_command">Execute Command</option>
                                    <option value="acknowledge_alert">Acknowledge Alert</option>
                                    <option value="resolve_alert">Resolve Alert</option>
                                    <option value="isolate_network">Isolate</option>
                                    <option value="deploy_policy">Deploy Policy</option>
                                    <option value="revoke_token">Revoke Token</option>
                                    <option value="change_settings">Change Settings</option>
                                    <option value="register">Register</option>
                                </select>
                                <ChevronDown className="absolute right-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-400 pointer-events-none" />
                            </div>
                        </div>

                        <div className="relative">
                            <label className="block text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider mb-2">
                                Resource Type
                            </label>
                            <div className="relative">
                                <select
                                    value={filters.resource_type}
                                    onChange={(e) => setFilters({ ...filters, resource_type: e.target.value })}
                                    className="appearance-none w-44 bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-700/80 text-slate-700 dark:text-slate-200 rounded-lg pl-3 pr-8 py-2 text-sm focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500 transition-all hover:bg-white dark:hover:bg-slate-800 cursor-pointer"
                                >
                                    <option value="">All Types</option>
                                    <option value="user">User</option>
                                    <option value="agent">Agent</option>
                                    <option value="alert">Alert</option>
                                    <option value="rule">Rule</option>
                                    <option value="policy">Policy</option>
                                    <option value="command">Command</option>
                                    <option value="system">System</option>
                                    <option value="dashboard">Dashboard</option>
                                    <option value="settings">Settings</option>
                                    <option value="token">Token</option>
                                </select>
                                <ChevronDown className="absolute right-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-400 pointer-events-none" />
                            </div>
                        </div>

                        <div className="flex-1 min-w-[200px]">
                            <label className="block text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider mb-2">
                                Search Identity
                            </label>
                            <div className="relative">
                                <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-400" />
                                <input
                                    type="text"
                                    placeholder="Search by user, resource..."
                                    value={filters.search}
                                    onChange={(e) => setFilters({ ...filters, search: e.target.value })}
                                    className="w-full bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-700/80 text-slate-700 dark:text-slate-200 rounded-lg pl-9 pr-3 py-2 text-sm focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500 transition-all hover:bg-white dark:hover:bg-slate-800"
                                />
                            </div>
                        </div>
                    </div>
                </div>

                {/* Logs Table */}
                <div className="relative flex-1 flex flex-col min-h-0 bg-white dark:bg-slate-800/70 rounded-2xl border border-slate-200 dark:border-slate-700/60 shadow-sm overflow-hidden mt-2">
                    {isLoading ? (
                        <div className="p-4">
                            <SkeletonTable rows={10} columns={6} />
                        </div>
                    ) : filteredLogs.length === 0 ? (
                        <div className="text-center py-12 flex-1 flex flex-col items-center justify-center">
                            <Activity className="w-12 h-12 text-gray-400 mx-auto mb-4 opacity-50" />
                            <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">
                                No Audit Logs Found
                            </h3>
                            <p className="text-gray-500">
                                Try adjusting your identity/action filters or widen the date range.
                            </p>
                        </div>
                    ) : (
                        <div className="flex-1 overflow-auto custom-scrollbar transform-gpu">
                            <table className="w-full text-left border-collapse">
                                <thead className="sticky top-0 z-10 bg-slate-100 dark:bg-slate-800 border-b-2 border-slate-200 dark:border-slate-700/80">
                                    <tr>
                                        <th className="py-3 px-4 text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider">Timestamp</th>
                                        <th className="py-3 px-4 text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider">User</th>
                                        <th className="py-3 px-4 text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider">Action</th>
                                        <th className="py-3 px-4 text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider">Resource</th>
                                        <th className="py-3 px-4 text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider">Result</th>
                                        <th className="py-3 px-4 text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider text-right">Actions</th>
                                    </tr>
                                </thead>
                                <tbody>
                                {filteredLogs.map((log) => {
                                    const iconConfig = ACTION_ICONS[log.action] || { icon: Activity, color: 'text-gray-500' };
                                    const Icon = iconConfig.icon;
                                    const resourceBadge = RESOURCE_BADGES[log.resource_type] || { label: log.resource_type, color: 'badge-info' };

                                    return (
                                        <tr key={log.id} className="border-b border-slate-100 dark:border-slate-800/60 hover:bg-slate-50 dark:hover:bg-slate-800/40 transition-colors group">
                                            <td className="py-4 px-4 whitespace-nowrap">
                                                <div className="text-sm font-medium text-slate-900 dark:text-slate-200">
                                                    {new Date(log.timestamp).toLocaleDateString()}
                                                </div>
                                                <div className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
                                                    {new Date(log.timestamp).toLocaleTimeString()}
                                                </div>
                                            </td>
                                            <td className="py-4 px-4">
                                                <div className="flex items-center gap-3">
                                                    <div className="w-8 h-8 rounded-full bg-slate-200 dark:bg-slate-700 flex items-center justify-center shrink-0">
                                                        <User className="w-4 h-4 text-slate-500" />
                                                    </div>
                                                    <div>
                                                        <p className="font-semibold text-slate-900 dark:text-slate-200 text-sm">
                                                            {log.username}
                                                        </p>
                                                        <p className="text-xs text-slate-500 font-mono mt-0.5">
                                                            {log.ip_address || 'N/A'}
                                                        </p>
                                                    </div>
                                                </div>
                                            </td>
                                            <td className="py-4 px-4">
                                                <div className="flex items-center gap-2">
                                                    <Icon className={`w-4 h-4 ${iconConfig.color}`} />
                                                    <span className="text-sm font-medium text-slate-700 dark:text-slate-300">
                                                        {formatAction(log.action)}
                                                    </span>
                                                </div>
                                            </td>
                                            <td className="py-4 px-4">
                                                <span className={`px-2 py-0.5 rounded text-[11px] font-semibold tracking-wide uppercase ${resourceBadge.color}`}>
                                                    {resourceBadge.label}
                                                </span>
                                                {log.resource_id && (
                                                    <p className="text-[11px] text-slate-500 dark:text-slate-400 font-mono mt-1 w-24 truncate" title={log.resource_id}>
                                                        {log.resource_id}
                                                    </p>
                                                )}
                                            </td>
                                            <td className="py-4 px-4">
                                                <span className={`inline-flex items-center gap-1 px-2 py-1 rounded text-xs font-semibold border ${log.result === 'success' ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/20' : 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border-rose-500/20'}`}>
                                                    {log.result === 'success' ? (
                                                        <Check className="w-3 h-3" />
                                                    ) : (
                                                        <X className="w-3 h-3" />
                                                    )}
                                                    {log.result}
                                                </span>
                                            </td>
                                            <td className="py-4 px-4 text-right">
                                                <button
                                                    onClick={() => setSelectedLog(log)}
                                                    className="p-1.5 text-slate-400 hover:text-cyan-500 hover:bg-cyan-50 dark:hover:bg-cyan-500/10 rounded transition-colors"
                                                    title="View Full Identity Trace"
                                                >
                                                    <Eye className="w-4 h-4" />
                                                </button>
                                            </td>
                                        </tr>
                                    );
                                })}
                            </tbody>
                        </table>
                    </div>
                )}

                {/* Footer Pagination Strip */}
                <div className="shrink-0 px-4 py-3 bg-slate-50/50 dark:bg-slate-900/40 border-t border-slate-200 dark:border-slate-800/60 text-sm text-slate-500 flex justify-between items-center">
                    <span>Showing {logs.length} trace items</span>
                </div>
            </div>

            {/* Detail Modal */}
            <AuditDetailModal
                log={selectedLog}
                isOpen={!!selectedLog}
                onClose={() => setSelectedLog(null)}
            />
        </div>
        </div>
    );
}
