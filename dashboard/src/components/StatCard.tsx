import React, { useMemo } from 'react';
import { TrendingUp, TrendingDown, Minus } from 'lucide-react';
import { LineChart, Line, ResponsiveContainer } from 'recharts';
import type { LucideIcon } from 'lucide-react';

type Color = 'cyan' | 'red' | 'amber' | 'emerald';

interface Trend {
    value: number;
    direction: 'up' | 'down' | 'flat';
}

interface StatCardProps {
    title: string;
    value: string | number;
    icon: LucideIcon;
    color?: Color;
    trend?: Trend;
    sparkline?: number[];
    subtext?: string;
    onClick?: () => void;
}

const COLOR_MAP: Record<Color, {
    icon: string;
    iconBg: string;
    ring: string;
    glow: string;
    sparkline: string;
    trendUp: string;
    trendDown: string;
}> = {
    cyan: {
        icon: 'text-cyan-400',
        iconBg: 'bg-cyan-500/10 ring-1 ring-cyan-500/20',
        ring: 'hover:ring-1 hover:ring-cyan-500/30',
        glow: 'dark:shadow-cyan-900/20',
        sparkline: '#22d3ee',
        trendUp: 'text-emerald-400',
        trendDown: 'text-red-400',
    },
    red: {
        icon: 'text-rose-400',
        iconBg: 'bg-rose-500/10 ring-1 ring-rose-500/20',
        ring: 'hover:ring-1 hover:ring-rose-500/30',
        glow: 'dark:shadow-rose-900/20',
        sparkline: '#f43f5e',
        trendUp: 'text-rose-400',
        trendDown: 'text-emerald-400',
    },
    amber: {
        icon: 'text-amber-400',
        iconBg: 'bg-amber-500/10 ring-1 ring-amber-500/20',
        ring: 'hover:ring-1 hover:ring-amber-500/30',
        glow: 'dark:shadow-amber-900/20',
        sparkline: '#f59e0b',
        trendUp: 'text-rose-400',
        trendDown: 'text-emerald-400',
    },
    emerald: {
        icon: 'text-emerald-400',
        iconBg: 'bg-emerald-500/10 ring-1 ring-emerald-500/20',
        ring: 'hover:ring-1 hover:ring-emerald-500/30',
        glow: 'dark:shadow-emerald-900/20',
        sparkline: '#10b981',
        trendUp: 'text-emerald-400',
        trendDown: 'text-red-400',
    },
};

export default function StatCard({ title, value, icon: Icon, color = 'cyan', trend, sparkline, subtext, onClick }: StatCardProps) {
    const theme = COLOR_MAP[color];

    const sparkData = useMemo(() => (sparkline || []).map((v, i) => ({ i, v })), [sparkline]);

    const trendIcon = trend ? (
        trend.direction === 'up' ? TrendingUp :
        trend.direction === 'down' ? TrendingDown : Minus
    ) : null;

    const trendColor = trend ? (
        trend.direction === 'up' ? theme.trendUp :
        trend.direction === 'down' ? theme.trendDown : 'text-slate-500'
    ) : 'text-slate-500';

    return (
        <div
            onClick={onClick}
            role={onClick ? 'button' : undefined}
            tabIndex={onClick ? 0 : undefined}
            onKeyDown={onClick ? (e) => e.key === 'Enter' && onClick() : undefined}
            className={`
                card ${theme.ring} ${theme.glow}
                shadow-lg border border-slate-200/80 dark:border-slate-700/60
                bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm
                rounded-xl p-5 flex flex-col gap-4
                transition-all duration-200
                animate-slide-up-fade
                ${onClick ? 'cursor-pointer hover:-translate-y-1 hover:shadow-xl' : ''}
            `}
        >
            <div className="flex items-start justify-between gap-3">
                <div className="flex-1 min-w-0">
                    <p className="text-xs font-semibold uppercase tracking-widest text-slate-400 mb-1.5 truncate">
                        {title}
                    </p>
                    <p className="text-3xl font-bold text-slate-900 dark:text-white tracking-tight font-mono animate-count-up">
                        {value}
                    </p>
                </div>
                <div className={`p-2.5 rounded-xl shrink-0 ${theme.iconBg}`}>
                    <Icon className={`w-5 h-5 ${theme.icon}`} />
                </div>
            </div>

            {/* Sparkline + trend */}
            {(sparkData.length > 0 || trend) && (
                <div className="flex items-end gap-3">
                    {sparkData.length > 0 && (
                        <div className="flex-1 h-10">
                            <ResponsiveContainer width="100%" height="100%">
                                <LineChart data={sparkData}>
                                    <Line
                                        type="monotone"
                                        dataKey="v"
                                        stroke={theme.sparkline}
                                        strokeWidth={2}
                                        dot={false}
                                        isAnimationActive
                                    />
                                </LineChart>
                            </ResponsiveContainer>
                        </div>
                    )}
                    {trend && trendIcon && (
                        <div className={`flex items-center gap-1 text-xs font-bold shrink-0 ${trendColor}`}>
                            {React.createElement(trendIcon, { className: 'w-3.5 h-3.5' })}
                            {trend.direction !== 'flat' && `${Math.abs(trend.value)}%`}
                        </div>
                    )}
                </div>
            )}

            {/* Subtext */}
            {subtext && (
                <p className="text-xs text-slate-500 dark:text-slate-400 -mt-2">{subtext}</p>
            )}
        </div>
    );
}
