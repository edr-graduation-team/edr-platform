import { Shield, Lock, Users, Smartphone, Clock } from 'lucide-react';

interface FeatureCardProps {
    icon: React.ElementType;
    iconBg: string;
    iconColor: string;
    title: string;
    eta: string;
    description: string;
    features: string[];
}

function FeatureCard({ icon: Icon, iconBg, iconColor, title, eta, description, features }: FeatureCardProps) {
    return (
        <div className="card relative overflow-hidden">
            {/* ETA ribbon */}
            <div className="absolute top-4 right-4 flex items-center gap-1 px-2.5 py-1 bg-slate-100 dark:bg-slate-700 rounded-full">
                <Clock className="w-3 h-3 text-slate-400" />
                <span className="text-xs text-slate-500 dark:text-slate-400 font-medium">{eta}</span>
            </div>

            <div className="flex items-center gap-3 mb-4">
                <div className={`w-10 h-10 rounded-xl ${iconBg} flex items-center justify-center`}>
                    <Icon className={`w-5 h-5 ${iconColor}`} />
                </div>
                <div>
                    <h2 className="text-base font-semibold text-slate-900 dark:text-white">{title}</h2>
                    <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-400">
                        Coming Soon
                    </span>
                </div>
            </div>

            <p className="text-sm text-slate-600 dark:text-slate-400 mb-4 leading-relaxed">{description}</p>

            <div className="space-y-2">
                {features.map((f) => (
                    <div key={f} className="flex items-center gap-2 text-sm text-slate-500 dark:text-slate-400">
                        <div className="w-1.5 h-1.5 rounded-full bg-slate-300 dark:bg-slate-600 flex-shrink-0" />
                        {f}
                    </div>
                ))}
            </div>

            {/* Decorative gradient */}
            <div
                className={`absolute bottom-0 left-0 right-0 h-1 ${iconBg} opacity-50`}
                aria-hidden="true"
            />
        </div>
    );
}

export default function SecurityTab() {
    return (
        <div className="space-y-6 animate-fade-in">
            {/* ── Header Banner ── */}
            <div className="card bg-gradient-to-r from-primary-600 to-primary-800 text-white shadow-lg">
                <div className="flex items-center gap-4">
                    <div className="w-12 h-12 rounded-2xl bg-white/20 flex items-center justify-center flex-shrink-0">
                        <Shield className="w-7 h-7 text-white" />
                    </div>
                    <div>
                        <h2 className="text-lg font-bold">Security Center</h2>
                        <p className="text-sm text-primary-100 leading-relaxed">
                            Advanced security controls are under active development. The features below will be
                            available in upcoming releases.
                        </p>
                    </div>
                </div>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                {/* RBAC */}
                <FeatureCard
                    icon={Users}
                    iconBg="bg-indigo-100 dark:bg-indigo-900/40"
                    iconColor="text-indigo-600 dark:text-indigo-400"
                    title="Role-Based Access Control"
                    eta="Q2 2026"
                    description="Fine-grained permission management. Assign analysts, investigators, and administrators to control who can view, edit, and act on EDR data."
                    features={[
                        'Pre-built analyst, responder, and admin roles',
                        'Custom role creation with granular permissions',
                        'Resource-scoped access (endpoint groups, rules)',
                        'Audit trail for all permission changes',
                    ]}
                />

                {/* MFA */}
                <FeatureCard
                    icon={Smartphone}
                    iconBg="bg-teal-100 dark:bg-teal-900/40"
                    iconColor="text-teal-600 dark:text-teal-400"
                    title="Multi-Factor Authentication"
                    eta="Q2 2026"
                    description="Add a critical second layer of identity verification to protect against credential compromise, even if passwords are exposed."
                    features={[
                        'Time-based OTP (TOTP) — Google Authenticator, Authy',
                        'Hardware security keys (FIDO2 / WebAuthn)',
                        'Recovery code generation and management',
                        'Enforce MFA org-wide via policy',
                    ]}
                />

                {/* Session Management */}
                <FeatureCard
                    icon={Lock}
                    iconBg="bg-rose-100 dark:bg-rose-900/40"
                    iconColor="text-rose-600 dark:text-rose-400"
                    title="Session Management"
                    eta="Q3 2026"
                    description="Full visibility into active sessions across all users. Remotely revoke suspicious sessions and configure session lifetime policies."
                    features={[
                        'View all active sessions with IP and device info',
                        'Force-logout individual or all sessions',
                        'Configurable session timeout and idle limits',
                        'Suspicious login geo-fencing alerts',
                    ]}
                />

                {/* SSO */}
                <FeatureCard
                    icon={Shield}
                    iconBg="bg-violet-100 dark:bg-violet-900/40"
                    iconColor="text-violet-600 dark:text-violet-400"
                    title="Single Sign-On (SSO)"
                    eta="Q3 2026"
                    description="Integrate with your organisation's identity provider for centralised user management and seamless authentication."
                    features={[
                        'SAML 2.0 support (Okta, Azure AD, Google Workspace)',
                        'OIDC / OAuth 2.0 support',
                        'Just-in-time (JIT) user provisioning',
                        'Group-to-role mapping from IdP',
                    ]}
                />
            </div>
        </div>
    );
}
