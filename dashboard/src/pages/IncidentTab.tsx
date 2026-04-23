import { useState, useMemo } from 'react';
import { useQuery, useMutation } from '@tanstack/react-query';
import {
    AlertTriangle, CheckCircle2, Clock, Loader2, XCircle,
    ShieldAlert, Activity, HardDrive, Network, Lock,
    Database, Bug, ChevronDown, ChevronRight, RefreshCw,
    SkipForward, Bell, Flag, TrendingUp, Printer,
} from 'lucide-react';
import {
    incidentApi,
    type Agent,
    type IncidentData,
    type PlaybookStep,
    type TriageSnapshot,
    type IocEnrichment,
    type ProcessInfo,
    type PersistenceItem,
    type LsassAccessEvent,
    type TimelineFile,
    type NetConn,
    type DnsEntry,
    type AgentIntegrity,
    type PostIsolationAlert,
} from '../api/client';

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

const verdictColor = (verdict: string) => {
    switch (verdict) {
        case 'malicious': return 'text-red-500 bg-red-50 border-red-200';
        case 'suspicious': return 'text-amber-600 bg-amber-50 border-amber-200';
        case 'clean': return 'text-green-600 bg-green-50 border-green-200';
        default: return 'text-gray-500 bg-gray-50 border-gray-200';
    }
};

const stepStatusIcon = (status: string) => {
    switch (status) {
        case 'success': return <CheckCircle2 className="w-4 h-4 text-green-500" />;
        case 'failed': return <XCircle className="w-4 h-4 text-red-500" />;
        case 'running': return <Loader2 className="w-4 h-4 text-blue-500 animate-spin" />;
        case 'skipped': return <SkipForward className="w-4 h-4 text-gray-400" />;
        default: return <Clock className="w-4 h-4 text-gray-300" />;
    }
};

const stepDuration = (step: PlaybookStep): string => {
    if (!step.started_at) return '';
    const start = new Date(step.started_at).getTime();
    const end = step.finished_at ? new Date(step.finished_at).getTime() : Date.now();
    const secs = ((end - start) / 1000).toFixed(1);
    return `${secs}s`;
};

const snapshotForKind = (snapshots: TriageSnapshot[], kind: string): Record<string, unknown> | null => {
    const s = snapshots.find(s => s.kind === kind);
    return s ? s.payload : null;
};

function relTime(iso: string): string {
    const diff = Date.now() - new Date(iso).getTime();
    if (diff < 60000) return `${Math.round(diff / 1000)}s ago`;
    if (diff < 3600000) return `${Math.round(diff / 60000)}m ago`;
    return `${Math.round(diff / 3600000)}h ago`;
}

// ─────────────────────────────────────────────────────────────────────────────
// Sub-components
// ─────────────────────────────────────────────────────────────────────────────

function KpiCard({ label, value, color }: { label: string; value: number | string; color?: string }) {
    return (
        <div className={`bg-white rounded-lg border p-4 flex flex-col gap-1 ${color ?? 'border-gray-200'}`}>
            <span className="text-xs text-gray-500 font-medium uppercase tracking-wide">{label}</span>
            <span className={`text-2xl font-bold ${color ? color.replace('border-', 'text-') : 'text-gray-800'}`}>
                {value}
            </span>
        </div>
    );
}

function PlaybookTimeline({ steps }: { steps: PlaybookStep[] }) {
    return (
        <div className="flex flex-col gap-1">
            {steps.map(step => (
                <div key={step.id} className="flex items-center gap-3 px-3 py-2 rounded hover:bg-gray-50 transition-colors">
                    {stepStatusIcon(step.status)}
                    <span className="flex-1 text-sm font-medium text-gray-700 capitalize">
                        {step.step_name.replace(/_/g, ' ')}
                    </span>
                    {step.started_at && (
                        <span className="text-xs text-gray-400 font-mono">{stepDuration(step)}</span>
                    )}
                    {step.error && (
                        <span className="text-xs text-red-400 truncate max-w-[120px]" title={step.error}>
                            {step.error}
                        </span>
                    )}
                </div>
            ))}
            {steps.length === 0 && (
                <p className="text-xs text-gray-400 py-2 text-center">No steps yet</p>
            )}
        </div>
    );
}

// ── Interactive Process Tree ──────────────────────────────────────────────────

interface TreeNode {
    proc: ProcessInfo;
    children: TreeNode[];
}

function buildTree(procs: ProcessInfo[]): TreeNode[] {
    const map = new Map<number, TreeNode>();
    procs.forEach(p => map.set(p.pid, { proc: p, children: [] }));
    const roots: TreeNode[] = [];
    procs.forEach(p => {
        const parent = map.get(p.ppid ?? -1);
        if (parent && p.ppid !== p.pid) {
            parent.children.push(map.get(p.pid)!);
        } else {
            roots.push(map.get(p.pid)!);
        }
    });
    return roots;
}

function ProcessNode({
    node, depth, collapsed, onToggle,
}: {
    node: TreeNode; depth: number; collapsed: Set<number>; onToggle: (pid: number) => void;
}) {
    const [hovered, setHovered] = useState(false);
    const { proc } = node;
    const isCollapsed = collapsed.has(proc.pid);
    const hasChildren = node.children.length > 0;

    const rowClass = !proc.signed
        ? 'bg-red-50 border-l-2 border-red-400'
        : 'hover:bg-gray-50';

    return (
        <>
            <div
                className={`flex items-start gap-1 px-2 py-1 rounded text-xs cursor-pointer relative ${rowClass}`}
                style={{ paddingLeft: `${depth * 16 + 8}px` }}
                onClick={() => hasChildren && onToggle(proc.pid)}
                onMouseEnter={() => setHovered(true)}
                onMouseLeave={() => setHovered(false)}
            >
                <span className="shrink-0 w-3 mt-0.5">
                    {hasChildren
                        ? (isCollapsed ? <ChevronRight className="w-3 h-3 text-gray-400" /> : <ChevronDown className="w-3 h-3 text-gray-400" />)
                        : <span className="w-3 inline-block" />
                    }
                </span>
                <span className={`font-semibold mr-1 ${!proc.signed ? 'text-red-700' : 'text-gray-800'}`}>
                    {proc.name}
                </span>
                <span className="text-gray-400 font-mono mr-2">({proc.pid})</span>
                {proc.signed
                    ? <span className="text-green-500 text-xs">✓</span>
                    : <span className="text-red-500 font-bold text-xs">✗ unsigned</span>
                }
                {hovered && proc.sha256 && (
                    <span className="ml-2 text-gray-400 font-mono text-xs hidden lg:inline">
                        {proc.sha256.substring(0, 16)}…
                    </span>
                )}
                {hovered && proc.path && (
                    <span className="ml-2 text-gray-400 text-xs hidden xl:inline truncate max-w-[200px]">
                        {proc.path}
                    </span>
                )}
            </div>
            {!isCollapsed && node.children.map(child => (
                <ProcessNode
                    key={child.proc.pid}
                    node={child}
                    depth={depth + 1}
                    collapsed={collapsed}
                    onToggle={onToggle}
                />
            ))}
        </>
    );
}

function ProcessTreePanel({ snapshot }: { snapshot: Record<string, unknown> | null }) {
    const [collapsed, setCollapsed] = useState<Set<number>>(new Set());
    if (!snapshot) return <EmptyState label="Process tree not collected yet" />;

    const processes = (snapshot.processes as ProcessInfo[]) ?? [];
    const unsigned = processes.filter(p => !p.signed);
    const roots = useMemo(() => buildTree(processes), [processes]);

    const toggle = (pid: number) =>
        setCollapsed(prev => {
            const next = new Set(prev);
            next.has(pid) ? next.delete(pid) : next.add(pid);
            return next;
        });

    const collapseAll = () => setCollapsed(new Set(processes.map(p => p.pid)));
    const expandAll = () => setCollapsed(new Set());

    return (
        <div className="space-y-2">
            <div className="flex items-center justify-between">
                <p className="text-xs text-gray-500">
                    {processes.length} processes
                    {unsigned.length > 0 && (
                        <span className="ml-2 px-1.5 py-0.5 bg-red-100 text-red-600 rounded font-medium">
                            {unsigned.length} unsigned
                        </span>
                    )}
                </p>
                <div className="flex gap-1">
                    <button onClick={expandAll} className="text-xs text-blue-500 hover:underline">Expand all</button>
                    <span className="text-gray-300">·</span>
                    <button onClick={collapseAll} className="text-xs text-blue-500 hover:underline">Collapse all</button>
                </div>
            </div>
            <div className="overflow-auto max-h-96 border rounded-lg bg-white py-1">
                {roots.length === 0
                    ? <p className="text-xs text-center text-gray-400 py-4">No processes</p>
                    : roots.map(root => (
                        <ProcessNode
                            key={root.proc.pid}
                            node={root}
                            depth={0}
                            collapsed={collapsed}
                            onToggle={toggle}
                        />
                    ))
                }
            </div>
        </div>
    );
}

function PersistencePanel({ snapshot }: { snapshot: Record<string, unknown> | null }) {
    if (!snapshot) return <EmptyState label="Persistence scan not collected yet" />;
    const items = (snapshot.persistence_items as PersistenceItem[]) ?? [];

    return (
        <div className="overflow-auto max-h-96 border rounded-lg">
            <table className="w-full text-xs">
                <thead className="bg-gray-50 sticky top-0">
                    <tr>
                        <th className="text-left px-3 py-2 text-gray-600">Type</th>
                        <th className="text-left px-3 py-2 text-gray-600">Location</th>
                        <th className="text-left px-3 py-2 text-gray-600">Value</th>
                        <th className="text-left px-3 py-2 text-gray-600">SHA-256</th>
                    </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                    {items.map((item, i) => (
                        <tr key={i} className="hover:bg-gray-50">
                            <td className="px-3 py-1.5">
                                <span className="px-1.5 py-0.5 rounded text-xs bg-purple-100 text-purple-700 font-medium capitalize">
                                    {item.type.replace(/_/g, ' ')}
                                </span>
                            </td>
                            <td className="px-3 py-1.5 text-gray-500 truncate max-w-[140px]" title={item.location}>
                                {item.location}
                            </td>
                            <td className="px-3 py-1.5 font-mono truncate max-w-[200px]" title={item.value}>
                                {item.value}
                            </td>
                            <td className="px-3 py-1.5 font-mono text-gray-400">
                                {item.sha256 ? item.sha256.substring(0, 12) + '…' : '—'}
                            </td>
                        </tr>
                    ))}
                    {items.length === 0 && (
                        <tr><td colSpan={4} className="text-center py-4 text-gray-400">No persistence items found</td></tr>
                    )}
                </tbody>
            </table>
        </div>
    );
}

function LsassPanel({ snapshot }: { snapshot: Record<string, unknown> | null }) {
    if (!snapshot) return <EmptyState label="LSASS audit not collected yet" />;
    const events = (snapshot.lsass_accesses as LsassAccessEvent[]) ?? [];

    const isSuspicious = (mask: string) =>
        ['0x1010', '0x1410', '0x1438', '0x143a', '0x1fffff'].some(m =>
            mask?.toLowerCase().includes(m.toLowerCase())
        );

    return (
        <div className="space-y-2">
            {events.length > 0 && (
                <p className="text-xs text-amber-600 font-medium">
                    ⚠ {events.filter(e => isSuspicious(e.access_mask ?? '')).length} suspicious access(es) detected
                </p>
            )}
            <div className="overflow-auto max-h-80 border rounded-lg">
                <table className="w-full text-xs">
                    <thead className="bg-gray-50 sticky top-0">
                        <tr>
                            <th className="text-left px-3 py-2 text-gray-600">Time</th>
                            <th className="text-left px-3 py-2 text-gray-600">Event ID</th>
                            <th className="text-left px-3 py-2 text-gray-600">Actor PID</th>
                            <th className="text-left px-3 py-2 text-gray-600">Access Mask</th>
                        </tr>
                    </thead>
                    <tbody className="divide-y divide-gray-100">
                        {events.map((ev, i) => (
                            <tr key={i} className={isSuspicious(ev.access_mask ?? '') ? 'bg-red-50' : 'hover:bg-gray-50'}>
                                <td className="px-3 py-1.5 font-mono text-gray-500">{ev.time_created}</td>
                                <td className="px-3 py-1.5 font-mono">{ev.event_id}</td>
                                <td className="px-3 py-1.5 font-mono">{ev.actor_pid}</td>
                                <td className="px-3 py-1.5 font-mono">
                                    <span className={isSuspicious(ev.access_mask ?? '') ? 'text-red-600 font-bold' : ''}>
                                        {ev.access_mask ?? '—'}
                                    </span>
                                </td>
                            </tr>
                        ))}
                        {events.length === 0 && (
                            <tr><td colSpan={4} className="text-center py-4 text-gray-400">No LSASS access events found</td></tr>
                        )}
                    </tbody>
                </table>
            </div>
        </div>
    );
}

function NetworkPanel({ snapshot }: { snapshot: Record<string, unknown> | null }) {
    if (!snapshot) return <EmptyState label="Network last-seen not collected yet" />;
    const conns = (snapshot.tcp_conns as NetConn[]) ?? [];
    const dns = (snapshot.dns_cache as DnsEntry[]) ?? [];

    return (
        <div className="space-y-4">
            <div>
                <h4 className="text-xs font-semibold text-gray-600 uppercase tracking-wide mb-2">
                    TCP Connections ({conns.length})
                </h4>
                <div className="overflow-auto max-h-52 border rounded-lg">
                    <table className="w-full text-xs">
                        <thead className="bg-gray-50 sticky top-0">
                            <tr>
                                <th className="text-left px-3 py-2 text-gray-600">Proto</th>
                                <th className="text-left px-3 py-2 text-gray-600">Local</th>
                                <th className="text-left px-3 py-2 text-gray-600">Remote</th>
                                <th className="text-left px-3 py-2 text-gray-600">State</th>
                                <th className="text-left px-3 py-2 text-gray-600">PID</th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-gray-100">
                            {conns.slice(0, 100).map((c, i) => (
                                <tr key={i} className="hover:bg-gray-50">
                                    <td className="px-3 py-1.5 font-mono text-xs">{c.proto}</td>
                                    <td className="px-3 py-1.5 font-mono text-xs text-gray-500">{c.local_addr}</td>
                                    <td className="px-3 py-1.5 font-mono text-xs font-semibold">{c.remote_addr}</td>
                                    <td className="px-3 py-1.5 text-xs">{c.state}</td>
                                    <td className="px-3 py-1.5 font-mono text-xs text-gray-400">{c.pid}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            </div>

            <div>
                <h4 className="text-xs font-semibold text-gray-600 uppercase tracking-wide mb-2">
                    DNS Cache ({dns.length})
                </h4>
                <div className="overflow-auto max-h-48 border rounded-lg">
                    <table className="w-full text-xs">
                        <thead className="bg-gray-50 sticky top-0">
                            <tr>
                                <th className="text-left px-3 py-2 text-gray-600">Name</th>
                                <th className="text-left px-3 py-2 text-gray-600">Type</th>
                                <th className="text-left px-3 py-2 text-gray-600">Answer</th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-gray-100">
                            {dns.slice(0, 100).map((d, i) => (
                                <tr key={i} className="hover:bg-gray-50">
                                    <td className="px-3 py-1.5 font-mono">{d.name}</td>
                                    <td className="px-3 py-1.5 text-gray-500">{d.type}</td>
                                    <td className="px-3 py-1.5 font-mono text-gray-400">{d.answer ?? '—'}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    );
}

function FilesystemPanel({ snapshot }: { snapshot: Record<string, unknown> | null }) {
    if (!snapshot) return <EmptyState label="Filesystem timeline not collected yet" />;
    const files = (snapshot.files as TimelineFile[]) ?? [];

    return (
        <div className="overflow-auto max-h-96 border rounded-lg">
            <table className="w-full text-xs">
                <thead className="bg-gray-50 sticky top-0">
                    <tr>
                        <th className="text-left px-3 py-2 text-gray-600">Modified</th>
                        <th className="text-left px-3 py-2 text-gray-600">Path</th>
                        <th className="text-left px-3 py-2 text-gray-600">Size</th>
                        <th className="text-left px-3 py-2 text-gray-600">SHA-256</th>
                    </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                    {files.map((f, i) => (
                        <tr key={i} className="hover:bg-gray-50">
                            <td className="px-3 py-1.5 font-mono text-gray-500 whitespace-nowrap">{f.mtime}</td>
                            <td className="px-3 py-1.5 font-mono truncate max-w-[200px]" title={f.path}>{f.path}</td>
                            <td className="px-3 py-1.5 text-right text-gray-400">
                                {f.size_bytes > 1048576
                                    ? `${(f.size_bytes / 1048576).toFixed(1)}MB`
                                    : `${(f.size_bytes / 1024).toFixed(0)}KB`}
                            </td>
                            <td className="px-3 py-1.5 font-mono text-gray-400">
                                {f.sha256 ? f.sha256.substring(0, 12) + '…' : '—'}
                            </td>
                        </tr>
                    ))}
                    {files.length === 0 && (
                        <tr><td colSpan={4} className="text-center py-4 text-gray-400">No files modified in window</td></tr>
                    )}
                </tbody>
            </table>
        </div>
    );
}

function IocPanel({ iocs }: { iocs: IocEnrichment[] }) {
    if (iocs.length === 0) return <EmptyState label="No IOC enrichment data yet" />;

    return (
        <div className="overflow-auto max-h-96 border rounded-lg">
            <table className="w-full text-xs">
                <thead className="bg-gray-50 sticky top-0">
                    <tr>
                        <th className="text-left px-3 py-2 text-gray-600">Type</th>
                        <th className="text-left px-3 py-2 text-gray-600">Value</th>
                        <th className="text-left px-3 py-2 text-gray-600">Provider</th>
                        <th className="text-left px-3 py-2 text-gray-600">Verdict</th>
                        <th className="text-right px-3 py-2 text-gray-600">Score</th>
                    </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                    {iocs.map(ioc => (
                        <tr key={ioc.id} className="hover:bg-gray-50">
                            <td className="px-3 py-1.5">
                                <span className="px-1.5 py-0.5 rounded text-xs bg-blue-100 text-blue-700 font-medium uppercase">
                                    {ioc.ioc_type}
                                </span>
                            </td>
                            <td className="px-3 py-1.5 font-mono truncate max-w-[180px]" title={ioc.ioc_value}>
                                {ioc.ioc_value}
                            </td>
                            <td className="px-3 py-1.5 text-gray-500 capitalize">{ioc.provider}</td>
                            <td className="px-3 py-1.5">
                                <span className={`px-2 py-0.5 rounded-full border text-xs font-semibold capitalize ${verdictColor(ioc.verdict)}`}>
                                    {ioc.verdict}
                                </span>
                            </td>
                            <td className="px-3 py-1.5 text-right font-mono font-bold">
                                {ioc.verdict === 'malicious'
                                    ? <span className="text-red-600">{ioc.score}</span>
                                    : ioc.verdict === 'suspicious'
                                        ? <span className="text-amber-600">{ioc.score}</span>
                                        : ioc.score}
                            </td>
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    );
}

function IntegrityPanel({ snapshot }: { snapshot: Record<string, unknown> | null }) {
    if (!snapshot) return <EmptyState label="Agent integrity check not collected yet" />;
    const integrity = snapshot as unknown as AgentIntegrity;

    return (
        <div className="grid grid-cols-2 gap-3">
            <div className={`p-3 rounded-lg border ${integrity.signature_valid ? 'bg-green-50 border-green-200' : 'bg-red-50 border-red-200'}`}>
                <p className="text-xs text-gray-500 mb-1">Binary Signature</p>
                <p className={`text-sm font-bold ${integrity.signature_valid ? 'text-green-700' : 'text-red-700'}`}>
                    {integrity.signature_valid ? '✓ Valid' : '✗ Invalid'}
                </p>
            </div>
            <div className={`p-3 rounded-lg border ${integrity.etw_healthy ? 'bg-green-50 border-green-200' : 'bg-amber-50 border-amber-200'}`}>
                <p className="text-xs text-gray-500 mb-1">ETW Sessions</p>
                <p className={`text-sm font-bold ${integrity.etw_healthy ? 'text-green-700' : 'text-amber-700'}`}>
                    {integrity.etw_healthy ? '✓ Healthy' : '⚠ Degraded'}
                </p>
            </div>
            <div className="col-span-2 p-3 rounded-lg border border-gray-200">
                <p className="text-xs text-gray-500 mb-1">Agent Binary</p>
                <p className="text-xs font-mono text-gray-700 truncate">{integrity.exe_path}</p>
                {integrity.exe_sha256 && (
                    <p className="text-xs font-mono text-gray-400 mt-0.5">SHA-256: {integrity.exe_sha256.substring(0, 24)}…</p>
                )}
            </div>
        </div>
    );
}

function EmptyState({ label }: { label: string }) {
    return (
        <div className="py-8 text-center text-gray-400 text-sm">
            <Clock className="w-8 h-8 mx-auto mb-2 opacity-40" />
            {label}
        </div>
    );
}

// ── Post-Isolation Sigma Alerts Panel ─────────────────────────────────────────

const severityBadge = (sev: PostIsolationAlert['severity']) => {
    const map: Record<string, string> = {
        critical: 'bg-red-100 text-red-700 border-red-300',
        high:     'bg-orange-100 text-orange-700 border-orange-300',
        medium:   'bg-amber-100 text-amber-700 border-amber-300',
        low:      'bg-blue-100 text-blue-700 border-blue-300',
        informational: 'bg-gray-100 text-gray-600 border-gray-300',
    };
    return map[sev] ?? map.informational;
};

function AlertsPanel({ agentId, since }: { agentId: string; since?: string }) {
    const { data: alerts = [], isLoading } = useQuery<PostIsolationAlert[]>({
        queryKey: ['post-isolation-alerts', agentId, since],
        queryFn: () => incidentApi.listAlerts(agentId, since),
        refetchInterval: 30000,
    });

    if (isLoading) return (
        <div className="flex justify-center py-6"><Loader2 className="w-5 h-5 animate-spin text-blue-500" /></div>
    );
    if (alerts.length === 0) return <EmptyState label="No post-isolation Sigma alerts detected" />;

    return (
        <div className="space-y-1 overflow-auto max-h-96">
            {alerts.map(alert => (
                <div
                    key={alert.id}
                    className="flex items-start gap-3 px-3 py-2.5 rounded-lg border border-gray-100 hover:bg-gray-50 transition-colors"
                >
                    <span className={`mt-0.5 px-1.5 py-0.5 rounded text-xs font-bold border capitalize ${severityBadge(alert.severity)}`}>
                        {alert.severity}
                    </span>
                    <div className="flex-1 min-w-0">
                        <p className="text-xs font-semibold text-gray-800 truncate">{alert.title}</p>
                        {alert.rule_name && (
                            <p className="text-xs text-gray-400 font-mono">{alert.rule_name}</p>
                        )}
                    </div>
                    <div className="text-right shrink-0">
                        {alert.risk_score > 0 && (
                            <span className={`text-xs font-bold ${alert.risk_score >= 80 ? 'text-red-600' : alert.risk_score >= 50 ? 'text-amber-600' : 'text-gray-500'}`}>
                                {alert.risk_score}
                            </span>
                        )}
                        <p className="text-xs text-gray-400 mt-0.5">
                            {relTime(alert.detected_at)}
                        </p>
                    </div>
                </div>
            ))}
        </div>
    );
}

// ─────────────────────────────────────────────────────────────────────────────
// Main IncidentTab component
// ─────────────────────────────────────────────────────────────────────────────

interface IncidentTabProps {
    agent: Agent;
    onUnIsolate: () => void;
}

const DETAIL_PANELS = [
    { id: 'processes', label: 'Process Tree', icon: Activity },
    { id: 'persistence', label: 'Persistence', icon: Database },
    { id: 'lsass', label: 'LSASS Accesses', icon: Lock },
    { id: 'network', label: 'Network Last-Seen', icon: Network },
    { id: 'filesystem', label: 'File Timeline', icon: HardDrive },
    { id: 'iocs', label: 'IOC Enrichment', icon: Bug },
    { id: 'integrity', label: 'Agent Integrity', icon: ShieldAlert },
    { id: 'alerts', label: 'Sigma Alerts', icon: Bell },
] as const;

type PanelId = typeof DETAIL_PANELS[number]['id'];

export function IncidentTab({ agent, onUnIsolate }: IncidentTabProps) {
    const [activePanel, setActivePanel] = useState<PanelId>('processes');
    const [memoryDumpConfirm, setMemoryDumpConfirm] = useState(false);
    const [fpConfirm, setFpConfirm] = useState(false);
    const [escalateConfirm, setEscalateConfirm] = useState(false);

    // Poll every 2s while the playbook is running, 30s after completion.
    const { data: incident, isLoading, refetch } = useQuery<IncidentData>({
        queryKey: ['incident', agent.id],
        queryFn: () => incidentApi.getSummary(agent.id),
        refetchInterval: (query) => {
            const d = query.state.data as IncidentData | undefined;
            return d?.run?.status === 'running' ? 2000 : 30000;
        },
        enabled: true,
    });

    const { mutate: collectMemory, isPending: memoryLoading } = useMutation({
        mutationFn: () => incidentApi.collectMemory(agent.id),
        onSuccess: () => {
            setMemoryDumpConfirm(false);
            refetch();
        },
    });

    const { mutate: doMarkFp, isPending: fpLoading } = useMutation({
        mutationFn: () => incidentApi.markFalsePositive(agent.id),
        onSuccess: () => { setFpConfirm(false); refetch(); },
    });

    const { mutate: doEscalate, isPending: escalateLoading } = useMutation({
        mutationFn: () => incidentApi.escalate(agent.id),
        onSuccess: () => { setEscalateConfirm(false); refetch(); },
    });

    const successSteps = incident?.steps.filter(s => s.status === 'success').length ?? 0;
    const totalSteps = incident?.steps.length ?? 0;
    const maliciousIocs = incident?.iocs.filter(i => i.verdict === 'malicious').length ?? 0;
    const suspiciousIocs = incident?.iocs.filter(i => i.verdict === 'suspicious').length ?? 0;
    const isEscalated = (incident?.run?.summary as Record<string, unknown> | undefined)?.escalated === true;

    const processSnap = snapshotForKind(incident?.snapshots ?? [], 'process_tree_snapshot')
        ?? snapshotForKind(incident?.snapshots ?? [], 'post_isolation_triage');
    const persistenceSnap = snapshotForKind(incident?.snapshots ?? [], 'persistence_scan');
    const lsassSnap = snapshotForKind(incident?.snapshots ?? [], 'lsass_access_audit');
    const networkSnap = snapshotForKind(incident?.snapshots ?? [], 'network_last_seen');
    const filesystemSnap = snapshotForKind(incident?.snapshots ?? [], 'filesystem_timeline');
    const integritySnap = snapshotForKind(incident?.snapshots ?? [], 'agent_integrity_check');

    const persistenceItems = (persistenceSnap?.persistence_items as PersistenceItem[] | undefined) ?? [];
    const lsassAccesses = (lsassSnap?.lsass_accesses as LsassAccessEvent[] | undefined) ?? [];
    const timelineFiles = (filesystemSnap?.files as TimelineFile[] | undefined) ?? [];

    if (isLoading) {
        return (
            <div className="flex items-center justify-center py-16">
                <Loader2 className="w-8 h-8 animate-spin text-blue-500" />
                <span className="ml-3 text-gray-500">Loading incident data…</span>
            </div>
        );
    }

    return (
        <div className="space-y-4">
            {/* ── Header Banner ─────────────────────────────────────────── */}
            <div className="bg-red-50 border border-red-200 rounded-xl px-5 py-4 flex items-center gap-4">
                <AlertTriangle className="w-6 h-6 text-red-500 shrink-0" />
                <div className="flex-1 min-w-0">
                    <p className="text-sm font-bold text-red-800">
                        Endpoint Isolated — {agent.hostname}
                    </p>
                    <p className="text-xs text-red-600 mt-0.5">
                        Post-isolation playbook: {' '}
                        <span className="font-semibold">
                            {incident?.run?.status === 'running'
                                ? `Running ${successSteps}/${totalSteps} steps`
                                : incident?.run?.status
                                    ? `${incident.run.status} · ${successSteps}/${totalSteps} steps`
                                    : 'Waiting…'}
                        </span>
                    </p>
                </div>
                <div className="flex items-center gap-2">
                    <button
                        onClick={() => refetch()}
                        className="p-2 rounded-lg hover:bg-red-100 text-red-600 transition-colors"
                        title="Refresh"
                    >
                        <RefreshCw className="w-4 h-4" />
                    </button>
                    {!memoryDumpConfirm ? (
                        <button
                            onClick={() => setMemoryDumpConfirm(true)}
                            disabled={memoryLoading}
                            className="px-3 py-1.5 bg-amber-500 hover:bg-amber-600 text-white text-xs font-semibold rounded-lg transition-colors disabled:opacity-50"
                        >
                            {memoryLoading ? <Loader2 className="w-3 h-3 animate-spin" /> : 'Request Memory Dump'}
                        </button>
                    ) : (
                        <div className="flex items-center gap-1">
                            <span className="text-xs text-red-700 font-medium">Confirm dump?</span>
                            <button
                                onClick={() => collectMemory()}
                                className="px-2 py-1 bg-red-600 hover:bg-red-700 text-white text-xs font-bold rounded"
                            >Yes</button>
                            <button
                                onClick={() => setMemoryDumpConfirm(false)}
                                className="px-2 py-1 bg-gray-200 hover:bg-gray-300 text-gray-700 text-xs font-bold rounded"
                            >No</button>
                        </div>
                    )}
                    <button
                        onClick={onUnIsolate}
                        className="px-3 py-1.5 bg-green-600 hover:bg-green-700 text-white text-xs font-semibold rounded-lg transition-colors"
                    >
                        Un-isolate
                    </button>
                </div>
            </div>

            {/* ── KPI Cards ─────────────────────────────────────────────── */}
            <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
                <KpiCard
                    label="Persistence Items"
                    value={persistenceItems.length}
                    color={persistenceItems.length > 0 ? 'border-amber-300' : 'border-gray-200'}
                />
                <KpiCard
                    label="LSASS Accesses"
                    value={lsassAccesses.length}
                    color={lsassAccesses.length > 0 ? 'border-red-300' : 'border-gray-200'}
                />
                <KpiCard
                    label="Malicious IOCs"
                    value={maliciousIocs}
                    color={maliciousIocs > 0 ? 'border-red-400' : 'border-gray-200'}
                />
                <KpiCard
                    label="Files Modified"
                    value={timelineFiles.length}
                    color="border-gray-200"
                />
                <div
                    className="bg-white rounded-lg border border-gray-200 p-4 flex flex-col gap-1 cursor-pointer hover:border-purple-300 transition-colors"
                    onClick={() => setActivePanel('alerts')}
                    title="View post-isolation Sigma alerts"
                >
                    <span className="text-xs text-gray-500 font-medium uppercase tracking-wide">Sigma Alerts</span>
                    <span className="text-2xl font-bold text-purple-600 flex items-center gap-1">
                        <Bell className="w-4 h-4" />
                        <span>—</span>
                    </span>
                </div>
            </div>

            {/* ── Two-column layout: Timeline | Detail ──────────────────── */}
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
                {/* Playbook Timeline */}
                <div className="bg-white border border-gray-200 rounded-xl p-4">
                    <h3 className="text-sm font-semibold text-gray-700 mb-3 flex items-center gap-2">
                        <Activity className="w-4 h-4 text-blue-500" />
                        Playbook Steps
                    </h3>
                    <PlaybookTimeline steps={incident?.steps ?? []} />
                </div>

                {/* Detail panel */}
                <div className="lg:col-span-2 bg-white border border-gray-200 rounded-xl p-4">
                    {/* Panel tabs */}
                    <div className="flex flex-wrap gap-1 mb-4 border-b border-gray-100 pb-3">
                        {DETAIL_PANELS.map(panel => {
                            const Icon = panel.icon;
                            return (
                                <button
                                    key={panel.id}
                                    onClick={() => setActivePanel(panel.id)}
                                    className={`flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs font-medium transition-colors ${
                                        activePanel === panel.id
                                            ? 'bg-blue-100 text-blue-700'
                                            : 'text-gray-500 hover:text-gray-700 hover:bg-gray-100'
                                    }`}
                                >
                                    <Icon className="w-3.5 h-3.5" />
                                    {panel.label}
                                </button>
                            );
                        })}
                    </div>

                    {/* Active panel content */}
                    {activePanel === 'processes' && (
                        <ProcessTreePanel snapshot={processSnap} />
                    )}
                    {activePanel === 'persistence' && (
                        <PersistencePanel snapshot={persistenceSnap} />
                    )}
                    {activePanel === 'lsass' && (
                        <LsassPanel snapshot={lsassSnap} />
                    )}
                    {activePanel === 'network' && (
                        <NetworkPanel snapshot={networkSnap} />
                    )}
                    {activePanel === 'filesystem' && (
                        <FilesystemPanel snapshot={filesystemSnap} />
                    )}
                    {activePanel === 'iocs' && (
                        <IocPanel iocs={incident?.iocs ?? []} />
                    )}
                    {activePanel === 'integrity' && (
                        <IntegrityPanel snapshot={integritySnap} />
                    )}
                    {activePanel === 'alerts' && (
                        <AlertsPanel
                            agentId={agent.id}
                            since={incident?.run?.started_at}
                        />
                    )}
                </div>
            </div>

            {/* ── Run history footer ─────────────────────────────────────── */}
            {incident?.run && (
                <div className="text-xs text-gray-400 flex items-center gap-2">
                    <Clock className="w-3 h-3" />
                    Run started {relTime(incident.run.started_at)}
                    {incident.run.finished_at && ` · finished ${relTime(incident.run.finished_at)}`}
                    {suspiciousIocs > 0 && (
                        <span className="ml-3 px-2 py-0.5 bg-amber-100 text-amber-700 rounded-full font-medium">
                            {suspiciousIocs} suspicious IOC{suspiciousIocs > 1 ? 's' : ''}
                        </span>
                    )}
                    {isEscalated && (
                        <span className="ml-3 px-2 py-0.5 bg-red-100 text-red-700 rounded-full font-medium flex items-center gap-1">
                            <TrendingUp className="w-3 h-3" /> Escalated
                        </span>
                    )}
                </div>
            )}

            {/* ── Bottom Action Bar ──────────────────────────────────────── */}
            <div className="bg-white border border-gray-200 rounded-xl px-5 py-3 flex flex-wrap items-center gap-3 print:hidden">
                <span className="text-xs text-gray-400 font-medium mr-auto">Incident Actions</span>

                {/* Export / Print */}
                <button
                    onClick={() => window.print()}
                    className="flex items-center gap-1.5 px-3 py-1.5 bg-gray-100 hover:bg-gray-200 text-gray-700 text-xs font-semibold rounded-lg transition-colors"
                >
                    <Printer className="w-3.5 h-3.5" />
                    Export Report
                </button>

                {/* Escalate */}
                {!escalateConfirm ? (
                    <button
                        onClick={() => setEscalateConfirm(true)}
                        disabled={escalateLoading || isEscalated}
                        className="flex items-center gap-1.5 px-3 py-1.5 bg-orange-100 hover:bg-orange-200 text-orange-700 text-xs font-semibold rounded-lg transition-colors disabled:opacity-50"
                    >
                        <TrendingUp className="w-3.5 h-3.5" />
                        {isEscalated ? 'Escalated' : 'Escalate'}
                    </button>
                ) : (
                    <div className="flex items-center gap-1.5">
                        <span className="text-xs text-orange-700 font-medium">Escalate this incident?</span>
                        <button
                            onClick={() => doEscalate()}
                            disabled={escalateLoading}
                            className="px-2 py-1 bg-orange-600 hover:bg-orange-700 text-white text-xs font-bold rounded"
                        >
                            {escalateLoading ? <Loader2 className="w-3 h-3 animate-spin" /> : 'Yes'}
                        </button>
                        <button
                            onClick={() => setEscalateConfirm(false)}
                            className="px-2 py-1 bg-gray-200 hover:bg-gray-300 text-gray-700 text-xs font-bold rounded"
                        >No</button>
                    </div>
                )}

                {/* Mark as False Positive */}
                {!fpConfirm ? (
                    <button
                        onClick={() => setFpConfirm(true)}
                        disabled={fpLoading || incident?.run?.status === 'false_positive'}
                        className="flex items-center gap-1.5 px-3 py-1.5 bg-blue-50 hover:bg-blue-100 text-blue-700 text-xs font-semibold rounded-lg transition-colors disabled:opacity-50"
                    >
                        <Flag className="w-3.5 h-3.5" />
                        {incident?.run?.status === 'false_positive' ? 'Marked False Positive' : 'Mark as False Positive'}
                    </button>
                ) : (
                    <div className="flex items-center gap-1.5">
                        <span className="text-xs text-blue-700 font-medium">Mark as false positive?</span>
                        <button
                            onClick={() => doMarkFp()}
                            disabled={fpLoading}
                            className="px-2 py-1 bg-blue-600 hover:bg-blue-700 text-white text-xs font-bold rounded"
                        >
                            {fpLoading ? <Loader2 className="w-3 h-3 animate-spin" /> : 'Yes'}
                        </button>
                        <button
                            onClick={() => setFpConfirm(false)}
                            className="px-2 py-1 bg-gray-200 hover:bg-gray-300 text-gray-700 text-xs font-bold rounded"
                        >No</button>
                    </div>
                )}
            </div>
        </div>
    );
}

export default IncidentTab;
