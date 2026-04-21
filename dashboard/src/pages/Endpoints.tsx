import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { authApi } from '../api/client';
import React, { useState, useEffect, useMemo } from 'react';
import { Link } from 'react-router-dom';
import {
    Search, Monitor, Wifi, WifiOff, AlertTriangle, ChevronDown,
    Play, Shield, FileX, Folder, RefreshCw, X, Check, Clock, Loader2, Power, ShieldAlert, Square, Zap,
    LayoutGrid, List, PanelLeft, Building2, Layers, UserPlus, Terminal
} from 'lucide-react';
import {
    agentsApi,
    type Agent,
    type CommandType,
    type CommandRequest,
    type FilterPolicy
} from '../api/client';
import { Modal, useToast, SkeletonTable } from '../components';
import { useDebounce } from '../hooks/useDebounce';

// Command definitions
const COMMAND_DEFINITIONS: Record<CommandType, { label: string; icon: typeof Play; description: string; color: string }> = {
    kill_process: { label: 'Kill Process', icon: X, description: 'Terminate a running process', color: 'text-red-500' },
    quarantine_file: { label: 'Quarantine File', icon: FileX, description: 'Move file to quarantine', color: 'text-orange-500' },
    collect_logs: { label: 'Collect Logs', icon: Folder, description: 'Gather forensic logs', color: 'text-blue-500' },
    update_policy: { label: 'Update Policy', icon: Shield, description: 'Apply new security policy', color: 'text-indigo-500' },
    restart_agent: { label: 'Restart Agent', icon: RefreshCw, description: 'Restart EDR agent service', color: 'text-amber-500' },
    stop_agent: { label: 'Stop Agent', icon: Square, description: 'Stop the EDR agent service', color: 'text-red-500' },
    start_agent: { label: 'Start Agent', icon: Play, description: 'Start / re-enable the EDR agent service', color: 'text-green-500' },
    restart_machine: { label: 'Restart Machine', icon: RefreshCw, description: 'Reboot the endpoint machine (OS-level restart)', color: 'text-red-500' },
    shutdown_machine: { label: 'Shutdown Machine', icon: Power, description: 'Power off the endpoint machine (OS-level shutdown)', color: 'text-red-700' },
    isolate_network: { label: 'Isolate Network', icon: WifiOff, description: 'Block all network traffic', color: 'text-red-600' },
    restore_network: { label: 'Restore Network', icon: Wifi, description: 'Restore network connectivity', color: 'text-green-500' },
    scan_file: { label: 'Scan File', icon: Search, description: 'Scan a specific file', color: 'text-purple-500' },
    scan_memory: { label: 'Scan Memory', icon: Monitor, description: 'Perform memory analysis', color: 'text-cyan-500' },
    custom: { label: 'Custom Command', icon: Zap, description: 'Execute custom command', color: 'text-gray-500' },
    update_filter_policy: { label: 'Update Filter Policy', icon: Shield, description: 'Push new filtering rules to agent', color: 'text-teal-500' },
};

// Status badge component
const StatusBadge = React.memo(function StatusBadge({ status }: { status: Agent['status'] }) {
    const config = {
        online: { label: 'Online', color: 'badge-online', icon: Wifi },
        offline: { label: 'Offline', color: 'badge-offline', icon: WifiOff },
        degraded: { label: 'Degraded', color: 'badge-degraded', icon: AlertTriangle },
        pending: { label: 'Pending', color: 'badge-warning', icon: Clock },
        suspended: { label: 'Suspended', color: 'badge-danger', icon: X },
    };

    const { label, color, icon: Icon } = config[status] || config.offline;

    return (
        <span className={`badge ${color} flex items-center gap-1`}>
            <Icon className="w-3 h-3" />
            {label}
        </span>
    );
});

// Isolation badge — red warning indicator
const IsolationBadge = React.memo(function IsolationBadge({ isIsolated }: { isIsolated?: boolean }) {
    if (!isIsolated) return null;
    return (
        <span className="badge flex items-center gap-1 text-xs font-bold px-2 py-0.5 rounded-full bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-400 border border-red-300 dark:border-red-700 animate-pulse">
            <ShieldAlert className="w-3 h-3" />
            ISOLATED
        </span>
    );
});

// Health Score Bar
const HealthScoreBar = React.memo(function HealthScoreBar({ score }: { score: number }) {
    const getColor = () => {
        if (score >= 80) return 'bg-green-500';
        if (score >= 60) return 'bg-amber-500';
        return 'bg-red-500';
    };

    return (
        <div className="flex items-center gap-2">
            <div className="health-bar w-16">
                <div
                    className={`health-bar-fill ${getColor()}`}
                    style={{ width: `${Math.min(100, Math.max(0, score))}%` }}
                />
            </div>
            <span className="text-sm text-gray-600 dark:text-gray-300">{score.toFixed(0)}%</span>
        </div>
    );
});

// OS Icon
const OSIcon = React.memo(function OSIcon({ os }: { os: string }) {
    const icons: Record<string, string> = {
        windows: '🖥️',
        linux: '🐧',
        macos: '🍎',
    };
    return <span className="text-lg">{icons[os?.toLowerCase()] || '💻'}</span>;
});

// Command Execution Modal
function CommandExecutionModal({
    isOpen,
    onClose,
    agent,
    commandType,
}: {
    isOpen: boolean;
    onClose: () => void;
    agent: Agent | null;
    commandType: CommandType | null;
}) {
    const queryClient = useQueryClient();
    const { showToast } = useToast();
    const [parameters, setParameters] = useState<Record<string, string>>({});
    const [status, setStatus] = useState<'idle' | 'executing' | 'completed' | 'failed'>('idle');

    // Sensible defaults so one-click "Collect" and memory snapshot work without empty params.
    useEffect(() => {
        if (!isOpen || !commandType) return;
        if (commandType === 'collect_logs') {
            setParameters({ log_types: 'security', time_range: '24h' });
        } else if (commandType === 'scan_memory') {
            setParameters({ log_types: 'security,system', time_range: '24h' });
        }
    }, [isOpen, commandType]);

    const executeMutation = useMutation({
        mutationFn: async ({ agentId, command }: { agentId: string; command: CommandRequest }) => {
            return agentsApi.executeCommand(agentId, command);
        },
        onSuccess: (data, variables) => {
            setStatus('completed');
            showToast(`Command queued successfully (ID: ${data.command_id})`, 'success');
            // Immediate invalidation for status changes that DB updates right away
            queryClient.invalidateQueries({ queryKey: ['agents'] });
            queryClient.invalidateQueries({ queryKey: ['agent-commands', variables.agentId] });
            // Isolation commands take 5-15s for the agent to respond with SUCCESS.
            // The backend only writes is_isolated AFTER the agent's gRPC ACK arrives.
            // Schedule a delayed re-fetch so the UI reflects the actual state.
            const cmdType = variables.command.command_type;
            if (cmdType === 'isolate_network' || cmdType === 'restore_network') {
                setTimeout(() => {
                    queryClient.invalidateQueries({ queryKey: ['agents'] });
                }, 10000);
                setTimeout(() => {
                    queryClient.invalidateQueries({ queryKey: ['agents'] });
                }, 20000);
            }
        },
        onError: (error: Error) => {
            setStatus('failed');
            showToast(`Command failed: ${error.message}`, 'error');
        },
    });

    const handleExecute = () => {
        if (!agent || !commandType) return;

        // Force 'confirm: true' for dangerous OS commands
        const finalParams = { ...parameters };
        if (commandType === 'restart_machine' || commandType === 'shutdown_machine') {
            finalParams['confirm'] = 'true';
        }
        if (commandType === 'collect_logs') {
            const lt = (finalParams.log_types || '').trim();
            if (!lt) {
                showToast('Select at least one log type (or use defaults by re-opening this dialog).', 'error');
                return;
            }
        }
        if (commandType === 'custom') {
            if (!(finalParams.cmd || '').trim()) {
                showToast('Enter a command (e.g. ipconfig /all). Only whitelisted diagnostics are allowed on the agent.', 'error');
                return;
            }
        }

        const forensicCmd =
            commandType === 'collect_logs' ||
            commandType === 'scan_memory' ||
            commandType === 'scan_file' ||
            commandType === 'collect_forensics';
        const timeoutSec = forensicCmd ? 900 : 300;

        setStatus('executing');
        executeMutation.mutate({
            agentId: agent.id,
            command: {
                command_type: commandType,
                parameters: finalParams,
                timeout: timeoutSec,
            },
        });
    };

    const handleClose = () => {
        setStatus('idle');
        setParameters({});
        onClose();
    };

    if (!agent || !commandType) return null;

    const cmdDef = COMMAND_DEFINITIONS[commandType];

    // Parameter fields based on command type
    const renderParameterFields = () => {
        switch (commandType) {
            case 'kill_process':
                return (
                    <div className="space-y-3">
                        <div>
                            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                                Process ID (PID)
                            </label>
                            <input
                                type="text"
                                className="input"
                                placeholder="e.g., 1234"
                                value={parameters.pid || ''}
                                onChange={(e) => setParameters({ ...parameters, pid: e.target.value })}
                            />
                        </div>
                        <div>
                            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                                Process Name (optional)
                            </label>
                            <input
                                type="text"
                                className="input"
                                placeholder="e.g., malware.exe"
                                value={parameters.process_name || ''}
                                onChange={(e) => setParameters({ ...parameters, process_name: e.target.value })}
                            />
                        </div>
                    </div>
                );

            case 'quarantine_file':
                return (
                    <div className="space-y-3">
                        <div>
                            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                                File Path
                            </label>
                            <input
                                type="text"
                                className="input"
                                placeholder="C:\Users\...\malware.exe"
                                value={parameters.file_path || ''}
                                onChange={(e) => setParameters({ ...parameters, file_path: e.target.value })}
                            />
                        </div>
                        <div>
                            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                                SHA256 Hash (optional)
                            </label>
                            <input
                                type="text"
                                className="input font-mono text-xs"
                                placeholder="abc123..."
                                value={parameters.hash_sha256 || ''}
                                onChange={(e) => setParameters({ ...parameters, hash_sha256: e.target.value })}
                            />
                        </div>
                    </div>
                );

            case 'isolate_network':
                return (
                    <div className="space-y-3">
                        <div>
                            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                                Allow List (comma-separated IPs/domains)
                            </label>
                            <input
                                type="text"
                                className="input"
                                placeholder="10.0.0.1, management-server.local"
                                value={parameters.allow_list || ''}
                                onChange={(e) => setParameters({ ...parameters, allow_list: e.target.value })}
                            />
                            <p className="text-xs text-gray-500 mt-1">
                                Device will only communicate with these addresses
                            </p>
                        </div>
                    </div>
                );

            case 'collect_logs':
                return (
                    <div className="space-y-3">
                        <div>
                            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                                Log Types
                            </label>
                            <div className="flex flex-wrap gap-2">
                                {['security', 'sysmon', 'application', 'powershell'].map((type) => (
                                    <label key={type} className="flex items-center gap-1.5">
                                        <input
                                            type="checkbox"
                                            checked={(parameters.log_types || '').includes(type)}
                                            onChange={(e) => {
                                                const current = (parameters.log_types || '').split(',').filter(Boolean);
                                                const updated = e.target.checked
                                                    ? [...current, type]
                                                    : current.filter(t => t !== type);
                                                setParameters({ ...parameters, log_types: updated.join(',') });
                                            }}
                                            className="rounded"
                                        />
                                        <span className="text-sm capitalize">{type}</span>
                                    </label>
                                ))}
                            </div>
                        </div>
                        <div>
                            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                                Time Range
                            </label>
                            <select
                                className="input"
                                value={parameters.time_range || '24h'}
                                onChange={(e) => setParameters({ ...parameters, time_range: e.target.value })}
                            >
                                <option value="1h">Last 1 hour</option>
                                <option value="6h">Last 6 hours</option>
                                <option value="24h">Last 24 hours</option>
                                <option value="7d">Last 7 days</option>
                            </select>
                        </div>
                    </div>
                );

            case 'scan_file':
                return (
                    <div>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                            File Path to Scan
                        </label>
                        <input
                            type="text"
                            className="input"
                            placeholder="C:\Users\...\suspicious.exe"
                            value={parameters.file_path || ''}
                            onChange={(e) => setParameters({ ...parameters, file_path: e.target.value })}
                        />
                    </div>
                );

            case 'custom':
                return (
                    <div className="space-y-2">
                        <p className="text-xs text-amber-800 dark:text-amber-200/90 bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-800 rounded-lg p-2.5">
                            The agent runs <strong>only whitelisted</strong> diagnostics (no cmd.exe): ipconfig, netstat, ping, tracert, pathping, nslookup, whoami, hostname, systeminfo, tasklist, arp, route.
                        </p>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                            Executable + arguments
                        </label>
                        <input
                            type="text"
                            className="input font-mono text-xs"
                            placeholder="ipconfig /all"
                            value={parameters.cmd || ''}
                            onChange={(e) => setParameters({ ...parameters, cmd: e.target.value })}
                        />
                    </div>
                );

            case 'scan_memory':
                return (
                    <div className="space-y-3">
                        <p className="text-xs text-gray-600 dark:text-gray-400 bg-slate-100 dark:bg-slate-800/80 rounded-lg p-2.5 border border-slate-200 dark:border-slate-700">
                            Pulls a <strong>forensic event sample</strong> from the endpoint (Security + System by default) — not a full physical memory image.
                            Add Sysmon below if it is installed.
                        </p>
                        <div>
                            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                                Log channels (comma-separated keys)
                            </label>
                            <input
                                type="text"
                                className="input font-mono text-xs"
                                placeholder="security,system"
                                value={parameters.log_types || ''}
                                onChange={(e) => setParameters({ ...parameters, log_types: e.target.value })}
                            />
                        </div>
                        <div>
                            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                                Time range
                            </label>
                            <select
                                className="input"
                                value={parameters.time_range || '24h'}
                                onChange={(e) => setParameters({ ...parameters, time_range: e.target.value })}
                            >
                                <option value="1h">Last 1 hour</option>
                                <option value="6h">Last 6 hours</option>
                                <option value="24h">Last 24 hours</option>
                                <option value="7d">Last 7 days</option>
                            </select>
                        </div>
                    </div>
                );

            case 'restart_machine':
                return (
                    <div className="space-y-3">
                        <div className="p-3 bg-red-50 dark:bg-red-950/40 border border-red-300 dark:border-red-800 rounded-lg">
                            <p className="text-sm font-bold text-red-700 dark:text-red-300 flex items-center gap-2">
                                <AlertTriangle className="w-4 h-4" />
                                Critical Action Warning
                            </p>
                            <p className="text-xs text-red-600 dark:text-red-400 mt-1">
                                This will forcibly reboot the endpoint at the OS level. Any unsaved work on the target machine will be lost.
                            </p>
                        </div>
                        {/* Hidden input to satisfy React state, but we'll also force it on Mount/Execute */}
                        <p className="text-xs text-gray-500">
                            By clicking Execute, you confirm this action. (Safety parameter <code>confirm="true"</code> will be sent to the agent).
                        </p>
                    </div>
                );

            case 'shutdown_machine':
                return (
                    <div className="space-y-3">
                        <div className="p-3 bg-red-50 dark:bg-red-950/40 border border-red-300 dark:border-red-800 rounded-lg">
                            <p className="text-sm font-bold text-red-700 dark:text-red-300 flex items-center gap-2">
                                <AlertTriangle className="w-4 h-4" />
                                Critical Action Warning
                            </p>
                            <p className="text-xs text-red-600 dark:text-red-400 mt-1">
                                This will completely power off the endpoint. You will NOT be able to start it back up from this console.
                            </p>
                        </div>
                        <p className="text-xs text-gray-500">
                            By clicking Execute, you confirm this action. (Safety parameter <code>confirm="true"</code> will be sent to the agent).
                        </p>
                    </div>
                );

            default:
                return (
                    <p className="text-sm text-gray-500">
                        No additional parameters required for this command.
                    </p>
                );
        }
    };

    return (
        <Modal
            isOpen={isOpen}
            onClose={handleClose}
            title={`${cmdDef.label} - ${agent.hostname}`}
            size="md"
        >
            <div className="space-y-4">
                {/* Status indicator */}
                {status !== 'idle' && (
                    <div className={`p-3 rounded-lg flex items-center gap-3 ${status === 'executing' ? 'bg-blue-50 dark:bg-blue-900/20' :
                        status === 'completed' ? 'bg-green-50 dark:bg-green-900/20' :
                            'bg-red-50 dark:bg-red-900/20'
                        }`}>
                        {status === 'executing' && <Loader2 className="w-5 h-5 text-blue-500 animate-spin" />}
                        {status === 'completed' && <Check className="w-5 h-5 text-green-500" />}
                        {status === 'failed' && <X className="w-5 h-5 text-red-500" />}
                        <span className="text-sm font-medium">
                            {status === 'executing' && 'Executing command...'}
                            {status === 'completed' && 'Command queued successfully'}
                            {status === 'failed' && 'Command failed'}
                        </span>
                    </div>
                )}

                {/* Target info */}
                <div className="p-3 bg-gray-50 dark:bg-gray-900/50 rounded-lg">
                    <div className="flex items-center gap-3">
                        <OSIcon os={agent.os_type} />
                        <div>
                            <p className="font-medium text-gray-900 dark:text-white">{agent.hostname}</p>
                            <p className="text-xs text-gray-500">{agent.os_version}</p>
                        </div>
                        <StatusBadge status={agent.status} />
                    </div>
                </div>

                {/* Command description */}
                <div className="flex items-start gap-3 p-3 border border-gray-200 dark:border-gray-700 rounded-lg">
                    <cmdDef.icon className={`w-5 h-5 mt-0.5 ${cmdDef.color}`} />
                    <div>
                        <p className="font-medium text-gray-900 dark:text-white">{cmdDef.label}</p>
                        <p className="text-sm text-gray-500">{cmdDef.description}</p>
                    </div>
                </div>

                {/* Parameters */}
                {status === 'idle' && renderParameterFields()}

                {/* Actions */}
                <div className="flex justify-end gap-3 pt-4 border-t border-gray-200 dark:border-gray-700">
                    <button onClick={handleClose} className="btn btn-secondary">
                        {status === 'idle' ? 'Cancel' : 'Close'}
                    </button>
                    {status === 'idle' && (
                        <button
                            onClick={handleExecute}
                            disabled={executeMutation.isPending}
                            className="btn btn-primary flex items-center gap-2"
                        >
                            {executeMutation.isPending ? (
                                <Loader2 className="w-4 h-4 animate-spin" />
                            ) : (
                                <Play className="w-4 h-4" />
                            )}
                            Execute
                        </button>
                    )}
                </div>
            </div>
        </Modal>
    );
}

// Format relative time
function formatRelativeTime(timestamp: string) {
    const diff = Date.now() - new Date(timestamp).getTime();
    const minutes = Math.floor(diff / 60000);
    if (minutes < 1) return 'Just now';
    if (minutes < 60) return `${minutes}m ago`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h ago`;
    return `${Math.floor(hours / 24)}d ago`;
}

// Safe date formatters — returns 'N/A' for zero/invalid dates (Go zero time = 0001-01-01)
function formatDate(dateStr?: string | null): string {
    if (!dateStr) return 'N/A';
    const d = new Date(dateStr);
    if (isNaN(d.getTime()) || d.getFullYear() <= 1) return 'N/A';
    return d.toLocaleDateString();
}
function formatDateTime(dateStr?: string | null): string {
    if (!dateStr) return 'N/A';
    const d = new Date(dateStr);
    if (isNaN(d.getTime()) || d.getFullYear() <= 1) return 'N/A';
    return d.toLocaleString();
}

// STALE THRESHOLD: 1 minute in milliseconds (matches server-side sweeper)
const STALE_THRESHOLD_MS = 1 * 60 * 1000;

/**
 * Computes the effective agent status by cross-checking last_seen.
 * If the raw status is 'online' but last_seen > 5 min ago, return 'offline'.
 * This is a frontend safety net — the backend sweeper does the same server-side.
 */
function getEffectiveStatus(agent: Agent): Agent['status'] {
    if (agent.status === 'online' || agent.status === 'degraded') {
        const elapsed = Date.now() - new Date(agent.last_seen).getTime();
        if (elapsed > STALE_THRESHOLD_MS) {
            return 'offline';
        }
    }
    return agent.status;
}

function buildAvailableCommands(agent: Agent): CommandType[] {
    return [
        ...(agent.is_isolated ? (['restore_network'] as CommandType[]) : (['isolate_network'] as CommandType[])),
        'kill_process',
        'quarantine_file',
        'collect_logs',
        'scan_memory',
        'custom',
        'restart_agent',
        'stop_agent',
        'start_agent',
        'restart_machine',
        'shutdown_machine',
    ];
}

function isCommandDisabledForAgent(agent: Agent, cmd: CommandType): boolean {
    const effectiveStatus = getEffectiveStatus(agent);
    const agentRunning = effectiveStatus === 'online' || effectiveStatus === 'degraded';
    const agentSuspended = effectiveStatus === 'suspended';
    const machineOnline = effectiveStatus !== 'offline' && effectiveStatus !== 'pending';
    switch (cmd) {
        case 'start_agent':
            return !agentSuspended;
        case 'stop_agent':
        case 'restart_agent':
            return !agentRunning;
        case 'restart_machine':
        case 'shutdown_machine':
            return !machineOnline;
        case 'restore_network':
            return false;
        default:
            return !agentRunning;
    }
}

// =============================================================================
// Inline Expandable Agent Detail Panel — appears BELOW the clicked row
// =============================================================================

function InlineAgentDetail({ agent }: { agent: Agent }) {
    const queryClient = useQueryClient();
    const { showToast } = useToast();
    const { data: agentCmds, isLoading: cmdsLoading, isError: cmdsError } = useQuery({
        queryKey: ['agent-commands', agent.id],
        queryFn: () => agentsApi.getCommands(agent.id, { limit: 20 }),
        staleTime: 30_000,
    });
    const canPushPolicy = authApi.canPushPolicy();
    const [policyJson, setPolicyJson] = useState(JSON.stringify({
        exclude_processes: ['svchost.exe'],
        exclude_event_ids: [4, 7, 15, 22],
        trusted_hashes: [],
        rate_limit: { enabled: true, default_max_eps: 500, critical_bypass: true },
    }, null, 2));
    const [policyError, setPolicyError] = useState('');

    const policyMutation = useMutation({
        mutationFn: async ({ agentId, policy }: { agentId: string; policy: FilterPolicy }) => {
            return agentsApi.updateFilterPolicy(agentId, policy);
        },
        onSuccess: (data) => {
            showToast(`Filter policy pushed (Command ID: ${data.command_id})`, 'success');
            queryClient.invalidateQueries({ queryKey: ['agents'] });
            queryClient.invalidateQueries({ queryKey: ['agent-commands', agent.id] });
        },
        onError: (error: Error) => {
            showToast(`Policy push failed: ${error.message}`, 'error');
        },
    });

    const handlePolicySubmit = () => {
        try {
            const parsed = JSON.parse(policyJson) as FilterPolicy;
            setPolicyError('');
            policyMutation.mutate({ agentId: agent.id, policy: parsed });
        } catch {
            setPolicyError('Invalid JSON — check syntax before pushing');
        }
    };

    // ─── Computed telemetry ───
    const eventsCollected = agent.events_collected || agent.events_delivered || 0;
    const eventsDropped = agent.events_dropped || 0;
    const eventsDelivered = agent.events_delivered || 0;
    const dropRate = eventsCollected > 0 ? (eventsDropped / eventsCollected) * 100 : 0;
    const deliveryRate = eventsCollected > 0 ? (eventsDelivered / eventsCollected) * 100 : 0;
    const isBlindingRisk = dropRate > 20;
    const isHighDrop = dropRate > 5;
    const effectiveStatus = getEffectiveStatus(agent);
    const isStale = effectiveStatus === 'offline' && agent.status === 'online';
    const certExpiry = agent.cert_expires_at ? new Date(agent.cert_expires_at) : null;
    const certDaysLeft = certExpiry ? Math.ceil((certExpiry.getTime() - Date.now()) / 86400000) : null;
    const healthPct = Math.min(100, agent.health_score);
    const healthColor = healthPct >= 80 ? 'text-green-400' : healthPct >= 60 ? 'text-yellow-400' : healthPct >= 40 ? 'text-orange-400' : 'text-red-400';
    const healthBg = healthPct >= 80 ? 'bg-green-500' : healthPct >= 60 ? 'bg-yellow-500' : healthPct >= 40 ? 'bg-orange-500' : 'bg-red-500';
    const cpuPct = agent.cpu_usage || 0;
    const memPct = agent.memory_mb && agent.memory_used_mb ? Math.min(100, (agent.memory_used_mb / agent.memory_mb) * 100) : 0;

    return (
        <div className="bg-gray-50/80 dark:bg-[#0d1321] border-t-2 border-b-2 border-primary-400/30 dark:border-primary-700/40 px-6 py-5 animate-in whitespace-normal text-left">

            {/* ═══ Alert Banners ═══ */}
            {isBlindingRisk && (
                <div className="flex items-start gap-3 p-3.5 mb-4 bg-red-50 dark:bg-red-950/40 border border-red-300 dark:border-red-800 rounded-lg shadow-sm">
                    <AlertTriangle className="w-5 h-5 text-red-500 flex-shrink-0 mt-0.5 animate-pulse" />
                    <div>
                        <p className="text-sm font-bold text-red-700 dark:text-red-300">⚠ CRITICAL — Potential Blinding Attack</p>
                        <p className="text-xs text-red-600 dark:text-red-400 mt-1">
                            Agent is dropping <strong>{dropRate.toFixed(1)}%</strong> of collected events ({eventsDropped.toLocaleString()} / {eventsCollected.toLocaleString()}).
                            An adversary may be flooding the endpoint with noise to overwhelm the filter and hide malicious activity.
                            <strong className="block mt-1">Recommended: Inspect Event Pipeline → push stricter filter policy → isolate network if necessary.</strong>
                        </p>
                    </div>
                </div>
            )}
            {!isBlindingRisk && isHighDrop && (
                <div className="flex items-start gap-3 p-3 mb-4 bg-yellow-50 dark:bg-yellow-950/30 border border-yellow-300 dark:border-yellow-700 rounded-lg">
                    <AlertTriangle className="w-4 h-4 text-yellow-600 flex-shrink-0 mt-0.5" />
                    <p className="text-xs text-yellow-700 dark:text-yellow-300">
                        Elevated drop rate: <strong>{dropRate.toFixed(1)}%</strong>. Consider reviewing the agent's filter policy to ensure only benign events are excluded.
                    </p>
                </div>
            )}
            {isStale && (
                <div className="flex items-center gap-2 p-3 mb-4 bg-yellow-50 dark:bg-yellow-950/20 border border-yellow-300 dark:border-yellow-700 rounded-lg">
                    <Clock className="w-4 h-4 text-yellow-600" />
                    <p className="text-xs text-yellow-700 dark:text-yellow-300">
                        Agent reports <strong>online</strong> but last heartbeat was <strong>{formatRelativeTime(agent.last_seen)}</strong>. Possible network issue or agent freeze.
                    </p>
                </div>
            )}

            <div className="grid grid-cols-1 lg:grid-cols-4 gap-4">

                {/* ══════════════ SECTION 1: Identity & System ══════════════ */}
                <div className="bg-white dark:bg-gray-800/80 rounded-xl p-4 border border-gray-200 dark:border-gray-700/60 shadow-sm">
                    <h4 className="text-[11px] font-bold text-gray-400 uppercase tracking-widest mb-3 flex items-center gap-1.5">
                        <Monitor className="w-3.5 h-3.5" /> Identity & System
                    </h4>
                    <div className="space-y-2 text-sm">
                        <DetailRow label="Agent ID" mono value={agent.id} />
                        <DetailRow label="Hostname" value={agent.hostname} />
                        <DetailRow label="OS" value={`${agent.os_type.charAt(0).toUpperCase() + agent.os_type.slice(1)} ${agent.os_version || ''}`} />
                        <DetailRow label="Architecture" value={agent.cpu_count ? `${agent.cpu_count} CPU Cores` : 'N/A'} />
                        <DetailRow label="Total RAM" value={agent.memory_mb ? `${(agent.memory_mb / 1024).toFixed(1)} GB` : 'N/A'} />
                        <DetailRow label="Agent Version" value={agent.agent_version ? `v${agent.agent_version}` : 'Unknown'} />
                        <DetailRow label="Installed" value={formatDate(agent.installed_date)} />
                        <DetailRow label="Enrolled" value={formatDate(agent.created_at)} />
                        <DetailRow label="Last Updated" value={formatDateTime(agent.updated_at)} />
                        {agent.current_cert_id && (
                            <DetailRow label="Cert ID" mono value={agent.current_cert_id} />
                        )}
                        {agent.tags && Object.keys(agent.tags).length > 0 && (
                            <div className="pt-2 border-t border-gray-100 dark:border-gray-700">
                                <span className="text-[10px] text-gray-400 uppercase tracking-wider">Tags</span>
                                <div className="flex flex-wrap gap-1.5 mt-1.5">
                                    {Object.entries(agent.tags).map(([k, v]) => (
                                        <span key={k} className="text-[10px] px-2 py-0.5 bg-primary-100 dark:bg-primary-900/40 text-primary-700 dark:text-primary-300 rounded-full font-medium">{k}: {v}</span>
                                    ))}
                                </div>
                            </div>
                        )}
                        {agent.metadata && Object.keys(agent.metadata).length > 0 && (
                            <div className="pt-2 border-t border-gray-100 dark:border-gray-700">
                                <span className="text-[10px] text-gray-400 uppercase tracking-wider">Metadata</span>
                                <div className="mt-1.5 space-y-1">
                                    {Object.entries(agent.metadata).map(([k, v]) => (
                                        <div key={k} className="flex justify-between text-[10px]">
                                            <span className="text-gray-500">{k}</span>
                                            <span className="text-gray-300 font-mono">{v}</span>
                                        </div>
                                    ))}
                                </div>
                            </div>
                        )}
                    </div>
                </div>

                {/* ══════════════ SECTION 2: Network ══════════════ */}
                <div className="bg-white dark:bg-gray-800/80 rounded-xl p-4 border border-gray-200 dark:border-gray-700/60 shadow-sm">
                    <h4 className="text-[11px] font-bold text-gray-400 uppercase tracking-widest mb-3 flex items-center gap-1.5">
                        <Wifi className="w-3.5 h-3.5" /> Network
                    </h4>
                    <div className="space-y-3 text-sm">
                        {/* Connection Status */}
                        <DetailRow label="Status" value={<StatusBadge status={effectiveStatus} />} />
                        <DetailRow label="Last Heartbeat" value={new Date(agent.last_seen).toLocaleString()} />
                        <DetailRow label="Heartbeat Age" value={formatRelativeTime(agent.last_seen)} />

                        {/* IP Addresses as Tags */}
                        <div className="pt-2 border-t border-gray-100 dark:border-gray-700">
                            <span className="text-[10px] text-gray-400 uppercase tracking-wider">IP Addresses</span>
                            {(!agent.ip_addresses || agent.ip_addresses.length === 0) ? (
                                <p className="text-xs text-gray-500 mt-2 italic">Awaiting next heartbeat...</p>
                            ) : (
                                <div className="flex flex-wrap gap-1.5 mt-2">
                                    {agent.ip_addresses.map((ip, i) => (
                                        <span
                                            key={i}
                                            className="inline-flex items-center gap-1 px-2.5 py-1 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 text-emerald-700 dark:text-emerald-300 text-xs font-mono rounded-lg cursor-pointer hover:bg-emerald-100 dark:hover:bg-emerald-900/40 transition-colors"
                                            title="Click to copy"
                                            onClick={() => { navigator.clipboard.writeText(ip); showToast(`Copied: ${ip}`, 'success'); }}
                                        >
                                            <Wifi className="w-3 h-3" />
                                            {ip}
                                        </span>
                                    ))}
                                </div>
                            )}
                        </div>

                        {/* mTLS Certificate */}
                        <div className="pt-2 border-t border-gray-100 dark:border-gray-700">
                            <span className="text-[10px] text-gray-400 uppercase tracking-wider">mTLS Certificate</span>
                            <div className="mt-2 space-y-1.5">
                                <DetailRow label="Status" value={
                                    certDaysLeft !== null ? (
                                        <span className={`font-semibold ${certDaysLeft <= 7 ? 'text-red-500' : certDaysLeft <= 30 ? 'text-yellow-500' : 'text-green-400'}`}>
                                            {certDaysLeft > 0 ? `✓ Valid (${certDaysLeft}d remaining)` : '✗ EXPIRED'}
                                        </span>
                                    ) : <span className="text-gray-500 italic">Not provisioned</span>
                                } />
                                {certExpiry && (
                                    <DetailRow label="Expiry Date" value={certExpiry.toLocaleDateString()} />
                                )}
                            </div>
                        </div>
                    </div>
                </div>

                {/* ══════════════ SECTION 3: Health & QoS (Telemetry) ══════════════ */}
                <div className="bg-white dark:bg-gray-800/80 rounded-xl p-4 border border-gray-200 dark:border-gray-700/60 shadow-sm">
                    <h4 className="text-[11px] font-bold text-gray-400 uppercase tracking-widest mb-3 flex items-center gap-1.5">
                        <Shield className="w-3.5 h-3.5" /> Health & QoS
                    </h4>
                    <div className="space-y-2.5 text-sm">
                        {/* Health Score */}
                        <div>
                            <div className="flex justify-between text-xs mb-1">
                                <span className="text-gray-400">Health Score</span>
                                <span className={`font-bold ${healthColor}`}>{healthPct.toFixed(0)}%</span>
                            </div>
                            <div className="h-2.5 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
                                <div className={`h-full rounded-full transition-all duration-500 ${healthBg}`} style={{ width: `${healthPct}%` }} />
                            </div>
                        </div>

                        {/* Event Pipeline Metrics */}
                        <div className="pt-2 border-t border-gray-100 dark:border-gray-700 space-y-1.5">
                            <DetailRow label="Events Collected" value={eventsCollected.toLocaleString()} />
                            <DetailRow label="Events Delivered" value={eventsDelivered.toLocaleString()} />
                            <DetailRow label="Events Dropped" value={
                                <span className={eventsDropped > 0 ? (isBlindingRisk ? 'text-red-500 font-bold' : 'text-orange-400 font-semibold') : 'text-green-400'}>
                                    {eventsDropped.toLocaleString()}
                                </span>
                            } />
                        </div>

                        {/* Drop Rate Bar */}
                        <div>
                            <div className="flex justify-between text-xs mb-1">
                                <span className="text-gray-400">Drop Rate</span>
                                <span className={`font-semibold ${isBlindingRisk ? 'text-red-500' : isHighDrop ? 'text-yellow-400' : 'text-green-400'}`}>
                                    {dropRate.toFixed(1)}%
                                </span>
                            </div>
                            <div className="h-2 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
                                <div className={`h-full rounded-full ${isBlindingRisk ? 'bg-red-500' : isHighDrop ? 'bg-yellow-500' : 'bg-green-500'}`} style={{ width: `${Math.min(100, dropRate * 2)}%` }} />
                            </div>
                        </div>

                        {eventsCollected > 0 && (
                            <DetailRow label="Delivery Rate" value={
                                <span className={deliveryRate >= 95 ? 'text-green-400 font-semibold' : deliveryRate >= 80 ? 'text-yellow-400' : 'text-red-400 font-semibold'}>
                                    {deliveryRate.toFixed(1)}%
                                </span>
                            } />
                        )}

                        {/* Resource Usage */}
                        <div className="pt-2 border-t border-gray-100 dark:border-gray-700 space-y-2">
                            <div>
                                <div className="flex justify-between text-xs mb-1">
                                    <span className="text-gray-400">CPU</span>
                                    <span className={cpuPct > 80 ? 'text-red-400 font-bold' : 'text-gray-300'}>{cpuPct.toFixed(1)}%</span>
                                </div>
                                <div className="h-1.5 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
                                    <div className={`h-full rounded-full ${cpuPct > 80 ? 'bg-red-500' : cpuPct > 50 ? 'bg-yellow-500' : 'bg-blue-500'}`} style={{ width: `${cpuPct}%` }} />
                                </div>
                            </div>
                            <div>
                                <div className="flex justify-between text-xs mb-1">
                                    <span className="text-gray-400">Memory</span>
                                    <span className={memPct > 90 ? 'text-red-400 font-bold' : 'text-gray-300'}>{agent.memory_used_mb || 0} / {agent.memory_mb || '?'} MB</span>
                                </div>
                                <div className="h-1.5 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
                                    <div className={`h-full rounded-full ${memPct > 90 ? 'bg-red-500' : memPct > 70 ? 'bg-yellow-500' : 'bg-blue-500'}`} style={{ width: `${memPct}%` }} />
                                </div>
                            </div>
                            <DetailRow label="Queue Depth" value={
                                <span className={(agent.queue_depth || 0) > 100 ? 'text-orange-400 font-bold' : ''}>{(agent.queue_depth || 0).toLocaleString()}</span>
                            } />
                        </div>
                    </div>
                </div>

                {/* ══════════════ SECTION 4: Policy ══════════════ */}
                <div className="bg-white dark:bg-gray-800/80 rounded-xl p-4 border border-gray-200 dark:border-gray-700/60 shadow-sm">
                    <h4 className="text-[11px] font-bold text-gray-400 uppercase tracking-widest mb-3 flex items-center gap-1.5">
                        <Shield className="w-3.5 h-3.5" /> Filter Policy
                    </h4>
                    <div className="space-y-3 text-sm">
                        {/* Read-Only Current Policy */}
                        <div>
                            <span className="text-[10px] text-gray-400 uppercase tracking-wider">Active Policy (Agent-Side)</span>
                            <pre className="mt-1.5 p-2.5 text-[10px] font-mono whitespace-pre-wrap break-words bg-gray-100 dark:bg-gray-900/80 border border-gray-200 dark:border-gray-700 rounded-lg overflow-y-auto max-h-64 text-gray-700 dark:text-gray-300 leading-relaxed">
                                {agent.metadata?.filter_policy
                                    ? JSON.stringify(JSON.parse(agent.metadata.filter_policy), null, 2)
                                    : '{ "status": "No policy deployed yet" }'}
                            </pre>
                        </div>

                        {/* Editable Push Form */}
                        {canPushPolicy && (
                            <div className="pt-2 border-t border-gray-100 dark:border-gray-700">
                                <span className="text-[10px] text-gray-400 uppercase tracking-wider">Push New Policy via C2</span>
                                <textarea
                                    className="mt-1.5 w-full px-2.5 py-2 text-[11px] font-mono bg-gray-50 dark:bg-gray-900/60 border border-gray-200 dark:border-gray-600 rounded-lg resize-none focus:ring-2 focus:ring-primary-500/50 focus:border-primary-500 transition-all"
                                    rows={6}
                                    value={policyJson}
                                    onChange={(e) => { setPolicyJson(e.target.value); setPolicyError(''); }}
                                    spellCheck={false}
                                />
                                {policyError && (
                                    <p className="text-[10px] text-red-500 mt-1 flex items-center gap-1">
                                        <AlertTriangle className="w-3 h-3" /> {policyError}
                                    </p>
                                )}
                                <button
                                    onClick={handlePolicySubmit}
                                    disabled={policyMutation.isPending}
                                    className="mt-2 w-full text-xs py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg flex items-center justify-center gap-2 transition-all disabled:opacity-50 shadow-sm hover:shadow-md"
                                >
                                    {policyMutation.isPending ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Shield className="w-3.5 h-3.5" />}
                                    Push Policy to Agent
                                </button>
                            </div>
                        )}
                    </div>
                </div>
            </div>

            <div className="mt-5 pt-4 border-t border-gray-200 dark:border-gray-700/80">
                <div className="flex flex-wrap items-center justify-between gap-2 mb-3">
                    <h4 className="text-[11px] font-bold text-gray-400 uppercase tracking-widest flex items-center gap-1.5">
                        <Terminal className="w-3.5 h-3.5" /> Recent commands
                    </h4>
                    <Link
                        to={`/responses?agent_id=${encodeURIComponent(agent.id)}`}
                        className="text-xs font-medium text-primary-600 dark:text-primary-400 hover:underline"
                    >
                        Open in Action Center
                    </Link>
                </div>
                {cmdsLoading && <p className="text-xs text-gray-500">Loading command history…</p>}
                {cmdsError && <p className="text-xs text-red-500">Could not load command history.</p>}
                {!cmdsLoading && !cmdsError && (agentCmds?.data?.length ?? 0) === 0 && (
                    <p className="text-xs text-gray-500 dark:text-gray-400 italic">No commands recorded for this device yet.</p>
                )}
                {!cmdsLoading && !cmdsError && (agentCmds?.data?.length ?? 0) > 0 && (
                    <div className="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700 bg-white/50 dark:bg-gray-900/40">
                        <table className="w-full text-left text-[11px] text-gray-700 dark:text-gray-300">
                            <thead>
                                <tr className="border-b border-gray-200 dark:border-gray-700 text-gray-500 uppercase tracking-wider">
                                    <th className="py-2 px-2 font-semibold">ID</th>
                                    <th className="py-2 px-2 font-semibold">Type</th>
                                    <th className="py-2 px-2 font-semibold">Status</th>
                                    <th className="py-2 px-2 font-semibold">Issued</th>
                                </tr>
                            </thead>
                            <tbody>
                                {agentCmds!.data.map((c) => (
                                    <tr key={c.id} className="border-b border-gray-100 dark:border-gray-800/80 last:border-0">
                                        <td className="py-1.5 px-2 font-mono text-[10px] text-gray-500">{c.id.slice(0, 8)}…</td>
                                        <td className="py-1.5 px-2">{c.command_type.replace(/_/g, ' ')}</td>
                                        <td className="py-1.5 px-2">{c.status}</td>
                                        <td className="py-1.5 px-2 whitespace-nowrap">{new Date(c.issued_at).toLocaleString()}</td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                )}
            </div>
        </div>
    );
}

// ─── Helper: label-value row ───
function DetailRow({ label, value, mono }: { label: string; value: React.ReactNode; mono?: boolean }) {
    return (
        <div className="flex justify-between items-start gap-3">
            <span className="text-gray-500 dark:text-gray-400 text-xs whitespace-nowrap">{label}</span>
            <span className={`text-right text-gray-900 dark:text-white ${mono ? 'font-mono text-[10px] break-all leading-relaxed' : 'text-xs'}`}>{value}</span>
        </div>
    );
}

function dmProfile(agent: Agent): string {
    return agent.tags?.profile ?? agent.metadata?.profile ?? '—';
}
function dmCustomer(agent: Agent): string {
    return agent.tags?.customer ?? agent.metadata?.customer ?? '—';
}
function dmLastUser(agent: Agent): string {
    return agent.metadata?.logged_in_user ?? agent.tags?.logged_in_user ?? '—';
}
function dmComponentsLabel(agent: Agent): string {
    const eff = getEffectiveStatus(agent);
    return eff === 'online' || eff === 'degraded' ? 'EDR' : '—';
}

// Main Endpoints Page
export default function Endpoints() {
    const queryClient = useQueryClient();
    useToast(); // Toast is used inside mutations
    const [selectedAgent, setSelectedAgent] = useState<Agent | null>(null);
    const [selectedCommand, setSelectedCommand] = useState<CommandType | null>(null);
    const [expandedAgentId, setExpandedAgentId] = useState<string | null>(null);
    const [filters, setFilters] = useState({
        status: '',
        os_type: '',
        search: '',
    });
    const [viewMode, setViewMode] = useState<'table' | 'grid'>('table');
    const [treeVisible, setTreeVisible] = useState(false);
    const [treeSearch, setTreeSearch] = useState('');
    const [selectedGroupId, setSelectedGroupId] = useState<string>('g1');
    const [selectedIds, setSelectedIds] = useState<Set<string>>(() => new Set());

    const debouncedSearch = useDebounce(filters.search, 300);

    const toggleSelectOne = (id: string) => {
        setSelectedIds((prev) => {
            const next = new Set(prev);
            if (next.has(id)) next.delete(id);
            else next.add(id);
            return next;
        });
    };

    // Fetch agents
    const { data, isLoading, error } = useQuery({
        queryKey: ['agents', filters.status, filters.os_type, debouncedSearch],
        queryFn: () => agentsApi.list({
            limit: 50,
            status: filters.status || undefined,
            os_type: filters.os_type || undefined,
            search: debouncedSearch || undefined,
            sort_by: 'health_score',
            sort_order: 'desc',
        }),
        refetchInterval: 10000, // Refresh every 10 seconds for near-real-time status
    });

    const agents = data?.data || [];
    const total = data?.pagination?.total || 0;

    const toggleSelectAll = () => {
        setSelectedIds((prev) => {
            if (prev.size === agents.length && agents.length > 0) return new Set();
            return new Set(agents.map((a) => a.id));
        });
    };

    useEffect(() => {
        const valid = new Set(agents.map((a) => a.id));
        setSelectedIds((prev) => {
            const next = new Set<string>();
            prev.forEach((id) => {
                if (valid.has(id)) next.add(id);
            });
            return next;
        });
    }, [agents]);

    const handleCommand = (agent: Agent, command: CommandType) => {
        setSelectedAgent(agent);
        setSelectedCommand(command);
    };

    const toolbarTargetAgent = useMemo(() => {
        if (selectedIds.size !== 1) return null;
        const id = [...selectedIds][0];
        return agents.find((a) => a.id === id) ?? null;
    }, [selectedIds, agents]);

    if (error) {
        return (
            <div className="card text-center py-12">
                <WifiOff className="w-12 h-12 text-gray-400 mx-auto mb-4" />
                <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">
                    Connection Manager Unavailable
                </h3>
                <p className="text-gray-500 mb-4">
                    Unable to connect to the Connection Manager service.
                </p>
                <button
                    onClick={() => queryClient.invalidateQueries({ queryKey: ['agents'] })}
                    className="btn btn-primary"
                >
                    <RefreshCw className="w-4 h-4 mr-2" />
                    Retry
                </button>
            </div>
        );
    }

    return (
        <div
            data-section-id="management-devices-root"
            className="relative flex flex-col min-h-[calc(100vh-2rem)] lg:min-h-[calc(100vh-1rem)] h-full -mx-4 sm:-mx-6 lg:-mx-8 -my-4 sm:-my-6 lg:-my-8 p-4 sm:p-6 lg:p-8 bg-slate-50 dark:bg-gradient-to-br dark:from-slate-900 dark:via-[#0b1120] dark:to-slate-900 transition-colors overflow-hidden"
        >
            {/* Background ambient glow effect for Endpoints interface */}
            <div className="absolute top-1/4 right-1/4 w-[600px] h-[600px] pointer-events-none -translate-y-1/2 translate-x-1/3" style={{ background: 'radial-gradient(circle, rgba(59,130,246,0.08) 0%, transparent 70%)' }} />
            
            <div className="relative flex-1 flex flex-col min-h-0 max-w-[1800px] mx-auto w-full space-y-4 lg:space-y-5">
                <div className="flex flex-wrap items-center justify-between gap-3 shrink-0">
                    <div>
                        <h1 className="text-2xl sm:text-3xl font-bold text-slate-900 dark:text-white tracking-tight">Device management</h1>
                        <p className="text-sm text-slate-500 mt-1">Fleet list, grouping shell, and actions (OpenEDR / Xcitium-style layout)</p>
                    </div>
                    {agents.length > 0 && (() => {
                        let onlineCount = 0, offlineCount = 0, degradedCount = 0;
                        agents.forEach((a) => {
                            const eff = getEffectiveStatus(a);
                            if (eff === 'online') onlineCount++;
                            else if (eff === 'degraded') degradedCount++;
                            else offlineCount++;
                        });
                        return (
                            <div className="flex flex-wrap items-center gap-2 sm:gap-3">
                                <span className="flex items-center gap-2 px-3 py-1.5 bg-green-500/10 dark:bg-green-500/20 text-green-700 dark:text-green-400 border border-green-500/20 rounded-full text-xs font-bold uppercase tracking-wider">
                                    <span className="w-1.5 h-1.5 rounded-full bg-green-500" />
                                    {onlineCount} Online
                                </span>
                                <span className="flex items-center gap-2 px-3 py-1.5 bg-slate-500/10 dark:bg-slate-500/20 text-slate-600 dark:text-slate-400 border border-slate-500/20 rounded-full text-xs font-bold uppercase tracking-wider">
                                    <span className="w-1.5 h-1.5 rounded-full bg-slate-400" />
                                    {offlineCount} Offline
                                </span>
                                <span className="flex items-center gap-2 px-3 py-1.5 bg-amber-500/10 dark:bg-amber-500/20 text-amber-700 dark:text-amber-400 border border-amber-500/20 rounded-full text-xs font-bold uppercase tracking-wider">
                                    <span className="w-1.5 h-1.5 rounded-full bg-amber-500 animate-pulse" />
                                    {degradedCount} Degraded
                                </span>
                            </div>
                        );
                    })()}
                </div>

                <div
                    data-section-id="dm-structure"
                    className="flex flex-1 min-h-0 rounded-xl border border-slate-200/90 dark:border-slate-700/60 bg-white dark:bg-slate-900/35 overflow-hidden shadow-sm"
                >
                    {treeVisible && (
                        <aside className="w-[min(100%,260px)] sm:w-[260px] shrink-0 border-r border-slate-200 dark:border-slate-700 flex flex-col bg-slate-50/90 dark:bg-slate-950/80">
                            <div className="p-3 border-b border-slate-200 dark:border-slate-700 space-y-2">
                                <div className="relative">
                                    <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-400" />
                                    <input
                                        type="text"
                                        placeholder="Search group name"
                                        value={treeSearch}
                                        onChange={(e) => setTreeSearch(e.target.value)}
                                        className="w-full pl-9 pr-3 py-2 text-sm rounded-lg border border-slate-200 dark:border-slate-600 bg-white dark:bg-slate-900 text-slate-800 dark:text-slate-100 focus:outline-none focus:ring-2 focus:ring-primary-500/40"
                                    />
                                </div>
                                <button
                                    type="button"
                                    onClick={() => {
                                        setSelectedGroupId('g1');
                                        setFilters((f) => ({ ...f, search: '' }));
                                    }}
                                    className="w-full flex items-center gap-2 px-2 py-1.5 text-xs font-medium rounded-lg border border-dashed border-slate-300 dark:border-slate-600 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800/80"
                                >
                                    <Layers className="w-3.5 h-3.5 opacity-70" />
                                    Show all
                                </button>
                            </div>
                            <div className="flex-1 overflow-y-auto custom-scrollbar p-2 text-sm">
                                {(!treeSearch.trim() || 'default tenant'.includes(treeSearch.trim().toLowerCase())) && (
                                    <div className="mb-1">
                                        <div className="flex items-center gap-2 px-2 py-1.5 text-slate-700 dark:text-slate-200 font-medium">
                                            <Building2 className="w-4 h-4 shrink-0 text-slate-500" />
                                            <span className="truncate">Default tenant</span>
                                        </div>
                                        {(!treeSearch.trim() || 'all endpoints'.includes(treeSearch.trim().toLowerCase()) || 'default'.includes(treeSearch.trim().toLowerCase())) && (
                                            <div className="pl-3 ml-1.5 border-l border-slate-200 dark:border-slate-700">
                                                <button
                                                    type="button"
                                                    onClick={() => setSelectedGroupId('g1')}
                                                    className={`w-full text-left flex items-center gap-2 px-2 py-1.5 rounded-md transition-colors ${selectedGroupId === 'g1' ? 'bg-primary-500/15 text-primary-700 dark:text-primary-300' : 'hover:bg-slate-200/60 dark:hover:bg-slate-800'}`}
                                                >
                                                    <Folder className="w-4 h-4 shrink-0 text-slate-500" />
                                                    <span className="truncate">All endpoints</span>
                                                </button>
                                            </div>
                                        )}
                                    </div>
                                )}
                            </div>
                        </aside>
                    )}

                    <div className="flex-1 flex flex-col min-w-0 min-h-0">
                        <div className="flex items-stretch border-b border-slate-200 dark:border-slate-700 bg-slate-50/80 dark:bg-slate-900/50">
                            <button
                                type="button"
                                onClick={() => setTreeVisible((v) => !v)}
                                className={`shrink-0 px-2.5 flex items-center justify-center border-r border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 ${treeVisible ? 'text-[#f19637]' : 'text-slate-500'}`}
                                title={treeVisible ? 'Hide tree' : 'Show tree'}
                            >
                                <PanelLeft className="w-4 h-4" />
                            </button>
                            <nav className="flex items-end gap-1 px-1 min-w-0 overflow-x-auto">
                                <Link
                                    to="/management/profiles"
                                    className="shrink-0 px-3 py-2.5 text-sm text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white border-b-2 border-transparent"
                                >
                                    Group management
                                </Link>
                                <span className="shrink-0 px-3 py-2.5 text-sm font-semibold text-slate-900 dark:text-white border-b-2 border-[#f19637]">
                                    Device management
                                </span>
                            </nav>
                        </div>

                        <div
                            className="sticky top-0 z-20 flex flex-wrap items-center gap-x-1 gap-y-1 px-2 py-2 border-b border-slate-700/40 shadow-[0_1px_0_rgba(0,0,0,0.06)] overflow-x-auto custom-scrollbar"
                            style={{ background: 'var(--xc-nav-bg, #0a043d)' }}
                        >
                            <Link
                                to="/deploy"
                                className="inline-flex items-center gap-1.5 px-2 py-1.5 rounded text-[11px] text-white/95 hover:bg-white/10 border border-white/15 whitespace-nowrap"
                            >
                                <UserPlus className="w-3.5 h-3.5 shrink-0" />
                                Enroll device
                            </Link>
                            {authApi.canExecuteCommands() && (() => {
                                // Always render action buttons — use a generic command list when no device is selected
                                const genericCommands: CommandType[] = [
                                    'isolate_network', 'kill_process', 'quarantine_file',
                                    'collect_logs', 'scan_memory', 'custom', 'restart_agent',
                                    'stop_agent', 'start_agent', 'restart_machine', 'shutdown_machine',
                                ];
                                const cmds = toolbarTargetAgent
                                    ? buildAvailableCommands(toolbarTargetAgent)
                                    : genericCommands;
                                return cmds.map((cmd) => {
                                    const def = COMMAND_DEFINITIONS[cmd];
                                    const Icon = def.icon;
                                    // Disabled if: no device selected OR device is not eligible
                                    const disabled = !toolbarTargetAgent || isCommandDisabledForAgent(toolbarTargetAgent, cmd);
                                    const showSeparator = cmd === 'restart_machine';
                                    return (
                                        <React.Fragment key={cmd}>
                                            {showSeparator && (
                                                <div
                                                    className="hidden sm:block w-px h-6 bg-white/25 mx-0.5 shrink-0 self-center"
                                                    aria-hidden
                                                />
                                            )}
                                            <button
                                                type="button"
                                                disabled={disabled}
                                                title={
                                                    !toolbarTargetAgent
                                                        ? `${def.label} — Select an online device first`
                                                        : `${def.label} — ${def.description}`
                                                }
                                                onClick={() => toolbarTargetAgent && handleCommand(toolbarTargetAgent, cmd)}
                                                className="inline-flex items-center gap-1 px-2 py-1.5 rounded text-[11px] text-white/90 border border-white/10 disabled:opacity-40 disabled:cursor-not-allowed whitespace-nowrap hover:bg-white/10"
                                            >
                                                <Icon className={`w-3.5 h-3.5 shrink-0 ${def.color}`} />
                                                <span className="max-xl:hidden">{def.label}</span>
                                            </button>
                                        </React.Fragment>
                                    );
                                });
                            })()}
                        </div>

                        <div className="flex flex-wrap items-center gap-2 px-3 py-2 border-b border-slate-200 dark:border-slate-700 bg-slate-50/90 dark:bg-slate-900/40">
                            <div className="relative flex-1 min-w-[200px]">
                                <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-400" />
                                <input
                                    type="text"
                                    placeholder="Search for devices"
                                    value={filters.search}
                                    onChange={(e) => setFilters({ ...filters, search: e.target.value })}
                                    className="w-full bg-white dark:bg-slate-900/70 border border-slate-200 dark:border-slate-600 text-slate-800 dark:text-slate-100 rounded-lg pl-9 pr-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500/40"
                                />
                            </div>
                            <button
                                type="button"
                                onClick={() => queryClient.invalidateQueries({ queryKey: ['agents'] })}
                                className="p-2 rounded-lg border border-slate-200 dark:border-slate-600 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800"
                                title="Refresh table"
                            >
                                <RefreshCw className="w-4 h-4" />
                            </button>
                            <button
                                type="button"
                                onClick={() => setFilters({ status: '', os_type: '', search: '' })}
                                className="p-2 rounded-lg border border-slate-200 dark:border-slate-600 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800"
                                title="Clear filters"
                            >
                                <X className="w-4 h-4" />
                            </button>
                        </div>

                        <div className="flex flex-wrap gap-3 items-end px-3 py-2 border-b border-slate-200 dark:border-slate-700 bg-white/60 dark:bg-slate-900/20">
                            <div className="relative">
                                <label className="block text-[10px] font-semibold uppercase tracking-wider text-slate-500 mb-1">Status</label>
                                <select
                                    value={filters.status}
                                    onChange={(e) => setFilters({ ...filters, status: e.target.value })}
                                    className="w-40 appearance-none bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-200 rounded-lg pl-3 pr-8 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500/50"
                                >
                                    <option value="">All</option>
                                    <option value="online">Online</option>
                                    <option value="offline">Offline</option>
                                    <option value="degraded">Degraded</option>
                                    <option value="pending">Pending</option>
                                </select>
                                <ChevronDown className="pointer-events-none absolute right-2 bottom-2.5 w-4 h-4 text-slate-500" />
                            </div>
                            <div className="relative">
                                <label className="block text-[10px] font-semibold uppercase tracking-wider text-slate-500 mb-1">OS</label>
                                <select
                                    value={filters.os_type}
                                    onChange={(e) => setFilters({ ...filters, os_type: e.target.value })}
                                    className="w-40 appearance-none bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-200 rounded-lg pl-3 pr-8 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500/50"
                                >
                                    <option value="">All</option>
                                    <option value="windows">Windows</option>
                                    <option value="linux">Linux</option>
                                    <option value="macos">macOS</option>
                                </select>
                                <ChevronDown className="pointer-events-none absolute right-2 bottom-2.5 w-4 h-4 text-slate-500" />
                            </div>
                            <div className="ml-auto flex items-center gap-1">
                                <button
                                    type="button"
                                    onClick={() => setViewMode('table')}
                                    className={`p-2 rounded-lg transition-all ${viewMode === 'table' ? 'bg-cyan-500/20 text-cyan-600 dark:text-cyan-400' : 'text-slate-400 hover:bg-slate-200 dark:hover:bg-slate-800'}`}
                                    title="Table"
                                >
                                    <List className="w-4 h-4" />
                                </button>
                                <button
                                    type="button"
                                    onClick={() => setViewMode('grid')}
                                    className={`p-2 rounded-lg transition-all ${viewMode === 'grid' ? 'bg-cyan-500/20 text-cyan-600 dark:text-cyan-400' : 'text-slate-400 hover:bg-slate-200 dark:hover:bg-slate-800'}`}
                                    title="Grid"
                                >
                                    <LayoutGrid className="w-4 h-4" />
                                </button>
                            </div>
                        </div>

                        <div className="relative z-10 flex-1 flex flex-col min-h-0 overflow-hidden">
            {isLoading ? (
                <div className="p-4 bg-white dark:bg-slate-800 rounded-2xl border border-slate-200 dark:border-slate-700/50">
                    <SkeletonTable rows={8} columns={7} />
                </div>
            ) : agents.length === 0 ? (
                <div className="bg-white dark:bg-slate-800 rounded-2xl border border-slate-200 dark:border-slate-700/50 flex flex-col items-center justify-center text-center py-16">
                    <Monitor className="w-12 h-12 text-blue-400 mx-auto mb-4 opacity-50" />
                    <h3 className="text-lg font-medium text-slate-900 dark:text-white mb-2">No Endpoints Found</h3>
                    <p className="text-slate-500">{filters.search || filters.status || filters.os_type ? 'Try adjusting your filters' : 'No agents have registered yet'}</p>
                </div>
            ) : viewMode === 'grid' ? (
                /* Card Grid View */
                <div className="flex-1 overflow-auto custom-scrollbar">
                    <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4 gap-4 pb-4">
                        {agents.map(agent => {
                            const eff = getEffectiveStatus(agent);
                            const pulseCls = eff === 'online' ? 'bg-emerald-500 status-pulse-online' : eff === 'degraded' ? 'bg-amber-400 status-pulse-degraded' : 'bg-slate-400 status-pulse-offline';
                            const health = agent.health_score ?? 0;
                            const healthClr = health >= 80 ? 'bg-emerald-500' : health >= 50 ? 'bg-amber-400' : 'bg-rose-500';
                            const osIcon = agent.os_type === 'windows' ? '⊞' : agent.os_type === 'linux' ? '🐧' : '🍎';
                            return (
                                <div key={agent.id} className="agent-card bg-white dark:bg-slate-900/60 border border-slate-200 dark:border-slate-700/50 rounded-xl p-4 flex flex-col gap-3 shadow-sm" style={{ animation: 'slideInRight 0.15s ease-out' }}>
                                    {/* Card Header */}
                                    <div className="flex items-center gap-3">
                                        <span className={`w-3 h-3 rounded-full shrink-0 ${pulseCls}`} />
                                        <Link
                                            to={`/management/devices/${agent.id}`}
                                            className="font-bold text-slate-800 dark:text-white text-sm truncate flex-1 hover:text-primary-600 dark:hover:text-primary-400"
                                            onClick={(e) => e.stopPropagation()}
                                        >
                                            {agent.hostname}
                                        </Link>
                                        <span className="text-lg" title={agent.os_type}>{osIcon}</span>
                                    </div>
                                    {/* Health bar */}
                                    <div>
                                        <div className="flex justify-between text-[11px] text-slate-500 mb-1">
                                            <span>Health</span><span className="font-semibold">{health}%</span>
                                        </div>
                                        <div className="h-1.5 bg-slate-200 dark:bg-slate-700 rounded-full overflow-hidden">
                                            <div className={`h-full rounded-full transition-all ${healthClr}`} style={{ width: `${health}%` }} />
                                        </div>
                                    </div>
                                    {/* Meta */}
                                    <div className="flex items-center justify-between text-[11px] text-slate-500 dark:text-slate-400">
                                        <span className="font-mono truncate max-w-[120px]" title={(agent.ip_addresses || [])[0] || ''}>{(agent.ip_addresses || [])[0] || '—'}</span>
                                        <span className="flex items-center gap-1"><Clock className="w-3 h-3" />{agent.last_seen ? new Date(agent.last_seen).toLocaleTimeString() : '—'}</span>
                                    </div>
                                    {/* Actions */}
                                    <div className="flex gap-2 pt-1 border-t border-slate-100 dark:border-slate-800">
                                        {authApi.canViewResponses() && (
                                            <>
                                                <button onClick={() => handleCommand(agent, 'isolate_network')} className="flex-1 py-1.5 rounded-lg text-[11px] font-semibold bg-rose-500/10 text-rose-600 dark:text-rose-400 hover:bg-rose-500/20 transition-colors border border-rose-500/20" title="Isolate Network">Isolate</button>
                                                <button onClick={() => handleCommand(agent, 'collect_logs')} className="flex-1 py-1.5 rounded-lg text-[11px] font-semibold bg-cyan-500/10 text-cyan-600 dark:text-cyan-400 hover:bg-cyan-500/20 transition-colors border border-cyan-500/20" title="Collect Logs">Collect</button>
                                            </>
                                        )}
                                        <button onClick={() => setExpandedAgentId(expandedAgentId === agent.id ? null : agent.id)} className="flex-1 py-1.5 rounded-lg text-[11px] font-semibold bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-700 transition-colors">{expandedAgentId === agent.id ? 'Less' : 'Details'}</button>
                                    </div>
                                </div>
                            );
                        })}
                    </div>
                </div>
            ) : (
                <div className="flex-1 flex flex-col min-h-0 bg-white dark:bg-slate-800 border-t border-slate-200 dark:border-slate-700/50 overflow-hidden">
                    <div className="flex-1 overflow-auto overflow-x-auto custom-scrollbar">
                        <table className="w-full text-left text-sm whitespace-nowrap min-w-[1000px]">
                            <thead className="sticky top-0 z-10 bg-slate-100 dark:bg-slate-800 border-b-2 border-slate-200 dark:border-slate-700/80 text-[10px] uppercase tracking-wider text-slate-600 dark:text-slate-300 font-bold shadow-sm">
                                <tr>
                                    <th className="py-3 px-2 w-11">
                                        <input
                                            type="checkbox"
                                            className="rounded border-slate-300 dark:border-slate-600"
                                            checked={agents.length > 0 && selectedIds.size === agents.length}
                                            onChange={toggleSelectAll}
                                            aria-label="Select all on page"
                                        />
                                    </th>
                                    <th className="py-3 px-2">OS</th>
                                    <th className="py-3 px-3 min-w-[180px]">Name</th>
                                    <th className="py-3 px-2">Status</th>
                                    <th className="py-3 px-2">Health</th>
                                    <th className="py-3 px-2">Profile</th>
                                    <th className="py-3 px-2">Active components</th>
                                    <th className="py-3 px-2">Customer</th>
                                    <th className="py-3 px-2 min-w-[120px]">Last logged in user</th>
                                    <th className="py-3 px-2">Last activity</th>
                                </tr>
                            </thead>
                            <tbody>
                                {agents.map((agent, rowIdx) => (
                                    <React.Fragment key={agent.id}>
                                        <tr
                                            className={`transition-colors cursor-pointer border-b border-slate-100 dark:border-slate-700/50 ${rowIdx % 2 === 0 ? 'bg-slate-50/80 dark:bg-slate-900/30' : 'bg-white dark:bg-slate-800/80'} ${expandedAgentId === agent.id
                                                ? 'bg-primary-50/90 dark:bg-primary-900/25 border-l-2 border-l-primary-500'
                                                : 'hover:bg-slate-100/80 dark:hover:bg-slate-800/90'
                                                }`}
                                            onClick={() => setExpandedAgentId((prev) => (prev === agent.id ? null : agent.id))}
                                        >
                                            <td className="py-3 px-2 align-middle" onClick={(e) => e.stopPropagation()}>
                                                <input
                                                    type="checkbox"
                                                    className="rounded border-slate-300 dark:border-slate-600"
                                                    checked={selectedIds.has(agent.id)}
                                                    onChange={() => toggleSelectOne(agent.id)}
                                                    aria-label={`Select ${agent.hostname}`}
                                                />
                                            </td>
                                            <td className="py-3 px-2 align-middle">
                                                <div className="flex items-center gap-1.5">
                                                    <OSIcon os={agent.os_type} />
                                                    <span className="text-xs capitalize text-slate-700 dark:text-slate-200">{agent.os_type}</span>
                                                </div>
                                            </td>
                                            <td className="py-3 px-3 align-middle">
                                                <div className="flex items-start gap-2 min-w-0">
                                                    <div className={`w-1 self-stretch min-h-[2rem] rounded-full shrink-0 ${expandedAgentId === agent.id ? 'bg-primary-500' : 'bg-transparent'}`} />
                                                    <div className="min-w-0">
                                                        <Link
                                                            to={`/management/devices/${agent.id}`}
                                                            className="font-medium text-slate-900 dark:text-white truncate block hover:text-primary-600 dark:hover:text-primary-400"
                                                            onClick={(e) => e.stopPropagation()}
                                                        >
                                                            {agent.hostname}
                                                        </Link>
                                                        <p className="text-[11px] text-slate-500 truncate">
                                                            {agent.ip_addresses?.[0] || agent.id.slice(0, 8)}
                                                            {agent.agent_version ? ` · v${agent.agent_version}` : ''}
                                                        </p>
                                                    </div>
                                                    <ChevronDown className={`w-4 h-4 text-slate-400 shrink-0 ml-auto transition-transform ${expandedAgentId === agent.id ? 'rotate-180' : ''}`} />
                                                </div>
                                            </td>
                                            <td className="py-3 px-2 align-middle">
                                                <div className="flex flex-wrap items-center gap-1">
                                                    <StatusBadge status={getEffectiveStatus(agent)} />
                                                    <IsolationBadge isIsolated={agent.is_isolated} />
                                                </div>
                                            </td>
                                            <td className="py-3 px-2 align-middle max-w-[120px]">
                                                <HealthScoreBar score={agent.health_score} />
                                            </td>
                                            <td className="py-3 px-2 align-middle text-xs text-slate-600 dark:text-slate-300 max-w-[100px] truncate" title={dmProfile(agent)}>
                                                {dmProfile(agent)}
                                            </td>
                                            <td className="py-3 px-2 align-middle text-xs text-slate-600 dark:text-slate-300">
                                                {dmComponentsLabel(agent)}
                                            </td>
                                            <td className="py-3 px-2 align-middle text-xs text-slate-600 dark:text-slate-300 max-w-[100px] truncate" title={dmCustomer(agent)}>
                                                {dmCustomer(agent)}
                                            </td>
                                            <td className="py-3 px-2 align-middle text-xs text-slate-600 dark:text-slate-300 max-w-[120px] truncate" title={dmLastUser(agent)}>
                                                {dmLastUser(agent)}
                                            </td>
                                            <td className="py-3 px-2 align-middle text-xs text-slate-500">
                                                <span title={new Date(agent.last_seen).toLocaleString()}>
                                                    {formatRelativeTime(agent.last_seen)}
                                                </span>
                                            </td>
                                        </tr>
                                        {expandedAgentId === agent.id && (
                                            <tr className={rowIdx % 2 === 0 ? 'bg-slate-50/80 dark:bg-slate-900/30' : 'bg-white dark:bg-slate-800/80'}>
                                                <td colSpan={10} className="p-0 border-b-2 border-primary-500/20">
                                                    <InlineAgentDetail agent={agent} />
                                                </td>
                                            </tr>
                                        )}
                                    </React.Fragment>
                                ))}
                            </tbody>
                        </table>
                    </div>
                    <div className="shrink-0 px-4 py-2.5 bg-slate-50/50 dark:bg-slate-900/40 border-t border-slate-200 dark:border-slate-700/60 text-xs text-slate-500 flex flex-wrap items-center justify-between gap-2">
                        <span>Showing {agents.length} of {total} devices</span>
                        {selectedIds.size > 0 && (
                            <span className="text-slate-600 dark:text-slate-400">{selectedIds.size} selected</span>
                        )}
                    </div>
                </div>
            )}
                        </div>
                    </div>
                </div>

            <CommandExecutionModal
                isOpen={!!selectedAgent && !!selectedCommand}
                onClose={() => {
                    setSelectedAgent(null);
                    setSelectedCommand(null);
                }}
                agent={selectedAgent}
                commandType={selectedCommand}
            />

            </div>
        </div>
    );
}
