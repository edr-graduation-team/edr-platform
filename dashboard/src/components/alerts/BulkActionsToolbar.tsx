import React from 'react';
import { X } from 'lucide-react';
import { authApi } from '../../api/client';

interface BulkActionsToolbarProps {
    selectedCount: number;
    onAction: (action: string) => void;
    onClear: () => void;
}

export const BulkActionsToolbar = React.memo(function BulkActionsToolbar({
    selectedCount,
    onAction,
    onClear
}: BulkActionsToolbarProps) {
    if (selectedCount === 0) return null;
    if (!authApi.canWriteAlerts()) return null;

    return (
        <div className="flex items-center gap-4 p-3 bg-primary-50 dark:bg-primary-900/20 rounded-lg mb-4 animate-slide-up">
            <span className="text-sm font-medium text-primary-700 dark:text-primary-300">
                {selectedCount} alert(s) selected
            </span>
            <div className="flex-1" />
            <button onClick={() => onAction('acknowledged')} className="btn btn-sm btn-secondary">
                Acknowledge
            </button>
            <button onClick={() => onAction('resolved')} className="btn btn-sm btn-success">
                Resolve
            </button>
            <button onClick={() => onAction('false_positive')} className="btn btn-sm btn-secondary">
                False Positive
            </button>
            <button onClick={onClear} className="p-1 text-slate-500 hover:text-slate-700">
                <X className="w-4 h-4" />
            </button>
        </div>
    );
});

export default BulkActionsToolbar;
