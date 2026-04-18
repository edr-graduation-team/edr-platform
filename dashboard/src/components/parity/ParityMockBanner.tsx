/**
 * Shown when parity API fell back to local mock data (404 / not implemented / network).
 */
export function ParityMockBanner() {
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
        </div>
    );
}
