import { useMemo } from 'react';

interface ThreatMeterProps {
    score: number;       // 0–100
    label?: string;      // auto-derived from score if not provided
    size?: number;       // SVG size in px (default 160)
}

function deriveLabel(score: number): { text: string; color: string } {
    if (score >= 80) return { text: 'CRITICAL', color: '#f43f5e' };
    if (score >= 60) return { text: 'HIGH',     color: '#f97316' };
    if (score >= 40) return { text: 'ELEVATED', color: '#f59e0b' };
    if (score >= 20) return { text: 'GUARDED',  color: '#3b82f6' };
    return             { text: 'LOW',       color: '#10b981' };
}

export default function ThreatMeter({ score, label, size = 160 }: ThreatMeterProps) {
    const clampedScore = Math.max(0, Math.min(100, score));
    const { text: derivedText, color } = useMemo(() => deriveLabel(clampedScore), [clampedScore]);
    const displayLabel = label || derivedText;

    // SVG arc math — 270° arc (135° start → 45° end)
    const cx = size / 2;
    const cy = size / 2;
    const r  = size * 0.38;
    const strokeWidth = size * 0.075;
    const circumference = 2 * Math.PI * r;
    const arcFraction = 0.75; // 270° out of 360°
    const arcLength = circumference * arcFraction;

    // Clamp dash offset
    const dashOffset = arcLength - (arcLength * clampedScore) / 100;

    // Rotation: start at -225deg (bottom-left) and arc 270° clockwise
    const rotation = -225;

    return (
        <div
            className="flex flex-col items-center select-none"
            style={{ width: size, minWidth: size }}
            role="meter"
            aria-valuenow={clampedScore}
            aria-valuemin={0}
            aria-valuemax={100}
            aria-label={`Threat level: ${displayLabel} (${clampedScore})`}
        >
            <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
                <defs>
                    <linearGradient id="threatGradient" x1="0%" y1="0%" x2="100%" y2="0%">
                        <stop offset="0%"   stopColor="#10b981" />
                        <stop offset="33%"  stopColor="#f59e0b" />
                        <stop offset="66%"  stopColor="#f97316" />
                        <stop offset="100%" stopColor="#f43f5e" />
                    </linearGradient>
                    {/* Glow filter */}
                    <filter id="threatGlow">
                        <feGaussianBlur stdDeviation="3" result="blur" />
                        <feComposite in="SourceGraphic" in2="blur" operator="over" />
                    </filter>
                </defs>

                {/* Background track */}
                <circle
                    cx={cx} cy={cy} r={r}
                    fill="none"
                    stroke="rgba(30, 48, 72, 0.5)"
                    strokeWidth={strokeWidth}
                    strokeDasharray={`${arcLength} ${circumference}`}
                    strokeLinecap="round"
                    transform={`rotate(${rotation} ${cx} ${cy})`}
                />

                {/* Progress arc */}
                <circle
                    cx={cx} cy={cy} r={r}
                    fill="none"
                    stroke="url(#threatGradient)"
                    strokeWidth={strokeWidth}
                    strokeDasharray={`${arcLength - dashOffset} ${circumference}`}
                    strokeDashoffset={0}
                    strokeLinecap="round"
                    transform={`rotate(${rotation} ${cx} ${cy})`}
                    filter="url(#threatGlow)"
                    style={{ transition: 'stroke-dasharray 1s cubic-bezier(0.16,1,0.3,1)' }}
                />

                {/* Center score number */}
                <text
                    x={cx} y={cy - size * 0.04}
                    textAnchor="middle"
                    dominantBaseline="middle"
                    fontSize={size * 0.22}
                    fontWeight="800"
                    fontFamily="'JetBrains Mono', monospace"
                    fill={color}
                >
                    {clampedScore}
                </text>

                {/* Label below number */}
                <text
                    x={cx} y={cy + size * 0.18}
                    textAnchor="middle"
                    dominantBaseline="middle"
                    fontSize={size * 0.075}
                    fontWeight="700"
                    fontFamily="'Inter', sans-serif"
                    fill={color}
                    letterSpacing="0.12em"
                >
                    {displayLabel}
                </text>

                {/* Min/Max labels */}
                <text
                    x={cx - r * 0.95} y={cy + r * 0.82}
                    textAnchor="middle"
                    fontSize={size * 0.065}
                    fontFamily="'JetBrains Mono', monospace"
                    fill="rgba(148,163,184,0.7)"
                >0</text>
                <text
                    x={cx + r * 0.95} y={cy + r * 0.82}
                    textAnchor="middle"
                    fontSize={size * 0.065}
                    fontFamily="'JetBrains Mono', monospace"
                    fill="rgba(148,163,184,0.7)"
                >100</text>
            </svg>

            {/* Subtext */}
            <p className="text-[10px] text-slate-500 uppercase tracking-widest font-bold -mt-2">
                Threat Score
            </p>
        </div>
    );
}
