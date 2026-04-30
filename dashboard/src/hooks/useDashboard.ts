import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useMemo, useRef, useState, useCallback } from 'react';
import {
    statsApi,
    alertsApi,
    agentsApi,
    createAlertStream,
    type Alert,
    type Agent,
    type AgentStats,
    type AlertStats,
    type TimelineDataPoint,
} from '../api/client';

const STALE_THRESHOLD_MS = 1 * 60 * 1000;

export interface DashboardData {
    // Alert stats
    alertStats: AlertStats | undefined;
    statsLoading: boolean;

    // Agent stats
    agentStats: AgentStats | undefined;

    // Agent list
    agents: Agent[];

    // Recent alerts (from polling)
    recentAlerts: Alert[];

    // Live alerts (from WebSocket + polling)
    liveAlerts: Alert[];

    // Timeline data for sparklines
    timelineData: TimelineDataPoint[] | undefined;

    // Computed
    agentMap: Record<string, string>;
    threatScore: number;
    sparklines: {
        critical: number[];
        high: number[];
        total: number[];
    };

    // Actions
    handleAlertClick: (alert: Alert) => void;
    handleCloseDrawer: () => void;
    drawerAlert: Alert | null;
}

function calcThreatScore(stats: AlertStats | undefined): number {
    if (!stats) return 0;
    const s = stats.by_severity || {};
    const raw = (s.critical || 0) * 20 + (s.high || 0) * 8 + (s.medium || 0) * 3 + (s.low || 0);
    return Math.min(100, Math.round((raw / Math.max(raw, 200)) * 100));
}

export function useDashboard(): DashboardData {
    const queryClient = useQueryClient();
    const [liveAlerts, setLiveAlerts] = useState<Alert[]>([]);
    const [drawerAlert, setDrawerAlert] = useState<Alert | null>(null);
    const statsInvalidateTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

    // ── Queries ───────────────────────────────────────────────
    const { data: alertStats, isLoading: statsLoading } = useQuery({
        queryKey: ['alertStats'],
        queryFn: statsApi.alerts,
        refetchInterval: 1000,
    });

    const { data: agentStats } = useQuery({
        queryKey: ['agentStats'],
        queryFn: agentsApi.stats,
        retry: false,
        refetchInterval: 5000,
    });

    const { data: agentListData } = useQuery({
        queryKey: ['agents'],
        queryFn: () => agentsApi.list({ limit: 200 }),
        retry: false,
        refetchInterval: 10000,
    });

    const { data: recentAlertsData } = useQuery({
        queryKey: ['recentAlerts'],
        queryFn: () => alertsApi.list({ limit: 100 }),
        refetchInterval: 1000,
    });

    const { data: timelineData } = useQuery({
        queryKey: ['dashboardTimeline'],
        queryFn: () => {
            const to = new Date().toISOString();
            const from = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString();
            return statsApi.timeline({ from, to, granularity: '1d' });
        },
        refetchInterval: 60000,
    });

    // ── Computed values ───────────────────────────────────────
    const agents = useMemo(() => agentListData?.data || [], [agentListData]);

    const agentMap = useMemo<Record<string, string>>(() => {
        const map: Record<string, string> = {};
        agents.forEach((a) => {
            map[a.id] = a.hostname;
        });
        return map;
    }, [agents]);

    const threatScore = useMemo(() => calcThreatScore(alertStats), [alertStats]);

    const sparklines = useMemo(() => {
        const pts = timelineData?.data || [];
        const critical = pts.map((p) => p.critical);
        const high = pts.map((p) => p.high);
        const total = pts.map((p) => p.critical + p.high + p.medium + p.low + p.informational);
        return { critical, high, total };
    }, [timelineData]);

    const recentAlerts = useMemo(() => recentAlertsData?.alerts || [], [recentAlertsData]);

    // ── WebSocket stream ──────────────────────────────────────
    useEffect(() => {
        if (recentAlerts.length > 0 && liveAlerts.length === 0) {
            setLiveAlerts(recentAlerts);
        }
    }, [recentAlerts, liveAlerts.length]);

    useEffect(() => {
        const stream = createAlertStream(
            (alert) => {
                setLiveAlerts((prev) => [alert, ...prev.slice(0, 99)]);

                // Debounce: batch rapid WebSocket arrivals into a single
                // stats refetch (150ms window) to avoid API hammering.
                if (statsInvalidateTimer.current) {
                    clearTimeout(statsInvalidateTimer.current);
                }
                statsInvalidateTimer.current = setTimeout(() => {
                    queryClient.invalidateQueries({ queryKey: ['alertStats'] });
                    queryClient.invalidateQueries({ queryKey: ['recentAlerts'] });
                }, 150);
            },
            { severity: ['critical', 'high', 'medium', 'low'] }
        );

        return () => {
            stream.close();
            if (statsInvalidateTimer.current) {
                clearTimeout(statsInvalidateTimer.current);
            }
        };
    }, [queryClient]);

    // ── Actions ───────────────────────────────────────────────
    const handleAlertClick = useCallback((alert: Alert) => {
        setDrawerAlert(alert);
    }, []);

    const handleCloseDrawer = useCallback(() => {
        setDrawerAlert(null);
    }, []);

    // ── Document title ─────────────────────────────────────────
    useEffect(() => {
        document.title = 'Security Posture — EDR Platform';
    }, []);

    return {
        alertStats,
        statsLoading,
        agentStats,
        agents,
        recentAlerts,
        liveAlerts,
        timelineData: timelineData?.data,
        agentMap,
        threatScore,
        sparklines,
        handleAlertClick,
        handleCloseDrawer,
        drawerAlert,
    };
}

// Export helper for components
export { STALE_THRESHOLD_MS, calcThreatScore };
