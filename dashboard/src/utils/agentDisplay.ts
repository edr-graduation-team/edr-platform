import type { Agent } from '../api/client';

/** Matches server-side sweeper / Device Management list behavior (1 minute). */
export const STALE_THRESHOLD_MS = 60 * 1000;

export function formatRelativeTime(timestamp: string): string {
    const diff = Date.now() - new Date(timestamp).getTime();
    const minutes = Math.floor(diff / 60000);
    if (minutes < 1) return 'Just now';
    if (minutes < 60) return `${minutes}m ago`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h ago`;
    return `${Math.floor(hours / 24)}d ago`;
}

export function formatDate(dateStr?: string | null): string {
    if (!dateStr) return 'N/A';
    const d = new Date(dateStr);
    if (isNaN(d.getTime()) || d.getFullYear() <= 1) return 'N/A';
    return d.toLocaleDateString();
}

export function formatDateTime(dateStr?: string | null): string {
    if (!dateStr) return 'N/A';
    const d = new Date(dateStr);
    if (isNaN(d.getTime()) || d.getFullYear() <= 1) return 'N/A';
    return d.toLocaleString();
}

/** If status is online/degraded but last_seen is stale, treat as offline. */
export function getEffectiveStatus(agent: Agent): Agent['status'] {
    if (agent.status === 'online' || agent.status === 'degraded') {
        const elapsed = Date.now() - new Date(agent.last_seen).getTime();
        if (elapsed > STALE_THRESHOLD_MS) {
            return 'offline';
        }
    }
    return agent.status;
}

/**
 * Whether this agent has been decommissioned (server-confirmed uninstall or
 * still waiting for the agent's final UNINSTALL_CONFIRM). The Endpoints UI
 * hides action buttons and most command affordances for these rows.
 */
export function isDecommissioned(agent: Agent): boolean {
    return agent.status === 'uninstalled' || agent.status === 'pending_uninstall';
}
