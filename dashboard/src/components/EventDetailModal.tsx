import { useQuery } from '@tanstack/react-query';
import { Loader2 } from 'lucide-react';
import { eventsApi, type CmEventDetail } from '../api/client';
import { Modal } from './Modal';

export function formatEventRaw(raw: unknown): string {
    if (raw === null || raw === undefined) return '';
    if (typeof raw === 'string') {
        try {
            return JSON.stringify(JSON.parse(raw), null, 2);
        } catch {
            return raw;
        }
    }
    try {
        return JSON.stringify(raw, null, 2);
    } catch {
        return String(raw);
    }
}

export function EventDetailBody({ ev }: { ev: CmEventDetail }) {
    return (
        <div className="space-y-3 text-sm text-slate-700 dark:text-slate-200">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 text-xs font-mono">
                <div>
                    <span className="text-slate-500 uppercase tracking-wide">id</span>
                    <div className="break-all">{ev.id}</div>
                </div>
                <div>
                    <span className="text-slate-500 uppercase tracking-wide">agent_id</span>
                    <div className="break-all">{ev.agent_id}</div>
                </div>
                <div>
                    <span className="text-slate-500 uppercase tracking-wide">event_type</span>
                    <div>{ev.event_type}</div>
                </div>
                <div>
                    <span className="text-slate-500 uppercase tracking-wide">severity</span>
                    <div>{ev.severity}</div>
                </div>
                <div className="sm:col-span-2">
                    <span className="text-slate-500 uppercase tracking-wide">timestamp</span>
                    <div>{new Date(ev.timestamp).toISOString()}</div>
                </div>
                <div className="sm:col-span-2">
                    <span className="text-slate-500 uppercase tracking-wide">summary</span>
                    <div>{ev.summary}</div>
                </div>
            </div>
            <div>
                <div className="text-xs font-semibold text-slate-500 uppercase mb-1">raw</div>
                <pre className="max-h-[60vh] overflow-auto rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-950 text-slate-100 p-3 text-xs leading-relaxed">
                    {formatEventRaw(ev.raw) || '(empty)'}
                </pre>
            </div>
        </div>
    );
}

type EventDetailModalProps = {
    eventId: string | null;
    onClose: () => void;
    /** Gates the fetch (e.g. requires `alerts:read` like `GET /api/v1/events/:id`). Default true. */
    fetchEnabled?: boolean;
};

/** Loads `GET /api/v1/events/:id` and shows metadata + raw JSON (same guard as Events search). */
export function EventDetailModal({ eventId, onClose, fetchEnabled = true }: EventDetailModalProps) {
    const detailQ = useQuery({
        queryKey: ['event-detail', eventId],
        queryFn: () => eventsApi.get(eventId!),
        enabled: fetchEnabled && !!eventId,
        staleTime: 30_000,
        retry: 1,
    });

    return (
        <Modal isOpen={!!eventId} onClose={onClose} title="Event details" size="xl" closeOnOverlayClick>
            {!fetchEnabled && eventId ? (
                <div className="text-sm text-slate-600 dark:text-slate-400">
                    Your role needs <code className="text-xs">alerts:read</code> to load stored event payloads.
                </div>
            ) : eventId && detailQ.isLoading ? (
                <div className="flex justify-center py-12">
                    <Loader2 className="w-8 h-8 animate-spin text-cyan-500" />
                </div>
            ) : eventId && detailQ.isError ? (
                <div className="text-sm text-rose-700 dark:text-rose-300">
                    Could not load event. Confirm <code className="text-xs">GET /api/v1/events/:id</code> is reachable (same nginx rules as search).
                </div>
            ) : eventId && detailQ.data?.data ? (
                <EventDetailBody ev={detailQ.data.data} />
            ) : null}
        </Modal>
    );
}
