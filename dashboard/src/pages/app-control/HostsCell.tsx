import { useEffect, useMemo, useRef, useState } from 'react';
import { Monitor } from 'lucide-react';

function uniqSorted(xs: string[]): string[] {
    return Array.from(new Set(xs.map((s) => (s ?? '').trim()).filter(Boolean))).sort((a, b) => a.localeCompare(b));
}

export default function HostsCell({ hostnames, fallbackCount }: { hostnames?: string[]; fallbackCount?: number }) {
    const hosts = useMemo(() => uniqSorted(hostnames ?? []), [hostnames]);
    const count = hosts.length || (fallbackCount ?? 0);

    const [open, setOpen] = useState(false);
    const rootRef = useRef<HTMLDivElement | null>(null);

    useEffect(() => {
        if (!open) return;
        const onDoc = (e: MouseEvent) => {
            const el = rootRef.current;
            if (!el) return;
            if (!el.contains(e.target as Node)) setOpen(false);
        };
        document.addEventListener('mousedown', onDoc);
        return () => document.removeEventListener('mousedown', onDoc);
    }, [open]);

    const label = `${count} hosts`;
    const canShow = hosts.length > 0;

    return (
        <div ref={rootRef} className="relative inline-flex items-center justify-center">
            <button
                type="button"
                onClick={() => canShow && setOpen((v) => !v)}
                onMouseEnter={() => canShow && setOpen(true)}
                onMouseLeave={() => setOpen(false)}
                className={`inline-flex items-center justify-center gap-1 rounded-md px-2 py-1 text-xs tabular-nums ${
                    canShow ? 'hover:bg-slate-100 dark:hover:bg-slate-800/60 cursor-pointer' : 'cursor-default'
                }`}
                aria-label={label}
                title={canShow ? 'Hover or click to view hostnames' : label}
            >
                <Monitor className="w-3 h-3 text-slate-400" />
                <span className="text-slate-700 dark:text-slate-300 font-semibold">{count}</span>
                <span className="text-slate-400 font-normal">hosts</span>
            </button>

            {open && canShow && (
                <div
                    className="absolute z-40 top-full mt-2 w-64 rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 shadow-lg overflow-hidden"
                    role="dialog"
                    aria-label="Hostnames"
                >
                    <div className="px-3 py-2 border-b border-slate-100 dark:border-slate-800/60">
                        <p className="text-[11px] font-bold uppercase tracking-wider text-slate-500 dark:text-slate-400">
                            Hosts ({hosts.length})
                        </p>
                    </div>
                    <div className="max-h-48 overflow-auto">
                        <ul className="py-1">
                            {hosts.map((h) => (
                                <li key={h} className="px-3 py-1.5 text-xs font-mono text-slate-700 dark:text-slate-200 hover:bg-slate-50 dark:hover:bg-slate-800/40">
                                    {h}
                                </li>
                            ))}
                        </ul>
                    </div>
                </div>
            )}
        </div>
    );
}

