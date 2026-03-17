import { useState, useEffect, Fragment } from 'react';
import { Save, RefreshCw, Shield, AlertCircle, Check, Lock } from 'lucide-react';
import { rolesApi, type Role, type Permission } from '../../api/client';
import { ROLE_COLORS } from './types';

export default function RBACMatrix() {
    const [roles, setRoles] = useState<Role[]>([]);
    const [allPerms, setAllPerms] = useState<Permission[]>([]);
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');
    const [feedback, setFeedback] = useState('');
    // Track unsaved edits: roleId → Set<permId>
    const [edits, setEdits] = useState<Record<string, Set<string>>>({});

    const fetchData = async () => {
        setLoading(true); setError('');
        try {
            const [r, p] = await Promise.all([rolesApi.list(), rolesApi.permissions()]);
            setRoles(r || []);
            setAllPerms(p || []);
        } catch (err: any) {
            setError(err?.response?.data?.message || 'Failed to load RBAC data');
        } finally { setLoading(false); }
    };

    useEffect(() => { fetchData(); }, []);
    useEffect(() => { if (feedback) { const t = setTimeout(() => setFeedback(''), 3500); return () => clearTimeout(t); } }, [feedback]);

    // Group permissions by resource
    const resources = allPerms.reduce<Record<string, Permission[]>>((acc, p) => {
        (acc[p.resource] ??= []).push(p);
        return acc;
    }, {});

    const hasPermission = (role: Role, permId: string): boolean => {
        if (edits[role.id]) return edits[role.id].has(permId);
        return role.permissions?.some(p => p.id === permId) ?? false;
    };

    const toggle = (role: Role, permId: string) => {
        // Admin built-in always has all — immutable
        if (role.name === 'admin' && role.is_built_in) return;

        const current = edits[role.id] ?? new Set(role.permissions?.map(p => p.id) || []);
        const next = new Set(current);
        next.has(permId) ? next.delete(permId) : next.add(permId);
        setEdits(prev => ({ ...prev, [role.id]: next }));
    };

    const dirty = Object.keys(edits).length > 0;

    const handleSave = async () => {
        setSaving(true); setError('');
        try {
            for (const [roleId, permIds] of Object.entries(edits)) {
                await rolesApi.updatePermissions(roleId, Array.from(permIds));
            }
            setEdits({});
            setFeedback('Permissions saved successfully');
            fetchData();
        } catch (err: any) {
            setError(err?.response?.data?.message || 'Failed to save');
        } finally { setSaving(false); }
    };

    if (loading) {
        return (
            <div className="flex items-center justify-center py-24 text-gray-500 dark:text-gray-400">
                <RefreshCw size={18} className="animate-spin mr-2" /> Loading RBAC data…
            </div>
        );
    }

    if (roles.length === 0 || allPerms.length === 0) {
        return (
            <div className="flex flex-col items-center justify-center py-24 text-gray-500 dark:text-gray-400">
                <AlertCircle size={32} className="mb-3 text-amber-500 opacity-80" />
                <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-1">No RBAC Data Available</h3>
                <p className="text-sm max-w-md text-center">
                    The roles or permissions database is currently empty. Please ensure the backend migrations and seed data have been successfully applied.
                </p>
                <button onClick={fetchData} className="mt-5 flex items-center gap-2 px-4 py-2 border border-gray-300 dark:border-gray-700 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors text-sm font-medium">
                    <RefreshCw size={14} /> Retry Connection
                </button>
            </div>
        );
    }

    return (
        <div className="space-y-4 max-w-full">
            {/* ── Header ── */}
            <div className="flex items-center justify-between">
                <div>
                    <h2 className="text-xl font-bold text-gray-900 dark:text-white">Roles & Permissions</h2>
                    <p className="text-[13px] text-gray-500 dark:text-gray-400 mt-0.5">
                        Configure granular access control for each role — {allPerms.length} permissions across {Object.keys(resources).length} resources
                    </p>
                </div>
                <div className="flex gap-2">
                    <button onClick={fetchData} className="p-2.5 text-gray-500 hover:text-gray-900 dark:hover:text-white border border-gray-300 dark:border-gray-700 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors" title="Refresh">
                        <RefreshCw size={15} />
                    </button>
                    <button
                        onClick={handleSave} disabled={!dirty || saving}
                        className={`flex items-center gap-2 px-4 py-2.5 rounded-lg text-sm font-medium transition-colors ${
                            dirty ? 'bg-blue-600 hover:bg-blue-700 text-white shadow-sm' : 'bg-gray-100 text-gray-400 dark:bg-gray-800 dark:text-gray-600 cursor-not-allowed'
                        }`}
                    >
                        <Save size={14} /> {saving ? 'Saving…' : 'Save Changes'}
                    </button>
                </div>
            </div>

            {/* Toasts */}
            {feedback && (
                <div className="flex items-center gap-2 px-4 py-2.5 rounded-lg bg-emerald-50 text-emerald-600 border border-emerald-200 dark:bg-emerald-500/10 dark:text-emerald-400 dark:border-emerald-500/20 text-sm">
                    <Check size={14} /> {feedback}
                </div>
            )}
            {error && (
                <div className="flex items-center gap-2 px-4 py-2.5 rounded-lg bg-red-50 text-red-600 border border-red-200 dark:bg-red-500/10 dark:text-red-400 dark:border-red-500/20 text-sm">
                    <AlertCircle size={14} /> {error}
                </div>
            )}

            {/* ── Permission Matrix ── */}
            <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-xl overflow-hidden shadow-sm">
                <div className="overflow-x-auto">
                    <table className="w-full text-sm">
                        <thead>
                            <tr className="border-b border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900/50">
                                <th className="text-left px-5 py-3 text-[11px] font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider sticky left-0 bg-gray-50 dark:bg-gray-900/90 z-10 min-w-[220px]">
                                    Permission
                                </th>
                                {roles.map(role => {
                                    const s = ROLE_COLORS[role.name] || ROLE_COLORS.viewer;
                                    return (
                                        <th key={role.id} className="text-center px-3 py-3 min-w-[100px]">
                                            <div className="flex flex-col items-center gap-1">
                                                <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-[10px] font-bold uppercase tracking-wide border ${s.bg} ${s.text} ${s.border}`}>
                                                    <Shield size={9} /> {role.name}
                                                </span>
                                                {role.is_built_in && role.name === 'admin' && (
                                                    <span className="flex items-center gap-0.5 text-[9px] text-gray-500 dark:text-gray-400"><Lock size={8} /> Full Access</span>
                                                )}
                                            </div>
                                        </th>
                                    );
                                })}
                            </tr>
                        </thead>
                        <tbody>
                            {Object.entries(resources).map(([resource, perms]) => (
                                <Fragment key={resource}>
                                    {/* Resource group header */}
                                    <tr className="bg-gray-50/50 dark:bg-gray-800/50 border-y border-gray-200 dark:border-gray-700">
                                        <td colSpan={roles.length + 1} className="px-5 py-2 text-[10px] font-bold text-gray-700 dark:text-gray-300 uppercase tracking-[0.1em]">
                                            {resource}
                                        </td>
                                    </tr>
                                    {/* Permission rows */}
                                    {perms.map(perm => (
                                        <tr key={perm.id} className="border-b border-gray-200/50 dark:border-gray-700/50 last:border-b-0 hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors">
                                            <td className="px-5 py-2.5 sticky left-0 bg-white dark:bg-gray-800 z-10 shadow-[1px_0_0_0_#e5e7eb] dark:shadow-[1px_0_0_0_#374151]">
                                                <div className="text-gray-900 dark:text-white font-medium text-[13px]">{perm.action}</div>
                                                <div className="text-[11px] text-gray-500 dark:text-gray-400 leading-tight">{perm.description}</div>
                                            </td>
                                            {roles.map(role => {
                                                const granted = hasPermission(role, perm.id);
                                                const isAdmin = role.name === 'admin' && role.is_built_in;
                                                const modified = edits[role.id] !== undefined;

                                                return (
                                                    <td key={role.id} className="text-center px-3 py-2.5">
                                                        <button
                                                            onClick={() => toggle(role, perm.id)}
                                                            disabled={isAdmin}
                                                            className={`
                                                                w-7 h-7 rounded-md flex items-center justify-center mx-auto
                                                                transition-all duration-150 
                                                                ${granted
                                                                    ? isAdmin
                                                                        ? 'bg-emerald-50 dark:bg-emerald-500/10 text-emerald-500 dark:text-emerald-400 cursor-not-allowed opacity-80'
                                                                        : modified
                                                                            ? 'bg-amber-100 dark:bg-amber-500/20 text-amber-600 dark:text-amber-400 ring-1 ring-amber-300 dark:ring-amber-500/40 hover:bg-amber-200 dark:hover:bg-amber-500/30'
                                                                            : 'bg-emerald-100 dark:bg-emerald-500/20 text-emerald-600 dark:text-emerald-400 hover:bg-emerald-200 dark:hover:bg-emerald-500/30'
                                                                    : isAdmin
                                                                        ? 'bg-emerald-50 dark:bg-emerald-500/10 text-emerald-500 dark:text-emerald-400 cursor-not-allowed opacity-80'
                                                                        : 'bg-gray-100 dark:bg-gray-700 text-gray-400 dark:text-gray-500 hover:bg-gray-200 dark:hover:bg-gray-600'
                                                                }
                                                            `}
                                                            title={isAdmin ? 'Admin has full access' : `${granted ? 'Revoke' : 'Grant'} ${resource}:${perm.action}`}
                                                        >
                                                            {(granted || isAdmin) ? <Check size={13} strokeWidth={2.5} /> : <span className="text-[10px]">—</span>}
                                                        </button>
                                                    </td>
                                                );
                                            })}
                                        </tr>
                                    ))}
                                </Fragment>
                            ))}
                        </tbody>
                    </table>
                </div>
            </div>

            {/* ── Legend ── */}
            <div className="flex items-center gap-6 text-[11px] text-gray-500 dark:text-gray-400 px-1">
                <div className="flex items-center gap-1.5">
                    <div className="w-5 h-5 rounded-md bg-emerald-100 dark:bg-emerald-500/20 flex items-center justify-center"><Check size={10} strokeWidth={2.5} className="text-emerald-600 dark:text-emerald-400" /></div>
                    Granted
                </div>
                <div className="flex items-center gap-1.5">
                    <div className="w-5 h-5 rounded-md bg-amber-100 dark:bg-amber-500/20 flex items-center justify-center ring-1 ring-amber-300 dark:ring-amber-500/40"><Check size={10} strokeWidth={2.5} className="text-amber-600 dark:text-amber-400" /></div>
                    Modified (unsaved)
                </div>
                <div className="flex items-center gap-1.5">
                    <div className="w-5 h-5 rounded-md bg-gray-100 dark:bg-gray-700 flex items-center justify-center"><span className="text-[9px] text-gray-400 dark:text-gray-500">—</span></div>
                    Not granted
                </div>
                <div className="flex items-center gap-1.5 ml-auto">
                    <Lock size={10} /> Built-in roles show default grants; admin is fully immutable
                </div>
            </div>
        </div>
    );
}
