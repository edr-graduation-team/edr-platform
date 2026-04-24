export type SettingsTab = 'system';

export interface SettingsNavItem {
    id: SettingsTab;
    label: string;
    icon: string;
    description: string;
    requiredRole?: string[];
}

export const SETTINGS_NAV: SettingsNavItem[] = [
    {
        id: 'system',
        label: 'Platform preferences',
        icon: 'Settings',
        description: 'Dashboard-only appearance and local preferences (not server cluster config)',
    },
];

export function filterSettingsNavByRole(userRole?: string): SettingsNavItem[] {
    return SETTINGS_NAV.filter(
        (item) => !item.requiredRole || (!!userRole && item.requiredRole.includes(userRole))
    );
}

export const ROLE_COLORS: Record<string, { bg: string; text: string; border: string; dot: string }> = {
    admin:      { bg: 'bg-red-50 dark:bg-red-500/10',    text: 'text-red-700 dark:text-red-400',    border: 'border-red-200 dark:border-red-500/20',    dot: 'bg-red-500 dark:bg-red-400' },
    security:   { bg: 'bg-purple-50 dark:bg-purple-500/10', text: 'text-purple-700 dark:text-purple-400', border: 'border-purple-200 dark:border-purple-500/20', dot: 'bg-purple-500 dark:bg-purple-400' },
    analyst:    { bg: 'bg-blue-50 dark:bg-blue-500/10',   text: 'text-blue-700 dark:text-blue-400',   border: 'border-blue-200 dark:border-blue-500/20',   dot: 'bg-blue-500 dark:bg-blue-400' },
    operations: { bg: 'bg-amber-50 dark:bg-amber-500/10',  text: 'text-amber-700 dark:text-amber-400',  border: 'border-amber-200 dark:border-amber-500/20',  dot: 'bg-amber-500 dark:bg-amber-400' },
    viewer:     { bg: 'bg-gray-100 dark:bg-gray-500/10',  text: 'text-gray-700 dark:text-gray-400',  border: 'border-gray-200 dark:border-gray-500/20',  dot: 'bg-gray-500 dark:bg-gray-400' },
};

export const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
    active:   { bg: 'bg-emerald-50 dark:bg-emerald-500/10', text: 'text-emerald-700 dark:text-emerald-400', dot: 'bg-emerald-500 dark:bg-emerald-400' },
    inactive: { bg: 'bg-gray-100 dark:bg-gray-500/10',   text: 'text-gray-700 dark:text-gray-400',   dot: 'bg-gray-500 dark:bg-gray-400' },
    locked:   { bg: 'bg-red-50 dark:bg-red-500/10',     text: 'text-red-700 dark:text-red-400',     dot: 'bg-red-500 dark:bg-red-400' },
};
