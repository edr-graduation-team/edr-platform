import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import {
    ArrowLeft, Activity, Terminal, Shield, HardDrive, Clock, Loader2,
    Server, Network, AlertTriangle, CheckCircle2, XCircle,
} from 'lucide-react';
import {
    agentsApi,
    alertsApi,
    authApi,
    type Agent,
    type Alert,
    type CommandListItem,
    type CommandRequest,
    type CommandType,
    type EndpointRiskSummary,
    type QuarantineItem,
} from '../api/client';
import { Modal, useToast } from '../components';

const STALE_THRESHOLD_MS = 60 * 1000;

function getEffectiveStatus(agent: Agent): Agent['status'] {
    if (agent.status === 'online' || agent.status === 'degraded') {
        const elapsed = Date.now() - new Date(agent.last_seen).getTime();
        if (elapsed > STALE_THRESHOLD_MS) return 'offline';
    }
    return agent.status;
}

type DetailTab = 'summary' | 'response' | 'quarantine' | 'activity' | 'network' | 'software';

const TAB_LABELS: { id: DetailTab; label: string }[] = [
    { id: 'summary', label: 'Summary' },
    { id: 'response', label: 'Response / Command Center' },
    { id: 'quarantine', label: 'Quarantine' },
    { id: 'activity', label: 'Activity / Timeline' },
    { id: 'network', label: 'Network' },
    { id: 'software', label: 'Software inventory' },
];

const RESPONSE_OPTIONS: { value: CommandType; label: string; destructive?: boolean }[] = [
    { value: 'kill_process', label: 'Kill / terminate process' },
    { value: 'quarantine_file', label: 'Quarantine file' },
    { value: 'isolate_network', label: 'Isolate network' },
    { value: 'restore_network', label: 'Restore network (un-isolate)' },
    { value: 'block_ip', label: 'Block IP' },
    { value: 'unblock_ip', label: 'Unblock IP' },
    { value: 'block_domain', label: 'Block domain' },
    { value: 'unblock_domain', label: 'Unblock domain' },
    { value: 'run_cmd', label: 'Run CMD (whitelisted)' },
    { value: 'collect_forensics', label: 'Collect forensics' },
    { value: 'update_signatures', label: 'Update signatures' },
    { value: 'update_config', label: 'Update config (hot reload)' },
    { value: 'update_filter_policy', label: 'Update filter policy (JSON)' },
    { value: 'restart_service', label: 'Restart agent service' },
    { value: 'stop_agent', label: 'Stop agent service' },
    { value: 'start_agent', label: 'Start agent service' },
    { value: 'restart_machine', label: 'Restart machine', destructive: true },
    { value: 'shutdown_machine', label: 'Shutdown machine', destructive: true },
];

function buildCommandParameters(cmd: CommandType, f: Record<string, string>): Record<string, string> {
    switch (cmd) {
        case 'kill_process':
        case 'terminate_process': {
            const p: Record<string, string> = {};
            if (f.pid?.trim()) p.pid = f.pid.trim();
            if (f.process_name?.trim()) p.process_name = f.process_name.trim();
            p.kill_tree = f.kill_tree === 'true' ? 'true' : 'false';
            return p;
        }
        case 'quarantine_file':
            return { path: (f.path || f.file_path || '').trim() };
        case 'block_ip':
        case 'unblock_ip': {
            const o: Record<string, string> = { ip: (f.ip || '').trim() };
            if (f.direction && cmd === 'block_ip') o.direction = f.direction;
            return o;
        }
        case 'block_domain':
        case 'unblock_domain':
            return { domain: (f.domain || '').trim() };
        case 'run_cmd':
            return { cmd: (f.cmd || '').trim() };
        case 'collect_forensics': {
            const o: Record<string, string> = {};
            const et = f.event_types?.trim() || f.log_types?.trim();
            if (et) {
                o.event_types = et;
                o.log_types = et;
            }
            if (f.max_events?.trim()) o.max_events = f.max_events.trim();
            if (f.file_path?.trim()) o.file_path = f.file_path.trim();
            return o;
        }
        case 'update_signatures': {
            const o: Record<string, string> = { url: (f.sig_url || '').trim() };
            if (f.checksum_sha256?.trim()) o.checksum_sha256 = f.checksum_sha256.trim();
            if (f.format?.trim()) o.format = f.format.trim();
            if (f.force === 'true') o.force = 'true';
            return o;
        }
        case 'update_config': {
            const k = (f.config_key || '').trim();
            const v = (f.config_value || '').trim();
            if (!k) return {};
            return { [k]: v };
        }
        case 'update_filter_policy':
            return { policy: f.policy_json?.trim() || '{}' };
        case 'restart_machine':
        case 'shutdown_machine':
            return { confirm: f.confirm === 'true' ? 'true' : 'false' };
        default:
            return {};
    }
}

function resultPreview(result: unknown): string {
    if (result == null) return '—';
    if (typeof result === 'string') return result.slice(0, 120) + (result.length > 120 ? '…' : '');
    try {
        const s = JSON.stringify(result);
        return s.length > 140 ? `${s.slice(0, 140)}…` : s;
    } catch {
        return String(result);
    }
}

export default function EndpointDetail() {
    const { agentId = '' } = useParams<{ agentId: string }>();
    const navigate = useNavigate();
    const queryClient = useQueryClient();
    const { showToast } = useToast();
    const [searchParams, setSearchParams] = useSearchParams();

    const tabFromUrl = searchParams.get('tab') as DetailTab | null;
    const [tab, setTab] = useState<DetailTab>(() =>
        tabFromUrl && TAB_LABELS.some((t) => t.id === tabFromUrl) ? tabFromUrl : 'summary'
    );

    useEffect(() => {
        const t = searchParams.get('tab') as DetailTab | null;
        if (t && TAB_LABELS.some((x) => x.id === t)) setTab(t);
    }, [searchParams]);

    const setTabAndUrl = useCallback(
        (next: DetailTab) => {
            setTab(next);
            setSearchParams((prev) => {
                const n = new URLSearchParams(prev);
                n.set('tab', next);
                return n;
            });
        },
        [setSearchParams]
    );

    const [cmdType, setCmdType] = useState<CommandType>('kill_process');
    const [fields, setFields] = useState<Record<string, string>>({
        kill_tree: 'false',
        timeout: '300',
        direction: 'both',
        confirm: 'false',
    });
    const [destructiveOpen, setDestructiveOpen] = useState(false);
    const [pendingDestructive, setPendingDestructive] = useState<'restart_machine' | 'shutdown_machine' | null>(null);

    const { data: agent, isLoading: agentLoading, error: agentError } = useQuery({
        queryKey: ['agent', agentId],
        queryFn: () => agentsApi.get(agentId),
        enabled: !!agentId,
    });

    const { data: riskPayload } = useQuery({
        queryKey: ['endpoint-risk'],
        queryFn: () => alertsApi.endpointRisk(),
        staleTime: 60_000,
    });

    const riskRow: EndpointRiskSummary | undefined = useMemo(() => {
        const rows = riskPayload?.data;
        if (!rows) return undefined;
        return rows.find((r) => r.agent_id === agentId);
    }, [riskPayload, agentId]);

    const { data: recentCmds } = useQuery({
        queryKey: ['agent-commands', agentId, 'summary'],
        queryFn: () => agentsApi.getCommands(agentId, { limit: 5 }),
        enabled: !!agentId && tab === 'summary',
    });

    const { data: cmdPage, isLoading: cmdsLoading } = useQuery({
        queryKey: ['agent-commands', agentId, 'response'],
        queryFn: () => agentsApi.getCommands(agentId, { limit: 100 }),
        enabled: !!agentId && tab === 'response',
    });

    const { data: quarantineData, isLoading: qLoading } = useQuery({
        queryKey: ['agent-quarantine', agentId],
        queryFn: () => agentsApi.listQuarantine(agentId, { include_resolved: true }),
        enabled: !!agentId && tab === 'quarantine',
    });

    const { data: eventsPayload } = useQuery({
        queryKey: ['agent-events', agentId],
        queryFn: () => agentsApi.getAgentEvents(agentId),
        enabled: !!agentId && tab === 'activity',
    });

    const { data: alertsForAgent, isLoading: alertsLoading } = useQuery({
        queryKey: ['sigma-alerts', agentId],
        queryFn: () => alertsApi.list({ agent_id: agentId, limit: 80, order: 'desc' }),
        enabled: !!agentId && tab === 'activity',
    });

    const execMutation = useMutation({
        mutationFn: (req: CommandRequest) => agentsApi.executeCommand(agentId, req),
        onSuccess: (data) => {
            showToast(`Command queued (${data.command_id})`, 'success');
            queryClient.invalidateQueries({ queryKey: ['agent-commands', agentId] });
            queryClient.invalidateQueries({ queryKey: ['commands'] });
        },
        onError: (e: Error) => showToast(e.message || 'Command failed', 'error'),
    });

    const qDecisionMutation = useMutation({
        mutationFn: ({ entryId, decision }: { entryId: string; decision: 'acknowledge' | 'restore' | 'delete' }) =>
            agentsApi.quarantineDecision(agentId, entryId, decision),
        onSuccess: (_, v) => {
            showToast(`Quarantine: ${v.decision}`, 'success');
            queryClient.invalidateQueries({ queryKey: ['agent-quarantine', agentId] });
            queryClient.invalidateQueries({ queryKey: ['agent-commands', agentId] });
        },
        onError: (e: Error) => showToast(e.message || 'Decision failed', 'error'),
    });

    const submitCommand = () => {
        const opt = RESPONSE_OPTIONS.find((o) => o.value === cmdType);
        if (opt?.destructive) {
            setPendingDestructive(cmdType as 'restart_machine' | 'shutdown_machine');
            setDestructiveOpen(true);
            return;
        }
        const timeout = Math.min(3600, Math.max(0, parseInt(fields.timeout || '300', 10) || 300));
        const parameters = buildCommandParameters(cmdType, fields);
        execMutation.mutate({
            command_type: cmdType,
            parameters,
            timeout,
        });
    };

    const confirmDestructive = () => {
        if (!pendingDestructive) return;
        const timeout = Math.min(3600, Math.max(0, parseInt(fields.timeout || '300', 10) || 300));
        execMutation.mutate({
            command_type: pendingDestructive,
            parameters: { confirm: 'true' },
            timeout,
        });
        setDestructiveOpen(false);
        setPendingDestructive(null);
    };

    const eff = agent ? getEffectiveStatus(agent) : 'offline';
    const canExec = authApi.canExecuteCommands();
    const canViewResp = authApi.canViewResponses();

    if (!agentId) {
        return (
            <div className="p-8 text-center text-slate-500">
                Invalid device ID. <Link className="text-primary-600" to="/management/devices">Back to devices</Link>
            </div>
        );
    }

    if (agentLoading) {
        return (
            <div className="flex items-center justify-center min-h-[40vh]">
                <Loader2 className="w-10 h-10 animate-spin text-cyan-500" />
            </div>
        );
    }

    if (agentError || !agent) {
        return (
            <div className="p-8 text-center">
                <AlertTriangle className="w-12 h-12 text-amber-500 mx-auto mb-3" />
                <p className="text-slate-700 dark:text-slate-200 mb-4">Could not load this endpoint.</p>
                <Link to="/management/devices" className="text-primary-600 hover:underline">
                    ← Back to Device Management
                </Link>
            </div>
        );
    }

    const lastCmd = recentCmds?.data?.[0];

    return (
        <div className="relative flex flex-col min-h-[calc(100vh-2rem)] -mx-4 sm:-mx-6 lg:-mx-8 -my-4 sm:-my-6 lg:-my-8 p-4 sm:p-6 lg:p-8 bg-slate-50 dark:bg-gradient-to-br dark:from-slate-900 dark:via-[#0b1120] dark:to-slate-900">
            <div className="max-w-[1600px] mx-auto w-full space-y-4">
                <div className="flex flex-wrap items-start gap-4">
                    <button
                        type="button"
                        onClick={() => navigate('/management/devices')}
                        className="inline-flex items-center gap-2 text-sm text-slate-600 dark:text-slate-300 hover:text-primary-600"
                    >
                        <ArrowLeft className="w-4 h-4" />
                        Devices
                    </button>
                </div>

                <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-4">
                    <div>
                        <h1 className="text-2xl font-bold text-slate-900 dark:text-white flex flex-wrap items-center gap-3">
                            <Server className="w-7 h-7 text-cyan-500 shrink-0" />
                            <span className="break-all">{agent.hostname}</span>
                        </h1>
                        <p className="text-sm text-slate-500 mt-1 font-mono break-all">{agent.id}</p>
                    </div>
                    <div className="flex flex-wrap gap-2">
                        <span
                            className={`px-3 py-1 rounded-full text-xs font-bold uppercase ${
                                eff === 'online'
                                    ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'
                                    : eff === 'offline'
                                      ? 'bg-slate-500/15 text-slate-600'
                                      : 'bg-amber-500/15 text-amber-700 dark:text-amber-300'
                            }`}
                        >
                            {eff}
                        </span>
                        {agent.is_isolated && (
                            <span className="px-3 py-1 rounded-full text-xs font-bold uppercase bg-rose-500/15 text-rose-600 dark:text-rose-400">
                                Isolated
                            </span>
                        )}
                        <Link
                            to={`/responses?agent_id=${encodeURIComponent(agent.id)}`}
                            className="px-3 py-1 rounded-lg text-xs font-semibold bg-cyan-500/10 text-cyan-700 dark:text-cyan-300 border border-cyan-500/20 hover:bg-cyan-500/20"
                        >
                            Action Center (filtered)
                        </Link>
                    </div>
                </div>

                <div className="flex flex-wrap gap-1 border-b border-slate-200 dark:border-slate-700 pb-px">
                    {TAB_LABELS.map(({ id, label }) => (
                        <button
                            key={id}
                            type="button"
                            onClick={() => setTabAndUrl(id)}
                            className={`px-3 py-2 text-sm font-medium rounded-t-lg transition-colors ${
                                tab === id
                                    ? 'bg-white dark:bg-slate-800 text-cyan-600 dark:text-cyan-400 border border-b-0 border-slate-200 dark:border-slate-700'
                                    : 'text-slate-500 hover:text-slate-800 dark:hover:text-slate-200'
                            }`}
                        >
                            {label}
                        </button>
                    ))}
                </div>

                <div className="bg-white/80 dark:bg-slate-900/50 border border-slate-200 dark:border-slate-700 rounded-xl p-4 sm:p-6 shadow-sm">
                    {tab === 'summary' && (
                        <SummaryTab
                            agent={agent}
                            eff={eff}
                            riskRow={riskRow}
                            recent={recentCmds?.data || []}
                            lastCmd={lastCmd}
                        />
                    )}

                    {tab === 'response' && canViewResp && (
                        <ResponseTab
                            agent={agent}
                            cmds={cmdPage?.data || []}
                            cmdsLoading={cmdsLoading}
                            cmdType={cmdType}
                            setCmdType={setCmdType}
                            fields={fields}
                            setFields={setFields}
                            canExec={canExec}
                            execMutation={execMutation}
                            onSubmit={submitCommand}
                        />
                    )}

                    {tab === 'response' && !canViewResp && (
                        <p className="text-slate-500">You do not have permission to view response commands.</p>
                    )}

                    {tab === 'quarantine' && canViewResp && (
                        <QuarantineTab
                            items={quarantineData?.items || []}
                            loading={qLoading}
                            canExec={canExec}
                            onDecision={(id, d) => qDecisionMutation.mutate({ entryId: id, decision: d })}
                            busy={qDecisionMutation.isPending}
                            online={eff === 'online' || eff === 'degraded'}
                        />
                    )}

                    {tab === 'quarantine' && !canViewResp && (
                        <p className="text-slate-500">You do not have permission to view quarantine.</p>
                    )}

                    {tab === 'activity' && (
                        <ActivityTab
                            events={eventsPayload?.data || []}
                            alerts={alertsForAgent?.alerts || []}
                            alertsLoading={alertsLoading}
                        />
                    )}

                    {tab === 'network' && (
                        <div className="text-sm text-slate-600 dark:text-slate-400 space-y-2">
                            <p className="font-medium text-slate-800 dark:text-slate-200">Network activity</p>
                            {/* TODO: Wire when events/search is connected to event store (filter event_type=network + agent_id). */}
                            <p>
                                Event-based network telemetry will appear here once <code className="text-xs">POST /api/v1/events/search</code> is wired to the event database.
                            </p>
                        </div>
                    )}

                    {tab === 'software' && (
                        <div className="text-sm text-slate-600 dark:text-slate-400 space-y-2">
                            <p className="font-medium text-slate-800 dark:text-slate-200">Software inventory</p>
                            {/* TODO: Needs new API endpoint GET /api/v1/agents/:id/software — not yet implemented */}
                            <p>No inventory API is available yet. This tab is reserved for a future agent telemetry feed.</p>
                        </div>
                    )}
                </div>
            </div>

            <Modal
                isOpen={destructiveOpen}
                onClose={() => { setDestructiveOpen(false); setPendingDestructive(null); }}
                title="Confirm destructive action"
                footer={
                    <div className="flex justify-end gap-2">
                        <button type="button" className="btn btn-secondary" onClick={() => { setDestructiveOpen(false); setPendingDestructive(null); }}>
                            Cancel
                        </button>
                        <button type="button" className="btn bg-rose-600 hover:bg-rose-700 text-white" onClick={confirmDestructive}>
                            Confirm
                        </button>
                    </div>
                }
            >
                <p className="text-sm text-slate-600 dark:text-slate-300">
                    You are about to send <strong>{pendingDestructive}</strong> to <strong>{agent.hostname}</strong>.
                    This can disrupt the user session. Continue?
                </p>
            </Modal>
        </div>
    );
}

function SummaryTab({
    agent,
    eff,
    riskRow,
    recent,
    lastCmd,
}: {
    agent: Agent;
    eff: string;
    riskRow?: EndpointRiskSummary;
    recent: CommandListItem[];
    lastCmd?: CommandListItem;
}) {
    return (
        <div className="space-y-6">
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
                <div className="rounded-lg border border-slate-200 dark:border-slate-700 p-4 bg-slate-50/80 dark:bg-slate-800/40">
                    <div className="text-xs font-semibold text-slate-500 uppercase">Status</div>
                    <div className="text-lg font-bold text-slate-900 dark:text-white mt-1">{eff}</div>
                </div>
                <div className="rounded-lg border border-slate-200 dark:border-slate-700 p-4 bg-slate-50/80 dark:bg-slate-800/40">
                    <div className="text-xs font-semibold text-slate-500 uppercase">Health</div>
                    <div className="text-lg font-bold text-slate-900 dark:text-white mt-1">{Math.round(agent.health_score ?? 0)}%</div>
                </div>
                <div className="rounded-lg border border-slate-200 dark:border-slate-700 p-4 bg-slate-50/80 dark:bg-slate-800/40">
                    <div className="text-xs font-semibold text-slate-500 uppercase">Open alerts (sigma)</div>
                    <div className="text-lg font-bold text-slate-900 dark:text-white mt-1">{riskRow?.open_count ?? '—'}</div>
                </div>
                <div className="rounded-lg border border-slate-200 dark:border-slate-700 p-4 bg-slate-50/80 dark:bg-slate-800/40">
                    <div className="text-xs font-semibold text-slate-500 uppercase">Isolation</div>
                    <div className="text-lg font-bold text-slate-900 dark:text-white mt-1">{agent.is_isolated ? 'Yes' : 'No'}</div>
                </div>
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                <div>
                    <h3 className="text-sm font-bold text-slate-700 dark:text-slate-200 mb-2 flex items-center gap-2">
                        <Activity className="w-4 h-4" /> Identity
                    </h3>
                    <dl className="text-sm space-y-1.5">
                        <div className="flex justify-between gap-2"><dt className="text-slate-500">OS</dt><dd className="text-right">{agent.os_type} {agent.os_version}</dd></div>
                        <div className="flex justify-between gap-2"><dt className="text-slate-500">Agent</dt><dd className="text-right">v{agent.agent_version || '—'}</dd></div>
                        <div className="flex justify-between gap-2"><dt className="text-slate-500">Last seen</dt><dd className="text-right">{new Date(agent.last_seen).toLocaleString()}</dd></div>
                        <div className="flex justify-between gap-2"><dt className="text-slate-500">IPs</dt><dd className="text-right break-all">{(agent.ip_addresses || []).join(', ') || '—'}</dd></div>
                    </dl>
                </div>
                <div>
                    <h3 className="text-sm font-bold text-slate-700 dark:text-slate-200 mb-2 flex items-center gap-2">
                        <Clock className="w-4 h-4" /> Last command
                    </h3>
                    {lastCmd ? (
                        <div className="text-sm space-y-1">
                            <div><span className="text-slate-500">Type:</span> <code>{lastCmd.command_type}</code></div>
                            <div><span className="text-slate-500">Status:</span> {lastCmd.status}</div>
                            <div><span className="text-slate-500">Issued:</span> {new Date(lastCmd.issued_at).toLocaleString()}</div>
                        </div>
                    ) : (
                        <p className="text-slate-500 text-sm">No commands yet.</p>
                    )}
                </div>
            </div>

            <div>
                <h3 className="text-sm font-bold text-slate-700 dark:text-slate-200 mb-2">Recent commands</h3>
                <div className="overflow-x-auto rounded-lg border border-slate-200 dark:border-slate-700">
                    <table className="w-full text-left text-xs">
                        <thead className="bg-slate-100 dark:bg-slate-800/80 text-slate-600 uppercase">
                            <tr>
                                <th className="p-2">Type</th>
                                <th className="p-2">Status</th>
                                <th className="p-2">Issued</th>
                            </tr>
                        </thead>
                        <tbody>
                            {recent.length === 0 ? (
                                <tr><td colSpan={3} className="p-4 text-slate-500">No commands</td></tr>
                            ) : (
                                recent.map((c) => (
                                    <tr key={c.id} className="border-t border-slate-100 dark:border-slate-800">
                                        <td className="p-2 font-mono">{c.command_type}</td>
                                        <td className="p-2">{c.status}</td>
                                        <td className="p-2 whitespace-nowrap">{new Date(c.issued_at).toLocaleString()}</td>
                                    </tr>
                                ))
                            )}
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    );
}

function ResponseTab({
    agent: _agent,
    cmds,
    cmdsLoading,
    cmdType,
    setCmdType,
    fields,
    setFields,
    canExec,
    execMutation,
    onSubmit,
}: {
    agent: Agent;
    cmds: CommandListItem[];
    cmdsLoading: boolean;
    cmdType: CommandType;
    setCmdType: (c: CommandType) => void;
    fields: Record<string, string>;
    setFields: React.Dispatch<React.SetStateAction<Record<string, string>>>;
    canExec: boolean;
    execMutation: { isPending: boolean };
    onSubmit: () => void;
}) {
    const patch = (k: string, v: string) => setFields((f) => ({ ...f, [k]: v }));

    return (
        <div className="space-y-8">
            <div>
                <h3 className="text-sm font-bold text-slate-800 dark:text-slate-100 mb-3 flex items-center gap-2">
                    <Terminal className="w-4 h-4" /> Execute command
                </h3>
                <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                    <div className="space-y-3">
                        <label className="block text-xs font-semibold text-slate-500 uppercase">Command</label>
                        <select
                            className="w-full rounded-lg border border-slate-200 dark:border-slate-600 bg-white dark:bg-slate-900 px-3 py-2 text-sm"
                            value={cmdType}
                            onChange={(e) => setCmdType(e.target.value as CommandType)}
                            disabled={!canExec}
                        >
                            {RESPONSE_OPTIONS.map((o) => (
                                <option key={o.value} value={o.value}>
                                    {o.label}
                                </option>
                            ))}
                        </select>
                        <label className="block text-xs font-semibold text-slate-500 uppercase">Timeout (seconds)</label>
                        <input
                            className="w-full rounded-lg border border-slate-200 dark:border-slate-600 bg-white dark:bg-slate-900 px-3 py-2 text-sm"
                            type="number"
                            min={0}
                            max={3600}
                            value={fields.timeout ?? '300'}
                            onChange={(e) => patch('timeout', e.target.value)}
                            disabled={!canExec}
                        />

                        {(cmdType === 'kill_process' || cmdType === 'terminate_process') && (
                            <>
                                <label className="text-xs text-slate-500">PID</label>
                                <input className="input w-full" value={fields.pid || ''} onChange={(e) => patch('pid', e.target.value)} disabled={!canExec} />
                                <label className="text-xs text-slate-500">Process name (optional)</label>
                                <input className="input w-full" value={fields.process_name || ''} onChange={(e) => patch('process_name', e.target.value)} disabled={!canExec} />
                                <label className="text-xs text-slate-500">Kill tree</label>
                                <select className="input w-full" value={fields.kill_tree || 'false'} onChange={(e) => patch('kill_tree', e.target.value)} disabled={!canExec}>
                                    <option value="false">false</option>
                                    <option value="true">true</option>
                                </select>
                            </>
                        )}

                        {cmdType === 'quarantine_file' && (
                            <>
                                <label className="text-xs text-slate-500">File path</label>
                                <input className="input w-full font-mono text-xs" placeholder="C:\path\to\file" value={fields.path || ''} onChange={(e) => patch('path', e.target.value)} disabled={!canExec} />
                            </>
                        )}

                        {(cmdType === 'isolate_network' || cmdType === 'restore_network') && (
                            <p className="text-xs text-slate-500">No parameters required. C2 allowlist is injected server-side for isolation.</p>
                        )}

                        {(cmdType === 'block_ip' || cmdType === 'unblock_ip') && (
                            <>
                                <label className="text-xs text-slate-500">IP</label>
                                <input className="input w-full" value={fields.ip || ''} onChange={(e) => patch('ip', e.target.value)} disabled={!canExec} />
                                {cmdType === 'block_ip' && (
                                    <>
                                        <label className="text-xs text-slate-500">Direction</label>
                                        <select className="input w-full" value={fields.direction || 'both'} onChange={(e) => patch('direction', e.target.value)} disabled={!canExec}>
                                            <option value="both">both</option>
                                            <option value="in">in</option>
                                            <option value="out">out</option>
                                        </select>
                                    </>
                                )}
                            </>
                        )}

                        {(cmdType === 'block_domain' || cmdType === 'unblock_domain') && (
                            <>
                                <label className="text-xs text-slate-500">Domain</label>
                                <input className="input w-full" value={fields.domain || ''} onChange={(e) => patch('domain', e.target.value)} disabled={!canExec} />
                            </>
                        )}

                        {cmdType === 'run_cmd' && (
                            <>
                                <label className="text-xs text-slate-500">Command (whitelisted)</label>
                                <input className="input w-full font-mono text-xs" placeholder="hostname" value={fields.cmd || ''} onChange={(e) => patch('cmd', e.target.value)} disabled={!canExec} />
                            </>
                        )}

                        {cmdType === 'collect_forensics' && (
                            <>
                                <label className="text-xs text-slate-500">Event types (comma-separated)</label>
                                <input className="input w-full text-xs" placeholder="Security,System,Application" value={fields.event_types || ''} onChange={(e) => patch('event_types', e.target.value)} disabled={!canExec} />
                                <label className="text-xs text-slate-500">Max events</label>
                                <input className="input w-full" value={fields.max_events || '100'} onChange={(e) => patch('max_events', e.target.value)} disabled={!canExec} />
                                <label className="text-xs text-slate-500">File path (optional)</label>
                                <input className="input w-full font-mono text-xs" value={fields.file_path || ''} onChange={(e) => patch('file_path', e.target.value)} disabled={!canExec} />
                            </>
                        )}

                        {cmdType === 'update_signatures' && (
                            <>
                                <label className="text-xs text-slate-500">Feed URL</label>
                                <input className="input w-full text-xs" value={fields.sig_url || ''} onChange={(e) => patch('sig_url', e.target.value)} disabled={!canExec} />
                                <label className="text-xs text-slate-500">SHA256 checksum (optional)</label>
                                <input className="input w-full font-mono text-xs" value={fields.checksum_sha256 || ''} onChange={(e) => patch('checksum_sha256', e.target.value)} disabled={!canExec} />
                                <label className="text-xs text-slate-500">Format (optional)</label>
                                <input className="input w-full text-xs" placeholder="csv | ndjson" value={fields.format || ''} onChange={(e) => patch('format', e.target.value)} disabled={!canExec} />
                                <label className="flex items-center gap-2 text-xs text-slate-500">
                                    <input type="checkbox" checked={fields.force === 'true'} onChange={(e) => patch('force', e.target.checked ? 'true' : 'false')} disabled={!canExec} />
                                    Force
                                </label>
                            </>
                        )}

                        {cmdType === 'update_config' && (
                            <>
                                <label className="text-xs text-slate-500">Key (dot path)</label>
                                <input className="input w-full font-mono text-xs" placeholder="collectors.etw_enabled" value={fields.config_key || ''} onChange={(e) => patch('config_key', e.target.value)} disabled={!canExec} />
                                <label className="text-xs text-slate-500">Value</label>
                                <input className="input w-full text-xs" placeholder="false" value={fields.config_value || ''} onChange={(e) => patch('config_value', e.target.value)} disabled={!canExec} />
                            </>
                        )}

                        {cmdType === 'update_filter_policy' && (
                            <>
                                <label className="text-xs text-slate-500">Policy JSON</label>
                                <textarea
                                    className="w-full min-h-[120px] font-mono text-xs rounded-lg border border-slate-200 dark:border-slate-600 bg-white dark:bg-slate-900 p-2"
                                    value={fields.policy_json || '{\n  "exclude_processes": []\n}'}
                                    onChange={(e) => patch('policy_json', e.target.value)}
                                    disabled={!canExec}
                                />
                            </>
                        )}

                        {(cmdType === 'restart_machine' || cmdType === 'shutdown_machine') && (
                            <p className="text-xs text-amber-600 dark:text-amber-400">You will be asked to confirm — these actions affect the whole host.</p>
                        )}

                        <button
                            type="button"
                            className="btn btn-primary w-full sm:w-auto"
                            disabled={!canExec || execMutation.isPending}
                            onClick={onSubmit}
                        >
                            {execMutation.isPending ? <Loader2 className="w-4 h-4 animate-spin inline mr-2" /> : null}
                            Send command
                        </button>
                        {!canExec && <p className="text-xs text-rose-500">Your role cannot execute remote commands.</p>}
                    </div>
                </div>
            </div>

            <div>
                <h3 className="text-sm font-bold text-slate-800 dark:text-slate-100 mb-2">Command history</h3>
                {cmdsLoading ? (
                    <Loader2 className="w-8 h-8 animate-spin text-cyan-500" />
                ) : (
                    <div className="overflow-x-auto rounded-lg border border-slate-200 dark:border-slate-700">
                        <table className="w-full text-left text-xs">
                            <thead className="bg-slate-100 dark:bg-slate-800/80 text-slate-600 uppercase">
                                <tr>
                                    <th className="p-2">Status</th>
                                    <th className="p-2">Type</th>
                                    <th className="p-2">Issued</th>
                                    <th className="p-2">By</th>
                                    <th className="p-2">Output / error</th>
                                </tr>
                            </thead>
                            <tbody>
                                {cmds.length === 0 ? (
                                    <tr><td colSpan={5} className="p-4 text-slate-500">No commands</td></tr>
                                ) : (
                                    cmds.map((c) => (
                                        <tr key={c.id} className="border-t border-slate-100 dark:border-slate-800 align-top">
                                            <td className="p-2 whitespace-nowrap">{c.status}</td>
                                            <td className="p-2 font-mono">{c.command_type}</td>
                                            <td className="p-2 whitespace-nowrap">{new Date(c.issued_at).toLocaleString()}</td>
                                            <td className="p-2 max-w-[140px] break-all">{c.issued_by_user || c.issued_by || '—'}</td>
                                            <td className="p-2 max-w-md">
                                                {c.error_message ? (
                                                    <span className="text-rose-600 dark:text-rose-400">{c.error_message}</span>
                                                ) : (
                                                    <span className="text-slate-600 dark:text-slate-300">{resultPreview(c.result)}</span>
                                                )}
                                            </td>
                                        </tr>
                                    ))
                                )}
                            </tbody>
                        </table>
                    </div>
                )}
            </div>
        </div>
    );
}

function QuarantineTab({
    items,
    loading,
    canExec,
    onDecision,
    busy,
    online,
}: {
    items: QuarantineItem[];
    loading: boolean;
    canExec: boolean;
    onDecision: (id: string, d: 'acknowledge' | 'restore' | 'delete') => void;
    busy: boolean;
    online: boolean;
}) {
    return (
        <div className="space-y-4">
            <div className="flex items-center gap-2 text-slate-600 dark:text-slate-400 text-sm">
                <HardDrive className="w-4 h-4" />
                Files recorded in server inventory (telemetry + manual C2 quarantine).
            </div>
            {loading ? <Loader2 className="w-8 h-8 animate-spin text-cyan-500" /> : (
                <div className="overflow-x-auto rounded-lg border border-slate-200 dark:border-slate-700">
                    <table className="w-full text-left text-xs">
                        <thead className="bg-slate-100 dark:bg-slate-800/80 text-slate-600 uppercase">
                            <tr>
                                <th className="p-2">Original</th>
                                <th className="p-2">Quarantine</th>
                                <th className="p-2">Threat</th>
                                <th className="p-2">State</th>
                                <th className="p-2">Updated</th>
                                <th className="p-2">Actions</th>
                            </tr>
                        </thead>
                        <tbody>
                            {items.length === 0 ? (
                                <tr><td colSpan={6} className="p-4 text-slate-500">No quarantine items</td></tr>
                            ) : (
                                items.map((q) => (
                                    <tr key={q.id} className="border-t border-slate-100 dark:border-slate-800 align-top">
                                        <td className="p-2 font-mono break-all max-w-[200px]">{q.original_path}</td>
                                        <td className="p-2 font-mono break-all max-w-[200px]">{q.quarantine_path}</td>
                                        <td className="p-2">{q.threat_name || '—'}</td>
                                        <td className="p-2">{q.state}</td>
                                        <td className="p-2 whitespace-nowrap">{new Date(q.updated_at).toLocaleString()}</td>
                                        <td className="p-2">
                                            <div className="flex flex-wrap gap-1">
                                                <button
                                                    type="button"
                                                    className="px-2 py-1 rounded border border-slate-200 dark:border-slate-600 text-[10px] font-semibold hover:bg-slate-100 dark:hover:bg-slate-800 disabled:opacity-50"
                                                    disabled={!canExec || busy || q.state !== 'quarantined'}
                                                    onClick={() => onDecision(q.id, 'acknowledge')}
                                                >
                                                    Ack
                                                </button>
                                                <button
                                                    type="button"
                                                    className="px-2 py-1 rounded border border-emerald-500/40 text-[10px] font-semibold text-emerald-700 dark:text-emerald-400 hover:bg-emerald-500/10 disabled:opacity-50"
                                                    disabled={!canExec || busy || !online || (q.state !== 'quarantined' && q.state !== 'acknowledged')}
                                                    title={!online ? 'Agent must be online' : undefined}
                                                    onClick={() => onDecision(q.id, 'restore')}
                                                >
                                                    Restore
                                                </button>
                                                <button
                                                    type="button"
                                                    className="px-2 py-1 rounded border border-rose-500/40 text-[10px] font-semibold text-rose-700 dark:text-rose-400 hover:bg-rose-500/10 disabled:opacity-50"
                                                    disabled={!canExec || busy || !online || (q.state !== 'quarantined' && q.state !== 'acknowledged')}
                                                    title={!online ? 'Agent must be online' : undefined}
                                                    onClick={() => onDecision(q.id, 'delete')}
                                                >
                                                    Delete
                                                </button>
                                            </div>
                                        </td>
                                    </tr>
                                ))
                            )}
                        </tbody>
                    </table>
                </div>
            )}
        </div>
    );
}

function ActivityTab({
    events,
    alerts,
    alertsLoading,
}: {
    events: unknown[];
    alerts: Alert[];
    alertsLoading: boolean;
}) {
    const merged = useMemo(() => {
        const rows: { t: 'alert'; at: number; label: string; severity?: string; id: string }[] = [];
        for (const a of alerts) {
            const ts = new Date(a.timestamp || a.updated_at || a.created_at).getTime();
            rows.push({
                t: 'alert',
                at: Number.isFinite(ts) ? ts : 0,
                label: a.rule_title || a.rule_id || 'Alert',
                severity: a.severity,
                id: a.id,
            });
        }
        rows.sort((a, b) => b.at - a.at);
        return rows;
    }, [alerts]);

    return (
        <div className="space-y-6">
            <div>
                <h3 className="text-sm font-bold text-slate-800 dark:text-slate-100 mb-2 flex items-center gap-2">
                    <Network className="w-4 h-4" /> Endpoint events (connection-manager)
                </h3>
                {events.length === 0 ? (
                    <p className="text-sm text-slate-500">
                        Event telemetry view coming soon — <code className="text-xs">GET /api/v1/agents/:id/events</code> is not wired to the event store yet.
                    </p>
                ) : (
                    <pre className="text-xs bg-slate-100 dark:bg-slate-900 p-3 rounded-lg overflow-auto max-h-48">{JSON.stringify(events, null, 2)}</pre>
                )}
            </div>

            <div>
                <h3 className="text-sm font-bold text-slate-800 dark:text-slate-100 mb-2 flex items-center gap-2">
                    <Shield className="w-4 h-4" /> Sigma alerts (this device)
                </h3>
                {alertsLoading ? <Loader2 className="w-6 h-6 animate-spin text-cyan-500" /> : merged.length === 0 ? (
                    <p className="text-sm text-slate-500">No alerts for this agent in the current window.</p>
                ) : (
                    <ul className="space-y-2">
                        {merged.map((r) => (
                            <li key={r.id} className="flex items-start gap-3 text-sm border border-slate-200 dark:border-slate-700 rounded-lg p-3">
                                {r.severity === 'critical' || r.severity === 'high' ? (
                                    <XCircle className="w-4 h-4 text-rose-500 shrink-0 mt-0.5" />
                                ) : (
                                    <CheckCircle2 className="w-4 h-4 text-slate-400 shrink-0 mt-0.5" />
                                )}
                                <div>
                                    <div className="font-medium text-slate-900 dark:text-slate-100">{r.label}</div>
                                    <div className="text-xs text-slate-500">{new Date(r.at).toLocaleString()} · {r.severity || '—'}</div>
                                    <Link className="text-xs text-cyan-600 hover:underline mt-1 inline-block" to="/alerts">
                                        Open Alerts
                                    </Link>
                                </div>
                            </li>
                        ))}
                    </ul>
                )}
            </div>
        </div>
    );
}
