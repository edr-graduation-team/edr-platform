import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import {
    Key, Plus, Copy, ShieldOff, CheckCircle, XCircle,
    Clock, AlertTriangle, RefreshCw, Hash
} from 'lucide-react';
import { enrollmentTokensApi, authApi, type EnrollmentToken } from '../api/client';
import { Modal, SkeletonTable } from '../components';

// --------------------------------------------------------------------------
// Generate Token Modal
// --------------------------------------------------------------------------
function GenerateTokenModal({
    isOpen,
    onClose,
    onGenerated,
}: {
    isOpen: boolean;
    onClose: () => void;
    onGenerated: (token: EnrollmentToken) => void;
}) {
    const [description, setDescription] = useState('');
    const [expiresInHours, setExpiresInHours] = useState<string>('');
    const [maxUses, setMaxUses] = useState<string>('');
    const [isSubmitting, setIsSubmitting] = useState(false);
    const [error, setError] = useState('');

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        setError('');
        setIsSubmitting(true);

        try {
            const data: { description: string; expires_in_hours?: number; max_uses?: number } = {
                description: description.trim() || 'Unnamed Token',
            };
            if (expiresInHours && parseInt(expiresInHours) > 0) {
                data.expires_in_hours = parseInt(expiresInHours);
            }
            if (maxUses && parseInt(maxUses) > 0) {
                data.max_uses = parseInt(maxUses);
            }

            const token = await enrollmentTokensApi.generate(data);
            onGenerated(token);
            setDescription('');
            setExpiresInHours('');
            setMaxUses('');
            onClose();
        } catch {
            setError('Failed to generate token. Please try again.');
        } finally {
            setIsSubmitting(false);
        }
    };

    return (
        <Modal isOpen={isOpen} onClose={onClose} title="Generate Enrollment Token" size="md">
            <form onSubmit={handleSubmit} className="space-y-4">
                {error && (
                    <div className="p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">
                        {error}
                    </div>
                )}

                <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                        Description / Label
                    </label>
                    <input
                        type="text"
                        value={description}
                        onChange={(e) => setDescription(e.target.value)}
                        placeholder="e.g. HR Department Deployment"
                        className="input w-full"
                        autoFocus
                    />
                    <p className="text-xs text-gray-500 mt-1">
                        A friendly name to identify this token's purpose.
                    </p>
                </div>

                <div className="grid grid-cols-2 gap-4">
                    <div>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                            Expires In (hours)
                        </label>
                        <input
                            type="number"
                            value={expiresInHours}
                            onChange={(e) => setExpiresInHours(e.target.value)}
                            placeholder="Never"
                            min="1"
                            className="input w-full"
                        />
                        <p className="text-xs text-gray-500 mt-1">Leave empty for no expiry.</p>
                    </div>
                    <div>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                            Max Uses
                        </label>
                        <input
                            type="number"
                            value={maxUses}
                            onChange={(e) => setMaxUses(e.target.value)}
                            placeholder="Unlimited"
                            min="1"
                            className="input w-full"
                        />
                        <p className="text-xs text-gray-500 mt-1">Leave empty for unlimited.</p>
                    </div>
                </div>

                <div className="flex justify-end gap-3 pt-2">
                    <button type="button" onClick={onClose} className="btn btn-secondary">
                        Cancel
                    </button>
                    <button type="submit" disabled={isSubmitting} className="btn btn-primary flex items-center gap-2">
                        {isSubmitting ? (
                            <RefreshCw className="w-4 h-4 animate-spin" />
                        ) : (
                            <Plus className="w-4 h-4" />
                        )}
                        Generate Token
                    </button>
                </div>
            </form>
        </Modal>
    );
}

// --------------------------------------------------------------------------
// Token Created Success Modal (shows once after generation to copy)
// --------------------------------------------------------------------------
function TokenCreatedModal({
    token,
    isOpen,
    onClose,
}: {
    token: EnrollmentToken | null;
    isOpen: boolean;
    onClose: () => void;
}) {
    const [copied, setCopied] = useState(false);

    const handleCopy = async () => {
        if (!token) return;
        try {
            await navigator.clipboard.writeText(token.token);
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        } catch {
            // Fallback for non-HTTPS
            const el = document.createElement('textarea');
            el.value = token.token;
            document.body.appendChild(el);
            el.select();
            document.execCommand('copy');
            document.body.removeChild(el);
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        }
    };

    if (!token) return null;

    return (
        <Modal isOpen={isOpen} onClose={onClose} title="Token Generated Successfully" size="md">
            <div className="space-y-4">
                <div className="p-4 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg">
                    <div className="flex items-center gap-2 mb-2">
                        <CheckCircle className="w-5 h-5 text-green-600" />
                        <span className="font-medium text-green-800 dark:text-green-300">Token Ready</span>
                    </div>
                    <p className="text-xs text-green-700 dark:text-green-400 mb-3">
                        Copy this token now. You can always copy it later from the token list.
                    </p>
                    <div className="flex items-center gap-2">
                        <code className="flex-1 p-2 bg-white dark:bg-gray-900 border border-green-300 dark:border-green-700 rounded font-mono text-xs break-all text-gray-900 dark:text-gray-100">
                            {token.token}
                        </code>
                        <button
                            onClick={handleCopy}
                            className={`btn ${copied ? 'btn-success' : 'btn-primary'} flex items-center gap-1 whitespace-nowrap`}
                        >
                            {copied ? <CheckCircle className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
                            {copied ? 'Copied!' : 'Copy'}
                        </button>
                    </div>
                </div>

                {token.description && (
                    <p className="text-sm text-gray-600 dark:text-gray-400">
                        <strong>Label:</strong> {token.description}
                    </p>
                )}

                <div className="flex justify-end">
                    <button onClick={onClose} className="btn btn-secondary">Close</button>
                </div>
            </div>
        </Modal>
    );
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

function isExpired(token: EnrollmentToken): boolean {
    if (!token.expires_at) return false;
    return new Date(token.expires_at) < new Date();
}

function isMaxedOut(token: EnrollmentToken): boolean {
    if (token.max_uses === null) return false;
    return token.use_count >= token.max_uses;
}

function getStatusBadge(token: EnrollmentToken): { label: string; color: string; icon: typeof CheckCircle } {
    if (!token.is_active) return { label: 'Revoked', color: 'badge-danger', icon: XCircle };
    if (isExpired(token)) return { label: 'Expired', color: 'badge-warning', icon: Clock };
    if (isMaxedOut(token)) return { label: 'Max Uses', color: 'badge-warning', icon: Hash };
    return { label: 'Active', color: 'badge-success', icon: CheckCircle };
}

// --------------------------------------------------------------------------
// Main Page
// --------------------------------------------------------------------------
export default function EnrollmentTokens() {
    const queryClient = useQueryClient();
    const [showGenerateModal, setShowGenerateModal] = useState(false);
    const [createdToken, setCreatedToken] = useState<EnrollmentToken | null>(null);
    const [copiedId, setCopiedId] = useState<string | null>(null);

    const canManage = authApi.hasRole(['admin']);

    // Fetch tokens
    const { data, isLoading, error } = useQuery({
        queryKey: ['enrollmentTokens'],
        queryFn: () => enrollmentTokensApi.list(),
    });

    // Revoke mutation
    const revokeMutation = useMutation({
        mutationFn: (id: string) => enrollmentTokensApi.revoke(id),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['enrollmentTokens'] });
        },
    });

    const tokens = data?.data || [];

    const handleCopyToken = async (token: string, id: string) => {
        try {
            await navigator.clipboard.writeText(token);
        } catch {
            const el = document.createElement('textarea');
            el.value = token;
            document.body.appendChild(el);
            el.select();
            document.execCommand('copy');
            document.body.removeChild(el);
        }
        setCopiedId(id);
        setTimeout(() => setCopiedId(null), 2000);
    };

    const handleRevoke = (id: string) => {
        if (confirm('Are you sure you want to revoke this token? Agents using it will no longer be able to enroll.')) {
            revokeMutation.mutate(id);
        }
    };

    const handleTokenGenerated = (token: EnrollmentToken) => {
        queryClient.invalidateQueries({ queryKey: ['enrollmentTokens'] });
        setCreatedToken(token);
    };

    // Stats
    const activeCount = tokens.filter(t => t.is_active && !isExpired(t) && !isMaxedOut(t)).length;
    const revokedCount = tokens.filter(t => !t.is_active).length;
    const totalUses = tokens.reduce((sum, t) => sum + t.use_count, 0);

    if (error) {
        return (
            <div className="card text-center py-12">
                <AlertTriangle className="w-12 h-12 text-red-400 mx-auto mb-4" />
                <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">
                    Failed to Load Enrollment Tokens
                </h3>
                <p className="text-gray-500">Please try again later.</p>
            </div>
        );
    }

    return (
        <div className="space-y-6">
            {/* Header */}
            <div className="flex items-center justify-between">
                <div>
                    <h1 className="text-3xl font-bold text-gray-900 dark:text-white">Enrollment Tokens</h1>
                    <p className="text-gray-500 mt-1">Manage tokens for agent zero-touch provisioning</p>
                </div>
                {canManage && (
                    <button
                        onClick={() => setShowGenerateModal(true)}
                        className="btn btn-primary flex items-center gap-2"
                    >
                        <Plus className="w-4 h-4" />
                        Generate Token
                    </button>
                )}
            </div>

            {/* Stats */}
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                <div className="card">
                    <div className="flex items-center gap-3">
                        <div className="p-2 bg-green-100 dark:bg-green-900/30 rounded-lg">
                            <Key className="w-5 h-5 text-green-600" />
                        </div>
                        <div>
                            <p className="text-2xl font-bold text-gray-900 dark:text-white">{activeCount}</p>
                            <p className="text-sm text-gray-500">Active Tokens</p>
                        </div>
                    </div>
                </div>
                <div className="card">
                    <div className="flex items-center gap-3">
                        <div className="p-2 bg-red-100 dark:bg-red-900/30 rounded-lg">
                            <ShieldOff className="w-5 h-5 text-red-600" />
                        </div>
                        <div>
                            <p className="text-2xl font-bold text-gray-900 dark:text-white">{revokedCount}</p>
                            <p className="text-sm text-gray-500">Revoked</p>
                        </div>
                    </div>
                </div>
                <div className="card">
                    <div className="flex items-center gap-3">
                        <div className="p-2 bg-blue-100 dark:bg-blue-900/30 rounded-lg">
                            <Hash className="w-5 h-5 text-blue-600" />
                        </div>
                        <div>
                            <p className="text-2xl font-bold text-gray-900 dark:text-white">{totalUses}</p>
                            <p className="text-sm text-gray-500">Total Enrollments</p>
                        </div>
                    </div>
                </div>
            </div>

            {/* Tokens Table */}
            <div className="card overflow-hidden p-0">
                {isLoading ? (
                    <div className="p-4">
                        <SkeletonTable rows={5} columns={7} />
                    </div>
                ) : tokens.length === 0 ? (
                    <div className="text-center py-12">
                        <Key className="w-12 h-12 text-gray-400 mx-auto mb-4" />
                        <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">
                            No Enrollment Tokens
                        </h3>
                        <p className="text-gray-500 mb-4">
                            Generate a token to start enrolling agents automatically.
                        </p>
                        {canManage && (
                            <button
                                onClick={() => setShowGenerateModal(true)}
                                className="btn btn-primary"
                            >
                                Generate First Token
                            </button>
                        )}
                    </div>
                ) : (
                    <div className="overflow-x-auto">
                        <table className="table">
                            <thead className="bg-gray-50 dark:bg-gray-800">
                                <tr>
                                    <th>Status</th>
                                    <th>Description</th>
                                    <th>Token</th>
                                    <th>Uses</th>
                                    <th>Created</th>
                                    <th>Expires</th>
                                    <th>Actions</th>
                                </tr>
                            </thead>
                            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                                {tokens.map((token) => {
                                    const status = getStatusBadge(token);
                                    const StatusIcon = status.icon;

                                    return (
                                        <tr key={token.id} className="hover:bg-gray-50 dark:hover:bg-gray-800">
                                            <td>
                                                <span className={`badge ${status.color} flex items-center gap-1 w-fit`}>
                                                    <StatusIcon className="w-3 h-3" />
                                                    {status.label}
                                                </span>
                                            </td>
                                            <td>
                                                <p className="font-medium text-gray-900 dark:text-white text-sm">
                                                    {token.description || 'No description'}
                                                </p>
                                                <p className="text-xs text-gray-500">by {token.created_by}</p>
                                            </td>
                                            <td>
                                                <code className="text-xs font-mono text-gray-600 dark:text-gray-400">
                                                    {token.token.slice(0, 16)}...
                                                </code>
                                            </td>
                                            <td>
                                                <span className="text-sm text-gray-900 dark:text-white">
                                                    {token.use_count}
                                                    {token.max_uses !== null && (
                                                        <span className="text-gray-500"> / {token.max_uses}</span>
                                                    )}
                                                </span>
                                            </td>
                                            <td>
                                                <div className="text-sm text-gray-900 dark:text-white">
                                                    {new Date(token.created_at).toLocaleDateString()}
                                                </div>
                                                <div className="text-xs text-gray-500">
                                                    {new Date(token.created_at).toLocaleTimeString()}
                                                </div>
                                            </td>
                                            <td>
                                                {token.expires_at ? (
                                                    <div>
                                                        <div className={`text-sm ${isExpired(token) ? 'text-red-600' : 'text-gray-900 dark:text-white'}`}>
                                                            {new Date(token.expires_at).toLocaleDateString()}
                                                        </div>
                                                        <div className="text-xs text-gray-500">
                                                            {new Date(token.expires_at).toLocaleTimeString()}
                                                        </div>
                                                    </div>
                                                ) : (
                                                    <span className="text-sm text-gray-500">Never</span>
                                                )}
                                            </td>
                                            <td>
                                                <div className="flex items-center gap-1">
                                                    <button
                                                        onClick={() => handleCopyToken(token.token, token.id)}
                                                        className="p-1.5 text-gray-500 hover:text-primary-600 hover:bg-gray-100 dark:hover:bg-gray-700 rounded"
                                                        title="Copy Token"
                                                    >
                                                        {copiedId === token.id ? (
                                                            <CheckCircle className="w-4 h-4 text-green-600" />
                                                        ) : (
                                                            <Copy className="w-4 h-4" />
                                                        )}
                                                    </button>
                                                    {canManage && token.is_active && (
                                                        <button
                                                            onClick={() => handleRevoke(token.id)}
                                                            disabled={revokeMutation.isPending}
                                                            className="p-1.5 text-gray-500 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20 rounded disabled:opacity-50"
                                                            title="Revoke Token"
                                                        >
                                                            <ShieldOff className="w-4 h-4" />
                                                        </button>
                                                    )}
                                                </div>
                                            </td>
                                        </tr>
                                    );
                                })}
                            </tbody>
                        </table>
                    </div>
                )}

                {/* Footer */}
                {tokens.length > 0 && (
                    <div className="px-4 py-3 bg-gray-50 dark:bg-gray-800 border-t border-gray-200 dark:border-gray-700 text-sm text-gray-500">
                        Showing {tokens.length} tokens
                    </div>
                )}
            </div>

            {/* Modals */}
            <GenerateTokenModal
                isOpen={showGenerateModal}
                onClose={() => setShowGenerateModal(false)}
                onGenerated={handleTokenGenerated}
            />
            <TokenCreatedModal
                token={createdToken}
                isOpen={!!createdToken}
                onClose={() => setCreatedToken(null)}
            />
        </div>
    );
}
