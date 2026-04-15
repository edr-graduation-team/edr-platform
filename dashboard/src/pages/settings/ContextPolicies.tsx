import { useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Plus, Save, Trash2 } from 'lucide-react';
import { contextPoliciesApi, type ContextPolicy } from '../../api/client';

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

export default function ContextPolicies() {
    const queryClient = useQueryClient();
    const [draft, setDraft] = useState<EditablePolicy>(NEW_POLICY);

    const { data, isLoading, error } = useQuery({
        queryKey: ['contextPolicies'],
        queryFn: contextPoliciesApi.list,
        refetchInterval: 15000,
    });

    const items = data?.data ?? [];

    const createMutation = useMutation({
        mutationFn: (payload: EditablePolicy) => contextPoliciesApi.create(payload),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['contextPolicies'] });
            setDraft(NEW_POLICY);
        },
    });

    const updateMutation = useMutation({
        mutationFn: ({ id, payload }: { id: number; payload: EditablePolicy }) => contextPoliciesApi.update(id, payload),
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['contextPolicies'] }),
    });

    const deleteMutation = useMutation({
        mutationFn: (id: number) => contextPoliciesApi.remove(id),
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['contextPolicies'] }),
    });

    const globalExists = useMemo(
        () => items.some(i => i.scope_type === 'global' && i.scope_value === '*'),
        [items]
    );

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
                            onChange={(e) => setDraft({ ...draft, scope_type: e.target.value as EditablePolicy['scope_type'] })}
                        >
                            <option value="global">global</option>
                            <option value="agent">agent</option>
                            <option value="user">user</option>
                        </select>
                    </label>

                    <label className="space-y-1">
                        <div className="text-xs font-semibold text-gray-600 dark:text-gray-300 uppercase tracking-wide">Scope Value</div>
                        <input
                            className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-900"
                            placeholder={
                                draft.scope_type === 'global'
                                    ? 'Example: *'
                                    : draft.scope_type === 'agent'
                                        ? 'Example: 7f8c2a1b-4c11-4fd6-b2ae-...'
                                        : 'Example: admin or svc-backup'
                            }
                            value={draft.scope_value}
                            onChange={(e) => setDraft({ ...draft, scope_value: e.target.value })}
                        />
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
                        value={draft.trusted_networks.join(',')}
                        onChange={(e) => setDraft({
                            ...draft,
                            trusted_networks: e.target.value.split(',').map(v => v.trim()).filter(Boolean),
                        })}
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
                    onClick={() => createMutation.mutate(draft)}
                    className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-blue-600 text-white hover:bg-blue-500 disabled:opacity-50"
                    disabled={createMutation.isPending || !draft.name || !draft.scope_value}
                >
                    <Plus size={16} /> Add Policy
                </button>
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

