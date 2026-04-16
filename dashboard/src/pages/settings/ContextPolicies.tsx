import { useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Plus, Save, Trash2 } from 'lucide-react';
import { agentsApi, contextPoliciesApi, usersApi, type ContextPolicy } from '../../api/client';
import { useToast } from '../../components/Toast';

type EditablePolicy = Omit<ContextPolicy, 'id' | 'created_at' | 'updated_at'> & { id?: number };

const NEW_POLICY: EditablePolicy = {
    name: '',
    scope_type: 'agent',
    scope_value: '',
    enabled: true,
    user_role_weight: 1.0,
    device_criticality_weight: 1.0,
    network_anomaly_factor: 1.0,
    trusted_networks: [],
    notes: '',
};

const CIDR_PATTERN = /^(?:\d{1,3}\.){3}\d{1,3}\/(?:[0-9]|[1-2][0-9]|3[0-2])$/;

function parseTrustedNetworks(value: string): string[] {
    return value.split(',').map(v => v.trim()).filter(Boolean);
}

export default function ContextPolicies() {
    const queryClient = useQueryClient();
    const { showToast } = useToast();
    const [draft, setDraft] = useState<EditablePolicy>(NEW_POLICY);
    const [trustedNetworksText, setTrustedNetworksText] = useState('');
    const [validationError, setValidationError] = useState<string>('');

    const { data, isLoading, error } = useQuery({
        queryKey: ['contextPolicies'],
        queryFn: contextPoliciesApi.list,
        refetchInterval: 15000,
    });
    const { data: agentsData, isLoading: isAgentsLoading, refetch: refetchAgents } = useQuery({
        queryKey: ['contextPolicyAgents'],
        queryFn: () => agentsApi.list({ limit: 500, offset: 0, sort_by: 'hostname', sort_order: 'asc' }),
        staleTime: 30000,
    });
    const { data: usersData, isLoading: isUsersLoading, refetch: refetchUsers } = useQuery({
        queryKey: ['contextPolicyUsers'],
        queryFn: () => usersApi.list({ limit: 500, offset: 0 }),
        staleTime: 30000,
    });

    const items = data?.data ?? [];
    const agentOptions = useMemo(
        () => (agentsData?.data ?? []).map(a => ({ value: a.id, label: `${a.hostname} (${a.id})` })),
        [agentsData]
    );
    const userOptions = useMemo(
        () => (usersData?.data ?? []).map(u => ({ value: u.username, label: `${u.username}${u.full_name ? ` (${u.full_name})` : ''}` })),
        [usersData]
    );

    const matchingScope = useMemo(
        () => items.find(i => i.scope_type === draft.scope_type && i.scope_value === (draft.scope_type === 'global' ? '*' : draft.scope_value.trim())),
        [items, draft.scope_type, draft.scope_value]
    );

    const createMutation = useMutation({
        mutationFn: (payload: EditablePolicy) => contextPoliciesApi.create(payload),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['contextPolicies'] });
            setDraft(NEW_POLICY);
            setTrustedNetworksText('');
            setValidationError('');
            showToast('Context policy saved successfully', 'success');
        },
        onError: (err: unknown) => {
            const msg = err instanceof Error ? err.message : 'Failed to save context policy';
            showToast(msg, 'error');
        },
    });

    const updateMutation = useMutation({
        mutationFn: ({ id, payload }: { id: number; payload: EditablePolicy }) => contextPoliciesApi.update(id, payload),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['contextPolicies'] });
            showToast('Context policy updated', 'success');
        },
        onError: (err: unknown) => {
            const msg = err instanceof Error ? err.message : 'Failed to update context policy';
            showToast(msg, 'error');
        },
    });

    const deleteMutation = useMutation({
        mutationFn: (id: number) => contextPoliciesApi.remove(id),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['contextPolicies'] });
            showToast('Context policy deleted', 'success');
        },
        onError: (err: unknown) => {
            const msg = err instanceof Error ? err.message : 'Failed to delete context policy';
            showToast(msg, 'error');
        },
    });

    const globalExists = useMemo(
        () => items.some(i => i.scope_type === 'global' && i.scope_value === '*'),
        [items]
    );

    const hasPendingMutation = createMutation.isPending || updateMutation.isPending || deleteMutation.isPending;

    const validateDraft = (input: EditablePolicy, trustedNetworksRaw: string): string | null => {
        if (!input.name.trim()) return 'Policy name is required.';
        const scopeValue = input.scope_type === 'global' ? '*' : input.scope_value.trim();
        if (!scopeValue) return 'Scope value is required.';
        if (input.scope_type === 'agent' && !agentOptions.some(o => o.value === scopeValue)) {
            return 'Please select a valid agent from the dropdown.';
        }
        if (input.scope_type === 'user' && !userOptions.some(o => o.value === scopeValue)) {
            return 'Please select a valid user from the dropdown.';
        }
        if (input.user_role_weight <= 0 || input.device_criticality_weight <= 0 || input.network_anomaly_factor <= 0) {
            return 'All weights/factors must be greater than 0.';
        }
        const cidrs = parseTrustedNetworks(trustedNetworksRaw);
        if (cidrs.some(c => !CIDR_PATTERN.test(c))) {
            return 'Trusted networks must be valid CIDR values (example: 10.10.0.0/16).';
        }
        return null;
    };

    const handleCreatePolicy = () => {
        const errMsg = validateDraft(draft, trustedNetworksText);
        if (errMsg) {
            setValidationError(errMsg);
            showToast(errMsg, 'warning');
            return;
        }
        setValidationError('');
        const payload: EditablePolicy = {
            ...draft,
            name: draft.name.trim(),
            scope_value: draft.scope_type === 'global' ? '*' : draft.scope_value.trim(),
            trusted_networks: parseTrustedNetworks(trustedNetworksText),
            notes: (draft.notes || '').trim(),
        };
        createMutation.mutate(payload);
    };

    return (
        <div className="space-y-6">
            <div>
                <h2 className="text-xl font-bold text-gray-900 dark:text-white">Context Policies</h2>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    Hybrid context-aware controls. System infers context automatically, and these policies tune risk weighting by global, agent, and user scope.
                </p>
            </div>

            <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4 space-y-3">
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-3">
                    <label className="space-y-1">
                        <div className="text-xs font-semibold text-gray-600 dark:text-gray-300 uppercase tracking-wide">Policy Name</div>
                        <input
                            className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-900"
                            placeholder="Example: Finance Laptops - High Sensitivity"
                            value={draft.name}
                            onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                        />
                    </label>

                    <label className="space-y-1">
                        <div className="text-xs font-semibold text-gray-600 dark:text-gray-300 uppercase tracking-wide">Scope Type</div>
                        <select
                            className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-900"
                            value={draft.scope_type}
                            onChange={(e) => {
                                const nextScope = e.target.value as EditablePolicy['scope_type'];
                                setDraft(prev => ({
                                    ...prev,
                                    scope_type: nextScope,
                                    scope_value: nextScope === 'global' ? '*' : (prev.scope_value === '*' ? '' : prev.scope_value),
                                }));
                            }}
                        >
                            <option value="global">global</option>
                            <option value="agent">agent</option>
                            <option value="user">user</option>
                        </select>
                    </label>

                    <label className="space-y-1">
                        <div className="text-xs font-semibold text-gray-600 dark:text-gray-300 uppercase tracking-wide">Scope Value</div>
                        {draft.scope_type === 'global' ? (
                            <input
                                className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-900"
                                placeholder="Example: *"
                                value="*"
                                disabled
                                readOnly
                            />
                        ) : (
                            <select
                                className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-900"
                                value={draft.scope_value}
                                disabled={draft.scope_type === 'agent' ? isAgentsLoading : isUsersLoading}
                                onChange={(e) => setDraft({ ...draft, scope_value: e.target.value })}
                            >
                                <option value="">
                                    {draft.scope_type === 'agent'
                                        ? (isAgentsLoading ? 'Loading agents...' : 'Select agent...')
                                        : (isUsersLoading ? 'Loading users...' : 'Select user...')}
                                </option>
                                {(draft.scope_type === 'agent' ? agentOptions : userOptions).map((opt) => (
                                    <option key={opt.value} value={opt.value}>{opt.label}</option>
                                ))}
                            </select>
                        )}
                        {draft.scope_type === 'agent' && (
                            <div className="mt-2 flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
                                <span>{agentOptions.length === 0 && !isAgentsLoading ? 'No agents found yet.' : `${agentOptions.length} agents available`}</span>
                                <button
                                    type="button"
                                    className="underline hover:text-gray-700 dark:hover:text-gray-200"
                                    onClick={() => refetchAgents()}
                                >
                                    Refresh agents
                                </button>
                            </div>
                        )}
                        {draft.scope_type === 'user' && (
                            <div className="mt-2 flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
                                <span>{userOptions.length === 0 && !isUsersLoading ? 'No users found yet.' : `${userOptions.length} users available`}</span>
                                <button
                                    type="button"
                                    className="underline hover:text-gray-700 dark:hover:text-gray-200"
                                    onClick={() => refetchUsers()}
                                >
                                    Refresh users
                                </button>
                            </div>
                        )}
                    </label>

                    <label className="space-y-1">
                        <div className="text-xs font-semibold text-gray-600 dark:text-gray-300 uppercase tracking-wide">Policy Status</div>
                        <div className="h-[42px] px-3 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-900 flex items-center">
                            <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
                                <input
                                    type="checkbox"
                                    checked={draft.enabled}
                                    onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })}
                                />
                                Enabled
                            </label>
                        </div>
                    </label>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                    <label className="space-y-1">
                        <div className="text-xs font-semibold text-gray-600 dark:text-gray-300 uppercase tracking-wide">User Role Weight</div>
                        <input
                            type="number"
                            step="0.1"
                            min="0.1"
                            className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-900"
                            placeholder="Example: 1.2 (1.0 neutral)"
                            value={draft.user_role_weight}
                            onChange={(e) => setDraft({ ...draft, user_role_weight: Number(e.target.value) })}
                        />
                    </label>

                    <label className="space-y-1">
                        <div className="text-xs font-semibold text-gray-600 dark:text-gray-300 uppercase tracking-wide">Device Criticality Weight</div>
                        <input
                            type="number"
                            step="0.1"
                            min="0.1"
                            className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-900"
                            placeholder="Example: 1.4 for critical servers"
                            value={draft.device_criticality_weight}
                            onChange={(e) => setDraft({ ...draft, device_criticality_weight: Number(e.target.value) })}
                        />
                    </label>

                    <label className="space-y-1">
                        <div className="text-xs font-semibold text-gray-600 dark:text-gray-300 uppercase tracking-wide">Network Anomaly Factor</div>
                        <input
                            type="number"
                            step="0.1"
                            min="0.1"
                            className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-900"
                            placeholder="Example: 1.1 outside trusted network"
                            value={draft.network_anomaly_factor}
                            onChange={(e) => setDraft({ ...draft, network_anomaly_factor: Number(e.target.value) })}
                        />
                    </label>
                </div>

                <label className="space-y-1 block">
                    <div className="text-xs font-semibold text-gray-600 dark:text-gray-300 uppercase tracking-wide">Trusted Networks (CIDR)</div>
                    <input
                        className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-900"
                        placeholder="Example: 10.0.0.0/8, 192.168.1.0/24, 172.16.0.0/12"
                        value={trustedNetworksText}
                        onChange={(e) => {
                            setTrustedNetworksText(e.target.value);
                            setDraft({
                                ...draft,
                                trusted_networks: parseTrustedNetworks(e.target.value),
                            });
                        }}
                    />
                </label>

                <label className="space-y-1 block">
                    <div className="text-xs font-semibold text-gray-600 dark:text-gray-300 uppercase tracking-wide">Notes</div>
                    <input
                        className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-900"
                        placeholder="Example: Owner=SOC, reason=Tier-0 protection, review=monthly"
                        value={draft.notes || ''}
                        onChange={(e) => setDraft({ ...draft, notes: e.target.value })}
                    />
                </label>
                <button
                    onClick={handleCreatePolicy}
                    className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-blue-600 text-white hover:bg-blue-500 disabled:opacity-50"
                    disabled={hasPendingMutation || !draft.name || !(draft.scope_type === 'global' ? '*' : draft.scope_value.trim())}
                >
                    <Plus size={16} /> Add Policy
                </button>
                {matchingScope && (
                    <div className="text-xs text-blue-600 dark:text-blue-400">
                        Note: a policy already exists for `{matchingScope.scope_type}:{matchingScope.scope_value}`. Saving will replace it (upsert semantics).
                    </div>
                )}
                {validationError && (
                    <div className="text-xs text-red-600 dark:text-red-400">
                        {validationError}
                    </div>
                )}
                {!globalExists && (
                    <div className="text-xs text-amber-600 dark:text-amber-400">
                        Warning: global baseline policy is missing. Create one with scope `global` and scope value `*`.
                    </div>
                )}
            </div>

            <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
                {isLoading ? (
                    <div className="text-sm text-gray-500">Loading context policies...</div>
                ) : error ? (
                    <div className="text-sm text-red-500">Failed to load context policies.</div>
                ) : (
                    <div className="space-y-2">
                        {items.map((item) => (
                            <div key={item.id} className="flex items-center justify-between gap-3 p-3 rounded-lg border border-gray-200 dark:border-gray-700">
                                <div className="min-w-0">
                                    <div className="text-sm font-semibold text-gray-900 dark:text-white truncate">
                                        {item.name} <span className="text-xs font-normal text-gray-500">({item.scope_type}:{item.scope_value})</span>
                                    </div>
                                    <div className="text-xs text-gray-500 mt-1">
                                        W(user/dev/net): {item.user_role_weight.toFixed(2)} / {item.device_criticality_weight.toFixed(2)} / {item.network_anomaly_factor.toFixed(2)}
                                    </div>
                                </div>
                                <div className="flex items-center gap-2">
                                    <button
                                        onClick={() => updateMutation.mutate({
                                            id: item.id,
                                            payload: { ...item, enabled: !item.enabled },
                                        })}
                                        className="px-3 py-1.5 text-xs rounded-md border border-gray-300 dark:border-gray-700"
                                    >
                                        <Save size={14} className="inline mr-1" /> {item.enabled ? 'Disable' : 'Enable'}
                                    </button>
                                    <button
                                        onClick={() => deleteMutation.mutate(item.id)}
                                        className="px-3 py-1.5 text-xs rounded-md border border-red-300 text-red-600 dark:border-red-700"
                                        disabled={item.scope_type === 'global' && item.scope_value === '*'}
                                        title={item.scope_type === 'global' && item.scope_value === '*' ? 'Keep global baseline policy in place' : undefined}
                                    >
                                        <Trash2 size={14} className="inline mr-1" /> Delete
                                    </button>
                                </div>
                            </div>
                        ))}
                        {items.length === 0 && (
                            <div className="text-sm text-gray-500">No context policies found.</div>
                        )}
                    </div>
                )}
            </div>
        </div>
    );
}

