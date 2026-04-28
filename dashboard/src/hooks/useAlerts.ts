import { useEffect, useRef, useState, useCallback } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { alertsApi, agentsApi, createAlertStream, type Alert } from '../api/client';
import { useToast } from '../components';
import { useDebounce } from './useDebounce';
import type { DateRange } from '../components/DateRangePicker';

export type SortField = 'timestamp' | 'severity' | 'risk_score';

export interface AlertFilters {
    severities: string[];
    statuses: string[];
    search: string;
}

export interface UseAlertsReturn {
    // Data
    alerts: Alert[];
    total: number;
    totalPages: number;
    agentHostnameMap: Record<string, string>;

    // Loading states
    isLoading: boolean;
    isError: boolean;
    error: Error | null;

    // Pagination
    page: number;
    pageSize: number;
    setPage: (page: number) => void;
    setPageSize: (size: number) => void;

    // Sorting
    sortBy: SortField;
    sortOrder: 'asc' | 'desc';
    toggleSort: (field: SortField) => void;

    // Filters
    filters: AlertFilters;
    dateRange: DateRange;
    setFilters: (filters: AlertFilters) => void;
    setDateRange: (range: DateRange) => void;

    // Selection
    selectedIds: Set<string>;
    selectedAlert: Alert | null;
    toggleSelectAll: () => void;
    toggleSelect: (id: string) => void;
    setSelectedAlert: (alert: Alert | null) => void;
    clearSelection: () => void;

    // Actions
    handleStatusChange: (id: string, status: string) => void;
    handleBulkAction: (status: string) => void;
    isUpdating: boolean;
    isBulkUpdating: boolean;
}

export function useAlerts(): UseAlertsReturn {
    const queryClient = useQueryClient();
    const { showToast } = useToast();

    // Refs for stream handling
    const seenAlertIdsRef = useRef<Set<string>>(new Set());
    const pendingStreamIdsRef = useRef<Set<string>>(new Set());
    const streamSyncTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

    // State
    const [selectedAlert, setSelectedAlert] = useState<Alert | null>(null);
    const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(50);
    const [sortBy, setSortBy] = useState<SortField>('risk_score');
    const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');

    const [filters, setFilters] = useState<AlertFilters>({
        severities: [],
        statuses: [],
        search: '',
    });

    const [dateRange, setDateRange] = useState<DateRange>({
        from: new Date(Date.now() - 24 * 60 * 60 * 1000),
        to: new Date(),
    });

    const debouncedSearch = useDebounce(filters.search, 300);

    // Close drawer on Escape
    useEffect(() => {
        const handler = (e: KeyboardEvent) => {
            if (e.key === 'Escape') setSelectedAlert(null);
        };
        window.addEventListener('keydown', handler);
        return () => window.removeEventListener('keydown', handler);
    }, []);

    // Agent hostname lookup map
    const { data: agentListData } = useQuery({
        queryKey: ['agentsForAlerts'],
        queryFn: () => agentsApi.list({ limit: 500 }),
        staleTime: 120000,
        refetchInterval: 120000,
    });

    const agentHostnameMap = agentListData?.data?.reduce((acc: Record<string, string>, agent) => {
        acc[agent.id] = agent.hostname;
        return acc;
    }, {}) || {};

    // Fetch alerts
    const { data, isLoading, isError, error } = useQuery({
        queryKey: ['alerts', filters.severities, filters.statuses, debouncedSearch, dateRange.from?.toISOString(), page, pageSize, sortBy, sortOrder],
        queryFn: () => alertsApi.list({
            limit: pageSize,
            offset: (page - 1) * pageSize,
            severity: filters.severities.length > 0 ? filters.severities.join(',') : undefined,
            status: filters.statuses.length > 0 ? filters.statuses.join(',') : undefined,
            date_from: dateRange.from?.toISOString(),
            date_to: new Date().toISOString(),
            search: debouncedSearch || undefined,
            sort: sortBy,
            order: sortOrder,
        }),
        refetchInterval: 1000,
    });

    const alerts = data?.alerts || [];
    const total = data?.total || 0;
    const totalPages = Math.ceil(total / pageSize);

    // Track IDs already rendered from DB
    useEffect(() => {
        for (const alert of alerts) {
            seenAlertIdsRef.current.add(alert.id);
        }
    }, [alerts]);

    // Realtime stream setup
    useEffect(() => {
        const triggerDebouncedSync = () => {
            if (streamSyncTimerRef.current) {
                clearTimeout(streamSyncTimerRef.current);
            }
            streamSyncTimerRef.current = setTimeout(() => {
                const newCount = pendingStreamIdsRef.current.size;
                pendingStreamIdsRef.current.clear();

                queryClient.invalidateQueries({ queryKey: ['alerts'] });
                queryClient.invalidateQueries({ queryKey: ['alertStats'] });

                if (newCount > 0) {
                    showToast(`Received ${newCount} new alert${newCount > 1 ? 's' : ''}`, 'success');
                }
            }, 150);
        };

        const stream = createAlertStream((alert) => {
            if (!alert?.id || seenAlertIdsRef.current.has(alert.id)) {
                return;
            }
            seenAlertIdsRef.current.add(alert.id);
            pendingStreamIdsRef.current.add(alert.id);
            triggerDebouncedSync();
        });

        return () => {
            stream.close();
            if (streamSyncTimerRef.current) {
                clearTimeout(streamSyncTimerRef.current);
                streamSyncTimerRef.current = null;
            }
            pendingStreamIdsRef.current.clear();
        };
    }, [queryClient, showToast]);

    // Mutations
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

    // Handlers
    const handleStatusChange = useCallback((id: string, status: string) => {
        updateStatusMutation.mutate({ id, status });
        setSelectedAlert(null);
    }, [updateStatusMutation]);

    const handleBulkAction = useCallback((status: string) => {
        bulkUpdateMutation.mutate({ ids: Array.from(selectedIds), status });
    }, [bulkUpdateMutation, selectedIds]);

    const toggleSelectAll = useCallback(() => {
        if (selectedIds.size === alerts.length) {
            setSelectedIds(new Set());
        } else {
            setSelectedIds(new Set(alerts.map((a) => a.id)));
        }
    }, [selectedIds, alerts]);

    const toggleSelect = useCallback((id: string) => {
        const newSet = new Set(selectedIds);
        if (newSet.has(id)) {
            newSet.delete(id);
        } else {
            newSet.add(id);
        }
        setSelectedIds(newSet);
    }, [selectedIds]);

    const toggleSort = useCallback((field: SortField) => {
        if (sortBy === field) {
            setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
        } else {
            setSortBy(field);
            setSortOrder('desc');
        }
        setPage(1);
    }, [sortBy, sortOrder]);

    const clearSelection = useCallback(() => {
        setSelectedIds(new Set());
    }, []);

    // Wrapper for setPageSize that resets page
    const handleSetPageSize = useCallback((size: number) => {
        setPageSize(size);
        setPage(1);
    }, []);

    return {
        // Data
        alerts,
        total,
        totalPages,
        agentHostnameMap,

        // Loading states
        isLoading,
        isError,
        error,

        // Pagination
        page,
        pageSize,
        setPage,
        setPageSize: handleSetPageSize,

        // Sorting
        sortBy,
        sortOrder,
        toggleSort,

        // Filters
        filters,
        dateRange,
        setFilters,
        setDateRange,

        // Selection
        selectedIds,
        selectedAlert,
        toggleSelectAll,
        toggleSelect,
        setSelectedAlert,
        clearSelection,

        // Actions
        handleStatusChange,
        handleBulkAction,
        isUpdating: updateStatusMutation.isPending,
        isBulkUpdating: bulkUpdateMutation.isPending,
    };
}

export default useAlerts;
