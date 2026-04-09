import { BrowserRouter, Routes, Route, Link, useLocation, Navigate } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  Menu, X, Shield, Activity, Settings as SettingsIcon, FileText,
  AlertTriangle, Monitor, Target, PieChart, BarChart3, Zap,
  Key, LogOut, Moon, Sun, TrendingUp, Download
} from 'lucide-react';
import ProtocolLogo from './components/ProtocolLogo';
import { useState, Suspense, lazy, useEffect, useMemo, memo } from 'react';
import { ToastProvider } from './components';
import { authApi } from './api/client';
import './index.css';

// Lazy load pages for better performance
const Dashboard = lazy(() => import('./pages/Dashboard'));
const Alerts = lazy(() => import('./pages/Alerts'));
const Rules = lazy(() => import('./pages/Rules'));
const Stats = lazy(() => import('./pages/Stats'));
const Login = lazy(() => import('./pages/Login'));
const Settings = lazy(() => import('./pages/Settings'));
const Endpoints = lazy(() => import('./pages/Endpoints'));
const EndpointRisk = lazy(() => import('./pages/EndpointRisk'));
const Threats = lazy(() => import('./pages/Threats'));
const AuditLogs = lazy(() => import('./pages/AuditLogs'));
const EnrollmentTokens = lazy(() => import('./pages/EnrollmentTokens'));
const ActionCenter = lazy(() => import('./pages/ActionCenter'));
const AgentDeployment = lazy(() => import('./pages/AgentDeployment'));

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
      staleTime: 30000,
    },
  },
});

// Loading spinner for Suspense
function PageLoader() {
  return (
    <div className="flex items-center justify-center h-64">
      <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600"></div>
    </div>
  );
}

// Check if user is authenticated
function isAuthenticated() {
  return authApi.isAuthenticated();
}

// Protected Route component
function ProtectedRoute({ children, roles }: { children: React.ReactNode; roles?: string[] }) {
  if (!isAuthenticated()) {
    return <Navigate to="/login" replace />;
  }

  if (roles && roles.length > 0 && !authApi.hasRole(roles)) {
    return (
      <div className="card text-center py-12">
        <Shield className="w-12 h-12 text-gray-400 mx-auto mb-4" />
        <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">
          Access Denied
        </h3>
        <p className="text-gray-500">You don't have permission to view this page.</p>
      </div>
    );
  }

  return <>{children}</>;
}

const Navigation = memo(function Navigation() {
  const location = useLocation();
  const [darkMode, setDarkMode] = useState(() => {
    if (typeof window !== 'undefined') {
      const saved = localStorage.getItem('darkMode');
      if (saved !== null) return saved === 'true';
      return window.matchMedia('(prefers-color-scheme: dark)').matches;
    }
    return false;
  });
  const [isMobileOpen, setIsMobileOpen] = useState(false);

  // Apply dark mode on mount and change
  useEffect(() => {
    if (darkMode) {
      document.documentElement.classList.add('dark');
    } else {
      document.documentElement.classList.remove('dark');
    }
    localStorage.setItem('darkMode', String(darkMode));
  }, [darkMode]);

  const user = authApi.getCurrentUser();

  const navGroups = useMemo(() => [
    {
      title: 'ANALYTICS',
      items: [
        { path: '/', icon: BarChart3, label: 'Dashboard' },
        { path: '/stats', icon: PieChart, label: 'Statistics' },
      ]
    },
    {
      title: 'SECURITY',
      items: [
        { path: '/alerts', icon: AlertTriangle, label: 'Alerts' },
        { path: '/endpoints', icon: Monitor, label: 'Endpoints' },
        { path: '/endpoint-risk', icon: TrendingUp, label: 'Risk Intelligence' },
        { path: '/threats', icon: Target, label: 'Threats' },
        { path: '/rules', icon: FileText, label: 'Rules' },
        ...(authApi.canViewAuditLogs() ? [{ path: '/responses', icon: Zap, label: 'Action Center' }] : []),
      ]
    },
    {
      title: 'SYSTEM',
      items: [
        ...(authApi.canViewAuditLogs() ? [{ path: '/audit', icon: Activity, label: 'Audit Logs' }] : []),
        { path: '/tokens', icon: Key, label: 'Enrollment Tokens' },
        ...(authApi.canViewAuditLogs() ? [{ path: '/deploy', icon: Download, label: 'Agent Deployment' }] : []),
        { path: '/settings', icon: SettingsIcon, label: 'Settings' },
      ]
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  ], []); // stable — roles don't change during session

  const toggleDarkMode = () => {
    setDarkMode(!darkMode);
  };

  const handleLogout = async () => {
    try {
      await authApi.logout();
    } catch (error) {
      console.error("Logout error", error);
    }
    // Only navigate to login, client.ts handles local storage clearing
    window.location.href = '/login';
  };

  return (
    <>
      {/* Mobile menu button */}
      <button
        onClick={() => setIsMobileOpen(!isMobileOpen)}
        className="lg:hidden fixed top-4 left-4 z-50 p-2 bg-gray-900 text-white rounded-lg"
      >
        {isMobileOpen ? <X className="w-6 h-6" /> : <Menu className="w-6 h-6" />}
      </button>

      {/* Mobile overlay */}
      {isMobileOpen && (
        <div
          className="lg:hidden fixed inset-0 bg-black/50 z-40"
          onClick={() => setIsMobileOpen(false)}
        />
      )}

      <aside className={`
                fixed lg:sticky top-0 left-0 z-40 w-64 bg-gray-900 border-r border-slate-800 text-white h-screen p-4 flex flex-col overflow-y-auto [&::-webkit-scrollbar]:hidden [-ms-overflow-style:none] [scrollbar-width:none]
                transform transition-transform duration-200
                ${isMobileOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'}
            `}>
        {/* Logo & Branding */}
        <div className="flex items-center gap-3 mb-6 px-2 pb-4 border-b border-slate-800">
          <ProtocolLogo className="w-10 h-10 shrink-0 drop-shadow-[0_0_8px_rgba(34,211,238,0.5)]" idPrefix="app" />
          <div className="flex flex-col">
              <span className="text-cyan-400 text-[8px] font-bold tracking-[0.2em] uppercase leading-none mb-1">Protocol Soft</span>
              <div className="flex items-baseline gap-1">
                  <span className="text-lg font-extrabold text-white tracking-tight uppercase leading-tight">EDR</span>
                  <span className="text-lg font-normal text-white uppercase leading-tight">Platform</span>
              </div>
          </div>
        </div>

        {/* User info */}
        {user && (
          <div className="mb-2 p-3 bg-slate-800/40 border border-slate-700/50 rounded-xl flex items-center gap-3 hover:bg-slate-800/60 transition-colors cursor-default">
            <div className="w-10 h-10 rounded-full bg-gradient-to-br from-cyan-500 to-blue-600 flex items-center justify-center text-white font-bold shadow-[0_0_10px_rgba(34,211,238,0.3)] shrink-0">
              {user.username.slice(0, 2).toUpperCase()}
            </div>
            <div className="overflow-hidden">
              <p className="text-sm font-semibold text-white truncate">{user.username}</p>
              <div className="mt-1 inline-block bg-cyan-500/10 text-cyan-400 px-2 py-0.5 text-[10px] uppercase tracking-wider rounded-full border border-cyan-500/20 font-medium">
                {user.role}
              </div>
            </div>
          </div>
        )}

        {/* Navigation Links */}
        <nav className="space-y-6 flex-1 mt-4">
          {navGroups.map((group) => (
            <div key={group.title}>
              <h3 className="px-4 text-xs font-bold text-slate-500 uppercase tracking-wider mb-2">
                {group.title}
              </h3>
              <div className="space-y-1">
                {group.items.map((item) => {
                  const isActive = location.pathname === item.path;
                  return (
                    <Link
                      key={item.path}
                      to={item.path}
                      onClick={() => setIsMobileOpen(false)}
                      className={`group relative flex items-center gap-3 px-4 py-2.5 transition-all duration-200 ${
                        isActive
                          ? 'bg-slate-800/30 text-white'
                          : 'text-slate-300 hover:bg-slate-800/50 hover:text-white'
                      }`}
                    >
                      {isActive && (
                        <div className="absolute left-0 top-0 bottom-0 w-1 bg-cyan-400 rounded-r shadow-[0_0_10px_rgba(34,211,238,0.5)]" />
                      )}
                      <item.icon className={`w-5 h-5 transition-colors duration-200 ${isActive ? 'text-cyan-400' : 'text-slate-400 group-hover:text-cyan-400'}`} />
                      <span className="text-sm font-medium">{item.label}</span>
                    </Link>
                  );
                })}
              </div>
            </div>
          ))}
        </nav>

        {/* Bottom Actions */}
        <div className="space-y-1 border-t border-slate-800 pt-4 mt-auto">
          <button
            onClick={toggleDarkMode}
            className="flex items-center gap-3 px-4 py-2.5 w-full rounded-lg text-slate-300 hover:bg-slate-800/50 hover:text-white transition-all duration-200 group"
          >
            <div className="text-slate-400 group-hover:text-cyan-400 transition-colors duration-200">
                {darkMode ? <Sun className="w-5 h-5" /> : <Moon className="w-5 h-5" />}
            </div>
            <span className="text-sm font-medium">{darkMode ? 'Light Mode' : 'Dark Mode'}</span>
          </button>

          {isAuthenticated() && (
            <button
              onClick={handleLogout}
              className="group flex items-center gap-3 px-4 py-2.5 w-full rounded-lg text-slate-300 hover:bg-red-500/10 hover:text-red-400 transition-all duration-200"
            >
              <LogOut className="w-5 h-5 text-slate-400 group-hover:text-red-400 transition-colors duration-200" />
              <span className="text-sm font-medium">Logout</span>
            </button>
          )}
        </div>
      </aside>
    </>
  );
});

function Layout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-screen bg-gray-50 dark:bg-gray-900">
      <Navigation />
      <main className="flex-1 overflow-auto lg:ml-0">
        <div className="max-w-7xl mx-auto p-6 pt-16 lg:pt-6">
          <Suspense fallback={<PageLoader />}>
            {children}
          </Suspense>
        </div>
      </main>
    </div>
  );
}

function AppRoutes() {
  return (
    <Routes>
      {/* Login page without layout */}
      <Route path="/login" element={
        <Suspense fallback={<PageLoader />}>
          <Login />
        </Suspense>
      } />

      {/* Main app with layout */}
      <Route path="/*" element={
        <Layout>
          <Routes>
            <Route path="/" element={
              <ProtectedRoute>
                <Dashboard />
              </ProtectedRoute>
            } />
            <Route path="/alerts" element={
              <ProtectedRoute>
                <Alerts />
              </ProtectedRoute>
            } />
            <Route path="/endpoints" element={
              <ProtectedRoute>
                <Endpoints />
              </ProtectedRoute>
            } />
            <Route path="/endpoint-risk" element={
              <ProtectedRoute>
                <EndpointRisk />
              </ProtectedRoute>
            } />
            <Route path="/threats" element={
              <ProtectedRoute>
                <Threats />
              </ProtectedRoute>
            } />
            <Route path="/rules" element={
              <ProtectedRoute>
                <Rules />
              </ProtectedRoute>
            } />
            <Route path="/stats" element={
              <ProtectedRoute>
                <Stats />
              </ProtectedRoute>
            } />
            <Route path="/audit" element={
              <ProtectedRoute roles={['admin', 'security']}>
                <AuditLogs />
              </ProtectedRoute>
            } />
            <Route path="/tokens" element={
              <ProtectedRoute>
                <EnrollmentTokens />
              </ProtectedRoute>
            } />
            <Route path="/responses" element={
              <ProtectedRoute roles={['admin', 'security']}>
                <ActionCenter />
              </ProtectedRoute>
            } />
            <Route path="/deploy" element={
              <ProtectedRoute roles={['admin', 'security']}>
                <AgentDeployment />
              </ProtectedRoute>
            } />
            <Route path="/settings" element={
              <ProtectedRoute>
                <Settings />
              </ProtectedRoute>
            } />
          </Routes>
        </Layout>
      } />
    </Routes>
  );
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <BrowserRouter>
          <AppRoutes />
        </BrowserRouter>
      </ToastProvider>
    </QueryClientProvider>
  );
}

