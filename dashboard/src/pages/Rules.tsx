import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { Plus, Search, Edit, Trash2, Power, PowerOff, CheckCircle, XCircle, Shield, ChevronDown, Activity, Settings2 } from 'lucide-react';
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


    return (
        <div className="relative flex flex-col min-h-[calc(100vh-2rem)] lg:min-h-[calc(100vh-1rem)] h-full -mx-4 sm:-mx-6 lg:-mx-8 -my-4 sm:-my-6 lg:-my-8 p-4 sm:p-6 lg:p-8 bg-slate-50 dark:bg-gradient-to-br dark:from-slate-900 dark:via-[#0b1120] dark:to-slate-900 transition-colors overflow-hidden">
            {/* Ambient Glow */}
            <div className="absolute top-0 right-0 w-[500px] h-[500px] pointer-events-none mix-blend-screen" style={{ background: 'radial-gradient(circle, rgba(99,102,241,0.08) 0%, transparent 70%)' }} />

            <div className="relative flex-1 flex flex-col min-h-0 space-y-4 lg:space-y-6 max-w-[1600px] mx-auto w-full">
                {/* Header Section */}
                <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 shrink-0">
                    <div>
                        <h1 className="text-2xl font-bold bg-clip-text text-transparent bg-gradient-to-r from-gray-900 to-gray-600 dark:from-white dark:to-gray-300">
                            Detection Rules
                        </h1>
                        <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">Manage Sigma and YARA behavioral signatures</p>
                    </div>
                    
                    <button
                        onClick={() => {
                            setEditingRule(null);
                            setShowModal(true);
                        }}
                        className="flex items-center gap-2 px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white text-sm font-medium rounded-lg transition-colors shadow-sm shadow-indigo-500/20 pointer-events-auto cursor-pointer"
                    >
                        <Plus className="w-4 h-4" />
                        Create Rule
                    </button>
                </div>

                {/* KPI Cards */}
                <div className="grid grid-cols-1 md:grid-cols-3 gap-4 shrink-0">
                    <div className="relative overflow-hidden bg-white/60 dark:bg-slate-900/40 backdrop-blur-md rounded-xl border border-slate-200/80 dark:border-slate-700/50 p-5 shadow-sm transition-all hover:shadow-md group">
                        <div className="flex items-center gap-4 relative z-10">
                            <div className="p-3 rounded-lg bg-indigo-500/10 dark:bg-indigo-500/20 text-indigo-600 dark:text-indigo-400">
                                <Shield className="w-6 h-6" />
                            </div>
                            <div>
                                <div className="text-sm font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider mb-1">Total Rules</div>
                                <div className="text-2xl font-bold text-slate-900 dark:text-white">{ruleStats?.total_rules ?? data?.total ?? 0}</div>
                            </div>
                        </div>
                    </div>
                    <div className="relative overflow-hidden bg-white/60 dark:bg-slate-900/40 backdrop-blur-md rounded-xl border border-slate-200/80 dark:border-slate-700/50 p-5 shadow-sm transition-all hover:shadow-md group">
                        <div className="flex items-center gap-4 relative z-10">
                            <div className="p-3 rounded-lg bg-emerald-500/10 dark:bg-emerald-500/20 text-emerald-600 dark:text-emerald-400">
                                <CheckCircle className="w-6 h-6" />
                            </div>
                            <div>
                                <div className="text-sm font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider mb-1">Active Rules</div>
                                <div className="text-2xl font-bold text-slate-900 dark:text-white">{ruleStats?.enabled_rules ?? 0}</div>
                            </div>
                        </div>
                    </div>
                    <div className="relative overflow-hidden bg-white/60 dark:bg-slate-900/40 backdrop-blur-md rounded-xl border border-slate-200/80 dark:border-slate-700/50 p-5 shadow-sm transition-all hover:shadow-md group">
                        <div className="flex items-center gap-4 relative z-10">
                            <div className="p-3 rounded-lg bg-slate-500/10 dark:bg-slate-500/20 text-slate-600 dark:text-slate-400">
                                <PowerOff className="w-6 h-6" />
                            </div>
                            <div>
                                <div className="text-sm font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider mb-1">Disabled</div>
                                <div className="text-2xl font-bold text-slate-900 dark:text-white">{ruleStats?.disabled_rules ?? 0}</div>
                            </div>
                        </div>
                    </div>
                </div>

                {/* Filter Bar */}
                <div className="relative z-20 shrink-0 bg-white/60 dark:bg-slate-900/40 backdrop-blur-md border border-slate-200/80 dark:border-slate-700/50 rounded-xl p-4 shadow-sm flex flex-col md:flex-row gap-4 items-end">
                    <div className="w-full md:w-80 relative">
                        <label className="block text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider mb-2">
                            Search Signatures
                        </label>
                        <div className="relative">
                            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-400" />
                            <input
                                type="text"
                                placeholder="Search by title or ID..."
                                value={searchTerm}
                                onChange={(e) => setSearchTerm(e.target.value)}
                                className="w-full bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-700/80 text-slate-700 dark:text-slate-200 rounded-lg pl-9 pr-3 py-2 text-sm focus:ring-2 focus:ring-indigo-500/50 focus:border-indigo-500 transition-all hover:bg-white dark:hover:bg-slate-800"
                            />
                        </div>
                    </div>
                    <div className="w-full md:w-48 relative">
                        <label className="block text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider mb-2">
                            Rule Status
                        </label>
                        <div className="relative">
                            <select
                                value={enabledFilter === undefined ? '' : enabledFilter.toString()}
                                onChange={(e) => {
                                    const val = e.target.value;
                                    setEnabledFilter(val === '' ? undefined : val === 'true');
                                }}
                                className="appearance-none w-full bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-700/80 text-slate-700 dark:text-slate-200 rounded-lg pl-3 pr-8 py-2 text-sm focus:ring-2 focus:ring-indigo-500/50 focus:border-indigo-500 transition-all hover:bg-white dark:hover:bg-slate-800 cursor-pointer pointer-events-auto"
                            >
                                <option value="">All Rules</option>
                                <option value="true">Active Signatures</option>
                                <option value="false">Disabled Rules</option>
                            </select>
                            <ChevronDown className="absolute right-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-400 pointer-events-none" />
                        </div>
                    </div>
                </div>

                {/* Data Grid */}
                <div className="relative flex-1 flex flex-col min-h-0 bg-white dark:bg-slate-800/70 rounded-2xl border border-slate-200 dark:border-slate-700/60 shadow-sm overflow-hidden mt-2">
                    <div className="flex-1 overflow-auto custom-scrollbar">
                        {isLoading ? (
                            <div className="p-8 flex justify-center items-center">
                                <div className="space-y-4 w-full">
                                    <div className="h-12 bg-slate-200 dark:bg-slate-800 rounded animate-pulse w-full"></div>
                                    <div className="h-12 bg-slate-200 dark:bg-slate-800 rounded animate-pulse w-full opacity-80"></div>
                                    <div className="h-12 bg-slate-200 dark:bg-slate-800 rounded animate-pulse w-full opacity-60"></div>
                                </div>
                            </div>
                        ) : filteredRules.length === 0 ? (
                            <div className="text-center py-20 flex flex-col items-center justify-center">
                                <Activity className="w-12 h-12 text-slate-400 dark:text-slate-600 mb-4" />
                                <h3 className="text-lg font-medium text-slate-900 dark:text-slate-100 mb-1">No rules matched</h3>
                                <p className="text-slate-500 dark:text-slate-400">Try adjusting your filters or search terms.</p>
                            </div>
                        ) : (
                            <table className="w-full text-left border-collapse">
                                <thead className="sticky top-0 z-10 bg-slate-100 dark:bg-slate-800 border-b-2 border-slate-200 dark:border-slate-700/80">
                                    <tr>
                                        <th className="px-6 py-4 text-xs uppercase tracking-wider text-slate-400 font-semibold w-1/4">Rule ID</th>
                                        <th className="px-6 py-4 text-xs uppercase tracking-wider text-slate-400 font-semibold w-1/3">Signature Definition</th>
                                        <th className="px-6 py-4 text-xs uppercase tracking-wider text-slate-400 font-semibold">Severity</th>
                                        <th className="px-6 py-4 text-xs uppercase tracking-wider text-slate-400 font-semibold">Category</th>
                                        <th className="px-6 py-4 text-xs uppercase tracking-wider text-slate-400 font-semibold">Status</th>
                                        <th className="px-6 py-4 text-xs uppercase tracking-wider text-slate-400 font-semibold text-right">Actions</th>
                                    </tr>
                                </thead>
                                <tbody className="divide-y divide-slate-100 dark:divide-slate-800/60">
                                    {filteredRules.map((rule) => (
                                        <tr key={rule.id} className="hover:bg-slate-50 dark:hover:bg-slate-800/40 transition-colors group">
                                            <td className="px-6 py-4">
                                                <div className="font-mono text-xs font-medium text-slate-500 dark:text-slate-400 group-hover:text-indigo-600 dark:group-hover:text-indigo-400 transition-colors">
                                                    {rule.id.slice(0, 20)}...
                                                </div>
                                            </td>
                                            <td className="px-6 py-4">
                                                <div className="font-semibold text-sm text-slate-900 dark:text-slate-100 flex items-center gap-2">
                                                    {rule.title}
                                                </div>
                                                <div className="text-xs text-slate-500 dark:text-slate-400 mt-1 max-w-xs md:max-w-md truncate" title={rule.description}>
                                                    {rule.description}
                                                </div>
                                            </td>
                                            <td className="px-6 py-4">
                                                <span className={`inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-semibold border ${
                                                    rule.severity === 'critical' ? 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border-rose-500/20' :
                                                    rule.severity === 'high' ? 'bg-orange-500/10 text-orange-600 dark:text-orange-400 border-orange-500/20' :
                                                    rule.severity === 'medium' ? 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20' :
                                                    'bg-blue-500/10 text-blue-600 dark:text-blue-400 border-blue-500/20'
                                                }`}>
                                                    {(rule.severity || 'low').toUpperCase()}
                                                </span>
                                            </td>
                                            <td className="px-6 py-4">
                                                <span className="text-xs px-2 py-1 rounded bg-slate-100 dark:bg-slate-900/60 text-slate-600 dark:text-slate-300 font-medium whitespace-nowrap">
                                                    {rule.category || 'N/A'}
                                                </span>
                                            </td>
                                            <td className="px-6 py-4">
                                                {rule.enabled ? (
                                                    <span className="flex items-center gap-1.5 text-xs font-semibold text-emerald-600 dark:text-emerald-400 mt-0.5">
                                                        <CheckCircle className="w-3.5 h-3.5" /> Active
                                                    </span>
                                                ) : (
                                                    <span className="flex items-center gap-1.5 text-xs font-semibold text-slate-400 dark:text-slate-500 mt-0.5">
                                                        <XCircle className="w-3.5 h-3.5" /> Disabled
                                                    </span>
                                                )}
                                            </td>
                                            <td className="px-6 py-4 text-right">
                                                <div className="flex items-center justify-end gap-1.5 md:opacity-0 group-hover:opacity-100 transition-opacity">
                                                    <button
                                                        onClick={() => {
                                                            setEditingRule(rule);
                                                            setShowModal(true);
                                                        }}
                                                        className="p-1.5 rounded-md hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-400 hover:text-indigo-600 dark:hover:text-indigo-400 transition-colors pointer-events-auto cursor-pointer"
                                                        title="Edit Rule"
                                                    >
                                                        <Edit className="w-4 h-4" />
                                                    </button>
                                                    
                                                    {rule.enabled ? (
                                                        <button
                                                            onClick={() => disableMutation.mutate(rule.id)}
                                                            className="p-1.5 rounded-md hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-400 hover:text-orange-600 dark:hover:text-orange-400 transition-colors pointer-events-auto cursor-pointer"
                                                            title="Disable Rule"
                                                        >
                                                            <PowerOff className="w-4 h-4" />
                                                        </button>
                                                    ) : (
                                                        <button
                                                            onClick={() => enableMutation.mutate(rule.id)}
                                                            className="p-1.5 rounded-md hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-400 hover:text-emerald-600 dark:hover:text-emerald-400 transition-colors pointer-events-auto cursor-pointer"
                                                            title="Enable Rule"
                                                        >
                                                            <Power className="w-4 h-4" />
                                                        </button>
                                                    )}
                                                    
                                                    <div className="w-px h-4 bg-slate-200 dark:bg-slate-700 mx-1"></div>

                                                    <button
                                                        onClick={() => {
                                                            if (confirm('Delete this rule? This action cannot be undone.')) {
                                                                deleteMutation.mutate(rule.id);
                                                            }
                                                        }}
                                                        className="p-1.5 rounded-md hover:bg-rose-50 dark:hover:bg-rose-500/10 text-slate-400 hover:text-rose-600 dark:hover:text-rose-400 transition-colors pointer-events-auto cursor-pointer"
                                                        title="Delete Rule"
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

                    {/* Pagination / Item Count */}
                    <div className="shrink-0 px-4 py-3 bg-slate-50/50 dark:bg-slate-900/40 border-t border-slate-200 dark:border-slate-800/60 text-sm text-slate-500 dark:text-slate-400 flex justify-between items-center mt-auto">
                        <span>Showing <span className="font-semibold text-slate-700 dark:text-slate-200">{filteredRules.length}</span> signatures</span>
                    </div>
                </div>
            </div>

            {/* Modal - Placeholder for Glass UI overlay */}
            {showModal && (
                <div className="fixed inset-0 bg-slate-900/30 dark:bg-slate-900/60 backdrop-blur-sm flex items-center justify-center z-50">
                    <div className="bg-white dark:bg-slate-800 rounded-2xl shadow-2xl p-6 w-full max-w-lg border border-slate-200 dark:border-slate-700 m-4">
                        <div className="flex items-center gap-3 mb-6">
                            <div className="p-2.5 rounded-lg bg-indigo-500/10 text-indigo-600 dark:text-indigo-400">
                                <Settings2 className="w-5 h-5" />
                            </div>
                            <div>
                                <h2 className="text-xl font-bold text-slate-900 dark:text-white">
                                    {editingRule ? 'Edit Signature' : 'Deploy new Rule'}
                                </h2>
                                <p className="text-sm text-slate-500 dark:text-slate-400">Configure signature settings below</p>
                            </div>
                        </div>

                        <div className="bg-slate-50 dark:bg-slate-900/50 p-4 rounded-xl border border-slate-200 dark:border-slate-700 mb-6 text-sm text-slate-600 dark:text-slate-400">
                            Sigma rules and YARA signature configuration is coming soon to the new interface module. For now, please manage rules via the CLI.
                        </div>
                        
                        <div className="flex justify-end gap-3 mt-6">
                            <button
                                onClick={() => setShowModal(false)}
                                className="px-4 py-2 border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 hover:bg-slate-50 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300 text-sm font-medium rounded-lg transition-colors pointer-events-auto cursor-pointer"
                            >
                                Dismiss
                            </button>
                            <button
                                onClick={() => setShowModal(false)}
                                className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white text-sm font-medium rounded-lg transition-colors shadow-sm pointer-events-auto cursor-pointer"
                            >
                                View Documentation
                            </button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}
