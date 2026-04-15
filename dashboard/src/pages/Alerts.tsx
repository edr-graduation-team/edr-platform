import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import React, { useEffect, useRef, useState } from 'react';
import {
    Search, Check, Eye, X, ChevronLeft, ChevronRight,
    AlertTriangle, Clock, CheckCircle, XCircle, Shield, ArrowUpDown,
    GitBranch, Activity, TrendingUp, Cpu, Zap, Info, ChevronDown, ChevronUp
} from 'lucide-react';
import { alertsApi, authApi, createAlertStream, type Alert, type ContextSnapshot, type ScoreBreakdown, type AncestorEntry } from '../api/client';
import {
    Modal, MultiSelect, DateRangePicker, type DateRange, type MultiSelectOption,
    useToast, SkeletonTable
} from '../components';
import { useDebounce } from '../hooks/useDebounce';

// Severity options with counts
const SEVERITY_OPTIONS: MultiSelectOption[] = [
    { value: 'critical', label: 'Critical', color: '#ef4444' },
    { value: 'high', label: 'High', color: '#f97316' },
    { value: 'medium', label: 'Medium', color: '#eab308' },
    { value: 'low', label: 'Low', color: '#6366f1' },
    { value: 'informational', label: 'Info', color: '#3b82f6' },
];

// Status options
const STATUS_OPTIONS: MultiSelectOption[] = [
    { value: 'open', label: 'Open' },
    { value: 'in_progress', label: 'In Progress' },
    { value: 'acknowledged', label: 'Acknowledged' },
    { value: 'resolved', label: 'Resolved' },
    { value: 'false_positive', label: 'False Positive' },
];

// Severity badge colors
const severityColors: Record<string, string> = {
    critical: 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/20',
    high: 'bg-orange-500/10 text-orange-600 dark:text-orange-400 border border-orange-500/20',
    medium: 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20',
    low: 'bg-indigo-500/10 text-indigo-600 dark:text-indigo-400 border border-indigo-500/20',
    informational: 'bg-cyan-500/10 text-cyan-600 dark:text-cyan-400 border border-cyan-500/20',
};

// Status badge colors
const statusColors: Record<string, string> = {
    open: 'bg-rose-600/10 text-rose-700 dark:text-rose-400 border border-rose-600/20',
    in_progress: 'bg-amber-500/10 text-amber-700 dark:text-amber-400 border border-amber-500/20',
    acknowledged: 'bg-cyan-500/10 text-cyan-700 dark:text-cyan-400 border border-cyan-500/20',
    resolved: 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border border-emerald-500/20',
    false_positive: 'bg-slate-500/10 text-slate-700 dark:text-slate-400 border border-slate-500/20',
    closed: 'bg-slate-500/10 text-slate-700 dark:text-slate-400 border border-slate-500/20',
};

// Status icons
const statusIcons: Record<string, typeof AlertTriangle> = {
    open: AlertTriangle,
    in_progress: Clock,
    acknowledged: Eye,
    resolved: CheckCircle,
    false_positive: XCircle,
    closed: XCircle,
};

// =============================================================================
// Risk Score Badge
// =============================================================================

function getRiskScoreStyle(score: number): { bg: string; text: string; label: string; ring: string, shadow: string } {
    if (score >= 90) return { bg: 'bg-rose-500/10 dark:bg-rose-500/20', text: 'text-rose-600 dark:text-rose-400', label: 'CRITICAL', ring: 'border border-rose-500/30', shadow: 'shadow-[0_0_10px_rgba(244,63,94,0.2)]' };
    if (score >= 70) return { bg: 'bg-orange-500/10 dark:bg-orange-500/20', text: 'text-orange-600 dark:text-orange-400', label: 'HIGH', ring: 'border border-orange-500/30', shadow: 'shadow-[0_0_10px_rgba(249,115,22,0.2)]' };
    if (score >= 40) return { bg: 'bg-amber-500/10 dark:bg-amber-500/20', text: 'text-amber-600 dark:text-amber-400', label: 'MEDIUM', ring: 'border border-amber-500/30', shadow: 'shadow-[0_0_10px_rgba(245,158,11,0.2)]' };
    return { bg: 'bg-emerald-500/10 dark:bg-emerald-500/20', text: 'text-emerald-600 dark:text-emerald-400', label: 'LOW', ring: 'border border-emerald-500/30', shadow: 'shadow-[0_0_10px_rgba(16,185,129,0.2)]' };
}

const RiskScoreBadge = React.memo(function RiskScoreBadge({ score }: { score?: number }) {
    if (score === undefined || score === null) {
        return <span className="text-xs text-gray-400 font-mono">—</span>;
    }
    const style = getRiskScoreStyle(score);
    return (
        <div className="flex items-center gap-1.5">
            <span
                className={`inline-flex items-center justify-center w-9 h-9 rounded-full text-sm font-bold ${style.bg} ${style.text} ${style.ring} ${style.shadow}`}
                title={`Risk Score: ${score}/100 (${style.label})`}
            >
                {score}
            </span>
        </div>
    );
});


// =============================================================================
// Process Lineage Tree Visualiser
// =============================================================================

function ProcessNode({ name, path, integrity, isElevated, sigStatus, isTarget = false, isSuspicious = false }: {
    name: string;
    path?: string;
    integrity?: string;
    isElevated?: boolean;
    sigStatus?: string;
    isTarget?: boolean;
    isSuspicious?: boolean;
}) {
    const [expanded, setExpanded] = useState(false);
    const hasDetails = !!(path || integrity || isElevated || sigStatus);

    return (
        <div className={`rounded-lg border px-3 py-2 text-sm transition-all ${isTarget
                ? 'border-red-400 bg-red-50 dark:bg-red-950/40 dark:border-red-700'
                : isSuspicious
                    ? 'border-orange-400 bg-orange-50 dark:bg-orange-950/40 dark:border-orange-700'
                    : 'border-gray-200 bg-gray-50 dark:bg-gray-700/40 dark:border-gray-600'
            }`}>
            <div className="flex items-center justify-between gap-2">
                <div className="flex items-center gap-2 min-w-0">
                    <Cpu className={`w-3.5 h-3.5 shrink-0 ${isTarget ? 'text-red-500' : isSuspicious ? 'text-orange-500' : 'text-gray-400'}`} />
                    <span className={`font-mono font-semibold truncate ${isTarget ? 'text-red-700 dark:text-red-300' : 'text-gray-800 dark:text-gray-200'}`}>
                        {name}
                    </span>
                    {isElevated && (
                        <span className="badge bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-200 text-xs shrink-0">
                            ELEVATED
                        </span>
                    )}
                    {sigStatus === 'microsoft' && (
                        <span className="badge bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-200 text-xs shrink-0">
                            MS-SIGNED
                        </span>
                    )}
                </div>
                {hasDetails && (
                    <button
                        onClick={() => setExpanded(!expanded)}
                        className="text-gray-400 hover:text-gray-600 shrink-0"
                    >
                        {expanded ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
                    </button>
                )}
            </div>
            {expanded && hasDetails && (
                <div className="mt-2 space-y-1 text-xs text-gray-500 dark:text-gray-400">
                    {path && <p className="font-mono truncate" title={path}>{path}</p>}
                    {integrity && <p>Integrity: <span className="font-medium">{integrity}</span></p>}
                    {sigStatus && <p>Signature: <span className="font-medium">{sigStatus}</span></p>}
                </div>
            )}
        </div>
    );
}

function LineageTree({ snapshot }: { snapshot: ContextSnapshot }) {
    const suspicionLevel = snapshot.lineage_suspicion;
    const isSuspicious = suspicionLevel === 'critical' || suspicionLevel === 'high';

    const suspicionBadge: Record<string, string> = {
        critical: 'badge bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-200',
        high: 'badge bg-orange-100 text-orange-700 dark:bg-orange-900 dark:text-orange-200',
        medium: 'badge bg-yellow-100 text-yellow-700 dark:bg-yellow-900 dark:text-yellow-200',
        low: 'badge badge-info',
        none: 'badge badge-success',
    };

    // Build the chain from ancestor_chain if available, else fallback to flat fields
    const chain: AncestorEntry[] = snapshot.ancestor_chain || [];

    return (
        <div className="space-y-3">
            {/* Suspicion level header */}
            <div className="flex items-center justify-between">
                <span className="text-xs text-gray-500 uppercase tracking-wider">Process Lineage</span>
                <span className={suspicionBadge[suspicionLevel] || 'badge badge-info'}>
                    {suspicionLevel.toUpperCase()} SUSPICION
                </span>
            </div>

            {/* Ancestor chain as visual tree */}
            {chain.length > 0 ? (
                <div className="space-y-1">
                    {chain.map((node, idx) => (
                        <div key={idx} className="flex items-start gap-2">
                            {idx > 0 && (
                                <div className="flex flex-col items-center ml-3 mr-1">
                                    <div className="w-px h-3 bg-gray-300 dark:bg-gray-600" />
                                    <div className="w-3 h-px bg-gray-300 dark:bg-gray-600" />
                                </div>
                            )}
                            <div className={`flex-1 ${idx > 0 ? '' : ''}`}>
                                <ProcessNode
                                    name={node.name}
                                    path={node.path}
                                    integrity={node.integrity}
                                    isElevated={node.is_elevated}
                                    sigStatus={node.sig_status}
                                    isTarget={idx === 0}
                                    isSuspicious={isSuspicious && idx > 0}
                                />
                            </div>
                        </div>
                    ))}
                </div>
            ) : (
                /* Fallback: use flat fields from ContextSnapshot */
                <div className="space-y-1">
                    {snapshot.grandparent_name && (
                        <div>
                            <ProcessNode
                                name={snapshot.grandparent_name}
                                path={snapshot.grandparent_path}
                                isSuspicious={isSuspicious}
                            />
                            <div className="ml-6 my-1 flex items-center gap-1 text-gray-400">
                                <div className="w-px h-4 bg-gray-300 dark:bg-gray-600 ml-1" />
                                <span className="text-xs">spawned</span>
                            </div>
                        </div>
                    )}
                    {snapshot.parent_name && (
                        <div>
                            <ProcessNode
                                name={snapshot.parent_name}
                                path={snapshot.parent_path}
                                isSuspicious={isSuspicious}
                            />
                            <div className="ml-6 my-1 flex items-center gap-1 text-gray-400">
                                <div className="w-px h-4 bg-gray-300 dark:bg-gray-600 ml-1" />
                                <span className="text-xs">spawned</span>
                            </div>
                        </div>
                    )}
                    {snapshot.process_name && (
                        <ProcessNode
                            name={snapshot.process_name}
                            path={snapshot.process_path}
                            integrity={snapshot.integrity_level}
                            isElevated={snapshot.is_elevated}
                            sigStatus={snapshot.signature_status}
                            isTarget={true}
                        />
                    )}
                    {!snapshot.grandparent_name && !snapshot.parent_name && !snapshot.process_name && (
                        <p className="text-sm text-gray-400 italic">No lineage data captured for this alert.</p>
                    )}
                </div>
            )}
        </div>
    );
}

// =============================================================================
// UEBA Signals Panel
// =============================================================================

function UEBASignalBadge({ signal }: { signal: string }) {
    if (signal === 'anomaly') {
        return (
            <span className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs font-semibold bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-300 ring-1 ring-red-300 dark:ring-red-700">
                <Zap className="w-3 h-3" />
                Baseline Anomaly
            </span>
        );
    }
    if (signal === 'normal') {
        return (
            <span className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs font-semibold bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300 ring-1 ring-green-300 dark:ring-green-700">
                <CheckCircle className="w-3 h-3" />
                Normalcy Discount
            </span>
        );
    }
    return (
        <span className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs font-semibold bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400">
            <Info className="w-3 h-3" />
            No UEBA Signal
        </span>
    );
}

function UEBAPanel({ snapshot }: { snapshot: ContextSnapshot }) {
    const bd = snapshot.score_breakdown;
    return (
        <div className="space-y-4">
            {/* UEBA Signal */}
            <div>
                <span className="text-xs text-gray-500 uppercase tracking-wider block mb-2">Behavioral Signal</span>
                <div className="flex flex-wrap gap-2">
                    <UEBASignalBadge signal={bd.ueba_signal} />
                    {bd.ueba_signal === 'anomaly' && (
                        <span className="inline-flex items-center gap-1 text-xs text-red-600 dark:text-red-400 font-medium">
                            +{bd.ueba_bonus} pts added to risk score
                        </span>
                    )}
                    {bd.ueba_signal === 'normal' && (
                        <span className="inline-flex items-center gap-1 text-xs text-green-600 dark:text-green-400 font-medium">
                            −{bd.ueba_discount} pts subtracted (FP discount)
                        </span>
                    )}
                </div>
            </div>

            {/* Temporal Burst */}
            <div>
                <span className="text-xs text-gray-500 uppercase tracking-wider block mb-2">Temporal Burst</span>
                <div className="flex items-center gap-3">
                    <div className="flex items-center gap-1.5">
                        <Activity className={`w-4 h-4 ${snapshot.burst_count > 3 ? 'text-orange-500' : 'text-gray-400'}`} />
                        <span className={`font-semibold text-sm ${snapshot.burst_count > 3 ? 'text-orange-600 dark:text-orange-400' : 'text-gray-700 dark:text-gray-300'}`}>
                            {snapshot.burst_count} fire{snapshot.burst_count !== 1 ? 's' : ''}
                        </span>
                        <span className="text-xs text-gray-500">in {Math.round(snapshot.burst_window_sec / 60)} min window</span>
                    </div>
                    {bd.burst_bonus > 0 && (
                        <span className="badge bg-orange-100 text-orange-700 dark:bg-orange-900 dark:text-orange-200">
                            +{bd.burst_bonus} Burst Bonus
                        </span>
                    )}
                </div>
            </div>

            {/* Privilege Info */}
            <div>
                <span className="text-xs text-gray-500 uppercase tracking-wider block mb-2">Privilege Context</span>
                <div className="flex flex-wrap gap-2 text-sm">
                    {snapshot.integrity_level && (
                        <span className={`badge ${snapshot.integrity_level === 'System' || snapshot.integrity_level === 'High'
                                ? 'badge-danger'
                                : 'badge-info'
                            }`}>
                            {snapshot.integrity_level} Integrity
                        </span>
                    )}
                    {snapshot.is_elevated && (
                        <span className="badge badge-danger">Elevated Process</span>
                    )}
                    {snapshot.user_name && (
                        <span className="badge badge-info font-mono">{snapshot.user_name}</span>
                    )}
                    {snapshot.user_sid && !snapshot.user_name && (
                        <span className="badge badge-info font-mono" title="User SID is used for privilege scoring">{snapshot.user_sid}</span>
                    )}
                    {snapshot.signature_status && (
                        <span className={`badge ${snapshot.signature_status === 'microsoft' ? 'badge-success' : snapshot.signature_status === 'unsigned' ? 'badge-danger' : 'badge-warning'}`}>
                            {snapshot.signature_status === 'microsoft' ? '✓ Microsoft' : snapshot.signature_status}
                        </span>
                    )}
                    {!snapshot.integrity_level && !snapshot.is_elevated && !snapshot.user_name && !snapshot.user_sid && (
                        <span className="text-xs text-gray-400 italic">No privilege data captured.</span>
                    )}
                </div>
            </div>


            {/* Warnings */}
            {snapshot.warnings && snapshot.warnings.length > 0 && (
                <div className="rounded-md border border-yellow-200 dark:border-yellow-800 bg-yellow-50 dark:bg-yellow-900/20 p-3">
                    <span className="text-xs font-semibold text-yellow-700 dark:text-yellow-400 uppercase tracking-wider">
                        Partial Context (Degraded Signals)
                    </span>
                    <ul className="mt-1 space-y-1">
                        {snapshot.warnings.map((w, i) => (
                            <li key={i} className="text-xs text-yellow-600 dark:text-yellow-400 font-mono">{w}</li>
                        ))}
                    </ul>
                </div>
            )}
        </div>
    );
}

// =============================================================================
// Score Breakdown Panel
// =============================================================================

interface BreakdownRow {
    label: string;
    value: number;
    sign: '+' | '−' | '=';
    color: string;
    icon: React.ReactNode;
    description: string;
}

function ScoreBreakdownPanel({ breakdown, totalScore }: { breakdown: ScoreBreakdown; totalScore: number }) {
    const rows: BreakdownRow[] = [
        {
            label: 'Base Score',
            value: breakdown.base_score,
            sign: '+',
            color: 'text-blue-600 dark:text-blue-400',
            icon: <Shield className="w-3.5 h-3.5" />,
            description: 'Derived from Sigma rule severity',
        },
        {
            label: 'Lineage Bonus',
            value: breakdown.lineage_bonus,
            sign: '+',
            color: 'text-purple-600 dark:text-purple-400',
            icon: <GitBranch className="w-3.5 h-3.5" />,
            description: 'Suspicious parent→child process chain',
        },
        {
            label: 'Privilege Bonus',
            value: breakdown.privilege_bonus,
            sign: '+',
            color: 'text-orange-600 dark:text-orange-400',
            icon: <Cpu className="w-3.5 h-3.5" />,
            description: 'SYSTEM / elevated process context',
        },
        {
            label: 'Burst Bonus',
            value: breakdown.burst_bonus,
            sign: '+',
            color: 'text-yellow-600 dark:text-yellow-400',
            icon: <Activity className="w-3.5 h-3.5" />,
            description: 'Repeated rule firing in 5-min window',
        },
        {
            label: 'UEBA Bonus',
            value: breakdown.ueba_bonus,
            sign: '+',
            color: 'text-red-600 dark:text-red-400',
            icon: <Zap className="w-3.5 h-3.5" />,
            description: 'First-seen hour or statistical spike (Z>3σ)',
        },
        {
            label: 'Interaction',
            value: breakdown.interaction_bonus || 0,
            sign: '+',
            color: 'text-pink-600 dark:text-pink-400',
            icon: <ArrowUpDown className="w-3.5 h-3.5" />,
            description: 'Cross-dimensional signal convergence (≥2 high signals)',
        },
        {
            label: 'FP Discount',
            value: breakdown.fp_discount,
            sign: '−',
            color: 'text-green-600 dark:text-green-400',
            icon: <TrendingUp className="w-3.5 h-3.5" />,
            description: 'Trusted / Microsoft-signed binary',
        },
        {
            label: 'UEBA Discount',
            value: breakdown.ueba_discount,
            sign: '−',
            color: 'text-teal-600 dark:text-teal-400',
            icon: <CheckCircle className="w-3.5 h-3.5" />,
            description: 'Process within its normal baseline (within 1σ)',
        },
    ];

    const maxBar = 100;
    const { bg: scoreBg, text: scoreText } = getRiskScoreStyle(totalScore);
    const interactionVal = breakdown.interaction_bonus || 0;

    return (
        <div className="space-y-4">
            <span className="text-xs text-gray-500 uppercase tracking-wider block">Score Breakdown</span>

            {/* Visual bar chart */}
            <div className="space-y-2">
                {rows.map((row) => {
                    if (row.value === 0) return null;
                    const width = Math.min((Math.abs(row.value) / maxBar) * 100, 100);
                    const isDiscount = row.sign === '−';
                    return (
                        <div key={row.label} className="flex items-center gap-3">
                            <div className="flex items-center gap-1.5 w-32 shrink-0">
                                <span className={row.color}>{row.icon}</span>
                                <span className="text-xs text-gray-600 dark:text-gray-400">{row.label}</span>
                            </div>
                            <div className="flex-1 h-5 bg-gray-100 dark:bg-gray-700 rounded overflow-hidden">
                                <div
                                    className={`h-full rounded transition-all duration-500 ${isDiscount ? 'bg-green-400 dark:bg-green-600' : 'bg-blue-400 dark:bg-blue-500'}`}
                                    style={{ width: `${width}%` }}
                                />
                            </div>
                            <span className={`text-xs font-mono font-semibold w-10 text-right ${row.color}`}>
                                {row.sign}{row.value}
                            </span>
                        </div>
                    );
                })}
            </div>

            {/* Formula summary */}
            <div className="border-t border-gray-200 dark:border-gray-700 pt-3">
                <div className="flex items-center justify-between">
                    <div className="text-xs text-gray-500 font-mono">
                        {breakdown.base_score} + {breakdown.lineage_bonus} + {breakdown.privilege_bonus} + {breakdown.burst_bonus}
                        {breakdown.ueba_bonus > 0 && ` + ${breakdown.ueba_bonus}`}
                        {interactionVal > 0 && ` + ${interactionVal}`}
                        {breakdown.fp_discount > 0 && ` − ${breakdown.fp_discount}`}
                        {breakdown.ueba_discount > 0 && ` − ${breakdown.ueba_discount}`}
                        {' '}= {breakdown.raw_score} → clamped to {breakdown.final_score}
                    </div>
                    <span className={`inline-flex items-center justify-center w-10 h-10 rounded-full text-sm font-bold ring-2 ${scoreBg} ${scoreText} ring-offset-1`}>
                        {totalScore}
                    </span>
                </div>
            </div>
        </div>
    );
}

// =============================================================================
// Alert Detail Modal with enhanced Context tab
// =============================================================================

function AlertDetailModal({
    alert,
    isOpen,
    onClose,
    onStatusChange
}: {
    alert: Alert | null;
    isOpen: boolean;
    onClose: () => void;
    onStatusChange: (id: string, status: string) => void;
}) {
    const [activeTab, setActiveTab] = useState<'summary' | 'context' | 'event' | 'mitre' | 'actions'>('summary');

    if (!alert) return null;

    const hasContext = !!(alert.context_snapshot);
    const snapshot = alert.context_snapshot;
    const breakdown = alert.score_breakdown || snapshot?.score_breakdown;

    const tabs = [
        { id: 'summary', label: 'Summary' },
        { id: 'context', label: '⚡ Context', highlight: hasContext },
        { id: 'event', label: 'Event Details' },
        { id: 'mitre', label: 'MITRE ATT&CK' },
        ...(authApi.canWriteAlerts() ? [{ id: 'actions', label: 'Actions' }] : []),
    ];

    return (
        <Modal isOpen={isOpen} onClose={onClose} title="Alert Details" size="lg">
            {/* Tabs */}
            <div className="flex border-b border-gray-200 dark:border-gray-700 -mx-6 px-6 mb-4">
                {tabs.map((tab) => (
                    <button
                        key={tab.id}
                        onClick={() => setActiveTab(tab.id as typeof activeTab)}
                        className={`tab ${activeTab === tab.id ? 'tab-active' : ''} ${tab.highlight ? 'relative' : ''}`}
                    >
                        {tab.label}
                        {tab.highlight && activeTab !== tab.id && (
                            <span className="absolute top-1 right-0 w-2 h-2 rounded-full bg-red-500" />
                        )}
                    </button>
                ))}
            </div>

            {/* Summary Tab */}
            {activeTab === 'summary' && (
                <div className="space-y-4">
                    {/* Risk Score hero */}
                    {alert.risk_score !== undefined && (
                        <div className={`rounded-xl p-4 flex items-center gap-4 ${alert.risk_score >= 90
                                ? 'bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-800'
                                : alert.risk_score >= 70
                                    ? 'bg-orange-50 dark:bg-orange-950/30 border border-orange-200 dark:border-orange-800'
                                    : alert.risk_score >= 40
                                        ? 'bg-yellow-50 dark:bg-yellow-950/30 border border-yellow-200 dark:border-yellow-800'
                                        : 'bg-green-50 dark:bg-green-950/30 border border-green-200 dark:border-green-800'
                            }`}>
                            <div className="shrink-0">
                                <RiskScoreBadge score={alert.risk_score} />
                            </div>
                            <div>
                                <p className="text-sm font-semibold text-gray-800 dark:text-gray-100">
                                    Risk Score: {alert.risk_score}/100 — {getRiskScoreStyle(alert.risk_score).label}
                                </p>
                                {alert.false_positive_risk !== undefined && (
                                    <p className="text-xs text-gray-500">
                                        FP Risk: {(alert.false_positive_risk * 100).toFixed(0)}% probability this is a false positive
                                    </p>
                                )}
                                {typeof breakdown?.context_multiplier === 'number' && (
                                    <p className="text-xs text-gray-500">
                                        Context multiplier: {breakdown.context_multiplier.toFixed(2)}
                                        {typeof breakdown.user_role_weight === 'number' &&
                                            typeof breakdown.device_criticality_weight === 'number' &&
                                            typeof breakdown.network_anomaly_factor === 'number'
                                            ? ` (user ${breakdown.user_role_weight.toFixed(2)} × device ${breakdown.device_criticality_weight.toFixed(2)} × network ${breakdown.network_anomaly_factor.toFixed(2)})`
                                            : ''}
                                    </p>
                                )}
                                {breakdown?.ueba_signal && breakdown.ueba_signal !== 'none' && (
                                    <div className="mt-1">
                                        <UEBASignalBadge signal={breakdown.ueba_signal} />
                                    </div>
                                )}
                            </div>
                        </div>
                    )}

                    <div className="grid grid-cols-2 gap-4">
                        <div>
                            <label className="text-xs text-gray-500 uppercase tracking-wider">Rule</label>
                            <p className="font-medium text-gray-900 dark:text-white">{alert.rule_title}</p>
                        </div>
                        <div>
                            <label className="text-xs text-gray-500 uppercase tracking-wider">Category</label>
                            <p className="text-gray-700 dark:text-gray-300">{alert.category || 'N/A'}</p>
                        </div>
                        <div>
                            <label className="text-xs text-gray-500 uppercase tracking-wider">Agent ID</label>
                            <p className="font-mono text-sm text-gray-700 dark:text-gray-300">{alert.agent_id}</p>
                        </div>
                        <div>
                            <label className="text-xs text-gray-500 uppercase tracking-wider">Severity</label>
                            <p><span className={`badge ${severityColors[alert.severity]}`}>{alert.severity.toUpperCase()}</span></p>
                        </div>
                        <div>
                            <label className="text-xs text-gray-500 uppercase tracking-wider">Status</label>
                            <p><span className={`badge ${statusColors[alert.status]}`}>{alert.status.replace('_', ' ')}</span></p>
                        </div>
                        <div>
                            <label className="text-xs text-gray-500 uppercase tracking-wider">Confidence</label>
                            <p className="text-gray-700 dark:text-gray-300">{(alert.confidence * 100).toFixed(1)}%</p>
                        </div>
                        <div>
                            <label className="text-xs text-gray-500 uppercase tracking-wider">Event Count</label>
                            <p className="text-gray-700 dark:text-gray-300">{alert.event_count}</p>
                        </div>
                        <div>
                            <label className="text-xs text-gray-500 uppercase tracking-wider">Detected At</label>
                            <p className="text-gray-700 dark:text-gray-300">{new Date(alert.timestamp).toLocaleString()}</p>
                        </div>
                    </div>

                    {alert.notes && (
                        <div>
                            <label className="text-xs text-gray-500 uppercase tracking-wider">Notes</label>
                            <p className="text-gray-700 dark:text-gray-300 mt-1">{alert.notes}</p>
                        </div>
                    )}
                </div>
            )}

            {/* Context Tab — Process Lineage + UEBA + Score Breakdown */}
            {activeTab === 'context' && (
                <div className="space-y-6">
                    {!hasContext ? (
                        <div className="text-center py-8">
                            <Info className="w-10 h-10 text-gray-300 mx-auto mb-3" />
                            <p className="text-sm text-gray-500">
                                No context snapshot available for this alert.<br />
                                <span className="text-xs">Context scoring requires Sprint 3+ backend deployment.</span>
                            </p>
                        </div>
                    ) : (
                        <>
                            {/* Process Lineage */}
                            <div className="rounded-xl border border-gray-200 dark:border-gray-700 p-4">
                                <LineageTree snapshot={snapshot!} />
                            </div>

                            {/* UEBA & Burst Signals */}
                            <div className="rounded-xl border border-gray-200 dark:border-gray-700 p-4">
                                <UEBAPanel snapshot={snapshot!} />
                            </div>

                            {/* Score Breakdown */}
                            {breakdown && (
                                <div className="rounded-xl border border-gray-200 dark:border-gray-700 p-4">
                                    <ScoreBreakdownPanel
                                        breakdown={breakdown}
                                        totalScore={alert.risk_score ?? breakdown.final_score}
                                    />
                                </div>
                            )}

                            {/* Scored At */}
                            {snapshot?.scored_at && (
                                <p className="text-xs text-gray-400 text-right">
                                    Context scored at {new Date(snapshot.scored_at).toLocaleString()}
                                </p>
                            )}
                        </>
                    )}
                </div>
            )}

            {/* Event Details Tab */}
            {activeTab === 'event' && (
                <div>
                    <label className="text-xs text-gray-500 uppercase tracking-wider">Raw Event Data</label>
                    <pre className="mt-2 p-4 bg-gray-100 dark:bg-gray-900 rounded-lg overflow-auto max-h-96 text-xs font-mono text-gray-800 dark:text-gray-200">
                        {JSON.stringify(alert.event_data || alert.matched_fields || {}, null, 2)}
                    </pre>
                </div>
            )}

            {/* MITRE ATT&CK Tab */}
            {activeTab === 'mitre' && (
                <div className="space-y-4">
                    <div>
                        <label className="text-xs text-gray-500 uppercase tracking-wider">Tactics</label>
                        <div className="flex flex-wrap gap-2 mt-2">
                            {(alert.mitre_tactics || []).length > 0 ? (
                                alert.mitre_tactics?.map((tactic) => (
                                    <span key={tactic} className="badge badge-warning">{tactic}</span>
                                ))
                            ) : (
                                <span className="text-sm text-gray-400">No tactics identified</span>
                            )}
                        </div>
                    </div>
                    <div>
                        <label className="text-xs text-gray-500 uppercase tracking-wider">Techniques</label>
                        <div className="flex flex-wrap gap-2 mt-2">
                            {(alert.mitre_techniques || []).length > 0 ? (
                                alert.mitre_techniques?.map((technique) => (
                                    <span key={technique} className="badge badge-info">{technique}</span>
                                ))
                            ) : (
                                <span className="text-sm text-gray-400">No techniques identified</span>
                            )}
                        </div>
                    </div>
                </div>
            )}

            {/* Actions Tab */}
            {activeTab === 'actions' && (
                <div className="space-y-4">
                    <p className="text-sm text-gray-600 dark:text-gray-400">
                        Update the alert status to track investigation progress.
                    </p>
                    <div className="grid grid-cols-2 gap-3">
                        {alert.status === 'open' && (
                            <>
                                <button
                                    onClick={() => onStatusChange(alert.id, 'acknowledged')}
                                    className="btn btn-primary flex items-center justify-center gap-2"
                                >
                                    <Check className="w-4 h-4" />
                                    Acknowledge
                                </button>
                                <button
                                    onClick={() => onStatusChange(alert.id, 'in_progress')}
                                    className="btn btn-warning flex items-center justify-center gap-2"
                                >
                                    <Clock className="w-4 h-4" />
                                    Start Investigation
                                </button>
                            </>
                        )}
                        {(alert.status === 'acknowledged' || alert.status === 'in_progress') && (
                            <>
                                <button
                                    onClick={() => onStatusChange(alert.id, 'resolved')}
                                    className="btn btn-success flex items-center justify-center gap-2"
                                >
                                    <CheckCircle className="w-4 h-4" />
                                    Resolve
                                </button>
                                <button
                                    onClick={() => onStatusChange(alert.id, 'false_positive')}
                                    className="btn btn-secondary flex items-center justify-center gap-2"
                                >
                                    <XCircle className="w-4 h-4" />
                                    False Positive
                                </button>
                            </>
                        )}
                        {(alert.status === 'resolved' || alert.status === 'false_positive') && (
                            <button
                                onClick={() => onStatusChange(alert.id, 'open')}
                                className="btn btn-secondary flex items-center justify-center gap-2"
                            >
                                <AlertTriangle className="w-4 h-4" />
                                Reopen
                            </button>
                        )}
                    </div>
                </div>
            )}
        </Modal>
    );
}

// =============================================================================
// Bulk Actions Toolbar
// =============================================================================

const BulkActionsToolbar = React.memo(function BulkActionsToolbar({
    selectedCount,
    onAction,
    onClear
}: {
    selectedCount: number;
    onAction: (action: string) => void;
    onClear: () => void;
}) {
    if (selectedCount === 0) return null;
    if (!authApi.canWriteAlerts()) return null;

    return (
        <div className="flex items-center gap-4 p-3 bg-primary-50 dark:bg-primary-900/20 rounded-lg mb-4 animate-slide-up">
            <span className="text-sm font-medium text-primary-700 dark:text-primary-300">
                {selectedCount} alert(s) selected
            </span>
            <div className="flex-1" />
            <button onClick={() => onAction('acknowledged')} className="btn btn-sm btn-secondary">
                Acknowledge
            </button>
            <button onClick={() => onAction('resolved')} className="btn btn-sm btn-success">
                Resolve
            </button>
            <button onClick={() => onAction('false_positive')} className="btn btn-sm btn-secondary">
                False Positive
            </button>
            <button onClick={onClear} className="p-1 text-gray-500 hover:text-gray-700">
                <X className="w-4 h-4" />
            </button>
        </div>
    );
});


// =============================================================================
// Pagination
// =============================================================================

const Pagination = React.memo(function Pagination({
    page,
    totalPages,
    pageSize,
    total,
    onPageChange,
    onPageSizeChange
}: {
    page: number;
    totalPages: number;
    pageSize: number;
    total: number;
    onPageChange: (page: number) => void;
    onPageSizeChange: (size: number) => void;
}) {
    return (
        <div className="flex items-center justify-between px-6 py-4 bg-slate-50/50 dark:bg-slate-900/40 border-t border-slate-200 dark:border-slate-800/60 transition-colors">
            <div className="flex items-center gap-4 text-sm text-slate-500 dark:text-slate-400">
                <div className="flex items-center gap-2">
                    <span className="font-medium">Rows per page:</span>
                    <select
                        value={pageSize}
                        onChange={(e) => onPageSizeChange(Number(e.target.value))}
                        className="bg-transparent border border-slate-300 dark:border-slate-700 rounded-md py-1 pl-2 pr-6 text-slate-800 dark:text-slate-200 focus:outline-none focus:ring-1 focus:ring-primary-500 hover:border-slate-400 dark:hover:border-slate-500 transition-colors appearance-none cursor-pointer"
                        style={{ backgroundImage: 'url("data:image/svg+xml,%3csvg xmlns=\'http://www.w3.org/2000/svg\' fill=\'none\' viewBox=\'0 0 20 20\'%3e%3cpath stroke=\'%236b7280\' stroke-linecap=\'round\' stroke-linejoin=\'round\' stroke-width=\'1.5\' d=\'M6 8l4 4 4-4\'/%3e%3c/svg%3e")', backgroundPosition: 'right 0.2rem center', backgroundRepeat: 'no-repeat', backgroundSize: '1.5em 1.5em' }}
                    >
                        <option value={25}>25</option>
                        <option value={50}>50</option>
                        <option value={100}>100</option>
                    </select>
                </div>
                <div className="w-px h-4 bg-slate-300 dark:bg-slate-700 hidden sm:block" />
                <span className="font-medium hidden sm:block">
                    Showing {((page - 1) * pageSize) + 1}–{Math.min(page * pageSize, total)} of {total}
                </span>
            </div>
            <div className="flex items-center gap-3">
                <button
                    onClick={() => onPageChange(page - 1)}
                    disabled={page <= 1}
                    className="p-1.5 rounded-md text-slate-600 dark:text-slate-400 hover:bg-slate-200 dark:hover:bg-slate-800 hover:text-slate-900 dark:hover:text-slate-100 disabled:opacity-30 disabled:cursor-not-allowed transition-all"
                >
                    <ChevronLeft className="w-4 h-4" />
                </button>
                <span className="text-sm font-medium text-slate-700 dark:text-slate-300">
                    Page {page} of {totalPages || 1}
                </span>
                <button
                    onClick={() => onPageChange(page + 1)}
                    disabled={page >= totalPages}
                    className="p-1.5 rounded-md text-slate-600 dark:text-slate-400 hover:bg-slate-200 dark:hover:bg-slate-800 hover:text-slate-900 dark:hover:text-slate-100 disabled:opacity-30 disabled:cursor-not-allowed transition-all"
                >
                    <ChevronRight className="w-4 h-4" />
                </button>
            </div>
        </div>
    );
});


// =============================================================================
// Main Alerts Page
// =============================================================================

type SortField = 'timestamp' | 'severity' | 'risk_score';

export default function Alerts() {
    const queryClient = useQueryClient();
    const { showToast } = useToast();
    const seenAlertIdsRef = useRef<Set<string>>(new Set());
    const pendingStreamIdsRef = useRef<Set<string>>(new Set());
    const streamSyncTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const [selectedAlert, setSelectedAlert] = useState<Alert | null>(null);
    const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(50);
    // Default sort: risk_score descending (highest-risk first for SOC triage)
    const [sortBy, setSortBy] = useState<SortField>('risk_score');
    const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');

    const [filters, setFilters] = useState({
        severities: [] as string[],
        statuses: [] as string[],
        search: '',
    });
    const [dateRange, setDateRange] = useState<DateRange>({
        from: new Date(Date.now() - 24 * 60 * 60 * 1000),
        to: new Date(),
    });

    const debouncedSearch = useDebounce(filters.search, 300);

    // Fetch alerts with risk_score sort default
    const { data, isLoading, error } = useQuery({
        queryKey: ['alerts', filters.severities, filters.statuses, debouncedSearch, dateRange, page, pageSize, sortBy, sortOrder],
        queryFn: () => alertsApi.list({
            limit: pageSize,
            offset: (page - 1) * pageSize,
            severity: filters.severities.length > 0 ? filters.severities.join(',') : undefined,
            status: filters.statuses.length > 0 ? filters.statuses.join(',') : undefined,
            date_from: dateRange.from?.toISOString(),
            date_to: dateRange.to?.toISOString(),
            search: debouncedSearch || undefined,
            sort: sortBy,
            order: sortOrder,
        }),
        // DB is the source of truth. Keep lightweight fallback polling while
        // WebSocket stream provides low-latency invalidation triggers.
        refetchInterval: 30000,
    });

    const alerts = data?.alerts || [];
    const total = data?.total || 0;
    const totalPages = Math.ceil(total / pageSize);

    // Track IDs already rendered from DB so stream-triggered refreshes never
    // cause duplicate rendering semantics on the client.
    useEffect(() => {
        for (const alert of alerts) {
            seenAlertIdsRef.current.add(alert.id);
        }
    }, [alerts]);

    // Realtime path: stream is only a signal that new DB-persisted alerts exist.
    // We never use stream payload as source-of-truth rows in the table.
    useEffect(() => {
        const triggerDebouncedSync = () => {
            if (streamSyncTimerRef.current) {
                clearTimeout(streamSyncTimerRef.current);
            }
            streamSyncTimerRef.current = setTimeout(() => {
                const newCount = pendingStreamIdsRef.current.size;
                pendingStreamIdsRef.current.clear();

                queryClient.invalidateQueries({ queryKey: ['alerts'] });
                queryClient.invalidateQueries({ queryKey: ['alertStats'] });

                if (newCount > 0) {
                    showToast(`Received ${newCount} new alert${newCount > 1 ? 's' : ''}`, 'success');
                }
            }, 1000);
        };

        const stream = createAlertStream((alert) => {
            if (!alert?.id || seenAlertIdsRef.current.has(alert.id)) {
                return;
            }
            seenAlertIdsRef.current.add(alert.id);
            pendingStreamIdsRef.current.add(alert.id);
            triggerDebouncedSync();
        });

        return () => {
            stream.close();
            if (streamSyncTimerRef.current) {
                clearTimeout(streamSyncTimerRef.current);
                streamSyncTimerRef.current = null;
            }
            pendingStreamIdsRef.current.clear();
        };
    }, [queryClient, showToast]);

    // Filter locally for multi-select and search - REMOVED for strict backend filtering

    const updateStatusMutation = useMutation({
        mutationFn: ({ id, status }: { id: string; status: string }) =>
            alertsApi.updateStatus(id, status),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['alerts'] });
            queryClient.invalidateQueries({ queryKey: ['alertStats'] });
            showToast('Alert status updated', 'success');
        },
        onError: () => { showToast('Failed to update alert status', 'error'); },
    });

    const bulkUpdateMutation = useMutation({
        mutationFn: ({ ids, status }: { ids: string[]; status: string }) =>
            alertsApi.bulkUpdateStatus(ids, status),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['alerts'] });
            queryClient.invalidateQueries({ queryKey: ['alertStats'] });
            setSelectedIds(new Set());
            showToast(`${selectedIds.size} alerts updated`, 'success');
        },
        onError: () => { showToast('Failed to update alerts', 'error'); },
    });

    const handleStatusChange = (id: string, status: string) => {
        updateStatusMutation.mutate({ id, status });
        setSelectedAlert(null);
    };

    const handleBulkAction = (status: string) => {
        bulkUpdateMutation.mutate({ ids: Array.from(selectedIds), status });
    };

    const toggleSelectAll = () => {
        if (selectedIds.size === alerts.length) {
            setSelectedIds(new Set());
        } else {
            setSelectedIds(new Set(alerts.map((a) => a.id)));
        }
    };

    const toggleSelect = (id: string) => {
        const newSet = new Set(selectedIds);
        if (newSet.has(id)) { newSet.delete(id); } else { newSet.add(id); }
        setSelectedIds(newSet);
    };

    const toggleSort = (field: SortField) => {
        if (sortBy === field) {
            setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
        } else {
            setSortBy(field);
            setSortOrder('desc');
        }
        setPage(1);
    };

    if (error) {
        return (
            <div className="card text-center py-12">
                <AlertTriangle className="w-12 h-12 text-red-400 mx-auto mb-4" />
                <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">Failed to Load Alerts</h3>
                <p className="text-gray-500">Please try again later.</p>
            </div>
        );
    }

    return (
        <div className="relative flex flex-col min-h-[calc(100vh-2rem)] lg:min-h-[calc(100vh-1rem)] h-full -mx-4 sm:-mx-6 lg:-mx-8 -my-4 sm:-my-6 lg:-my-8 p-4 sm:p-6 lg:p-8 bg-slate-50 dark:bg-gradient-to-br dark:from-slate-900 dark:via-[#0b1120] dark:to-slate-900 transition-colors overflow-hidden">
            {/* Background ambient glow effect for Alerts specific interface */}
            <div className="absolute top-0 left-1/4 w-[800px] h-[500px] pointer-events-none -translate-y-1/2" style={{ background: 'radial-gradient(circle, rgba(6,182,212,0.08) 0%, transparent 70%)' }} />

            <div className="relative flex-1 flex flex-col min-h-0 space-y-4 lg:space-y-6 max-w-[1600px] mx-auto w-full">
                <div className="flex items-center justify-between shrink-0">
                    <div>
                        <h1 className="text-3xl font-bold text-slate-900 dark:text-white tracking-tight">Alerts</h1>
                        <p className="text-sm text-slate-500 mt-1">Deep triage and historic threat analysis</p>
                    </div>
                    <div className="flex items-center gap-2 text-sm text-slate-500 bg-white/50 dark:bg-slate-800/50 px-3 py-1.5 rounded-full border border-slate-200 dark:border-slate-700/50 backdrop-blur-sm shadow-sm">
                        <TrendingUp className="w-4 h-4 text-cyan-600 dark:text-cyan-400" />
                        <span>Sorted by <span className="font-semibold text-slate-800 dark:text-cyan-300">Risk Score</span></span>
                    </div>
                </div>

                {/* Filters */}
                <div className="relative z-20 shrink-0 bg-white dark:bg-slate-900/50 border border-slate-200/80 dark:border-slate-700/50 rounded-xl p-4 shadow-sm">
                    <div className="flex flex-wrap gap-4 items-end">
                        <MultiSelect
                            options={SEVERITY_OPTIONS}
                            selected={filters.severities}
                            onChange={(severities) => setFilters({ ...filters, severities })}
                            placeholder="All Severities"
                            label="Severity"
                        />
                        <MultiSelect
                            options={STATUS_OPTIONS}
                            selected={filters.statuses}
                            onChange={(statuses) => setFilters({ ...filters, statuses })}
                            placeholder="All Statuses"
                            label="Status"
                        />
                        <DateRangePicker
                            value={dateRange}
                            onChange={setDateRange}
                            label="Date Range"
                        />
                        <div className="flex-1 min-w-[200px]">
                            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Search</label>
                            <div className="relative">
                                <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
                                <input
                                    type="text"
                                    placeholder="   Search by rule, agent..."
                                    value={filters.search}
                                    onChange={(e) => setFilters({ ...filters, search: e.target.value })}
                                    className="input pl-9"
                                />
                            </div>
                        </div>
                    </div>
                </div>

                {/* Bulk Actions */}
                <BulkActionsToolbar
                    selectedCount={selectedIds.size}
                    onAction={handleBulkAction}
                    onClear={() => setSelectedIds(new Set())}
                />

                {/* Alerts Table */}
                <div className="relative z-10 flex-1 flex flex-col min-h-0 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700/50 rounded-2xl shadow-lg overflow-hidden">
                    {isLoading ? (
                        <div className="p-4 flex-1 overflow-auto">
                            <SkeletonTable rows={10} columns={8} />
                        </div>
                    ) : alerts.length === 0 ? (
                        <div className="flex-1 flex flex-col items-center justify-center text-center py-12">
                            <Shield className="w-12 h-12 text-green-400 mx-auto mb-4 opacity-50" />
                            <h3 className="text-lg font-medium text-slate-900 dark:text-white mb-2">No Alerts Found</h3>
                            <p className="text-slate-500">
                                {filters.search || filters.severities.length || filters.statuses.length
                                    ? 'Try adjusting your filters'
                                    : 'All clear! No alerts in this time range.'}
                            </p>
                        </div>
                    ) : (
                        <div className="flex-1 overflow-auto custom-scrollbar transform-gpu">
                            <table className="w-full text-left text-sm whitespace-nowrap">
                                <thead className="sticky top-0 z-10 bg-slate-100 dark:bg-slate-800 border-b-2 border-slate-200 dark:border-slate-700/80 text-xs uppercase tracking-wider text-slate-600 dark:text-slate-300 font-bold shadow-sm">
                                    <tr>
                                        <th className="py-4 px-4 w-10">
                                            <input
                                                type="checkbox"
                                                checked={selectedIds.size === alerts.length && alerts.length > 0}
                                                onChange={toggleSelectAll}
                                                className="rounded"
                                            />
                                        </th>
                                        {/* Risk Score column — primary sort */}
                                        <th className="py-4 px-4 w-24">
                                            <button
                                                onClick={() => toggleSort('risk_score')}
                                                className={`flex items-center gap-1 hover:text-slate-700 dark:hover:text-slate-200 ${sortBy === 'risk_score' ? 'text-primary-600 dark:text-primary-400' : ''}`}
                                            >
                                                <TrendingUp className="w-3.5 h-3.5" />
                                                Risk
                                                <ArrowUpDown className={`w-3 h-3 ${sortBy === 'risk_score' ? 'text-primary-500' : ''}`} />
                                            </button>
                                        </th>
                                        <th className="py-4 px-4">
                                            <button
                                                onClick={() => toggleSort('timestamp')}
                                                className="flex items-center gap-1 hover:text-slate-700 dark:hover:text-slate-200"
                                            >
                                                Time
                                                <ArrowUpDown className={`w-3 h-3 ${sortBy === 'timestamp' ? 'text-primary-500' : ''}`} />
                                            </button>
                                        </th>
                                        <th className="py-4 px-4">Rule</th>
                                        <th className="py-4 px-4">
                                            <button
                                                onClick={() => toggleSort('severity')}
                                                className="flex items-center gap-1 hover:text-slate-700 dark:hover:text-slate-200"
                                            >
                                                Severity
                                                <ArrowUpDown className={`w-3 h-3 ${sortBy === 'severity' ? 'text-primary-500' : ''}`} />
                                            </button>
                                        </th>
                                        <th className="py-4 px-4">Status</th>
                                        <th className="py-4 px-4">Agent</th>
                                        <th className="py-4 px-4">Actions</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {alerts.map((alert) => {
                                        const StatusIcon = statusIcons[alert.status] || AlertTriangle;
                                        const hasContext = !!alert.context_snapshot;
                                        const uebaSignal = alert.score_breakdown?.ueba_signal || alert.context_snapshot?.score_breakdown?.ueba_signal;
                                        return (
                                            <tr
                                                key={alert.id}
                                                className={`border-b border-slate-100 dark:border-slate-800/60 transition-all duration-200 ${selectedIds.has(alert.id)
                                                    ? 'bg-primary-50 dark:bg-primary-900/20'
                                                    : 'hover:bg-slate-50 dark:hover:bg-slate-800/40'
                                                    }`}
                                            >
                                                <td className="py-4 px-4">
                                                    <input
                                                        type="checkbox"
                                                        checked={selectedIds.has(alert.id)}
                                                        onChange={() => toggleSelect(alert.id)}
                                                        className="rounded"
                                                    />
                                                </td>
                                                {/* Risk Score cell */}
                                                <td className="py-4 px-4">
                                                    <div className="flex items-center gap-1.5">
                                                        <RiskScoreBadge score={alert.risk_score} />
                                                        {uebaSignal === 'anomaly' && (
                                                            <span title="Baseline Anomaly">
                                                                <Zap className="w-3 h-3 text-red-500" />
                                                            </span>
                                                        )}
                                                        {uebaSignal === 'normal' && (
                                                            <span title="Normalcy Discount">
                                                                <CheckCircle className="w-3 h-3 text-green-500" />
                                                            </span>
                                                        )}
                                                    </div>
                                                </td>
                                                <td className="whitespace-nowrap text-sm py-4 px-4">
                                                    {new Date(alert.timestamp).toLocaleString()}
                                                </td>
                                                <td className="py-4 px-4">
                                                    <div className="max-w-xs">
                                                        <p className="font-medium text-slate-800 dark:text-slate-200 truncate whitespace-normal">
                                                            {alert.rule_title}
                                                        </p>
                                                        <div className="flex items-center gap-1.5 mt-0.5">
                                                            {alert.mitre_techniques?.[0] && (
                                                                <p className="text-xs text-primary-600 dark:text-primary-400">
                                                                    {alert.mitre_techniques[0]}
                                                                </p>
                                                            )}
                                                            {hasContext && (
                                                                <span className="text-xs text-slate-400" title="Context snapshot available">
                                                                    <GitBranch className="w-3 h-3 inline" />
                                                                </span>
                                                            )}
                                                        </div>
                                                    </div>
                                                </td>
                                                <td className="py-4 px-4">
                                                    <span className={`badge px-2.5 py-1 ${severityColors[alert.severity]}`}>
                                                        {alert.severity.toUpperCase()}
                                                    </span>
                                                </td>
                                                <td className="py-4 px-4">
                                                    <span className={`badge px-2.5 py-1 ${statusColors[alert.status]} flex items-center gap-1.5 w-fit font-medium`}>
                                                        <StatusIcon className="w-3.5 h-3.5" />
                                                        {alert.status.replace('_', ' ')}
                                                    </span>
                                                </td>
                                                <td className="text-sm text-slate-500 dark:text-slate-400 font-mono py-4 px-4">
                                                    {alert.agent_id?.slice(0, 12)}...
                                                </td>
                                                <td className="py-4 px-4">
                                                    <div className="flex gap-1">
                                                        <button
                                                            onClick={() => setSelectedAlert(alert)}
                                                            className="p-1.5 text-slate-400 hover:text-primary-600 hover:bg-slate-100 dark:hover:bg-slate-700/50 rounded transition-colors"
                                                            title="View Details"
                                                        >
                                                            <Eye className="w-4 h-4" />
                                                        </button>
                                                        {alert.status === 'open' && authApi.canWriteAlerts() && (
                                                            <button
                                                                onClick={() => handleStatusChange(alert.id, 'acknowledged')}
                                                                className="p-1.5 text-slate-400 hover:text-green-600 hover:bg-green-50 dark:hover:bg-green-900/20 rounded transition-colors"
                                                                title="Acknowledge"
                                                            >
                                                                <Check className="w-4 h-4" />
                                                            </button>
                                                        )}
                                                    </div>
                                                </td>
                                            </tr>
                                        );
                                    })}
                                </tbody>
                            </table>
                        </div>
                    )}

                    {/* Pagination */}
                    <div className="shrink-0 z-20">
                        <Pagination
                            page={page}
                            totalPages={totalPages}
                            pageSize={pageSize}
                            total={total}
                            onPageChange={setPage}
                            onPageSizeChange={(size) => {
                                setPageSize(size);
                                setPage(1);
                            }}
                        />
                    </div>
                </div>

                <AlertDetailModal
                    alert={selectedAlert}
                    isOpen={!!selectedAlert}
                    onClose={() => setSelectedAlert(null)}
                    onStatusChange={handleStatusChange}
                />
            </div>
        </div>
    );
}
