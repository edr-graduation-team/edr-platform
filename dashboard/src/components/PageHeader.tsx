import React, { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { ChevronRight, Clock } from 'lucide-react';

interface Breadcrumb {
    label: string;
    path?: string;
}

interface PageHeaderProps {
    title: string;
    subtitle?: string;
    breadcrumbs?: Breadcrumb[];
    actions?: React.ReactNode;
    showTimestamp?: boolean;
}

export default function PageHeader({ title, subtitle, breadcrumbs, actions, showTimestamp = true }: PageHeaderProps) {
    const [now, setNow] = useState(new Date());

    useEffect(() => {
        if (!showTimestamp) return;
        const id = setInterval(() => setNow(new Date()), 1000);
        return () => clearInterval(id);
    }, [showTimestamp]);

    return (
        <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-4 mb-6 animate-slide-up-fade">
            <div className="flex-1 min-w-0">
                {/* Breadcrumbs */}
                {breadcrumbs && breadcrumbs.length > 0 && (
                    <nav className="flex items-center gap-1 mb-2" aria-label="Breadcrumb">
                        {breadcrumbs.map((crumb, i) => (
                            <React.Fragment key={i}>
                                {i > 0 && <ChevronRight className="w-3 h-3 text-slate-400 shrink-0" />}
                                {crumb.path ? (
                                    <Link
                                        to={crumb.path}
                                        className="text-xs text-slate-400 hover:text-cyan-500 transition-colors font-medium"
                                    >
                                        {crumb.label}
                                    </Link>
                                ) : (
                                    <span className="text-xs text-slate-500 font-medium">{crumb.label}</span>
                                )}
                            </React.Fragment>
                        ))}
                    </nav>
                )}

                {/* Title */}
                <h1 className="text-2xl font-bold text-slate-900 dark:text-white tracking-tight truncate">
                    {title}
                </h1>

                {/* Subtitle + timestamp row */}
                <div className="flex items-center gap-4 mt-1 flex-wrap">
                    {subtitle && (
                        <p className="text-sm text-slate-400">{subtitle}</p>
                    )}
                    {showTimestamp && (
                        <span className="flex items-center gap-1.5 text-[11px] text-slate-500 font-mono">
                            <Clock className="w-3 h-3" />
                            {now.toLocaleTimeString('en-US', { hour12: false })} UTC{now.toTimeString().slice(9, 15)}
                        </span>
                    )}
                </div>
            </div>

            {/* Action slot */}
            {actions && (
                <div className="flex items-center gap-2 shrink-0 flex-wrap">
                    {actions}
                </div>
            )}
        </div>
    );
}
