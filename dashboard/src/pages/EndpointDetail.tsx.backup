import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import {
    ArrowLeft, Activity, Terminal, Shield, HardDrive, Loader2,
    Server, Network, AlertTriangle, CheckCircle2, XCircle, Settings,
    RefreshCw, ChevronLeft, ChevronRight,
} from 'lucide-react';
import {
    agentBuildApi,
    agentPackagesApi,
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
    type EnrollmentToken,
    type EndpointRiskSummary,
    type QuarantineItem,
    type ForensicCollection,
    type ForensicEvent,
} from '../api/client';
import { EventDetailModal, Modal, useToast } from '../components';
import { formatRelativeTime, getEffectiveStatus } from '../utils/agentDisplay';
import { IncidentTab } from './IncidentTab';

type DetailTab =
    | 'overview'
    | 'incident'
    | 'response'
    | 'quarantine'
    | 'activity'
    | 'forensics'
    | 'network'
    | 'configuration'
    | 'software';

const TAB_LABELS: { id: DetailTab; label: string }[] = [
    { id: 'overview', label: 'Overview' },
    { id: 'incident', label: '🔴 Incident' },
    { id: 'response', label: 'Response' },
    { id: 'quarantine', label: 'Quarantine' },
    { id: 'activity', label: 'Activity' },
    { id: 'forensics', label: 'Forensic Logs' },
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
    { value: 'custom', label: 'Custom command (whitelisted)' },
    { value: 'collect_logs', label: 'Collect logs' },
    { value: 'collect_forensics', label: 'Collect forensics' },
    { value: 'scan_memory', label: 'Scan memory (file hash)' },
    { value: 'update_signatures', label: 'Update signatures' },
    { value: 'update_config', label: 'Update config (hot reload)' },
    { value: 'update_filter_policy', label: 'Update filter policy (JSON)' },
    { value: 'restart_agent', label: 'Restart agent' },
    { value: 'restart_service', label: 'Restart agent service' },
    { value: 'stop_agent', label: 'Stop agent service' },
    { value: 'start_agent', label: 'Start agent service' },
    { value: 'restart_machine', label: 'Restart machine', destructive: true },
    { value: 'shutdown_machine', label: 'Shutdown machine', destructive: true },
    { value: 'enable_sysmon', label: 'Enable Sysmon (install + channel)' },
    { value: 'disable_sysmon', label: 'Disable Sysmon (uninstall)' },
    { value: 'update_agent', label: 'Upgrade agent (in-place)' },
    { value: 'uninstall_agent', label: 'Uninstall agent (permanent)', destructive: true },
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
        case 'custom':
            return { cmd: (f.cmd || '').trim() };
        case 'collect_logs':
        case 'collect_forensics': {
            const o: Record<string, string> = {};
            const types = f.types?.trim() || f.log_types?.trim();
            if (types) o.log_types = types;
            const tr = f.time_range?.trim();
            if (tr) o.time_range = tr;
            if (f.max_events?.trim()) o.max_events = f.max_events.trim();
            if (f.file_path?.trim()) o.file_path = f.file_path.trim();
            // Return structured events so the backend can persist them for the Forensic Logs tab.
            o.return_events = 'true';
            return o;
        }
        case 'scan_memory':
        case 'scan_file':
            return { file_path: (f.file_path || f.path || '').trim() };
        case 'restart_agent':
            return { mode: 'restart' };
        case 'stop_agent':
            return { mode: 'stop' };
        case 'start_agent':
            return { mode: 'start' };
        case 'enable_sysmon': {
            const o: Record<string, string> = { mode: 'enable_sysmon' };
            if (f.sysmon_config_url?.trim()) o.config_url = f.sysmon_config_url.trim();
            return o;
        }
        case 'disable_sysmon':
            return { mode: 'disable_sysmon' };
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
        case 'uninstall_agent': {
            const o: Record<string, string> = {};
            if (f.reason?.trim()) o.reason = f.reason.trim();
            return o;
        }
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

function commandHistoryOutputCell(c: CommandListItem, agentId: string): React.ReactNode {
    if (c.command_type !== 'collect_logs' && c.command_type !== 'collect_forensics') {
        return <span className="text-slate-600 dark:text-slate-300">{resultPreview(c.result)}</span>;
    }
    const st = String(c.status || '').toLowerCase();
    const forensicsTo = `/management/devices/${encodeURIComponent(agentId)}?tab=forensics&command_id=${encodeURIComponent(c.id)}`;
    const forensicsLink = (
        <Link to={forensicsTo} className="text-cyan-700 dark:text-cyan-300 font-semibold hover:underline">
            Forensic Logs
        </Link>
    );
    if (st === 'failed' || st === 'timeout' || st === 'cancelled') {
        return (
            <div className="space-y-1">
                <div className="text-slate-600 dark:text-slate-300">{resultPreview(c.result)}</div>
                <div className="text-slate-500 text-xs">Successful collections are browsable under {forensicsLink}.</div>
            </div>
        );
    }
    const done = st === 'completed';
    return (
        <div className="text-slate-600 dark:text-slate-300 text-xs space-y-1">
            <p>
                {done
                    ? <>Full log output is not shown here. Open the {forensicsLink} tab for this device to browse events.</>
                    : <>When this command completes, browse collected logs on the {forensicsLink} tab.</>}
            </p>
            <p className="font-mono text-[11px] text-slate-500 dark:text-slate-400 break-all">{forensicsTo}</p>
        </div>
    );
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
    const [pendingDestructive, setPendingDestructive] = useState<'restart_machine' | 'shutdown_machine' | 'uninstall_agent' | null>(null);

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

    // Commands are fetched in Response tab (cmdPage). Overview uses that list for quick status panels.

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

    const execMutation = useMutation({
        mutationFn: (req: CommandRequest) => agentsApi.executeCommand(agentId, req),
        onSuccess: (data, variables) => {
            showToast(`Command queued (${data.command_id})`, 'success');
            queryClient.invalidateQueries({ queryKey: ['agent-commands', agentId] });
            queryClient.invalidateQueries({ queryKey: ['commands'] });
            const t = variables.command_type;
            if (t === 'quarantine_file' || t === 'restore_quarantine_file' || t === 'delete_quarantine_file') {
                queryClient.invalidateQueries({ queryKey: ['agent-quarantine', agentId] });
            }
            if (t === 'collect_logs' || t === 'collect_forensics') {
                queryClient.invalidateQueries({ queryKey: ['forensics', 'collections', agentId] });
            }
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
            setPendingDestructive(cmdType as 'restart_machine' | 'shutdown_machine' | 'uninstall_agent');
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
        // Uninstall carries an optional reason instead of a boolean confirm,
        // since the server-side audit trail is more useful than a re-confirm.
        const parameters =
            pendingDestructive === 'uninstall_agent'
                ? buildCommandParameters('uninstall_agent', fields)
                : { confirm: 'true' };
        execMutation.mutate({
            command_type: pendingDestructive,
            parameters,
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

    // lastCmd previously shown in Overview; Response tab now owns command details.

    return (
        <div className="relative flex flex-col min-h-[calc(100vh-2rem)] -mx-4 sm:-mx-6 lg:-mx-8 -my-4 sm:-my-6 lg:-my-8 p-4 sm:p-6 lg:p-8 bg-slate-200 dark:bg-gradient-to-br dark:from-slate-900 dark:via-[#0b1120] dark:to-slate-900">
            <div className=" w-full space-y-4">
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
                        <h2 className="text-2xl font-bold text-slate-900 dark:text-white flex flex-wrap items-center gap-3">
                            <Server className="w-7 h-7 text-cyan-500 shrink-0" />
                            <span className="break-all">{agent.hostname}</span>
                        </h2>
                        <p className="text-sm text-slate-500 mt-1 font-mono break-all">{agent.id}</p>
                    </div>
                    <div className="flex flex-wrap gap-2">
                        <span
                            className={`px-3 py-1 rounded-full text-xs font-bold uppercase ${
                                eff === 'online'
                                    ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'
                                    : eff === 'offline'
                                      ? 'bg-slate-500/15 text-slate-600'
                                      : eff === 'uninstalled'
                                        ? 'bg-rose-500/15 text-rose-700 dark:text-rose-400'
                                        : eff === 'pending_uninstall'
                                          ? 'bg-rose-500/10 text-rose-600 dark:text-rose-300'
                                          : 'bg-amber-500/15 text-amber-700 dark:text-amber-300'
                            }`}
                        >
                            {eff === 'pending_uninstall' ? 'Uninstalling…' : eff}
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
                            Command Center (filtered)
                        </Link>
                        <Link
                            to={`/events?agent_id=${encodeURIComponent(agent.id)}`}
                            className="px-3 py-1 rounded-lg text-xs font-semibold bg-slate-500/10 text-slate-700 dark:text-slate-300 border border-slate-500/20 hover:bg-slate-500/20"
                        >
                            Telemetry Search (filtered)
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
                    {tab === 'overview' && (
                        <OverviewTab
                            agent={agent}
                            eff={eff}
                            riskRow={riskRow}
                            cmds={cmdPage?.data || []}
                            overviewAlerts={overviewAlerts?.alerts ?? []}
                            overviewAlertsLoading={overviewAlertsLoading}
                            cmEvents={eventsPayload?.data || []}
                        />
                    )}

                    {tab === 'incident' && (
                        <IncidentTab
                            agent={agent}
                            onUnIsolate={() => {
                                setCmdType('restore_network');
                                setTab('response');
                            }}
                        />
                    )}

                    {tab === 'response' && canViewResp && (
                        <ResponseTab
                            agent={agent}
                            agentId={agentId}
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

                    {tab === 'forensics' && canViewResp && (
                        <ForensicsTab agentId={agent.id} />
                    )}

                    {tab === 'forensics' && !canViewResp && (
                        <p className="text-slate-500">You do not have permission to view forensic logs.</p>
                    )}

                    {tab === 'network' &&
                        (canViewAlerts ? (
                            <NetworkTab agentId={agentId} canViewAlerts={canViewAlerts} />
                        ) : (
                            <p className="text-sm text-slate-500">
                                Event search requires <code className="text-xs">alerts:read</code> (same guard as{' '}
                                event search permissions).
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
    cmds,
    overviewAlerts,
    overviewAlertsLoading,
    cmEvents,
}: {
    agent: Agent;
    eff: string;
    riskRow?: EndpointRiskSummary;
    cmds: CommandListItem[];
    overviewAlerts: Alert[];
    overviewAlertsLoading: boolean;
    cmEvents: CmEventSummary[];
}) {
    const tagEntries = Object.entries(agent.tags || {});
    const eventsCollected = agent.events_collected || agent.events_delivered || 0;
    const eventsDropped = agent.events_dropped || 0;
    const eventsDelivered = agent.events_delivered || 0;
    const dropRate = eventsCollected > 0 ? (eventsDropped / eventsCollected) * 100 : 0;
    const deliveryRate = eventsCollected > 0 ? (eventsDelivered / eventsCollected) * 100 : 0;
    const certExpiry = agent.cert_expires_at ? new Date(agent.cert_expires_at) : null;
    const certDaysLeft = certExpiry ? Math.ceil((certExpiry.getTime() - Date.now()) / 86400000) : null;
    const cpuPct = agent.cpu_usage || 0;
    const memPct = agent.memory_mb && agent.memory_used_mb ? Math.min(100, (agent.memory_used_mb / agent.memory_mb) * 100) : 0;

    const sysmonCmd = useMemo(() => {
        const list = cmds || [];
        return list.find((c) => c.command_type === 'enable_sysmon' || c.command_type === 'disable_sysmon');
    }, [cmds]);

    const sysmonObserved = useMemo(() => {
        const list = cmds || [];
        // If we have a successful collection request that explicitly asked for sysmon logs,
        // we can treat Sysmon as "observed" even if it was installed/enabled manually.
        return list.find((c) => {
            if (c.status !== 'completed') return false;
            if (c.command_type !== 'collect_logs' && c.command_type !== 'collect_forensics') return false;
            const lt = String((c.parameters as any)?.log_types || (c.parameters as any)?.types || '').toLowerCase();
            return lt.includes('sysmon');
        });
    }, [cmds]);

    const sysmonStatus = (() => {
        if (!sysmonCmd) {
            if (sysmonObserved) return { label: 'Observed', tone: 'ok' as const };
            return { label: 'Unknown', tone: 'muted' as const };
        }
        const ok = sysmonCmd.status === 'completed';
        if (sysmonCmd.command_type === 'enable_sysmon') return { label: ok ? 'Enabled' : 'Enable failed', tone: ok ? ('ok' as const) : ('bad' as const) };
        return { label: ok ? 'Disabled' : 'Disable failed', tone: ok ? ('warn' as const) : ('bad' as const) };
    })();

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
                        <div className="flex justify-between gap-2"><dt className="text-slate-500">Installed</dt><dd className="text-right">{agent.installed_date ? new Date(agent.installed_date).toLocaleDateString() : '—'}</dd></div>
                        <div className="flex justify-between gap-2"><dt className="text-slate-500">Enrolled</dt><dd className="text-right">{agent.created_at ? new Date(agent.created_at).toLocaleDateString() : '—'}</dd></div>
                        <div className="flex justify-between gap-2"><dt className="text-slate-500">Cert ID</dt><dd className="text-right font-mono text-xs break-all">{agent.current_cert_id || '—'}</dd></div>
                    </dl>
                </div>
                <div>
                    <h3 className="text-sm font-bold text-slate-700 dark:text-slate-200 mb-2 flex items-center gap-2">
                        <Settings className="w-4 h-4" /> Sysmon
                    </h3>
                    <div className="text-sm space-y-2">
                        <div className="flex items-center justify-between gap-3">
                            <span className="text-slate-500">Status</span>
                            <span
                                className={`px-2 py-0.5 rounded-full text-xs font-semibold border ${
                                    sysmonStatus.tone === 'ok'
                                        ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 border-emerald-500/20'
                                        : sysmonStatus.tone === 'warn'
                                        ? 'bg-amber-500/10 text-amber-800 dark:text-amber-300 border-amber-500/20'
                                        : sysmonStatus.tone === 'bad'
                                        ? 'bg-rose-500/10 text-rose-700 dark:text-rose-300 border-rose-500/20'
                                        : 'bg-slate-500/10 text-slate-700 dark:text-slate-300 border-slate-500/20'
                                }`}
                            >
                                {sysmonStatus.label}
                            </span>
                        </div>
                        <div className="text-xs text-slate-500">
                            Channel: <code>Microsoft-Windows-Sysmon/Operational</code>
                        </div>
                        {sysmonCmd ? (
                            <div className="text-xs text-slate-500 space-y-1">
                                <div>
                                    Last action: <code>{sysmonCmd.command_type}</code> · {new Date(sysmonCmd.issued_at).toLocaleString()}
                                </div>
                                {sysmonCmd.result?.output ? (
                                    <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white/60 dark:bg-slate-900/40 p-2 text-[11px] text-slate-700 dark:text-slate-200 whitespace-pre-wrap">
                                        {String((sysmonCmd.result as any).output)}
                                    </div>
                                ) : null}
                            </div>
                        ) : sysmonObserved ? (
                            <div className="text-xs text-slate-500 space-y-1">
                                <div>
                                    Observed via <code>{sysmonObserved.command_type}</code> · {new Date(sysmonObserved.issued_at).toLocaleString()}
                                </div>
                                <div className="text-[11px] text-slate-500">
                                    Note: Sysmon was not enabled/disabled via C2, so status is inferred from log collection.
                                </div>
                            </div>
                        ) : (
                            <p className="text-slate-500 text-sm">No Sysmon actions recorded yet.</p>
                        )}
                    </div>
                </div>
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
                <div className="rounded-lg border border-slate-200 dark:border-slate-700 p-4 bg-slate-50/80 dark:bg-slate-800/40">
                    <div className="text-xs font-semibold text-slate-500 uppercase mb-2 flex items-center gap-2">
                        <Network className="w-4 h-4" /> Network & mTLS
                    </div>
                    <div className="text-sm space-y-1.5">
                        <div className="flex justify-between gap-2"><span className="text-slate-500">Heartbeat age</span><span>{formatRelativeTime(agent.last_seen)}</span></div>
                        <div className="flex justify-between gap-2"><span className="text-slate-500">IPs</span><span className="text-right break-all">{(agent.ip_addresses || []).join(', ') || '—'}</span></div>
                        <div className="flex justify-between gap-2"><span className="text-slate-500">mTLS</span><span>{certDaysLeft != null ? (certDaysLeft > 0 ? `Valid (${certDaysLeft}d)` : 'Expired') : '—'}</span></div>
                        <div className="flex justify-between gap-2"><span className="text-slate-500">Expiry</span><span>{certExpiry ? certExpiry.toLocaleDateString() : '—'}</span></div>
                    </div>
                </div>
                <div className="rounded-lg border border-slate-200 dark:border-slate-700 p-4 bg-slate-50/80 dark:bg-slate-800/40">
                    <div className="text-xs font-semibold text-slate-500 uppercase mb-2 flex items-center gap-2">
                        <HardDrive className="w-4 h-4" /> Health & QoS
                    </div>
                    <div className="text-sm space-y-1.5">
                        <div className="flex justify-between gap-2"><span className="text-slate-500">Events collected</span><span className="font-mono text-xs">{eventsCollected.toLocaleString()}</span></div>
                        <div className="flex justify-between gap-2"><span className="text-slate-500">Delivered</span><span className="font-mono text-xs">{eventsDelivered.toLocaleString()}</span></div>
                        <div className="flex justify-between gap-2"><span className="text-slate-500">Dropped</span><span className="font-mono text-xs">{eventsDropped.toLocaleString()}</span></div>
                        <div className="flex justify-between gap-2"><span className="text-slate-500">Drop rate</span><span className="font-mono text-xs">{dropRate.toFixed(1)}%</span></div>
                        <div className="flex justify-between gap-2"><span className="text-slate-500">Delivery</span><span className="font-mono text-xs">{deliveryRate.toFixed(1)}%</span></div>
                    </div>
                </div>
                <div className="rounded-lg border border-slate-200 dark:border-slate-700 p-4 bg-slate-50/80 dark:bg-slate-800/40">
                    <div className="text-xs font-semibold text-slate-500 uppercase mb-2 flex items-center gap-2">
                        <Settings className="w-4 h-4" /> Resource usage
                    </div>
                    <div className="text-sm space-y-1.5">
                        <div className="flex justify-between gap-2"><span className="text-slate-500">CPU</span><span className="font-mono text-xs">{cpuPct.toFixed(1)}%</span></div>
                        <div className="flex justify-between gap-2"><span className="text-slate-500">Memory</span><span className="font-mono text-xs">{agent.memory_used_mb || 0} / {agent.memory_mb || '—'} MB ({memPct.toFixed(0)}%)</span></div>
                        <div className="flex justify-between gap-2"><span className="text-slate-500">Queue depth</span><span className="font-mono text-xs">{(agent.queue_depth || 0).toLocaleString()}</span></div>
                    </div>
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
                        No raw events in the latest fetch. Open the full Events view for filters and raw JSON per event.
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
                <div className="flex items-center justify-between gap-3">
                    <h3 className="text-sm font-bold text-slate-700 dark:text-slate-200">Commands</h3>
                    <Link
                        to={`/management/devices/${encodeURIComponent(agent.id)}?tab=response`}
                        className="text-xs font-semibold text-cyan-700 dark:text-cyan-300 hover:underline"
                    >
                        Open Response tab
                    </Link>
                </div>
                <p className="text-sm text-slate-500 mt-2">
                    Use the <strong>Response</strong> tab to execute commands and view full command history.
                </p>
            </div>
        </div>
    );
}

/** First log channel for Forensics API (lowercase), derived from collection summary. */
function primaryLogTypeFromCollectionField(logTypesField: string): string {
    const s = (logTypesField || '').trim();
    const lower = s.toLowerCase();
    if (!s) return 'security';
    if (lower.includes('sysmon')) return 'sysmon';
    if (lower.includes('powershell')) return 'powershell';
    const first = s.split(',')[0].trim().toLowerCase();
    return first || 'security';
}

function ForensicsTab({ agentId }: { agentId: string }) {
    const [searchParams, setSearchParams] = useSearchParams();
    const { showToast } = useToast();

    const preselectedCommandId = searchParams.get('command_id') || '';
    const [selectedCommandId, setSelectedCommandId] = useState<string>(preselectedCommandId);
    const [logType, setLogType] = useState<string>('security');
    const lastLogTypeSyncCommandRef = useRef<string | null>(null);
    const [cursor, setCursor] = useState<number | undefined>(undefined);
    const [rows, setRows] = useState<ForensicEvent[]>([]);
    const [rawOpen, setRawOpen] = useState(false);
    const [rawEvent, setRawEvent] = useState<ForensicEvent | null>(null);

    const collectionsQ = useQuery({
        queryKey: ['forensics', 'collections', agentId],
        queryFn: () => agentsApi.getForensicCollections(agentId, { limit: 25 }),
        refetchInterval: 30000,
    });

    const collections: ForensicCollection[] = (collectionsQ.data as any)?.data || [];

    useEffect(() => {
        lastLogTypeSyncCommandRef.current = null;
    }, [agentId]);

    useEffect(() => {
        if (!selectedCommandId) {
            const first = collections[0]?.command_id;
            if (first) setSelectedCommandId(first);
        }
    }, [collections, selectedCommandId]);

    useEffect(() => {
        // When the user picks another collection (View / dropdown), align log type with that run’s channels.
        if (!selectedCommandId) return;
        if (lastLogTypeSyncCommandRef.current === selectedCommandId) return;
        const sel = collections.find((c) => c.command_id === selectedCommandId);
        if (!sel) return;
        lastLogTypeSyncCommandRef.current = selectedCommandId;
        setLogType(primaryLogTypeFromCollectionField(sel.log_types || ''));
    }, [collections, selectedCommandId]);

    useEffect(() => {
        // Keep URL in sync so Command Center can deep-link.
        if (!selectedCommandId) return;
        const next = new URLSearchParams(searchParams);
        next.set('tab', 'forensics');
        next.set('command_id', selectedCommandId);
        setSearchParams(next, { replace: true });
        // Reset paging on selection changes.
        setCursor(undefined);
        setRows([]);
    }, [selectedCommandId, searchParams, setSearchParams]);

    const eventsQ = useQuery({
        queryKey: ['forensics', 'events', agentId, selectedCommandId, logType, cursor],
        enabled: Boolean(selectedCommandId && logType),
        queryFn: () =>
            agentsApi.getForensicEvents(agentId, selectedCommandId, {
                log_type: logType,
                limit: 100,
                cursor,
            }),
    });

    useEffect(() => {
        const payload: any = eventsQ.data;
        const data: ForensicEvent[] = payload?.data || [];
        if (!eventsQ.isSuccess) return;
        if (!cursor) setRows(data);
        else setRows((prev) => [...prev, ...data]);
    }, [eventsQ.data, eventsQ.isSuccess]);

    const nextCursor: number | undefined = (eventsQ.data as any)?.next_cursor;

    const selected = collections.find((c) => c.command_id === selectedCommandId);
    const summaryCounts = selected?.summary?.counts || selected?.summary?.output_text || '';
    const warnings = selected?.summary?.warnings as any[] | undefined;

    return (
        <div className="space-y-4 animate-slide-up-fade">
            <div>
                <h2 className="text-lg font-bold text-slate-900 dark:text-white">Forensic Logs</h2>
                <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
                    Browse events returned from <code className="text-xs">collect_logs</code> / <code className="text-xs">collect_forensics</code>.
                </p>
            </div>

            <div className="rounded-xl border border-slate-200 dark:border-slate-700 bg-white/60 dark:bg-slate-900/40 overflow-hidden">
                <div className="px-4 py-3 border-b border-slate-200 dark:border-slate-700 flex items-center justify-between">
                    <div className="text-sm font-semibold text-slate-700 dark:text-slate-200">Recent collections</div>
                    {collectionsQ.isLoading && <span className="text-xs text-slate-500">Loading…</span>}
                </div>
                <div className="overflow-x-auto">
                    <table className="min-w-full text-sm">
                        <thead className="bg-slate-50 dark:bg-slate-800/60 text-slate-600 dark:text-slate-300">
                            <tr>
                                <th className="text-left px-4 py-2">Issued</th>
                                <th className="text-left px-4 py-2">Time range</th>
                                <th className="text-left px-4 py-2">Log types</th>
                                <th className="text-left px-4 py-2">Summary</th>
                                <th className="text-right px-4 py-2">View</th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
                            {collections.map((c) => (
                                <tr key={c.command_id} className={c.command_id === selectedCommandId ? 'bg-cyan-50/60 dark:bg-cyan-500/10' : ''}>
                                    <td className="px-4 py-2 text-slate-700 dark:text-slate-200 whitespace-nowrap">
                                        {new Date(c.issued_at).toLocaleString()}
                                    </td>
                                    <td className="px-4 py-2 text-slate-600 dark:text-slate-300">{c.time_range || '—'}</td>
                                    <td className="px-4 py-2 font-mono text-xs text-slate-600 dark:text-slate-300">{c.log_types || '—'}</td>
                                    <td className="px-4 py-2 text-slate-600 dark:text-slate-300 truncate max-w-[28rem]" title={String(c.summary?.counts || '')}>
                                        {String(c.summary?.counts || '').slice(0, 140) || '—'}
                                    </td>
                                    <td className="px-4 py-2 text-right">
                                        <button
                                            type="button"
                                            className="px-2.5 py-1.5 text-xs font-semibold rounded-lg border border-cyan-500/30 bg-cyan-500/10 text-cyan-800 dark:text-cyan-300 hover:bg-cyan-500/20"
                                            onClick={() => setSelectedCommandId(c.command_id)}
                                        >
                                            View
                                        </button>
                                    </td>
                                </tr>
                            ))}
                            {!collectionsQ.isLoading && collections.length === 0 && (
                                <tr>
                                    <td className="px-4 py-3 text-slate-500" colSpan={5}>
                                        No forensic collections yet. Run <strong>Collect logs</strong> from the Response tab.
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>
                </div>
            </div>

            <div className="flex flex-col lg:flex-row gap-3 lg:items-end">
                <div className="flex-1">
                    <label className="block text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider mb-1">Collection</label>
                    <select
                        value={selectedCommandId}
                        onChange={(e) => setSelectedCommandId(e.target.value)}
                        className="w-full bg-white dark:bg-slate-900/60 border border-slate-200 dark:border-slate-700 rounded-lg px-3 py-2 text-sm"
                    >
                        {collections.map((c) => (
                            <option key={c.command_id} value={c.command_id}>
                                {c.command_id.slice(0, 8)} — {new Date(c.issued_at).toLocaleString()}
                            </option>
                        ))}
                    </select>
                </div>
                <div className="w-full lg:w-64">
                    <label className="block text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider mb-1">Log type</label>
                    <select
                        value={logType}
                        onChange={(e) => {
                            setLogType(e.target.value);
                            setCursor(undefined);
                            setRows([]);
                        }}
                        className="w-full bg-white dark:bg-slate-900/60 border border-slate-200 dark:border-slate-700 rounded-lg px-3 py-2 text-sm"
                    >
                        <option value="security">security</option>
                        <option value="system">system</option>
                        <option value="application">application</option>
                        <option value="powershell">powershell</option>
                        <option value="sysmon">sysmon</option>
                    </select>
                </div>
            </div>

            {summaryCounts && (
                <div className="text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-900/40 px-3 py-2 text-slate-700 dark:text-slate-200">
                    <div className="font-semibold">Summary</div>
                    <div className="mt-1 font-mono whitespace-pre-wrap break-words">{String(summaryCounts)}</div>
                    {warnings && warnings.length > 0 && (
                        <div className="mt-2 text-amber-700 dark:text-amber-300">
                            <div className="font-semibold">Warnings</div>
                            <div className="mt-1 font-mono whitespace-pre-wrap break-words">{JSON.stringify(warnings, null, 2)}</div>
                        </div>
                    )}
                </div>
            )}

            <div className="rounded-xl border border-slate-200 dark:border-slate-700 bg-white/60 dark:bg-slate-900/40 overflow-hidden">
                <div className="px-4 py-3 border-b border-slate-200 dark:border-slate-700 flex items-center justify-between">
                    <div className="text-sm font-semibold text-slate-700 dark:text-slate-200">
                        Events ({rows.length})
                    </div>
                    {eventsQ.isFetching && <span className="text-xs text-slate-500">Loading…</span>}
                </div>
                <div className="overflow-x-auto">
                    <table className="min-w-full text-sm">
                        <thead className="bg-slate-50 dark:bg-slate-800/60 text-slate-600 dark:text-slate-300">
                            <tr>
                                <th className="text-left px-4 py-2">Time</th>
                                <th className="text-left px-4 py-2">LogType</th>
                                <th className="text-left px-4 py-2">EventID</th>
                                <th className="text-left px-4 py-2">Level</th>
                                <th className="text-left px-4 py-2">Provider</th>
                                <th className="text-left px-4 py-2">Message</th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
                            {rows.map((e) => (
                                <tr
                                    key={e.id}
                                    className="hover:bg-slate-50 dark:hover:bg-slate-800/40 cursor-pointer"
                                    onClick={() => { setRawEvent(e); setRawOpen(true); }}
                                >
                                    <td className="px-4 py-2 whitespace-nowrap text-slate-700 dark:text-slate-200">
                                        {e.timestamp ? new Date(e.timestamp).toLocaleString() : '—'}
                                    </td>
                                    <td className="px-4 py-2 font-mono text-xs text-slate-600 dark:text-slate-300">{e.log_type}</td>
                                    <td className="px-4 py-2 font-mono text-xs text-slate-600 dark:text-slate-300">{e.event_id || '—'}</td>
                                    <td className="px-4 py-2 text-slate-600 dark:text-slate-300">{e.level || '—'}</td>
                                    <td className="px-4 py-2 text-slate-600 dark:text-slate-300">{e.provider || '—'}</td>
                                    <td className="px-4 py-2 text-slate-600 dark:text-slate-300 truncate max-w-[36rem]" title={e.message || ''}>
                                        {e.message || '—'}
                                    </td>
                                </tr>
                            ))}
                            {eventsQ.isSuccess && rows.length === 0 && (
                                <tr>
                                    <td className="px-4 py-3 text-slate-500" colSpan={6}>
                                        No events stored for this log type.
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>
                </div>
                <div className="px-4 py-3 border-t border-slate-200 dark:border-slate-700 flex justify-end">
                    <button
                        type="button"
                        disabled={!nextCursor || eventsQ.isFetching}
                        onClick={() => setCursor(nextCursor)}
                        className="px-3 py-2 text-sm font-semibold rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900/60 disabled:opacity-50"
                    >
                        Load more
                    </button>
                </div>
            </div>

            <Modal
                isOpen={rawOpen}
                onClose={() => { setRawOpen(false); setRawEvent(null); }}
                title="Event raw payload"
                footer={
                    <div className="flex justify-end gap-2">
                        <button
                            type="button"
                            className="btn btn-secondary"
                            onClick={() => { setRawOpen(false); setRawEvent(null); }}
                        >
                            Close
                        </button>
                        <button
                            type="button"
                            className="btn bg-cyan-600 hover:bg-cyan-700 text-white"
                            onClick={async () => {
                                try {
                                    await navigator.clipboard.writeText(JSON.stringify(rawEvent?.raw ?? {}, null, 2));
                                    showToast('Copied raw JSON to clipboard', 'success');
                                } catch {
                                    showToast('Copy failed', 'error');
                                }
                            }}
                        >
                            Copy
                        </button>
                    </div>
                }
            >
                <pre className="bg-slate-900 p-3 rounded-md text-slate-100 whitespace-pre-wrap break-words max-h-[30rem] overflow-y-auto text-xs">
                    {JSON.stringify(rawEvent?.raw ?? {}, null, 2)}
                </pre>
            </Modal>
        </div>
    );
}

function ConfigurationTab({ agent }: { agent: Agent }) {
    const meta = agent.metadata || {};
    const filterPolicy = meta.filter_policy_json || meta.filter_policy;
    const { showToast } = useToast();
    const canPushPolicy = authApi.canPushPolicy();
    const queryClient = useQueryClient();
    const [policyJson, setPolicyJson] = useState(
        JSON.stringify(
            {
                exclude_processes: ['svchost.exe'],
                exclude_event_ids: [4, 7, 15, 22],
                trusted_hashes: [],
                rate_limit: { enabled: true, default_max_eps: 500, critical_bypass: true },
            },
            null,
            2
        )
    );
    const [policyError, setPolicyError] = useState('');
    const [exceptionProcess, setExceptionProcess] = useState('');
    const [exceptionReason, setExceptionReason] = useState('');

    const policyMutation = useMutation({
        mutationFn: async () => {
            const parsed = JSON.parse(policyJson);
            return agentsApi.updateFilterPolicy(agent.id, parsed);
        },
        onSuccess: (data) => {
            showToast(`Filter policy pushed (Command ID: ${data.command_id})`, 'success');
            queryClient.invalidateQueries({ queryKey: ['agent', agent.id] });
            queryClient.invalidateQueries({ queryKey: ['agent-commands', agent.id] });
        },
        onError: (e: Error) => showToast(e.message || 'Policy push failed', 'error'),
    });
    const exceptionMutation = useMutation({
        mutationFn: async () => {
            const pn = exceptionProcess.trim();
            if (!pn) throw new Error('process_name is required');
            return agentsApi.addProcessException(agent.id, { process_name: pn, reason: exceptionReason.trim() || undefined });
        },
        onSuccess: (data) => {
            showToast(`Process exception pushed (Command ID: ${data.command_id})`, 'success');
            setExceptionProcess('');
            setExceptionReason('');
            queryClient.invalidateQueries({ queryKey: ['agent', agent.id] });
            queryClient.invalidateQueries({ queryKey: ['agent-commands', agent.id] });
        },
        onError: (e: Error) => showToast(e.message || 'Failed to push process exception', 'error'),
    });

    return (
        <div className="space-y-6 text-sm">
            <p className="text-slate-600 dark:text-slate-400">
                Read-only view of labels and metadata. Full policy editing belongs on the Response tab.
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

            <div className="rounded-lg border border-slate-200 dark:border-slate-700 p-4 bg-white/60 dark:bg-slate-900/40">
                <h3 className="text-sm font-bold text-slate-800 dark:text-slate-100 mb-2">Filter policy</h3>
                <p className="text-xs text-slate-500 mb-2">
                    Push a new policy to the agent.
                </p>
                <textarea
                    className="w-full rounded-lg border border-slate-200 dark:border-slate-600 bg-white dark:bg-slate-950 px-3 py-2 text-xs font-mono min-h-[180px]"
                    value={policyJson}
                    onChange={(e) => {
                        setPolicyJson(e.target.value);
                        setPolicyError('');
                    }}
                    spellCheck={false}
                    disabled={!canPushPolicy}
                />
                {policyError ? <p className="text-xs text-rose-600 mt-2">{policyError}</p> : null}
                <div className="flex items-center justify-end gap-2 mt-3">
                    {!canPushPolicy ? (
                        <span className="text-xs text-slate-500">Missing permission to push policy.</span>
                    ) : null}
                    <button
                        type="button"
                        disabled={!canPushPolicy || policyMutation.isPending}
                        onClick={() => {
                            try {
                                JSON.parse(policyJson);
                            } catch {
                                setPolicyError('Invalid JSON — check syntax before pushing');
                                return;
                            }
                            policyMutation.mutate();
                        }}
                        className="px-3 py-2 rounded-lg text-xs font-semibold bg-cyan-600 hover:bg-cyan-700 text-white disabled:opacity-50"
                    >
                        {policyMutation.isPending ? 'Pushing…' : 'Push policy to agent'}
                    </button>
                </div>
            </div>

            <div className="rounded-lg border border-slate-200 dark:border-slate-700 p-4 bg-white/60 dark:bg-slate-900/40">
                <h3 className="text-sm font-bold text-slate-800 dark:text-slate-100 mb-2">Process exceptions</h3>
                <p className="text-xs text-slate-500 mb-3">
                    Allow a process name to bypass <strong>process auto-response</strong>. This calls{' '}
                    an internal exception update and pushes{' '}
                    <code className="text-xs">exclude_process</code> to the agent at runtime.
                </p>
                <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                    <div className="md:col-span-1">
                        <label className="block text-[10px] font-semibold uppercase tracking-wide text-slate-500 mb-1">Process name</label>
                        <input
                            className="w-full rounded-lg border border-slate-200 dark:border-slate-600 bg-white dark:bg-slate-950 px-3 py-2 text-sm font-mono"
                            value={exceptionProcess}
                            onChange={(e) => setExceptionProcess(e.target.value)}
                            placeholder="e.g. powershell.exe"
                            disabled={!canPushPolicy}
                        />
                    </div>
                    <div className="md:col-span-2">
                        <label className="block text-[10px] font-semibold uppercase tracking-wide text-slate-500 mb-1">Reason (optional)</label>
                        <input
                            className="w-full rounded-lg border border-slate-200 dark:border-slate-600 bg-white dark:bg-slate-950 px-3 py-2 text-sm"
                            value={exceptionReason}
                            onChange={(e) => setExceptionReason(e.target.value)}
                            placeholder="approved automation / known good"
                            disabled={!canPushPolicy}
                        />
                    </div>
                </div>
                <div className="flex items-center justify-end gap-2 mt-3">
                    {!canPushPolicy ? <span className="text-xs text-slate-500">Missing permission to push exceptions.</span> : null}
                    <button
                        type="button"
                        disabled={!canPushPolicy || exceptionMutation.isPending}
                        onClick={() => exceptionMutation.mutate()}
                        className="px-3 py-2 rounded-lg text-xs font-semibold bg-cyan-600 hover:bg-cyan-700 text-white disabled:opacity-50"
                    >
                        {exceptionMutation.isPending ? 'Pushing…' : 'Add process exception'}
                    </button>
                </div>
            </div>
        </div>
    );
}

const NETWORK_PAGE_SIZE = 25;

function NetworkTab({ agentId, canViewAlerts }: { agentId: string; canViewAlerts: boolean }) {
    const queryClient = useQueryClient();
    const [detailId, setDetailId] = useState<string | null>(null);
    const [rangeDays, setRangeDays] = useState<7 | 30 | 90>(30);
    const [page, setPage] = useState(1);

    const { from, to } = useMemo(() => {
        const toDate = new Date();
        const fromDate = new Date(Date.now() - rangeDays * 24 * 60 * 60 * 1000);
        return { from: fromDate.toISOString(), to: toDate.toISOString() };
    }, [rangeDays]);

    useEffect(() => {
        setPage(1);
    }, [rangeDays]);

    const offset = (page - 1) * NETWORK_PAGE_SIZE;

    const networkSearch = useQuery({
        queryKey: ['events-search-network', agentId, from, to, offset, NETWORK_PAGE_SIZE],
        queryFn: () =>
            eventsApi.search({
                filters: [
                    { field: 'agent_id', operator: 'equals', value: agentId },
                    { field: 'event_type', operator: 'equals', value: 'network' },
                ],
                logic: 'AND',
                time_range: { from, to },
                limit: NETWORK_PAGE_SIZE,
                offset,
            }),
        enabled: !!agentId && canViewAlerts,
        staleTime: 15_000,
        retry: 1,
    });

    const rows = networkSearch.data?.data ?? [];
    const total = networkSearch.data?.pagination?.total ?? 0;
    const loading = networkSearch.isLoading;
    const totalPages = Math.max(1, Math.ceil(total / NETWORK_PAGE_SIZE));

    useEffect(() => {
        if (page > totalPages) setPage(totalPages);
    }, [page, totalPages]);

    const eventsPageHref = useMemo(() => {
        const p = new URLSearchParams();
        p.set('agent_id', agentId);
        p.set('event_type', 'network');
        p.set('from', from);
        p.set('to', to);
        p.set('page', '1');
        return `/events?${p.toString()}`;
    }, [agentId, from, to]);

    const refetchNetwork = () => {
        void queryClient.invalidateQueries({ queryKey: ['events-search-network', agentId] });
    };

    return (
        <div className="space-y-4 text-sm">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                <p className="text-slate-600 dark:text-slate-400 max-w-3xl">
                    Network telemetry via <code className="text-xs">POST /api/v1/events/search</code> — filters{' '}
                    <code className="text-xs">agent_id</code> + <code className="text-xs">event_type=network</code>. Click a row for raw JSON (
                    raw event requests).
                </p>
                <div className="flex flex-wrap items-center gap-2 shrink-0">
                    <Link
                        to={eventsPageHref}
                        className="px-3 py-1.5 rounded-lg text-xs font-semibold border border-slate-200 dark:border-slate-600 bg-white/70 dark:bg-slate-900/50 text-cyan-700 dark:text-cyan-300 hover:bg-slate-100 dark:hover:bg-slate-800"
                    >
                        Open in Events
                    </Link>
                    <button
                        type="button"
                        onClick={refetchNetwork}
                        disabled={networkSearch.isFetching}
                        className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold border border-slate-200 dark:border-slate-600 bg-white/70 dark:bg-slate-900/50 hover:bg-slate-100 dark:hover:bg-slate-800 disabled:opacity-50"
                        title="Refresh list"
                    >
                        <RefreshCw className={`w-3.5 h-3.5 ${networkSearch.isFetching ? 'animate-spin' : ''}`} />
                        Refresh
                    </button>
                </div>
            </div>

            <div className="flex flex-wrap items-center gap-2">
                <span className="text-[10px] font-semibold uppercase tracking-wide text-slate-500">Time range</span>
                {([7, 30, 90] as const).map((d) => (
                    <button
                        key={d}
                        type="button"
                        onClick={() => setRangeDays(d)}
                        className={`px-2.5 py-1 rounded-md text-xs font-medium border transition-colors ${
                            rangeDays === d
                                ? 'border-cyan-500/60 bg-cyan-500/10 text-cyan-800 dark:text-cyan-200'
                                : 'border-slate-200 dark:border-slate-600 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'
                        }`}
                    >
                        Last {d} days
                    </button>
                ))}
            </div>

            <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-slate-500 dark:text-slate-400">
                <span>
                    {loading ? 'Loading…' : <>Showing {rows.length} row{rows.length !== 1 ? 's' : ''}</>}
                    {!loading && total > 0 && (
                        <>
                            {' '}
                            · total <span className="font-mono text-slate-700 dark:text-slate-200">{total}</span> in range
                        </>
                    )}
                </span>
                <span className="font-mono text-[10px] truncate max-w-[min(100%,420px)]" title={`${from} → ${to}`}>
                    {from.slice(0, 19)}Z → {to.slice(0, 19)}Z
                </span>
            </div>

            {loading ? (
                <div className="flex justify-center py-12">
                    <Loader2 className="w-8 h-8 animate-spin text-cyan-500" />
                </div>
            ) : rows.length === 0 ? (
                <div className="rounded-lg border border-dashed border-slate-300 dark:border-slate-600 p-4 text-slate-500">
                    No network events in this window. Try <strong>Last 90 days</strong>, generate outbound telemetry from the agent, or open{' '}
                    <Link className="text-cyan-600 dark:text-cyan-400 underline" to={eventsPageHref}>
                        Events
                    </Link>
                    . If the list stays empty, confirm network connectivity to the connection manager.
                </div>
            ) : (
                <>
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
                                {rows.map((e: CmEventSummary) => (
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
                                        <td className="p-2 max-w-xl truncate" title={e.summary}>
                                            {e.summary}
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>

                    {totalPages > 1 && (
                        <div className="flex items-center justify-between gap-3 pt-1">
                            <button
                                type="button"
                                className="inline-flex items-center gap-1 px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-700 text-xs font-medium disabled:opacity-40"
                                disabled={page <= 1}
                                onClick={() => setPage((p) => Math.max(1, p - 1))}
                            >
                                <ChevronLeft className="w-4 h-4" /> Prev
                            </button>
                            <span className="text-xs text-slate-500">
                                Page <span className="font-semibold text-slate-700 dark:text-slate-200">{page}</span> / {totalPages}
                            </span>
                            <button
                                type="button"
                                className="inline-flex items-center gap-1 px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-700 text-xs font-medium disabled:opacity-40"
                                disabled={page >= totalPages}
                                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                            >
                                Next <ChevronRight className="w-4 h-4" />
                            </button>
                        </div>
                    )}
                </>
            )}

            <EventDetailModal eventId={detailId} onClose={() => setDetailId(null)} fetchEnabled={canViewAlerts} />
        </div>
    );
}

function ResponseTab({
    agent: _agent,
    agentId,
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
    agentId: string;
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
    const uniqueOptions = useMemo(() => {
        const seen = new Set<string>();
        return RESPONSE_OPTIONS.filter((o) => {
            if (seen.has(o.value)) return false;
            seen.add(o.value);
            return true;
        });
    }, []);

    const { showToast } = useToast();
    const queryClient = useQueryClient();
    const [upgradeOpen, setUpgradeOpen] = useState(false);
    const [upgradeForm, setUpgradeForm] = useState<{
        serverIP: string;
        serverDomain: string;
        serverPort: string;
        tokenId: string;
        installSysmon: boolean;
    }>({ serverIP: '', serverDomain: '', serverPort: '47051', tokenId: '', installSysmon: false });

    const { data: upgradeTokens = [], isLoading: upgradeTokensLoading } = useQuery<EnrollmentToken[]>({
        queryKey: ['enrollment-tokens-valid', 'upgrade'],
        queryFn: () => agentBuildApi.listValidTokens(),
        enabled: upgradeOpen,
    });

    const upgradeMutation = useMutation({
        mutationFn: async () => {
            // Bind the package to THIS agent's identity. The download endpoint
            // verifies the agent_id against the peer mTLS certificate CN and
            // revokes the link after the first successful download (or expiry).
            const pkg = await agentPackagesApi.create({
                server_ip: upgradeForm.serverIP || undefined,
                server_domain: upgradeForm.serverDomain || undefined,
                server_port: upgradeForm.serverPort || undefined,
                // Lets the CM mint URLs reachable from remote agents (browser origin).
                public_api_base_url:
                    typeof window !== 'undefined' ? window.location.origin : undefined,
                token_id: upgradeForm.tokenId,
                skip_config: false,
                install_sysmon: upgradeForm.installSysmon,
                expires_in_seconds: 900,
                agent_id: agentId,
            });
            await agentsApi.executeCommand(agentId, {
                command_type: 'update_agent',
                parameters: {
                    url: pkg.url,
                    checksum: pkg.sha256,
                    version: pkg.package_id,
                    server_domain: upgradeForm.serverDomain,
                    server_port: upgradeForm.serverPort,
                    server_ip: upgradeForm.serverIP,
                },
                timeout: 3600,
            });
            return pkg;
        },
        onSuccess: (pkg) => {
            showToast(`Upgrade queued (package ${pkg.package_id.slice(0, 8)})`, 'success');
            queryClient.invalidateQueries({ queryKey: ['agent-commands', agentId] });
            queryClient.invalidateQueries({ queryKey: ['commands'] });
            setUpgradeOpen(false);
        },
        onError: (e: Error) => showToast(e.message || 'Upgrade failed', 'error'),
    });

    return (
        <div className="space-y-8">
            <div>
                <div className="flex items-center justify-between gap-3 mb-4">
                    <h3 className="text-sm font-bold text-slate-800 dark:text-slate-100 flex items-center gap-2">
                        <Terminal className="w-4 h-4" /> Execute command
                    </h3>
                    {canExec && (
                        <button
                            type="button"
                            onClick={() => setUpgradeOpen(true)}
                            className="px-3 py-2 rounded-lg text-xs font-semibold border border-cyan-500/40 bg-cyan-500/10 text-cyan-800 dark:text-cyan-300 hover:bg-cyan-500/20 transition-colors"
                        >
                            Upgrade agent
                        </button>
                    )}
                </div>
                <div className="grid grid-cols-1 xl:grid-cols-12 gap-6">
                    <div className="xl:col-span-7 space-y-3">
                        <label className="block text-xs font-bold tracking-wider text-slate-500 uppercase mb-2">Select Action</label>
                        <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 max-h-[400px] overflow-y-auto pr-2 custom-scrollbar">
                            {uniqueOptions.map((o) => {
                                const isSelected = cmdType === o.value;
                                return (
                                    <button
                                        key={o.value}
                                        type="button"
                                        disabled={!canExec}
                                        onClick={() => setCmdType(o.value)}
                                        className={`flex flex-col items-start p-3 rounded-xl border text-left transition-all ${
                                            isSelected
                                                ? 'border-cyan-500 bg-cyan-50 dark:bg-cyan-900/20 shadow-sm ring-1 ring-cyan-500'
                                                : 'border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800/50 hover:border-slate-300 dark:hover:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-800'
                                        } ${!canExec ? 'opacity-50 cursor-not-allowed' : ''}`}
                                    >
                                        <div className="flex items-center justify-between w-full">
                                            <span className={`text-sm font-medium ${isSelected ? 'text-cyan-700 dark:text-cyan-400 font-semibold' : 'text-slate-700 dark:text-slate-300'}`}>
                                                {o.label}
                                            </span>
                                            {o.destructive && (
                                                <AlertTriangle className={`w-3.5 h-3.5 shrink-0 ${isSelected ? 'text-rose-500' : 'text-slate-400'}`} />
                                            )}
                                        </div>
                                    </button>
                                );
                            })}
                        </div>
                    </div>

                    <div className="xl:col-span-5 bg-slate-50 dark:bg-slate-900/30 p-5 rounded-2xl border border-slate-200 dark:border-slate-700/60 flex flex-col">
                        <h4 className="text-sm font-bold text-slate-800 dark:text-slate-100 flex items-center gap-2 mb-4">
                            <Settings className="w-4 h-4 text-slate-400" /> Parameters
                        </h4>
                        
                        <div className="space-y-4 flex-1">
                            <div>
                                <label className="block text-xs font-semibold text-slate-500 uppercase mb-1">Timeout (seconds)</label>
                                <input
                                    className="w-full rounded-lg border border-slate-200 dark:border-slate-600 bg-white dark:bg-slate-900 px-3 py-2 text-sm focus:ring-2 focus:ring-cyan-500/20 focus:border-cyan-500 transition-colors"
                                    type="number"
                                    min={0}
                                    max={3600}
                                    value={fields.timeout ?? '300'}
                                    onChange={(e) => patch('timeout', e.target.value)}
                                    disabled={!canExec}
                                />
                            </div>

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

                        {(cmdType === 'run_cmd' || cmdType === 'custom') && (
                            <>
                                <label className="text-xs text-slate-500">Command (whitelisted)</label>
                                <input className="input w-full font-mono text-xs" placeholder="hostname" value={fields.cmd || ''} onChange={(e) => patch('cmd', e.target.value)} disabled={!canExec} />
                            </>
                        )}

                        {(cmdType === 'collect_logs' || cmdType === 'collect_forensics') && (
                            <>
                                <label className="text-xs text-slate-500">Log types (comma-separated)</label>
                                <input
                                    className="input w-full text-xs"
                                    placeholder="Security,System,Application,Sysmon,PowerShell"
                                    value={fields.log_types || ''}
                                    onChange={(e) => patch('log_types', e.target.value)}
                                    disabled={!canExec}
                                />
                                <label className="text-xs text-slate-500">Time range</label>
                                <input
                                    className="input w-full text-xs"
                                    placeholder="24h | 7d | Last 6 hours"
                                    value={fields.time_range || '24h'}
                                    onChange={(e) => patch('time_range', e.target.value)}
                                    disabled={!canExec}
                                />
                                <label className="text-xs text-slate-500">Max events (per log)</label>
                                <input className="input w-full" value={fields.max_events || '500'} onChange={(e) => patch('max_events', e.target.value)} disabled={!canExec} />
                                <label className="text-xs text-slate-500">File path (optional, hash scan)</label>
                                <input className="input w-full font-mono text-xs" placeholder="C:\\path\\to\\file.exe" value={fields.file_path || ''} onChange={(e) => patch('file_path', e.target.value)} disabled={!canExec} />
                            </>
                        )}

                        {(cmdType === 'scan_memory' || cmdType === 'scan_file') && (
                            <>
                                <label className="text-xs text-slate-500">File path</label>
                                <input
                                    className="input w-full font-mono text-xs"
                                    placeholder="C:\\path\\to\\file.exe"
                                    value={fields.file_path || ''}
                                    onChange={(e) => patch('file_path', e.target.value)}
                                    disabled={!canExec}
                                />
                                <p className="text-xs text-slate-500">
                                    Note: in this build, <code>scan_memory</code> is implemented as a safe on-disk hash scan (same as <code>scan_file</code>).
                                </p>
                            </>
                        )}

                        {(cmdType === 'restart_agent' || cmdType === 'stop_agent' || cmdType === 'start_agent' || cmdType === 'restart_service') && (
                            <p className="text-xs text-slate-500">
                                No parameters required. The agent service is controlled via Windows SCM (mode: stop/start/restart).
                            </p>
                        )}

                        {cmdType === 'enable_sysmon' && (
                            <>
                                <label className="text-xs text-slate-500">Sysmon config URL (optional)</label>
                                <input
                                    className="input w-full font-mono text-xs"
                                    placeholder="https://example.com/sysmonconfig.xml"
                                    value={fields.sysmon_config_url || ''}
                                    onChange={(e) => patch('sysmon_config_url', e.target.value)}
                                    disabled={!canExec}
                                />
                                <p className="text-xs text-slate-500">
                                    Installs Sysmon if missing, enables <code>Microsoft-Windows-Sysmon/Operational</code>, and applies config if provided.
                                </p>
                            </>
                        )}

                        {cmdType === 'disable_sysmon' && (
                            <p className="text-xs text-slate-500">
                                Disables the Sysmon channel and uninstalls Sysmon if present.
                            </p>
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
                            <p className="text-xs text-amber-600 dark:text-amber-400 bg-amber-50 dark:bg-amber-900/20 p-3 rounded-lg border border-amber-200 dark:border-amber-800/30 font-medium">You will be asked to confirm — these actions affect the whole host.</p>
                        )}

                        {cmdType === 'uninstall_agent' && (
                            <>
                                <label className="text-xs text-slate-500">Reason (audit trail)</label>
                                <input
                                    className="input w-full text-xs"
                                    placeholder="Decommission / replaced host / offboarding"
                                    value={fields.reason || ''}
                                    onChange={(e) => patch('reason', e.target.value)}
                                    disabled={!canExec}
                                />
                                <p className="text-xs text-rose-600 dark:text-rose-400 bg-rose-50 dark:bg-rose-900/20 p-3 rounded-lg border border-rose-200 dark:border-rose-800/30 font-medium">
                                    Permanent: the agent will release its protections, confirm back to the server, and remove itself. No further commands will be accepted.
                                </p>
                            </>
                        )}
                        </div>

                        <div className="pt-5 mt-5 border-t border-slate-200 dark:border-slate-700/60 flex items-center justify-between">
                            {!canExec && <span className="text-xs text-rose-500 font-medium">Your role cannot execute remote commands.</span>}
                            <button
                                type="button"
                                disabled={!canExec || execMutation.isPending}
                                onClick={onSubmit}
                                className="ml-auto flex items-center gap-2 px-5 py-2.5 rounded-xl text-sm font-semibold bg-gradient-to-r from-cyan-600 to-blue-600 hover:from-cyan-500 hover:to-blue-500 text-white shadow-sm disabled:opacity-50 transition-all"
                            >
                                {execMutation.isPending ? <Loader2 className="w-4 h-4 animate-spin" /> : <Terminal className="w-4 h-4" />}
                                Send command
                            </button>
                        </div>
                    </div>
                </div>
            </div>

            <Modal isOpen={upgradeOpen} onClose={() => setUpgradeOpen(false)} title="Upgrade agent (in-place)">
                <div className="space-y-4 animate-slide-up-fade">
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                        <div>
                            <label className="block text-xs font-semibold text-slate-500 uppercase mb-1">Server IP</label>
                            <input className="input w-full" value={upgradeForm.serverIP} onChange={(e) => setUpgradeForm((p) => ({ ...p, serverIP: e.target.value }))} />
                        </div>
                        <div>
                            <label className="block text-xs font-semibold text-slate-500 uppercase mb-1">Server domain</label>
                            <input className="input w-full" value={upgradeForm.serverDomain} onChange={(e) => setUpgradeForm((p) => ({ ...p, serverDomain: e.target.value }))} />
                        </div>
                        <div>
                            <label className="block text-xs font-semibold text-slate-500 uppercase mb-1">gRPC port</label>
                            <input className="input w-full" value={upgradeForm.serverPort} onChange={(e) => setUpgradeForm((p) => ({ ...p, serverPort: e.target.value }))} />
                        </div>
                        <div>
                            <label className="block text-xs font-semibold text-slate-500 uppercase mb-1">Enrollment token</label>
                            <select
                                className="input w-full"
                                value={upgradeForm.tokenId}
                                onChange={(e) => setUpgradeForm((p) => ({ ...p, tokenId: e.target.value }))}
                                disabled={upgradeTokensLoading}
                            >
                                <option value="">{upgradeTokensLoading ? 'Loading…' : 'Select token'}</option>
                                {upgradeTokens.map((t) => (
                                    <option key={t.id} value={t.id}>
                                        {t.description || t.id.slice(0, 8)}
                                    </option>
                                ))}
                            </select>
                        </div>
                    </div>

                    <label className="flex items-center gap-2 text-sm">
                        <input
                            type="checkbox"
                            checked={upgradeForm.installSysmon}
                            onChange={(e) => setUpgradeForm((p) => ({ ...p, installSysmon: e.target.checked }))}
                        />
                        Install and enable Sysmon on first run
                    </label>

                    <div className="flex justify-end gap-2 pt-2">
                        <button type="button" className="btn" onClick={() => setUpgradeOpen(false)}>
                            Cancel
                        </button>
                        <button
                            type="button"
                            className="btn btn-primary"
                            disabled={!upgradeForm.tokenId || upgradeMutation.isPending}
                            onClick={() => upgradeMutation.mutate()}
                        >
                            {upgradeMutation.isPending ? 'Queuing…' : 'Queue upgrade'}
                        </button>
                    </div>
                </div>
            </Modal>

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
                                                    commandHistoryOutputCell(c, agentId)
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
        <div className="space-y-4 animate-slide-up-fade">
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
                        No timeline entries yet. Alerts need Sigma data; raw events need telemetry data populated.
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


