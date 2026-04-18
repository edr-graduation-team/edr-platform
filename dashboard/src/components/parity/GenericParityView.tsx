import { useParityQuery, type ParityResult } from '../../api/parity/withFallback';
import { ParityMockBanner } from './ParityMockBanner';
import { Skeleton } from '../Skeleton';

type GenericParityViewProps<T> = {
    title: string;
    description?: string;
    queryKey: unknown[];
    fetcher: () => Promise<T>;
    mock: T;
};

export function GenericParityView<T>({ title, description, queryKey, fetcher, mock }: GenericParityViewProps<T>) {
    const q = useParityQuery(queryKey, fetcher, mock);

    if (q.isLoading) {
        return (
            <div className="space-y-4">
                <div className="h-8 w-48 rounded bg-slate-200 dark:bg-slate-700 animate-pulse" />
                <Skeleton className="h-64 w-full rounded-xl" />
            </div>
        );
    }

    if (q.isError) {
        return (
            <div className="rounded-xl border border-red-200 bg-red-50 p-4 text-red-800 text-sm dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200">
                Unable to load this section ({q.error?.message || 'error'}). Check permissions or try again.
            </div>
        );
    }

    const payload = q.data as ParityResult<T>;

    return (
        <div className="space-y-4">
            <div>
                <h1 className="text-xl font-bold text-gray-900 dark:text-white">{title}</h1>
                {description && <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">{description}</p>}
            </div>
            {payload.isMock && <ParityMockBanner />}
            <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800/80 overflow-hidden">
                <pre className="text-xs p-4 overflow-auto max-h-[min(70vh,560px)] text-gray-800 dark:text-gray-200 font-mono">
                    {JSON.stringify(payload.data, null, 2)}
                </pre>
            </div>
        </div>
    );
}
