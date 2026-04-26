import { Search } from 'lucide-react';
import { MultiSelect, DateRangePicker, type DateRange } from '../';
import { SEVERITY_OPTIONS, STATUS_OPTIONS } from './alertsConstants';

export interface AlertFilters {
    severities: string[];
    statuses: string[];
    search: string;
}

interface AlertsFiltersBarProps {
    filters: AlertFilters;
    dateRange: DateRange;
    onFiltersChange: (filters: AlertFilters) => void;
    onDateRangeChange: (range: DateRange) => void;
}

export function AlertsFiltersBar({
    filters,
    dateRange,
    onFiltersChange,
    onDateRangeChange
}: AlertsFiltersBarProps) {
    return (
        <div className="relative z-20 shrink-0 bg-white dark:bg-slate-900/50 border border-slate-200/80 dark:border-slate-700/50 rounded-xl p-4 shadow-sm">
            <div className="flex flex-wrap gap-4 items-end">
                <MultiSelect
                    options={SEVERITY_OPTIONS}
                    selected={filters.severities}
                    onChange={(severities) => onFiltersChange({ ...filters, severities })}
                    placeholder="All Severities"
                    label="Severity"
                />
                <MultiSelect
                    options={STATUS_OPTIONS}
                    selected={filters.statuses}
                    onChange={(statuses) => onFiltersChange({ ...filters, statuses })}
                    placeholder="All Statuses"
                    label="Status"
                />
                <DateRangePicker
                    value={dateRange}
                    onChange={onDateRangeChange}
                    label="Date Range"
                />
                <div className="flex-1 min-w-[200px]">
                    <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">Search</label>
                    <div className="relative">
                        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-400" />
                        <input
                            type="text"
                            placeholder="   Search by rule, agent..."
                            value={filters.search}
                            onChange={(e) => onFiltersChange({ ...filters, search: e.target.value })}
                            className="input pl-9"
                        />
                    </div>
                </div>
            </div>
        </div>
    );
}

export default AlertsFiltersBar;
