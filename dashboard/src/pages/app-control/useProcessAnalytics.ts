import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { appControlApi, type ProcessAnalyticsRow } from '../../api/client';
import { classifyProcess } from './classifyProcess';
import type { ProcessAggRow } from './types';

// ────────────────────────────────────────────────────────────────────────────
// Shared data hook for Application Control — Process Analytics tab.
//
// Uses the server-side aggregation endpoint (GET /api/v1/app-control/process-analytics)
// which runs a SQL GROUP BY on the events table. This is far more efficient
// than fetching thousands of raw events and aggregating client-side.
// ────────────────────────────────────────────────────────────────────────────

/** How far back (hours) to look for process events. */
const LOOKBACK_HOURS = 24;

/** Convert a backend ProcessAnalyticsRow to the UI ProcessAggRow shape. */
function toAggRow(r: ProcessAnalyticsRow): ProcessAggRow {
    const name = r.name.toLowerCase();
    return {
        name,
        executable: r.executable,
        count: r.count,
        category: classifyProcess(name),
        lastSeen: r.last_seen,
        hostnames: (r.hostnames ?? []).filter(Boolean),
    };
}

// ─── Hook ────────────────────────────────────────────────────────────────────

export interface UseProcessAnalyticsResult {
    /** Aggregated rows, sorted by count desc (from server). */
    rows: ProcessAggRow[];
    /** Total raw events in the time window. */
    totalEvents: number;
    /** Loading state. */
    isLoading: boolean;
    /** Error state. */
    isError: boolean;
    /** Refetch callback. */
    refetch: () => void;
    /** Whether a background refetch is in progress. */
    isFetching: boolean;
}

export function useProcessAnalytics(): UseProcessAnalyticsResult {
    const query = useQuery({
        queryKey: ['app-control', 'process-analytics', LOOKBACK_HOURS],
        queryFn: () => appControlApi.getProcessAnalytics(LOOKBACK_HOURS),
        staleTime: 60_000,       // 1 min
        refetchInterval: 120_000, // 2 min
        retry: 1,
    });

    const serverRows = query.data?.data ?? [];
    const totalEvents = query.data?.total_events ?? 0;

    const rows = useMemo(
        () => serverRows.map(toAggRow),
        [serverRows],
    );

    return {
        rows,
        totalEvents,
        isLoading: query.isLoading,
        isError: query.isError,
        refetch: () => query.refetch(),
        isFetching: query.isFetching,
    };
}
