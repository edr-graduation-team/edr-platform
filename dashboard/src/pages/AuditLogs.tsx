import { useQuery } from '@tanstack/react-query';
import { useState, useMemo, useEffect, useRef } from 'react';
import {
    Search,
    Download,
    Eye,
    User,
    Activity,
    Check,
    X,
    RefreshCw,
    Shield,
    AlertTriangle,
    Settings,
    ChevronDown,
    Trash2,
    Filter,
    FileSpreadsheet,
} from 'lucide-react';
import { auditApi, authApi, type AuditLog } from '../api/client';
import { Modal, DateRangePicker, type DateRange, SkeletonTable } from '../components';
import { useDebounce } from '../hooks/useDebounce';

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

function formatAction(action: string): string {
    return action
        .split('_')
        .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
        .join(' ');
}

function fmtCellValue(v: Record<string, unknown> | undefined | null): string {
    if (!v || typeof v !== 'object' || Object.keys(v).length === 0) return '—';
    try {
        const s = JSON.stringify(v);
        return s.length > 96 ? `${s.slice(0, 93)}…` : s;
    } catch {
        return '—';
    }
}

function affectedObjectLabel(log: AuditLog): string {
    const id = log.resource_id?.trim();
    if (id) return id;
    return log.resource_type || '—';
}

/** OpenEDR-style log creation timestamp */
function formatLogDate(iso: string): string {
    try {
        const d = new Date(iso);
        return d.toLocaleString(undefined, {
            year: 'numeric',
            month: '2-digit',
            day: '2-digit',
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit',
            hour12: true,
        });
    } catch {
        return iso;
    }
}

function AuditDetailModal({ log, isOpen, onClose }: { log: AuditLog | null; isOpen: boolean; onClose: () => void }) {
    if (!log) return null;

    const iconConfig = ACTION_ICONS[log.action] || { icon: Activity, color: 'text-gray-500' };
    const Icon = iconConfig.icon;

    return (
        <Modal isOpen={isOpen} onClose={onClose} title="Audit Log Details" size="lg">
            <div className="space-y-4">
                <div className="flex items-center gap-4 p-4 bg-gray-50 dark:bg-gray-900/50 rounded-lg">
                    <div
                        className={`p-3 rounded-lg ${log.result === 'success' ? 'bg-green-100 dark:bg-green-900/30' : 'bg-red-100 dark:bg-red-900/30'}`}
                    >
                        <Icon className={`w-6 h-6 ${iconConfig.color}`} />
                    </div>
                    <div className="flex-1">
                        <h3 className="text-lg font-semibold text-gray-900 dark:text-white">{formatAction(log.action)}</h3>
                        <p className="text-sm text-gray-500">{formatLogDate(log.timestamp)}</p>
                    </div>
                    <span
                        className={`px-2.5 py-1 text-xs font-semibold rounded-md border ${
                            log.result === 'success'
                                ? 'bg-green-500/10 text-green-500 border-green-500/20'
                                : 'bg-rose-500/10 text-rose-500 border-rose-500/20'
                        }`}
                    >
                        {log.result}
                    </span>
                </div>

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

                {log.user_agent && (
                    <div>
                        <label className="text-xs text-gray-500 uppercase tracking-wider">User Agent</label>
                        <p className="text-sm text-gray-600 dark:text-gray-400 font-mono break-all">{log.user_agent}</p>
                    </div>
                )}

                {log.error_message && (
                    <div className="p-3 bg-red-50 dark:bg-red-900/20 rounded-lg border border-red-200 dark:border-red-800">
                        <label className="text-xs text-red-600 uppercase tracking-wider">Error</label>
                        <p className="text-sm text-red-700 dark:text-red-300">{log.error_message}</p>
                    </div>
                )}

                {log.details && (
                    <div>
                        <label className="text-xs text-gray-500 uppercase tracking-wider">Extra info</label>
                        <p className="text-sm text-gray-700 dark:text-gray-300">{log.details}</p>
                    </div>
                )}

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

function exportToCSV(logs: AuditLog[]) {
    const headers = [
        'Log creation date',
        'Staff',
        'Event name',
        'Affected object',
        'Old value',
        'New value',
        'Extra info',
        'Session / trace ID',
        'Result',
        'IP',
    ];
    const rows = logs.map((log) => [
        new Date(log.timestamp).toISOString(),
        log.username,
        log.action,
        affectedObjectLabel(log),
        fmtCellValue(log.old_value),
        fmtCellValue(log.new_value),
        log.details || '',
        log.id,
        log.result,
        log.ip_address || '',
    ]);

    const csv = [headers.join(','), ...rows.map((row) => row.map((cell) => `"${String(cell).replace(/"/g, '""')}"`).join(','))].join('\n');
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `audit-logs-${new Date().toISOString().split('T')[0]}.csv`;
    a.click();
    URL.revokeObjectURL(url);
}

const PAGE_SIZES = [20, 50, 100, 200] as const;

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
        staff: '',
        affected: '',
        oldVal: '',
        newVal: '',
        extra: '',
        session: '',
    });
    const [filterPanelOpen, setFilterPanelOpen] = useState(true);
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState<(typeof PAGE_SIZES)[number]>(20);
    const [exportMenuOpen, setExportMenuOpen] = useState(false);
    const exportMenuRef = useRef<HTMLDivElement>(null);

    const debouncedSearch = useDebounce(filters.search, 300);

    useEffect(() => {
        const fn = (e: MouseEvent) => {
            if (exportMenuRef.current && !exportMenuRef.current.contains(e.target as Node)) setExportMenuOpen(false);
        };
        document.addEventListener('mousedown', fn);
        return () => document.removeEventListener('mousedown', fn);
    }, []);

    const canViewAudit = authApi.canViewAuditLogs();

    useEffect(() => {
        setPage(1);
    }, [filters.action, filters.resource_type, filters.user_id, debouncedSearch, dateRange.from, dateRange.to]);

    const offset = (page - 1) * pageSize;

    const { data, isLoading, error, isFetching, refetch } = useQuery({
        queryKey: [
            'auditLogs',
            filters.action,
            filters.resource_type,
            filters.user_id,
            debouncedSearch,
            dateRange,
            page,
            pageSize,
        ],
        queryFn: () =>
            auditApi.list({
                limit: pageSize,
                offset,
                action: filters.action || undefined,
                resource_type: filters.resource_type || undefined,
                user_id: filters.user_id || undefined,
                from: dateRange.from?.toISOString(),
                to: dateRange.to?.toISOString(),
            }),
        enabled: canViewAudit,
    });

    const logs = data?.data ?? [];
    const total = data?.pagination?.total ?? logs.length;

    const filteredLogs = useMemo(() => {
        let list = logs;
        if (debouncedSearch) {
            const s = debouncedSearch.toLowerCase();
            list = list.filter(
                (log) =>
                    log.username?.toLowerCase().includes(s) ||
                    log.action?.toLowerCase().includes(s) ||
                    log.resource_type?.toLowerCase().includes(s) ||
                    (log.resource_id || '').toLowerCase().includes(s) ||
                    (log.ip_address || '').toLowerCase().includes(s),
            );
        }
        if (filters.staff.trim()) {
            const s = filters.staff.toLowerCase();
            list = list.filter((log) => log.username?.toLowerCase().includes(s));
        }
        if (filters.affected.trim()) {
            const s = filters.affected.toLowerCase();
            list = list.filter(
                (log) =>
                    (log.resource_id || '').toLowerCase().includes(s) ||
                    log.resource_type.toLowerCase().includes(s),
            );
        }
        if (filters.oldVal.trim()) {
            const s = filters.oldVal.toLowerCase();
            list = list.filter((log) => fmtCellValue(log.old_value).toLowerCase().includes(s));
        }
        if (filters.newVal.trim()) {
            const s = filters.newVal.toLowerCase();
            list = list.filter((log) => fmtCellValue(log.new_value).toLowerCase().includes(s));
        }
        if (filters.extra.trim()) {
            const s = filters.extra.toLowerCase();
            list = list.filter((log) => (log.details || '').toLowerCase().includes(s));
        }
        if (filters.session.trim()) {
            const s = filters.session.toLowerCase();
            list = list.filter((log) => log.id.toLowerCase().includes(s));
        }
        return list;
    }, [logs, debouncedSearch, filters.staff, filters.affected, filters.oldVal, filters.newVal, filters.extra, filters.session]);

    const fromIdx = total === 0 ? 0 : offset + 1;
    const displayRange =
        total === 0 ? '0' : `${fromIdx}-${Math.min(offset + filteredLogs.length, offset + logs.length)}`;

    if (!canViewAudit) {
        return (
            <div className="card text-center py-12">
                <Shield className="w-12 h-12 text-gray-400 mx-auto mb-4" />
                <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">Access Denied</h3>
                <p className="text-gray-500">You don&apos;t have permission to view audit logs.</p>
                <p className="text-sm text-gray-400 mt-2">Required role: Admin or Security</p>
            </div>
        );
    }

    if (error) {
        return (
            <div className="card text-center py-12">
                <AlertTriangle className="w-12 h-12 text-red-400 mx-auto mb-4" />
                <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">Failed to Load Audit Logs</h3>
                <p className="text-gray-500">Please try again later.</p>
            </div>
        );
    }

    return (
        <div
            data-section-id="dashboard-audit-logs-root"
            className="flex flex-col min-h-0 w-full  gap-0"
        >
            {/* Title — compact, above OpenEDR-style toolbar */}
            <div className="mb-3 shrink-0">
                <h1 className="text-xl font-bold text-gray-900 dark:text-white">Audit Logs</h1>
                <p className="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
                    Platform activity (OpenEDR-style grid — columns align with Xcitium audit viewer)
                </p>
            </div>

            {/* dm-toolbar–like strip */}
            <div
                className="flex flex-wrap items-center gap-1 px-2 py-1.5 rounded-t-lg border border-b-0 shrink-0"
                style={{
                    background: 'var(--xc-nav-bg, #0a043d)',
                    borderColor: 'var(--xc-nav-border, rgba(255,255,255,0.08))',
                }}
            >
                <button
                    type="button"
                    disabled
                    title="Delete logs is not enabled for this deployment"
                    className="inline-flex items-center gap-2 px-2 py-1.5 rounded text-[13px] text-[var(--xc-nav-text,#c0ced6)] opacity-40 cursor-not-allowed border border-transparent"
                >
                    <Trash2 className="w-4 h-4 shrink-0" />
                    <span className="hidden sm:inline">Delete logs</span>
                </button>

                <div className="relative" ref={exportMenuRef}>
                    <button
                        type="button"
                        onClick={() => setExportMenuOpen((o) => !o)}
                        disabled={filteredLogs.length === 0}
                        className="inline-flex items-center gap-2 px-2 py-1.5 rounded text-[13px] text-[var(--xc-nav-text,#c0ced6)] hover:bg-[var(--xc-nav-hover,rgb(8,3,49))] disabled:opacity-40 border border-transparent"
                    >
                        <Download className="w-4 h-4 shrink-0" />
                        <span>Export</span>
                        <ChevronDown className={`w-3.5 h-3.5 transition-transform ${exportMenuOpen ? 'rotate-180' : ''}`} />
                    </button>
                    {exportMenuOpen && (
                        <div
                            className="absolute left-0 top-full mt-1 min-w-[200px] rounded-md border py-1 shadow-xl z-50"
                            style={{
                                background: 'var(--xc-nav-bg,#0a043d)',
                                borderColor: 'var(--xc-nav-border,rgba(255,255,255,0.08))',
                            }}
                        >
                            <button
                                type="button"
                                className="w-full text-left px-3 py-2 text-[13px] text-[var(--xc-nav-text,#c0ced6)] hover:bg-[var(--xc-nav-hover,rgb(8,3,49))] flex items-center gap-2"
                                onClick={() => {
                                    exportToCSV(filteredLogs);
                                    setExportMenuOpen(false);
                                }}
                            >
                                <FileSpreadsheet className="w-4 h-4" />
                                Export to CSV
                            </button>
                        </div>
                    )}
                </div>

                <div className="ml-auto flex items-center gap-1">
                    <button
                        type="button"
                        onClick={() => refetch()}
                        disabled={isFetching}
                        title="Refresh"
                        className="inline-flex items-center justify-center p-2 rounded text-[var(--xc-nav-text,#c0ced6)] hover:bg-[var(--xc-nav-hover,rgb(8,3,49))] disabled:opacity-50"
                    >
                        <RefreshCw className={`w-4 h-4 ${isFetching ? 'animate-spin' : ''}`} />
                    </button>
                    <button
                        type="button"
                        onClick={() => setFilterPanelOpen((v) => !v)}
                        title="Table filter"
                        className={`inline-flex items-center justify-center p-2 rounded ${
                            filterPanelOpen
                                ? 'text-[var(--xc-nav-active,#f19637)] bg-[var(--xc-nav-hover,rgb(8,3,49))]'
                                : 'text-[var(--xc-nav-text,#c0ced6)] hover:bg-[var(--xc-nav-hover,rgb(8,3,49))]'
                        }`}
                    >
                        <Filter className="w-4 h-4" />
                    </button>
                </div>
            </div>

            {/* Filter panel — OpenEDR “cover” style */}
            {filterPanelOpen && (
                <div className="border border-gray-200 dark:border-gray-700 border-t-0 rounded-b-lg bg-white dark:bg-slate-900/90 p-4 space-y-4 shadow-sm">
                    <div className="flex flex-wrap gap-4 items-end">
                        <DateRangePicker value={dateRange} onChange={setDateRange} label="Log creation date" />
                        <div className="min-w-[140px]">
                            <label className="block text-[11px] font-semibold text-gray-500 uppercase tracking-wide mb-1">
                                Staff
                            </label>
                            <input
                                type="text"
                                placeholder="Staff"
                                value={filters.staff}
                                onChange={(e) => setFilters((f) => ({ ...f, staff: e.target.value }))}
                                className="w-full rounded-md border border-gray-200 dark:border-gray-600 bg-white dark:bg-slate-800 px-2 py-1.5 text-sm text-gray-900 dark:text-gray-100"
                            />
                        </div>
                        <div>
                            <label className="block text-[11px] font-semibold text-gray-500 uppercase tracking-wide mb-1">
                                Event name
                            </label>
                            <select
                                value={filters.action}
                                onChange={(e) => setFilters((f) => ({ ...f, action: e.target.value }))}
                                className="rounded-md border border-gray-200 dark:border-gray-600 bg-white dark:bg-slate-800 px-2 py-1.5 text-sm min-w-[180px]"
                            >
                                <option value="">All</option>
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
                        </div>
                        <div>
                            <label className="block text-[11px] font-semibold text-gray-500 uppercase tracking-wide mb-1">
                                Resource
                            </label>
                            <select
                                value={filters.resource_type}
                                onChange={(e) => setFilters((f) => ({ ...f, resource_type: e.target.value }))}
                                className="rounded-md border border-gray-200 dark:border-gray-600 bg-white dark:bg-slate-800 px-2 py-1.5 text-sm min-w-[140px]"
                            >
                                <option value="">All types</option>
                                <option value="user">User</option>
                                <option value="agent">Agent</option>
                                <option value="alert">Alert</option>
                                <option value="rule">Rule</option>
                                <option value="policy">Policy</option>
                                <option value="command">Command</option>
                                <option value="system">System</option>
                                <option value="settings">Settings</option>
                                <option value="token">Token</option>
                            </select>
                        </div>
                        <div className="flex-1 min-w-[200px]">
                            <label className="block text-[11px] font-semibold text-gray-500 uppercase tracking-wide mb-1">
                                Search
                            </label>
                            <div className="relative">
                                <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
                                <input
                                    type="text"
                                    placeholder="User, action, resource…"
                                    value={filters.search}
                                    onChange={(e) => setFilters((f) => ({ ...f, search: e.target.value }))}
                                    className="w-full rounded-md border border-gray-200 dark:border-gray-600 bg-white dark:bg-slate-800 pl-8 pr-2 py-1.5 text-sm"
                                />
                            </div>
                        </div>
                    </div>
                    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3 pt-2 border-t border-gray-100 dark:border-gray-800">
                        {(
                            [
                                ['affected', 'Affected object', filters.affected],
                                ['oldVal', 'Old value', filters.oldVal],
                                ['newVal', 'New value', filters.newVal],
                                ['extra', 'Extra info', filters.extra],
                                ['session', 'Session / trace ID', filters.session],
                            ] as const
                        ).map(([key, label, val]) => (
                            <div key={key}>
                                <label className="block text-[11px] font-semibold text-gray-500 uppercase tracking-wide mb-1">
                                    {label}
                                </label>
                                <input
                                    type="text"
                                    placeholder={label}
                                    value={val}
                                    onChange={(e) => setFilters((f) => ({ ...f, [key]: e.target.value }))}
                                    className="w-full rounded-md border border-gray-200 dark:border-gray-600 bg-white dark:bg-slate-800 px-2 py-1.5 text-sm"
                                />
                            </div>
                        ))}
                    </div>
                    <p className="text-[11px] text-gray-400">
                        Column filters (Staff, Affected object, values…) apply to the current result page. Use date range
                        and server filters for broad queries.
                    </p>
                </div>
            )}

            {/* Table card */}
            <div className="border border-gray-200 dark:border-gray-700 rounded-b-lg overflow-hidden bg-white dark:bg-slate-900/50 flex flex-col min-h-[320px]">
                {isLoading ? (
                    <div className="p-4">
                        <SkeletonTable rows={8} columns={9} />
                    </div>
                ) : filteredLogs.length === 0 ? (
                    <div className="text-center py-16 text-gray-500">
                        <p className="italic text-sm" style={{ color: '#6E6E6E' }}>
                            No results found.
                        </p>
                    </div>
                ) : (
                    <div className="overflow-x-auto">
                        <table className="w-full text-left text-sm border-collapse items table-striped">
                            <thead>
                                <tr className="border-b border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-slate-800/80">
                                    <th className="py-2.5 px-3 text-[11px] font-semibold uppercase tracking-wide text-gray-600 dark:text-gray-400 whitespace-nowrap min-w-[150px]">
                                        Staff
                                    </th>
                                    <th className="py-2.5 px-3 text-[11px] font-semibold uppercase tracking-wide text-gray-600 dark:text-gray-400 whitespace-nowrap">
                                        Event name
                                    </th>
                                    <th className="py-2.5 px-3 text-[11px] font-semibold uppercase tracking-wide text-gray-600 dark:text-gray-400 whitespace-nowrap min-w-[120px]">
                                        Affected object
                                    </th>
                                    <th className="py-2.5 px-3 text-[11px] font-semibold uppercase tracking-wide text-gray-600 dark:text-gray-400 whitespace-nowrap max-w-[140px]">
                                        Old value
                                    </th>
                                    <th className="py-2.5 px-3 text-[11px] font-semibold uppercase tracking-wide text-gray-600 dark:text-gray-400 whitespace-nowrap max-w-[140px]">
                                        New value
                                    </th>
                                    <th className="py-2.5 px-3 text-[11px] font-semibold uppercase tracking-wide text-gray-600 dark:text-gray-400 whitespace-nowrap max-w-[160px]">
                                        Extra info
                                    </th>
                                    <th className="py-2.5 px-3 text-[11px] font-semibold uppercase tracking-wide text-gray-600 dark:text-gray-400 whitespace-nowrap font-mono text-xs">
                                        Session ID
                                    </th>
                                    <th className="py-2.5 px-3 text-[11px] font-semibold uppercase tracking-wide text-gray-600 dark:text-gray-400 whitespace-nowrap w-[180px]">
                                        Log creation date
                                    </th>
                                    <th className="py-2.5 px-3 text-[11px] font-semibold uppercase tracking-wide text-gray-600 dark:text-gray-400 text-right w-12">
                                        {' '}
                                    </th>
                                </tr>
                            </thead>
                            <tbody>
                                {filteredLogs.map((log, i) => {
                                    const iconConfig = ACTION_ICONS[log.action] || { icon: Activity, color: 'text-gray-500' };
                                    const Icon = iconConfig.icon;
                                    const resourceBadge = RESOURCE_BADGES[log.resource_type] || {
                                        label: log.resource_type,
                                        color: 'bg-slate-500/10 text-slate-500 border border-slate-500/20',
                                    };
                                    return (
                                        <tr
                                            key={log.id}
                                            className={`border-b border-gray-100 dark:border-gray-800/80 ${
                                                i % 2 === 1 ? 'bg-gray-50/80 dark:bg-slate-800/40' : ''
                                            } hover:bg-cyan-500/5 dark:hover:bg-slate-800/60`}
                                        >
                                            <td className="py-2 px-3 align-top">
                                                <div className="flex items-start gap-2 min-w-0">
                                                    <div className="w-7 h-7 rounded-full bg-slate-200 dark:bg-slate-700 flex items-center justify-center shrink-0 mt-0.5">
                                                        <User className="w-3.5 h-3.5 text-slate-500" />
                                                    </div>
                                                    <div className="min-w-0">
                                                        <span className="text-cyan-600 dark:text-cyan-400 font-medium truncate block">
                                                            {log.username}
                                                        </span>
                                                    </div>
                                                </div>
                                            </td>
                                            <td className="py-2 px-3 align-top">
                                                <div className="flex items-center gap-2">
                                                    <Icon className={`w-4 h-4 shrink-0 ${iconConfig.color}`} />
                                                    <span className="text-gray-800 dark:text-gray-200">
                                                        {formatAction(log.action)}
                                                    </span>
                                                </div>
                                            </td>
                                            <td className="py-2 px-3 align-top">
                                                <span
                                                    className={`inline-block px-1.5 py-0.5 rounded text-[10px] font-semibold uppercase ${resourceBadge.color}`}
                                                >
                                                    {resourceBadge.label}
                                                </span>
                                                <p
                                                    className="text-xs text-gray-600 dark:text-gray-400 font-mono mt-1 truncate max-w-[220px]"
                                                    title={log.resource_id || ''}
                                                >
                                                    {affectedObjectLabel(log)}
                                                </p>
                                            </td>
                                            <td className="py-2 px-3 align-top log-text-col text-xs font-mono text-gray-600 dark:text-gray-400 max-w-[200px] break-all">
                                                {fmtCellValue(log.old_value)}
                                            </td>
                                            <td className="py-2 px-3 align-top log-text-col text-xs font-mono text-gray-600 dark:text-gray-400 max-w-[200px] break-all">
                                                {fmtCellValue(log.new_value)}
                                            </td>
                                            <td className="py-2 px-3 align-top text-xs text-gray-600 dark:text-gray-400 max-w-[200px] break-words">
                                                {log.details || '—'}
                                            </td>
                                            <td className="py-2 px-3 align-top font-mono text-[11px] text-gray-500 dark:text-gray-400 max-w-[140px] truncate" title={log.id}>
                                                {log.id}
                                            </td>
                                            <td className="py-2 px-3 align-top whitespace-nowrap text-gray-700 dark:text-gray-300 text-xs">
                                                {formatLogDate(log.timestamp)}
                                            </td>
                                            <td className="py-2 px-3 align-top text-right">
                                                <button
                                                    type="button"
                                                    onClick={() => setSelectedLog(log)}
                                                    className="p-1.5 text-gray-400 hover:text-cyan-500 rounded"
                                                    title="Details"
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

                {/* Pagination — OpenEDR style */}
                {!isLoading && logs.length > 0 && (
                    <div className="flex flex-wrap items-center justify-between gap-3 px-3 py-2 border-t border-gray-200 dark:border-gray-700 bg-gray-50/80 dark:bg-slate-900/60 text-[12px] text-gray-500">
                        <div className="flex items-center gap-2">
                            <span className="text-gray-500">Results per page:</span>
                            <select
                                value={pageSize}
                                onChange={(e) => {
                                    setPageSize(Number(e.target.value) as (typeof PAGE_SIZES)[number]);
                                    setPage(1);
                                }}
                                className="rounded border border-gray-200 dark:border-gray-600 bg-white dark:bg-slate-800 px-2 py-1 text-xs"
                            >
                                {PAGE_SIZES.map((n) => (
                                    <option key={n} value={n}>
                                        {n}
                                    </option>
                                ))}
                            </select>
                        </div>
                        <span className="text-gray-500">
                            Displaying {displayRange} of {total} results
                        </span>
                        <div className="flex items-center gap-2">
                            <button
                                type="button"
                                disabled={page <= 1}
                                onClick={() => setPage((p) => Math.max(1, p - 1))}
                                className="px-2 py-1 rounded border border-gray-200 dark:border-gray-600 disabled:opacity-40 text-xs"
                            >
                                Previous
                            </button>
                            <span className="text-xs tabular-nums">
                                Page {page} / {Math.max(1, Math.ceil(total / pageSize))}
                            </span>
                            <button
                                type="button"
                                disabled={offset + logs.length >= total}
                                onClick={() => setPage((p) => p + 1)}
                                className="px-2 py-1 rounded border border-gray-200 dark:border-gray-600 disabled:opacity-40 text-xs"
                            >
                                Next
                            </button>
                        </div>
                    </div>
                )}
            </div>

            <AuditDetailModal log={selectedLog} isOpen={!!selectedLog} onClose={() => setSelectedLog(null)} />
        </div>
    );
}
