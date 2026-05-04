// CommandApprovalProvider — global OTP-gate modal for manual endpoint commands.
//
// Mounts at the App root (inside ToastProvider). When the server rejects a
// manual command with APPROVAL_REQUIRED, the axios interceptor in client.ts
// calls the resolver registered here, which opens a two-step modal:
//
//   1. Issue   → sends POST /commands/approval → user receives a 6-digit code
//   2. Verify  → sends POST /commands/approval/verify → returns approval_token
//
// The resolver Promise resolves with the token so the interceptor replays the
// original request with X-Approval-Token.  If the user dismisses the modal,
// the Promise rejects and the original error propagates to the caller.

import {
    createContext,
    useCallback,
    useContext,
    useEffect,
    useRef,
    useState,
    type ReactNode,
} from 'react';
import ReactDOM from 'react-dom';
import { ShieldCheck, X, Mail, AlertTriangle, Loader2, ArrowLeft, KeyRound } from 'lucide-react';
import {
    commandApprovalApi,
    setApprovalTokenResolver,
    type ApprovalContext,
} from '../api/client';

// ─── Context (for future imperative use if needed) ───────────────────────────

interface CommandApprovalContextValue {
    /** True when the modal is visible. */
    isOpen: boolean;
}

const CommandApprovalContext = createContext<CommandApprovalContextValue>({ isOpen: false });

export const useCommandApproval = () => useContext(CommandApprovalContext);

// ─── Internal types ──────────────────────────────────────────────────────────

type Phase = 'issue' | 'code';

interface PendingRequest {
    ctx: ApprovalContext;
    resolve: (token: string) => void;
    reject: (reason?: unknown) => void;
}

// ─── Provider ────────────────────────────────────────────────────────────────

export function CommandApprovalProvider({ children }: { children: ReactNode }) {
    const [isOpen, setIsOpen] = useState(false);
    const [phase, setPhase] = useState<Phase>('issue');
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');

    // Data from the Issue step
    const [approvalId, setApprovalId] = useState('');
    const [maskedEmail, setMaskedEmail] = useState('');
    const [expiresAt, setExpiresAt] = useState('');

    // OTP code
    const [code, setCode] = useState('');
    const codeInputRef = useRef<HTMLInputElement | null>(null);

    // The pending request that opened this modal. Only one at a time.
    const pendingRef = useRef<PendingRequest | null>(null);

    // Focus the code input whenever we switch to the code phase.
    useEffect(() => {
        if (phase === 'code' && codeInputRef.current) {
            setTimeout(() => codeInputRef.current?.focus(), 80);
        }
    }, [phase]);

    // ── Resolver callback registered with the axios interceptor ──────────

    const resolverFn = useCallback(
        (ctx: ApprovalContext): Promise<string> => {
            return new Promise<string>((resolve, reject) => {
                pendingRef.current = { ctx, resolve, reject };
                // Reset state for a fresh flow
                setPhase('issue');
                setLoading(false);
                setError(ctx.invalidPrevious ? 'The previous approval code was rejected. Please request a new one.' : '');
                setApprovalId('');
                setMaskedEmail('');
                setExpiresAt('');
                setCode('');
                setIsOpen(true);
            });
        },
        [],
    );

    // Register/deregister with the axios module.
    useEffect(() => {
        setApprovalTokenResolver(resolverFn);
        return () => setApprovalTokenResolver(null);
    }, [resolverFn]);

    // ── Actions ──────────────────────────────────────────────────────────

    const dismiss = useCallback(() => {
        setIsOpen(false);
        if (pendingRef.current) {
            pendingRef.current.reject(new Error('User cancelled command approval'));
            pendingRef.current = null;
        }
    }, []);

    const handleIssue = useCallback(async () => {
        setError('');
        setLoading(true);
        try {
            const ctx = pendingRef.current?.ctx ?? {};
            const res = await commandApprovalApi.issue({
                agent_id: ctx.agentId,
                command_type: ctx.commandType,
                summary: ctx.summary,
            });
            setApprovalId(res.approval_id);
            setMaskedEmail(res.masked_email);
            setExpiresAt(res.expires_at);
            setPhase('code');
        } catch (err: any) {
            const msg = err?.response?.data?.message || err?.message || 'Failed to request approval code';
            setError(msg);
        } finally {
            setLoading(false);
        }
    }, []);

    const handleVerify = useCallback(async () => {
        if (!approvalId || code.length < 4) return;
        setError('');
        setLoading(true);
        try {
            const res = await commandApprovalApi.verify({
                approval_id: approvalId,
                code: code.trim(),
            });
            // Success — resolve the pending promise so the interceptor can
            // replay the original request with the approval_token.
            setIsOpen(false);
            if (pendingRef.current) {
                pendingRef.current.resolve(res.approval_token);
                pendingRef.current = null;
            }
        } catch (err: any) {
            const data = err?.response?.data;
            const errCode = data?.error_code;
            if (errCode === 'APPROVAL_EXPIRED' || errCode === 'APPROVAL_LOCKED') {
                // Challenge is dead — force the user back to the issue step.
                setPhase('issue');
                setCode('');
                setApprovalId('');
            }
            const msg = data?.message || err?.message || 'Verification failed';
            setError(msg);
        } finally {
            setLoading(false);
        }
    }, [approvalId, code]);

    // ── Helpers ──────────────────────────────────────────────────────────

    const commandLabel = pendingRef.current?.ctx?.summary
        ?? pendingRef.current?.ctx?.commandType
        ?? 'Execute Command';

    const expiresLabel = (() => {
        if (!expiresAt) return '';
        try {
            const d = new Date(expiresAt);
            const mins = Math.max(0, Math.round((d.getTime() - Date.now()) / 60_000));
            return mins > 0 ? `Expires in ~${mins} min` : 'Expires soon';
        } catch {
            return '';
        }
    })();

    // ── Render ───────────────────────────────────────────────────────────

    const modal = isOpen
        ? ReactDOM.createPortal(
              <div className="fixed inset-0 z-[10000] flex items-center justify-center p-4">
                  {/* Backdrop */}
                  <div
                      className="absolute inset-0 bg-black/70 backdrop-blur-md animate-fade-in"
                      onClick={dismiss}
                      aria-hidden="true"
                  />

                  {/* Card */}
                  <div
                      className="relative w-full max-w-md bg-slate-900 border border-slate-700/70 rounded-2xl shadow-2xl shadow-cyan-900/30 overflow-hidden animate-slide-up-fade"
                      role="dialog"
                      aria-modal="true"
                      aria-labelledby="cmd-approval-title"
                  >
                      {/* Header */}
                      <div className="flex items-center justify-between px-6 py-4 border-b border-slate-700/60 bg-slate-800/60">
                          <div className="flex items-center gap-3">
                              <div className="p-2 bg-amber-500/10 border border-amber-500/30 rounded-xl">
                                  <ShieldCheck className="w-5 h-5 text-amber-400" />
                              </div>
                              <div>
                                  <h2 id="cmd-approval-title" className="text-sm font-bold text-white tracking-tight">
                                      Command Verification Required
                                  </h2>
                                  <p className="text-[11px] text-slate-400 mt-0.5 font-medium truncate max-w-[260px]">
                                      {commandLabel}
                                  </p>
                              </div>
                          </div>
                          <button
                              onClick={dismiss}
                              className="p-1.5 text-slate-400 hover:text-slate-200 hover:bg-slate-700 rounded-lg transition-all duration-150"
                              aria-label="Cancel"
                          >
                              <X className="w-4 h-4" />
                          </button>
                      </div>

                      {/* Body */}
                      <div className="px-6 py-5 space-y-5">
                          {/* Error banner */}
                          {error && (
                              <div className="flex items-start gap-3 p-3.5 bg-red-900/30 border border-red-500/40 rounded-lg text-red-300 text-xs font-medium">
                                  <AlertTriangle className="w-4 h-4 shrink-0 mt-0.5 text-red-400" />
                                  <span>{error}</span>
                              </div>
                          )}

                          {phase === 'issue' ? (
                              /* ─── Step 1: Request Code ──────────────── */
                              <>
                                  <div className="flex items-start gap-3 p-4 bg-cyan-500/5 border border-cyan-500/20 rounded-lg">
                                      <KeyRound className="w-5 h-5 text-cyan-400 shrink-0 mt-0.5" />
                                      <div className="text-sm text-slate-300">
                                          <p className="font-semibold text-slate-200">
                                              Out-of-Band Approval
                                          </p>
                                          <p className="mt-1 text-slate-400 text-xs leading-relaxed">
                                              To protect against unauthorized command execution, a
                                              6-digit verification code will be sent to the
                                              designated security mailbox. Click below to request
                                              the code.
                                          </p>
                                      </div>
                                  </div>

                                  <button
                                      type="button"
                                      onClick={handleIssue}
                                      disabled={loading}
                                      className="w-full bg-gradient-to-r from-amber-600 to-orange-600 hover:from-amber-500 hover:to-orange-500 text-white font-bold py-3 px-4 rounded-lg flex items-center justify-center gap-2 hover:scale-[1.02] active:scale-[0.98] transition-all duration-200 shadow-[0_0_18px_rgba(245,158,11,0.2)] disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:scale-100"
                                  >
                                      {loading ? (
                                          <Loader2 className="w-5 h-5 animate-spin" />
                                      ) : (
                                          <>
                                              <Mail className="w-5 h-5" />
                                              Send Verification Code
                                          </>
                                      )}
                                  </button>
                              </>
                          ) : (
                              /* ─── Step 2: Enter Code ────────────────── */
                              <>
                                  <div className="flex items-start gap-3 p-4 bg-cyan-500/5 border border-cyan-500/20 rounded-lg">
                                      <ShieldCheck className="w-5 h-5 text-cyan-400 shrink-0 mt-0.5" />
                                      <div className="text-sm text-slate-300">
                                          <p className="font-semibold text-slate-200">Check the security mailbox</p>
                                          <p className="mt-1 text-slate-400 text-xs leading-relaxed">
                                              A 6-digit code was sent to{' '}
                                              <span className="inline-flex items-center gap-1 text-cyan-300 font-mono text-[11px]">
                                                  <Mail className="w-3 h-3" />
                                                  {maskedEmail || '***'}
                                              </span>
                                              .{' '}
                                              {expiresLabel && (
                                                  <span className="text-amber-400/80">{expiresLabel}.</span>
                                              )}
                                          </p>
                                      </div>
                                  </div>

                                  <div>
                                      <label className="block text-[10px] font-semibold text-slate-400 uppercase tracking-wider mb-2">
                                          Verification Code
                                      </label>
                                      <input
                                          ref={codeInputRef}
                                          type="text"
                                          inputMode="numeric"
                                          autoComplete="one-time-code"
                                          pattern="[0-9]*"
                                          maxLength={6}
                                          value={code}
                                          onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))}
                                          onKeyDown={(e) => {
                                              if (e.key === 'Enter' && code.length >= 4 && !loading) {
                                                  handleVerify();
                                              }
                                          }}
                                          className="w-full bg-slate-800/50 border border-slate-700/50 rounded-lg px-4 py-3 text-white text-center text-2xl tracking-[0.6em] font-mono placeholder-slate-600 focus:outline-none focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500 transition-all"
                                          placeholder="●●●●●●"
                                      />
                                  </div>

                                  <button
                                      type="button"
                                      onClick={handleVerify}
                                      disabled={loading || code.length < 4}
                                      className="w-full bg-gradient-to-r from-blue-600 to-cyan-600 hover:from-blue-500 hover:to-cyan-500 text-white font-bold py-3 px-4 rounded-lg flex items-center justify-center gap-2 hover:scale-[1.02] active:scale-[0.98] transition-all duration-200 shadow-[0_0_18px_rgba(6,182,212,0.25)] disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:scale-100"
                                  >
                                      {loading ? (
                                          <Loader2 className="w-5 h-5 animate-spin" />
                                      ) : (
                                          <>
                                              <ShieldCheck className="w-5 h-5" />
                                              Verify &amp; Execute
                                          </>
                                      )}
                                  </button>

                                  <button
                                      type="button"
                                      onClick={() => {
                                          setPhase('issue');
                                          setCode('');
                                          setError('');
                                      }}
                                      className="w-full flex items-center justify-center gap-2 text-xs text-slate-400 hover:text-slate-200 transition-colors"
                                  >
                                      <ArrowLeft className="w-3.5 h-3.5" />
                                      Request a new code
                                  </button>
                              </>
                          )}
                      </div>

                      {/* Footer — subtle branding */}
                      <div className="px-6 py-3 border-t border-slate-700/40 bg-slate-800/40">
                          <p className="text-[10px] text-slate-500 text-center tracking-widest uppercase font-medium">
                              EC2 Command Verification
                          </p>
                      </div>
                  </div>
              </div>,
              document.body,
          )
        : null;

    return (
        <CommandApprovalContext.Provider value={{ isOpen }}>
            {children}
            {modal}
        </CommandApprovalContext.Provider>
    );
}

export default CommandApprovalProvider;
