import {
    memo,
    useCallback,
    useEffect,
    useLayoutEffect,
    useMemo,
    useRef,
    useState,
    type ReactNode,
    type RefObject,
} from 'react';
import { createPortal } from 'react-dom';
import { NavLink, Link, useLocation } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
    ChevronDown,
    LogOut,
    Menu,
    Moon,
    Sun,
    X,
} from 'lucide-react';
import ProtocolLogo from '../components/ProtocolLogo';
import { authApi, statsApi } from '../api/client';
import { filterSettingsNavByRole } from '../components/settings/types';
import {
    DASHBOARD_MAIN_TABS,
    DASHBOARD_MORE_TABS,
    ITSM_TABS,
    MANAGED_SECURITY_TABS,
    MANAGEMENT_TABS,
    SECURITY_MODULE_TABS,
    SOC_CONTEXT_TABS,
    isSocPath,
} from './openEdrNavConfig';

const navLinkBase =
    'text-[13px] font-medium px-3 py-2 rounded-md transition-colors whitespace-nowrap';
const navLinkIdle = 'text-[var(--xc-nav-text)] hover:bg-[var(--xc-nav-hover)]';
const navLinkActive = 'text-[var(--xc-nav-active)] bg-[var(--xc-nav-hover)]';

const ctxTabBase =
    'inline-flex items-center gap-1 px-3 py-2 text-[13px] font-medium rounded-md transition-colors whitespace-nowrap border border-transparent';
const ctxTabIdle = 'text-[var(--xc-nav-text)] hover:bg-[var(--xc-nav-hover)]';
const ctxTabActive = 'text-[var(--xc-nav-active)] border-[var(--xc-nav-border)] bg-[var(--xc-nav-hover)]';

function cx(...parts: (string | false | undefined)[]) {
    return parts.filter(Boolean).join(' ');
}

/**
 * Renders menu in a portal with position:fixed so it is not clipped by
 * ancestor overflow (e.g. nav overflow-x-auto, which hides absolute dropdowns).
 */
function DropdownMenuPortal({
    open,
    anchorRef,
    onClose,
    children,
    minWidth = 240,
}: {
    open: boolean;
    anchorRef: RefObject<HTMLElement | null>;
    onClose: () => void;
    children: ReactNode;
    minWidth?: number;
}) {
    const menuRef = useRef<HTMLDivElement>(null);
    const [pos, setPos] = useState({ top: 0, left: 0 });

    const updatePosition = useCallback(() => {
        const el = anchorRef.current;
        if (!el) return;
        const r = el.getBoundingClientRect();
        let left = r.left;
        const vw = window.innerWidth;
        if (left + minWidth > vw - 8) {
            left = Math.max(8, vw - minWidth - 8);
        }
        setPos({ top: r.bottom + 4, left });
    }, [anchorRef, minWidth]);

    useLayoutEffect(() => {
        if (!open) return;
        updatePosition();
        const onScrollOrResize = () => updatePosition();
        window.addEventListener('scroll', onScrollOrResize, true);
        window.addEventListener('resize', onScrollOrResize);
        return () => {
            window.removeEventListener('scroll', onScrollOrResize, true);
            window.removeEventListener('resize', onScrollOrResize);
        };
    }, [open, updatePosition]);

    useEffect(() => {
        if (!open) return;
        const fn = (e: MouseEvent) => {
            const t = e.target as Node;
            if (anchorRef.current?.contains(t)) return;
            if (menuRef.current?.contains(t)) return;
            onClose();
        };
        const t = window.setTimeout(() => {
            document.addEventListener('mousedown', fn);
        }, 0);
        return () => {
            clearTimeout(t);
            document.removeEventListener('mousedown', fn);
        };
    }, [open, onClose, anchorRef]);

    if (!open) return null;

    return createPortal(
        <div
            ref={menuRef}
            role="menu"
            className="fixed rounded-md border py-1 shadow-xl"
            style={{
                top: pos.top,
                left: pos.left,
                minWidth,
                zIndex: 10000,
                background: 'var(--xc-nav-bg)',
                borderColor: 'var(--xc-nav-border)',
            }}
        >
            {children}
        </div>,
        document.body,
    );
}

function NavDropdown({
    label,
    id,
    active,
    openId,
    setOpenId,
    children,
}: {
    label: string;
    id: string;
    active: boolean;
    openId: string | null;
    setOpenId: (v: string | null) => void;
    children: ReactNode;
}) {
    const buttonRef = useRef<HTMLButtonElement>(null);
    const open = openId === id;
    const close = useCallback(() => setOpenId(null), [setOpenId]);

    return (
        <div className="relative inline-flex shrink-0">
            <button
                ref={buttonRef}
                type="button"
                id={`nav-dd-${id}`}
                aria-expanded={open}
                onClick={() => setOpenId(open ? null : id)}
                className={cx(
                    navLinkBase,
                    'inline-flex items-center gap-1',
                    active || open ? navLinkActive : navLinkIdle,
                )}
            >
                {label}
                <ChevronDown className={`w-3.5 h-3.5 shrink-0 transition-transform ${open ? 'rotate-180' : ''}`} />
            </button>
            <DropdownMenuPortal open={open} anchorRef={buttonRef} onClose={close}>
                {children}
            </DropdownMenuPortal>
        </div>
    );
}

function DropdownLink({
    to,
    onNavigate,
    children,
}: {
    to: string;
    onNavigate: () => void;
    children: ReactNode;
}) {
    return (
        <NavLink
            to={to}
            role="menuitem"
            onClick={onNavigate}
            className={({ isActive }) =>
                cx(
                    'block px-3 py-2 text-[13px] transition-colors',
                    isActive ? 'text-[var(--xc-nav-active)] bg-[var(--xc-nav-hover)]' : 'text-[var(--xc-nav-text)] hover:bg-[var(--xc-nav-hover)]',
                )
            }
        >
            {children}
        </NavLink>
    );
}

function EngineHealthChip() {
    const { data: perf } = useQuery({
        queryKey: ['shellPerfStats'],
        queryFn: statsApi.performance,
        refetchInterval: 10000,
        enabled: authApi.isAuthenticated(),
    });

    const eps = perf?.events_per_second ?? null;
    const errRate = perf?.error_rate ?? 0;

    const status =
        eps === null ? 'unknown' : errRate > 0.05 ? 'degraded' : eps > 0 ? 'online' : 'idle';

    const cfg = {
        online: {
            dot: 'bg-emerald-500 shadow-[0_0_6px_rgba(16,185,129,0.6)]',
            label: 'Engine',
            text: 'text-emerald-300',
            animate: 'animate-pulse',
        },
        idle: {
            dot: 'bg-amber-400',
            label: 'Engine',
            text: 'text-amber-200',
            animate: '',
        },
        degraded: {
            dot: 'bg-rose-500',
            label: 'Engine',
            text: 'text-rose-200',
            animate: 'animate-pulse',
        },
        unknown: { dot: 'bg-slate-500', label: 'Engine', text: 'text-slate-300', animate: '' },
    }[status];

    return (
        <div
            className="hidden xl:flex items-center gap-2 px-2 py-1 rounded-md border text-[11px]"
            style={{ borderColor: 'var(--xc-nav-border)', color: 'var(--xc-nav-text)' }}
        >
            <span className={`w-1.5 h-1.5 rounded-full shrink-0 ${cfg.dot} ${cfg.animate}`} />
            <span className={cfg.text}>{cfg.label}</span>
            {eps !== null && <span className="font-mono opacity-80">{eps.toFixed(1)} ev/s</span>}
        </div>
    );
}

function filterSocTabs() {
    return SOC_CONTEXT_TABS.filter((t) => {
        if (t.to === '/' || t.to === '/stats') return true;
        if (t.to === '/alerts') return authApi.canViewAlerts();
        if (t.to === '/endpoints') return authApi.canViewEndpoints();
        if (t.to === '/endpoint-risk' || t.to === '/threats') return authApi.canViewAlerts();
        if (t.to === '/rules') return authApi.canViewRules();
        if (t.to === '/responses') return authApi.canViewResponses();
        return true;
    });
}

export const OpenEdrAppShell = memo(function OpenEdrAppShell({ children }: { children: ReactNode }) {
    const location = useLocation();
    const pathname = location.pathname;
    const [openId, setOpenId] = useState<string | null>(null);
    // Dashboards tabs are rendered directly (no “More” menu).

    const [darkMode, setDarkMode] = useState(() => {
        if (typeof window !== 'undefined') {
            const saved = localStorage.getItem('darkMode');
            if (saved !== null) return saved === 'true';
            return window.matchMedia('(prefers-color-scheme: dark)').matches;
        }
        return false;
    });
    const [mobileOpen, setMobileOpen] = useState(false);

    useEffect(() => {
        if (darkMode) document.documentElement.classList.add('dark');
        else document.documentElement.classList.remove('dark');
        localStorage.setItem('darkMode', String(darkMode));
    }, [darkMode]);

    useEffect(() => {
        setOpenId(null);
        setMobileOpen(false);
    }, [pathname]);

    const user = authApi.getCurrentUser();

    const { data: sidebarAlertStats } = useQuery({
        queryKey: ['topNavAlertStats'],
        queryFn: statsApi.alerts,
        refetchInterval: 15000,
        enabled: authApi.isAuthenticated() && authApi.canViewAlerts(),
    });
    const openAlertCount = (sidebarAlertStats?.by_status?.['open'] || 0) as number;

    const securityTopActive = useMemo(() => {
        if (pathname.startsWith('/security')) return true;
        return isSocPath(pathname);
    }, [pathname]);

    const managementTopActive = useMemo(() => pathname.startsWith('/management'), [pathname]);

    const itsmTopActive = useMemo(() => pathname.startsWith('/itsm'), [pathname]);

    const managedTopActive = useMemo(() => pathname.startsWith('/managed-security'), [pathname]);

    const dashboardsTopActive = useMemo(() => pathname.startsWith('/dashboards'), [pathname]);

    const systemTopActive = useMemo(() => {
        return ['/audit', '/tokens', '/deploy'].some((p) => pathname === p || pathname.startsWith(`${p}/`));
    }, [pathname]);

    const settingsTopActive = useMemo(() => pathname.startsWith('/settings'), [pathname]);

    const contextRow = useMemo(() => {
        if (pathname.startsWith('/dashboards')) {
            return { title: 'Dashboards', variant: 'dashboards' as const };
        }
        if (pathname.startsWith('/security')) {
            return { title: 'Security', variant: 'security-modules' as const };
        }
        if (isSocPath(pathname)) {
            return { title: 'SOC', variant: 'soc' as const };
        }
        if (pathname.startsWith('/managed-security')) {
            return { title: 'Managed Security', variant: 'managed' as const };
        }
        if (pathname.startsWith('/itsm')) {
            return { title: 'ITSM', variant: 'itsm' as const };
        }
        if (pathname.startsWith('/management')) {
            return { title: 'Management', variant: 'management' as const };
        }
        if (pathname.startsWith('/settings')) {
            return { title: 'Settings', variant: 'settings' as const };
        }
        return null;
    }, [pathname]);

    const handleLogout = async () => {
        try {
            await authApi.logout();
        } catch (e) {
            console.error('Logout error', e);
        }
        window.location.href = '/login';
    };

    const mobileNavClass =
        'fixed inset-y-0 left-0 z-[70] w-72 max-w-[85vw] border-r border-[var(--xc-nav-border)] shadow-2xl flex flex-col lg:hidden transition-transform duration-200';

    return (
        <div className="min-h-screen flex flex-col bg-gray-50 dark:bg-gray-900">
            <header
                className="sticky top-0 z-50 flex flex-col border-b shadow-sm"
                style={{ borderColor: 'var(--xc-nav-border)', background: 'var(--xc-nav-bg)' }}
            >
                <div className="flex items-center gap-2 px-2 sm:px-3 h-[46px] min-h-[46px]">
                    <button
                        type="button"
                        className="lg:hidden p-2 rounded-md text-[var(--xc-nav-text)] hover:bg-[var(--xc-nav-hover)]"
                        aria-label="Open menu"
                        onClick={() => setMobileOpen(true)}
                    >
                        <Menu className="w-5 h-5" />
                    </button>

                    <Link to="/" className="flex items-center gap-2 shrink-0 mr-2 sm:mr-4" onClick={() => setMobileOpen(false)}>
                        <ProtocolLogo className="w-9 h-9 shrink-0 drop-shadow-[0_0_8px_rgba(34,211,238,0.45)]" idPrefix="shell" />
                        <div className="hidden sm:flex flex-col leading-tight">
                            <span className="text-[8px] font-bold tracking-[0.18em] uppercase" style={{ color: 'var(--xc-brand-original)' }}>
                                Protocol Soft
                            </span>
                            <span className="text-sm font-extrabold tracking-tight text-white uppercase">EDR Platform</span>
                        </div>
                    </Link>

                    <Link
                        to="/"
                        className="hidden md:inline text-[12px] px-2 py-1 rounded text-[var(--xc-nav-text)] hover:bg-[var(--xc-nav-hover)]"
                    >
                        Essential Platform
                    </Link>

                    <nav className="hidden lg:flex items-center gap-0.5 flex-1 min-w-0 overflow-x-auto [&::-webkit-scrollbar]:hidden">
                        <NavLink
                            to="/dashboards"
                            className={({ isActive }) => cx(navLinkBase, isActive || dashboardsTopActive ? navLinkActive : navLinkIdle)}
                        >
                            Dashboards
                        </NavLink>

                        <NavDropdown id="soc" label="SOC" active={isSocPath(pathname)} openId={openId} setOpenId={setOpenId}>
                            {authApi.canViewAlerts() && (
                                <DropdownLink to="/alerts" onNavigate={() => setOpenId(null)}>
                                    Alerts (Triage)
                                    {openAlertCount > 0 && (
                                        <span className="ml-2 text-[10px] px-1.5 py-0.5 rounded-full bg-white/10">
                                            {openAlertCount > 99 ? '99+' : openAlertCount}
                                        </span>
                                    )}
                                </DropdownLink>
                            )}
                            <DropdownLink to="/events" onNavigate={() => setOpenId(null)}>
                                Telemetry Search
                            </DropdownLink>
                            {authApi.canViewEndpoints() && (
                                <DropdownLink to="/endpoints" onNavigate={() => setOpenId(null)}>
                                    Devices
                                </DropdownLink>
                            )}
                            {authApi.canViewAlerts() && (
                                <DropdownLink to="/endpoint-risk" onNavigate={() => setOpenId(null)}>
                                    Endpoint Risk
                                </DropdownLink>
                            )}
                            {authApi.canViewAlerts() && (
                                <DropdownLink to="/threats" onNavigate={() => setOpenId(null)}>
                                    ATT&CK Analytics
                                </DropdownLink>
                            )}
                            {authApi.canViewRules() && (
                                <DropdownLink to="/rules" onNavigate={() => setOpenId(null)}>
                                    Detection Rules
                                </DropdownLink>
                            )}
                            {authApi.canViewResponses() && (
                                <DropdownLink to="/responses" onNavigate={() => setOpenId(null)}>
                                    Command Center
                                </DropdownLink>
                            )}
                            <DropdownLink to="/stats" onNavigate={() => setOpenId(null)}>
                                Reports &amp; Statistics
                            </DropdownLink>
                        </NavDropdown>

                        <NavDropdown id="security" label="Security" active={securityTopActive} openId={openId} setOpenId={setOpenId}>
                            <DropdownLink to="/security/endpoint-zero-trust" onNavigate={() => setOpenId(null)}>
                                Endpoint Zero Trust
                            </DropdownLink>
                            <DropdownLink to="/security/siem-x" onNavigate={() => setOpenId(null)}>
                                SIEM Connectors
                            </DropdownLink>
                        </NavDropdown>

                        <NavLink
                            to="/managed-security/overview"
                            className={({ isActive }) => cx(navLinkBase, isActive || managedTopActive ? navLinkActive : navLinkIdle)}
                        >
                            Managed Detection &amp; Response
                        </NavLink>

                        <NavDropdown id="workflows" label="Workflows" active={itsmTopActive} openId={openId} setOpenId={setOpenId}>
                            {ITSM_TABS.map((t) => (
                                <DropdownLink key={t.to} to={t.to} onNavigate={() => setOpenId(null)}>
                                    {t.label}
                                </DropdownLink>
                            ))}
                        </NavDropdown>

                        <NavDropdown
                            id="management"
                            label="Management"
                            active={managementTopActive}
                            openId={openId}
                            setOpenId={setOpenId}
                        >
                            {MANAGEMENT_TABS.map((t) => (
                                <DropdownLink key={t.to} to={t.to} onNavigate={() => setOpenId(null)}>
                                    {t.label}
                                </DropdownLink>
                            ))}
                        </NavDropdown>

                        {(authApi.canViewAuditLogs() || authApi.canViewTokens() || authApi.canViewAgentDeploy()) && (
                            <NavDropdown id="system" label="System" active={systemTopActive} openId={openId} setOpenId={setOpenId}>
                                {authApi.canViewAuditLogs() && (
                                    <DropdownLink to="/audit" onNavigate={() => setOpenId(null)}>
                                        Audit Logs
                                    </DropdownLink>
                                )}
                                {authApi.canViewTokens() && (
                                    <DropdownLink to="/tokens" onNavigate={() => setOpenId(null)}>
                                        Enrollment Tokens
                                    </DropdownLink>
                                )}
                                {authApi.canViewAgentDeploy() && (
                                    <DropdownLink to="/deploy" onNavigate={() => setOpenId(null)}>
                                        Agent Deployment
                                    </DropdownLink>
                                )}
                            </NavDropdown>
                        )}

                        <NavDropdown
                            id="settings"
                            label="Settings"
                            active={settingsTopActive}
                            openId={openId}
                            setOpenId={setOpenId}
                        >
                            {filterSettingsNavByRole(user?.role).map((item) => (
                                <DropdownLink key={item.id} to={`/settings/${item.id}`} onNavigate={() => setOpenId(null)}>
                                    {item.label}
                                </DropdownLink>
                            ))}
                        </NavDropdown>
                    </nav>

                    <div className="flex items-center gap-1 sm:gap-2 ml-auto shrink-0">
                        <EngineHealthChip />
                        {user && (
                            <span className="hidden lg:inline max-w-[160px] truncate text-xs text-white/90" title={user.username}>
                                {user.username}
                            </span>
                        )}
                        <button
                            type="button"
                            onClick={() => setDarkMode((d) => !d)}
                            className="p-2 rounded-md text-[var(--xc-nav-text)] hover:bg-[var(--xc-nav-hover)]"
                            aria-label={darkMode ? 'Light mode' : 'Dark mode'}
                        >
                            {darkMode ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
                        </button>
                        {authApi.isAuthenticated() && (
                            <button
                                type="button"
                                onClick={handleLogout}
                                className="p-2 rounded-md text-[var(--xc-nav-text)] hover:bg-red-500/15 hover:text-red-300"
                                aria-label="Logout"
                            >
                                <LogOut className="w-4 h-4" />
                            </button>
                        )}
                    </div>
                </div>

                {contextRow && (
                    <div
                        className="flex items-stretch min-h-[52px] border-t px-2 sm:px-3 gap-2 overflow-x-auto [&::-webkit-scrollbar]:h-1.5"
                        style={{
                            borderColor: 'var(--xc-nav-border)',
                            background: 'var(--xc-nav-bg)',
                        }}
                    >
                        <span className="hidden sm:flex items-center shrink-0 text-xs font-bold uppercase tracking-wide text-white/80 pr-2 border-r my-2 pl-1"
                            style={{ borderColor: 'var(--xc-nav-border)' }}>
                            {contextRow.title}
                        </span>
                        <div className="flex items-center gap-1 py-1.5 flex-1 min-w-0">
                            {contextRow.variant === 'dashboards' && (
                                <>
                                    {[...DASHBOARD_MAIN_TABS, ...DASHBOARD_MORE_TABS].map((t) => (
                                        <NavLink
                                            key={t.to}
                                            to={t.to}
                                            end={t.end}
                                            className={({ isActive }) =>
                                                cx(ctxTabBase, isActive ? ctxTabActive : ctxTabIdle)
                                            }
                                        >
                                            {t.label}
                                        </NavLink>
                                    ))}
                                </>
                            )}
                            {contextRow.variant === 'security-modules' &&
                                SECURITY_MODULE_TABS.map((t) => (
                                    <NavLink
                                        key={t.to}
                                        to={t.to}
                                        className={({ isActive }) => cx(ctxTabBase, isActive ? ctxTabActive : ctxTabIdle)}
                                    >
                                        {t.label}
                                    </NavLink>
                                ))}
                            {contextRow.variant === 'soc' &&
                                filterSocTabs().map((t) => (
                                    <NavLink
                                        key={t.to}
                                        to={t.to}
                                        end={t.to === '/'}
                                        className={({ isActive }) => cx(ctxTabBase, isActive ? ctxTabActive : ctxTabIdle)}
                                    >
                                        {t.label}
                                    </NavLink>
                                ))}
                            {contextRow.variant === 'managed' &&
                                MANAGED_SECURITY_TABS.map((t) => (
                                    <NavLink
                                        key={t.to}
                                        to={t.to}
                                        className={({ isActive }) => cx(ctxTabBase, isActive ? ctxTabActive : ctxTabIdle)}
                                    >
                                        {t.label}
                                    </NavLink>
                                ))}
                            {contextRow.variant === 'itsm' &&
                                ITSM_TABS.map((t) => (
                                    <NavLink
                                        key={t.to}
                                        to={t.to}
                                        className={({ isActive }) => cx(ctxTabBase, isActive ? ctxTabActive : ctxTabIdle)}
                                    >
                                        {t.label}
                                    </NavLink>
                                ))}
                            {contextRow.variant === 'management' &&
                                MANAGEMENT_TABS.map((t) => (
                                    <NavLink
                                        key={t.to}
                                        to={t.to}
                                        className={({ isActive }) => cx(ctxTabBase, isActive ? ctxTabActive : ctxTabIdle)}
                                    >
                                        {t.label}
                                    </NavLink>
                                ))}
                            {contextRow.variant === 'settings' &&
                                filterSettingsNavByRole(user?.role).map((item) => (
                                    <NavLink
                                        key={item.id}
                                        to={`/settings/${item.id}`}
                                        className={({ isActive }) => cx(ctxTabBase, isActive ? ctxTabActive : ctxTabIdle)}
                                    >
                                        {item.label}
                                    </NavLink>
                                ))}
                        </div>
                    </div>
                )}
            </header>

            {mobileOpen && (
                <>
                    <button
                        type="button"
                        className="fixed inset-0 z-[65] bg-black/50 lg:hidden"
                        aria-label="Close menu"
                        onClick={() => setMobileOpen(false)}
                    />
                    <aside
                        className={cx(mobileNavClass, mobileOpen ? 'translate-x-0' : '-translate-x-full')}
                        style={{
                            background: 'var(--xc-nav-bg)',
                            borderColor: 'var(--xc-nav-border)',
                        }}
                    >
                        <div className="flex items-center justify-between px-3 py-3 border-b" style={{ borderColor: 'var(--xc-nav-border)' }}>
                            <span className="text-sm font-semibold text-white">Menu</span>
                            <button
                                type="button"
                                className="p-2 rounded-md text-[var(--xc-nav-text)] hover:bg-[var(--xc-nav-hover)]"
                                onClick={() => setMobileOpen(false)}
                                aria-label="Close"
                            >
                                <X className="w-5 h-5" />
                            </button>
                        </div>
                        <div className="flex-1 overflow-y-auto py-2 px-2 space-y-1 text-[var(--xc-nav-text)]">
                            <NavLink to="/dashboards" onClick={() => setMobileOpen(false)} className="block px-3 py-2 rounded-md hover:bg-[var(--xc-nav-hover)]">
                                Dashboards
                            </NavLink>
                            <NavLink to="/security/endpoint-zero-trust" onClick={() => setMobileOpen(false)} className="block px-3 py-2 rounded-md hover:bg-[var(--xc-nav-hover)]">
                                Security
                            </NavLink>
                            <NavLink to="/managed-security/overview" onClick={() => setMobileOpen(false)} className="block px-3 py-2 rounded-md hover:bg-[var(--xc-nav-hover)]">
                                Managed Security
                            </NavLink>
                            <NavLink to="/itsm/tickets" onClick={() => setMobileOpen(false)} className="block px-3 py-2 rounded-md hover:bg-[var(--xc-nav-hover)]">
                                ITSM
                            </NavLink>
                            <NavLink to="/management/devices" onClick={() => setMobileOpen(false)} className="block px-3 py-2 rounded-md hover:bg-[var(--xc-nav-hover)]">
                                Management
                            </NavLink>
                            <div className="px-3 pt-2 pb-0.5 text-[10px] font-bold uppercase tracking-wider opacity-60">Settings</div>
                            {filterSettingsNavByRole(user?.role).map((item) => (
                                <NavLink
                                    key={item.id}
                                    to={`/settings/${item.id}`}
                                    onClick={() => setMobileOpen(false)}
                                    className="block px-3 py-2 rounded-md hover:bg-[var(--xc-nav-hover)]"
                                >
                                    {item.label}
                                </NavLink>
                            ))}
                            <div className="border-t my-2 pt-2" style={{ borderColor: 'var(--xc-nav-border)' }} />
                            {filterSocTabs().map((t) => (
                                <NavLink key={t.to} to={t.to} onClick={() => setMobileOpen(false)} className="block px-3 py-2 rounded-md hover:bg-[var(--xc-nav-hover)]">
                                    {t.label}
                                </NavLink>
                            ))}
                        </div>
                    </aside>
                </>
            )}

            <main className="flex-1 overflow-x-hidden overflow-y-auto">
                <div className="w-full px-4 sm:px-6 lg:px-8 py-5">{children}</div>
            </main>
        </div>
    );
});
