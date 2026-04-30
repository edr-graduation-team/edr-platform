import { X, Clock, ExternalLink } from 'lucide-react';
import { Link } from 'react-router-dom';
import type { Alert } from '../../api/client';

interface AlertDrawerProps {
    alert: Alert;
    agentMap: Record<string, string>;
    onClose: () => void;
}

export function AlertDrawer({ alert, agentMap, onClose }: AlertDrawerProps) {
    const severityColor: Record<string, string> = {
        critical: '#f43f5e',
        high: '#f97316',
        medium: '#f59e0b',
        low: '#3b82f6',
        informational: '#22d3ee',
    };
    const color = severityColor[alert.severity] || '#64748b';
    const hostname = agentMap[alert.agent_id] || alert.agent_id?.slice(0, 12) + '...';

    return (
        <div
            className="fixed inset-0 z-50 flex"
            onClick={(e) => e.target === e.currentTarget && onClose()}
        >
            {/* Dim backdrop */}
            <div className="flex-1 bg-black/40 backdrop-blur-sm animate-fade-in" onClick={onClose} />

            {/* Drawer panel */}
            <div
                className="
                    w-full max-w-lg bg-white dark:bg-slate-900
                    border-l border-slate-200 dark:border-slate-700
                    shadow-2xl drawer-enter flex flex-col h-full
                "
                onClick={(e) => e.stopPropagation()}
            >
                {/* Header */}
                <div className="flex items-start justify-between p-5 border-b border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/80">
                    <div className="flex-1 min-w-0 pr-4">
                        <div
                            className="text-[10px] font-bold uppercase tracking-widest mb-2 px-2 py-0.5 rounded-full inline-block"
                            style={{ color, background: `${color}20`, border: `1px solid ${color}40` }}
                        >
                            {alert.severity}
                        </div>
                        <h2 className="text-base font-bold text-slate-900 dark:text-white leading-snug">
                            {alert.rule_title}
                        </h2>
                    </div>
                    <button
                        onClick={onClose}
                        className="p-1.5 rounded-lg text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-700 transition-all"
                    >
                        <X className="w-4 h-4" />
                    </button>
                </div>

                {/* Scrollable body */}
                <div className="flex-1 overflow-y-auto p-5 space-y-5">
                    {/* Meta grid */}
                    <div className="grid grid-cols-2 gap-3">
                        {[
                            { label: 'Endpoint', value: hostname },
                            { label: 'Status', value: alert.status },
                            { label: 'Category', value: alert.category || '—' },
                            { label: 'Events', value: String(alert.event_count || 1) },
                            { label: 'Risk Score', value: alert.risk_score != null ? String(alert.risk_score) : '—' },
                            {
                                label: 'Confidence',
                                value: alert.confidence != null ? `${((alert.confidence as number) * 100).toFixed(0)}%` : '—',
                            },
                        ].map(({ label, value }) => (
                            <div key={label} className="bg-slate-100 dark:bg-slate-800 rounded-lg p-3">
                                <p className="text-[10px] text-slate-400 uppercase tracking-widest mb-0.5">{label}</p>
                                <p className="text-sm font-semibold text-slate-800 dark:text-slate-200 truncate font-mono">
                                    {value}
                                </p>
                            </div>
                        ))}
                    </div>

                    {/* MITRE Tactics */}
                    {(alert.mitre_tactics?.length ?? 0) > 0 && (
                        <div>
                            <p className="text-[10px] text-slate-400 uppercase tracking-widest mb-2">MITRE Tactics</p>
                            <div className="flex flex-wrap gap-2">
                                {alert.mitre_tactics!.map((t) => (
                                    <span key={t} className="badge badge-mitre">
                                        {t}
                                    </span>
                                ))}
                            </div>
                        </div>
                    )}

                    {/* MITRE Techniques */}
                    {(alert.mitre_techniques?.length ?? 0) > 0 && (
                        <div>
                            <p className="text-[10px] text-slate-400 uppercase tracking-widest mb-2">
                                MITRE Techniques
                            </p>
                            <div className="flex flex-wrap gap-2">
                                {alert.mitre_techniques!.map((t) => (
                                    <span key={t} className="badge badge-mitre">
                                        {t}
                                    </span>
                                ))}
                            </div>
                        </div>
                    )}

                    {/* Timestamps */}
                    <div>
                        <p className="text-[10px] text-slate-400 uppercase tracking-widest mb-2">Timeline</p>
                        <div className="space-y-1.5">
                            <div className="flex items-center gap-2 text-xs font-mono text-slate-500">
                                <Clock className="w-3 h-3 shrink-0" />
                                <span className="text-slate-400">Detected:</span>
                                {new Date(alert.timestamp).toLocaleString()}
                            </div>
                            <div className="flex items-center gap-2 text-xs font-mono text-slate-500">
                                <Clock className="w-3 h-3 shrink-0" />
                                <span className="text-slate-400">Created:</span>
                                {new Date(alert.created_at).toLocaleString()}
                            </div>
                        </div>
                    </div>
                </div>

                {/* Footer actions */}
                <div className="border-t border-slate-200 dark:border-slate-700 p-4 flex gap-3 bg-slate-50 dark:bg-slate-800/80">
                    <Link
                        to={`/alerts?id=${alert.id}`}
                        className="btn btn-primary flex-1 justify-center text-sm"
                    >
                        <ExternalLink className="w-3.5 h-3.5" />
                        View in Alerts
                    </Link>
                    <button onClick={onClose} className="btn btn-secondary text-sm">
                        Close
                    </button>
                </div>
            </div>
        </div>
    );
}
