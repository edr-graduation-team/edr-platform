import { type HTMLAttributes } from 'react';

interface SkeletonProps extends HTMLAttributes<HTMLDivElement> {
    className?: string;
}

// Base Skeleton component with shimmer animation
export function Skeleton({ className = '', ...props }: SkeletonProps) {
    return (
        <div
            className={`animate-pulse bg-gray-200 dark:bg-gray-700 rounded ${className}`}
            {...props}
        />
    );
}

// Card Skeleton
export function SkeletonCard() {
    return (
        <div className="card space-y-4">
            <div className="flex items-center justify-between">
                <div>
                    <Skeleton className="h-4 w-24 mb-2" />
                    <Skeleton className="h-8 w-16" />
                </div>
                <Skeleton className="h-12 w-12 rounded-lg" />
            </div>
        </div>
    );
}

// Table Row Skeleton
export function SkeletonTableRow({ columns = 5 }: { columns?: number }) {
    return (
        <tr className="border-b border-gray-200 dark:border-gray-700">
            {Array.from({ length: columns }).map((_, i) => (
                <td key={i} className="px-4 py-4">
                    <Skeleton className="h-4 w-full max-w-[120px]" />
                </td>
            ))}
        </tr>
    );
}

// Table Skeleton
export function SkeletonTable({ rows = 5, columns = 5 }: { rows?: number; columns?: number }) {
    return (
        <div className="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
            {/* Header */}
            <div className="bg-gray-50 dark:bg-gray-800 px-4 py-3 flex gap-4">
                {Array.from({ length: columns }).map((_, i) => (
                    <Skeleton key={i} className="h-3 w-20" />
                ))}
            </div>
            {/* Rows */}
            <table className="w-full">
                <tbody>
                    {Array.from({ length: rows }).map((_, i) => (
                        <SkeletonTableRow key={i} columns={columns} />
                    ))}
                </tbody>
            </table>
        </div>
    );
}

// Chart Skeleton
export function SkeletonChart({ height = 300 }: { height?: number }) {
    return (
        <div className="card">
            <Skeleton className="h-5 w-48 mb-4" />
            <div className="flex items-end justify-around gap-2" style={{ height }}>
                {Array.from({ length: 8 }).map((_, i) => (
                    <Skeleton
                        key={i}
                        className="flex-1"
                        style={{ height: `${Math.random() * 60 + 40}%` }}
                    />
                ))}
            </div>
        </div>
    );
}

// KPI Cards Skeleton
export function SkeletonKPICards({ count = 4 }: { count?: number }) {
    return (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            {Array.from({ length: count }).map((_, i) => (
                <SkeletonCard key={i} />
            ))}
        </div>
    );
}

// Alert Detail Skeleton
export function SkeletonAlertDetail() {
    return (
        <div className="space-y-4">
            <div>
                <Skeleton className="h-3 w-16 mb-2" />
                <Skeleton className="h-5 w-48" />
            </div>
            <div>
                <Skeleton className="h-3 w-16 mb-2" />
                <Skeleton className="h-5 w-64" />
            </div>
            <div>
                <Skeleton className="h-3 w-16 mb-2" />
                <Skeleton className="h-6 w-20 rounded-full" />
            </div>
            <div>
                <Skeleton className="h-3 w-16 mb-2" />
                <Skeleton className="h-5 w-32" />
            </div>
            <div className="pt-4 border-t flex gap-2">
                <Skeleton className="h-9 w-28 rounded-md" />
                <Skeleton className="h-9 w-20 rounded-md" />
            </div>
        </div>
    );
}

// Page Loading Skeleton
export function SkeletonPage() {
    return (
        <div className="space-y-6">
            {/* Header */}
            <Skeleton className="h-9 w-48" />

            {/* KPI Cards */}
            <SkeletonKPICards />

            {/* Main Content */}
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                <SkeletonChart />
                <SkeletonChart />
            </div>
        </div>
    );
}

export default Skeleton;
