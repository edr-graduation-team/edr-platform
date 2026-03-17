// ─── Core Domain Types ──────────────────────────────────────────────────────

export type Severity = 'critical' | 'high' | 'medium' | 'low';
export type DeliveryMethod = 'email' | 'slack' | 'browser';
export type TabId = 'general' | 'notifications' | 'integrations' | 'security';

// ─── Settings Interfaces ─────────────────────────────────────────────────────

export interface GeneralSettings {
  displayName: string;
  email: string;
  timezone: string;
  dateFormat: string;
  darkMode: boolean;
}

/**
 * Notification matrix: severity → delivery method → enabled
 * e.g. notifications['critical']['email'] = true
 */
export interface NotificationSettings {
  [severity: string]: {
    [method: string]: boolean;
  };
}

export interface IntegrationSettings {
  webhookUrl: string;
  slackChannel: string;
  splunkEnabled: boolean;
}

// ─── Persisted Full Settings Shape ───────────────────────────────────────────

export interface PersistedSettings {
  general: GeneralSettings;
  notifications: NotificationSettings;
  integrations: IntegrationSettings;
}

// ─── UI State ────────────────────────────────────────────────────────────────

export type TabSavedState = Record<TabId, boolean>;

// ─── Static Data ─────────────────────────────────────────────────────────────

export const TIMEZONES: { value: string; label: string }[] = [
  { value: 'UTC', label: 'UTC' },
  { value: 'America/New_York', label: 'Eastern Time (ET)' },
  { value: 'America/Chicago', label: 'Central Time (CT)' },
  { value: 'America/Denver', label: 'Mountain Time (MT)' },
  { value: 'America/Los_Angeles', label: 'Pacific Time (PT)' },
  { value: 'Europe/London', label: 'London (GMT/BST)' },
  { value: 'Europe/Paris', label: 'Paris (CET/CEST)' },
  { value: 'Europe/Istanbul', label: 'Istanbul (TRT)' },
  { value: 'Asia/Dubai', label: 'Dubai (GST)' },
  { value: 'Asia/Tokyo', label: 'Tokyo (JST)' },
  { value: 'Asia/Singapore', label: 'Singapore (SGT)' },
  { value: 'Australia/Sydney', label: 'Sydney (AEST)' },
];

export const SEVERITIES: { id: Severity; label: string; color: string; badgeClass: string }[] = [
  { id: 'critical', label: 'Critical', color: 'bg-red-600', badgeClass: 'badge-critical' },
  { id: 'high', label: 'High', color: 'bg-orange-500', badgeClass: 'badge-high' },
  { id: 'medium', label: 'Medium', color: 'bg-yellow-500', badgeClass: 'badge-medium' },
  { id: 'low', label: 'Low', color: 'bg-blue-500', badgeClass: 'badge-low' },
];

export const DELIVERY_METHODS: { id: DeliveryMethod; label: string; description: string }[] = [
  { id: 'email', label: 'Email', description: 'Send to registered email' },
  { id: 'slack', label: 'Slack', description: 'Post to Slack channel' },
  { id: 'browser', label: 'Dashboard', description: 'In-app notification' },
];

// ─── Default State Factories ─────────────────────────────────────────────────

export const createDefaultNotifications = (): NotificationSettings => ({
  critical: { email: true, slack: true, browser: true },
  high:     { email: true, slack: true, browser: true },
  medium:   { email: false, slack: false, browser: true },
  low:      { email: false, slack: false, browser: false },
});

export const createDefaultGeneral = (): GeneralSettings => {
  const storedUser = (() => {
    try {
      const raw = localStorage.getItem('user');
      return raw ? JSON.parse(raw) : null;
    } catch {
      return null;
    }
  })();

  return {
    displayName: storedUser?.full_name || storedUser?.username || 'User',
    email: storedUser?.email || '',
    timezone: 'UTC',
    dateFormat: 'YYYY-MM-DD',
    darkMode: document.documentElement.classList.contains('dark'),
  };
};

export const createDefaultIntegrations = (): IntegrationSettings => ({
  webhookUrl: '',
  slackChannel: '',
  splunkEnabled: false,
});
