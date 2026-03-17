import { Globe, Moon, Save, Check, Sun, User } from 'lucide-react';
import type { GeneralSettings } from './types';
import { TIMEZONES } from './types';

interface GeneralTabProps {
    settings: GeneralSettings;
    onChange: (updated: GeneralSettings) => void;
    onSave: () => void;
    saved: boolean;
}

export default function GeneralTab({ settings, onChange, onSave, saved }: GeneralTabProps) {
    const set = (partial: Partial<GeneralSettings>) => onChange({ ...settings, ...partial });

    const toggleDarkMode = () => {
        const newValue = !settings.darkMode;
        document.documentElement.classList.toggle('dark', newValue);
        set({ darkMode: newValue });
    };

    return (
        <div className="space-y-6 animate-fade-in">
            {/* ── Save Button ── */}
            <div className="flex justify-end">
                <button
                    onClick={onSave}
                    className={`btn flex items-center gap-2 transition-all ${saved ? 'btn-success' : 'btn-primary'
                        }`}
                >
                    {saved ? (
                        <>
                            <Check className="w-4 h-4" />
                            Saved!
                        </>
                    ) : (
                        <>
                            <Save className="w-4 h-4" />
                            Save Changes
                        </>
                    )}
                </button>
            </div>

            {/* ── User Profile ── */}
            <div className="card">
                <div className="flex items-center gap-3 mb-5">
                    <div className="w-9 h-9 rounded-lg bg-primary-100 dark:bg-primary-900/40 flex items-center justify-center">
                        <User className="w-5 h-5 text-primary-600 dark:text-primary-400" />
                    </div>
                    <div>
                        <h2 className="text-base font-semibold text-gray-900 dark:text-white">User Profile</h2>
                        <p className="text-xs text-gray-500 dark:text-gray-400">Manage your display name and account details</p>
                    </div>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                            Display Name
                        </label>
                        <input
                            type="text"
                            value={settings.displayName}
                            onChange={(e) => set({ displayName: e.target.value })}
                            className="input"
                            placeholder="Your name"
                        />
                    </div>
                    <div>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                            Email Address
                        </label>
                        <div className="relative">
                            <input
                                type="email"
                                value={settings.email}
                                disabled
                                className="input bg-gray-100 dark:bg-gray-700/60 cursor-not-allowed text-gray-500 dark:text-gray-400 pr-16"
                            />
                            <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs font-medium text-gray-400 dark:text-gray-500 bg-gray-100 dark:bg-gray-700 px-1.5 py-0.5 rounded">
                                Read-only
                            </span>
                        </div>
                    </div>
                </div>
            </div>

            {/* ── Timezone & Regional ── */}
            <div className="card">
                <div className="flex items-center gap-3 mb-5">
                    <div className="w-9 h-9 rounded-lg bg-blue-100 dark:bg-blue-900/40 flex items-center justify-center">
                        <Globe className="w-5 h-5 text-blue-600 dark:text-blue-400" />
                    </div>
                    <div>
                        <h2 className="text-base font-semibold text-gray-900 dark:text-white">Timezone & Regional</h2>
                        <p className="text-xs text-gray-500 dark:text-gray-400">Configure how dates and times are displayed</p>
                    </div>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                            Timezone
                        </label>
                        <select
                            value={settings.timezone}
                            onChange={(e) => set({ timezone: e.target.value })}
                            className="input"
                        >
                            {TIMEZONES.map((tz) => (
                                <option key={tz.value} value={tz.value}>
                                    {tz.label}
                                </option>
                            ))}
                        </select>
                    </div>
                    <div>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                            Date Format
                        </label>
                        <select
                            value={settings.dateFormat}
                            onChange={(e) => set({ dateFormat: e.target.value })}
                            className="input"
                        >
                            <option value="YYYY-MM-DD">ISO — 2026-03-09</option>
                            <option value="MM/DD/YYYY">US — 03/09/2026</option>
                            <option value="DD/MM/YYYY">EU — 09/03/2026</option>
                            <option value="DD MMM YYYY">Long — 09 Mar 2026</option>
                        </select>
                    </div>
                </div>
            </div>

            {/* ── Appearance ── */}
            <div className="card">
                <div className="flex items-center gap-3 mb-5">
                    <div className="w-9 h-9 rounded-lg bg-purple-100 dark:bg-purple-900/40 flex items-center justify-center">
                        <Moon className="w-5 h-5 text-purple-600 dark:text-purple-400" />
                    </div>
                    <div>
                        <h2 className="text-base font-semibold text-gray-900 dark:text-white">Appearance</h2>
                        <p className="text-xs text-gray-500 dark:text-gray-400">Customize the visual theme of the platform</p>
                    </div>
                </div>

                <div className="flex items-center justify-between p-4 bg-gray-50 dark:bg-gray-700/60 rounded-xl border border-gray-100 dark:border-gray-700">
                    <div className="flex items-center gap-3">
                        <div className="w-9 h-9 rounded-lg bg-white dark:bg-gray-800 shadow-sm flex items-center justify-center">
                            {settings.darkMode ? (
                                <Moon className="w-5 h-5 text-indigo-500" />
                            ) : (
                                <Sun className="w-5 h-5 text-amber-500" />
                            )}
                        </div>
                        <div>
                            <p className="text-sm font-medium text-gray-900 dark:text-white">Dark Mode</p>
                            <p className="text-xs text-gray-500 dark:text-gray-400">
                                {settings.darkMode ? 'Dark theme is active' : 'Light theme is active'}
                            </p>
                        </div>
                    </div>

                    {/* Pill Toggle — inline-flex pattern keeps thumb inside track */}
                    <button
                        type="button"
                        onClick={toggleDarkMode}
                        role="switch"
                        aria-checked={settings.darkMode}
                        className={`relative inline-flex h-7 w-14 shrink-0 cursor-pointer rounded-full border-2 border-transparent
                            transition-colors duration-200 ease-in-out
                            focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2 dark:focus-visible:ring-offset-gray-800
                            ${settings.darkMode ? 'bg-primary-600' : 'bg-gray-300 dark:bg-gray-600'}`}
                    >
                        <span className="sr-only">Toggle dark mode</span>
                        <span
                            aria-hidden="true"
                            className={`pointer-events-none inline-block h-6 w-6 rounded-full bg-white shadow-md ring-0
                                transition-transform duration-200 ease-in-out
                                ${settings.darkMode ? 'translate-x-7' : 'translate-x-0'}`}
                        />
                    </button>
                </div>
            </div>
        </div>
    );
}
