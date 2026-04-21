import { useEffect } from 'react';
import { Outlet, useLocation, useNavigate, useMatch } from 'react-router-dom';
import SettingsLayout from '../components/settings/SettingsLayout';
import { type SettingsTab } from '../components/settings/types';
import { authApi } from '../api/client';

const VALID_TABS = new Set<SettingsTab>(['profile', 'system', 'context', 'reliability', 'users', 'roles']);

export default function Settings() {
    const match = useMatch('/settings/:tab');
    const tab = match?.params.tab;
    const navigate = useNavigate();
    const location = useLocation();
    const currentUser = authApi.getCurrentUser();
    const userRole = currentUser?.role;

    const activeTab: SettingsTab =
        tab && VALID_TABS.has(tab as SettingsTab) ? (tab as SettingsTab) : 'profile';

    useEffect(() => {
        const sp = new URLSearchParams(location.search);
        const t = sp.get('tab');
        if (t && VALID_TABS.has(t as SettingsTab)) {
            navigate(`/settings/${t}`, { replace: true });
        }
    }, [location.search, navigate]);

    useEffect(() => {
        if (tab && !VALID_TABS.has(tab as SettingsTab)) {
            navigate('/settings/profile', { replace: true });
        }
    }, [tab, navigate]);

    return (
        <div className="h-full p-6">
            <SettingsLayout activeTab={activeTab} userRole={userRole}>
                <Outlet />
            </SettingsLayout>
        </div>
    );
}
