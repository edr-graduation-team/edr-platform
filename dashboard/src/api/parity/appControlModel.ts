/** Types for `GET /api/v1/management/application-control/policies` (parity / future backend). */

export type AppControlPolicyMode = 'enforce' | 'audit';

export type AppControlPolicyState = 'draft' | 'published';

export type AppControlScopeType = 'fleet' | 'group' | 'tag';

export interface AppControlPolicy {
    id: string;
    name: string;
    description?: string;
    scope_type: AppControlScopeType;
    scope_label: string;
    mode: AppControlPolicyMode;
    state: AppControlPolicyState;
    priority: number;
    rule_count: number;
    coverage_percent: number;
    endpoints_synced: number;
    endpoints_lagged: number;
    last_published_at: string | null;
    updated_at: string;
    /** In audit mode: executions that would have been blocked (7d). */
    audit_only_blocks_7d: number;
    /** In enforce mode: actual blocks (7d). */
    enforce_blocks_7d: number;
}

export interface AppControlRolloutRow {
    hostname: string;
    agent_id: string;
    policy_sync: 'ok' | 'lagging' | 'unknown';
    last_policy_sync_at: string | null;
}

export interface AppControlAuditSummary {
    /** Sum of "would block" across audit policies (7d), demo aggregate. */
    would_block_events_7d: number;
    distinct_binaries_touched: number;
}

export interface AppControlPoliciesPayload {
    data: AppControlPolicy[];
    pagination: { total: number; limit: number; offset: number; has_more: boolean };
    meta?: { request_id: string; timestamp: string };
    /** Optional fleet rollout snapshot — may be omitted by real API. */
    rollout_preview?: AppControlRolloutRow[];
    audit_summary?: AppControlAuditSummary;
}
