/**
 * Shown when parity API fell back to local mock data (404 / not implemented / network).
 */
export function ParityMockBanner({ missingApi }: { missingApi?: string | string[] }) {
    const missing = Array.isArray(missingApi) ? missingApi : missingApi ? [missingApi] : [];
    return (
        <div
            className="mb-4 px-3 py-2 rounded-lg border border-amber-200 bg-amber-50 text-amber-900 text-sm dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-200"
            role="status"
        >
            <span className="font-semibold">Demo data</span>
            <span className="text-amber-800/90 dark:text-amber-200/90">
                {' '}
                — API not available or returned an error; showing placeholder data until the backend endpoint is live.
            </span>
            {missing.length > 0 && (
                <div className="mt-1.5 text-xs text-amber-800/90 dark:text-amber-200/90">
                    Missing API:{' '}
                    {missing.map((m, i) => (
                        <code key={m} className="text-[11px] font-mono">
                            {i ? `, ${m}` : m}
                        </code>
                    ))}
                </div>
            )}
        </div>
    );
}
