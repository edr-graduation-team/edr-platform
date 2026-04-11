import { useState, useEffect } from 'react';
import { User, Lock, Save, Check, AlertCircle, Eye, EyeOff } from 'lucide-react';
import { authApi, usersApi } from '../../api/client';

export default function UserProfile() {
    const currentUser = authApi.getCurrentUser();
    const [fullName, setFullName] = useState(currentUser?.full_name || '');
    const [email, setEmail] = useState(currentUser?.email || '');
    const [saving, setSaving] = useState(false);
    const [feedback, setFeedback] = useState<{ type: 'success' | 'error'; msg: string } | null>(null);

    // Password change state
    const [pwExpanded, setPwExpanded] = useState(false);
    const [oldPw, setOldPw] = useState('');
    const [newPw, setNewPw] = useState('');
    const [confirmPw, setConfirmPw] = useState('');
    const [showNew, setShowNew] = useState(false);

    useEffect(() => {
        if (feedback) {
            const t = setTimeout(() => setFeedback(null), 4000);
            return () => clearTimeout(t);
        }
    }, [feedback]);

    const handleSaveProfile = async () => {
        if (!currentUser) return;
        setSaving(true);
        try {
            const updated = await usersApi.update(currentUser.id, { full_name: fullName, email });
            localStorage.setItem('user', JSON.stringify({ ...currentUser, ...updated }));
            setFeedback({ type: 'success', msg: 'Profile updated successfully' });
        } catch (err: any) {
            setFeedback({ type: 'error', msg: err?.response?.data?.message || 'Failed to update profile' });
        } finally {
            setSaving(false);
        }
    };

    const handleChangePassword = async () => {
        if (!currentUser) return;
        if (newPw !== confirmPw) { setFeedback({ type: 'error', msg: 'New passwords do not match' }); return; }
        if (newPw.length < 12)   { setFeedback({ type: 'error', msg: 'Password must be at least 12 characters' }); return; }
        setSaving(true);
        try {
            await usersApi.changePassword(currentUser.id, oldPw, newPw);
            // Password changed successfully — force logout and redirect to login.
            // The backend has already blacklisted the current JWT, so any subsequent
            // API call would fail with 401 anyway. Clear local state preemptively.
            localStorage.removeItem('auth_token');
            localStorage.removeItem('user');
            // Brief delay so the user sees the success message before redirect
            setFeedback({ type: 'success', msg: 'Password changed — redirecting to login…' });
            setTimeout(() => {
                window.location.href = '/login';
            }, 1200);
        } catch (err: any) {
            setFeedback({ type: 'error', msg: err?.response?.data?.message || 'Failed to change password' });
        } finally {
            setSaving(false);
        }
    };

    const inputClass = 'w-full px-3.5 py-2.5 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-sm text-gray-900 dark:text-gray-100 placeholder-gray-400 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500/30 transition-colors';
    const readonlyClass = 'w-full px-3.5 py-2.5 bg-gray-50 dark:bg-gray-900 border border-gray-200 dark:border-gray-800 rounded-lg text-sm text-gray-500 dark:text-gray-400 cursor-not-allowed';
    const labelClass = 'block text-[11px] font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-1.5';

    return (
        <div className="space-y-5 max-w-3xl">
            {/* ── Feedback Toast ── */}
            {feedback && (
                <div className={`flex items-center gap-2 px-4 py-2.5 rounded-lg text-sm border transition-all ${
                    feedback.type === 'success'
                        ? 'bg-emerald-50 text-emerald-600 border-emerald-200 dark:bg-emerald-500/10 dark:text-emerald-400 dark:border-emerald-500/20'
                        : 'bg-red-50 text-red-600 border-red-200 dark:bg-red-500/10 dark:text-red-400 dark:border-red-500/20'
                }`}>
                    {feedback.type === 'success' ? <Check size={15} /> : <AlertCircle size={15} />}
                    {feedback.msg}
                </div>
            )}

            {/* ════════════════════════════════════════════════════════
                PANEL 1 — Personal Information
               ════════════════════════════════════════════════════════ */}
            <section className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-xl overflow-hidden shadow-sm">
                {/* Panel header */}
                <div className="flex items-center gap-3 px-6 py-4 border-b border-gray-200 dark:border-gray-700 bg-gray-50/50 dark:bg-gray-800/50">
                    <div className="w-9 h-9 rounded-lg bg-blue-100 text-blue-600 dark:bg-blue-500/20 dark:text-blue-400 flex items-center justify-center">
                        <User size={17} className="text-inherit" />
                    </div>
                    <div>
                        <h3 className="text-[15px] font-semibold text-gray-900 dark:text-white leading-tight">Personal Information</h3>
                        <p className="text-[12px] text-gray-500 dark:text-gray-400 leading-tight mt-0.5">Update your name and email</p>
                    </div>
                </div>

                <div className="p-6">
                    <div className="grid grid-cols-2 gap-4">
                        <div>
                            <label className={labelClass}>Username</label>
                            <input type="text" value={currentUser?.username || ''} disabled className={readonlyClass} />
                        </div>
                        <div>
                            <label className={labelClass}>Role</label>
                            <input type="text" value={(currentUser?.role || '').charAt(0).toUpperCase() + (currentUser?.role || '').slice(1)} disabled className={readonlyClass} />
                        </div>
                        <div>
                            <label className={labelClass}>Full Name</label>
                            <input type="text" value={fullName} onChange={e => setFullName(e.target.value)} className={inputClass} placeholder="Your full name" />
                        </div>
                        <div>
                            <label className={labelClass}>Email</label>
                            <input type="email" value={email} onChange={e => setEmail(e.target.value)} className={inputClass} placeholder="your@email.com" />
                        </div>
                    </div>

                    <div className="flex justify-end mt-5">
                        <button
                            onClick={handleSaveProfile}
                            disabled={saving}
                            className="flex items-center gap-2 px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white text-sm font-medium rounded-lg disabled:opacity-50 transition-colors"
                        >
                            <Save size={14} /> Save Profile
                        </button>
                    </div>
                </div>
            </section>

            {/* ════════════════════════════════════════════════════════
                PANEL 2 — Security (Password Change)
               ════════════════════════════════════════════════════════ */}
            <section className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-xl overflow-hidden shadow-sm">
                <div className="flex items-center gap-3 px-6 py-4 border-b border-gray-200 dark:border-gray-700 bg-gray-50/50 dark:bg-gray-800/50">
                    <div className="w-9 h-9 rounded-lg bg-amber-100 text-amber-600 dark:bg-amber-500/20 dark:text-amber-400 flex items-center justify-center">
                        <Lock size={17} className="text-inherit" />
                    </div>
                    <div>
                        <h3 className="text-[15px] font-semibold text-gray-900 dark:text-white leading-tight">Security</h3>
                        <p className="text-[12px] text-gray-500 dark:text-gray-400 leading-tight mt-0.5">Change your password</p>
                    </div>
                </div>

                <div className="p-6">
                    {!pwExpanded ? (
                        <button
                            onClick={() => setPwExpanded(true)}
                            className="px-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm text-gray-700 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white hover:border-blue-500/40 transition-all font-medium"
                        >
                            Change Password
                        </button>
                    ) : (
                        <div className="max-w-sm space-y-4">
                            <div>
                                <label className={labelClass}>Current Password</label>
                                <input type="password" value={oldPw} onChange={e => setOldPw(e.target.value)} className={inputClass} placeholder="Enter current password" />
                            </div>
                            <div>
                                <label className={labelClass}>New Password</label>
                                <div className="relative">
                                    <input
                                        type={showNew ? 'text' : 'password'}
                                        value={newPw}
                                        onChange={e => setNewPw(e.target.value)}
                                        className={inputClass + ' pr-10'}
                                        placeholder="Min 12 characters"
                                    />
                                    <button onClick={() => setShowNew(!showNew)} className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors">
                                        {showNew ? <EyeOff size={15} /> : <Eye size={15} />}
                                    </button>
                                </div>
                                {newPw.length > 0 && newPw.length < 12 && (
                                    <p className="text-[11px] text-red-500 dark:text-red-400 mt-1.5 font-medium">Must be at least 12 characters</p>
                                )}
                            </div>
                            <div>
                                <label className={labelClass}>Confirm New Password</label>
                                <input type="password" value={confirmPw} onChange={e => setConfirmPw(e.target.value)} className={inputClass} placeholder="Re-enter new password" />
                                {confirmPw.length > 0 && confirmPw !== newPw && (
                                    <p className="text-[11px] text-red-500 dark:text-red-400 mt-1.5 font-medium">Passwords do not match</p>
                                )}
                            </div>
                            <div className="flex gap-2 pt-2">
                                <button
                                    onClick={() => { setPwExpanded(false); setOldPw(''); setNewPw(''); setConfirmPw(''); }}
                                    className="px-4 py-2 text-sm text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white transition-colors"
                                >
                                    Cancel
                                </button>
                                <button
                                    onClick={handleChangePassword}
                                    disabled={saving || newPw.length < 12 || confirmPw !== newPw}
                                    className="flex items-center gap-2 px-4 py-2 text-sm bg-amber-500 hover:bg-amber-600 text-white rounded-lg disabled:opacity-40 font-medium transition-colors shadow-sm"
                                >
                                    <Lock size={13} /> {saving ? 'Changing...' : 'Update Password'}
                                </button>
                            </div>
                        </div>
                    )}
                </div>
            </section>
        </div>
    );
}
