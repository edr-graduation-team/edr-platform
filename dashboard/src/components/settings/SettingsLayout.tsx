export default function SettingsLayout({
    children,
}: {
    activeTab?: string;
    onChangeTab?: (id: string) => void;
    children: React.ReactNode;
    userRole?: string;
}) {
    return (
        <div className="flex-1 flex flex-col h-full bg-gray-50 dark:bg-gray-900 overflow-hidden">
            {/* ── Header ── */}
            <div className="px-8 py-5 border-b border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 shrink-0">
                <h1 className="text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
                    Platform Settings
                </h1>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    Manage your preferences, user access, and system configurations.
                </p>
            </div>

            {/* ── Main Content Area ── */}
            <main className="flex-1 overflow-y-auto bg-gray-50 dark:bg-gray-900/50 p-6 md:p-8">
                <div className="max-w-6xl animate-fade-in">
                    {children}
                </div>
            </main>
        </div>
    );
}
