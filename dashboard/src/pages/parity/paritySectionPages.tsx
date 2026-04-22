import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import React from 'react';
import { GenericParityView } from '../../components/parity/GenericParityView';
import { parityApi } from '../../api/parity/parityApi';
import * as mocks from '../../api/parity/mocks';
import StatCard from '../../components/StatCard';
import { Activity, AlertTriangle, Loader2, Shield } from 'lucide-react';
import { agentsApi, alertsApi, authApi, type Agent, type CommandType } from '../../api/client';
import { useToast } from '../../components';
import { formatRelativeTime, getEffectiveStatus } from '../../utils/agentDisplay';

/** Commercial / MSP modules outside current self-hosted endpoint scope. */
function SelfHostedOutOfScope({ title }: { title: string }) {
    return (
        <div className="rounded-xl border border-amber-200 dark:border-amber-900/40 bg-amber-50/90 dark:bg-amber-950/25 p-6 space-y-3">
            <p className="font-semibold text-gray-900 dark:text-white">{title}</p>
            <p className="text-sm text-gray-600 dark:text-gray-400">
                Not part of the self-hosted EDR MVP. Use the sections below for real fleet and response workflows.
            </p>
            <div className="flex flex-wrap gap-x-4 gap-y-2 text-sm">
                <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/management/devices">
                    Devices (Fleet)
                </Link>
                <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/responses">
                    Command Center
                </Link>
                <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to="/system/access/users">
                    Access
                </Link>
            </div>
        </div>
    );
}

export function SecurityEndpointZeroTrustPage() {
    const statsQ = useQuery({ queryKey: ['agents-stats'], queryFn: () => agentsApi.stats(), staleTime: 30_000 });
    const riskQ = useQuery({ queryKey: ['endpoint-risk'], queryFn: () => alertsApi.endpointRisk(), staleTime: 60_000, retry: 1 });

    const s = statsQ.data;
    const riskRows = riskQ.data?.data ?? [];
    const withOpenAlerts = riskRows.filter((r) => r.open_count > 0).length;

    return (
        <div className="space-y-4">
            <div>
                <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Endpoint Zero Trust</h2>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    Practical posture view for self-hosted EDR. Uses agent metrics and{' '}
                    endpoint risk scores.
                </p>
            </div>

            {(statsQ.isLoading || riskQ.isLoading) && <div className="h-40 rounded-xl bg-gray-100 dark:bg-gray-800 animate-pulse" />}

            {(statsQ.isError || !s) && (
                <div className="rounded-xl border border-rose-200 dark:border-rose-900/50 bg-rose-50/80 dark:bg-rose-950/20 p-6 text-sm text-rose-900 dark:text-rose-200">
                    Could not load agent stats. Verify connection-manager and roles.
                </div>
            )}

            {s && (
                <>
                    <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                        <StatCard title="Endpoints" value={String(s.total)} icon={Shield} />
                        <StatCard title="Online" value={String(s.online)} icon={Activity} color="emerald" />
                        <StatCard title="Degraded" value={String(s.degraded)} icon={AlertTriangle} color="amber" />
                        <StatCard title="Offline" value={String(s.offline)} icon={Shield} />
                    </div>
                    <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                        <StatCard title="Suspended" value={String(s.suspended)} icon={AlertTriangle} color="red" />
                        <StatCard title="Avg health" value={`${Math.round(s.avg_health)}%`} icon={Activity} color="cyan" />
                        <StatCard title="Agents w/ open alerts" value={String(withOpenAlerts)} icon={Shield} color="amber" />
                        <StatCard title="Pending" value={String(s.pending)} icon={Activity} />
                    </div>
                    <div className="flex flex-wrap gap-3 text-sm">
                        <Link to="/management/devices" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                            Devices (Fleet) →
                        </Link>
                        <Link to="/endpoint-risk" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                            Endpoint Risk →
                        </Link>
                    </div>
                </>
            )}
        </div>
    );
}

export function SecurityCloudZeroTrustPage() {
    return (
        <GenericParityView
            title="Cloud Security — Zero Trust"
            missingApi="true"
            queryKey={['parity', 'security', 'posture', 'cloud']}
            fetcher={() => parityApi.getSecurityPostureCloud()}
            mock={mocks.mockSecurityPostureCloud}
        />
    );
}

export function SecuritySiemPage() {
    const KEY = 'edr.siem.connectors';
    const [url, setUrl] = React.useState('');
    const [saved, setSaved] = React.useState<string[]>(() => {
        try {
            const raw = localStorage.getItem(KEY);
            const parsed = raw ? (JSON.parse(raw) as string[]) : [];
            return Array.isArray(parsed) ? parsed : [];
        } catch {
            return [];
        }
    });

    const save = () => {
        const u = url.trim();
        if (!u) return;
        const next = Array.from(new Set([u, ...saved]));
        setSaved(next);
        localStorage.setItem(KEY, JSON.stringify(next));
        setUrl('');
    };

    const remove = (u: string) => {
        const next = saved.filter((x) => x !== u);
        setSaved(next);
        localStorage.setItem(KEY, JSON.stringify(next));
    };

    return (
        <div className="space-y-4">
            <div>
                <h2 className="text-lg font-semibold text-gray-900 dark:text-white">SIEM connectors</h2>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    Configure webhook destinations for alerts/commands export. Backend connector APIs are not exposed yet; configs are stored locally in this browser.
                </p>
            </div>

            <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40 p-4 space-y-3">
                <div className="flex flex-col sm:flex-row gap-2">
                    <input
                        className="flex-1 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-950 px-3 py-2 text-sm font-mono"
                        value={url}
                        onChange={(e) => setUrl(e.target.value)}
                        placeholder="https://your-siem-webhook.example/ingest"
                    />
                    <button
                        type="button"
                        onClick={save}
                        className="px-3 py-2 rounded-lg text-sm font-semibold bg-cyan-600 hover:bg-cyan-700 text-white"
                    >
                        Add connector
                    </button>
                </div>

                {saved.length === 0 ? (
                    <p className="text-sm text-gray-500">No connectors yet.</p>
                ) : (
                    <ul className="text-sm space-y-2">
                        {saved.map((u) => (
                            <li key={u} className="flex items-center justify-between gap-3 rounded-lg border border-gray-200 dark:border-gray-700 px-3 py-2">
                                <code className="text-xs font-mono break-all">{u}</code>
                                <button type="button" className="text-xs text-rose-600 hover:underline" onClick={() => remove(u)}>
                                    Remove
                                </button>
                            </li>
                        ))}
                    </ul>
                )}
            </div>

            <div className="rounded-xl border border-dashed border-gray-300 dark:border-gray-700 bg-white/50 dark:bg-gray-900/20 p-4">
                <p className="text-sm text-gray-600 dark:text-gray-400">
                    Next step (backend): store connectors server-side and stream alerts/events. Until then, use the existing pages to export JSON manually:
                </p>
                <div className="mt-2 flex flex-wrap gap-3 text-sm">
                    <Link to="/alerts" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                        Alerts →
                    </Link>
                    <Link to="/responses" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                        Command Center →
                    </Link>
                    <Link to="/events" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                        Telemetry Search →
                    </Link>
                </div>
            </div>
        </div>
    );
}

export function SecurityThreatLabsPage() {
    return (
        <GenericParityView
            title="Threat Labs — IOC feed"
            missingApi="true"
            queryKey={['parity', 'threat-labs', 'iocs']}
            fetcher={() => parityApi.getThreatLabsIocs()}
            mock={mocks.mockThreatLabsIocs.data}
        />
    );
}

export function ManagedSecurityOverviewPage() {
    const alertsQ = useQuery({
        queryKey: ['sigma-alerts', 'managed-overview'],
        queryFn: () => alertsApi.list({ limit: 200, order: 'desc' }),
        staleTime: 30_000,
        retry: 1,
    });

    if (alertsQ.isLoading) return <div className="h-40 rounded-xl bg-gray-100 dark:bg-gray-800 animate-pulse" />;
    if (alertsQ.isError || !alertsQ.data) return null;
    const alerts = alertsQ.data.alerts ?? [];
    const open = alerts.filter((a) => (a.status || 'open') === 'open').length;
    const high = alerts.filter((a) => a.severity === 'high' || a.severity === 'critical').length;

    return (
        <div className="space-y-4">
            <div>
                <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Managed Security — overview</h2>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    For self-hosted deployments, incidents map to Sigma alerts. Open incidents live under the Incidents tab.
                </p>
            </div>
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
                <StatCard title="Open incidents" value={String(open)} icon={Shield} color="amber" />
                <StatCard title="High/Critical" value={String(high)} icon={AlertTriangle} color="red" />
                <StatCard title="Total alerts (loaded)" value={String(alerts.length)} icon={Activity} color="cyan" />
            </div>
            <div className="flex flex-wrap gap-3 text-sm">
                <Link to="/managed-security/incidents" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                    Incidents →
                </Link>
                <Link to="/alerts" className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium">
                    Alerts →
                </Link>
            </div>
        </div>
    );
}

export function ManagedSecurityIncidentsPage() {
    const q = useQuery({
        queryKey: ['sigma-alerts', 'managed-incidents'],
        queryFn: () => alertsApi.list({ limit: 200, order: 'desc' }),
        staleTime: 15_000,
        retry: 1,
    });

    if (q.isLoading) return <div className="h-40 rounded-xl bg-gray-100 dark:bg-gray-800 animate-pulse" />;
    if (q.isError || !q.data) return null;
    const rows = q.data.alerts ?? [];

    return (
        <div className="space-y-4">
            <div>
                <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Managed Security — incidents</h2>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">Incidents are represented by Sigma alerts (sorted newest first).</p>
            </div>
            <div className="overflow-x-auto rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40">
                <table className="min-w-full text-left text-sm">
                    <thead className="bg-gray-50 dark:bg-gray-800/80 text-gray-600 dark:text-gray-400 text-xs uppercase">
                        <tr>
                            <th className="px-3 py-2">Time</th>
                            <th className="px-3 py-2">Severity</th>
                            <th className="px-3 py-2">Rule</th>
                            <th className="px-3 py-2">Agent</th>
                        </tr>
                    </thead>
                    <tbody>
                        {rows.map((a) => (
                            <tr key={a.id} className="border-t border-gray-100 dark:border-gray-800">
                                <td className="px-3 py-2 text-xs whitespace-nowrap">{new Date(a.timestamp || a.created_at).toLocaleString()}</td>
                                <td className="px-3 py-2 text-xs font-mono">{a.severity || '—'}</td>
                                <td className="px-3 py-2">{a.rule_title || a.rule_id || 'Alert'}</td>
                                <td className="px-3 py-2">
                                    {a.agent_id ? (
                                        <Link className="text-cyan-600 dark:text-cyan-400 hover:underline font-mono text-xs" to={`/management/devices/${encodeURIComponent(a.agent_id)}?tab=activity`}>
                                            {a.agent_id.slice(0, 8)}…
                                        </Link>
                                    ) : (
                                        '—'
                                    )}
                                </td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>
        </div>
    );
}

export function ManagedSecuritySlaPage() {
    return (
        <GenericParityView
            title="Managed Security — SLA"
            missingApi="true"
            queryKey={['parity', 'managed', 'sla']}
            fetcher={() => parityApi.getManagedSla()}
            mock={mocks.mockManagedSla}
        />
    );
}

export function ItsmTicketsPage() {
    return (
        <GenericParityView
            title="ITSM — tickets"
            missingApi="true"
            queryKey={['parity', 'itsm', 'tickets']}
            fetcher={() => parityApi.getItsmTickets()}
            mock={mocks.mockItsmTickets}
        />
    );
}

export function ItsmPlaybooksPage() {
    const queryClient = useQueryClient();
    const { showToast } = useToast();
    const canExec = authApi.canExecuteCommands();
    const [agentId, setAgentId] = React.useState('');
    const [killName, setKillName] = React.useState('notepad.exe');
    const [domain, setDomain] = React.useState('example.com');
    const [ip, setIp] = React.useState('1.2.3.4');

    const exec = useMutation({
        mutationFn: async (req: { command_type: CommandType; parameters?: Record<string, string>; timeout?: number }) => {
            const aid = agentId.trim();
            if (!aid) throw new Error('agent_id is required');
            return agentsApi.executeCommand(aid, {
                command_type: req.command_type,
                parameters: req.parameters ?? {},
                timeout: req.timeout ?? 300,
            });
        },
        onSuccess: (d) => {
            showToast(`Queued (${d.command_id})`, 'success');
            queryClient.invalidateQueries({ queryKey: ['commands'] });
            queryClient.invalidateQueries({ queryKey: ['agent-commands', agentId.trim()] });
        },
        onError: (e: Error) => showToast(e.message || 'Failed', 'error'),
    });

    const PlaybookCard = ({
        title,
        description,
        onRun,
        disabled,
        children,
    }: {
        title: string;
        description: string;
        onRun: () => void;
        disabled?: boolean;
        children?: React.ReactNode;
    }) => (
        <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40 p-4 space-y-2">
            <div className="font-semibold text-gray-900 dark:text-white">{title}</div>
            <div className="text-xs text-gray-500">{description}</div>
            {children}
            <div className="flex justify-end">
                <button
                    type="button"
                    disabled={disabled || !canExec || exec.isPending}
                    onClick={onRun}
                    className="px-3 py-2 rounded-lg text-sm font-semibold bg-cyan-600 hover:bg-cyan-700 text-white disabled:opacity-50"
                >
                    {exec.isPending ? 'Running…' : 'Run playbook'}
                </button>
            </div>
        </div>
    );

    return (
        <div className="space-y-4">
            <div>
                <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Playbooks</h2>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    Curated incident response playbooks built on the existing command pipeline. For full control, use{' '}
                    <Link className="text-cyan-600 dark:text-cyan-400 hover:underline font-medium" to="/responses">
                        Command Center
                    </Link>
                    .
                </p>
            </div>

            <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40 p-4 space-y-2">
                <label className="block text-xs font-semibold text-gray-500 uppercase">Target agent_id</label>
                <input
                    className="w-full rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-950 px-3 py-2 text-sm font-mono"
                    value={agentId}
                    onChange={(e) => setAgentId(e.target.value)}
                    placeholder="UUID"
                />
                {!canExec ? <p className="text-xs text-amber-700 dark:text-amber-300">Missing responses:execute permission.</p> : null}
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                <PlaybookCard
                    title="Contain: isolate network"
                    description="Immediate containment. Use Restore Network later to revert."
                    onRun={() => exec.mutate({ command_type: 'isolate_network', timeout: 300 })}
                />

                <PlaybookCard
                    title="Triage: collect forensics"
                    description="Collect a bounded set of telemetry for investigation."
                    onRun={() =>
                        exec.mutate({
                            command_type: 'collect_forensics',
                            parameters: { event_types: 'process,file,network,dns,registry', max_events: '500' },
                            timeout: 900,
                        })
                    }
                />

                <PlaybookCard
                    title="Stop suspicious process"
                    description="Terminate a process by name (best-effort)."
                    onRun={() => exec.mutate({ command_type: 'kill_process', parameters: { process_name: killName, kill_tree: 'true' }, timeout: 300 })}
                >
                    <label className="block text-[10px] font-semibold uppercase tracking-wide text-gray-500 mt-2">process_name</label>
                    <input
                        className="w-full rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-950 px-3 py-2 text-sm font-mono"
                        value={killName}
                        onChange={(e) => setKillName(e.target.value)}
                    />
                </PlaybookCard>

                <PlaybookCard
                    title="Block indicators (IP + domain)"
                    description="Push network blocks to the agent."
                    onRun={async () => {
                        await exec.mutateAsync({ command_type: 'block_ip', parameters: { ip, direction: 'both' }, timeout: 300 });
                        await exec.mutateAsync({ command_type: 'block_domain', parameters: { domain }, timeout: 300 });
                    }}
                    disabled={!ip.trim() || !domain.trim()}
                >
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 mt-2">
                        <div>
                            <label className="block text-[10px] font-semibold uppercase tracking-wide text-gray-500">ip</label>
                            <input
                                className="w-full rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-950 px-3 py-2 text-sm font-mono"
                                value={ip}
                                onChange={(e) => setIp(e.target.value)}
                            />
                        </div>
                        <div>
                            <label className="block text-[10px] font-semibold uppercase tracking-wide text-gray-500">domain</label>
                            <input
                                className="w-full rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-950 px-3 py-2 text-sm font-mono"
                                value={domain}
                                onChange={(e) => setDomain(e.target.value)}
                            />
                        </div>
                    </div>
                </PlaybookCard>
            </div>
        </div>
    );
}

export function ItsmAutomationsPage() {
    const queryClient = useQueryClient();
    const { showToast } = useToast();
    const canExec = authApi.canExecuteCommands();
    const [agentId, setAgentId] = React.useState('');
    const [busy, setBusy] = React.useState(false);

    const run = useMutation({
        mutationFn: async ({ command_type, parameters }: { command_type: CommandType; parameters?: Record<string, string> }) => {
            return agentsApi.executeCommand(agentId.trim(), { command_type, parameters: parameters ?? {}, timeout: 600 });
        },
        onSuccess: (data) => {
            showToast(`Automation step queued (${data.command_id})`, 'success');
            queryClient.invalidateQueries({ queryKey: ['commands'] });
            queryClient.invalidateQueries({ queryKey: ['agent-commands', agentId.trim()] });
        },
        onError: (e: Error) => showToast(e.message || 'Automation failed', 'error'),
    });

    const simpleAutomations: { id: string; title: string; steps: { t: CommandType; p?: Record<string, string> }[] }[] = [
        {
            id: 'isolate-collect',
            title: 'Containment: isolate + collect forensics',
            steps: [
                { t: 'isolate_network' },
                { t: 'collect_forensics', p: { event_types: 'process,file,network,dns,registry', max_events: '500' } },
            ],
        },
        {
            id: 'signatures-collect',
            title: 'Triage: update signatures + collect forensics',
            steps: [
                { t: 'update_signatures', p: { url: 'https://example.com/signatures.ndjson' } },
                { t: 'collect_forensics', p: { event_types: 'process,file,network', max_events: '300' } },
            ],
        },
    ];

    const execAutomation = async (steps: { t: CommandType; p?: Record<string, string> }[]) => {
        const aid = agentId.trim();
        if (!aid) {
            showToast('Enter an Agent ID first.', 'info');
            return;
        }
        if (!canExec) {
            showToast('Missing responses:execute permission.', 'error');
            return;
        }
        setBusy(true);
        try {
            for (const s of steps) {
                // eslint-disable-next-line no-await-in-loop
                await run.mutateAsync({ command_type: s.t, parameters: s.p });
            }
        } finally {
            setBusy(false);
        }
    };

    return (
        <div className="space-y-4">
            <div>
                <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Automations</h2>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    Lightweight multi-step response automations implemented on top of the command pipeline.
                </p>
            </div>

            <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40 p-4 space-y-3">
                <label className="block text-xs font-semibold text-gray-500 uppercase">Target agent_id</label>
                <input
                    className="w-full rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-950 px-3 py-2 text-sm font-mono"
                    value={agentId}
                    onChange={(e) => setAgentId(e.target.value)}
                    placeholder="UUID"
                    disabled={busy}
                />
                <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                    {simpleAutomations.map((a) => (
                        <button
                            key={a.id}
                            type="button"
                            className="rounded-xl border border-gray-200 dark:border-gray-700 p-4 text-left hover:bg-gray-50 dark:hover:bg-gray-800/50 disabled:opacity-50"
                            disabled={busy}
                            onClick={() => execAutomation(a.steps)}
                        >
                            <div className="font-semibold text-gray-900 dark:text-white">{a.title}</div>
                            <div className="text-xs text-gray-500 mt-1">Steps: {a.steps.map((s) => s.t).join(' → ')}</div>
                        </button>
                    ))}
                </div>
                <p className="text-xs text-gray-500">
                    Tip: for full response workflows and parameters, use <Link className="text-cyan-600 dark:text-cyan-400 hover:underline" to="/responses">Command Center</Link>.
                </p>
            </div>
        </div>
    );
}

export function ItsmIntegrationsPage() {
    return (
        <div className="space-y-4">
            <div>
                <h2 className="text-lg font-semibold text-gray-900 dark:text-white">ITSM — integrations</h2>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    Integrations will connect ticketing/chat/webhooks. For now, configure outbound destinations under{' '}
                    <Link className="text-cyan-600 dark:text-cyan-400 hover:underline" to="/security/siem-x">
                        SIEM connectors
                    </Link>
                    .
                </p>
            </div>
            <div className="rounded-xl border border-dashed border-gray-300 dark:border-gray-700 bg-white/50 dark:bg-gray-900/20 p-6 text-sm text-gray-600 dark:text-gray-400">
                Backend integration endpoints are not exposed yet. Once available, this page will manage credentials + test connections.
            </div>
        </div>
    );
}

export function ManagementDevicesPage() {
    return (
        <GenericParityView
            title="Device management"
            description="Aligned with `/management/devices` — can mirror `/api/v1/agents` later."
            missingApi="true"
            queryKey={['parity', 'management', 'devices']}
            fetcher={() => parityApi.getManagementDevices()}
            mock={mocks.mockManagementDevices}
        />
    );
}

/** Fleet addresses + isolation — from live `GET /api/v1/agents` (no separate network topology API yet). */
export function ManagementNetworkPage() {
    const q = useQuery({
        queryKey: ['management-network-fleet'],
        queryFn: () => agentsApi.list({ limit: 200, sort_by: 'hostname', sort_order: 'asc' }),
        staleTime: 30_000,
    });

    if (q.isLoading) {
        return (
            <div className="flex items-center justify-center py-16 text-gray-500 gap-2">
                <Loader2 className="w-6 h-6 animate-spin" /> Loading agents…
            </div>
        );
    }

    if (q.isError || !q.data?.data) {
        return (
            <div className="rounded-lg border border-rose-200 dark:border-rose-900/50 p-4 text-sm text-rose-800 dark:text-rose-200">
                Could not load agents. Check connection-manager and <code className="text-xs">endpoints:read</code>.
            </div>
        );
    }

    const rows = q.data.data;

    return (
        <div className="space-y-4">
            <div>
                <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Fleet connectivity</h2>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    Last-seen addresses reported by enrolled agents. Full network telemetry belongs in{' '}
                    <Link className="text-cyan-600 dark:text-cyan-400 hover:underline" to="/management/devices">
                        device detail → Network
                    </Link>
                    {' '}when event search is wired.
                </p>
            </div>
            <div className="overflow-x-auto rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40">
                <table className="min-w-full text-left text-sm">
                    <thead className="bg-gray-50 dark:bg-gray-800/80 text-gray-600 dark:text-gray-400 text-xs uppercase">
                        <tr>
                            <th className="px-3 py-2">Host</th>
                            <th className="px-3 py-2">Status</th>
                            <th className="px-3 py-2">IPs</th>
                            <th className="px-3 py-2">Isolated</th>
                        </tr>
                    </thead>
                    <tbody>
                        {rows.map((a) => (
                            <tr key={a.id} className="border-t border-gray-100 dark:border-gray-800">
                                <td className="px-3 py-2">
                                    <Link
                                        className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline"
                                        to={`/management/devices/${encodeURIComponent(a.id)}`}
                                    >
                                        {a.hostname}
                                    </Link>
                                </td>
                                <td className="px-3 py-2 font-mono text-xs">{a.status}</td>
                                <td className="px-3 py-2 text-xs break-all max-w-md">
                                    {(a.ip_addresses || []).join(', ') || '—'}
                                </td>
                                <td className="px-3 py-2">{a.is_isolated ? 'Yes' : 'No'}</td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>
        </div>
    );
}

export function ManagementStaffPage() {
    return <SelfHostedOutOfScope title="Staff / shifts" />;
}

export function ManagementAccountPage() {
    return <SelfHostedOutOfScope title="Account / tenant branding" />;
}

export function ManagementProfilesPage() {
    return (
        <GenericParityView
            title="Profile management"
            missingApi="true"
            queryKey={['parity', 'management', 'profiles']}
            fetcher={() => parityApi.getManagementProfiles()}
            mock={mocks.mockManagementProfiles.data}
        />
    );
}

export function ManagementRmmPage() {
    return <SelfHostedOutOfScope title="Remote monitoring & management (RMM)" />;
}

export function ManagementPatchPage() {
    return (
        <div className="space-y-6">
            <GenericParityView
                title="Patch — overview"
                missingApi="true"
                queryKey={['parity', 'patch', 'overview']}
                fetcher={() => parityApi.getPatchOverview()}
                mock={mocks.mockPatchOverview}
            />
            <GenericParityView
                title="Patch — missing"
                missingApi="true"
                queryKey={['parity', 'patch', 'missing']}
                fetcher={() => parityApi.getPatchMissing()}
                mock={mocks.mockPatchMissing.data}
            />
        </div>
    );
}

export function ManagementVulnPage() {
    const { showToast } = useToast();
    const queryClient = useQueryClient();
    const canExec = authApi.canExecuteCommands();

    const agentsQ = useQuery({ queryKey: ['agents', 'vuln'], queryFn: () => agentsApi.list({ limit: 200, sort_by: 'hostname', sort_order: 'asc' }), staleTime: 30_000 });
    const riskQ = useQuery({ queryKey: ['endpoint-risk'], queryFn: () => alertsApi.endpointRisk(), staleTime: 60_000, retry: 1 });

    const runForensics = useMutation({
        mutationFn: async (agentId: string) => {
            return agentsApi.executeCommand(agentId, {
                command_type: 'collect_forensics',
                parameters: { event_types: 'process,file,network,dns,registry', max_events: '500' },
                timeout: 900,
            });
        },
        onSuccess: (d) => {
            showToast(`Forensics queued (${d.command_id})`, 'success');
            queryClient.invalidateQueries({ queryKey: ['commands'] });
        },
        onError: (e: Error) => showToast(e.message || 'Failed to queue forensics', 'error'),
    });

    const agents = agentsQ.data?.data ?? [];
    const riskRows = riskQ.data?.data ?? [];
    const riskByAgent = new Map(riskRows.map((r) => [r.agent_id, r]));

    return (
        <div className="space-y-4">
            <div>
                <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Vulnerability</h2>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    No dedicated vulnerability scanner API exists yet. This page prioritizes <strong>triage</strong> by combining agent posture + open alerts,
                    and provides a one-click <code className="text-xs">collect_forensics</code> workflow.
                </p>
            </div>

            {(agentsQ.isLoading || riskQ.isLoading) && <div className="h-40 rounded-xl bg-gray-100 dark:bg-gray-800 animate-pulse" />}

            {agents.length > 0 && (
                <div className="overflow-x-auto rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40">
                    <table className="min-w-full text-left text-sm">
                        <thead className="bg-gray-50 dark:bg-gray-800/80 text-gray-600 dark:text-gray-400 text-xs uppercase">
                            <tr>
                                <th className="px-3 py-2">Host</th>
                                <th className="px-3 py-2">Status</th>
                                <th className="px-3 py-2">Last seen</th>
                                <th className="px-3 py-2">Open alerts</th>
                                <th className="px-3 py-2">Actions</th>
                            </tr>
                        </thead>
                        <tbody>
                            {agents.map((a: Agent) => {
                                const eff = getEffectiveStatus(a);
                                const openCount = riskByAgent.get(a.id)?.open_count ?? 0;
                                return (
                                    <tr key={a.id} className="border-t border-gray-100 dark:border-gray-800">
                                        <td className="px-3 py-2">
                                            <Link className="text-cyan-600 dark:text-cyan-400 font-medium hover:underline" to={`/management/devices/${encodeURIComponent(a.id)}`}>
                                                {a.hostname}
                                            </Link>
                                        </td>
                                        <td className="px-3 py-2 text-xs font-mono">{eff}</td>
                                        <td className="px-3 py-2 text-xs">{formatRelativeTime(a.last_seen)}</td>
                                        <td className="px-3 py-2 text-xs font-mono">{openCount}</td>
                                        <td className="px-3 py-2">
                                            <button
                                                type="button"
                                                disabled={!canExec || runForensics.isPending}
                                                className="px-2 py-1 rounded border border-gray-200 dark:border-gray-700 text-xs hover:bg-gray-50 dark:hover:bg-gray-800 disabled:opacity-50"
                                                onClick={() => runForensics.mutate(a.id)}
                                                title="Queue collect_forensics on this agent"
                                            >
                                                Collect forensics
                                            </button>
                                        </td>
                                    </tr>
                                );
                            })}
                        </tbody>
                    </table>
                </div>
            )}

            <p className="text-xs text-gray-500">
                Future: add server-side vulnerability findings table + agent scanner output ingestion. Until then, use Alerts/Events + forensics collection.
            </p>
        </div>
    );
}

export function ManagementAppControlPage() {
    const { showToast } = useToast();
    const queryClient = useQueryClient();
    const canExec = authApi.canExecuteCommands();
    const [agentId, setAgentId] = React.useState('');
    const [policyJson, setPolicyJson] = React.useState(
        JSON.stringify(
            {
                mode: 'audit',
                allow_paths: ['C:\\\\Program Files\\\\'],
                deny_hashes: [],
            },
            null,
            2
        )
    );

    const push = useMutation({
        mutationFn: async () => {
            const aid = agentId.trim();
            if (!aid) throw new Error('agent_id is required');
            JSON.parse(policyJson);
            return agentsApi.executeCommand(aid, {
                command_type: 'update_config',
                parameters: { app_control_policy_json: policyJson },
                timeout: 300,
            });
        },
        onSuccess: (d) => {
            showToast(`Policy pushed (Command ID: ${d.command_id})`, 'success');
            queryClient.invalidateQueries({ queryKey: ['agent-commands', agentId.trim()] });
        },
        onError: (e: Error) => showToast(e.message || 'Failed to push policy', 'error'),
    });

    return (
        <div className="space-y-4">
            <div>
                <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Application control policies</h2>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    MVP policy editor that pushes JSON to the agent via <code className="text-xs">update_config</code>. Agent-side enforcement is a future feature.
                </p>
            </div>

            <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40 p-4 space-y-3">
                <label className="block text-xs font-semibold text-gray-500 uppercase">Target agent_id</label>
                <input
                    className="w-full rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-950 px-3 py-2 text-sm font-mono"
                    value={agentId}
                    onChange={(e) => setAgentId(e.target.value)}
                    placeholder="UUID"
                />

                <label className="block text-xs font-semibold text-gray-500 uppercase">Policy JSON</label>
                <textarea
                    className="w-full rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-950 px-3 py-2 text-xs font-mono min-h-[220px]"
                    value={policyJson}
                    onChange={(e) => setPolicyJson(e.target.value)}
                    spellCheck={false}
                />

                <div className="flex justify-end gap-2">
                    <button
                        type="button"
                        disabled={!canExec || push.isPending}
                        onClick={() => push.mutate()}
                        className="px-3 py-2 rounded-lg text-sm font-semibold bg-cyan-600 hover:bg-cyan-700 text-white disabled:opacity-50"
                    >
                        {push.isPending ? 'Pushing…' : 'Push to agent'}
                    </button>
                </div>
            </div>
        </div>
    );
}

export function ManagementLicensesPage() {
    return <SelfHostedOutOfScope title="Licenses" />;
}

export function ManagementBillingPage() {
    return <SelfHostedOutOfScope title="Billing" />;
}

