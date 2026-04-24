import { useState, useMemo } from 'react';
import { useQuery, useMutation } from '@tanstack/react-query';
import {
    AlertTriangle, CheckCircle2, Clock, Loader2, XCircle,
    ShieldAlert, Activity, HardDrive, Network, Lock,
    Database, Bug, ChevronDown, ChevronRight, RefreshCw,
    SkipForward, Bell, Flag, TrendingUp, Printer, Shield,
    Cpu, GitBranch,
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
import StatCard from '../components/StatCard';
import { useToast } from '../components/Toast';

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

const verdictColor = (verdict: string) => {
    switch (verdict) {
        case 'malicious':   return 'badge badge-critical';
        case 'suspicious':  return 'badge badge-warning';
        case 'clean':       return 'badge badge-success';
        default:            return 'badge badge-info';
    }
};

const stepStatusIcon = (status: string) => {
    switch (status) {
        case 'success': return <CheckCircle2 className="w-4 h-4 text-emerald-500" />;
        case 'failed':  return <XCircle      className="w-4 h-4 text-rose-500" />;
        case 'running': return <Loader2      className="w-4 h-4 text-cyan-500 animate-spin" />;
        case 'skipped': return <SkipForward  className="w-4 h-4 text-slate-400" />;
        default:        return <Clock        className="w-4 h-4 text-slate-300" />;
    }
};

const stepDuration = (step: PlaybookStep): string => {
    if (!step.started_at) return '';
    const start = new Date(step.started_at).getTime();
    const end = step.finished_at ? new Date(step.finished_at).getTime() : Date.now();
    return `${((end - start) / 1000).toFixed(1)}s`;
};

const snapshotForKind = (
    snapshots: TriageSnapshot[],
    kind: string,
): Record<string, unknown> | null => {
    const s = snapshots.find(s => s.kind === kind);
    return s ? s.payload : null;
};

function relTime(iso: string): string {
    const diff = Date.now() - new Date(iso).getTime();
    if (diff < 60_000)   return `${Math.round(diff / 1000)}s ago`;
    if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
    return `${Math.round(diff / 3_600_000)}h ago`;
}

// ─────────────────────────────────────────────────────────────────────────────
// Shared table styles
// ─────────────────────────────────────────────────────────────────────────────

const TH = 'py-2.5 px-3 text-[11px] font-semibold uppercase tracking-wide text-slate-600 dark:text-slate-400 whitespace-nowrap text-left';
const TD = 'py-2 px-3 text-xs align-top';
const THEAD = 'sticky top-0 z-10 bg-slate-50 dark:bg-slate-800 border-b border-slate-200 dark:border-slate-700';
const TR    = 'border-b border-slate-100 dark:border-slate-800/60 hover:bg-slate-50/70 dark:hover:bg-slate-800/40 transition-colors';
const TABLE_WRAP = 'overflow-auto border border-slate-200 dark:border-slate-700 rounded-xl bg-white dark:bg-slate-900/50';

// ─────────────────────────────────────────────────────────────────────────────
// Playbook Timeline
// ─────────────────────────────────────────────────────────────────────────────

function PlaybookTimeline({ steps }: { steps: PlaybookStep[] }) {
    return (
        <div className="flex flex-col gap-0.5">
            {steps.map(step => (
                <div
                    key={step.id}
                    className="flex items-center gap-2.5 px-3 py-2 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-800/40 transition-colors"
                >
                    {stepStatusIcon(step.status)}
                    <span className="flex-1 text-xs font-medium text-slate-700 dark:text-slate-300 capitalize">
                        {step.step_name.replace(/_/g, ' ')}
                    </span>
                    {step.started_at && (
                        <span className="text-[10px] text-slate-400 font-mono tabular-nums">
                            {stepDuration(step)}
                        </span>
                    )}
                    {step.error && (
                        <span className="text-[10px] text-rose-400 truncate max-w-[100px]" title={step.error}>
                            {step.error}
                        </span>
                    )}
                </div>
            ))}
            {steps.length === 0 && (
                <p className="text-xs text-slate-400 py-3 text-center">No steps yet</p>
            )}
        </div>
    );
}

// ─────────────────────────────────────────────────────────────────────────────
// Interactive Process Tree
// ─────────────────────────────────────────────────────────────────────────────

interface TreeNode { proc: ProcessInfo; children: TreeNode[] }

function buildTree(procs: ProcessInfo[]): TreeNode[] {
    const map = new Map<number, TreeNode>();
    procs.forEach(p => map.set(p.pid, { proc: p, children: [] }));
    const roots: TreeNode[] = [];
    procs.forEach(p => {
        const parent = map.get(p.ppid ?? -1);
        if (parent && p.ppid !== p.pid) parent.children.push(map.get(p.pid)!);
        else roots.push(map.get(p.pid)!);
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

    const rowCls = !proc.signed
        ? 'bg-rose-50 dark:bg-rose-900/10 border-l-2 border-rose-400'
        : 'hover:bg-slate-50 dark:hover:bg-slate-800/30';

    return (
        <>
            <div
                className={`flex items-center gap-1 py-1 rounded text-xs cursor-default relative ${rowCls}`}
                style={{ paddingLeft: `${depth * 16 + 8}px` }}
                onClick={() => hasChildren && onToggle(proc.pid)}
                onMouseEnter={() => setHovered(true)}
                onMouseLeave={() => setHovered(false)}
            >
                <span className="shrink-0 w-3">
                    {hasChildren
                        ? (isCollapsed
                            ? <ChevronRight className="w-3 h-3 text-slate-400 cursor-pointer" />
                            : <ChevronDown  className="w-3 h-3 text-slate-400 cursor-pointer" />)
                        : <span className="w-3 inline-block" />}
                </span>
                <span className={`font-semibold mr-1 ${!proc.signed ? 'text-rose-700 dark:text-rose-400' : 'text-slate-800 dark:text-slate-200'}`}>
                    {proc.name}
                </span>
                <span className="text-slate-400 font-mono mr-1">({proc.pid})</span>
                {proc.signed
                    ? <span className="text-emerald-500 text-[10px]">✓</span>
                    : <span className="badge badge-critical text-[9px]">unsigned</span>}
                {hovered && proc.sha256 && (
                    <span className="ml-2 text-slate-400 font-mono text-[10px] hidden lg:inline">
                        {proc.sha256.substring(0, 16)}…
                    </span>
                )}
                {hovered && proc.path && (
                    <span className="ml-2 text-slate-400 text-[10px] hidden xl:inline truncate max-w-[200px]">
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
    const unsigned  = processes.filter(p => !p.signed);
    const roots     = useMemo(() => buildTree(processes), [processes]);

    const toggle     = (pid: number) => setCollapsed((prev: Set<number>) => { const n = new Set(prev); n.has(pid) ? n.delete(pid) : n.add(pid); return n; });
    const collapseAll = () => setCollapsed(new Set(processes.map(p => p.pid)));
    const expandAll   = () => setCollapsed(new Set());

    return (
        <div className="space-y-2">
            <div className="flex items-center justify-between">
                <p className="text-xs text-slate-500 dark:text-slate-400">
                    {processes.length} processes
                    {unsigned.length > 0 && (
                        <span className="ml-2 badge badge-critical">{unsigned.length} unsigned</span>
                    )}
                </p>
                <div className="flex gap-2 text-xs">
                    <button onClick={expandAll}   className="text-cyan-600 hover:underline">Expand all</button>
                    <span className="text-slate-300">·</span>
                    <button onClick={collapseAll} className="text-cyan-600 hover:underline">Collapse all</button>
                </div>
            </div>
            <div className={`${TABLE_WRAP} max-h-96 py-1`}>
                {roots.length === 0
                    ? <p className="text-xs text-center text-slate-400 py-4">No processes</p>
                    : roots.map(root => (
                        <ProcessNode
                            key={root.proc.pid}
                            node={root}
                            depth={0}
                            collapsed={collapsed}
                            onToggle={toggle}
                        />
                    ))}
            </div>
        </div>
    );
}

// ─────────────────────────────────────────────────────────────────────────────
// Persistence Panel
// ─────────────────────────────────────────────────────────────────────────────

function PersistencePanel({ snapshot }: { snapshot: Record<string, unknown> | null }) {
    if (!snapshot) return <EmptyState label="Persistence scan not collected yet" />;
    const items = (snapshot.persistence_items as PersistenceItem[]) ?? [];

    return (
        <div className={`${TABLE_WRAP} max-h-96`}>
            <table className="w-full">
                <thead className={THEAD}><tr>
                    <th className={TH}>Type</th>
                    <th className={TH}>Location</th>
                    <th className={TH}>Value</th>
                    <th className={TH}>SHA-256</th>
                </tr></thead>
                <tbody>
                    {items.map((item, i) => (
                        <tr key={i} className={TR}>
                            <td className={TD}>
                                <span className="badge" style={{ background: 'rgb(139 92 246 / 0.12)', color: '#7c3aed', borderColor: 'rgb(139 92 246 / 0.25)' }}>
                                    {item.type.replace(/_/g, ' ')}
                                </span>
                            </td>
                            <td className={`${TD} text-slate-500 truncate max-w-[140px]`} title={item.location}>{item.location}</td>
                            <td className={`${TD} font-mono truncate max-w-[200px]`} title={item.value}>{item.value}</td>
                            <td className={`${TD} font-mono text-slate-400`}>{item.sha256 ? item.sha256.substring(0, 12) + '…' : '—'}</td>
                        </tr>
                    ))}
                    {items.length === 0 && (
                        <tr><td colSpan={4} className="text-center py-8 text-slate-400 text-xs">No persistence items found</td></tr>
                    )}
                </tbody>
            </table>
        </div>
    );
}

// ─────────────────────────────────────────────────────────────────────────────
// LSASS Panel
// ─────────────────────────────────────────────────────────────────────────────

function LsassPanel({ snapshot }: { snapshot: Record<string, unknown> | null }) {
    if (!snapshot) return <EmptyState label="LSASS audit not collected yet" />;
    const events = (snapshot.lsass_accesses as LsassAccessEvent[]) ?? [];

    const isSuspicious = (mask: string) =>
        ['0x1010', '0x1410', '0x1438', '0x143a', '0x1fffff'].some(m =>
            mask?.toLowerCase().includes(m.toLowerCase())
        );

    const suspCount = events.filter(e => isSuspicious(e.access_mask ?? '')).length;

    return (
        <div className="space-y-2">
            {suspCount > 0 && (
                <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-rose-50 dark:bg-rose-900/20 border border-rose-200 dark:border-rose-800/40 text-rose-700 dark:text-rose-400 text-xs font-medium">
                    <AlertTriangle className="w-3.5 h-3.5 shrink-0" />
                    {suspCount} suspicious access pattern{suspCount > 1 ? 's' : ''} detected (possible credential dump)
                </div>
            )}
            <div className={`${TABLE_WRAP} max-h-80`}>
                <table className="w-full">
                    <thead className={THEAD}><tr>
                        <th className={TH}>Time</th>
                        <th className={TH}>Event ID</th>
                        <th className={TH}>Actor PID</th>
                        <th className={TH}>Access Mask</th>
                    </tr></thead>
                    <tbody>
                        {events.map((ev, i) => (
                            <tr key={i} className={isSuspicious(ev.access_mask ?? '') ? 'bg-rose-50 dark:bg-rose-900/10 border-b border-rose-100 dark:border-rose-900/30' : TR}>
                                <td className={`${TD} font-mono text-slate-500`}>{ev.time_created}</td>
                                <td className={`${TD} font-mono`}>{ev.event_id}</td>
                                <td className={`${TD} font-mono`}>{ev.actor_pid}</td>
                                <td className={TD}>
                                    <span className={isSuspicious(ev.access_mask ?? '') ? 'font-mono font-bold text-rose-600 dark:text-rose-400' : 'font-mono'}>
                                        {ev.access_mask ?? '—'}
                                    </span>
                                </td>
                            </tr>
                        ))}
                        {events.length === 0 && (
                            <tr><td colSpan={4} className="text-center py-8 text-slate-400 text-xs">No LSASS access events found</td></tr>
                        )}
                    </tbody>
                </table>
            </div>
        </div>
    );
}

// ─────────────────────────────────────────────────────────────────────────────
// Network Panel
// ─────────────────────────────────────────────────────────────────────────────

function NetworkPanel({ snapshot }: { snapshot: Record<string, unknown> | null }) {
    if (!snapshot) return <EmptyState label="Network last-seen not collected yet" />;
    const conns = (snapshot.tcp_conns as NetConn[]) ?? [];
    const dns   = (snapshot.dns_cache as DnsEntry[]) ?? [];

    return (
        <div className="space-y-4 animate-slide-up-fade">
            <div>
                <p className="text-[11px] font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wide mb-2">
                    TCP Connections ({conns.length})
                </p>
                <div className={`${TABLE_WRAP} max-h-52`}>
                    <table className="w-full">
                        <thead className={THEAD}><tr>
                            <th className={TH}>Proto</th>
                            <th className={TH}>Local</th>
                            <th className={TH}>Remote</th>
                            <th className={TH}>State</th>
                            <th className={TH}>PID</th>
                        </tr></thead>
                        <tbody>
                            {conns.slice(0, 100).map((c, i) => (
                                <tr key={i} className={TR}>
                                    <td className={`${TD} font-mono`}>{c.proto}</td>
                                    <td className={`${TD} font-mono text-slate-500`}>{c.local_addr}</td>
                                    <td className={`${TD} font-mono font-semibold text-slate-800 dark:text-slate-200`}>{c.remote_addr}</td>
                                    <td className={TD}>{c.state}</td>
                                    <td className={`${TD} font-mono text-slate-400`}>{c.pid}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            </div>
            <div>
                <p className="text-[11px] font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wide mb-2">
                    DNS Cache ({dns.length})
                </p>
                <div className={`${TABLE_WRAP} max-h-48`}>
                    <table className="w-full">
                        <thead className={THEAD}><tr>
                            <th className={TH}>Name</th>
                            <th className={TH}>Type</th>
                            <th className={TH}>Answer</th>
                        </tr></thead>
                        <tbody>
                            {dns.slice(0, 100).map((d, i) => (
                                <tr key={i} className={TR}>
                                    <td className={`${TD} font-mono`}>{d.name}</td>
                                    <td className={`${TD} text-slate-500`}>{d.type}</td>
                                    <td className={`${TD} font-mono text-slate-400`}>{d.answer ?? '—'}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    );
}

// ─────────────────────────────────────────────────────────────────────────────
// Filesystem Timeline Panel
// ─────────────────────────────────────────────────────────────────────────────

function FilesystemPanel({ snapshot }: { snapshot: Record<string, unknown> | null }) {
    if (!snapshot) return <EmptyState label="Filesystem timeline not collected yet" />;
    const files = (snapshot.files as TimelineFile[]) ?? [];

    return (
        <div className={`${TABLE_WRAP} max-h-96`}>
            <table className="w-full">
                <thead className={THEAD}><tr>
                    <th className={TH}>Modified</th>
                    <th className={TH}>Path</th>
                    <th className={`${TH} text-right`}>Size</th>
                    <th className={TH}>SHA-256</th>
                </tr></thead>
                <tbody>
                    {files.map((f, i) => (
                        <tr key={i} className={TR}>
                            <td className={`${TD} font-mono text-slate-500 whitespace-nowrap`}>{f.mtime}</td>
                            <td className={`${TD} font-mono truncate max-w-[200px]`} title={f.path}>{f.path}</td>
                            <td className={`${TD} text-right font-mono text-slate-400`}>
                                {f.size_bytes > 1_048_576
                                    ? `${(f.size_bytes / 1_048_576).toFixed(1)} MB`
                                    : `${(f.size_bytes / 1024).toFixed(0)} KB`}
                            </td>
                            <td className={`${TD} font-mono text-slate-400`}>
                                {f.sha256 ? f.sha256.substring(0, 12) + '…' : '—'}
                            </td>
                        </tr>
                    ))}
                    {files.length === 0 && (
                        <tr><td colSpan={4} className="text-center py-8 text-slate-400 text-xs">No files modified in window</td></tr>
                    )}
                </tbody>
            </table>
        </div>
    );
}

// ─────────────────────────────────────────────────────────────────────────────
// IOC Panel
// ─────────────────────────────────────────────────────────────────────────────

function IocPanel({ iocs }: { iocs: IocEnrichment[] }) {
    if (iocs.length === 0) return <EmptyState label="No IOC enrichment data yet" />;

    return (
        <div className={`${TABLE_WRAP} max-h-96`}>
            <table className="w-full">
                <thead className={THEAD}><tr>
                    <th className={TH}>Type</th>
                    <th className={TH}>Value</th>
                    <th className={TH}>Provider</th>
                    <th className={TH}>Verdict</th>
                    <th className={`${TH} text-right`}>Score</th>
                </tr></thead>
                <tbody>
                    {iocs.map(ioc => (
                        <tr key={ioc.id} className={TR}>
                            <td className={TD}>
                                <span className="badge badge-info uppercase">{ioc.ioc_type}</span>
                            </td>
                            <td className={`${TD} font-mono truncate max-w-[180px]`} title={ioc.ioc_value}>
                                {ioc.ioc_value}
                            </td>
                            <td className={`${TD} text-slate-500 capitalize`}>{ioc.provider}</td>
                            <td className={TD}>
                                <span className={verdictColor(ioc.verdict)}>{ioc.verdict}</span>
                            </td>
                            <td className={`${TD} text-right font-mono font-bold`}>
                                <span className={
                                    ioc.verdict === 'malicious'  ? 'text-rose-600 dark:text-rose-400' :
                                    ioc.verdict === 'suspicious' ? 'text-amber-600 dark:text-amber-400' :
                                    'text-slate-500'
                                }>{ioc.score}</span>
                            </td>
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    );
}

// ─────────────────────────────────────────────────────────────────────────────
// Agent Integrity Panel
// ─────────────────────────────────────────────────────────────────────────────

function IntegrityPanel({ snapshot }: { snapshot: Record<string, unknown> | null }) {
    if (!snapshot) return <EmptyState label="Agent integrity check not collected yet" />;
    const integrity = snapshot as unknown as AgentIntegrity;

    return (
        <div className="grid grid-cols-2 gap-3">
            {[
                {
                    label: 'Binary Signature',
                    ok: integrity.signature_valid,
                    good: '✓ Valid',
                    bad: '✗ Invalid',
                },
                {
                    label: 'ETW Sessions',
                    ok: integrity.etw_healthy,
                    good: '✓ Healthy',
                    bad: '⚠ Degraded',
                },
            ].map(item => (
                <div
                    key={item.label}
                    className={`p-3 rounded-xl border ${item.ok
                        ? 'bg-emerald-50 dark:bg-emerald-900/20 border-emerald-200 dark:border-emerald-800/40'
                        : 'bg-rose-50 dark:bg-rose-900/20 border-rose-200 dark:border-rose-800/40'}`}
                >
                    <p className="text-[11px] text-slate-500 dark:text-slate-400 mb-1">{item.label}</p>
                    <p className={`text-sm font-bold ${item.ok
                        ? 'text-emerald-700 dark:text-emerald-400'
                        : 'text-rose-700 dark:text-rose-400'}`}>
                        {item.ok ? item.good : item.bad}
                    </p>
                </div>
            ))}
            <div className="col-span-2 p-3 rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/40">
                <p className="text-[11px] text-slate-500 dark:text-slate-400 mb-1">Agent Binary</p>
                <p className="text-xs font-mono text-slate-700 dark:text-slate-300 truncate">{integrity.exe_path}</p>
                {integrity.exe_sha256 && (
                    <p className="text-[10px] font-mono text-slate-400 mt-0.5">
                        SHA-256: {integrity.exe_sha256.substring(0, 24)}…
                    </p>
                )}
            </div>
        </div>
    );
}

// ─────────────────────────────────────────────────────────────────────────────
// Post-Isolation Sigma Alerts Panel
// ─────────────────────────────────────────────────────────────────────────────

const SEV_BADGE: Record<string, string> = {
    critical:      'badge badge-critical',
    high:          'badge badge-high',
    medium:        'badge badge-medium',
    low:           'badge badge-low',
    informational: 'badge badge-info',
};

function AlertsPanel({ agentId, since }: { agentId: string; since?: string }) {
    const { data: alerts = [], isLoading } = useQuery<PostIsolationAlert[]>({
        queryKey: ['post-isolation-alerts', agentId, since],
        queryFn:  () => incidentApi.listAlerts(agentId, since),
        refetchInterval: 30_000,
    });

    if (isLoading) return (
        <div className="flex justify-center py-8">
            <Loader2 className="w-5 h-5 animate-spin text-cyan-500" />
        </div>
    );
    if (alerts.length === 0) return <EmptyState label="No post-isolation Sigma alerts detected" />;

    return (
        <div className={`${TABLE_WRAP} max-h-96`}>
            <table className="w-full">
                <thead className={THEAD}><tr>
                    <th className={TH}>Severity</th>
                    <th className={TH}>Title</th>
                    <th className={TH}>Rule</th>
                    <th className={`${TH} text-right`}>Risk</th>
                    <th className={TH}>When</th>
                </tr></thead>
                <tbody>
                    {alerts.map(alert => (
                        <tr key={alert.id} className={TR}>
                            <td className={TD}>
                                <span className={SEV_BADGE[alert.severity] ?? 'badge badge-info'}>
                                    {alert.severity}
                                </span>
                            </td>
                            <td className={`${TD} font-medium text-slate-800 dark:text-slate-200 max-w-[180px] truncate`} title={alert.title}>
                                {alert.title}
                            </td>
                            <td className={`${TD} font-mono text-slate-400 truncate max-w-[120px]`} title={alert.rule_name}>
                                {alert.rule_name ?? '—'}
                            </td>
                            <td className={`${TD} text-right font-mono font-bold`}>
                                <span className={
                                    alert.risk_score >= 80 ? 'text-rose-600 dark:text-rose-400' :
                                    alert.risk_score >= 50 ? 'text-amber-600 dark:text-amber-400' :
                                    'text-slate-500'
                                }>{alert.risk_score || '—'}</span>
                            </td>
                            <td className={`${TD} text-slate-400 whitespace-nowrap`}>
                                {relTime(alert.detected_at)}
                            </td>
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    );
}

// ─────────────────────────────────────────────────────────────────────────────
// Empty State
// ─────────────────────────────────────────────────────────────────────────────

function EmptyState({ label }: { label: string }) {
    return (
        <div className="flex flex-col items-center justify-center py-10 text-center">
            <Clock className="w-8 h-8 text-slate-300 dark:text-slate-600 mb-2" />
            <p className="text-sm text-slate-400 dark:text-slate-500">{label}</p>
        </div>
    );
}

// ─────────────────────────────────────────────────────────────────────────────
// Main IncidentTab
// ─────────────────────────────────────────────────────────────────────────────

interface IncidentTabProps {
    agent: Agent;
    onUnIsolate: () => void;
}

const DETAIL_PANELS = [
    { id: 'processes',   label: 'Process Tree',      icon: GitBranch  },
    { id: 'persistence', label: 'Persistence',        icon: Database   },
    { id: 'lsass',       label: 'LSASS Accesses',     icon: Lock       },
    { id: 'network',     label: 'Network Last-Seen',  icon: Network    },
    { id: 'filesystem',  label: 'File Timeline',      icon: HardDrive  },
    { id: 'iocs',        label: 'IOC Enrichment',     icon: Bug        },
    { id: 'integrity',   label: 'Agent Integrity',    icon: ShieldAlert},
    { id: 'alerts',      label: 'Sigma Alerts',       icon: Bell       },
] as const;

type PanelId = typeof DETAIL_PANELS[number]['id'];

export function IncidentTab({ agent, onUnIsolate }: IncidentTabProps) {
    const { showToast } = useToast();

    const [activePanel, setActivePanel] = useState<PanelId>('processes');
    const [memoryDumpConfirm, setMemoryDumpConfirm] = useState(false);
    const [fpConfirm,         setFpConfirm]         = useState(false);
    const [escalateConfirm,   setEscalateConfirm]   = useState(false);

    // ── Data queries ──────────────────────────────────────────────────────────

    const { data: incident, isLoading, refetch } = useQuery<IncidentData>({
        queryKey: ['incident', agent.id],
        queryFn:  () => incidentApi.getSummary(agent.id),
        refetchInterval: (query: { state: { data: unknown } }) => {
            const d = query.state.data as IncidentData | undefined;
            return d?.run?.status === 'running' ? 2000 : 30_000;
        },
    });

    // ── Mutations ─────────────────────────────────────────────────────────────

    const { mutate: collectMemory, isPending: memoryLoading } = useMutation({
        mutationFn: () => incidentApi.collectMemory(agent.id),
        onSuccess: () => {
            setMemoryDumpConfirm(false);
            showToast('Memory dump command sent to agent', 'success');
            refetch();
        },
        onError: () => showToast('Failed to trigger memory dump', 'error'),
    });

    const { mutate: doMarkFp, isPending: fpLoading } = useMutation({
        mutationFn: () => incidentApi.markFalsePositive(agent.id),
        onSuccess: () => {
            setFpConfirm(false);
            showToast('Incident marked as false positive', 'success');
            refetch();
        },
        onError: () => showToast('Failed to mark as false positive', 'error'),
    });

    const { mutate: doEscalate, isPending: escalateLoading } = useMutation({
        mutationFn: () => incidentApi.escalate(agent.id),
        onSuccess: () => {
            setEscalateConfirm(false);
            showToast('Incident escalated — severity set to high', 'warning');
            refetch();
        },
        onError: () => showToast('Failed to escalate incident', 'error'),
    });

    // ── Derived values ────────────────────────────────────────────────────────

    const successSteps  = incident?.steps.filter(s => s.status === 'success').length ?? 0;
    const totalSteps    = incident?.steps.length ?? 0;
    const maliciousIocs = incident?.iocs.filter(i => i.verdict === 'malicious').length ?? 0;
    const suspiciousIocs = incident?.iocs.filter(i => i.verdict === 'suspicious').length ?? 0;
    const isEscalated   = (incident?.run?.summary as Record<string, unknown> | undefined)?.escalated === true;
    const isFalsePos    = incident?.run?.status === 'false_positive';

    const processSnap   = snapshotForKind(incident?.snapshots ?? [], 'process_tree_snapshot')
                       ?? snapshotForKind(incident?.snapshots ?? [], 'post_isolation_triage');
    const persistenceSnap = snapshotForKind(incident?.snapshots ?? [], 'persistence_scan');
    const lsassSnap     = snapshotForKind(incident?.snapshots ?? [], 'lsass_access_audit');
    const networkSnap   = snapshotForKind(incident?.snapshots ?? [], 'network_last_seen');
    const filesystemSnap = snapshotForKind(incident?.snapshots ?? [], 'filesystem_timeline');
    const integritySnap = snapshotForKind(incident?.snapshots ?? [], 'agent_integrity_check');

    const persistenceItems = (persistenceSnap?.persistence_items as PersistenceItem[] | undefined) ?? [];
    const lsassAccesses    = (lsassSnap?.lsass_accesses as LsassAccessEvent[] | undefined) ?? [];
    const timelineFiles    = (filesystemSnap?.files as TimelineFile[] | undefined) ?? [];

    // ── Loading ───────────────────────────────────────────────────────────────

    if (isLoading) {
        return (
            <div className="flex items-center justify-center min-h-[20vh]">
                <Loader2 className="w-8 h-8 animate-spin text-cyan-500" />
                <span className="ml-3 text-slate-500 dark:text-slate-400">Loading incident data…</span>
            </div>
        );
    }

    // ── Playbook status badge ─────────────────────────────────────────────────

    const playbookStatus = () => {
        const st = incident?.run?.status;
        if (!st) return <span className="badge badge-info">Waiting…</span>;
        if (st === 'running')        return <span className="badge badge-warning animate-pulse">Running {successSteps}/{totalSteps}</span>;
        if (st === 'completed')      return <span className="badge badge-success">Completed {successSteps}/{totalSteps}</span>;
        if (st === 'false_positive') return <span className="badge" style={{background:'rgb(148 163 184/0.15)',color:'#64748b',borderColor:'rgb(148 163 184/0.3)'}}>False Positive</span>;
        if (st === 'failed')         return <span className="badge badge-critical">Failed</span>;
        return <span className="badge badge-info capitalize">{st}</span>;
    };

    // ── Render ────────────────────────────────────────────────────────────────

    return (
        <div className="space-y-4 sm:space-y-6">

            {/* ── Header Banner ──────────────────────────────────────────────── */}
            <div className="bg-rose-50 dark:bg-rose-900/20 border border-rose-200 dark:border-rose-800/40 rounded-xl px-5 py-4 flex flex-wrap items-center gap-4">
                <div className="p-2 rounded-lg bg-rose-100 dark:bg-rose-900/40">
                    <AlertTriangle className="w-5 h-5 text-rose-600 dark:text-rose-400" />
                </div>
                <div className="flex-1 min-w-0">
                    <p className="text-sm font-bold text-rose-800 dark:text-rose-300">
                        Endpoint Isolated — {agent.hostname}
                    </p>
                    <div className="flex items-center gap-2 mt-1">
                        <span className="text-xs text-rose-600 dark:text-rose-400">Post-isolation playbook:</span>
                        {playbookStatus()}
                        {incident?.run?.started_at && (
                            <span className="text-xs text-slate-400">· {relTime(incident.run.started_at)}</span>
                        )}
                        {isEscalated && (
                            <span className="badge badge-critical flex items-center gap-1">
                                <TrendingUp className="w-2.5 h-2.5" /> Escalated
                            </span>
                        )}
                    </div>
                </div>
                <div className="flex items-center gap-2 flex-wrap">
                    {/* Refresh */}
                    <button
                        onClick={() => refetch()}
                        className="btn btn-secondary p-2"
                        title="Refresh"
                    >
                        <RefreshCw className="w-4 h-4" />
                    </button>

                    {/* Memory Dump */}
                    {!memoryDumpConfirm ? (
                        <button
                            onClick={() => setMemoryDumpConfirm(true)}
                            disabled={memoryLoading}
                            className="btn btn-warning"
                        >
                            {memoryLoading ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Cpu className="w-3.5 h-3.5" />}
                            Memory Dump
                        </button>
                    ) : (
                        <div className="flex items-center gap-1.5 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800/40 rounded-lg px-3 py-1.5">
                            <span className="text-xs text-amber-700 dark:text-amber-400 font-medium">Confirm dump?</span>
                            <button onClick={() => collectMemory()} className="btn btn-warning py-0.5 px-2 text-[11px]">Yes</button>
                            <button onClick={() => setMemoryDumpConfirm(false)} className="btn btn-secondary py-0.5 px-2 text-[11px]">No</button>
                        </div>
                    )}

                    {/* Un-isolate — navigates to Response tab with restore_network pre-selected */}
                    <button
                        onClick={() => {
                            onUnIsolate();
                            showToast('Redirected to Response — select "Restore Network" to un-isolate', 'info');
                        }}
                        className="btn btn-success"
                    >
                        <Shield className="w-3.5 h-3.5" />
                        Un-isolate
                    </button>
                </div>
            </div>

            {/* ── KPI Cards ──────────────────────────────────────────────────── */}
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                <StatCard
                    title="Persistence Items"
                    value={persistenceItems.length}
                    icon={Database}
                    color={persistenceItems.length > 0 ? 'amber' : 'cyan'}
                    onClick={() => setActivePanel('persistence')}
                />
                <StatCard
                    title="LSASS Accesses"
                    value={lsassAccesses.length}
                    icon={Lock}
                    color={lsassAccesses.length > 0 ? 'red' : 'cyan'}
                    onClick={() => setActivePanel('lsass')}
                />
                <StatCard
                    title="Malicious IOCs"
                    value={maliciousIocs}
                    icon={Bug}
                    color={maliciousIocs > 0 ? 'red' : 'cyan'}
                    onClick={() => setActivePanel('iocs')}
                    subtext={suspiciousIocs > 0 ? `+${suspiciousIocs} suspicious` : undefined}
                />
                <StatCard
                    title="Files Modified"
                    value={timelineFiles.length}
                    icon={HardDrive}
                    color="cyan"
                    onClick={() => setActivePanel('filesystem')}
                />
            </div>

            {/* ── Main panel: Timeline + Detail ──────────────────────────────── */}
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">

                {/* Playbook Timeline sidebar */}
                <div className="bg-white/80 dark:bg-slate-900/50 border border-slate-200 dark:border-slate-700 rounded-xl p-4 shadow-sm">
                    <div className="flex items-center justify-between mb-3">
                        <h3 className="text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider flex items-center gap-2">
                            <Activity className="w-3.5 h-3.5 text-cyan-500" />
                            Playbook Steps
                        </h3>
                        <span className="text-[10px] text-slate-400 font-mono">{successSteps}/{totalSteps}</span>
                    </div>
                    <PlaybookTimeline steps={incident?.steps ?? []} />
                </div>

                {/* Detail panel */}
                <div className="lg:col-span-2 bg-white/80 dark:bg-slate-900/50 border border-slate-200 dark:border-slate-700 rounded-xl p-4 shadow-sm">
                    {/* Panel tabs */}
                    <div className="flex flex-wrap gap-1 mb-4 border-b border-slate-200 dark:border-slate-700 pb-3">
                        {DETAIL_PANELS.map(panel => {
                            const Icon = panel.icon;
                            const active = activePanel === panel.id;
                            return (
                                <button
                                    key={panel.id}
                                    onClick={() => setActivePanel(panel.id)}
                                    className={`flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs font-medium transition-colors ${
                                        active
                                            ? 'bg-cyan-500/10 dark:bg-cyan-500/20 text-cyan-700 dark:text-cyan-300 border border-cyan-500/20'
                                            : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800/60'
                                    }`}
                                >
                                    <Icon className="w-3.5 h-3.5" />
                                    {panel.label}
                                </button>
                            );
                        })}
                    </div>

                    {/* Active panel */}
                    {activePanel === 'processes'   && <ProcessTreePanel  snapshot={processSnap}    />}
                    {activePanel === 'persistence' && <PersistencePanel  snapshot={persistenceSnap} />}
                    {activePanel === 'lsass'       && <LsassPanel        snapshot={lsassSnap}      />}
                    {activePanel === 'network'     && <NetworkPanel      snapshot={networkSnap}    />}
                    {activePanel === 'filesystem'  && <FilesystemPanel   snapshot={filesystemSnap} />}
                    {activePanel === 'iocs'        && <IocPanel          iocs={incident?.iocs ?? []} />}
                    {activePanel === 'integrity'   && <IntegrityPanel    snapshot={integritySnap}  />}
                    {activePanel === 'alerts'      && (
                        <AlertsPanel agentId={agent.id} since={incident?.run?.started_at} />
                    )}
                </div>
            </div>

            {/* ── Footer metadata ────────────────────────────────────────────── */}
            {incident?.run && (
                <div className="flex flex-wrap items-center gap-3 px-1 text-xs text-slate-400">
                    <span className="flex items-center gap-1">
                        <Clock className="w-3 h-3" />
                        Run started {relTime(incident.run.started_at)}
                        {incident.run.finished_at && ` · finished ${relTime(incident.run.finished_at)}`}
                    </span>
                    {suspiciousIocs > 0 && (
                        <span className="badge badge-warning">
                            {suspiciousIocs} suspicious IOC{suspiciousIocs > 1 ? 's' : ''}
                        </span>
                    )}
                </div>
            )}

            {/* ── Action Bar ─────────────────────────────────────────────────── */}
            <div className="bg-white/80 dark:bg-slate-900/50 border border-slate-200 dark:border-slate-700 rounded-xl px-5 py-3 flex flex-wrap items-center gap-3 print:hidden shadow-sm">
                <span className="text-[11px] font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider mr-auto">
                    Incident Actions
                </span>

                {/* Export / Print */}
                <button
                    onClick={() => {
                        window.print();
                        showToast('Print dialog opened — save as PDF to export', 'info');
                    }}
                    className="btn btn-secondary"
                >
                    <Printer className="w-3.5 h-3.5" />
                    Export Report
                </button>

                {/* Escalate */}
                {!escalateConfirm ? (
                    <button
                        onClick={() => setEscalateConfirm(true)}
                        disabled={escalateLoading || isEscalated}
                        className="btn btn-warning"
                    >
                        <TrendingUp className="w-3.5 h-3.5" />
                        {isEscalated ? 'Escalated' : 'Escalate'}
                    </button>
                ) : (
                    <div className="flex items-center gap-1.5 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800/40 rounded-lg px-3 py-1.5">
                        <span className="text-xs text-amber-700 dark:text-amber-400 font-medium">Escalate incident?</span>
                        <button onClick={() => doEscalate()} disabled={escalateLoading} className="btn btn-warning py-0.5 px-2 text-[11px]">
                            {escalateLoading ? <Loader2 className="w-3 h-3 animate-spin" /> : 'Yes'}
                        </button>
                        <button onClick={() => setEscalateConfirm(false)} className="btn btn-secondary py-0.5 px-2 text-[11px]">No</button>
                    </div>
                )}

                {/* Mark as False Positive */}
                {!fpConfirm ? (
                    <button
                        onClick={() => setFpConfirm(true)}
                        disabled={fpLoading || isFalsePos}
                        className="btn btn-ghost"
                    >
                        <Flag className="w-3.5 h-3.5" />
                        {isFalsePos ? 'Marked False Positive' : 'Mark as False Positive'}
                    </button>
                ) : (
                    <div className="flex items-center gap-1.5 bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 rounded-lg px-3 py-1.5">
                        <span className="text-xs text-slate-700 dark:text-slate-300 font-medium">Mark as false positive?</span>
                        <button onClick={() => doMarkFp()} disabled={fpLoading} className="btn btn-primary py-0.5 px-2 text-[11px]">
                            {fpLoading ? <Loader2 className="w-3 h-3 animate-spin" /> : 'Yes'}
                        </button>
                        <button onClick={() => setFpConfirm(false)} className="btn btn-secondary py-0.5 px-2 text-[11px]">No</button>
                    </div>
                )}
            </div>
        </div>
    );
}

export default IncidentTab;
