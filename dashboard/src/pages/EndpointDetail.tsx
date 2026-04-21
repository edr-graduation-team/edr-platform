import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import {
    ArrowLeft, Activity, Terminal, Shield, HardDrive, Clock, Loader2,
    Server, Network, AlertTriangle, CheckCircle2, XCircle, Settings,
} from 'lucide-react';
import {
    agentsApi,
    alertsApi,
    authApi,
    eventsApi,
    type Agent,
    type Alert,
    type CmEventSummary,
    type CommandListItem,
    type CommandRequest,
    type CommandType,
    type EndpointRiskSummary,
    type QuarantineItem,
} from '../api/client';
import { AgentDeepDivePanel, EventDetailModal, Modal, useToast } from '../components';
import { getEffectiveStatus } from '../utils/agentDisplay';

type DetailTab =
    | 'overview'
    | 'response'
    | 'quarantine'
    | 'activity'
    | 'network'
    | 'configuration'
    | 'software';

const TAB_LABELS: { id: DetailTab; label: string }[] = [
    { id: 'overview', label: 'Overview' },
    { id: 'response', label: 'Response' },
    { id: 'quarantine', label: 'Quarantine' },
    { id: 'activity', label: 'Activity' },
    { id: 'network', label: 'Network' },
    { id: 'configuration', label: 'Configuration' },
    { id: 'software', label: 'Software' },
];

/** Legacy `?tab=summary` from earlier builds → Overview. */
const LEGACY_TAB_MAP: Record<string, DetailTab> = { summary: 'overview' };

function normalizeTab(raw: string | null): DetailTab | null {
    if (!raw) return null;
    if (LEGACY_TAB_MAP[raw]) return LEGACY_TAB_MAP[raw];
    if (TAB_LABELS.some((t) => t.id === (raw as DetailTab))) return raw as DetailTab;
    return null;
}

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

    const [tab, setTab] = useState<DetailTab>(() => {
        const n = normalizeTab(searchParams.get('tab'));
        return n ?? 'overview';
    });

    useEffect(() => {
        const n = normalizeTab(searchParams.get('tab'));
        if (n) setTab(n);
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

    const canViewAlerts = authApi.canViewAlerts();

    const { data: recentCmds } = useQuery({
        queryKey: ['agent-commands', agentId, 'overview'],
        queryFn: () => agentsApi.getCommands(agentId, { limit: 5 }),
        enabled: !!agentId && tab === 'overview',
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
        enabled: !!agentId && (tab === 'activity' || tab === 'overview'),
    });

    const { data: overviewAlerts, isLoading: overviewAlertsLoading } = useQuery({
        queryKey: ['sigma-alerts-overview', agentId],
        queryFn: () => alertsApi.list({ agent_id: agentId, limit: 6, order: 'desc' }),
        enabled: !!agentId && tab === 'overview' && canViewAlerts,
    });

    const { data: alertsForAgent, isLoading: alertsLoading } = useQuery({
        queryKey: ['sigma-alerts', agentId],
        queryFn: () => alertsApi.list({ agent_id: agentId, limit: 80, order: 'desc' }),
        enabled: !!agentId && tab === 'activity' && canViewAlerts,
    });

    const { data: networkSearch, isLoading: networkSearchLoading } = useQuery({
        queryKey: ['events-search-network', agentId],
        queryFn: () =>
            eventsApi.search({
                filters: [
                    { field: 'agent_id', operator: 'equals', value: agentId },
                    { field: 'event_type', operator: 'equals', value: 'network' },
                ],
                logic: 'AND',
                time_range: {
                    from: new Date(Date.now() - 30 * 24 * 60 * 60 * 1000).toISOString(),
                    to: new Date().toISOString(),
                },
                limit: 50,
                offset: 0,
            }),
        enabled: !!agentId && tab === 'network' && canViewAlerts,
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
                        <Link
                            to={`/events?agent_id=${encodeURIComponent(agent.id)}`}
                            className="px-3 py-1 rounded-lg text-xs font-semibold bg-slate-500/10 text-slate-700 dark:text-slate-300 border border-slate-500/20 hover:bg-slate-500/20"
                        >
                            Events (filtered)
                        </Link>
                    </div>
                </div>

                <AgentDeepDivePanel agent={agent} />

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
                    {tab === 'overview' && (
                        <OverviewTab
                            agent={agent}
                            eff={eff}
                            riskRow={riskRow}
                            recent={recentCmds?.data || []}
                            lastCmd={lastCmd}
                            overviewAlerts={overviewAlerts?.alerts ?? []}
                            overviewAlertsLoading={overviewAlertsLoading}
                            cmEvents={eventsPayload?.data || []}
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
                            canViewAlerts={canViewAlerts}
                        />
                    )}

                    {tab === 'network' &&
                        (canViewAlerts ? (
                            <NetworkTab
                                agentId={agentId}
                                data={networkSearch?.data}
                                loading={networkSearchLoading}
                                canViewAlerts={canViewAlerts}
                            />
                        ) : (
                            <p className="text-sm text-slate-500">
                                Event search requires <code className="text-xs">alerts:read</code> (same guard as{' '}
                                <code className="text-xs">POST /api/v1/events/search</code> on the server).
                            </p>
                        ))}

                    {tab === 'configuration' && <ConfigurationTab agent={agent} />}

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

function OverviewTab({
    agent,
    eff,
    riskRow,
    recent,
    lastCmd,
    overviewAlerts,
    overviewAlertsLoading,
    cmEvents,
}: {
    agent: Agent;
    eff: string;
    riskRow?: EndpointRiskSummary;
    recent: CommandListItem[];
    lastCmd?: CommandListItem;
    overviewAlerts: Alert[];
    overviewAlertsLoading: boolean;
    cmEvents: CmEventSummary[];
}) {
    const tagEntries = Object.entries(agent.tags || {});

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
                        <div className="flex justify-between gap-2"><dt className="text-slate-500">Tags</dt><dd className="text-right break-all">{tagEntries.length ? tagEntries.map(([k, v]) => `${k}=${v}`).join(', ') : '—'}</dd></div>
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
                <h3 className="text-sm font-bold text-slate-700 dark:text-slate-200 mb-2 flex items-center gap-2">
                    <Shield className="w-4 h-4" /> Recent Sigma alerts
                </h3>
                {overviewAlertsLoading ? (
                    <Loader2 className="w-6 h-6 animate-spin text-cyan-500" />
                ) : overviewAlerts.length === 0 ? (
                    <p className="text-sm text-slate-500">No recent alerts for this device (or you lack alerts access).</p>
                ) : (
                    <ul className="space-y-2">
                        {overviewAlerts.map((a) => (
                            <li key={a.id} className="text-sm border border-slate-200 dark:border-slate-700 rounded-lg p-2 flex justify-between gap-2">
                                <span className="font-medium text-slate-900 dark:text-slate-100 truncate">{a.rule_title || a.rule_id || 'Alert'}</span>
                                <span className="text-xs text-slate-500 shrink-0">{a.severity || '—'}</span>
                            </li>
                        ))}
                    </ul>
                )}
            </div>

            <div>
                <h3 className="text-sm font-bold text-slate-700 dark:text-slate-200 mb-2 flex items-center gap-2">
                    <Network className="w-4 h-4" /> Telemetry snapshot (connection-manager)
                </h3>
                {cmEvents.length === 0 ? (
                    <p className="text-sm text-slate-500">
                        No raw events in the latest fetch — confirm the agent is ingesting and that <code className="text-xs">GET /api/v1/agents/:id/events</code> returns rows. Open the full Events view for filters and raw JSON per event.
                    </p>
                ) : (
                    <ul className="text-sm space-y-1">
                        {cmEvents.slice(0, 5).map((e) => (
                            <li key={e.id} className="flex flex-wrap gap-x-2 border border-slate-100 dark:border-slate-800 rounded px-2 py-1">
                                <span className="font-mono text-xs text-cyan-700 dark:text-cyan-300">{e.event_type}</span>
                                <span className="text-slate-600 dark:text-slate-400">{e.summary}</span>
                                <span className="text-xs text-slate-400 ml-auto">{new Date(e.timestamp).toLocaleString()}</span>
                            </li>
                        ))}
                    </ul>
                )}
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

function ConfigurationTab({ agent }: { agent: Agent }) {
    const meta = agent.metadata || {};
    const filterPolicy = meta.filter_policy_json || meta.filter_policy;

    return (
        <div className="space-y-6 text-sm">
            <p className="text-slate-600 dark:text-slate-400">
                Read-only view of labels and metadata returned by <code className="text-xs">GET /api/v1/agents/:id</code>. Full policy editing belongs on the Response tab (e.g. update_filter_policy).
            </p>
            <div>
                <h3 className="text-sm font-bold text-slate-800 dark:text-slate-100 mb-2 flex items-center gap-2">
                    <Settings className="w-4 h-4" /> Tags
                </h3>
                {Object.keys(agent.tags || {}).length === 0 ? (
                    <p className="text-slate-500">No tags.</p>
                ) : (
                    <pre className="text-xs bg-slate-100 dark:bg-slate-900 p-3 rounded-lg overflow-auto max-h-40">
                        {JSON.stringify(agent.tags, null, 2)}
                    </pre>
                )}
            </div>
            <div>
                <h3 className="text-sm font-bold text-slate-800 dark:text-slate-100 mb-2">Metadata</h3>
                {Object.keys(meta).length === 0 ? (
                    <p className="text-slate-500">No metadata.</p>
                ) : (
                    <pre className="text-xs bg-slate-100 dark:bg-slate-900 p-3 rounded-lg overflow-auto max-h-64">
                        {JSON.stringify(meta, null, 2)}
                    </pre>
                )}
            </div>
            {filterPolicy && (
                <div>
                    <h3 className="text-sm font-bold text-slate-800 dark:text-slate-100 mb-2">Filter policy hint</h3>
                    <p className="text-xs text-slate-500 mb-1">If the agent stores a serialized policy in metadata, it is shown here for review.</p>
                    <pre className="text-xs bg-slate-100 dark:bg-slate-900 p-3 rounded-lg overflow-auto max-h-48 whitespace-pre-wrap break-all">
                        {typeof filterPolicy === 'string' ? filterPolicy : JSON.stringify(filterPolicy, null, 2)}
                    </pre>
                </div>
            )}
        </div>
    );
}

function NetworkTab({
    agentId: _agentId,
    data,
    loading,
    canViewAlerts,
}: {
    agentId: string;
    data: CmEventSummary[] | undefined;
    loading: boolean;
    canViewAlerts: boolean;
}) {
    const [detailId, setDetailId] = useState<string | null>(null);

    return (
        <div className="space-y-4 text-sm">
            <p className="text-slate-600 dark:text-slate-400">
                Network-related events from <code className="text-xs">POST /api/v1/events/search</code> (filters:{' '}
                <code className="text-xs">agent_id</code> + <code className="text-xs">event_type=network</code>). Click a row for raw JSON.
            </p>
            {loading ? (
                <Loader2 className="w-8 h-8 animate-spin text-cyan-500" />
            ) : !data || data.length === 0 ? (
                <div className="rounded-lg border border-dashed border-slate-300 dark:border-slate-600 p-4 text-slate-500">
                    No network events in this window. Widen the time range on the Events page or generate telemetry; if search stays empty, verify nginx proxies <code className="text-xs">/api/v1/events/</code> to connection-manager.
                </div>
            ) : (
                <div className="overflow-x-auto rounded-lg border border-slate-200 dark:border-slate-700">
                    <table className="w-full text-left text-xs">
                        <thead className="bg-slate-100 dark:bg-slate-800/80 text-slate-600 uppercase">
                            <tr>
                                <th className="p-2">Time</th>
                                <th className="p-2">Type</th>
                                <th className="p-2">Summary</th>
                            </tr>
                        </thead>
                        <tbody>
                            {data.map((e) => (
                                <tr
                                    key={e.id}
                                    role="button"
                                    tabIndex={0}
                                    className="border-t border-slate-100 dark:border-slate-800 cursor-pointer hover:bg-slate-50/90 dark:hover:bg-slate-800/40"
                                    onClick={() => setDetailId(e.id)}
                                    onKeyDown={(ev) => {
                                        if (ev.key === 'Enter' || ev.key === ' ') {
                                            ev.preventDefault();
                                            setDetailId(e.id);
                                        }
                                    }}
                                >
                                    <td className="p-2 whitespace-nowrap">{new Date(e.timestamp).toLocaleString()}</td>
                                    <td className="p-2 font-mono">{e.event_type}</td>
                                    <td className="p-2">{e.summary}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}

            <EventDetailModal
                eventId={detailId}
                onClose={() => setDetailId(null)}
                fetchEnabled={canViewAlerts}
            />
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

type TimelineRow =
    | { kind: 'alert'; at: number; id: string; alert: Alert }
    | { kind: 'event'; at: number; id: string; ev: CmEventSummary };

function ActivityTab({
    events,
    alerts,
    alertsLoading,
    canViewAlerts,
}: {
    events: CmEventSummary[];
    alerts: Alert[];
    alertsLoading: boolean;
    canViewAlerts: boolean;
}) {
    const [detailId, setDetailId] = useState<string | null>(null);

    const merged = useMemo(() => {
        const rows: TimelineRow[] = [];
        if (canViewAlerts) {
            for (const a of alerts) {
                const ts = new Date(a.timestamp || a.updated_at || a.created_at).getTime();
                rows.push({
                    kind: 'alert',
                    at: Number.isFinite(ts) ? ts : 0,
                    id: `alert-${a.id}`,
                    alert: a,
                });
            }
        }
        for (const e of events) {
            const ts = new Date(e.timestamp).getTime();
            rows.push({ kind: 'event', at: Number.isFinite(ts) ? ts : 0, id: `ev-${e.id}`, ev: e });
        }
        rows.sort((a, b) => b.at - a.at);
        return rows;
    }, [alerts, events, canViewAlerts]);

    return (
        <div className="space-y-6">
            <div>
                <h3 className="text-sm font-bold text-slate-800 dark:text-slate-100 mb-2 flex items-center gap-2">
                    <Activity className="w-4 h-4" /> Timeline (Sigma + connection-manager events)
                </h3>
                {!canViewAlerts && (
                    <p className="text-sm text-amber-700 dark:text-amber-300 mb-2">
                        Sigma alerts are hidden — your role does not include alerts read. Connection-manager events still appear below when available.
                    </p>
                )}
                {alertsLoading && canViewAlerts ? (
                    <Loader2 className="w-6 h-6 animate-spin text-cyan-500" />
                ) : merged.length === 0 ? (
                    <p className="text-sm text-slate-500">
                        No timeline entries yet. Alerts need Sigma data; raw events need <code className="text-xs">GET /api/v1/agents/:id/events</code> populated on the server.
                    </p>
                ) : (
                    <ul className="space-y-2">
                        {merged.map((r) =>
                            r.kind === 'alert' ? (
                                <li
                                    key={r.id}
                                    className="flex items-start gap-3 text-sm border border-slate-200 dark:border-slate-700 rounded-lg p-3"
                                >
                                    {r.alert.severity === 'critical' || r.alert.severity === 'high' ? (
                                        <XCircle className="w-4 h-4 text-rose-500 shrink-0 mt-0.5" />
                                    ) : (
                                        <CheckCircle2 className="w-4 h-4 text-slate-400 shrink-0 mt-0.5" />
                                    )}
                                    <div>
                                        <div className="font-medium text-slate-900 dark:text-slate-100">
                                            {r.alert.rule_title || r.alert.rule_id || 'Alert'}
                                        </div>
                                        <div className="text-xs text-slate-500">
                                            {new Date(r.at).toLocaleString()} · Sigma · {r.alert.severity || '—'}
                                        </div>
                                        <Link className="text-xs text-cyan-600 hover:underline mt-1 inline-block" to="/alerts">
                                            Open Alerts
                                        </Link>
                                    </div>
                                </li>
                            ) : (
                                <li
                                    key={r.id}
                                    role="button"
                                    tabIndex={0}
                                    className="flex items-start gap-3 text-sm border border-slate-200 dark:border-slate-700 rounded-lg p-3 cursor-pointer hover:bg-slate-50/90 dark:hover:bg-slate-800/50 transition-colors"
                                    onClick={() => setDetailId(r.ev.id)}
                                    onKeyDown={(ev) => {
                                        if (ev.key === 'Enter' || ev.key === ' ') {
                                            ev.preventDefault();
                                            setDetailId(r.ev.id);
                                        }
                                    }}
                                >
                                    <Network className="w-4 h-4 text-cyan-500 shrink-0 mt-0.5" />
                                    <div className="min-w-0 flex-1">
                                        <div className="font-mono text-xs text-cyan-700 dark:text-cyan-300">{r.ev.event_type}</div>
                                        <div className="text-slate-800 dark:text-slate-100">{r.ev.summary}</div>
                                        <div className="text-xs text-slate-500">{new Date(r.at).toLocaleString()} · CM event · click for raw JSON</div>
                                    </div>
                                </li>
                            )
                        )}
                    </ul>
                )}
            </div>

            <EventDetailModal
                eventId={detailId}
                onClose={() => setDetailId(null)}
                fetchEnabled={canViewAlerts}
            />
        </div>
    );
}
