import { useState, useEffect, useCallback } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import {
    Download, Shield, Server, Key, Terminal, CheckCircle,
    AlertTriangle, Copy, Loader2, HardDrive, Info,
    ShieldCheck, Cpu, ChevronRight, ExternalLink
} from 'lucide-react';
import { agentBuildApi, type EnrollmentToken } from '../api/client';
import { Modal } from '../components';

// ─────────────────────────────────────────────────────────────────────────────
// Session-persistent form state
// ─────────────────────────────────────────────────────────────────────────────
const STORAGE_KEY = 'edr_agent_build_form';

interface BuildFormState {
    serverIP: string;
    serverDomain: string;
    serverPort: string;
    tokenId: string;
}

function loadFormState(): BuildFormState {
    try {
        const raw = sessionStorage.getItem(STORAGE_KEY);
        if (raw) return JSON.parse(raw);
    } catch { /* ignore */ }
    return { serverIP: '', serverDomain: '', serverPort: '47051', tokenId: '' };
}

function saveFormState(state: BuildFormState) {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(state));
}

function clearFormState() {
    sessionStorage.removeItem(STORAGE_KEY);
}

// ─────────────────────────────────────────────────────────────────────────────
// Build Configuration Modal
// ─────────────────────────────────────────────────────────────────────────────
function BuildModal({
    isOpen,
    onClose,
    tokens,
    tokensLoading,
}: {
    isOpen: boolean;
    onClose: () => void;
    tokens: EnrollmentToken[];
    tokensLoading: boolean;
}) {
    const navigate = useNavigate();
    const [form, setForm] = useState<BuildFormState>(loadFormState);
    const [isBuilding, setIsBuilding] = useState(false);
    const [buildProgress, setBuildProgress] = useState('');
    const [error, setError] = useState('');
    const [buildResult, setBuildResult] = useState<{ sha256: string; filename: string } | null>(null);

    // Persist form state on change
    useEffect(() => {
        saveFormState(form);
    }, [form]);

    const updateField = (field: keyof BuildFormState, value: string) => {
        setForm(prev => ({ ...prev, [field]: value }));
    };

    const handleBuild = async (skip: boolean) => {
        setError('');
        setBuildResult(null);

        if (!form.tokenId) {
            setError('An enrollment token is required. Select a valid token.');
            return;
        }
        if (!skip && (!form.serverIP || !form.serverDomain)) {
            setError('Server IP and Domain are required unless you choose "Skip Config".');
            return;
        }

        setIsBuilding(true);
        setBuildProgress('Preparing build environment...');

        try {
            setBuildProgress('Cross-compiling agent for Windows (amd64)...');

            const result = await agentBuildApi.build({
                server_ip: skip ? undefined : form.serverIP,
                server_domain: skip ? undefined : form.serverDomain,
                server_port: skip ? undefined : (form.serverPort || '47051'),
                token_id: form.tokenId,
                skip_config: skip,
            });

            setBuildProgress('Build complete. Starting download...');

            // Trigger browser download
            const url = URL.createObjectURL(result.blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = result.filename;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);

            setBuildResult({ sha256: result.sha256, filename: result.filename });
            setBuildProgress('');
            clearFormState();
        } catch (err: unknown) {
            // Try to read error from blob response
            let message = 'Build failed. Check server logs for details.';
            if (err && typeof err === 'object' && 'response' in err) {
                const resp = (err as { response?: { data?: Blob } }).response;
                if (resp?.data instanceof Blob) {
                    try {
                        const text = await resp.data.text();
                        const parsed = JSON.parse(text);
                        message = parsed.message || parsed.build_output || message;
                    } catch { /* ignore */ }
                }
            }
            setError(message);
            setBuildProgress('');
        } finally {
            setIsBuilding(false);
        }
    };

    const handleClose = () => {
        if (!isBuilding) {
            onClose();
        }
    };

    const selectedToken = tokens.find(t => t.id === form.tokenId);

    return (
        <Modal isOpen={isOpen} onClose={handleClose} title="Build Agent Binary" size="lg" closeOnOverlayClick={!isBuilding}>
            <div className="space-y-5">
                {/* Error message */}
                {error && (
                    <div className="p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300 flex items-start gap-2">
                        <AlertTriangle className="w-4 h-4 mt-0.5 shrink-0" />
                        <span>{error}</span>
                    </div>
                )}

                {/* Build success */}
                {buildResult && (
                    <div className="p-4 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg">
                        <div className="flex items-center gap-2 text-emerald-700 dark:text-emerald-300 font-medium mb-2">
                            <CheckCircle className="w-5 h-5" />
                            Agent Built Successfully
                        </div>
                        <div className="text-sm text-emerald-600 dark:text-emerald-400 space-y-1">
                            <p><span className="font-medium">File:</span> {buildResult.filename}</p>
                            <p className="font-mono text-xs break-all">
                                <span className="font-medium font-sans">SHA256:</span> {buildResult.sha256}
                            </p>
                        </div>
                    </div>
                )}

                {/* Token Selection */}
                <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                        <span className="flex items-center gap-1.5">
                            <Key className="w-4 h-4 text-cyan-500" />
                            Enrollment Token <span className="text-red-500">*</span>
                        </span>
                    </label>
                    {tokensLoading ? (
                        <div className="flex items-center gap-2 text-sm text-gray-500 py-2">
                            <Loader2 className="w-4 h-4 animate-spin" /> Loading tokens...
                        </div>
                    ) : tokens.length === 0 ? (
                        <div className="p-3 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg">
                            <p className="text-sm text-amber-700 dark:text-amber-300 mb-2">
                                No valid enrollment tokens available.
                            </p>
                            <button
                                onClick={() => navigate('/tokens')}
                                className="text-sm text-cyan-600 dark:text-cyan-400 hover:underline flex items-center gap-1"
                            >
                                Create a new token <ExternalLink className="w-3.5 h-3.5" />
                            </button>
                        </div>
                    ) : (
                        <>
                            <select
                                value={form.tokenId}
                                onChange={e => updateField('tokenId', e.target.value)}
                                className="input w-full"
                                disabled={isBuilding}
                            >
                                <option value="">— Select a token —</option>
                                {tokens.map(t => (
                                    <option key={t.id} value={t.id}>
                                        {t.description || 'Unnamed'} — uses: {t.use_count}{t.max_uses ? `/${t.max_uses}` : '/∞'}
                                        {t.expires_at ? ` — expires: ${new Date(t.expires_at).toLocaleDateString()}` : ''}
                                    </option>
                                ))}
                            </select>
                            {selectedToken && (
                                <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                                    Token: <span className="font-mono">{selectedToken.token.slice(0, 8)}...{selectedToken.token.slice(-4)}</span>
                                </p>
                            )}
                        </>
                    )}
                </div>

                {/* Divider */}
                <div className="relative">
                    <div className="absolute inset-0 flex items-center"><div className="w-full border-t border-gray-200 dark:border-gray-700" /></div>
                    <div className="relative flex justify-center text-xs">
                        <span className="bg-white dark:bg-gray-800 px-3 text-gray-500">Server Configuration</span>
                    </div>
                </div>

                {/* Server Config */}
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    <div>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                            Server IP
                        </label>
                        <input
                            type="text"
                            value={form.serverIP}
                            onChange={e => updateField('serverIP', e.target.value)}
                            placeholder="e.g. 192.168.1.10"
                            className="input w-full"
                            disabled={isBuilding}
                        />
                    </div>
                    <div>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                            Server Domain
                        </label>
                        <input
                            type="text"
                            value={form.serverDomain}
                            onChange={e => updateField('serverDomain', e.target.value)}
                            placeholder="e.g. edr.local"
                            className="input w-full"
                            disabled={isBuilding}
                        />
                    </div>
                    <div>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                            gRPC Port
                        </label>
                        <input
                            type="text"
                            value={form.serverPort}
                            onChange={e => updateField('serverPort', e.target.value)}
                            placeholder="47051"
                            className="input w-full"
                            disabled={isBuilding}
                        />
                    </div>
                </div>

                <div className="p-3 bg-blue-50 dark:bg-blue-900/15 border border-blue-200 dark:border-blue-800 rounded-lg text-xs text-blue-700 dark:text-blue-300 flex items-start gap-2">
                    <Info className="w-4 h-4 mt-0.5 shrink-0" />
                    <div>
                        <strong>Skip Config</strong> embeds only the token and CA certificate. The installer
                        will require server IP, domain, and port as CLI arguments during installation.
                    </div>
                </div>

                {/* Building progress */}
                {isBuilding && (
                    <div className="p-4 bg-gray-50 dark:bg-gray-900/50 border border-gray-200 dark:border-gray-700 rounded-lg">
                        <div className="flex items-center gap-3">
                            <Loader2 className="w-5 h-5 text-cyan-500 animate-spin" />
                            <div>
                                <p className="text-sm font-medium text-gray-900 dark:text-white">Building Agent...</p>
                                <p className="text-xs text-gray-500 dark:text-gray-400">{buildProgress}</p>
                            </div>
                        </div>
                        <div className="mt-3 w-full bg-gray-200 dark:bg-gray-700 rounded-full h-1.5 overflow-hidden">
                            <div className="bg-cyan-500 h-full rounded-full animate-pulse" style={{ width: '60%' }} />
                        </div>
                    </div>
                )}

                {/* Action buttons */}
                <div className="flex flex-col sm:flex-row gap-3 pt-2">
                    <button
                        onClick={() => handleBuild(false)}
                        disabled={isBuilding || tokens.length === 0}
                        className="flex-1 flex items-center justify-center gap-2 px-4 py-2.5 bg-cyan-600 hover:bg-cyan-700 text-white rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed font-medium text-sm"
                    >
                        <Download className="w-4 h-4" />
                        Build & Download
                    </button>
                    <button
                        onClick={() => handleBuild(true)}
                        disabled={isBuilding || tokens.length === 0}
                        className="flex-1 flex items-center justify-center gap-2 px-4 py-2.5 bg-gray-600 hover:bg-gray-700 dark:bg-gray-700 dark:hover:bg-gray-600 text-white rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed font-medium text-sm"
                    >
                        <Terminal className="w-4 h-4" />
                        Build (Skip Config)
                    </button>
                </div>
            </div>
        </Modal>
    );
}

// ─────────────────────────────────────────────────────────────────────────────
// Copyable Code Block
// ─────────────────────────────────────────────────────────────────────────────
function CodeBlock({ code, label }: { code: string; label?: string }) {
    const [copied, setCopied] = useState(false);

    const handleCopy = useCallback(() => {
        navigator.clipboard.writeText(code);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
    }, [code]);

    return (
        <div className="relative group">
            {label && (
                <span className="text-xs font-medium text-gray-500 dark:text-gray-400 mb-1 block">{label}</span>
            )}
            <div className="bg-gray-900 dark:bg-black rounded-lg p-4 font-mono text-sm text-green-400 overflow-x-auto">
                <pre className="whitespace-pre-wrap">{code}</pre>
            </div>
            <button
                onClick={handleCopy}
                className="absolute top-2 right-2 p-1.5 bg-gray-700 hover:bg-gray-600 text-gray-300 rounded-md opacity-0 group-hover:opacity-100 transition-opacity"
                title="Copy to clipboard"
            >
                {copied ? <CheckCircle className="w-4 h-4 text-green-400" /> : <Copy className="w-4 h-4" />}
            </button>
        </div>
    );
}

// ─────────────────────────────────────────────────────────────────────────────
// Step Card
// ─────────────────────────────────────────────────────────────────────────────
function StepCard({ step, title, children, icon: Icon }: {
    step: number;
    title: string;
    children: React.ReactNode;
    icon: React.ElementType;
}) {
    return (
        <div className="relative flex gap-4">
            {/* Step number + connector */}
            <div className="flex flex-col items-center">
                <div className="w-10 h-10 rounded-full bg-gradient-to-br from-cyan-500 to-blue-600 flex items-center justify-center text-white font-bold text-sm shadow-lg shadow-cyan-500/20 shrink-0">
                    {step}
                </div>
                <div className="w-0.5 flex-1 bg-gradient-to-b from-cyan-500/30 to-transparent mt-2" />
            </div>

            {/* Content */}
            <div className="flex-1 pb-8">
                <div className="flex items-center gap-2 mb-2">
                    <Icon className="w-5 h-5 text-cyan-500" />
                    <h3 className="text-base font-semibold text-gray-900 dark:text-white">{title}</h3>
                </div>
                <div className="text-sm text-gray-600 dark:text-gray-400 space-y-3">
                    {children}
                </div>
            </div>
        </div>
    );
}

// ─────────────────────────────────────────────────────────────────────────────
// Main Page
// ─────────────────────────────────────────────────────────────────────────────
export default function AgentDeployment() {
    const [isModalOpen, setIsModalOpen] = useState(false);

    const { data: validTokens = [], isLoading: tokensLoading } = useQuery({
        queryKey: ['valid-enrollment-tokens'],
        queryFn: () => agentBuildApi.listValidTokens(),
        staleTime: 30_000,
    });

    return (
        <div className="space-y-6">
            {/* Page Header */}
            <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
                <div>
                    <h1 className="text-2xl font-bold text-gray-900 dark:text-white flex items-center gap-3">
                        <div className="p-2.5 bg-gradient-to-br from-cyan-500 to-blue-600 rounded-xl shadow-lg shadow-cyan-500/20">
                            <Download className="w-6 h-6 text-white" />
                        </div>
                        Agent Deployment
                    </h1>
                    <p className="text-gray-500 dark:text-gray-400 mt-1">
                        Build, configure, and deploy the EDR agent to endpoint machines.
                    </p>
                </div>
                <button
                    id="build-agent-btn"
                    onClick={() => setIsModalOpen(true)}
                    className="flex items-center gap-2 px-5 py-2.5 bg-gradient-to-r from-cyan-600 to-blue-600 hover:from-cyan-700 hover:to-blue-700 text-white rounded-xl font-medium transition-all duration-200 shadow-lg shadow-cyan-500/20 hover:shadow-cyan-500/30"
                >
                    <Cpu className="w-5 h-5" />
                    Build Agent
                </button>
            </div>

            {/* Status Cards */}
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                <div className="card p-5">
                    <div className="flex items-center gap-3 mb-2">
                        <div className="p-2 bg-emerald-100 dark:bg-emerald-900/30 rounded-lg">
                            <ShieldCheck className="w-5 h-5 text-emerald-600 dark:text-emerald-400" />
                        </div>
                        <h3 className="font-semibold text-gray-900 dark:text-white">Secure Build</h3>
                    </div>
                    <p className="text-sm text-gray-600 dark:text-gray-400">
                        CA certificate is embedded at build time — no insecure network fetch required.
                    </p>
                </div>

                <div className="card p-5">
                    <div className="flex items-center gap-3 mb-2">
                        <div className="p-2 bg-cyan-100 dark:bg-cyan-900/30 rounded-lg">
                            <Key className="w-5 h-5 text-cyan-600 dark:text-cyan-400" />
                        </div>
                        <h3 className="font-semibold text-gray-900 dark:text-white">Valid Tokens</h3>
                    </div>
                    <p className="text-sm text-gray-600 dark:text-gray-400">
                        {tokensLoading ? (
                            <span className="flex items-center gap-1"><Loader2 className="w-3 h-3 animate-spin" /> Loading...</span>
                        ) : (
                            <><span className="text-2xl font-bold text-cyan-600 dark:text-cyan-400">{validTokens.length}</span> enrollment tokens available for agent builds.</>
                        )}
                    </p>
                </div>

                <div className="card p-5">
                    <div className="flex items-center gap-3 mb-2">
                        <div className="p-2 bg-violet-100 dark:bg-violet-900/30 rounded-lg">
                            <HardDrive className="w-5 h-5 text-violet-600 dark:text-violet-400" />
                        </div>
                        <h3 className="font-semibold text-gray-900 dark:text-white">Cross-Platform</h3>
                    </div>
                    <p className="text-sm text-gray-600 dark:text-gray-400">
                        Agents are compiled as Windows x64 binaries, ready for deployment on any Windows endpoint.
                    </p>
                </div>
            </div>

            {/* Main Content — Two Column Layout */}
            <div className="grid grid-cols-1 lg:grid-cols-5 gap-6">
                {/* Left: Instructions */}
                <div className="lg:col-span-3 card p-6">
                    <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-6 flex items-center gap-2">
                        <Terminal className="w-5 h-5 text-cyan-500" />
                        Deployment Guide
                    </h2>

                    <StepCard step={1} title="Build the Agent" icon={Cpu}>
                        <p>
                            Click <strong className="text-cyan-600 dark:text-cyan-400">"Build Agent"</strong> above
                            to configure and compile a pre-configured agent binary. The build process embeds the CA
                            certificate and your selected enrollment token directly into the binary.
                        </p>
                    </StepCard>

                    <StepCard step={2} title="Transfer to Endpoint" icon={HardDrive}>
                        <p>
                            Copy the downloaded <code className="px-1.5 py-0.5 bg-gray-100 dark:bg-gray-700 rounded text-xs font-mono">edr-agent.exe</code> to
                            the target Windows machine. Use a secure transfer method (USB, internal share, SCCM, etc.).
                        </p>
                    </StepCard>

                    <StepCard step={3} title="Install (Full Config)" icon={Shield}>
                        <p>
                            If the agent was built with all server configuration embedded:
                        </p>
                        <CodeBlock
                            label="Run as Administrator"
                            code={`.\\edr-agent.exe -install`}
                        />
                        <p className="text-xs text-gray-500 dark:text-gray-400">
                            No additional parameters needed — server IP, domain, port, and token are all embedded.
                        </p>
                    </StepCard>

                    <StepCard step={4} title="Install (Skip Config)" icon={Terminal}>
                        <p>
                            If the agent was built with <strong>"Skip Config"</strong>, you must provide
                            server details as CLI arguments:
                        </p>
                        <CodeBlock
                            label="Run as Administrator"
                            code={`.\\edr-agent.exe -install ^\n  -server-ip 192.168.1.10 ^\n  -server-domain edr.local ^\n  -server-port 47051`}
                        />
                        <p className="text-xs text-gray-500 dark:text-gray-400">
                            The enrollment token is still embedded — no need to pass it via CLI (security policy).
                        </p>
                    </StepCard>

                    <StepCard step={5} title="Verify Installation" icon={CheckCircle}>
                        <p>Check that the agent service is running:</p>
                        <CodeBlock code={`sc query EDRAgent\nGet-Content C:\\ProgramData\\EDR\\logs\\agent.log -Tail 20`} />
                        <p>
                            The agent should appear in the{' '}
                            <a href="/endpoints" className="text-cyan-600 dark:text-cyan-400 hover:underline inline-flex items-center gap-1">
                                Endpoints <ChevronRight className="w-3 h-3" />
                            </a>{' '}
                            page within seconds.
                        </p>
                    </StepCard>
                </div>

                {/* Right: Quick Reference Panel */}
                <div className="lg:col-span-2 space-y-4">
                    <div className="card p-5">
                        <h3 className="font-semibold text-gray-900 dark:text-white mb-3 flex items-center gap-2">
                            <Server className="w-4 h-4 text-cyan-500" />
                            Architecture Overview
                        </h3>
                        <div className="space-y-3 text-sm text-gray-600 dark:text-gray-400">
                            <div className="flex items-start gap-2">
                                <div className="w-2 h-2 rounded-full bg-cyan-500 mt-1.5 shrink-0" />
                                <span><strong>Build server</strong> compiles agent with embedded CA cert + token</span>
                            </div>
                            <div className="flex items-start gap-2">
                                <div className="w-2 h-2 rounded-full bg-emerald-500 mt-1.5 shrink-0" />
                                <span><strong>Agent</strong> installs as Windows Service (SYSTEM account)</span>
                            </div>
                            <div className="flex items-start gap-2">
                                <div className="w-2 h-2 rounded-full bg-violet-500 mt-1.5 shrink-0" />
                                <span><strong>mTLS enrollment</strong> using embedded token for certificate provisioning</span>
                            </div>
                            <div className="flex items-start gap-2">
                                <div className="w-2 h-2 rounded-full bg-amber-500 mt-1.5 shrink-0" />
                                <span><strong>Connectivity check</strong> validates DNS + TCP before service registration</span>
                            </div>
                        </div>
                    </div>

                    <div className="card p-5">
                        <h3 className="font-semibold text-gray-900 dark:text-white mb-3 flex items-center gap-2">
                            <AlertTriangle className="w-4 h-4 text-amber-500" />
                            Important Notes
                        </h3>
                        <ul className="space-y-2 text-sm text-gray-600 dark:text-gray-400">
                            <li className="flex items-start gap-2">
                                <ChevronRight className="w-4 h-4 text-cyan-500 mt-0.5 shrink-0" />
                                <span>Token is <strong className="text-gray-800 dark:text-gray-200">always required</strong> — cannot be skipped or overridden from CLI.</span>
                            </li>
                            <li className="flex items-start gap-2">
                                <ChevronRight className="w-4 h-4 text-cyan-500 mt-0.5 shrink-0" />
                                <span>Run the installer as <strong className="text-gray-800 dark:text-gray-200">Administrator</strong> on the target machine.</span>
                            </li>
                            <li className="flex items-start gap-2">
                                <ChevronRight className="w-4 h-4 text-cyan-500 mt-0.5 shrink-0" />
                                <span>If the CA certificate is rotated, all agents must be <strong className="text-gray-800 dark:text-gray-200">rebuilt and redeployed</strong>.</span>
                            </li>
                            <li className="flex items-start gap-2">
                                <ChevronRight className="w-4 h-4 text-cyan-500 mt-0.5 shrink-0" />
                                <span>The hosts file is updated idempotently — reinstalling won't create duplicate entries.</span>
                            </li>
                        </ul>
                    </div>

                    <div className="card p-5">
                        <h3 className="font-semibold text-gray-900 dark:text-white mb-3 flex items-center gap-2">
                            <Terminal className="w-4 h-4 text-cyan-500" />
                            Uninstall Command
                        </h3>
                        <CodeBlock code={`.\\edr-agent.exe -uninstall -token <uninstall-token>`} />
                        <p className="text-xs text-gray-500 dark:text-gray-400 mt-2">
                            Requires an authorization token to prevent unauthorized uninstallation.
                        </p>
                    </div>
                </div>
            </div>

            {/* Build Modal */}
            <BuildModal
                isOpen={isModalOpen}
                onClose={() => setIsModalOpen(false)}
                tokens={validTokens}
                tokensLoading={tokensLoading}
            />
        </div>
    );
}
