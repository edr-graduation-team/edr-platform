import { useState, useEffect, useCallback, useRef } from 'react';

/**
 * Hook that provides auto-refresh capability with a visual "last updated" indicator.
 * 
 * @param refetchFn - The function to call on each refresh tick (e.g. queryClient.invalidateQueries)
 * @param intervalMs - Refresh interval in milliseconds (default: 30_000 = 30s)
 * @param enabled - Whether auto-refresh is active (default: true)
 * 
 * @returns { lastUpdated, secondsAgo, refresh, paused, setPaused }
 */
export function useAutoRefresh(
    refetchFn: () => void | Promise<void>,
    intervalMs = 30_000,
    enabled = true,
) {
    const [lastUpdated, setLastUpdated] = useState<Date>(new Date());
    const [secondsAgo, setSecondsAgo] = useState(0);
    const [paused, setPaused] = useState(false);
    const refetchRef = useRef(refetchFn);
    refetchRef.current = refetchFn;

    const refresh = useCallback(async () => {
        try {
            await refetchRef.current();
        } catch {
            // silently ignore – the query's own error state handles UI
        }
        setLastUpdated(new Date());
        setSecondsAgo(0);
    }, []);

    // Auto-refresh interval
    useEffect(() => {
        if (!enabled || paused) return;
        const id = setInterval(() => {
            refresh();
        }, intervalMs);
        return () => clearInterval(id);
    }, [enabled, paused, intervalMs, refresh]);

    // Seconds-ago ticker (updates every second for display)
    useEffect(() => {
        const id = setInterval(() => {
            setSecondsAgo(Math.floor((Date.now() - lastUpdated.getTime()) / 1000));
        }, 1000);
        return () => clearInterval(id);
    }, [lastUpdated]);

    return {
        /** When the last successful refresh happened */
        lastUpdated,
        /** How many seconds since last refresh */
        secondsAgo,
        /** Trigger a manual refresh */
        refresh,
        /** Whether auto-refresh is paused */
        paused,
        /** Pause/resume auto-refresh */
        setPaused,
        /** Human-readable "X seconds ago" / "X minutes ago" string */
        agoText: secondsAgo < 5 ? 'just now' : secondsAgo < 60 ? `${secondsAgo}s ago` : `${Math.floor(secondsAgo / 60)}m ago`,
    };
}
