import { useEffect, useRef, useState, useCallback } from 'react';
import { Mail, ShieldAlert, Loader2, X, RefreshCw } from 'lucide-react';
import {
    setApprovalTokenResolver,
    commandApprovalApi,
    type ApprovalContext,
} from '../api/client';

// ────────────────────────────────────────────────────────────────────────────
// Types
// ────────────────────────────────────────────────────────────────────────────

type Stage = 'issuing' | 'code_entry' | 'verifying' | 'error';

interface ModalState {
    open: boolean;
    ctx: ApprovalContext;
    stage: Stage;
    approvalId: string;
    maskedEmail: string;
    errorMsg: string;
    resolve: ((token: string) => void) | null;
    reject: ((reason?: unknown) => void) | null;
}

const INITIAL: ModalState = {
    open: false,
    ctx: {},
    stage: 'issuing',
    approvalId: '',
    maskedEmail: '',
    errorMsg: '',
    resolve: null,
    reject: null,
};

// ────────────────────────────────────────────────────────────────────────────
// Provider
// ────────────────────────────────────────────────────────────────────────────

/**
 * Mount once at the app root (inside QueryClientProvider + ToastProvider).
 * Registers an approval-token resolver with the axios interceptor in
 * client.ts. When the server returns 403 APPROVAL_REQUIRED the interceptor
 * calls the resolver, which opens the modal, walks the operator through the
 * OTP flow, and resolves/rejects the promise so the interceptor can replay
 * (or surface an error to) the original request.
 */
export function CommandApprovalProvider({ children }: { children: React.ReactNode }) {
    const [state, setState] = useState<ModalState>(INITIAL);
    const codeRef = useRef<HTMLInputElement>(null);

    // ── Register / unregister with the axios interceptor ─────────────────
    useEffect(() => {
        setApprovalTokenResolver(async (ctx: ApprovalContext): Promise<string> => {
            return new Promise<string>((resolve, reject) => {
                setState({
                    open: true,
                    ctx,
                    stage: 'issuing',
                    approvalId: '',
                    maskedEmail: '',
                    errorMsg: ctx.invalidPrevious
                        ? 'The previous code was rejected. A new code has been sent.'
                        : '',
                    resolve,
                    reject,
                });
            });
        });
        return () => setApprovalTokenResolver(null);
    }, []);

    // ── Auto-issue OTP when modal opens (stage = issuing) ────────────────
    useEffect(() => {
        if (!state.open || state.stage !== 'issuing') return;
        let cancelled = false;

        commandApprovalApi.issue({
            agent_id: state.ctx.agentId,
            command_type: state.ctx.commandType,
            summary: state.ctx.summary,
        }).then((data) => {
            if (cancelled) return;
            setState((s) => ({
                ...s,
                stage: 'code_entry',
                approvalId: data.approval_id,
                maskedEmail: data.masked_email,
                // Preserve invalidPrevious message if it was set.
            }));
            // Focus the code input on next tick.
            setTimeout(() => codeRef.current?.focus(), 60);
        }).catch((err) => {
            if (cancelled) return;
            const msg: string =
                err?.response?.data?.message ||
                err?.message ||
                'Failed to request approval code. Check SMTP configuration.';
            // If the server says approval is disabled (503 APPROVAL_DISABLED),
            // silently proceed by resolving with an empty token — the backend
            // gate will be a no-op.
            if (err?.response?.status === 503 &&
                err?.response?.data?.error_code === 'APPROVAL_DISABLED') {
                state.resolve?.('__disabled__');
                setState(INITIAL);
                return;
            }
            setState((s) => ({ ...s, stage: 'error', errorMsg: msg }));
        });

        return () => { cancelled = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [state.open, state.stage === 'issuing']);

    // ── Handlers ─────────────────────────────────────────────────────────
    const [code, setCode] = useState('');

    const handleCancel = useCallback(() => {
        state.reject?.(new Error('cancelled'));
        setState(INITIAL);
        setCode('');
    }, [state]);

    const handleVerify = useCallback(async () => {
        if (code.length !== 6 || !state.approvalId) return;
        setState((s) => ({ ...s, stage: 'verifying' }));
        try {
            const data = await commandApprovalApi.verify({
                approval_id: state.approvalId,
                code,
            });
            state.resolve?.(data.approval_token);
            setState(INITIAL);
            setCode('');
        } catch (err: any) {
            const msg: string =
                err?.response?.data?.message || err?.message || 'Invalid code';
            // Re-enter code on wrong OTP
            setState((s) => ({ ...s, stage: 'code_entry', errorMsg: msg }));
            setCode('');
            setTimeout(() => codeRef.current?.focus(), 60);
        }
    }, [code, state]);

    const handleResend = useCallback(() => {
        setCode('');
        setState((s) => ({ ...s, stage: 'issuing', approvalId: '', maskedEmail: '', errorMsg: '' }));
    }, []);

    const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
        if (e.key === 'Enter') handleVerify();
        if (e.key === 'Escape') handleCancel();
    }, [handleVerify, handleCancel]);

    // ── Render ────────────────────────────────────────────────────────────
    const cmdLabel = state.ctx.commandType
        ? state.ctx.commandType.replace(/_/g, ' ')
        : 'command';

    return (
        <>
            {children}

            {state.open && (
                <div className="fixed inset-0 z-[9999] flex items-center justify-center p-4">
                    {/* Backdrop */}
                    <div
                        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
                        onClick={handleCancel}
                    />

                    {/* Card */}
                    <div
                        className="relative z-10 w-full max-w-md rounded-2xl border border-amber-500/30 bg-slate-900 shadow-2xl"
                        onKeyDown={handleKeyDown}
                        role="dialog"
                        aria-modal="true"
                        aria-labelledby="approval-title"
                    >
                        {/* Header */}
                        <div className="flex items-start justify-between gap-4 border-b border-slate-700/60 px-6 py-4">
                            <div className="flex items-center gap-3">
                                <div className="rounded-lg bg-amber-500/10 p-2">
                                    <ShieldAlert className="h-5 w-5 text-amber-400" />
                                </div>
                                <div>
                                    <h2 id="approval-title" className="text-sm font-bold text-slate-100">
                                        Command Approval Required
                                    </h2>
                                    <p className="mt-0.5 text-xs text-slate-400">
                                        Action:{' '}
                                        <span className="font-semibold text-amber-300">{cmdLabel}</span>
                                        {state.ctx.agentId && (
                                            <>
                                                {' '}on{' '}
                                                <span className="font-mono text-slate-300">
                                                    {state.ctx.agentId.slice(0, 8)}…
                                                </span>
                                            </>
                                        )}
                                    </p>
                                </div>
                            </div>
                            <button
                                type="button"
                                onClick={handleCancel}
                                className="rounded p-1 text-slate-400 hover:text-slate-100 transition-colors"
                                aria-label="Cancel"
                            >
                                <X className="h-4 w-4" />
                            </button>
                        </div>

                        {/* Body */}
                        <div className="px-6 py-5 space-y-5">

                            {/* ISSUING — spinner */}
                            {state.stage === 'issuing' && (
                                <div className="flex flex-col items-center gap-3 py-4">
                                    <Loader2 className="h-8 w-8 animate-spin text-amber-400" />
                                    <p className="text-sm text-slate-300">Sending approval code…</p>
                                </div>
                            )}

                            {/* CODE ENTRY */}
                            {(state.stage === 'code_entry' || state.stage === 'verifying') && (
                                <>
                                    <div className="flex items-start gap-3 rounded-lg bg-amber-500/10 border border-amber-500/20 px-4 py-3">
                                        <Mail className="mt-0.5 h-4 w-4 shrink-0 text-amber-400" />
                                        <p className="text-xs text-amber-200">
                                            A 6-digit approval code was sent to{' '}
                                            <strong>{state.maskedEmail}</strong>.
                                            Enter it below to authorise the command.
                                        </p>
                                    </div>

                                    {state.errorMsg && (
                                        <p className="rounded-md bg-red-500/10 border border-red-500/20 px-3 py-2 text-xs text-red-400">
                                            {state.errorMsg}
                                        </p>
                                    )}

                                    <div className="space-y-1.5">
                                        <label className="block text-xs font-semibold uppercase tracking-wide text-slate-400">
                                            Approval Code
                                        </label>
                                        <input
                                            ref={codeRef}
                                            type="text"
                                            inputMode="numeric"
                                            pattern="[0-9]{6}"
                                            maxLength={6}
                                            value={code}
                                            onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                                            disabled={state.stage === 'verifying'}
                                            placeholder="000000"
                                            className="w-full rounded-lg border border-slate-600 bg-slate-800 px-4 py-2.5 text-center font-mono text-xl tracking-[.5em] text-slate-100 placeholder-slate-600 focus:border-amber-400 focus:outline-none focus:ring-1 focus:ring-amber-400 disabled:opacity-50"
                                        />
                                    </div>

                                    <div className="flex items-center gap-3">
                                        <button
                                            type="button"
                                            onClick={handleVerify}
                                            disabled={code.length !== 6 || state.stage === 'verifying'}
                                            className="flex-1 rounded-lg bg-amber-500 px-4 py-2.5 text-sm font-bold text-slate-900 hover:bg-amber-400 disabled:opacity-40 disabled:cursor-not-allowed transition-colors flex items-center justify-center gap-2"
                                        >
                                            {state.stage === 'verifying' ? (
                                                <><Loader2 className="h-4 w-4 animate-spin" /> Verifying…</>
                                            ) : (
                                                'Confirm & Execute'
                                            )}
                                        </button>
                                        <button
                                            type="button"
                                            onClick={handleResend}
                                            disabled={state.stage === 'verifying'}
                                            title="Resend code"
                                            className="rounded-lg border border-slate-600 bg-slate-800 px-3 py-2.5 text-slate-400 hover:text-slate-100 hover:border-slate-500 disabled:opacity-40 transition-colors"
                                        >
                                            <RefreshCw className="h-4 w-4" />
                                        </button>
                                    </div>
                                </>
                            )}

                            {/* ERROR */}
                            {state.stage === 'error' && (
                                <>
                                    <div className="rounded-lg bg-red-500/10 border border-red-500/20 px-4 py-3">
                                        <p className="text-sm text-red-400 font-semibold">Failed to send approval code</p>
                                        <p className="mt-1 text-xs text-red-300">{state.errorMsg}</p>
                                    </div>
                                    <div className="flex gap-3">
                                        <button
                                            type="button"
                                            onClick={handleResend}
                                            className="flex-1 rounded-lg bg-slate-700 px-4 py-2.5 text-sm font-semibold text-slate-100 hover:bg-slate-600 transition-colors"
                                        >
                                            Retry
                                        </button>
                                        <button
                                            type="button"
                                            onClick={handleCancel}
                                            className="flex-1 rounded-lg border border-slate-600 px-4 py-2.5 text-sm text-slate-300 hover:bg-slate-800 transition-colors"
                                        >
                                            Cancel
                                        </button>
                                    </div>
                                </>
                            )}
                        </div>

                        {/* Footer hint */}
                        <div className="border-t border-slate-700/60 px-6 py-3">
                            <p className="text-[11px] text-slate-500">
                                This command requires out-of-band approval. Automated playbooks are not affected.
                            </p>
                        </div>
                    </div>
                </div>
            )}
        </>
    );
}
