import React, { useState, useEffect } from 'react';
import { Shield, TrendingUp, Lightbulb, Clock, AlertCircle, CheckCircle, BrainCircuit } from 'lucide-react';

interface Suggestion {
  id: string;
  type: 'urgent' | 'recommendation' | 'tip';
  icon: React.ReactNode;
  title: string;
  description: string;
  action: string;
  priority: 'high' | 'medium' | 'low';
}

interface AlertDetails {
  severity: string;
  ruleName: string;
  agentId: string;
  title: string;
  riskScore?: number;
}

interface UserAssistantProps {
  alertContext?: {
    alertId: string;
    alertDetails: AlertDetails;
    timestamp: string;
  };
  onSuggestionAction: (action: string) => void;
}

export function UserAssistant({ alertContext, onSuggestionAction }: UserAssistantProps) {
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);

  useEffect(() => {
    if (alertContext) {
      generateSuggestions();
    }
  }, [alertContext]);

  const generateSuggestions = () => {
    const newSuggestions: Suggestion[] = [];

    // Severity-based suggestions
    if (alertContext?.alertDetails.severity === 'critical') {
      newSuggestions.push({
        id: 'critical_containment',
        type: 'urgent',
        icon: <Shield className="w-4 h-4 text-rose-600" />,
        title: 'Immediate Containment Required',
        description: 'Critical threat detected. Network isolation is highly recommended.',
        action: 'execute_immediate_containment',
        priority: 'high',
      });
    }

    // Rule name-based suggestions
    const ruleName = alertContext?.alertDetails.ruleName?.toLowerCase() || '';
    if (ruleName.includes('malware') || ruleName.includes('trojan') || ruleName.includes('mimikatz')) {
      newSuggestions.push({
        id: 'malware_analysis',
        type: 'recommendation',
        icon: <TrendingUp className="w-4 h-4 text-amber-600" />,
        title: 'Advanced Malware Analysis',
        description: 'Suspicious payload detected. Run deep forensic sweep on the endpoint.',
        action: 'run_advanced_analysis',
        priority: 'medium',
      });
    }

    // Risk score-based suggestions
    if (alertContext?.alertDetails.riskScore && alertContext.alertDetails.riskScore >= 70) {
      newSuggestions.push({
        id: 'high_risk_scan',
        type: 'urgent',
        icon: <AlertCircle className="w-4 h-4 text-rose-600" />,
        title: 'Comprehensive System Scan',
        description: 'High risk score indicates multiple converged signals. Full system scan advised.',
        action: 'run_comprehensive_scan',
        priority: 'high',
      });
    }

    // Ransomware-specific suggestions
    if (ruleName.includes('ransomware') || ruleName.includes('encryption') || ruleName.includes('vssadmin')) {
      newSuggestions.push({
        id: 'ransomware_response',
        type: 'urgent',
        icon: <Shield className="w-4 h-4 text-rose-600" />,
        title: 'Ransomware Protocol',
        description: 'File encryption or shadow copy deletion detected. Suspend processes instantly.',
        action: 'execute_ransomware_response',
        priority: 'high',
      });
    }

    // General tips
    newSuggestions.push({
      id: 'review_rules',
      type: 'tip',
      icon: <Lightbulb className="w-4 h-4 text-indigo-600" />,
      title: 'Review Automation Rules',
      description: 'Consider creating a new automation rule for this specific threat pattern.',
      action: 'review_automation_rules',
      priority: 'low',
    });

    // Sort by priority
    newSuggestions.sort((a, b) => {
      const priorityOrder = { high: 0, medium: 1, low: 2 };
      return priorityOrder[a.priority] - priorityOrder[b.priority];
    });

    setSuggestions(newSuggestions);
  };

  const getSuggestionTypeColor = (type: string) => {
    switch (type) {
      case 'urgent': return 'border-rose-200 bg-rose-50 dark:border-rose-800/50 dark:bg-rose-900/20';
      case 'recommendation': return 'border-amber-200 bg-amber-50 dark:border-amber-800/50 dark:bg-amber-900/20';
      case 'tip': return 'border-indigo-200 bg-indigo-50 dark:border-indigo-800/50 dark:bg-indigo-900/20';
      default: return 'border-slate-200 bg-slate-50 dark:border-slate-800/50 dark:bg-slate-900/20';
    }
  };

  const getPriorityColor = (priority: string) => {
    switch (priority) {
      case 'high': return 'text-rose-600 dark:text-rose-400';
      case 'medium': return 'text-amber-600 dark:text-amber-400';
      case 'low': return 'text-slate-600 dark:text-slate-400';
      default: return 'text-slate-600 dark:text-slate-400';
    }
  };

  const getPriorityBgColor = (priority: string) => {
    switch (priority) {
      case 'high': return 'bg-rose-100 dark:bg-rose-900/30';
      case 'medium': return 'bg-amber-100 dark:bg-amber-900/30';
      case 'low': return 'bg-slate-100 dark:bg-slate-800';
      default: return 'bg-slate-100 dark:bg-slate-800';
    }
  };

  if (!alertContext) {
    return null;
  }

  return (
    <div className="border border-purple-200 dark:border-purple-800/50 rounded-xl bg-purple-50/50 dark:bg-purple-900/10 shadow-sm">
      <div className="flex items-center justify-between p-4 border-b border-purple-200 dark:border-purple-800/50">
        <div className="flex items-center gap-2">
          <BrainCircuit className="w-5 h-5 text-purple-600 dark:text-purple-400" />
          <h3 className="font-bold text-purple-900 dark:text-purple-100">
            Copilot Recommendations
          </h3>
        </div>
        <div className="flex items-center gap-1.5 text-[11px] font-medium uppercase tracking-wider text-purple-600 dark:text-purple-400">
          <Clock className="w-3.5 h-3.5" />
          <span>Real-time</span>
        </div>
      </div>
      
      <div className="p-6 space-y-4">
        {suggestions.length === 0 ? (
          <div className="text-center text-slate-500 dark:text-slate-400 py-6">
            <Lightbulb className="w-10 h-10 mx-auto mb-3 opacity-30 text-purple-500" />
            <p className="font-medium text-slate-600 dark:text-slate-300">No specific recommendations available</p>
            <p className="text-sm mt-1">Review standard playbooks below.</p>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {suggestions.map((suggestion) => (
                <div
                key={suggestion.id}
                className={`border rounded-xl p-4 transition-all hover:shadow-md ${getSuggestionTypeColor(suggestion.type)}`}
                >
                <div className="flex items-start gap-4">
                    <div className="flex-shrink-0 mt-0.5 p-2 bg-white dark:bg-slate-800 rounded-lg shadow-sm border border-slate-100 dark:border-slate-700">
                    {suggestion.icon}
                    </div>
                    <div className="flex-1 min-w-0">
                    <div className="flex items-center justify-between mb-1.5">
                        <h4 className={`font-bold text-sm ${getPriorityColor(suggestion.priority)}`}>
                        {suggestion.title}
                        </h4>
                        <span className={`text-[10px] uppercase tracking-wider font-bold px-2 py-0.5 rounded-full ${getPriorityColor(suggestion.priority)} ${getPriorityBgColor(suggestion.priority)}`}>
                        {suggestion.priority} Priority
                        </span>
                    </div>
                    <p className="text-sm text-slate-700 dark:text-slate-300 mb-4 font-medium">
                        {suggestion.description}
                    </p>
                    <button
                        onClick={() => onSuggestionAction(suggestion.action)}
                        className="text-xs font-bold uppercase tracking-wider text-purple-700 dark:text-purple-300 hover:text-purple-800 dark:hover:text-purple-200 flex items-center gap-1.5 bg-purple-100 dark:bg-purple-900/40 px-3 py-1.5 rounded-lg w-fit transition-colors"
                    >
                        <CheckCircle className="w-3.5 h-3.5" />
                        Apply Action
                    </button>
                    </div>
                </div>
                </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
