import { useState, lazy, Suspense } from 'react';
import { RefreshCw } from 'lucide-react';
import SettingsLayout from '../components/settings/SettingsLayout';
import { type SettingsTab } from '../components/settings/types';
import { authApi } from '../api/client';

// Lazy-load each tab for code-splitting
const UserProfile = lazy(() => import('../components/settings/UserProfile'));
const SystemConfiguration = lazy(() => import('./settings/SystemConfiguration'));
const AccessManagement = lazy(() => import('../components/settings/AccessManagement'));
const RBACMatrix = lazy(() => import('../components/settings/RBACMatrix'));

// ── Skeleton while tab loads ──
function TabFallback() {
    return (
        <div className="flex items-center justify-center py-24 text-[var(--text-tertiary)] text-sm">
            <RefreshCw size={16} className="animate-spin mr-2" /> Loading…
        </div>
    );
}

export default function Settings() {
    const [activeTab, setActiveTab] = useState<SettingsTab>('profile');
    const currentUser = authApi.getCurrentUser();
    const userRole = currentUser?.role;

    const renderTab = () => {
        switch (activeTab) {
            case 'profile':
                return <Suspense fallback={<TabFallback />}><UserProfile /></Suspense>;
            case 'system':
                return <Suspense fallback={<TabFallback />}><SystemConfiguration /></Suspense>;
            case 'users':
                return <Suspense fallback={<TabFallback />}><AccessManagement /></Suspense>;
            case 'roles':
                return <Suspense fallback={<TabFallback />}><RBACMatrix /></Suspense>;
            default:
                return null;
        }
    };

    return (
        <div className="h-full p-6">
            <SettingsLayout 
                activeTab={activeTab} 
                onChangeTab={(id) => setActiveTab(id as SettingsTab)}
                userRole={userRole}
            >
                {renderTab()}
            </SettingsLayout>
        </div>
    );
}
