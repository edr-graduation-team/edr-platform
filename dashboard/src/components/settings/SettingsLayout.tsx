import { SETTINGS_NAV } from './types';
import { User, Settings, Users, Shield, ChevronRight, Activity } from 'lucide-react';

const ICONS: Record<string, React.ElementType> = {
    User,
    Settings,
    Users,
    Shield,
    Activity,
};

export default function SettingsLayout({
    activeTab,
    onChangeTab,
    children,
    userRole,
}: {
    activeTab: string;
    onChangeTab: (id: string) => void;
    children: React.ReactNode;
    userRole?: string;
}) {
    // Filter nav items by required role. Operations and Analysts do not see Roles or Users by default if we strictly enforce this.
    const navItems = SETTINGS_NAV.filter(
        item => !item.requiredRole || (userRole && item.requiredRole.includes(userRole))
    );

    const activeItem = navItems.find(item => item.id === activeTab) || navItems[0];

    return (
        <div className="flex-1 flex flex-col h-full bg-gray-50 dark:bg-gray-900 overflow-hidden">
            {/* ── Breadcrumbs & Header ── */}
            <div className="px-8 py-5 border-b border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 shrink-0">
                <div className="flex items-center text-[13px] text-gray-400 dark:text-gray-500 font-medium mb-1.5">
                    <span>Settings</span>
                    <ChevronRight size={14} className="mx-1" />
                    <span className="text-gray-900 dark:text-gray-200">{activeItem?.label}</span>
                </div>
                <h1 className="text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
                    Platform Settings
                </h1>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    Manage your preferences, user access, and system configurations.
                </p>
            </div>

            <div className="flex flex-1 overflow-hidden">
                {/* ── Inner Sidebar ── */}
                <aside className="w-64 border-r border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 shrink-0 overflow-y-auto">
                    <nav className="p-4 space-y-1">
                        {navItems.map(item => {
                            const Icon = ICONS[item.icon];
                            const isActive = activeTab === item.id;
                            return (
                                <button
                                    key={item.id}
                                    onClick={() => onChangeTab(item.id)}
                                    className={`relative w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-all group ${
                                        isActive
                                            ? 'text-blue-700 dark:text-blue-400 bg-blue-50 dark:bg-blue-500/10'
                                            : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800/80 hover:text-gray-900 dark:hover:text-gray-200'
                                    }`}
                                >
                                    {/* Active Indicator Line */}
                                    {isActive && (
                                        <div className="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-6 bg-blue-600 dark:bg-blue-500 rounded-r-full" />
                                    )}
                                    <Icon
                                        size={18}
                                        className={`transition-colors ${
                                            isActive
                                                ? 'text-blue-600 dark:text-blue-500'
                                                : 'text-gray-400 group-hover:text-gray-500 dark:group-hover:text-gray-300'
                                        }`}
                                    />
                                    <div className="flex flex-col items-start pr-1 truncate">
                                        <span className={isActive ? 'font-semibold' : ''}>{item.label}</span>
                                        <span className={`text-[11px] truncate w-full text-left ${
                                            isActive ? 'text-blue-600/70 dark:text-blue-400/70 font-normal' : 'text-gray-400 dark:text-gray-500'
                                        }`}>
                                            {item.description}
                                        </span>
                                    </div>
                                </button>
                            );
                        })}
                    </nav>
                </aside>

                {/* ── Main Content Area ── */}
                <main className="flex-1 overflow-y-auto bg-gray-50 dark:bg-gray-900/50 p-6 md:p-8">
                    <div className="max-w-6xl animate-fade-in">
                        {children}
                    </div>
                </main>
            </div>
        </div>
    );
}
