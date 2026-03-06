import { useState, useEffect } from 'react';
import { User, Bell, Globe, Key, Moon, Sun, Save, Check } from 'lucide-react';

interface UserSettings {
    displayName: string;
    email: string;
    timezone: string;
    dateFormat: string;
    darkMode: boolean;
    notifications: {
        email: boolean;
        browser: boolean;
        critical: boolean;
        high: boolean;
        medium: boolean;
        low: boolean;
    };
    integrations: {
        webhookUrl: string;
        slackChannel: string;
        splunkEnabled: boolean;
    };
}

const TIMEZONES = [
    { value: 'UTC', label: 'UTC' },
    { value: 'America/New_York', label: 'Eastern Time' },
    { value: 'America/Los_Angeles', label: 'Pacific Time' },
    { value: 'Europe/London', label: 'London' },
    { value: 'Europe/Paris', label: 'Paris' },
    { value: 'Asia/Tokyo', label: 'Tokyo' },
];

export default function Settings() {
    const [saved, setSaved] = useState(false);

    // Load user profile from JWT/login data stored in localStorage
    const storedUser = (() => {
        try {
            const raw = localStorage.getItem('user');
            return raw ? JSON.parse(raw) : null;
        } catch { return null; }
    })();

    const [settings, setSettings] = useState<UserSettings>({
        displayName: storedUser?.full_name || storedUser?.username || 'User',
        email: storedUser?.email || '',
        timezone: 'UTC',
        dateFormat: 'YYYY-MM-DD',
        darkMode: document.documentElement.classList.contains('dark'),
        notifications: {
            email: true,
            browser: true,
            critical: true,
            high: true,
            medium: false,
            low: false,
        },
        integrations: {
            webhookUrl: '',
            slackChannel: '',
            splunkEnabled: false,
        },
    });

    // Load settings from localStorage
    useEffect(() => {
        const savedSettings = localStorage.getItem('user_settings');
        if (savedSettings) {
            setSettings(JSON.parse(savedSettings));
        }
    }, []);

    const handleSave = () => {
        localStorage.setItem('user_settings', JSON.stringify(settings));
        setSaved(true);
        setTimeout(() => setSaved(false), 2000);
    };

    const toggleDarkMode = () => {
        const newValue = !settings.darkMode;
        setSettings({ ...settings, darkMode: newValue });
        document.documentElement.classList.toggle('dark', newValue);
    };

    return (
        <div>
            <div className="flex justify-between items-center mb-6">
                <h1 className="text-3xl font-bold text-gray-900 dark:text-white">Settings</h1>
                <button
                    onClick={handleSave}
                    className="btn btn-primary flex items-center gap-2"
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

            <div className="space-y-6">
                {/* User Profile */}
                <div className="card">
                    <div className="flex items-center gap-3 mb-4">
                        <User className="w-5 h-5 text-primary-600" />
                        <h2 className="text-lg font-semibold">User Profile</h2>
                    </div>
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                        <div>
                            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                                Display Name
                            </label>
                            <input
                                type="text"
                                value={settings.displayName}
                                onChange={(e) => setSettings({ ...settings, displayName: e.target.value })}
                                className="input"
                            />
                        </div>
                        <div>
                            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                                Email Address
                            </label>
                            <input
                                type="email"
                                value={settings.email}
                                disabled
                                className="input bg-gray-100 dark:bg-gray-600 cursor-not-allowed"
                            />
                        </div>
                        <div>
                            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                                Timezone
                            </label>
                            <select
                                value={settings.timezone}
                                onChange={(e) => setSettings({ ...settings, timezone: e.target.value })}
                                className="input"
                            >
                                {TIMEZONES.map((tz) => (
                                    <option key={tz.value} value={tz.value}>{tz.label}</option>
                                ))}
                            </select>
                        </div>
                        <div>
                            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                                Date Format
                            </label>
                            <select
                                value={settings.dateFormat}
                                onChange={(e) => setSettings({ ...settings, dateFormat: e.target.value })}
                                className="input"
                            >
                                <option value="YYYY-MM-DD">2026-01-10</option>
                                <option value="MM/DD/YYYY">01/10/2026</option>
                                <option value="DD/MM/YYYY">10/01/2026</option>
                            </select>
                        </div>
                    </div>
                </div>

                {/* Appearance */}
                <div className="card">
                    <div className="flex items-center gap-3 mb-4">
                        <Globe className="w-5 h-5 text-primary-600" />
                        <h2 className="text-lg font-semibold">Appearance</h2>
                    </div>
                    <div className="flex items-center justify-between p-4 bg-gray-50 dark:bg-gray-700 rounded-lg">
                        <div className="flex items-center gap-3">
                            {settings.darkMode ? (
                                <Moon className="w-5 h-5 text-gray-600" />
                            ) : (
                                <Sun className="w-5 h-5 text-yellow-500" />
                            )}
                            <div>
                                <p className="font-medium">Dark Mode</p>
                                <p className="text-sm text-gray-500">Toggle dark/light theme</p>
                            </div>
                        </div>
                        <button
                            onClick={toggleDarkMode}
                            className={`relative w-14 h-7 rounded-full transition-colors ${settings.darkMode ? 'bg-primary-600' : 'bg-gray-300'
                                }`}
                        >
                            <span
                                className={`absolute top-1 w-5 h-5 bg-white rounded-full transition-transform ${settings.darkMode ? 'translate-x-8' : 'translate-x-1'
                                    }`}
                            />
                        </button>
                    </div>
                </div>

                {/* Notifications */}
                <div className="card">
                    <div className="flex items-center gap-3 mb-4">
                        <Bell className="w-5 h-5 text-primary-600" />
                        <h2 className="text-lg font-semibold">Notification Preferences</h2>
                    </div>
                    <div className="space-y-3">
                        {[
                            { key: 'email', label: 'Email Notifications' },
                            { key: 'browser', label: 'Browser Notifications' },
                            { key: 'critical', label: 'Critical Alerts' },
                            { key: 'high', label: 'High Severity Alerts' },
                            { key: 'medium', label: 'Medium Severity Alerts' },
                            { key: 'low', label: 'Low Severity Alerts' },
                        ].map((item) => (
                            <label
                                key={item.key}
                                className="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-700 rounded-lg cursor-pointer"
                            >
                                <span>{item.label}</span>
                                <input
                                    type="checkbox"
                                    checked={settings.notifications[item.key as keyof typeof settings.notifications]}
                                    onChange={(e) =>
                                        setSettings({
                                            ...settings,
                                            notifications: {
                                                ...settings.notifications,
                                                [item.key]: e.target.checked,
                                            },
                                        })
                                    }
                                    className="w-4 h-4 text-primary-600 rounded"
                                />
                            </label>
                        ))}
                    </div>
                </div>

                {/* Integrations */}
                <div className="card">
                    <div className="flex items-center gap-3 mb-4">
                        <Key className="w-5 h-5 text-primary-600" />
                        <h2 className="text-lg font-semibold">Integrations (Week 6)</h2>
                    </div>
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                        <div>
                            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                                Webhook URL
                            </label>
                            <input
                                type="url"
                                value={settings.integrations.webhookUrl}
                                onChange={(e) =>
                                    setSettings({
                                        ...settings,
                                        integrations: { ...settings.integrations, webhookUrl: e.target.value },
                                    })
                                }
                                placeholder="https://your-webhook.com/alerts"
                                className="input"
                            />
                        </div>
                        <div>
                            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                                Slack Channel
                            </label>
                            <input
                                type="text"
                                value={settings.integrations.slackChannel}
                                onChange={(e) =>
                                    setSettings({
                                        ...settings,
                                        integrations: { ...settings.integrations, slackChannel: e.target.value },
                                    })
                                }
                                placeholder="#security-alerts"
                                className="input"
                            />
                        </div>
                    </div>
                    <p className="mt-4 text-sm text-gray-500 dark:text-gray-400">
                        Full integration support coming in Week 6 (Webhooks, Splunk, ServiceNow)
                    </p>
                </div>
            </div>
        </div>
    );
}
