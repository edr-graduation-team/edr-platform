import { useState, useRef, useEffect } from 'react';
import { ChevronDown, Check, X } from 'lucide-react';

export interface MultiSelectOption {
    value: string;
    label: string;
    count?: number;
    color?: string;
}

interface MultiSelectProps {
    options: MultiSelectOption[];
    selected: string[];
    onChange: (selected: string[]) => void;
    placeholder?: string;
    label?: string;
    showCounts?: boolean;
}

export function MultiSelect({
    options,
    selected,
    onChange,
    placeholder = 'Select...',
    label,
    showCounts = true,
}: MultiSelectProps) {
    const [isOpen, setIsOpen] = useState(false);
    const containerRef = useRef<HTMLDivElement>(null);

    // Close on outside click
    useEffect(() => {
        const handleClickOutside = (e: MouseEvent) => {
            if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
                setIsOpen(false);
            }
        };

        document.addEventListener('mousedown', handleClickOutside);
        return () => document.removeEventListener('mousedown', handleClickOutside);
    }, []);

    const toggleOption = (value: string) => {
        if (selected.includes(value)) {
            onChange(selected.filter((v) => v !== value));
        } else {
            onChange([...selected, value]);
        }
    };

    const selectAll = () => {
        onChange(options.map((o) => o.value));
    };

    const clearAll = () => {
        onChange([]);
    };

    const selectedLabels = options
        .filter((o) => selected.includes(o.value))
        .map((o) => o.label);

    return (
        <div ref={containerRef} className="relative">
            {label && (
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                    {label}
                </label>
            )}

            {/* Trigger Button */}
            <button
                type="button"
                onClick={() => setIsOpen(!isOpen)}
                className="w-full flex items-center justify-between gap-2 px-3 py-2 text-left border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 hover:border-primary-500 focus:ring-2 focus:ring-primary-500 focus:border-primary-500 outline-none transition-colors"
            >
                <span className="truncate text-gray-700 dark:text-gray-200">
                    {selected.length === 0
                        ? placeholder
                        : selected.length === 1
                            ? selectedLabels[0]
                            : `${selected.length} selected`}
                </span>
                <ChevronDown
                    className={`w-4 h-4 text-gray-400 transition-transform ${isOpen ? 'rotate-180' : ''}`}
                />
            </button>

            {/* Selected Chips */}
            {selected.length > 0 && (
                <div className="flex flex-wrap gap-1 mt-2">
                    {selected.slice(0, 3).map((value) => {
                        const option = options.find((o) => o.value === value);
                        return (
                            <span
                                key={value}
                                className="inline-flex items-center gap-1 px-2 py-0.5 text-xs font-medium rounded-full bg-primary-100 text-primary-700 dark:bg-primary-900 dark:text-primary-200"
                            >
                                {option?.label || value}
                                <button
                                    type="button"
                                    onClick={(e) => {
                                        e.stopPropagation();
                                        toggleOption(value);
                                    }}
                                    className="hover:text-primary-900 dark:hover:text-white"
                                >
                                    <X className="w-3 h-3" />
                                </button>
                            </span>
                        );
                    })}
                    {selected.length > 3 && (
                        <span className="px-2 py-0.5 text-xs text-gray-500 dark:text-gray-400">
                            +{selected.length - 3} more
                        </span>
                    )}
                </div>
            )}

            {/* Dropdown */}
            {isOpen && (
                <div className="absolute z-50 mt-1 w-full bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg">
                    {/* Actions */}
                    <div className="flex items-center justify-between px-3 py-2 border-b border-gray-200 dark:border-gray-700 text-xs">
                        <button
                            type="button"
                            onClick={selectAll}
                            className="text-primary-600 hover:text-primary-700 dark:text-primary-400"
                        >
                            Select All
                        </button>
                        <button
                            type="button"
                            onClick={clearAll}
                            className="text-gray-500 hover:text-gray-700 dark:hover:text-gray-300"
                        >
                            Clear All
                        </button>
                    </div>

                    {/* Options */}
                    <div className="max-h-60 overflow-y-auto py-1">
                        {options.map((option) => {
                            const isSelected = selected.includes(option.value);
                            return (
                                <button
                                    key={option.value}
                                    type="button"
                                    onClick={() => toggleOption(option.value)}
                                    className={`w-full flex items-center gap-3 px-3 py-2 text-left hover:bg-gray-100 dark:hover:bg-gray-700 ${isSelected ? 'bg-primary-50 dark:bg-primary-900/20' : ''
                                        }`}
                                >
                                    {/* Checkbox */}
                                    <div
                                        className={`w-4 h-4 rounded border flex items-center justify-center ${isSelected
                                                ? 'bg-primary-600 border-primary-600'
                                                : 'border-gray-300 dark:border-gray-600'
                                            }`}
                                    >
                                        {isSelected && <Check className="w-3 h-3 text-white" />}
                                    </div>

                                    {/* Color indicator */}
                                    {option.color && (
                                        <div
                                            className="w-3 h-3 rounded-full"
                                            style={{ backgroundColor: option.color }}
                                        />
                                    )}

                                    {/* Label */}
                                    <span className="flex-1 text-sm text-gray-700 dark:text-gray-200">
                                        {option.label}
                                    </span>

                                    {/* Count */}
                                    {showCounts && option.count !== undefined && (
                                        <span className="text-xs text-gray-400">
                                            ({option.count})
                                        </span>
                                    )}
                                </button>
                            );
                        })}
                    </div>
                </div>
            )}
        </div>
    );
}

export default MultiSelect;
