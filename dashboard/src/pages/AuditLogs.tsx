import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import {
    Search, Download, Eye, User, Activity,
    Check, X, RefreshCw, Shield, AlertTriangle, Settings
} from 'lucide-react';
import { auditApi, authApi, type AuditLog } from '../api/client';
import { Modal, DateRangePicker, type DateRange, SkeletonTable } from '../components';

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
};

// Resource type badges
const RESOURCE_BADGES: Record<string, { label: string; color: string }> = {
    user: { label: 'User', color: 'badge-info' },
    agent: { label: 'Agent', color: 'badge-success' },
    alert: { label: 'Alert', color: 'badge-warning' },
    rule: { label: 'Rule', color: 'badge-danger' },
    policy: { label: 'Policy', color: 'badge-info' },
    command: { label: 'Command', color: 'badge-warning' },
    session: { label: 'Session', color: 'badge-success' },
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
                    <span className={`badge ${log.result === 'success' ? 'badge-success' : 'badge-danger'}`}>
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

    // Check permissions
    const canViewAudit = authApi.canViewAuditLogs();

    // Fetch audit logs
    const { data, isLoading, error } = useQuery({
        queryKey: ['auditLogs', filters, dateRange],
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
        <div className="space-y-6">
            <div className="flex items-center justify-between">
                <h1 className="text-3xl font-bold text-gray-900 dark:text-white">Audit Logs</h1>
                <button
                    onClick={() => exportToCSV(logs)}
                    disabled={logs.length === 0}
                    className="btn btn-secondary flex items-center gap-2 disabled:opacity-50"
                >
                    <Download className="w-4 h-4" />
                    Export CSV
                </button>
            </div>

            {/* Filters */}
            <div className="card">
                <div className="flex flex-wrap gap-4 items-end">
                    <DateRangePicker
                        value={dateRange}
                        onChange={setDateRange}
                        label="Date Range"
                    />

                    <div>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                            Action
                        </label>
                        <select
                            value={filters.action}
                            onChange={(e) => setFilters({ ...filters, action: e.target.value })}
                            className="input w-40"
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
                        </select>
                    </div>

                    <div>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                            Resource Type
                        </label>
                        <select
                            value={filters.resource_type}
                            onChange={(e) => setFilters({ ...filters, resource_type: e.target.value })}
                            className="input w-36"
                        >
                            <option value="">All Types</option>
                            <option value="user">User</option>
                            <option value="agent">Agent</option>
                            <option value="alert">Alert</option>
                            <option value="rule">Rule</option>
                            <option value="policy">Policy</option>
                            <option value="command">Command</option>
                        </select>
                    </div>

                    <div className="flex-1 min-w-[200px]">
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                            Search
                        </label>
                        <div className="relative">
                            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
                            <input
                                type="text"
                                placeholder="Search by user, resource..."
                                value={filters.search}
                                onChange={(e) => setFilters({ ...filters, search: e.target.value })}
                                className="input pl-9"
                            />
                        </div>
                    </div>
                </div>
            </div>

            {/* Logs Table */}
            <div className="card overflow-hidden p-0">
                {isLoading ? (
                    <div className="p-4">
                        <SkeletonTable rows={10} columns={6} />
                    </div>
                ) : logs.length === 0 ? (
                    <div className="text-center py-12">
                        <Activity className="w-12 h-12 text-gray-400 mx-auto mb-4" />
                        <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">
                            No Audit Logs Found
                        </h3>
                        <p className="text-gray-500">
                            Try adjusting your filters or date range
                        </p>
                    </div>
                ) : (
                    <div className="overflow-x-auto">
                        <table className="table">
                            <thead className="bg-gray-50 dark:bg-gray-800">
                                <tr>
                                    <th>Timestamp</th>
                                    <th>User</th>
                                    <th>Action</th>
                                    <th>Resource</th>
                                    <th>Result</th>
                                    <th>Actions</th>
                                </tr>
                            </thead>
                            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                                {logs.map((log) => {
                                    const iconConfig = ACTION_ICONS[log.action] || { icon: Activity, color: 'text-gray-500' };
                                    const Icon = iconConfig.icon;
                                    const resourceBadge = RESOURCE_BADGES[log.resource_type] || { label: log.resource_type, color: 'badge-info' };

                                    return (
                                        <tr key={log.id} className="hover:bg-gray-50 dark:hover:bg-gray-800">
                                            <td className="whitespace-nowrap">
                                                <div className="text-sm text-gray-900 dark:text-white">
                                                    {new Date(log.timestamp).toLocaleDateString()}
                                                </div>
                                                <div className="text-xs text-gray-500">
                                                    {new Date(log.timestamp).toLocaleTimeString()}
                                                </div>
                                            </td>
                                            <td>
                                                <div className="flex items-center gap-2">
                                                    <div className="w-8 h-8 rounded-full bg-gray-200 dark:bg-gray-700 flex items-center justify-center">
                                                        <User className="w-4 h-4 text-gray-500" />
                                                    </div>
                                                    <div>
                                                        <p className="font-medium text-gray-900 dark:text-white text-sm">
                                                            {log.username}
                                                        </p>
                                                        <p className="text-xs text-gray-500">
                                                            {log.ip_address || 'N/A'}
                                                        </p>
                                                    </div>
                                                </div>
                                            </td>
                                            <td>
                                                <div className="flex items-center gap-2">
                                                    <Icon className={`w-4 h-4 ${iconConfig.color}`} />
                                                    <span className="text-sm text-gray-700 dark:text-gray-300">
                                                        {formatAction(log.action)}
                                                    </span>
                                                </div>
                                            </td>
                                            <td>
                                                <span className={`badge ${resourceBadge.color}`}>
                                                    {resourceBadge.label}
                                                </span>
                                                {log.resource_id && (
                                                    <p className="text-xs text-gray-500 font-mono mt-0.5">
                                                        {log.resource_id.slice(0, 12)}...
                                                    </p>
                                                )}
                                            </td>
                                            <td>
                                                <span className={`badge ${log.result === 'success' ? 'badge-success' : 'badge-danger'}`}>
                                                    {log.result === 'success' ? (
                                                        <Check className="w-3 h-3 mr-1" />
                                                    ) : (
                                                        <X className="w-3 h-3 mr-1" />
                                                    )}
                                                    {log.result}
                                                </span>
                                            </td>
                                            <td>
                                                <button
                                                    onClick={() => setSelectedLog(log)}
                                                    className="p-1 text-gray-500 hover:text-primary-600 hover:bg-gray-100 dark:hover:bg-gray-700 rounded"
                                                    title="View Details"
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

                {/* Footer */}
                <div className="px-4 py-3 bg-gray-50 dark:bg-gray-800 border-t border-gray-200 dark:border-gray-700 text-sm text-gray-500">
                    Showing {logs.length} logs
                </div>
            </div>

            {/* Detail Modal */}
            <AuditDetailModal
                log={selectedLog}
                isOpen={!!selectedLog}
                onClose={() => setSelectedLog(null)}
            />
        </div>
    );
}
