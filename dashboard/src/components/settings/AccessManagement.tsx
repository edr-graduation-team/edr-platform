import { useState, useEffect, useCallback } from 'react';
import {
    Search, Plus, Edit3, Trash2, Lock, Unlock, RefreshCw,
    X, AlertCircle, Check, UserPlus, ChevronDown,
} from 'lucide-react';
import { usersApi, type User } from '../../api/client';
import { ROLE_COLORS, STATUS_STYLES } from './types';

const ROLES = ['admin', 'security', 'analyst', 'operations', 'viewer'] as const;

// ─── Role Badge ─────────────────────────────────────────────────────────────
function RoleBadge({ role }: { role: string }) {
    const s = ROLE_COLORS[role] || ROLE_COLORS.viewer;
    return (
        <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md text-[11px] font-semibold uppercase tracking-wide border ${s.bg} ${s.text} ${s.border}`}>
            <span className={`w-1.5 h-1.5 rounded-full ${s.dot}`} />
            {role}
        </span>
    );
}

function StatusBadge({ status }: { status: string }) {
    const s = STATUS_STYLES[status] || STATUS_STYLES.inactive;
    return (
        <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md text-[11px] font-medium ${s.bg} ${s.text}`}>
            <span className={`w-1.5 h-1.5 rounded-full ${s.dot}`} />
            {status}
        </span>
    );
}

// ─── Main Component ─────────────────────────────────────────────────────────
export default function AccessManagement() {
    const [users, setUsers] = useState<User[]>([]);
    const [total, setTotal] = useState(0);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const [search, setSearch] = useState('');
    const [roleFilter, setRoleFilter] = useState('');
    const [statusFilter, setStatusFilter] = useState('');
    const [feedback, setFeedback] = useState('');

    // Modals
    const [showAddModal, setShowAddModal] = useState(false);
    const [editUser, setEditUser] = useState<User | null>(null);
    const [saving, setSaving] = useState(false);

    // Create form
    const [form, setForm] = useState({ username: '', email: '', password: '', full_name: '', role: 'analyst' });

    // ── Data fetching ──
    const fetchUsers = useCallback(async () => {
        setLoading(true);
        setError('');
        try {
            const params: Record<string, string> = {};
            if (search.trim()) params.search = search.trim();
            if (roleFilter) params.role = roleFilter;
            if (statusFilter) params.status = statusFilter;
            const res = await usersApi.list(params);
            setUsers(res.data || []);
            setTotal(res.pagination?.total ?? (res.data?.length || 0));
        } catch (err: any) {
            setError(err?.response?.data?.message || 'Failed to load users');
        } finally {
            setLoading(false);
        }
    }, [search, roleFilter, statusFilter]);

    useEffect(() => {
        const debounce = setTimeout(fetchUsers, 300);
        return () => clearTimeout(debounce);
    }, [fetchUsers]);

    useEffect(() => {
        if (feedback) { const t = setTimeout(() => setFeedback(''), 3500); return () => clearTimeout(t); }
    }, [feedback]);

    // ── CRUD ──
    const handleCreate = async () => {
        if (!form.username || !form.email || !form.password || !form.role) {
            setError('All fields are required'); return;
        }
        setSaving(true);
        try {
            await usersApi.create(form);
            setShowAddModal(false);
            setForm({ username: '', email: '', password: '', full_name: '', role: 'analyst' });
            setFeedback('User created successfully');
            fetchUsers();
        } catch (err: any) {
            setError(err?.response?.data?.message || 'Failed to create user');
        } finally {
            setSaving(false);
        }
    };

    const handleUpdate = async () => {
        if (!editUser) return;
        setSaving(true);
        try {
            await usersApi.update(editUser.id, {
                email: editUser.email, full_name: editUser.full_name,
                role: editUser.role, status: editUser.status,
            });
            setEditUser(null);
            setFeedback('User updated');
            fetchUsers();
        } catch (err: any) {
            setError(err?.response?.data?.message || 'Failed to update user');
        } finally {
            setSaving(false);
        }
    };

    const handleDelete = async (id: string, name: string) => {
        if (!confirm(`Deactivate "${name}"? They will no longer be able to log in.`)) return;
        try {
            await usersApi.delete(id);
            setFeedback('User deactivated');
            fetchUsers();
        } catch (err: any) {
            setError(err?.response?.data?.message || 'Failed');
        }
    };

    const handleToggleStatus = async (user: User) => {
        const next = user.status === 'active' ? 'inactive' : 'active';
        try {
            await usersApi.update(user.id, { status: next });
            setFeedback(`User ${next === 'active' ? 'activated' : 'deactivated'}`);
            fetchUsers();
        } catch (err: any) {
            setError(err?.response?.data?.message || 'Failed');
        }
    };

    // ── Shared styles ──
    const inputClass = 'w-full px-3.5 py-2.5 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-sm text-gray-900 dark:text-gray-100 placeholder-gray-400 dark:placeholder-gray-500 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500/30 transition-colors';
    const labelClass = 'block text-[11px] font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-1.5';

    return (
        <div className="space-y-4 max-w-7xl">
            {/* ── Header ── */}
            <div className="flex items-center justify-between">
                <div>
                    <h2 className="text-xl font-bold text-gray-900 dark:text-white">User Management</h2>
                    <p className="text-[13px] text-gray-500 dark:text-gray-400 mt-0.5">
                        {total} user{total !== 1 ? 's' : ''} registered
                    </p>
                </div>
                <button
                    onClick={() => setShowAddModal(true)}
                    className="flex items-center gap-2 px-4 py-2.5 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors text-sm font-medium"
                >
                    <UserPlus size={15} /> Add User
                </button>
            </div>

            {/* ── Toasts ── */}
            {feedback && (
                <div className="flex items-center gap-2 px-4 py-2.5 rounded-lg bg-emerald-50 text-emerald-600 border border-emerald-200 dark:bg-emerald-500/10 dark:text-emerald-400 dark:border-emerald-500/20 text-sm">
                    <Check size={14} /> {feedback}
                </div>
            )}
            {error && (
                <div className="flex items-center gap-2 px-4 py-2.5 rounded-lg bg-red-50 text-red-600 border border-red-200 dark:bg-red-500/10 dark:text-red-400 dark:border-red-500/20 text-sm">
                    <AlertCircle size={14} /> {error}
                    <button onClick={() => setError('')} className="ml-auto text-red-500/70 hover:text-red-600 dark:text-red-400/60 dark:hover:text-red-400"><X size={14} /></button>
                </div>
            )}

            {/* ── Filters ── */}
            <div className="flex gap-3">
                <div className="relative flex-1">
                    <Search size={15} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                    <input
                        type="text" placeholder="Search by name, email, or username…"
                        value={search} onChange={e => setSearch(e.target.value)}
                        className={inputClass + ' pl-9'}
                    />
                </div>
                <div className="relative">
                    <select value={roleFilter} onChange={e => setRoleFilter(e.target.value)} className={inputClass + ' pr-8 appearance-none min-w-[130px]'}>
                        <option value="">All Roles</option>
                        {ROLES.map(r => <option key={r} value={r}>{r.charAt(0).toUpperCase() + r.slice(1)}</option>)}
                    </select>
                    <ChevronDown size={14} className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-500 pointer-events-none" />
                </div>
                <div className="relative">
                    <select value={statusFilter} onChange={e => setStatusFilter(e.target.value)} className={inputClass + ' pr-8 appearance-none min-w-[130px]'}>
                        <option value="">All Status</option>
                        <option value="active">Active</option>
                        <option value="inactive">Inactive</option>
                        <option value="locked">Locked</option>
                    </select>
                    <ChevronDown size={14} className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-500 pointer-events-none" />
                </div>
                <button onClick={fetchUsers} className="p-2.5 text-gray-500 hover:text-gray-900 dark:hover:text-white border border-gray-300 dark:border-gray-700 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors" title="Refresh">
                    <RefreshCw size={15} className={loading ? 'animate-spin' : ''} />
                </button>
            </div>

            {/* ── Table ── */}
            <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-xl overflow-hidden shadow-sm">
                <table className="w-full text-sm">
                    <thead>
                        <tr className="border-b border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900/50">
                            {['User', 'Role', 'Status', 'Last Login', 'Actions'].map((h, i) => (
                                <th key={h} className={`px-5 py-3 text-[11px] font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider ${i === 4 ? 'text-right' : 'text-left'}`}>
                                    {h}
                                </th>
                            ))}
                        </tr>
                    </thead>
                    <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                        {loading ? (
                            <tr><td colSpan={5} className="text-center py-16 text-gray-500 dark:text-gray-400"><RefreshCw size={18} className="inline animate-spin mr-2" />Loading users…</td></tr>
                        ) : users.length === 0 ? (
                            <tr><td colSpan={5} className="text-center py-16 text-gray-500 dark:text-gray-400">No users found</td></tr>
                        ) : users.map(user => (
                            <tr key={user.id} className="hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors">
                                <td className="px-5 py-3.5">
                                    <div className="flex items-center gap-3">
                                        <div className="w-9 h-9 rounded-lg bg-blue-100 dark:bg-blue-500/15 flex items-center justify-center text-blue-700 dark:text-blue-400 font-bold text-xs flex-shrink-0">
                                            {(user.full_name || user.username).charAt(0).toUpperCase()}
                                        </div>
                                        <div className="min-w-0">
                                            <div className="font-medium text-gray-900 dark:text-white truncate">{user.full_name || user.username}</div>
                                            <div className="text-[12px] text-gray-500 dark:text-gray-400 truncate">{user.email || user.username}</div>
                                        </div>
                                    </div>
                                </td>
                                <td className="px-5 py-3.5"><RoleBadge role={user.role} /></td>
                                <td className="px-5 py-3.5"><StatusBadge status={user.status} /></td>
                                <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400 text-[13px]">
                                    {user.last_login && !user.last_login.startsWith('0001')
                                        ? new Date(user.last_login).toLocaleDateString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
                                        : <span className="italic opacity-80">— Never —</span>}
                                </td>
                                <td className="px-5 py-3.5">
                                    <div className="flex items-center justify-end gap-1">
                                        <button onClick={() => setEditUser({ ...user })} className="p-2 text-gray-400 hover:text-blue-600 hover:bg-blue-50 dark:hover:bg-blue-500/10 dark:hover:text-blue-400 rounded-lg transition-all" title="Edit"><Edit3 size={15} /></button>
                                        <button onClick={() => handleToggleStatus(user)} className="p-2 text-gray-400 hover:text-amber-600 hover:bg-amber-50 dark:hover:bg-amber-500/10 dark:hover:text-amber-400 rounded-lg transition-all" title={user.status === 'active' ? 'Deactivate' : 'Activate'}>
                                            {user.status === 'active' ? <Lock size={15} /> : <Unlock size={15} />}
                                        </button>
                                        <button onClick={() => handleDelete(user.id, user.full_name || user.username)} className="p-2 text-gray-400 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-500/10 dark:hover:text-red-400 rounded-lg transition-all" title="Delete"><Trash2 size={15} /></button>
                                    </div>
                                </td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>

            {/* ═══ Add User Modal ═══ */}
            {showAddModal && (
                <div className="fixed inset-0 bg-black/60 backdrop-blur-sm z-[100] flex items-center justify-center p-4 transition-opacity" onClick={() => setShowAddModal(false)}>
                    <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-xl w-full max-w-md shadow-2xl" onClick={e => e.stopPropagation()}>
                        <div className="flex items-center justify-between px-6 py-4 border-b border-gray-200 dark:border-gray-700">
                            <div className="flex items-center gap-2.5">
                                <div className="w-8 h-8 rounded-lg bg-blue-100 dark:bg-blue-500/10 flex items-center justify-center">
                                    <Plus size={16} className="text-blue-600 dark:text-blue-400" />
                                </div>
                                <h3 className="text-[15px] font-semibold text-gray-900 dark:text-white">Add New User</h3>
                            </div>
                            <button onClick={() => setShowAddModal(false)} className="text-gray-400 hover:text-gray-900 dark:hover:text-white p-1"><X size={18} /></button>
                        </div>
                        <div className="p-6 space-y-4">
                            {[
                                { key: 'full_name', label: 'Full Name', type: 'text', placeholder: 'John Doe' },
                                { key: 'username', label: 'Username', type: 'text', placeholder: 'johndoe' },
                                { key: 'email', label: 'Email', type: 'email', placeholder: 'john@edr.local' },
                                { key: 'password', label: 'Password', type: 'password', placeholder: 'Min 12 characters' },
                            ].map(f => (
                                <div key={f.key}>
                                    <label className={labelClass}>{f.label}</label>
                                    <input type={f.type} value={(form as any)[f.key]} onChange={e => setForm(p => ({ ...p, [f.key]: e.target.value }))} className={inputClass} placeholder={f.placeholder} />
                                </div>
                            ))}
                            <div>
                                <label className={labelClass}>Role</label>
                                <div className="relative">
                                    <select value={form.role} onChange={e => setForm(p => ({ ...p, role: e.target.value }))} className={inputClass + ' appearance-none pr-8'}>
                                        {ROLES.map(r => <option key={r} value={r}>{r.charAt(0).toUpperCase() + r.slice(1)}</option>)}
                                    </select>
                                    <ChevronDown size={14} className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 pointer-events-none" />
                                </div>
                            </div>
                        </div>
                        <div className="flex justify-end gap-2 px-6 py-4 border-t border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900/50 rounded-b-xl">
                            <button onClick={() => setShowAddModal(false)} className="px-4 py-2 text-sm text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white transition-colors">Cancel</button>
                            <button onClick={handleCreate} disabled={saving} className="flex items-center gap-2 px-5 py-2 text-sm bg-blue-600 hover:bg-blue-700 text-white rounded-lg disabled:opacity-50 font-medium transition-colors">
                                {saving ? 'Creating…' : 'Create User'}
                            </button>
                        </div>
                    </div>
                </div>
            )}

            {/* ═══ Edit User Modal ═══ */}
            {editUser && (
                <div className="fixed inset-0 bg-black/60 backdrop-blur-sm z-[100] flex items-center justify-center p-4 transition-opacity" onClick={() => setEditUser(null)}>
                    <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-xl w-full max-w-md shadow-2xl" onClick={e => e.stopPropagation()}>
                        <div className="flex items-center justify-between px-6 py-4 border-b border-gray-200 dark:border-gray-700">
                            <div className="flex items-center gap-2.5">
                                <div className="w-8 h-8 rounded-lg bg-amber-100 dark:bg-amber-500/10 flex items-center justify-center">
                                    <Edit3 size={15} className="text-amber-600 dark:text-amber-400" />
                                </div>
                                <h3 className="text-[15px] font-semibold text-gray-900 dark:text-white">Edit User: <span className="text-blue-600 dark:text-blue-400">{editUser.username}</span></h3>
                            </div>
                            <button onClick={() => setEditUser(null)} className="text-gray-400 hover:text-gray-900 dark:hover:text-white p-1"><X size={18} /></button>
                        </div>
                        <div className="p-6 space-y-4">
                            <div>
                                <label className={labelClass}>Full Name</label>
                                <input type="text" value={editUser.full_name || ''} onChange={e => setEditUser(p => p ? { ...p, full_name: e.target.value } : null)} className={inputClass} />
                            </div>
                            <div>
                                <label className={labelClass}>Email</label>
                                <input type="email" value={editUser.email || ''} onChange={e => setEditUser(p => p ? { ...p, email: e.target.value } : null)} className={inputClass} />
                            </div>
                            <div>
                                <label className={labelClass}>Role</label>
                                <div className="relative">
                                    <select value={editUser.role} onChange={e => setEditUser(p => p ? { ...p, role: e.target.value as User['role'] } : null)} className={inputClass + ' appearance-none pr-8'}>
                                        {ROLES.map(r => <option key={r} value={r}>{r.charAt(0).toUpperCase() + r.slice(1)}</option>)}
                                    </select>
                                    <ChevronDown size={14} className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 pointer-events-none" />
                                </div>
                            </div>
                            <div>
                                <label className={labelClass}>Status</label>
                                <div className="relative">
                                    <select value={editUser.status} onChange={e => setEditUser(p => p ? { ...p, status: e.target.value as User['status'] } : null)} className={inputClass + ' appearance-none pr-8'}>
                                        <option value="active">Active</option>
                                        <option value="inactive">Inactive</option>
                                        <option value="locked">Locked</option>
                                    </select>
                                    <ChevronDown size={14} className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 pointer-events-none" />
                                </div>
                            </div>
                        </div>
                        <div className="flex justify-end gap-2 px-6 py-4 border-t border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900/50 rounded-b-xl">
                            <button onClick={() => setEditUser(null)} className="px-4 py-2 text-sm text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white transition-colors">Cancel</button>
                            <button onClick={handleUpdate} disabled={saving} className="flex items-center gap-2 px-5 py-2 text-sm bg-blue-600 hover:bg-blue-700 text-white rounded-lg disabled:opacity-50 font-medium transition-colors">
                                {saving ? 'Saving…' : 'Save Changes'}
                            </button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}
