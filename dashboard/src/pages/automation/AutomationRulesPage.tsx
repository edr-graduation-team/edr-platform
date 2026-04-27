import { useState, useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { AlertContextPanel } from '../../components/automation/AlertContextPanel';
import { UserAssistant } from '../../components/automation/UserAssistant';
import { Settings, TrendingUp, Clock, AlertTriangle, Plus, Activity, Power, X, CheckCircle, Trash2 } from 'lucide-react';
import { automationApi } from '../../api/client';

interface AutomationRule {
  id: string;
  name: string;
  description: string;
  priority: number;
  autoExecute: boolean;
  enabled: boolean;
  successRate: number;
  cooldownMinutes: number;
  lastExecution?: string;       // only set when DB has an actual last_execution
  matchesCurrentAlert?: boolean;
  playbookId?: string;
  triggerConditions?: any;
}

interface AlertContext {
  alertId: string;
  alertDetails: {
    severity: string;
    ruleName: string;
    agentId: string;
    title: string;
    description?: string;
    riskScore?: number;
  };
  timestamp: string;
}

export function AutomationRulesPage() {
  const location = useLocation();
  const [alertContext, setAlertContext] = useState<AlertContext | null>(null);
  const [rules, setRules] = useState<AutomationRule[]>([]);
  const [loading, setLoading] = useState(true);

  // Modal State
  const [isCreatingRule, setIsCreatingRule] = useState(false);
  const [newRuleName, setNewRuleName] = useState('');
  const [triggerCondition, setTriggerCondition] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [playbooks, setPlaybooks] = useState<any[]>([]);
  const [selectedPlaybookId, setSelectedPlaybookId] = useState('');
  const [editingRuleId, setEditingRuleId] = useState<string | null>(null);
  const [autoExecute, setAutoExecute] = useState(true);

  useEffect(() => {
    // Extract alert context from navigation state
    const state = location.state as any;
    if (state?.alertId && state?.alertDetails) {
      setAlertContext({
        alertId: state.alertId,
        alertDetails: state.alertDetails,
        timestamp: new Date().toISOString(),
      });
    }

    // Fetch automation rules and playbooks
    fetchRules();
    fetchPlaybooks();
  }, [location.state]);

  const fetchPlaybooks = async () => {
    try {
      const res = await automationApi.listPlaybooks();
      let pbs = res.playbooks || [];
      
      setPlaybooks(pbs);
      if (pbs.length > 0 && !selectedPlaybookId) {
        setSelectedPlaybookId(pbs[0].id);
      }
    } catch (error) {
      console.error('Failed to fetch playbooks:', error);
    }
  };

  const fetchRules = async () => {
    try {
      setLoading(true);
      const res = await automationApi.listRules();
      
      let formattedRules: AutomationRule[] = (res.rules || []).map((r: any) => ({
        id: r.id,
        name: r.name,
        description: r.description,
        priority: r.priority ?? 5,
        autoExecute: Boolean(r.auto_execute),
        enabled: Boolean(r.enabled),
        successRate: typeof r.success_rate === 'number' ? r.success_rate : 0,
        cooldownMinutes: r.cooldown_minutes ?? 30,
        // Only show lastExecution if the DB actually has a non-null last_execution field
        lastExecution: r.last_execution ?? undefined,
        matchesCurrentAlert: false,
        playbookId: r.playbook_id,
        triggerConditions: r.trigger_conditions,
      }));

      setRules(formattedRules);
    } catch (error) {
      console.error('Failed to fetch automation rules:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleSuggestionAction = (action: string) => {
    console.log('Suggestion action:', action);
    if (action === 'review_automation_rules') {
        openCreateModal();
    }
  };

  const handleDeleteRule = async (ruleId: string) => {
    if (!window.confirm("Are you sure you want to delete this rule?")) return;
    
    try {
      if (ruleId) {
        await automationApi.deleteRule(ruleId);
      }
      setRules(prev => prev.filter(r => r.id !== ruleId));
    } catch (error) {
      console.error("Failed to delete rule:", error);
      alert("Failed to delete rule.");
    }
  };


  const handleRuleToggle = async (rule: any) => {
    // Optimistic UI update
    setRules(prev => prev.map(r =>
      r.id === rule.id ? { ...r, enabled: !r.enabled } : r
    ));

    try {
      if (rule.id) {
        await automationApi.toggleRule(rule.id, !rule.enabled);
        // Re-fetch to guarantee UI matches DB state
        await fetchRules();
      }
    } catch (error) {
      console.error('Failed to toggle rule:', error);
      // Revert optimistic update on failure
      setRules(prev => prev.map(r =>
        r.id === rule.id ? { ...r, enabled: rule.enabled } : r
      ));
    }
  };

  const openCreateModal = () => {
    setIsCreatingRule(true);
    setEditingRuleId(null);
    setAutoExecute(true);
    if (alertContext?.alertDetails?.ruleName) {
      setNewRuleName(`Response Rule for: ${alertContext.alertDetails.ruleName}`);
      setTriggerCondition(`RuleName == "${alertContext.alertDetails.ruleName}" && Severity == "${alertContext.alertDetails.severity}"`);
    } else {
      setNewRuleName('');
      setTriggerCondition('');
    }
  };

  const openEditModal = (rule: AutomationRule) => {
    setIsCreatingRule(true);
    setEditingRuleId(rule.id);
    setNewRuleName(rule.name);
    // Best effort mapping of condition
    let cond = rule.description.replace('Custom rule triggering on condition: ', '');
    if (rule.triggerConditions && typeof rule.triggerConditions === 'string') cond = rule.triggerConditions;
    setTriggerCondition(cond);
    setAutoExecute(rule.autoExecute);
    if (rule.playbookId) setSelectedPlaybookId(rule.playbookId);
  };

  const confirmCreateRule = async () => {
    if (!newRuleName || !triggerCondition) {
      alert("Please fill out required fields.");
      return;
    }
    setIsSaving(true);
    
    try {
      const payload = {
        name: newRuleName,
        description: `Custom rule triggering on condition: ${triggerCondition}`,
        trigger_conditions: { condition: triggerCondition },
        priority: 5,
        auto_execute: autoExecute,
        enabled: true,
        playbook_id: selectedPlaybookId || undefined
      };
      
      if (editingRuleId) {
        await automationApi.updateRule(editingRuleId, payload);
        alert(`Automation Rule "${newRuleName}" updated successfully!`);
      } else {
        await automationApi.createRule(payload);
        alert(`Automation Rule "${newRuleName}" created successfully!`);
      }
      
      setIsSaving(false);
      setIsCreatingRule(false);
      
      fetchRules();
    } catch (err) {
      console.error("Failed to save rule:", err);
      alert("Failed to save rule. Please try again.");
      setIsSaving(false);
    }
  };

  const getPriorityColor = (priority: number) => {
    if (priority <= 2) return 'bg-rose-100 text-rose-800 border border-rose-200 dark:bg-rose-900/30 dark:text-rose-400 dark:border-rose-800/50';
    if (priority <= 5) return 'bg-amber-100 text-amber-800 border border-amber-200 dark:bg-amber-900/30 dark:text-amber-400 dark:border-amber-800/50';
    return 'bg-slate-100 text-slate-800 border border-slate-200 dark:bg-slate-800 dark:text-slate-400 dark:border-slate-700/50';
  };

  const getPriorityLabel = (priority: number) => {
    if (priority <= 2) return 'Critical Priority';
    if (priority <= 5) return 'High Priority';
    if (priority <= 8) return 'Medium Priority';
    return 'Low Priority';
  };

  const getSuccessRateColor = (rate: number) => {
    if (rate >= 0.9) return 'text-emerald-600 dark:text-emerald-400';
    if (rate >= 0.7) return 'text-amber-600 dark:text-amber-400';
    return 'text-rose-600 dark:text-rose-400';
  };

  return (
    <div className="space-y-6 relative">
      {/* Page Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-blue-100 dark:bg-blue-900/30 rounded-lg">
            <Activity className="w-6 h-6 text-blue-600 dark:text-blue-400" />
          </div>
          <div>
            <h1 className="text-2xl font-bold text-slate-900 dark:text-white">
              Automation Rules
            </h1>
            <p className="text-sm text-slate-500 mt-1">Configure trigger conditions to autonomously deploy playbooks.</p>
          </div>
        </div>
        <div className="flex items-center gap-4">
            {alertContext && (
            <div className="flex items-center gap-2 text-sm text-blue-600 dark:text-blue-400 bg-blue-50 dark:bg-blue-900/20 px-3 py-1.5 rounded-full border border-blue-200 dark:border-blue-800/50 font-medium">
                <AlertTriangle className="w-4 h-4" />
                <span>Active Alert Context</span>
            </div>
            )}
            <button
                className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg flex items-center gap-2 font-medium transition-colors"
                onClick={openCreateModal}
            >
                <Plus className="w-4 h-4" />
                Create Rule
            </button>
        </div>
      </div>

      {/* Alert Context Panel */}
      {alertContext && (
        <AlertContextPanel
          alertId={alertContext.alertId}
          alertDetails={alertContext.alertDetails}
          onClearContext={() => setAlertContext(null)}
        />
      )}

      {/* User Assistant */}
      <UserAssistant
        alertContext={alertContext || undefined}
        onSuggestionAction={handleSuggestionAction}
      />

      {/* Rules List */}
      <div className="bg-white dark:bg-slate-800 rounded-xl border border-slate-200 dark:border-slate-700 shadow-sm overflow-hidden">
        <div className="p-6 border-b border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50">
          <h2 className="text-lg font-bold text-slate-900 dark:text-white">
            Configured Automation Rules
          </h2>
          <p className="text-sm text-slate-500 mt-1">
            Rules evaluate incoming alerts and telemetry against conditions to trigger autonomous responses.
          </p>
        </div>

        {loading ? (
          <div className="p-12 text-center">
            <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
            <p className="text-sm font-medium text-slate-500 mt-4">Loading rules...</p>
          </div>
        ) : (
          <div className="divide-y divide-slate-200 dark:divide-slate-700/80">
            {rules.map((rule) => (
              <div key={rule.id} className={`p-6 transition-colors ${rule.enabled ? 'hover:bg-slate-50 dark:hover:bg-slate-800/40' : 'bg-slate-50/50 dark:bg-slate-800/20 opacity-80'}`}>
                <div className="flex flex-col lg:flex-row lg:items-start justify-between gap-6">
                  <div className="flex-1">
                    <div className="flex items-center gap-3 mb-2">
                      <h3 className={`font-bold text-lg ${rule.enabled ? 'text-slate-900 dark:text-white' : 'text-slate-500 dark:text-slate-400'}`}>
                        {rule.name}
                      </h3>
                      <span className={`px-2.5 py-0.5 text-[11px] uppercase tracking-wider font-bold rounded-md ${getPriorityColor(rule.priority)}`}>
                        {getPriorityLabel(rule.priority)}
                      </span>
                      {rule.matchesCurrentAlert && (
                        <span className="px-2.5 py-0.5 text-[11px] uppercase tracking-wider font-bold rounded-md bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400 border border-blue-200 dark:border-blue-800">
                          Matches Active Alert
                        </span>
                      )}
                    </div>
                    <p className={`text-sm mb-4 max-w-3xl ${rule.enabled ? 'text-slate-600 dark:text-slate-400' : 'text-slate-400 dark:text-slate-500'}`}>
                      {rule.description}
                    </p>
                    
                    {/* Metadata Badges — all driven from DB data */}
                    <div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-sm mb-4">

                      {/* Success Rate */}
                      <div className="flex items-center gap-2">
                        <TrendingUp className="w-4 h-4 text-slate-400 shrink-0" />
                        <span className="text-slate-500">Success Rate:</span>
                        <span className={`font-bold ${getSuccessRateColor(rule.successRate)}`}>
                          {rule.successRate > 0 ? `${(rule.successRate * 100).toFixed(1)}%` : 'No data yet'}
                        </span>
                      </div>

                      {/* Cooldown */}
                      <div className="flex items-center gap-2">
                        <Clock className="w-4 h-4 text-slate-400 shrink-0" />
                        <span className="text-slate-500">Cooldown:</span>
                        <span className="font-semibold text-slate-700 dark:text-slate-300">
                          {rule.cooldownMinutes === 0 ? 'None' : `${rule.cooldownMinutes} min`}
                        </span>
                      </div>

                      {/* Linked Playbook */}
                      {rule.playbookId && (() => {
                        const pb = playbooks.find((p: any) => p.id === rule.playbookId);
                        return pb ? (
                          <div className="flex items-center gap-2">
                            <Activity className="w-4 h-4 text-slate-400 shrink-0" />
                            <span className="text-slate-500">Playbook:</span>
                            <span className="font-semibold text-indigo-600 dark:text-indigo-400 truncate max-w-[200px]" title={pb.name}>
                              {pb.name}
                            </span>
                          </div>
                        ) : null;
                      })()}

                      {/* Last Run — only shown if DB has an actual last_execution value */}
                      {rule.lastExecution ? (
                        <div className="flex items-center gap-2 text-slate-500">
                          <Clock className="w-4 h-4 shrink-0" />
                          <span>Last Run: {new Date(rule.lastExecution).toLocaleString()}</span>
                        </div>
                      ) : (
                        <div className="flex items-center gap-2 text-slate-400">
                          <Clock className="w-4 h-4 shrink-0" />
                          <span className="italic">Never executed</span>
                        </div>
                      )}
                    </div>

                    {/* Trigger Conditions */}
                    {rule.triggerConditions && (
                      <div className="text-xs bg-slate-100 dark:bg-slate-800/60 rounded-lg px-3 py-2 font-mono text-slate-600 dark:text-slate-400 max-w-xl truncate" title={JSON.stringify(rule.triggerConditions)}>
                        <span className="font-sans font-semibold text-slate-500 mr-2 not-italic">Trigger:</span>
                        {typeof rule.triggerConditions === 'string'
                          ? rule.triggerConditions
                          : JSON.stringify(rule.triggerConditions)}
                      </div>
                    )}
                  </div>

                  {/* Actions */}
                  <div className="flex lg:flex-col items-center justify-center gap-3 shrink-0 min-w-[140px]">
                    <button
                      onClick={() => handleRuleToggle(rule)}
                      className={`px-4 py-2 rounded-lg font-medium flex items-center justify-center gap-2 w-full transition-all border ${
                          rule.enabled 
                          ? 'bg-rose-50 text-rose-700 border-rose-200 hover:bg-rose-100 dark:bg-rose-900/20 dark:text-rose-400 dark:border-rose-800/50 dark:hover:bg-rose-900/40' 
                          : 'bg-emerald-50 text-emerald-700 border-emerald-200 hover:bg-emerald-100 dark:bg-emerald-900/20 dark:text-emerald-400 dark:border-emerald-800/50 dark:hover:bg-emerald-900/40'
                      }`}
                    >
                      <Power className="w-4 h-4" />
                      {rule.enabled ? 'Disable Rule' : 'Enable Rule'}
                    </button>
                    <div className="flex w-full gap-2">
                        <button
                            onClick={() => openEditModal(rule)}
                            className="flex-1 px-4 py-2 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-300 dark:border-slate-600 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-700 font-medium transition-colors flex items-center justify-center gap-2"
                        >
                            <Settings className="w-4 h-4" />
                            Configure
                        </button>
                        <button
                            onClick={() => handleDeleteRule(rule.id)}
                            className="px-3 py-2 bg-white dark:bg-slate-800 text-rose-600 dark:text-rose-400 border border-slate-300 dark:border-slate-600 rounded-lg hover:bg-rose-50 dark:hover:bg-rose-900/20 font-medium transition-colors flex items-center justify-center"
                            title="Delete Rule"
                        >
                            <Trash2 className="w-4 h-4" />
                        </button>
                    </div>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Create Rule Modal */}
      {isCreatingRule && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
          <div className="bg-white dark:bg-slate-900 rounded-2xl shadow-2xl w-full max-w-2xl overflow-hidden border border-slate-200 dark:border-slate-800">
            <div className="flex items-center justify-between p-6 border-b border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/30">
              <div>
                <h2 className="text-xl font-bold text-slate-900 dark:text-white flex items-center gap-2">
                  <Activity className="w-5 h-5 text-blue-500" />
                  {editingRuleId ? 'Edit Automation Rule' : 'Create Automation Rule'}
                </h2>
                <p className="text-sm text-slate-500 mt-1">Map alert telemetry to automated playbooks.</p>
              </div>
              <button 
                onClick={() => setIsCreatingRule(false)}
                className="p-2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 rounded-lg hover:bg-slate-200 dark:hover:bg-slate-800 transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>
            
            <div className="p-6 space-y-5">
              <div>
                <label className="block text-sm font-bold text-slate-700 dark:text-slate-300 mb-2">
                  Rule Name <span className="text-rose-500">*</span>
                </label>
                <input 
                  type="text" 
                  value={newRuleName}
                  onChange={(e) => setNewRuleName(e.target.value)}
                  placeholder="e.g., Contain Ransomware Behavior"
                  className="w-full bg-white dark:bg-slate-950 border border-slate-300 dark:border-slate-700 rounded-lg px-4 py-3 text-slate-900 dark:text-white focus:ring-2 focus:ring-blue-500 focus:border-blue-500 outline-none transition-shadow"
                />
              </div>

              <div>
                <label className="block text-sm font-bold text-slate-700 dark:text-slate-300 mb-2">
                  Trigger Condition (Sigma or SQL-like syntax) <span className="text-rose-500">*</span>
                </label>
                <textarea 
                  value={triggerCondition}
                  onChange={(e) => setTriggerCondition(e.target.value)}
                  placeholder="e.g., RuleName == 'Suspicious File Write' && RiskScore > 80"
                  className="w-full h-24 bg-white dark:bg-slate-950 border border-slate-300 dark:border-slate-700 rounded-lg px-4 py-3 text-slate-900 dark:text-white focus:ring-2 focus:ring-blue-500 focus:border-blue-500 outline-none font-mono text-sm"
                />
                {alertContext?.alertDetails?.ruleName && (
                  <p className="text-xs text-emerald-600 dark:text-emerald-400 mt-2 flex items-center gap-1.5 font-medium">
                    <CheckCircle className="w-3.5 h-3.5" />
                    Condition pre-filled based on the currently active alert context.
                  </p>
                )}
              </div>

              <div>
                <label className="block text-sm font-bold text-slate-700 dark:text-slate-300 mb-2">
                  Target Playbook
                </label>
                <select 
                  value={selectedPlaybookId}
                  onChange={(e) => setSelectedPlaybookId(e.target.value)}
                  className="w-full bg-white dark:bg-slate-950 border border-slate-300 dark:border-slate-700 rounded-lg px-4 py-3 text-slate-900 dark:text-white focus:ring-2 focus:ring-blue-500 focus:border-blue-500 outline-none appearance-none"
                >
                  {playbooks.map(pb => (
                    <option key={pb.id} value={pb.id}>{pb.name}</option>
                  ))}
                </select>
              </div>

              <div className="flex items-center gap-3 pt-2">
                <input 
                  type="checkbox" 
                  id="autoExec" 
                  checked={autoExecute}
                  onChange={(e) => setAutoExecute(e.target.checked)}
                  className="w-4 h-4 text-blue-600 rounded border-gray-300 focus:ring-blue-500" 
                />
                <label htmlFor="autoExec" className="text-sm font-medium text-slate-700 dark:text-slate-300">
                  Enable Auto-Execution (No manual approval required)
                </label>
              </div>
            </div>

            <div className="flex items-center justify-end gap-3 p-6 border-t border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/30">
              <button 
                onClick={() => setIsCreatingRule(false)}
                className="px-5 py-2.5 font-medium text-slate-600 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-800 rounded-lg transition-colors"
                disabled={isSaving}
              >
                Cancel
              </button>
              <button 
                onClick={confirmCreateRule}
                disabled={isSaving}
                className="px-6 py-2.5 font-bold text-white bg-blue-600 hover:bg-blue-700 rounded-lg shadow-md transition-all flex items-center gap-2 disabled:opacity-70 disabled:cursor-not-allowed"
              >
                {isSaving ? (
                  <>
                    <div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                    Saving...
                  </>
                ) : (
                  <>
                    <Activity className="w-4 h-4" />
                    {editingRuleId ? 'Save Changes' : 'Create Rule'}
                  </>
                )}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
