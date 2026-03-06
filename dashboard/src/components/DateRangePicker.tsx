import { useState } from 'react';
import { Calendar, Clock } from 'lucide-react';

export interface DateRange {
    from: Date | null;
    to: Date | null;
}

interface DateRangePickerProps {
    value: DateRange;
    onChange: (range: DateRange) => void;
    label?: string;
}

type PresetKey = 'last24h' | 'last7d' | 'last30d' | 'today' | 'yesterday' | 'custom';

const presets: { key: PresetKey; label: string; getRange: () => DateRange }[] = [
    {
        key: 'last24h',
        label: 'Last 24h',
        getRange: () => ({
            from: new Date(Date.now() - 24 * 60 * 60 * 1000),
            to: new Date(),
        }),
    },
    {
        key: 'last7d',
        label: 'Last 7 Days',
        getRange: () => ({
            from: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000),
            to: new Date(),
        }),
    },
    {
        key: 'last30d',
        label: 'Last 30 Days',
        getRange: () => ({
            from: new Date(Date.now() - 30 * 24 * 60 * 60 * 1000),
            to: new Date(),
        }),
    },
    {
        key: 'today',
        label: 'Today',
        getRange: () => {
            const today = new Date();
            today.setHours(0, 0, 0, 0);
            return { from: today, to: new Date() };
        },
    },
    {
        key: 'yesterday',
        label: 'Yesterday',
        getRange: () => {
            const yesterday = new Date(Date.now() - 24 * 60 * 60 * 1000);
            yesterday.setHours(0, 0, 0, 0);
            const yesterdayEnd = new Date(yesterday);
            yesterdayEnd.setHours(23, 59, 59, 999);
            return { from: yesterday, to: yesterdayEnd };
        },
    },
];

export function DateRangePicker({ value, onChange, label }: DateRangePickerProps) {
    const [isOpen, setIsOpen] = useState(false);
    const [activePreset, setActivePreset] = useState<PresetKey | null>('last24h');
    const [customFrom, setCustomFrom] = useState('');
    const [customTo, setCustomTo] = useState('');

    const handlePresetClick = (preset: typeof presets[0]) => {
        setActivePreset(preset.key);
        onChange(preset.getRange());
        if (preset.key !== 'custom') {
            setIsOpen(false);
        }
    };

    const handleCustomApply = () => {
        if (customFrom && customTo) {
            onChange({
                from: new Date(customFrom),
                to: new Date(customTo),
            });
            setActivePreset('custom');
            setIsOpen(false);
        }
    };

    const formatDateRange = () => {
        if (!value.from || !value.to) return 'Select date range';

        const preset = presets.find((p) => p.key === activePreset);
        if (preset && activePreset !== 'custom') {
            return preset.label;
        }

        const formatDate = (date: Date) =>
            date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });

        return `${formatDate(value.from)} - ${formatDate(value.to)}`;
    };

    return (
        <div className="relative">
            {label && (
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                    {label}
                </label>
            )}

            {/* Trigger */}
            <button
                type="button"
                onClick={() => setIsOpen(!isOpen)}
                className="flex items-center gap-2 px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 hover:border-primary-500 focus:ring-2 focus:ring-primary-500 outline-none transition-colors text-sm"
            >
                <Calendar className="w-4 h-4 text-gray-400" />
                <span className="text-gray-700 dark:text-gray-200">{formatDateRange()}</span>
            </button>

            {/* Dropdown */}
            {isOpen && (
                <div className="absolute z-50 mt-1 w-72 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg">
                    {/* Presets */}
                    <div className="p-2 border-b border-gray-200 dark:border-gray-700">
                        <div className="flex flex-wrap gap-1">
                            {presets.map((preset) => (
                                <button
                                    key={preset.key}
                                    type="button"
                                    onClick={() => handlePresetClick(preset)}
                                    className={`px-2 py-1 text-xs rounded-md transition-colors ${activePreset === preset.key
                                            ? 'bg-primary-600 text-white'
                                            : 'text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'
                                        }`}
                                >
                                    {preset.label}
                                </button>
                            ))}
                        </div>
                    </div>

                    {/* Custom Range */}
                    <div className="p-3">
                        <p className="text-xs font-medium text-gray-500 dark:text-gray-400 mb-2">
                            Custom Range
                        </p>
                        <div className="space-y-2">
                            <div className="flex items-center gap-2">
                                <Clock className="w-4 h-4 text-gray-400" />
                                <input
                                    type="datetime-local"
                                    value={customFrom}
                                    onChange={(e) => setCustomFrom(e.target.value)}
                                    className="flex-1 px-2 py-1 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                                    placeholder="From"
                                />
                            </div>
                            <div className="flex items-center gap-2">
                                <Clock className="w-4 h-4 text-gray-400" />
                                <input
                                    type="datetime-local"
                                    value={customTo}
                                    onChange={(e) => setCustomTo(e.target.value)}
                                    className="flex-1 px-2 py-1 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                                    placeholder="To"
                                />
                            </div>
                            <button
                                type="button"
                                onClick={handleCustomApply}
                                disabled={!customFrom || !customTo}
                                className="w-full py-1.5 text-sm font-medium text-white bg-primary-600 rounded-md hover:bg-primary-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                            >
                                Apply
                            </button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}

export default DateRangePicker;
