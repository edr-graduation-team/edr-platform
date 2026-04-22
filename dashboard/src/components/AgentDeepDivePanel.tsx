import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import {
    AlertTriangle,
    Clock,
    Loader2,
    Monitor,
    Shield,
    Terminal,
    Trash2,
    Wifi,
    WifiOff,
} from 'lucide-react';
import React from 'react';
import { agentsApi, authApi, type Agent, type FilterPolicy } from '../api/client';
import { useToast } from './Toast';
import {
    formatDate,
    formatDateTime,
    formatRelativeTime,
    getEffectiveStatus,
} from '../utils/agentDisplay';

function DetailRow({ label, value, mono }: { label: string; value: React.ReactNode; mono?: boolean }) {
    return (
        <div className="flex justify-between items-start gap-3">
            <span className="text-gray-500 dark:text-gray-400 text-xs whitespace-nowrap">{label}</span>
            <span className={`text-right text-gray-900 dark:text-white ${mono ? 'font-mono text-[10px] break-all leading-relaxed' : 'text-xs'}`}>{value}</span>
        </div>
    );
}

const StatusBadge = React.memo(function StatusBadge({ status }: { status: Agent['status'] }) {
    const config = {
        online: { label: 'Online', color: 'badge-online', icon: Wifi },
        offline: { label: 'Offline', color: 'badge-offline', icon: WifiOff },
        degraded: { label: 'Degraded', color: 'badge-degraded', icon: AlertTriangle },
        pending: { label: 'Pending', color: 'badge-warning', icon: Clock },
        suspended: { label: 'Suspended', color: 'badge-danger', icon: WifiOff },
        pending_uninstall: { label: 'Uninstalling…', color: 'badge-warning', icon: Trash2 },
        uninstalled: { label: 'Uninstalled', color: 'badge-offline', icon: Trash2 },
    } as const;
    const { label, color, icon: Icon } = config[status as keyof typeof config] || config.offline;
    return (
        <span className={`badge ${color} flex items-center gap-1`}>
            <Icon className="w-3 h-3" />
            {label}
        </span>
    );
});

function formatActivePolicyPreview(agent: Agent): string {
    const raw = agent.metadata?.filter_policy;
    if (!raw) return '{ "status": "No policy deployed yet" }';
    try {
        const parsed = typeof raw === 'string' ? JSON.parse(raw) : raw;
        return JSON.stringify(parsed, null, 2);
    } catch {
        return typeof raw === 'string' ? raw : JSON.stringify(raw);
    }
}

/**
 * Rich device panels (Identity, Network, Health & QoS, Filter Policy, Recent commands).
 * Uses the same APIs as the former expandable row on Device Management:
 * - Agent fields from `GET /api/v1/agents/:id`
 * - `agentsApi.getCommands(agent.id, { limit: 20 })`
 * - `agentsApi.updateFilterPolicy` when pushing policy
 */
export function AgentDeepDivePanel({ agent }: { agent: Agent }) {
    const queryClient = useQueryClient();
    const { showToast } = useToast();
    const { data: agentCmds, isLoading: cmdsLoading, isError: cmdsError } = useQuery({
        queryKey: ['agent-commands', agent.id],
        queryFn: () => agentsApi.getCommands(agent.id, { limit: 20 }),
        staleTime: 30_000,
    });
    const canPushPolicy = authApi.canPushPolicy();
    const [policyJson, setPolicyJson] = React.useState(
        JSON.stringify(
            {
                exclude_processes: ['svchost.exe'],
                exclude_event_ids: [4, 7, 15, 22],
                trusted_hashes: [],
                rate_limit: { enabled: true, default_max_eps: 500, critical_bypass: true },
            },
            null,
            2,
        ),
    );
    const [policyError, setPolicyError] = React.useState('');

    const policyMutation = useMutation({
        mutationFn: async ({ agentId, policy }: { agentId: string; policy: FilterPolicy }) => {
            return agentsApi.updateFilterPolicy(agentId, policy);
        },
        onSuccess: (data) => {
            showToast(`Filter policy pushed (Command ID: ${data.command_id})`, 'success');
            queryClient.invalidateQueries({ queryKey: ['agents'] });
            queryClient.invalidateQueries({ queryKey: ['agent', agent.id] });
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
    const healthPct = Math.min(100, agent.health_score ?? 0);
    const healthColor =
        healthPct >= 80 ? 'text-green-400' : healthPct >= 60 ? 'text-yellow-400' : healthPct >= 40 ? 'text-orange-400' : 'text-red-400';
    const healthBg =
        healthPct >= 80 ? 'bg-green-500' : healthPct >= 60 ? 'bg-yellow-500' : healthPct >= 40 ? 'bg-orange-500' : 'bg-red-500';
    const cpuPct = agent.cpu_usage || 0;
    const memPct =
        agent.memory_mb && agent.memory_used_mb ? Math.min(100, (agent.memory_used_mb / agent.memory_mb) * 100) : 0;

    return (
        <div className="bg-gray-50/80 dark:bg-[#0d1321] border border-primary-400/20 dark:border-primary-700/40 rounded-xl px-4 py-5 sm:px-6 whitespace-normal text-left shadow-sm">
            {isBlindingRisk && (
                <div className="flex items-start gap-3 p-3.5 mb-4 bg-red-50 dark:bg-red-950/40 border border-red-300 dark:border-red-800 rounded-lg shadow-sm">
                    <AlertTriangle className="w-5 h-5 text-red-500 flex-shrink-0 mt-0.5 animate-pulse" />
                    <div>
                        <p className="text-sm font-bold text-red-700 dark:text-red-300">⚠ CRITICAL — Potential Blinding Attack</p>
                        <p className="text-xs text-red-600 dark:text-red-400 mt-1">
                            Agent is dropping <strong>{dropRate.toFixed(1)}%</strong> of collected events ({eventsDropped.toLocaleString()} /{' '}
                            {eventsCollected.toLocaleString()}).
                            <strong className="block mt-1">
                                Recommended: Inspect Event Pipeline → push stricter filter policy → isolate network if necessary.
                            </strong>
                        </p>
                    </div>
                </div>
            )}
            {!isBlindingRisk && isHighDrop && (
                <div className="flex items-start gap-3 p-3 mb-4 bg-yellow-50 dark:bg-yellow-950/30 border border-yellow-300 dark:border-yellow-700 rounded-lg">
                    <AlertTriangle className="w-4 h-4 text-yellow-600 flex-shrink-0 mt-0.5" />
                    <p className="text-xs text-yellow-700 dark:text-yellow-300">
                        Elevated drop rate: <strong>{dropRate.toFixed(1)}%</strong>. Consider reviewing the agent&apos;s filter policy.
                    </p>
                </div>
            )}
            {isStale && (
                <div className="flex items-center gap-2 p-3 mb-4 bg-yellow-50 dark:bg-yellow-950/20 border border-yellow-300 dark:border-yellow-700 rounded-lg">
                    <Clock className="w-4 h-4 text-yellow-600" />
                    <p className="text-xs text-yellow-700 dark:text-yellow-300">
                        Agent reports <strong>online</strong> but last heartbeat was <strong>{formatRelativeTime(agent.last_seen)}</strong>.
                    </p>
                </div>
            )}

            <div className="grid grid-cols-1 lg:grid-cols-4 gap-4">
                <div className="bg-white dark:bg-gray-800/80 rounded-xl p-4 border border-gray-200 dark:border-gray-700/60 shadow-sm">
                    <h4 className="text-[11px] font-bold text-gray-400 uppercase tracking-widest mb-3 flex items-center gap-1.5">
                        <Monitor className="w-3.5 h-3.5" /> Identity & System
                    </h4>
                    <div className="space-y-2 text-sm">
                        <DetailRow label="Agent ID" mono value={agent.id} />
                        <DetailRow label="Hostname" value={agent.hostname} />
                        <DetailRow
                            label="OS"
                            value={`${agent.os_type.charAt(0).toUpperCase() + agent.os_type.slice(1)} ${agent.os_version || ''}`}
                        />
                        <DetailRow label="Architecture" value={agent.cpu_count ? `${agent.cpu_count} CPU Cores` : 'N/A'} />
                        <DetailRow label="Total RAM" value={agent.memory_mb ? `${(agent.memory_mb / 1024).toFixed(1)} GB` : 'N/A'} />
                        <DetailRow label="Agent Version" value={agent.agent_version ? `v${agent.agent_version}` : 'Unknown'} />
                        <DetailRow label="Installed" value={formatDate(agent.installed_date)} />
                        <DetailRow label="Enrolled" value={formatDate(agent.created_at)} />
                        <DetailRow label="Last Updated" value={formatDateTime(agent.updated_at)} />
                        {agent.current_cert_id && <DetailRow label="Cert ID" mono value={agent.current_cert_id} />}
                        {agent.tags && Object.keys(agent.tags).length > 0 && (
                            <div className="pt-2 border-t border-gray-100 dark:border-gray-700">
                                <span className="text-[10px] text-gray-400 uppercase tracking-wider">Tags</span>
                                <div className="flex flex-wrap gap-1.5 mt-1.5">
                                    {Object.entries(agent.tags).map(([k, v]) => (
                                        <span
                                            key={k}
                                            className="text-[10px] px-2 py-0.5 bg-primary-100 dark:bg-primary-900/40 text-primary-700 dark:text-primary-300 rounded-full font-medium"
                                        >
                                            {k}: {v}
                                        </span>
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
                                            <span className="text-gray-300 font-mono">{String(v)}</span>
                                        </div>
                                    ))}
                                </div>
                            </div>
                        )}
                    </div>
                </div>

                <div className="bg-white dark:bg-gray-800/80 rounded-xl p-4 border border-gray-200 dark:border-gray-700/60 shadow-sm">
                    <h4 className="text-[11px] font-bold text-gray-400 uppercase tracking-widest mb-3 flex items-center gap-1.5">
                        <Wifi className="w-3.5 h-3.5" /> Network
                    </h4>
                    <div className="space-y-3 text-sm">
                        <DetailRow label="Status" value={<StatusBadge status={effectiveStatus} />} />
                        <DetailRow label="Last Heartbeat" value={new Date(agent.last_seen).toLocaleString()} />
                        <DetailRow label="Heartbeat Age" value={formatRelativeTime(agent.last_seen)} />
                        <div className="pt-2 border-t border-gray-100 dark:border-gray-700">
                            <span className="text-[10px] text-gray-400 uppercase tracking-wider">IP Addresses</span>
                            {!agent.ip_addresses || agent.ip_addresses.length === 0 ? (
                                <p className="text-xs text-gray-500 mt-2 italic">Awaiting next heartbeat...</p>
                            ) : (
                                <div className="flex flex-wrap gap-1.5 mt-2">
                                    {agent.ip_addresses.map((ip, i) => (
                                        <span
                                            key={i}
                                            className="inline-flex items-center gap-1 px-2.5 py-1 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 text-emerald-700 dark:text-emerald-300 text-xs font-mono rounded-lg cursor-pointer hover:bg-emerald-100 dark:hover:bg-emerald-900/40 transition-colors"
                                            title="Click to copy"
                                            onClick={() => {
                                                void navigator.clipboard.writeText(ip);
                                                showToast(`Copied: ${ip}`, 'success');
                                            }}
                                        >
                                            <Wifi className="w-3 h-3" />
                                            {ip}
                                        </span>
                                    ))}
                                </div>
                            )}
                        </div>
                        <div className="pt-2 border-t border-gray-100 dark:border-gray-700">
                            <span className="text-[10px] text-gray-400 uppercase tracking-wider">mTLS Certificate</span>
                            <div className="mt-2 space-y-1.5">
                                <DetailRow
                                    label="Status"
                                    value={
                                        certDaysLeft !== null ? (
                                            <span
                                                className={`font-semibold ${certDaysLeft <= 7 ? 'text-red-500' : certDaysLeft <= 30 ? 'text-yellow-500' : 'text-green-400'}`}
                                            >
                                                {certDaysLeft > 0 ? `✓ Valid (${certDaysLeft}d remaining)` : '✗ EXPIRED'}
                                            </span>
                                        ) : (
                                            <span className="text-gray-500 italic">Not provisioned</span>
                                        )
                                    }
                                />
                                {certExpiry && <DetailRow label="Expiry Date" value={certExpiry.toLocaleDateString()} />}
                            </div>
                        </div>
                    </div>
                </div>

                <div className="bg-white dark:bg-gray-800/80 rounded-xl p-4 border border-gray-200 dark:border-gray-700/60 shadow-sm">
                    <h4 className="text-[11px] font-bold text-gray-400 uppercase tracking-widest mb-3 flex items-center gap-1.5">
                        <Shield className="w-3.5 h-3.5" /> Health & QoS
                    </h4>
                    <div className="space-y-2.5 text-sm">
                        <div>
                            <div className="flex justify-between text-xs mb-1">
                                <span className="text-gray-400">Health Score</span>
                                <span className={`font-bold ${healthColor}`}>{healthPct.toFixed(0)}%</span>
                            </div>
                            <div className="h-2.5 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
                                <div className={`h-full rounded-full transition-all duration-500 ${healthBg}`} style={{ width: `${healthPct}%` }} />
                            </div>
                        </div>
                        <div className="pt-2 border-t border-gray-100 dark:border-gray-700 space-y-1.5">
                            <DetailRow label="Events Collected" value={eventsCollected.toLocaleString()} />
                            <DetailRow label="Events Delivered" value={eventsDelivered.toLocaleString()} />
                            <DetailRow
                                label="Events Dropped"
                                value={
                                    <span
                                        className={
                                            eventsDropped > 0
                                                ? isBlindingRisk
                                                    ? 'text-red-500 font-bold'
                                                    : 'text-orange-400 font-semibold'
                                                : 'text-green-400'
                                        }
                                    >
                                        {eventsDropped.toLocaleString()}
                                    </span>
                                }
                            />
                        </div>
                        <div>
                            <div className="flex justify-between text-xs mb-1">
                                <span className="text-gray-400">Drop Rate</span>
                                <span
                                    className={`font-semibold ${isBlindingRisk ? 'text-red-500' : isHighDrop ? 'text-yellow-400' : 'text-green-400'}`}
                                >
                                    {dropRate.toFixed(1)}%
                                </span>
                            </div>
                            <div className="h-2 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
                                <div
                                    className={`h-full rounded-full ${isBlindingRisk ? 'bg-red-500' : isHighDrop ? 'bg-yellow-500' : 'bg-green-500'}`}
                                    style={{ width: `${Math.min(100, dropRate * 2)}%` }}
                                />
                            </div>
                        </div>
                        {eventsCollected > 0 && (
                            <DetailRow
                                label="Delivery Rate"
                                value={
                                    <span
                                        className={
                                            deliveryRate >= 95 ? 'text-green-400 font-semibold' : deliveryRate >= 80 ? 'text-yellow-400' : 'text-red-400 font-semibold'
                                        }
                                    >
                                        {deliveryRate.toFixed(1)}%
                                    </span>
                                }
                            />
                        )}
                        <div className="pt-2 border-t border-gray-100 dark:border-gray-700 space-y-2">
                            <div>
                                <div className="flex justify-between text-xs mb-1">
                                    <span className="text-gray-400">CPU</span>
                                    <span className={cpuPct > 80 ? 'text-red-400 font-bold' : 'text-gray-300'}>{cpuPct.toFixed(1)}%</span>
                                </div>
                                <div className="h-1.5 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
                                    <div
                                        className={`h-full rounded-full ${cpuPct > 80 ? 'bg-red-500' : cpuPct > 50 ? 'bg-yellow-500' : 'bg-blue-500'}`}
                                        style={{ width: `${cpuPct}%` }}
                                    />
                                </div>
                            </div>
                            <div>
                                <div className="flex justify-between text-xs mb-1">
                                    <span className="text-gray-400">Memory</span>
                                    <span className={memPct > 90 ? 'text-red-400 font-bold' : 'text-gray-300'}>
                                        {agent.memory_used_mb || 0} / {agent.memory_mb || '?'} MB
                                    </span>
                                </div>
                                <div className="h-1.5 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
                                    <div
                                        className={`h-full rounded-full ${memPct > 90 ? 'bg-red-500' : memPct > 70 ? 'bg-yellow-500' : 'bg-blue-500'}`}
                                        style={{ width: `${memPct}%` }}
                                    />
                                </div>
                            </div>
                            <DetailRow
                                label="Queue Depth"
                                value={
                                    <span className={(agent.queue_depth || 0) > 100 ? 'text-orange-400 font-bold' : ''}>
                                        {(agent.queue_depth || 0).toLocaleString()}
                                    </span>
                                }
                            />
                        </div>
                    </div>
                </div>

                <div className="bg-white dark:bg-gray-800/80 rounded-xl p-4 border border-gray-200 dark:border-gray-700/60 shadow-sm">
                    <h4 className="text-[11px] font-bold text-gray-400 uppercase tracking-widest mb-3 flex items-center gap-1.5">
                        <Shield className="w-3.5 h-3.5" /> Filter Policy
                    </h4>
                    <div className="space-y-3 text-sm">
                        <div>
                            <span className="text-[10px] text-gray-400 uppercase tracking-wider">Active Policy (Agent-Side)</span>
                            <pre className="mt-1.5 p-2.5 text-[10px] font-mono whitespace-pre-wrap break-words bg-gray-100 dark:bg-gray-900/80 border border-gray-200 dark:border-gray-700 rounded-lg overflow-y-auto max-h-64 text-gray-700 dark:text-gray-300 leading-relaxed">
                                {formatActivePolicyPreview(agent)}
                            </pre>
                        </div>
                        {canPushPolicy && (
                            <div className="pt-2 border-t border-gray-100 dark:border-gray-700">
                                <span className="text-[10px] text-gray-400 uppercase tracking-wider">Push New Policy via C2</span>
                                <textarea
                                    className="mt-1.5 w-full px-2.5 py-2 text-[11px] font-mono bg-gray-50 dark:bg-gray-900/60 border border-gray-200 dark:border-gray-600 rounded-lg resize-none focus:ring-2 focus:ring-primary-500/50 focus:border-primary-500 transition-all"
                                    rows={6}
                                    value={policyJson}
                                    onChange={(e) => {
                                        setPolicyJson(e.target.value);
                                        setPolicyError('');
                                    }}
                                    spellCheck={false}
                                />
                                {policyError && (
                                    <p className="text-[10px] text-red-500 mt-1 flex items-center gap-1">
                                        <AlertTriangle className="w-3 h-3" /> {policyError}
                                    </p>
                                )}
                                <button
                                    type="button"
                                    onClick={handlePolicySubmit}
                                    disabled={policyMutation.isPending}
                                    className="mt-2 w-full text-xs py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg flex items-center justify-center gap-2 transition-all disabled:opacity-50 shadow-sm hover:shadow-md"
                                >
                                    {policyMutation.isPending ? (
                                        <Loader2 className="w-3.5 h-3.5 animate-spin" />
                                    ) : (
                                        <Shield className="w-3.5 h-3.5" />
                                    )}
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
                        Open in Command Center
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
