import type { MultiSelectOption } from '../MultiSelect';
import {
    AlertTriangle, Clock, CheckCircle, XCircle, Eye,
    type LucideIcon,
} from 'lucide-react';

// Severity options with counts
export const SEVERITY_OPTIONS: MultiSelectOption[] = [
    { value: 'critical', label: 'Critical', color: '#ef4444' },
    { value: 'high', label: 'High', color: '#f97316' },
    { value: 'medium', label: 'Medium', color: '#eab308' },
    { value: 'low', label: 'Low', color: '#6366f1' },
    { value: 'informational', label: 'Info', color: '#3b82f6' },
];

// Status options
export const STATUS_OPTIONS: MultiSelectOption[] = [
    { value: 'open', label: 'Open' },
    { value: 'in_progress', label: 'In Progress' },
    { value: 'acknowledged', label: 'Acknowledged' },
    { value: 'resolved', label: 'Resolved' },
    { value: 'false_positive', label: 'False Positive' },
];

// Severity badge colors
export const severityColors: Record<string, string> = {
    critical: 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/20',
    high: 'bg-orange-500/10 text-orange-600 dark:text-orange-400 border border-orange-500/20',
    medium: 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20',
    low: 'bg-indigo-500/10 text-indigo-600 dark:text-indigo-400 border border-indigo-500/20',
    informational: 'bg-cyan-500/10 text-cyan-600 dark:text-cyan-400 border border-cyan-500/20',
};

// Status badge colors
export const statusColors: Record<string, string> = {
    open: 'bg-rose-600/10 text-rose-700 dark:text-rose-400 border border-rose-600/20',
    in_progress: 'bg-amber-500/10 text-amber-700 dark:text-amber-400 border border-amber-500/20',
    acknowledged: 'bg-cyan-500/10 text-cyan-700 dark:text-cyan-400 border border-cyan-500/20',
    resolved: 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border border-emerald-500/20',
    false_positive: 'bg-slate-500/10 text-slate-700 dark:text-slate-400 border border-slate-500/20',
    closed: 'bg-slate-500/10 text-slate-700 dark:text-slate-400 border border-slate-500/20',
};

// Severity left-border stripe colour
export const severityStripe: Record<string, string> = {
    critical: 'border-l-rose-500',
    high: 'border-l-orange-500',
    medium: 'border-l-amber-400',
    low: 'border-l-indigo-400',
    informational: 'border-l-cyan-400',
};

// Status icons
export const statusIcons: Record<string, LucideIcon> = {
    open: AlertTriangle,
    in_progress: Clock,
    acknowledged: Eye,
    resolved: CheckCircle,
    false_positive: XCircle,
    closed: XCircle,
};
