import React, { useState } from 'react';
import { Shield, GitBranch, Cpu, Activity, Zap, ArrowUpDown, TrendingUp, CheckCircle, ChevronDown, ChevronUp } from 'lucide-react';
import { getRiskScoreStyle } from './alertsUtils';
import type { ScoreBreakdown } from '../../api/client';

interface BreakdownRow {
    label: string;
    value: number;
    sign: '+' | '−' | '=';
    color: string;
    icon: React.ReactNode;
    description: string;
}

interface ScoreBreakdownPanelProps {
    breakdown: ScoreBreakdown;
    totalScore: number;
}

export function ScoreBreakdownPanel({ breakdown, totalScore }: ScoreBreakdownPanelProps) {
    const [showFormula, setShowFormula] = useState(false);

    const rows: BreakdownRow[] = [
        {
            label: 'Base Score',
            value: breakdown.base_score,
            sign: '+',
            color: 'text-blue-600 dark:text-blue-400',
            icon: <Shield className="w-3.5 h-3.5" />,
            description: 'Starting score based on the detection rule severity (low=25, medium=45, high=65, critical=85)',
        },
        {
            label: 'Lineage Bonus',
            value: breakdown.lineage_bonus,
            sign: '+',
            color: 'text-purple-600 dark:text-purple-400',
            icon: <GitBranch className="w-3.5 h-3.5" />,
            description: 'Added when the process was spawned by an unusual or suspicious parent process',
        },
        {
            label: 'Privilege Bonus',
            value: breakdown.privilege_bonus,
            sign: '+',
            color: 'text-orange-600 dark:text-orange-400',
            icon: <Cpu className="w-3.5 h-3.5" />,
            description: 'Added when the process runs as SYSTEM or with elevated privileges',
        },
        {
            label: 'Burst Bonus',
            value: breakdown.burst_bonus,
            sign: '+',
            color: 'text-yellow-600 dark:text-yellow-400',
            icon: <Activity className="w-3.5 h-3.5" />,
            description: 'Added when this rule fires multiple times within a 5-minute window',
        },
        {
            label: 'UEBA Bonus',
            value: breakdown.ueba_bonus,
            sign: '+',
            color: 'text-red-600 dark:text-red-400',
            icon: <Zap className="w-3.5 h-3.5" />,
            description: 'Added when this activity is unusual for this user based on behavioral analysis',
        },
        {
            label: 'Interaction',
            value: breakdown.interaction_bonus || 0,
            sign: '+',
            color: 'text-pink-600 dark:text-pink-400',
            icon: <ArrowUpDown className="w-3.5 h-3.5" />,
            description: 'Added when multiple high-risk signals converge (e.g. elevated + suspicious lineage)',
        },
        {
            label: 'FP Discount',
            value: breakdown.fp_discount,
            sign: '−',
            color: 'text-green-600 dark:text-green-400',
            icon: <TrendingUp className="w-3.5 h-3.5" />,
            description: 'Subtracted when the process is a trusted/Microsoft-signed binary (less likely malicious)',
        },
        {
            label: 'UEBA Discount',
            value: breakdown.ueba_discount,
            sign: '−',
            color: 'text-teal-600 dark:text-teal-400',
            icon: <CheckCircle className="w-3.5 h-3.5" />,
            description: 'Subtracted when this activity is normal for this user based on behavioral baseline',
        },
    ];

    const maxBar = 100;
    const { bg: scoreBg, text: scoreText } = getRiskScoreStyle(totalScore);
    const interactionVal = breakdown.interaction_bonus || 0;

    return (
        <div className="space-y-4 animate-slide-up-fade">
            <div className="flex items-center justify-between">
                <span className="text-xs text-slate-500 uppercase tracking-wider block">Score Breakdown</span>
                <span className={`inline-flex items-center justify-center w-10 h-10 rounded-full text-sm font-bold ring-2 ${scoreBg} ${scoreText} ring-offset-1`}>
                    {totalScore}
                </span>
            </div>

            {/* Visual bar chart with hover tooltips */}
            <div className="space-y-2">
                {rows.map((row) => {
                    if (row.value === 0) return null;
                    const width = Math.min((Math.abs(row.value) / maxBar) * 100, 100);
                    const isDiscount = row.sign === '−';
                    return (
                        <div key={row.label} className="flex items-center gap-3 group" title={row.description}>
                            <div className="flex items-center gap-1.5 w-32 shrink-0">
                                <span className={row.color}>{row.icon}</span>
                                <span className="text-xs text-slate-600 dark:text-slate-400">{row.label}</span>
                            </div>
                            <div className="flex-1 h-5 bg-slate-100 dark:bg-slate-700 rounded overflow-hidden">
                                <div
                                    className={`h-full rounded transition-all duration-500 ${isDiscount ? 'bg-green-400 dark:bg-green-600' : 'bg-blue-400 dark:bg-blue-500'}`}
                                    style={{ width: `${width}%` }}
                                />
                            </div>
                            <span className={`text-xs font-mono font-semibold w-10 text-right ${row.color}`}>
                                {row.sign}{row.value}
                            </span>
                        </div>
                    );
                })}
            </div>

            {/* Collapsible formula — hidden by default for junior analysts */}
            <div className="border-t border-slate-200 dark:border-slate-700 pt-2">
                <button
                    onClick={() => setShowFormula(!showFormula)}
                    className="flex items-center gap-1.5 text-xs text-slate-400 hover:text-slate-600 dark:hover:text-slate-300 transition-colors"
                >
                    {showFormula ? <ChevronUp className="w-3 h-3" /> : <ChevronDown className="w-3 h-3" />}
                    {showFormula ? 'Hide calculation' : 'Show calculation'}
                </button>
                {showFormula && (
                    <div className="mt-2 text-xs text-slate-500 font-mono bg-slate-50 dark:bg-slate-800 rounded-lg p-2.5">
                        {breakdown.base_score} + {breakdown.lineage_bonus} + {breakdown.privilege_bonus} + {breakdown.burst_bonus}
                        {breakdown.ueba_bonus > 0 && ` + ${breakdown.ueba_bonus}`}
                        {interactionVal > 0 && ` + ${interactionVal}`}
                        {breakdown.fp_discount > 0 && ` − ${breakdown.fp_discount}`}
                        {breakdown.ueba_discount > 0 && ` − ${breakdown.ueba_discount}`}
                        {' '}= {breakdown.raw_score} → clamped to {breakdown.final_score}
                    </div>
                )}
            </div>
        </div>
    );
}

export default ScoreBreakdownPanel;
