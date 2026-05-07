// TEMPORARY DEBUG PAGE — internal development only.
//
// Renders a step-by-step trace of how every dashboard KPI is computed
// on the server (raw SQL, intermediate values, formulas, final value).
// Backed by GET /api/v1/debug/stats-trace.
//
// Remove this file (and the route in App.tsx) once internal stat-investigation
// is finished.

import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { ChevronDown, ChevronRight, Database, Clock, AlertTriangle, RefreshCw } from 'lucide-react';
import { connectionApi } from '../api/client';

// ---------------------------------------------------------------------------
// Types matching debugTraceResponse in handlers_debug.go
// ---------------------------------------------------------------------------
interface TraceStep {
    label: string;
    sql?: string;
    inputs?: unknown;
    output?: unknown;
    note?: string;
}

interface TraceSection {
    key: string;
    title: string;
    ui_route: string;
    http_route: string;
    formula: string;
    steps: TraceStep[];
    final_value: unknown;
    duration_ms: number;
    error?: string;
}

interface TraceResponse {
    generated_at: string;
    sections: TraceSection[];
    notes: string[];
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------
export default function DebugStats() {
    const [limit, setLimit] = useState(5);

    const { data, isLoading, isError, error, refetch, isFetching } = useQuery<TraceResponse>({
        queryKey: ['debug-stats-trace', limit],
        queryFn: async () => {
            const res = await connectionApi.get<TraceResponse>(`/api/v1/debug/stats-trace?limit=${limit}`);
            return res.data;
        },
        refetchOnWindowFocus: false,
        staleTime: 0,
    });

    return (
        <div className="space-y-6">
            <div className="flex items-start justify-between">
                <div>
                    <div className="flex items-center gap-2">
                        <Database className="w-6 h-6 text-amber-500" />
                        <h1 className="text-2xl font-bold text-gray-900 dark:text-white">
                            Stats Computation Trace
                        </h1>
                        <span className="px-2 py-0.5 text-xs rounded-full bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300 font-semibold">
                            DEBUG
                        </span>
                    </div>
                    <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                        Internal page — shows the exact SQL, intermediate values, and formulas
                        the server uses to produce dashboard KPIs.
                    </p>
                </div>
                <div className="flex items-center gap-2">
                    <label className="text-sm text-gray-600 dark:text-gray-300">
                        Health sample size:
                        <input
                            type="number"
                            min={1}
                            max={50}
                            value={limit}
                            onChange={(e) => setLimit(Math.max(1, Math.min(50, Number(e.target.value) || 1)))}
                            className="ml-2 w-16 px-2 py-1 border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-sm"
                        />
                    </label>
                    <button
                        onClick={() => refetch()}
                        className="inline-flex items-center gap-1 px-3 py-1.5 text-sm rounded bg-primary-600 hover:bg-primary-700 text-white"
                        disabled={isFetching}
                    >
                        <RefreshCw className={`w-4 h-4 ${isFetching ? 'animate-spin' : ''}`} />
                        {isFetching ? 'Tracing…' : 'Re-trace'}
                    </button>
                </div>
            </div>

            {isLoading && (
                <div className="card p-8 text-center text-gray-500">Computing trace…</div>
            )}

            {isError && (
                <div className="card p-4 border border-red-300 bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 flex items-start gap-2">
                    <AlertTriangle className="w-5 h-5 mt-0.5 shrink-0" />
                    <div>
                        <div className="font-semibold">Failed to load trace</div>
                        <div className="text-sm mt-1">
                            {(error as Error)?.message ||
                                'Make sure you have admin/security role and the backend is running.'}
                        </div>
                    </div>
                </div>
            )}

            {data && (
                <>
                    <div className="card p-4 text-xs text-gray-600 dark:text-gray-400 space-y-1">
                        <div className="flex items-center gap-2">
                            <Clock className="w-3.5 h-3.5" />
                            <span>Trace generated at {new Date(data.generated_at).toLocaleString()}</span>
                        </div>
                        {data.notes?.map((n, i) => (
                            <div key={i} className="ml-5 list-disc">• {n}</div>
                        ))}
                    </div>

                    {data.sections?.map((s) => (
                        <SectionCard key={s.key} section={s} />
                    ))}
                </>
            )}
        </div>
    );
}

// ---------------------------------------------------------------------------
// Section card (one per KPI / stat)
// ---------------------------------------------------------------------------
function SectionCard({ section }: { section: TraceSection }) {
    const [open, setOpen] = useState(true);

    return (
        <div className="card overflow-hidden">
            <button
                onClick={() => setOpen((v) => !v)}
                className="w-full flex items-start justify-between p-4 hover:bg-gray-50 dark:hover:bg-gray-800/50 text-left"
            >
                <div className="flex items-start gap-2">
                    {open ? <ChevronDown className="w-4 h-4 mt-1 text-gray-500" /> : <ChevronRight className="w-4 h-4 mt-1 text-gray-500" />}
                    <div>
                        <div className="font-semibold text-gray-900 dark:text-white">
                            {section.title}
                        </div>
                        <div className="text-xs text-gray-500 mt-0.5">
                            UI: <code className="font-mono">{section.ui_route}</code>{' '}
                            · HTTP: <code className="font-mono">{section.http_route}</code>
                        </div>
                    </div>
                </div>
                <div className="text-xs text-gray-500 shrink-0 ml-4">
                    <Clock className="inline w-3 h-3 mr-1" />
                    {section.duration_ms} ms
                </div>
            </button>

            {open && (
                <div className="border-t border-gray-200 dark:border-gray-700 p-4 space-y-4 bg-gray-50/50 dark:bg-gray-900/30">
                    {section.error && (
                        <div className="text-sm text-red-700 dark:text-red-300 bg-red-50 dark:bg-red-900/20 p-3 rounded border border-red-200">
                            <strong>Error:</strong> {section.error}
                        </div>
                    )}

                    <Block title="Formula / explanation">
                        <pre className="whitespace-pre-wrap text-xs font-mono text-gray-700 dark:text-gray-300">
                            {section.formula}
                        </pre>
                    </Block>

                    <Block title={`Computation steps (${section.steps.length})`}>
                        <ol className="space-y-3">
                            {section.steps.map((st, i) => (
                                <li key={i} className="border-l-2 border-primary-400 pl-3">
                                    <div className="text-sm font-semibold text-gray-800 dark:text-gray-100">
                                        {i + 1}. {st.label}
                                    </div>
                                    {st.sql && (
                                        <pre className="mt-1 text-xs font-mono bg-gray-900 text-gray-100 p-2 rounded overflow-x-auto">
                                            {st.sql}
                                        </pre>
                                    )}
                                    {st.inputs !== undefined && st.inputs !== null && (
                                        <KV label="inputs" value={st.inputs} />
                                    )}
                                    {st.output !== undefined && st.output !== null && (
                                        <KV label="output" value={st.output} />
                                    )}
                                    {st.note && (
                                        <div className="mt-1 text-xs text-gray-500 italic">
                                            note: {st.note}
                                        </div>
                                    )}
                                </li>
                            ))}
                        </ol>
                    </Block>

                    <Block title="Final value (sent to dashboard)">
                        <Json value={section.final_value} />
                    </Block>
                </div>
            )}
        </div>
    );
}

function Block({ title, children }: { title: string; children: React.ReactNode }) {
    return (
        <div>
            <div className="text-xs uppercase tracking-wide text-gray-500 mb-1 font-semibold">
                {title}
            </div>
            <div className="bg-white dark:bg-gray-800 rounded border border-gray-200 dark:border-gray-700 p-3">
                {children}
            </div>
        </div>
    );
}

function KV({ label, value }: { label: string; value: unknown }) {
    return (
        <div className="mt-1">
            <span className="text-xs text-gray-500 mr-1">{label}:</span>
            <Json value={value} inline />
        </div>
    );
}

function Json({ value, inline }: { value: unknown; inline?: boolean }) {
    const str = JSON.stringify(value, null, inline ? 0 : 2);
    return (
        <pre
            className={`text-xs font-mono ${
                inline
                    ? 'inline bg-gray-100 dark:bg-gray-900 px-1.5 py-0.5 rounded text-gray-700 dark:text-gray-200'
                    : 'whitespace-pre-wrap text-gray-700 dark:text-gray-200 overflow-x-auto'
            }`}
        >
            {str}
        </pre>
    );
}
