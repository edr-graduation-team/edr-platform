import { Bell, Mail, MessageSquare, Monitor, Check, Save } from 'lucide-react';
import type { DeliveryMethod, NotificationSettings, Severity } from './types';
import { DELIVERY_METHODS, SEVERITIES, createDefaultNotifications } from './types';

interface NotificationsTabProps {
    settings: NotificationSettings;
    onChange: (updated: NotificationSettings) => void;
    onSave: () => void;
    saved: boolean;
}

// Icon map for delivery channels
const METHOD_ICONS: Record<DeliveryMethod, React.ElementType> = {
    email: Mail,
    slack: MessageSquare,
    browser: Monitor,
};

// Severity ring / badge colour
const SEVERITY_RING: Record<Severity, string> = {
    critical: 'ring-red-500 dark:ring-red-600',
    high: 'ring-orange-500 dark:ring-orange-600',
    medium: 'ring-yellow-500 dark:ring-yellow-600',
    low: 'ring-blue-500 dark:ring-blue-600',
};

const SEVERITY_DOT: Record<Severity, string> = {
    critical: 'bg-red-600',
    high: 'bg-orange-500',
    medium: 'bg-yellow-500',
    low: 'bg-blue-500',
};

const SEVERITY_ACTIVE_COLOR: Record<Severity, string> = {
    critical: 'bg-red-600',
    high: 'bg-orange-500',
    medium: 'bg-yellow-500',
    low: 'bg-blue-500',
};

/**
 * Pill toggle switch — uses the Headless-UI / Tailwind-UI pattern:
 * `inline-flex` + `border-2 border-transparent` as inset spacing so that
 * `translate-x-5` (one thumb-width) always lands flush inside the track.
 */
function ToggleSwitch({
    checked,
    onChange,
    severity,
}: {
    checked: boolean;
    onChange: () => void;
    severity: Severity;
}) {
    return (
        <button
            type="button"
            role="switch"
            aria-checked={checked}
            onClick={onChange}
            className={`relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent
        transition-colors duration-200 ease-in-out
        focus:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 dark:focus-visible:ring-offset-gray-800 focus-visible:ring-primary-500
        ${checked ? SEVERITY_ACTIVE_COLOR[severity] : 'bg-gray-300 dark:bg-gray-600'}`}
        >
            <span className="sr-only">Toggle notification</span>
            <span
                aria-hidden="true"
                className={`pointer-events-none inline-block h-5 w-5 rounded-full bg-white shadow-md ring-0
          transition-transform duration-200 ease-in-out
          ${checked ? 'translate-x-5' : 'translate-x-0'}`}
            />
        </button>
    );
}

export default function NotificationsTab({ settings, onChange, onSave, saved }: NotificationsTabProps) {
    const toggle = (severity: Severity, method: DeliveryMethod) => {
        onChange({
            ...settings,
            [severity]: {
                ...settings[severity],
                [method]: !settings[severity][method],
            },
        });
    };

    const toggleAllForMethod = (method: DeliveryMethod, enabled: boolean) => {
        const updated = { ...settings };
        SEVERITIES.forEach((s) => {
            updated[s.id] = { ...updated[s.id], [method]: enabled };
        });
        onChange(updated);
    };

    const toggleAllForSeverity = (severity: Severity, enabled: boolean) => {
        onChange({
            ...settings,
            [severity]: { email: enabled, slack: enabled, browser: enabled },
        });
    };

    const isMethodAllEnabled = (method: DeliveryMethod) =>
        SEVERITIES.every((s) => settings[s.id][method]);

    const isSeverityAllEnabled = (severity: Severity) =>
        DELIVERY_METHODS.every((m) => settings[severity][m.id]);

    const allEnabled = DELIVERY_METHODS.every((m) =>
        SEVERITIES.every((s) => settings[s.id][m.id])
    );

    const enabledCount = SEVERITIES.reduce(
        (total, s) =>
            total + DELIVERY_METHODS.filter((m) => settings[s.id][m.id]).length,
        0
    );
    const totalCount = SEVERITIES.length * DELIVERY_METHODS.length;

    return (
        <div className="space-y-6 animate-fade-in">
            {/* ── Save Button ── */}
            <div className="flex justify-end">
                <button
                    onClick={onSave}
                    className={`btn flex items-center gap-2 transition-all ${saved ? 'btn-success' : 'btn-primary'}`}
                >
                    {saved ? (
                        <><Check className="w-4 h-4" />Saved!</>
                    ) : (
                        <><Save className="w-4 h-4" />Save Changes</>
                    )}
                </button>
            </div>

            <div className="card">
                {/* ── Header ── */}
                <div className="flex items-start justify-between mb-6">
                    <div className="flex items-center gap-3">
                        <div className="w-9 h-9 rounded-lg bg-amber-100 dark:bg-amber-900/40 flex items-center justify-center">
                            <Bell className="w-5 h-5 text-amber-600 dark:text-amber-400" />
                        </div>
                        <div>
                            <h2 className="text-base font-semibold text-gray-900 dark:text-white">Notification Matrix</h2>
                            <p className="text-xs text-gray-500 dark:text-gray-400">
                                Control delivery per severity level — {enabledCount}/{totalCount} channels active
                            </p>
                        </div>
                    </div>
                </div>

                {/* ── Matrix Table ── */}
                <div className="overflow-x-auto rounded-xl border border-gray-100 dark:border-gray-700">
                    <table className="w-full">
                        <thead>
                            <tr className="bg-gray-50 dark:bg-gray-700/60 border-b border-gray-200 dark:border-gray-700">
                                <th className="text-left py-3.5 px-5 text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 w-40">
                                    Severity
                                </th>
                                {DELIVERY_METHODS.map((method) => {
                                    const Icon = METHOD_ICONS[method.id];
                                    const allOn = isMethodAllEnabled(method.id);
                                    return (
                                        <th
                                            key={method.id}
                                            className="text-center py-3.5 px-6 text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400"
                                        >
                                            <div className="flex flex-col items-center gap-1.5">
                                                <div className="flex items-center gap-1.5">
                                                    <Icon className="w-4 h-4" />
                                                    <span>{method.label}</span>
                                                </div>
                                                <button
                                                    onClick={() => toggleAllForMethod(method.id, !allOn)}
                                                    className="text-xs font-medium text-primary-600 dark:text-primary-400 hover:text-primary-700 dark:hover:text-primary-300 transition-colors normal-case tracking-normal"
                                                >
                                                    {allOn ? '— Disable all' : '+ Enable all'}
                                                </button>
                                            </div>
                                        </th>
                                    );
                                })}
                                <th className="text-center py-3.5 px-5 text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                                    All
                                </th>
                            </tr>
                        </thead>
                        <tbody>
                            {SEVERITIES.map((severity, idx) => {
                                const severityAllOn = isSeverityAllEnabled(severity.id);
                                return (
                                    <tr
                                        key={severity.id}
                                        className={`border-b border-gray-100 dark:border-gray-700 last:border-0 transition-colors ${idx % 2 === 0
                                            ? 'bg-white dark:bg-gray-800'
                                            : 'bg-gray-50/50 dark:bg-gray-800/60'
                                            } hover:bg-primary-50 dark:hover:bg-primary-900/10`}
                                    >
                                        {/* Severity Label */}
                                        <td className="py-4 px-5">
                                            <div className="flex items-center gap-2.5">
                                                <span className={`w-2.5 h-2.5 rounded-full flex-shrink-0 ${SEVERITY_DOT[severity.id]}`} />
                                                <span className={`font-semibold text-sm ${severity.id === 'critical' ? 'text-red-600 dark:text-red-400' :
                                                    severity.id === 'high' ? 'text-orange-600 dark:text-orange-400' :
                                                        severity.id === 'medium' ? 'text-yellow-600 dark:text-yellow-400' :
                                                            'text-blue-600 dark:text-blue-400'
                                                    }`}>
                                                    {severity.label}
                                                </span>
                                            </div>
                                        </td>

                                        {/* Method Toggles */}
                                        {DELIVERY_METHODS.map((method) => (
                                            <td key={method.id} className="text-center py-4 px-6">
                                                <div className="flex justify-center">
                                                    <ToggleSwitch
                                                        checked={!!settings[severity.id]?.[method.id]}
                                                        onChange={() => toggle(severity.id, method.id)}
                                                        severity={severity.id}
                                                    />
                                                </div>
                                            </td>
                                        ))}

                                        {/* Row "All" toggle */}
                                        <td className="text-center py-4 px-5">
                                            <button
                                                onClick={() => toggleAllForSeverity(severity.id, !severityAllOn)}
                                                className={`text-xs font-medium px-2.5 py-1 rounded-full transition-colors ${severityAllOn
                                                    ? `ring-1 ${SEVERITY_RING[severity.id]} text-gray-700 dark:text-gray-300 hover:bg-red-50 dark:hover:bg-red-900/20`
                                                    : 'bg-gray-100 dark:bg-gray-700 text-gray-500 dark:text-gray-400 hover:bg-gray-200 dark:hover:bg-gray-600'
                                                    }`}
                                            >
                                                {severityAllOn ? 'None' : 'All'}
                                            </button>
                                        </td>
                                    </tr>
                                );
                            })}
                        </tbody>
                    </table>
                </div>

                {/* ── Quick Actions Footer ── */}
                <div className="mt-5 pt-4 border-t border-gray-100 dark:border-gray-700 flex flex-wrap items-center gap-3">
                    <span className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide">
                        Quick Actions:
                    </span>
                    <button
                        onClick={() => onChange(createDefaultNotifications())}
                        className="px-3 py-1.5 text-xs font-medium bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-600 transition-colors"
                    >
                        Reset to Defaults
                    </button>
                    <button
                        onClick={() => {
                            SEVERITIES.forEach((s) => toggleAllForSeverity(s.id, !allEnabled));
                        }}
                        className="px-3 py-1.5 text-xs font-medium bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-600 transition-colors"
                    >
                        {allEnabled ? 'Disable All Channels' : 'Enable All Channels'}
                    </button>
                </div>
            </div>

            {/* ── Notification Tips ── */}
            <div className="card bg-blue-50 dark:bg-blue-900/20 border border-blue-100 dark:border-blue-800">
                <div className="flex items-start gap-3">
                    <Bell className="w-5 h-5 text-blue-600 dark:text-blue-400 flex-shrink-0 mt-0.5" />
                    <div>
                        <h3 className="text-sm font-semibold text-blue-900 dark:text-blue-200 mb-1">
                            Delivery Channel Info
                        </h3>
                        <ul className="space-y-1">
                            {DELIVERY_METHODS.map((m) => (
                                <li key={m.id} className="text-xs text-blue-700 dark:text-blue-300 flex items-center gap-2">
                                    <span className="font-semibold">{m.label}:</span> {m.description}
                                </li>
                            ))}
                        </ul>
                    </div>
                </div>
            </div>
        </div>
    );
}
