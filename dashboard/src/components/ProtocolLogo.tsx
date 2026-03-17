import { memo } from 'react';

/**
 * ProtocolLogo — performance-optimized version.
 *
 * Changes from original:
 * - Removed both `<feGaussianBlur>` SVG filter elements (stdDeviation 3 and 5).
 *   SVG filters repaint the surroundings every animation frame and force the
 *   browser to create stacked GPU compositing layers that slow down the entire
 *   page — not just the logo — because the filter region bleeds into the
 *   parent stacking context.
 * - Replaced SVG filters with CSS `drop-shadow()` which is GPU-composited as
 *   a single pass without triggering layout/paint invalidation on siblings.
 * - Removed the animated data-stream path (stroke-dashoffset) — lowest visual
 *   value, highest animation overhead (triggers path re-rasterization per frame).
 * - Retained the 3 most visually-important animations (2 orbital rings + security
 *   triangle rotation) using `animateTransform`, which only affects transform and
 *   does not trigger paint.
 * - Wrapped in React.memo so the SVG is never re-created when the Navigation
 *   component re-renders on route changes.
 */
const ProtocolLogo = memo(function ProtocolLogo({
    className = '',
    idPrefix = 'logo',
}: {
    className?: string;
    idPrefix?: string;
}) {
    return (
        <svg
            className={className}
            viewBox="0 0 100 100"
            fill="none"
            xmlns="http://www.w3.org/2000/svg"
            style={{ willChange: 'transform' }}
        >
            <defs>
                <linearGradient id={`${idPrefix}-grad`} x1="0%" y1="0%" x2="100%" y2="100%">
                    <stop offset="0%" stopColor="#1E3A8A" />
                    <stop offset="100%" stopColor="#06B6D4" />
                </linearGradient>
            </defs>

            {/* Two orbital rings (animateTransform only — no paint) */}
            <g opacity="0.4">
                <ellipse cx="50" cy="50" rx="44" ry="16" fill="none" stroke="#06B6D4" strokeWidth="0.8" strokeDasharray="4 8">
                    <animateTransform attributeName="transform" type="rotate" from="0 50 50" to="360 50 50" dur="20s" repeatCount="indefinite" />
                </ellipse>
                <ellipse cx="50" cy="50" rx="44" ry="16" fill="none" stroke={`url(#${idPrefix}-grad)`} strokeWidth="1.2" strokeDasharray="6 12">
                    <animateTransform attributeName="transform" type="rotate" from="120 50 50" to="480 50 50" dur="25s" repeatCount="indefinite" />
                </ellipse>
            </g>

            {/* Hexagonal frame — CSS drop-shadow (single GPU pass, no compositing bleed) */}
            <g style={{ filter: 'drop-shadow(0 0 4px #06B6D4)' }}>
                <polygon
                    points="50,15 80,32 80,68 50,85 20,68 20,32"
                    fill="rgba(30, 58, 138, 0.15)"
                    stroke={`url(#${idPrefix}-grad)`}
                    strokeWidth="2.5"
                    strokeLinejoin="round"
                />
                <path d="M50 50 L50 15 M50 50 L80 68 M50 50 L20 68" stroke="#06B6D4" strokeWidth="1.5" strokeLinecap="round" opacity="0.8" />
                {/* Inner rotating security triangle */}
                <polygon points="50,30 65,58 35,58" fill="none" stroke="#ffffff" strokeWidth="1" strokeDasharray="2 3" opacity="0.6">
                    <animateTransform attributeName="transform" type="rotate" from="360 50 50" to="0 50 50" dur="8s" repeatCount="indefinite" />
                </polygon>
            </g>

            {/* Network nodes at hex vertices */}
            <g style={{ filter: 'drop-shadow(0 0 3px #00ffff)' }}>
                <circle cx="50" cy="15" r="2.5" fill="#00ffff" />
                <circle cx="80" cy="32" r="2.5" fill="#ffffff" />
                <circle cx="80" cy="68" r="2.5" fill="#00ffff" />
                <circle cx="50" cy="85" r="2.5" fill="#ffffff" />
                <circle cx="20" cy="68" r="2.5" fill="#00ffff" />
                <circle cx="20" cy="32" r="2.5" fill="#ffffff" />
            </g>

            {/* Central super-node */}
            <circle cx="50" cy="50" r="6" fill={`url(#${idPrefix}-grad)`} style={{ filter: 'drop-shadow(0 0 5px #06B6D4)' }}>
                <animate attributeName="r" values="6; 8; 6" dur="2s" repeatCount="indefinite" />
            </circle>
            <circle cx="50" cy="50" r="3" fill="#ffffff">
                <animate attributeName="opacity" values="0.4; 1; 0.4" dur="1s" repeatCount="indefinite" />
            </circle>
        </svg>
    );
});

export default ProtocolLogo;
