import { Shield, Server, Search, AlertTriangle } from 'lucide-react';

type IconType = 'shield' | 'server' | 'search' | 'alert';

interface EmptyStateProps {
    icon?: IconType;
    title: string;
    description?: string;
    action?: {
        label: string;
        onClick: () => void;
    };
    className?: string;
}

const ICONS: Record<IconType, typeof Shield> = {
    shield: Shield,
    server: Server,
    search: Search,
    alert:  AlertTriangle,
};

export default function EmptyState({ icon = 'shield', title, description, action, className = '' }: EmptyStateProps) {
    const Icon = ICONS[icon];

    return (
        <div className={`flex flex-col items-center justify-center py-16 px-8 text-center ${className}`}>
            {/* SVG Illustration backdrop */}
            <div className="relative mb-6">
                {/* Concentric rings */}
                <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
                    <div className="w-24 h-24 rounded-full border border-slate-200 dark:border-slate-700 opacity-50" />
                    <div className="absolute w-32 h-32 rounded-full border border-slate-200/60 dark:border-slate-700/60 opacity-30" />
                    <div className="absolute w-40 h-40 rounded-full border border-slate-200/40 dark:border-slate-700/40 opacity-20" />
                </div>

                <div className="relative z-10 w-16 h-16 rounded-2xl bg-slate-100 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 flex items-center justify-center">
                    <Icon className="w-8 h-8 text-slate-400 dark:text-slate-500" />
                </div>
            </div>

            <h3 className="text-base font-bold text-slate-700 dark:text-slate-300 mb-2 tracking-tight">
                {title}
            </h3>
            {description && (
                <p className="text-sm text-slate-400 dark:text-slate-500 max-w-xs leading-relaxed">
                    {description}
                </p>
            )}

            {action && (
                <button
                    onClick={action.onClick}
                    className="mt-6 btn btn-primary text-sm"
                >
                    {action.label}
                </button>
            )}
        </div>
    );
}
