import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { eventsApi, type EventSearchRequestBody } from '../../api/client';
import { classifyProcess } from './classifyProcess';
import type { ProcessAggRow } from './types';

// ────────────────────────────────────────────────────────────────────────────
// Shared data hook for Application Control.
//
// Fetches process-type events from the last N hours and aggregates them
// into ranked rows suitable for tables & charts.
// ────────────────────────────────────────────────────────────────────────────

/** How far back (hours) to look for process events. */
const LOOKBACK_HOURS = 24;

/** Max events fetched per search call. */
const EVENT_LIMIT = 2000;

function buildSearchBody(): EventSearchRequestBody {
    const now = new Date();
    const from = new Date(now.getTime() - LOOKBACK_HOURS * 60 * 60 * 1000);
    return {
        filters: [{ field: 'event_type', operator: 'eq', value: 'process' }],
        logic: 'AND',
        time_range: { from: from.toISOString(), to: now.toISOString() },
        limit: EVENT_LIMIT,
        offset: 0,
    };
}

/**
 * Extracts the executable name from `raw.executable` or `summary`.
 * Handles both full paths (C:\Windows\...) and bare names.
 */
function extractProcessName(raw: Record<string, unknown>, summary: string): { name: string; exe: string } {
    // Try raw.name first (ETW process events)
    const rawName = (raw?.name as string) ?? '';
    const rawExe = (raw?.executable as string) ?? '';

    if (rawName) {
        return { name: rawName.toLowerCase(), exe: rawExe || rawName };
    }

    // Fallback: parse from summary "Process START: pid=... name=X"
    const m = summary.match(/name=(\S+)/i);
    if (m) {
        return { name: m[1].toLowerCase(), exe: rawExe || m[1] };
    }

    // Last resort: use executable path basename
    if (rawExe) {
        const parts = rawExe.replace(/\\/g, '/').split('/');
        const base = parts[parts.length - 1];
        return { name: base.toLowerCase(), exe: rawExe };
    }

    return { name: 'unknown', exe: '' };
}

/** Aggregate raw events into ProcessAggRow[]. */
function aggregateProcessEvents(
    events: { id: string; agent_id: string; event_type: string; timestamp: string; summary: string; raw?: unknown }[],
): ProcessAggRow[] {
    const map = new Map<string, ProcessAggRow>();

    for (const evt of events) {
        const raw = (evt.raw ?? {}) as Record<string, unknown>;
        const { name, exe } = extractProcessName(raw, evt.summary);
        if (!name || name === 'unknown') continue;

        const existing = map.get(name);
        if (existing) {
            existing.count += 1;
            existing.agents.add(evt.agent_id);
            if (evt.timestamp > existing.lastSeen) {
                existing.lastSeen = evt.timestamp;
            }
        } else {
            map.set(name, {
                name,
                executable: exe,
                count: 1,
                category: classifyProcess(name),
                lastSeen: evt.timestamp,
                agents: new Set([evt.agent_id]),
            });
        }
    }

    // Sort by execution count descending
    return Array.from(map.values()).sort((a, b) => b.count - a.count);
}

// ─── Hook ────────────────────────────────────────────────────────────────────

export interface UseProcessAnalyticsResult {
    /** Aggregated rows, sorted by count desc. */
    rows: ProcessAggRow[];
    /** Total raw events fetched. */
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
        queryKey: ['app-control', 'process-events', LOOKBACK_HOURS],
        queryFn: async () => {
            const body = buildSearchBody();
            const result = await eventsApi.search(body);
            return result;
        },
        staleTime: 60_000,       // 1 min
        refetchInterval: 120_000, // 2 min
        retry: 1,
    });

    const rawEvents = query.data?.data ?? [];

    const rows = useMemo(() => aggregateProcessEvents(rawEvents as never[]), [rawEvents]);

    return {
        rows,
        totalEvents: rawEvents.length,
        isLoading: query.isLoading,
        isError: query.isError,
        refetch: () => query.refetch(),
        isFetching: query.isFetching,
    };
}
