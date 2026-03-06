import { BrowserRouter, Routes, Route, Link, useLocation, Navigate } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  Shield, AlertTriangle, FileText, BarChart3, Settings as SettingsIcon,
  Moon, Sun, PieChart, LogOut, Monitor, Target, Activity, Menu, X, Key
} from 'lucide-react';
import { useState, Suspense, lazy, useEffect } from 'react';
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
const Threats = lazy(() => import('./pages/Threats'));
const AuditLogs = lazy(() => import('./pages/AuditLogs'));
const EnrollmentTokens = lazy(() => import('./pages/EnrollmentTokens'));

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

function Navigation() {
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

  const navItems = [
    { path: '/', icon: BarChart3, label: 'Dashboard' },
    { path: '/alerts', icon: AlertTriangle, label: 'Alerts' },
    { path: '/endpoints', icon: Monitor, label: 'Endpoints' },
    { path: '/threats', icon: Target, label: 'Threats' },
    { path: '/rules', icon: FileText, label: 'Rules' },
    { path: '/stats', icon: PieChart, label: 'Statistics' },
    ...(authApi.canViewAuditLogs() ? [{ path: '/audit', icon: Activity, label: 'Audit Logs' }] : []),
    { path: '/tokens', icon: Key, label: 'Enrollment Tokens' },
    { path: '/settings', icon: SettingsIcon, label: 'Settings' },
  ];

  const toggleDarkMode = () => {
    setDarkMode(!darkMode);
  };

  const handleLogout = () => {
    authApi.logout();
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
                fixed lg:static inset-y-0 left-0 z-40 w-64 bg-gray-900 text-white min-h-screen p-4 flex flex-col
                transform transition-transform duration-200
                ${isMobileOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'}
            `}>
        {/* Logo */}
        <div className="flex items-center gap-3 mb-8 px-2">
          <Shield className="w-8 h-8 text-primary-400" />
          <span className="text-xl font-bold">EDR Console</span>
        </div>

        {/* User info */}
        {user && (
          <div className="mb-6 px-2 py-3 bg-gray-800 rounded-lg">
            <p className="text-sm font-medium text-white">{user.username}</p>
            <p className="text-xs text-gray-400 capitalize">{user.role}</p>
          </div>
        )}

        {/* Navigation Links */}
        <nav className="space-y-1 flex-1">
          {navItems.map((item) => {
            const isActive = location.pathname === item.path;
            return (
              <Link
                key={item.path}
                to={item.path}
                onClick={() => setIsMobileOpen(false)}
                className={`flex items-center gap-3 px-4 py-3 rounded-lg transition-colors ${isActive
                  ? 'bg-primary-600 text-white'
                  : 'text-gray-300 hover:bg-gray-800 hover:text-white'
                  }`}
              >
                <item.icon className="w-5 h-5" />
                <span>{item.label}</span>
              </Link>
            );
          })}
        </nav>

        {/* Bottom Actions */}
        <div className="space-y-2 border-t border-gray-800 pt-4">
          <button
            onClick={toggleDarkMode}
            className="flex items-center gap-3 px-4 py-3 w-full rounded-lg text-gray-300 hover:bg-gray-800"
          >
            {darkMode ? <Sun className="w-5 h-5" /> : <Moon className="w-5 h-5" />}
            <span>{darkMode ? 'Light Mode' : 'Dark Mode'}</span>
          </button>

          {isAuthenticated() && (
            <button
              onClick={handleLogout}
              className="flex items-center gap-3 px-4 py-3 w-full rounded-lg text-gray-300 hover:bg-gray-800"
            >
              <LogOut className="w-5 h-5" />
              <span>Logout</span>
            </button>
          )}
        </div>
      </aside>
    </>
  );
}

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

