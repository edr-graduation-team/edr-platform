import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import {
    Search, Check, Eye, X, ChevronLeft, ChevronRight,
    AlertTriangle, Clock, CheckCircle, XCircle, Shield, ArrowUpDown
} from 'lucide-react';
import { alertsApi, type Alert } from '../api/client';
import {
    Modal, MultiSelect, DateRangePicker, type DateRange, type MultiSelectOption,
    useToast, SkeletonTable
} from '../components';

// Severity options with counts
const SEVERITY_OPTIONS: MultiSelectOption[] = [
    { value: 'critical', label: 'Critical', color: '#ef4444' },
    { value: 'high', label: 'High', color: '#f97316' },
    { value: 'medium', label: 'Medium', color: '#eab308' },
    { value: 'low', label: 'Low', color: '#6366f1' },
    { value: 'informational', label: 'Info', color: '#3b82f6' },
];

// Status options
const STATUS_OPTIONS: MultiSelectOption[] = [
    { value: 'open', label: 'Open' },
    { value: 'in_progress', label: 'In Progress' },
    { value: 'acknowledged', label: 'Acknowledged' },
    { value: 'resolved', label: 'Resolved' },
    { value: 'false_positive', label: 'False Positive' },
];

// Severity badge colors
const severityColors: Record<string, string> = {
    critical: 'badge-critical',
    high: 'badge-high',
    medium: 'badge-medium',
    low: 'badge-low',
    informational: 'badge-info',
};

// Status badge colors
const statusColors: Record<string, string> = {
    open: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200',
    in_progress: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200',
    acknowledged: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200',
    resolved: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200',
    false_positive: 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200',
    closed: 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200',
};

// Status icons
const statusIcons: Record<string, typeof AlertTriangle> = {
    open: AlertTriangle,
    in_progress: Clock,
    acknowledged: Eye,
    resolved: CheckCircle,
    false_positive: XCircle,
    closed: XCircle,
};

// Alert Detail Modal with Tabs
function AlertDetailModal({
    alert,
    isOpen,
    onClose,
    onStatusChange
}: {
    alert: Alert | null;
    isOpen: boolean;
    onClose: () => void;
    onStatusChange: (id: string, status: string) => void;
}) {
    const [activeTab, setActiveTab] = useState<'summary' | 'event' | 'mitre' | 'actions'>('summary');

    if (!alert) return null;

    const tabs = [
        { id: 'summary', label: 'Summary' },
        { id: 'event', label: 'Event Details' },
        { id: 'mitre', label: 'MITRE ATT&CK' },
        { id: 'actions', label: 'Actions' },
    ];

    return (
        <Modal isOpen={isOpen} onClose={onClose} title="Alert Details" size="lg">
            {/* Tabs */}
            <div className="flex border-b border-gray-200 dark:border-gray-700 -mx-6 px-6 mb-4">
                {tabs.map((tab) => (
                    <button
                        key={tab.id}
                        onClick={() => setActiveTab(tab.id as typeof activeTab)}
                        className={`tab ${activeTab === tab.id ? 'tab-active' : ''}`}
                    >
                        {tab.label}
                    </button>
                ))}
            </div>

            {/* Summary Tab */}
            {activeTab === 'summary' && (
                <div className="space-y-4">
                    <div className="grid grid-cols-2 gap-4">
                        <div>
                            <label className="text-xs text-gray-500 uppercase tracking-wider">Rule</label>
                            <p className="font-medium text-gray-900 dark:text-white">{alert.rule_title}</p>
                        </div>
                        <div>
                            <label className="text-xs text-gray-500 uppercase tracking-wider">Category</label>
                            <p className="text-gray-700 dark:text-gray-300">{alert.category || 'N/A'}</p>
                        </div>
                        <div>
                            <label className="text-xs text-gray-500 uppercase tracking-wider">Agent ID</label>
                            <p className="font-mono text-sm text-gray-700 dark:text-gray-300">{alert.agent_id}</p>
                        </div>
                        <div>
                            <label className="text-xs text-gray-500 uppercase tracking-wider">Severity</label>
                            <p><span className={`badge ${severityColors[alert.severity]}`}>{alert.severity.toUpperCase()}</span></p>
                        </div>
                        <div>
                            <label className="text-xs text-gray-500 uppercase tracking-wider">Status</label>
                            <p><span className={`badge ${statusColors[alert.status]}`}>{alert.status.replace('_', ' ')}</span></p>
                        </div>
                        <div>
                            <label className="text-xs text-gray-500 uppercase tracking-wider">Confidence</label>
                            <p className="text-gray-700 dark:text-gray-300">{(alert.confidence * 100).toFixed(1)}%</p>
                        </div>
                        <div>
                            <label className="text-xs text-gray-500 uppercase tracking-wider">Event Count</label>
                            <p className="text-gray-700 dark:text-gray-300">{alert.event_count}</p>
                        </div>
                        <div>
                            <label className="text-xs text-gray-500 uppercase tracking-wider">Detected At</label>
                            <p className="text-gray-700 dark:text-gray-300">{new Date(alert.timestamp).toLocaleString()}</p>
                        </div>
                    </div>

                    {/* Notes */}
                    {alert.notes && (
                        <div>
                            <label className="text-xs text-gray-500 uppercase tracking-wider">Notes</label>
                            <p className="text-gray-700 dark:text-gray-300 mt-1">{alert.notes}</p>
                        </div>
                    )}
                </div>
            )}

            {/* Event Details Tab */}
            {activeTab === 'event' && (
                <div>
                    <label className="text-xs text-gray-500 uppercase tracking-wider">Raw Event Data</label>
                    <pre className="mt-2 p-4 bg-gray-100 dark:bg-gray-900 rounded-lg overflow-auto max-h-96 text-xs font-mono text-gray-800 dark:text-gray-200">
                        {JSON.stringify(alert.event_data || alert.matched_fields || {}, null, 2)}
                    </pre>
                </div>
            )}

            {/* MITRE ATT&CK Tab */}
            {activeTab === 'mitre' && (
                <div className="space-y-4">
                    <div>
                        <label className="text-xs text-gray-500 uppercase tracking-wider">Tactics</label>
                        <div className="flex flex-wrap gap-2 mt-2">
                            {(alert.mitre_tactics || []).length > 0 ? (
                                alert.mitre_tactics?.map((tactic) => (
                                    <span key={tactic} className="badge badge-warning">{tactic}</span>
                                ))
                            ) : (
                                <span className="text-sm text-gray-400">No tactics identified</span>
                            )}
                        </div>
                    </div>
                    <div>
                        <label className="text-xs text-gray-500 uppercase tracking-wider">Techniques</label>
                        <div className="flex flex-wrap gap-2 mt-2">
                            {(alert.mitre_techniques || []).length > 0 ? (
                                alert.mitre_techniques?.map((technique) => (
                                    <span key={technique} className="badge badge-info">{technique}</span>
                                ))
                            ) : (
                                <span className="text-sm text-gray-400">No techniques identified</span>
                            )}
                        </div>
                    </div>
                </div>
            )}

            {/* Actions Tab */}
            {activeTab === 'actions' && (
                <div className="space-y-4">
                    <p className="text-sm text-gray-600 dark:text-gray-400">
                        Update the alert status to track investigation progress.
                    </p>
                    <div className="grid grid-cols-2 gap-3">
                        {alert.status === 'open' && (
                            <>
                                <button
                                    onClick={() => onStatusChange(alert.id, 'acknowledged')}
                                    className="btn btn-primary flex items-center justify-center gap-2"
                                >
                                    <Check className="w-4 h-4" />
                                    Acknowledge
                                </button>
                                <button
                                    onClick={() => onStatusChange(alert.id, 'in_progress')}
                                    className="btn btn-warning flex items-center justify-center gap-2"
                                >
                                    <Clock className="w-4 h-4" />
                                    Start Investigation
                                </button>
                            </>
                        )}
                        {(alert.status === 'acknowledged' || alert.status === 'in_progress') && (
                            <>
                                <button
                                    onClick={() => onStatusChange(alert.id, 'resolved')}
                                    className="btn btn-success flex items-center justify-center gap-2"
                                >
                                    <CheckCircle className="w-4 h-4" />
                                    Resolve
                                </button>
                                <button
                                    onClick={() => onStatusChange(alert.id, 'false_positive')}
                                    className="btn btn-secondary flex items-center justify-center gap-2"
                                >
                                    <XCircle className="w-4 h-4" />
                                    False Positive
                                </button>
                            </>
                        )}
                        {(alert.status === 'resolved' || alert.status === 'false_positive') && (
                            <button
                                onClick={() => onStatusChange(alert.id, 'open')}
                                className="btn btn-secondary flex items-center justify-center gap-2"
                            >
                                <AlertTriangle className="w-4 h-4" />
                                Reopen
                            </button>
                        )}
                    </div>
                </div>
            )}
        </Modal>
    );
}

// Bulk Actions Toolbar
function BulkActionsToolbar({
    selectedCount,
    onAction,
    onClear
}: {
    selectedCount: number;
    onAction: (action: string) => void;
    onClear: () => void;
}) {
    if (selectedCount === 0) return null;

    return (
        <div className="flex items-center gap-4 p-3 bg-primary-50 dark:bg-primary-900/20 rounded-lg mb-4 animate-slide-up">
            <span className="text-sm font-medium text-primary-700 dark:text-primary-300">
                {selectedCount} alert(s) selected
            </span>
            <div className="flex-1" />
            <button onClick={() => onAction('acknowledged')} className="btn btn-sm btn-secondary">
                Acknowledge
            </button>
            <button onClick={() => onAction('resolved')} className="btn btn-sm btn-success">
                Resolve
            </button>
            <button onClick={() => onAction('false_positive')} className="btn btn-sm btn-secondary">
                False Positive
            </button>
            <button onClick={onClear} className="p-1 text-gray-500 hover:text-gray-700">
                <X className="w-4 h-4" />
            </button>
        </div>
    );
}

// Pagination Component
function Pagination({
    page,
    totalPages,
    pageSize,
    total,
    onPageChange,
    onPageSizeChange
}: {
    page: number;
    totalPages: number;
    pageSize: number;
    total: number;
    onPageChange: (page: number) => void;
    onPageSizeChange: (size: number) => void;
}) {
    return (
        <div className="flex items-center justify-between px-4 py-3 bg-gray-50 dark:bg-gray-800 border-t border-gray-200 dark:border-gray-700">
            <div className="flex items-center gap-2 text-sm text-gray-500">
                <span>Rows per page:</span>
                <select
                    value={pageSize}
                    onChange={(e) => onPageSizeChange(Number(e.target.value))}
                    className="input w-20 py-1 text-sm"
                >
                    <option value={25}>25</option>
                    <option value={50}>50</option>
                    <option value={100}>100</option>
                </select>
                <span className="ml-4">
                    Showing {((page - 1) * pageSize) + 1}-{Math.min(page * pageSize, total)} of {total}
                </span>
            </div>
            <div className="flex items-center gap-2">
                <button
                    onClick={() => onPageChange(page - 1)}
                    disabled={page <= 1}
                    className="p-2 rounded hover:bg-gray-200 dark:hover:bg-gray-700 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                    <ChevronLeft className="w-4 h-4" />
                </button>
                <span className="text-sm text-gray-600 dark:text-gray-300">
                    Page {page} of {totalPages || 1}
                </span>
                <button
                    onClick={() => onPageChange(page + 1)}
                    disabled={page >= totalPages}
                    className="p-2 rounded hover:bg-gray-200 dark:hover:bg-gray-700 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                    <ChevronRight className="w-4 h-4" />
                </button>
            </div>
        </div>
    );
}

// Main Alerts Page
export default function Alerts() {
    const queryClient = useQueryClient();
    const { showToast } = useToast();
    const [selectedAlert, setSelectedAlert] = useState<Alert | null>(null);
    const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(25);
    const [sortBy, setSortBy] = useState<'timestamp' | 'severity'>('timestamp');
    const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');

    const [filters, setFilters] = useState({
        severities: [] as string[],
        statuses: [] as string[],
        search: '',
    });
    const [dateRange, setDateRange] = useState<DateRange>({
        from: new Date(Date.now() - 24 * 60 * 60 * 1000),
        to: new Date(),
    });

    // Fetch alerts
    const { data, isLoading, error } = useQuery({
        queryKey: ['alerts', filters, dateRange, page, pageSize, sortBy, sortOrder],
        queryFn: () => alertsApi.list({
            limit: pageSize,
            offset: (page - 1) * pageSize,
            severity: filters.severities.length === 1 ? filters.severities[0] : undefined,
            status: filters.statuses.length === 1 ? filters.statuses[0] : undefined,
            from: dateRange.from?.toISOString(),
            to: dateRange.to?.toISOString(),
            sort: sortBy,
            order: sortOrder,
        }),
        refetchInterval: 15000,
    });

    const alerts = data?.alerts || [];
    const total = data?.total || 0;
    const totalPages = Math.ceil(total / pageSize);

    // Filter by severity locally if multiple selected
    const filteredAlerts = alerts.filter((alert) => {
        if (filters.severities.length > 1 && !filters.severities.includes(alert.severity)) {
            return false;
        }
        if (filters.statuses.length > 1 && !filters.statuses.includes(alert.status)) {
            return false;
        }
        if (filters.search) {
            const search = filters.search.toLowerCase();
            return (
                alert.rule_title?.toLowerCase().includes(search) ||
                alert.agent_id?.toLowerCase().includes(search) ||
                alert.rule_id?.toLowerCase().includes(search)
            );
        }
        return true;
    });

    // Update status mutation
    const updateStatusMutation = useMutation({
        mutationFn: ({ id, status }: { id: string; status: string }) =>
            alertsApi.updateStatus(id, status),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['alerts'] });
            queryClient.invalidateQueries({ queryKey: ['alertStats'] });
            showToast('Alert status updated', 'success');
        },
        onError: () => {
            showToast('Failed to update alert status', 'error');
        },
    });

    // Bulk update mutation
    const bulkUpdateMutation = useMutation({
        mutationFn: ({ ids, status }: { ids: string[]; status: string }) =>
            alertsApi.bulkUpdateStatus(ids, status),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['alerts'] });
            queryClient.invalidateQueries({ queryKey: ['alertStats'] });
            setSelectedIds(new Set());
            showToast(`${selectedIds.size} alerts updated`, 'success');
        },
        onError: () => {
            showToast('Failed to update alerts', 'error');
        },
    });

    const handleStatusChange = (id: string, status: string) => {
        updateStatusMutation.mutate({ id, status });
        setSelectedAlert(null);
    };

    const handleBulkAction = (status: string) => {
        bulkUpdateMutation.mutate({ ids: Array.from(selectedIds), status });
    };

    const toggleSelectAll = () => {
        if (selectedIds.size === filteredAlerts.length) {
            setSelectedIds(new Set());
        } else {
            setSelectedIds(new Set(filteredAlerts.map((a) => a.id)));
        }
    };

    const toggleSelect = (id: string) => {
        const newSet = new Set(selectedIds);
        if (newSet.has(id)) {
            newSet.delete(id);
        } else {
            newSet.add(id);
        }
        setSelectedIds(newSet);
    };

    const toggleSort = (field: 'timestamp' | 'severity') => {
        if (sortBy === field) {
            setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
        } else {
            setSortBy(field);
            setSortOrder('desc');
        }
    };

    if (error) {
        return (
            <div className="card text-center py-12">
                <AlertTriangle className="w-12 h-12 text-red-400 mx-auto mb-4" />
                <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">
                    Failed to Load Alerts
                </h3>
                <p className="text-gray-500">Please try again later.</p>
            </div>
        );
    }

    return (
        <div className="space-y-6">
            <h1 className="text-3xl font-bold text-gray-900 dark:text-white">Alerts</h1>

            {/* Filters */}
            <div className="card">
                <div className="flex flex-wrap gap-4 items-end">
                    <MultiSelect
                        options={SEVERITY_OPTIONS}
                        selected={filters.severities}
                        onChange={(severities) => setFilters({ ...filters, severities })}
                        placeholder="All Severities"
                        label="Severity"
                    />
                    <MultiSelect
                        options={STATUS_OPTIONS}
                        selected={filters.statuses}
                        onChange={(statuses) => setFilters({ ...filters, statuses })}
                        placeholder="All Statuses"
                        label="Status"
                    />
                    <DateRangePicker
                        value={dateRange}
                        onChange={setDateRange}
                        label="Date Range"
                    />
                    <div className="flex-1 min-w-[200px]">
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                            Search
                        </label>
                        <div className="relative">
                            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
                            <input
                                type="text"
                                placeholder="Search by rule, agent..."
                                value={filters.search}
                                onChange={(e) => setFilters({ ...filters, search: e.target.value })}
                                className="input pl-9"
                            />
                        </div>
                    </div>
                </div>

                {/* Active filters */}
                {(filters.severities.length > 0 || filters.statuses.length > 0 || filters.search) && (
                    <div className="flex flex-wrap gap-2 mt-4 pt-4 border-t border-gray-200 dark:border-gray-700">
                        {filters.severities.map((sev) => (
                            <span key={sev} className="badge badge-info flex items-center gap-1">
                                {sev}
                                <button onClick={() => setFilters({
                                    ...filters,
                                    severities: filters.severities.filter((s) => s !== sev)
                                })}>
                                    <X className="w-3 h-3" />
                                </button>
                            </span>
                        ))}
                        {filters.statuses.map((status) => (
                            <span key={status} className="badge badge-info flex items-center gap-1">
                                {status.replace('_', ' ')}
                                <button onClick={() => setFilters({
                                    ...filters,
                                    statuses: filters.statuses.filter((s) => s !== status)
                                })}>
                                    <X className="w-3 h-3" />
                                </button>
                            </span>
                        ))}
                        {filters.search && (
                            <span className="badge badge-info flex items-center gap-1">
                                "{filters.search}"
                                <button onClick={() => setFilters({ ...filters, search: '' })}>
                                    <X className="w-3 h-3" />
                                </button>
                            </span>
                        )}
                        <button
                            onClick={() => setFilters({ severities: [], statuses: [], search: '' })}
                            className="text-xs text-primary-600 hover:underline"
                        >
                            Clear all
                        </button>
                    </div>
                )}
            </div>

            {/* Bulk Actions */}
            <BulkActionsToolbar
                selectedCount={selectedIds.size}
                onAction={handleBulkAction}
                onClear={() => setSelectedIds(new Set())}
            />

            {/* Alerts Table */}
            <div className="card overflow-hidden p-0">
                {isLoading ? (
                    <div className="p-4">
                        <SkeletonTable rows={10} columns={7} />
                    </div>
                ) : filteredAlerts.length === 0 ? (
                    <div className="text-center py-12">
                        <Shield className="w-12 h-12 text-green-400 mx-auto mb-4" />
                        <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">
                            No Alerts Found
                        </h3>
                        <p className="text-gray-500">
                            {filters.search || filters.severities.length || filters.statuses.length
                                ? 'Try adjusting your filters'
                                : 'All clear! No alerts in this time range.'}
                        </p>
                    </div>
                ) : (
                    <div className="overflow-x-auto">
                        <table className="table">
                            <thead className="bg-gray-50 dark:bg-gray-800">
                                <tr>
                                    <th className="w-10">
                                        <input
                                            type="checkbox"
                                            checked={selectedIds.size === filteredAlerts.length && filteredAlerts.length > 0}
                                            onChange={toggleSelectAll}
                                            className="rounded"
                                        />
                                    </th>
                                    <th>
                                        <button
                                            onClick={() => toggleSort('timestamp')}
                                            className="flex items-center gap-1 hover:text-gray-700 dark:hover:text-gray-200"
                                        >
                                            Time
                                            <ArrowUpDown className={`w-3 h-3 ${sortBy === 'timestamp' ? 'text-primary-500' : ''}`} />
                                        </button>
                                    </th>
                                    <th>Rule</th>
                                    <th>
                                        <button
                                            onClick={() => toggleSort('severity')}
                                            className="flex items-center gap-1 hover:text-gray-700 dark:hover:text-gray-200"
                                        >
                                            Severity
                                            <ArrowUpDown className={`w-3 h-3 ${sortBy === 'severity' ? 'text-primary-500' : ''}`} />
                                        </button>
                                    </th>
                                    <th>Status</th>
                                    <th>Agent</th>
                                    <th>Actions</th>
                                </tr>
                            </thead>
                            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                                {filteredAlerts.map((alert) => {
                                    const StatusIcon = statusIcons[alert.status] || AlertTriangle;
                                    return (
                                        <tr
                                            key={alert.id}
                                            className={`transition-colors ${selectedIds.has(alert.id)
                                                ? 'bg-primary-50 dark:bg-primary-900/20'
                                                : 'hover:bg-gray-50 dark:hover:bg-gray-800'
                                                }`}
                                        >
                                            <td>
                                                <input
                                                    type="checkbox"
                                                    checked={selectedIds.has(alert.id)}
                                                    onChange={() => toggleSelect(alert.id)}
                                                    className="rounded"
                                                />
                                            </td>
                                            <td className="whitespace-nowrap text-sm">
                                                {new Date(alert.timestamp).toLocaleString()}
                                            </td>
                                            <td>
                                                <div className="max-w-xs">
                                                    <p className="font-medium text-gray-900 dark:text-white truncate">
                                                        {alert.rule_title}
                                                    </p>
                                                    {alert.mitre_techniques?.[0] && (
                                                        <p className="text-xs text-primary-600 dark:text-primary-400">
                                                            {alert.mitre_techniques[0]}
                                                        </p>
                                                    )}
                                                </div>
                                            </td>
                                            <td>
                                                <span className={`badge ${severityColors[alert.severity]}`}>
                                                    {alert.severity.toUpperCase()}
                                                </span>
                                            </td>
                                            <td>
                                                <span className={`badge ${statusColors[alert.status]} flex items-center gap-1 w-fit`}>
                                                    <StatusIcon className="w-3 h-3" />
                                                    {alert.status.replace('_', ' ')}
                                                </span>
                                            </td>
                                            <td className="text-sm text-gray-500 font-mono">
                                                {alert.agent_id?.slice(0, 12)}...
                                            </td>
                                            <td>
                                                <div className="flex gap-1">
                                                    <button
                                                        onClick={() => setSelectedAlert(alert)}
                                                        className="p-1.5 text-gray-500 hover:text-primary-600 hover:bg-gray-100 dark:hover:bg-gray-700 rounded"
                                                        title="View Details"
                                                    >
                                                        <Eye className="w-4 h-4" />
                                                    </button>
                                                    {alert.status === 'open' && (
                                                        <button
                                                            onClick={() => handleStatusChange(alert.id, 'acknowledged')}
                                                            className="p-1.5 text-gray-500 hover:text-green-600 hover:bg-green-50 dark:hover:bg-green-900/20 rounded"
                                                            title="Acknowledge"
                                                        >
                                                            <Check className="w-4 h-4" />
                                                        </button>
                                                    )}
                                                </div>
                                            </td>
                                        </tr>
                                    );
                                })}
                            </tbody>
                        </table>
                    </div>
                )}

                {/* Pagination */}
                {!isLoading && filteredAlerts.length > 0 && (
                    <Pagination
                        page={page}
                        totalPages={totalPages}
                        pageSize={pageSize}
                        total={total}
                        onPageChange={setPage}
                        onPageSizeChange={(size) => {
                            setPageSize(size);
                            setPage(1);
                        }}
                    />
                )}
            </div>

            {/* Alert Detail Modal */}
            <AlertDetailModal
                alert={selectedAlert}
                isOpen={!!selectedAlert}
                onClose={() => setSelectedAlert(null)}
                onStatusChange={handleStatusChange}
            />
        </div>
    );
}
