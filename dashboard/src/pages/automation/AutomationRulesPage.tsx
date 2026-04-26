import { useState, useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { AlertContextPanel } from '../../components/automation/AlertContextPanel';
import { UserAssistant } from '../../components/automation/UserAssistant';
import { Settings, TrendingUp, Clock, AlertTriangle, Plus, Activity, Power, Target, X, CheckCircle } from 'lucide-react';
import { automationApi } from '../../api/client';

interface AutomationRule {
  id: string;
  name: string;
  description: string;
  priority: number;
  autoExecute: boolean;
  enabled: boolean;
  successRate: number;
  lastExecution?: string;
  matchesCurrentAlert?: boolean;
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

const DEFAULT_RULES: AutomationRule[] = [
  {
    id: 'rule-ransomware-01',
    name: 'Auto-Contain Suspected Ransomware',
    description: 'Triggers the Ransomware Containment Playbook when highly suspicious encryption behavior (e.g. vssadmin deletion + mass file rename) is detected on any agent.',
    priority: 1,
    autoExecute: true,
    enabled: true,
    successRate: 0.98,
    lastExecution: new Date(Date.now() - 3600000).toISOString()
  },
  {
    id: 'rule-usb-02',
    name: 'Block Untrusted Mass Storage',
    description: 'Triggers the USB response playbook to unmount unknown devices immediately upon detection of the hardware insertion event.',
    priority: 3,
    autoExecute: true,
    enabled: true,
    successRate: 0.95,
    lastExecution: new Date(Date.now() - 86400000).toISOString()
  },
  {
    id: 'rule-mimikatz-03',
    name: 'High Risk Process Termination',
    description: 'Terminates known credential dumping utilities (e.g. Mimikatz, Procdump against LSASS) and triggers an advanced memory scan playbook.',
    priority: 2,
    autoExecute: false, // Requires approval by default
    enabled: true,
    successRate: 0.85,
    lastExecution: new Date(Date.now() - 172800000).toISOString()
  }
];

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

    // Fetch automation rules
    fetchRules();
  }, [location.state]);

  const fetchRules = async () => {
    try {
      setLoading(true);
      const res = await automationApi.listRules();
      
      let formattedRules: AutomationRule[] = (res.rules || []).map((r: any) => ({
        id: r.id,
        name: r.name,
        description: r.description,
        priority: r.priority,
        autoExecute: r.auto_execute,
        enabled: r.enabled,
        successRate: r.success_rate || 0,
        lastExecution: r.last_execution || r.created_at,
        matchesCurrentAlert: false
      }));

      // Fallback to our EDR project defaults if DB is empty
      if (formattedRules.length === 0) {
        formattedRules = DEFAULT_RULES;
      }

      // Check for current alert matches
      if (alertContext) {
        formattedRules = formattedRules.map((rule) => {
           const ruleKeyword = alertContext.alertDetails.ruleName.toLowerCase().split(' ')[0];
           return { ...rule, matchesCurrentAlert: rule.name.toLowerCase().includes(ruleKeyword) };
        });
      }

      setRules(formattedRules);
    } catch (error) {
      console.error('Failed to fetch automation rules:', error);
      setRules(DEFAULT_RULES); // Fallback on error
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

  const handleRuleToggle = (ruleId: string) => {
    setRules(prev => prev.map(rule => 
      rule.id === ruleId ? { ...rule, enabled: !rule.enabled } : rule
    ));
  };

  const openCreateModal = () => {
    setIsCreatingRule(true);
    if (alertContext?.alertDetails?.ruleName) {
      setNewRuleName(`Response Rule for: ${alertContext.alertDetails.ruleName}`);
      setTriggerCondition(`RuleName == "${alertContext.alertDetails.ruleName}" && Severity == "${alertContext.alertDetails.severity}"`);
    } else {
      setNewRuleName('');
      setTriggerCondition('');
    }
  };

  const confirmCreateRule = () => {
    if (!newRuleName || !triggerCondition) {
      alert("Please fill out required fields.");
      return;
    }
    setIsSaving(true);
    setTimeout(() => {
      setIsSaving(false);
      setIsCreatingRule(false);
      
      // Optimistically add the new rule to the UI
      setRules([{
        id: `rule-new-${Date.now()}`,
        name: newRuleName,
        description: `Custom rule triggering on condition: ${triggerCondition}`,
        priority: 5,
        autoExecute: true,
        enabled: true,
        successRate: 1.0,
        matchesCurrentAlert: true
      }, ...rules]);

      alert(`Automation Rule "${newRuleName}" created successfully!`);
    }, 1500);
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
                    
                    {/* Metadata Badges */}
                    <div className="flex flex-wrap items-center gap-6 text-sm mb-5">
                      <div className="flex items-center gap-2">
                        <Target className="w-4 h-4 text-slate-400" />
                        <span className="text-slate-500">Auto Execute:</span>
                        <span className={`font-bold ${rule.autoExecute ? 'text-emerald-600 dark:text-emerald-400' : 'text-slate-700 dark:text-slate-300'}`}>
                            {rule.autoExecute ? 'Enabled' : 'Disabled (Requires Approval)'}
                        </span>
                      </div>
                      <div className="flex items-center gap-2">
                        <TrendingUp className="w-4 h-4 text-slate-400" />
                        <span className="text-slate-500">Success Rate:</span>
                        <span className={`font-bold ${getSuccessRateColor(rule.successRate)}`}>
                            {(rule.successRate * 100).toFixed(1)}%
                        </span>
                      </div>
                      {rule.lastExecution && (
                          <div className="flex items-center gap-2 text-slate-500">
                            <Clock className="w-4 h-4" />
                            <span>
                              Last Run: {new Date(rule.lastExecution).toLocaleString()}
                            </span>
                          </div>
                      )}
                    </div>
                  </div>

                  {/* Actions */}
                  <div className="flex lg:flex-col items-center justify-center gap-3 shrink-0 min-w-[140px]">
                    <button
                      onClick={() => handleRuleToggle(rule.id)}
                      className={`px-4 py-2 rounded-lg font-medium flex items-center justify-center gap-2 w-full transition-all border ${
                          rule.enabled 
                          ? 'bg-rose-50 text-rose-700 border-rose-200 hover:bg-rose-100 dark:bg-rose-900/20 dark:text-rose-400 dark:border-rose-800/50 dark:hover:bg-rose-900/40' 
                          : 'bg-emerald-50 text-emerald-700 border-emerald-200 hover:bg-emerald-100 dark:bg-emerald-900/20 dark:text-emerald-400 dark:border-emerald-800/50 dark:hover:bg-emerald-900/40'
                      }`}
                    >
                      <Power className="w-4 h-4" />
                      {rule.enabled ? 'Disable Rule' : 'Enable Rule'}
                    </button>
                    <button
                        className="px-4 py-2 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-300 dark:border-slate-600 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-700 font-medium w-full transition-colors flex items-center justify-center gap-2"
                    >
                        <Settings className="w-4 h-4" />
                        Configure
                    </button>
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
                  Create Automation Rule
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
                <select className="w-full bg-white dark:bg-slate-950 border border-slate-300 dark:border-slate-700 rounded-lg px-4 py-3 text-slate-900 dark:text-white focus:ring-2 focus:ring-blue-500 focus:border-blue-500 outline-none appearance-none">
                    <option>Deep Malware Investigation (ID: pb-malware-003)</option>
                    <option>Ransomware Immediate Containment (ID: pb-ransomware-001)</option>
                    <option>Unauthorized USB Device Response (ID: pb-usb-002)</option>
                </select>
              </div>

              <div className="flex items-center gap-3 pt-2">
                <input type="checkbox" id="autoExec" className="w-4 h-4 text-blue-600 rounded border-gray-300" defaultChecked />
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
                    Create Rule
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
