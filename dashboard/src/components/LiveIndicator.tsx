interface LiveIndicatorProps {
    label?: string;
    color?: 'emerald' | 'cyan' | 'red' | 'amber';
    size?: 'sm' | 'md';
}

const COLOR_CONFIG = {
    emerald: { dot: 'bg-emerald-500', pulse: 'bg-emerald-400', text: 'text-emerald-500 dark:text-emerald-400' },
    cyan:    { dot: 'bg-cyan-500',    pulse: 'bg-cyan-400',    text: 'text-cyan-500 dark:text-cyan-400' },
    red:     { dot: 'bg-rose-500',    pulse: 'bg-rose-400',    text: 'text-rose-500 dark:text-rose-400' },
    amber:   { dot: 'bg-amber-500',   pulse: 'bg-amber-400',   text: 'text-amber-500 dark:text-amber-400' },
};

export default function LiveIndicator({ label = 'Live', color = 'emerald', size = 'sm' }: LiveIndicatorProps) {
    const cfg = COLOR_CONFIG[color];
    const dotSize = size === 'sm' ? 'w-1.5 h-1.5' : 'w-2 h-2';
    const textSize = size === 'sm' ? 'text-[10px]' : 'text-xs';

    return (
        <span className={`flex items-center gap-1.5 font-bold tracking-widest uppercase ${cfg.text} ${textSize}`}>
            <span className={`relative flex ${dotSize}`}>
                <span className={`animate-ping absolute inline-flex h-full w-full rounded-full ${cfg.pulse} opacity-60`} />
                <span className={`relative inline-flex rounded-full ${dotSize} ${cfg.dot}`} />
            </span>
            {label}
        </span>
    );
}
