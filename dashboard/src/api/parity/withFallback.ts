import { useQuery, type UseQueryOptions, type UseQueryResult } from '@tanstack/react-query';
import type { AxiosError } from 'axios';

/**
 * Returns true when the API is not available yet (404), not implemented (501),
 * server errors, or network/timeout — frontend should use mock data silently.
 */
export function shouldUseMockForParityError(error: unknown): boolean {
    if (!error || typeof error !== 'object') return true;
    const ax = error as AxiosError & { code?: string };
    const status = ax.response?.status;
    if (status === 404 || status === 501) return true;
    if (status !== undefined && status >= 500) return true;
    if (!ax.response) return true;
    const code = ax.code;
    if (code === 'ECONNABORTED' || code === 'ERR_NETWORK' || code === 'ENOTFOUND') return true;
    return false;
}

export type ParityResult<T> = { data: T; isMock: boolean };

/**
 * Runs an async fetcher; on "not ready" failures returns mock data instead of throwing.
 */
export async function withParityFallback<T>(fetcher: () => Promise<T>, mock: T): Promise<ParityResult<T>> {
    try {
        const data = await fetcher();
        return { data, isMock: false };
    } catch (error) {
        if (shouldUseMockForParityError(error)) {
            return { data: JSON.parse(JSON.stringify(mock)) as T, isMock: true };
        }
        throw error;
    }
}

type ParityQueryOptions<T> = Omit<UseQueryOptions<ParityResult<T>, Error>, 'queryKey' | 'queryFn'>;

/**
 * React Query wrapper: never surfaces 404/network as error state for parity pages;
 * returns { data, isMock } with mock substituted when needed.
 */
export function useParityQuery<T>(
    queryKey: unknown[],
    fetcher: () => Promise<T>,
    mock: T,
    options?: ParityQueryOptions<T>
): UseQueryResult<ParityResult<T>, Error> {
    return useQuery({
        queryKey,
        queryFn: () => withParityFallback(fetcher, mock),
        ...options,
    });
}
