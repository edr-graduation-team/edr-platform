import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { Plus, Search, Edit, Trash2, Power, PowerOff, CheckCircle, XCircle } from 'lucide-react';
import { rulesApi, statsApi, type Rule } from '../api/client';

export default function Rules() {
    const queryClient = useQueryClient();
    const [searchTerm, setSearchTerm] = useState('');
    const [showModal, setShowModal] = useState(false);
    const [editingRule, setEditingRule] = useState<Rule | null>(null);
    const [enabledFilter, setEnabledFilter] = useState<boolean | undefined>(undefined);

    // Fetch rules
    const { data, isLoading } = useQuery({
        queryKey: ['rules', enabledFilter],
        queryFn: () => rulesApi.list({ limit: 100, enabled: enabledFilter }),
    });

    // Fetch global rule stats for accurate stat cards
    const { data: ruleStats } = useQuery({
        queryKey: ['ruleStats'],
        queryFn: statsApi.rules,
    });

    // Enable/Disable mutations
    const enableMutation = useMutation({
        mutationFn: (id: string) => rulesApi.enable(id),
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['rules'] }),
    });

    const disableMutation = useMutation({
        mutationFn: (id: string) => rulesApi.disable(id),
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['rules'] }),
    });

    const deleteMutation = useMutation({
        mutationFn: (id: string) => rulesApi.delete(id),
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['rules'] }),
    });

    const filteredRules = (data?.rules || []).filter((rule) =>
        rule.title.toLowerCase().includes(searchTerm.toLowerCase()) ||
        rule.id.toLowerCase().includes(searchTerm.toLowerCase())
    );

    const severityColors: Record<string, string> = {
        critical: 'badge-critical',
        high: 'badge-high',
        medium: 'badge-medium',
        low: 'badge-low',
    };

    return (
        <div>
            <div className="flex justify-between items-center mb-6">
                <h1 className="text-3xl font-bold text-gray-900 dark:text-white">Rules</h1>
                <button
                    onClick={() => {
                        setEditingRule(null);
                        setShowModal(true);
                    }}
                    className="btn btn-primary flex items-center gap-2"
                >
                    <Plus className="w-4 h-4" />
                    Add Rule
                </button>
            </div>

            {/* Filters */}
            <div className="card mb-6">
                <div className="flex gap-4">
                    <div className="flex-1 flex items-center gap-2">
                        <Search className="w-4 h-4 text-gray-400" />
                        <input
                            type="text"
                            placeholder="Search rules..."
                            value={searchTerm}
                            onChange={(e) => setSearchTerm(e.target.value)}
                            className="input"
                        />
                    </div>
                    <select
                        value={enabledFilter === undefined ? '' : enabledFilter.toString()}
                        onChange={(e) => {
                            const val = e.target.value;
                            setEnabledFilter(val === '' ? undefined : val === 'true');
                        }}
                        className="input w-40"
                    >
                        <option value="">All Rules</option>
                        <option value="true">Enabled</option>
                        <option value="false">Disabled</option>
                    </select>
                </div>
            </div>

            {/* Stats — sourced from global /stats/rules endpoint, NOT the paginated response */}
            <div className="grid grid-cols-3 gap-4 mb-6">
                <div className="card text-center">
                    <p className="text-3xl font-bold text-gray-900 dark:text-white">
                        {ruleStats?.total_rules ?? data?.total ?? 0}
                    </p>
                    <p className="text-sm text-gray-500">Total Rules</p>
                </div>
                <div className="card text-center">
                    <p className="text-3xl font-bold text-green-600">
                        {ruleStats?.enabled_rules ?? 0}
                    </p>
                    <p className="text-sm text-gray-500">Enabled</p>
                </div>
                <div className="card text-center">
                    <p className="text-3xl font-bold text-gray-400">
                        {ruleStats?.disabled_rules ?? 0}
                    </p>
                    <p className="text-sm text-gray-500">Disabled</p>
                </div>
            </div>

            {/* Rules Table */}
            <div className="card overflow-hidden p-0">
                {isLoading ? (
                    <div className="p-8 text-center">Loading...</div>
                ) : (
                    <table className="table">
                        <thead className="bg-gray-50 dark:bg-gray-800">
                            <tr>
                                <th>ID</th>
                                <th>Title</th>
                                <th>Severity</th>
                                <th>Category</th>
                                <th>Status</th>
                                <th>Actions</th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                            {filteredRules.map((rule) => (
                                <tr key={rule.id}>
                                    <td className="font-mono text-sm">{rule.id.slice(0, 20)}...</td>
                                    <td>
                                        <div className="font-medium">{rule.title}</div>
                                        <div className="text-xs text-gray-500 max-w-xs truncate">
                                            {rule.description}
                                        </div>
                                    </td>
                                    <td>
                                        <span className={`badge ${severityColors[rule.severity] || 'badge-low'}`}>
                                            {(rule.severity || 'low').toUpperCase()}
                                        </span>
                                    </td>
                                    <td className="text-sm text-gray-500">{rule.category || '-'}</td>
                                    <td>
                                        {rule.enabled ? (
                                            <span className="flex items-center gap-1 text-green-600">
                                                <CheckCircle className="w-4 h-4" />
                                                Enabled
                                            </span>
                                        ) : (
                                            <span className="flex items-center gap-1 text-gray-400">
                                                <XCircle className="w-4 h-4" />
                                                Disabled
                                            </span>
                                        )}
                                    </td>
                                    <td>
                                        <div className="flex gap-1">
                                            <button
                                                onClick={() => {
                                                    setEditingRule(rule);
                                                    setShowModal(true);
                                                }}
                                                className="p-1.5 text-gray-500 hover:text-primary-600 rounded hover:bg-gray-100"
                                                title="Edit"
                                            >
                                                <Edit className="w-4 h-4" />
                                            </button>
                                            {rule.enabled ? (
                                                <button
                                                    onClick={() => disableMutation.mutate(rule.id)}
                                                    className="p-1.5 text-gray-500 hover:text-orange-600 rounded hover:bg-gray-100"
                                                    title="Disable"
                                                >
                                                    <PowerOff className="w-4 h-4" />
                                                </button>
                                            ) : (
                                                <button
                                                    onClick={() => enableMutation.mutate(rule.id)}
                                                    className="p-1.5 text-gray-500 hover:text-green-600 rounded hover:bg-gray-100"
                                                    title="Enable"
                                                >
                                                    <Power className="w-4 h-4" />
                                                </button>
                                            )}
                                            <button
                                                onClick={() => {
                                                    if (confirm('Delete this rule?')) {
                                                        deleteMutation.mutate(rule.id);
                                                    }
                                                }}
                                                className="p-1.5 text-gray-500 hover:text-red-600 rounded hover:bg-gray-100"
                                                title="Delete"
                                            >
                                                <Trash2 className="w-4 h-4" />
                                            </button>
                                        </div>
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                )}
            </div>

            {/* Modal Placeholder */}
            {showModal && (
                <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
                    <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl p-6 w-full max-w-lg">
                        <h2 className="text-xl font-bold mb-4">
                            {editingRule ? 'Edit Rule' : 'Create Rule'}
                        </h2>
                        <p className="text-gray-500">
                            Rule form implementation coming soon...
                        </p>
                        <div className="mt-6 flex justify-end gap-2">
                            <button
                                onClick={() => setShowModal(false)}
                                className="btn bg-gray-200 text-gray-800 hover:bg-gray-300"
                            >
                                Cancel
                            </button>
                            <button
                                onClick={() => setShowModal(false)}
                                className="btn btn-primary"
                            >
                                Save
                            </button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}
