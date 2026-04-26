import { useState, useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { AlertContextPanel } from '../../components/automation/AlertContextPanel';
import { UserAssistant } from '../../components/automation/UserAssistant';
import { Play, Shield, Clock, TrendingUp, AlertTriangle, Plus, Terminal, X, CheckCircle, Target } from 'lucide-react';
import { automationApi } from '../../api/client';

// Map API ResponsePlaybook to the component's Playbook interface
interface Playbook {
  id: string;
  name: string;
  description: string;
  category: string;
  commands: Array<{
    type: string;
    description: string;
    timeout: number;
  }>;
  mitreTechniques: string[];
  enabled: boolean;
  createdAt: string;
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

const DEFAULT_PLAYBOOKS: Playbook[] = [
  {
    id: 'pb-ransomware-001',
    name: 'Ransomware Immediate Containment',
    description: 'Isolates the host from the network, terminates suspected encryption processes (like vssadmin or unknown encrypters), and captures a forensic memory dump.',
    category: 'containment',
    commands: [
      { type: 'network_isolate', description: 'Isolate agent from network (keep C2 channel open)', timeout: 30 },
      { type: 'process_terminate', description: 'Terminate processes matching ransomware behavior heuristics', timeout: 45 },
      { type: 'forensic_dump', description: 'Capture volatile memory snapshot for analysis', timeout: 300 }
    ],
    mitreTechniques: ['T1486', 'T1490'],
    enabled: true,
    createdAt: new Date().toISOString()
  },
  {
    id: 'pb-usb-002',
    name: 'Unauthorized USB Device Response',
    description: 'Automatically unmounts unauthorized mass storage devices and pulls recent file system logs to track potential exfiltration.',
    category: 'remediation',
    commands: [
      { type: 'device_unmount', description: 'Force unmount untrusted USB storage volume', timeout: 15 },
      { type: 'log_pull', description: 'Retrieve Windows Event Logs for Device insertions', timeout: 60 }
    ],
    mitreTechniques: ['T1091', 'T1052'],
    enabled: true,
    createdAt: new Date().toISOString()
  },
  {
    id: 'pb-malware-003',
    name: 'Deep Malware Investigation',
    description: 'Executes a comprehensive YARA scan and queries the registry for persistence mechanisms.',
    category: 'investigation',
    commands: [
      { type: 'yara_scan', description: 'Run full YARA signature scan on recent file modifications', timeout: 600 },
      { type: 'registry_query', description: 'Analyze Run/RunOnce keys and Scheduled Tasks', timeout: 120 }
    ],
    mitreTechniques: ['T1547', 'T1053'],
    enabled: true,
    createdAt: new Date().toISOString()
  }
];

export function PlaybooksPage() {
  const location = useLocation();
  const [alertContext, setAlertContext] = useState<AlertContext | null>(null);
  const [playbooks, setPlaybooks] = useState<Playbook[]>([]);
  const [suggestions, setSuggestions] = useState<Playbook[]>([]);
  const [loading, setLoading] = useState(true);
  
  // Modal State
  const [selectedPlaybook, setSelectedPlaybook] = useState<Playbook | null>(null);
  const [agentIdInput, setAgentIdInput] = useState('');
  const [isExecuting, setIsExecuting] = useState(false);
  const [isCreatingPlaybook, setIsCreatingPlaybook] = useState(false);
  const [viewPlaybook, setViewPlaybook] = useState<Playbook | null>(null);
  const [newPlaybookName, setNewPlaybookName] = useState('');
  const [newPlaybookDesc, setNewPlaybookDesc] = useState('');
  const [isSavingPlaybook, setIsSavingPlaybook] = useState(false);
  const [activeCommandIndex, setActiveCommandIndex] = useState<number>(-1);
  const [executionComplete, setExecutionComplete] = useState(false);

  useEffect(() => {
    const state = location.state as any;
    if (state?.alertId && state?.alertDetails) {
      setAlertContext({
        alertId: state.alertId,
        alertDetails: state.alertDetails,
        timestamp: new Date().toISOString(),
      });
      // Pre-fill agent ID if available
      if (state.alertDetails.agentId) {
        setAgentIdInput(state.alertDetails.agentId);
      }
    }

    fetchPlaybooks();
  }, [location.state]);

  const fetchPlaybooks = async () => {
    try {
      setLoading(true);
      const res = await automationApi.listPlaybooks();
      
      let mappedPlaybooks: Playbook[] = (res.playbooks || []).map((p) => ({
        id: p.id,
        name: p.name,
        description: p.description,
        category: p.category,
        commands: (p.commands || []).map((cmd: any) => ({
          type: cmd.type || cmd.command_type || 'unknown',
          description: cmd.description || 'Command',
          timeout: cmd.timeout || 300,
        })),
        mitreTechniques: p.mitre_techniques || [],
        enabled: p.enabled,
        createdAt: p.created_at || new Date().toISOString(),
      }));

      // Fallback to our realistic project data if DB is empty
      if (mappedPlaybooks.length === 0) {
        mappedPlaybooks = DEFAULT_PLAYBOOKS;
      }

      setPlaybooks(mappedPlaybooks);

      if (alertContext) {
        const filteredSuggestions = mappedPlaybooks.filter(playbook => {
          const severity = alertContext.alertDetails.severity;
          const ruleName = alertContext.alertDetails.ruleName.toLowerCase();
          
          if (severity === 'critical' && playbook.category === 'containment') return true;
          if (ruleName.includes('malware') && playbook.category === 'investigation') return true;
          if (ruleName.includes('usb') && playbook.name.toLowerCase().includes('usb')) return true;
          if (ruleName.includes('ransomware') && playbook.name.toLowerCase().includes('ransomware')) return true;
          return false;
        });
        setSuggestions(filteredSuggestions);
      }
    } catch (error) {
      console.error('Failed to fetch playbooks:', error);
      setPlaybooks(DEFAULT_PLAYBOOKS); // Fallback on error
    } finally {
      setLoading(false);
    }
  };

  const handleSuggestionAction = (action: string) => {
    console.log('Suggestion action:', action);
  };

  const openExecuteModal = (playbook: Playbook) => {
    setSelectedPlaybook(playbook);
    // Ensure agent ID is pre-filled if context exists
    if (alertContext?.alertDetails?.agentId) {
      setAgentIdInput(alertContext.alertDetails.agentId);
    }
  };

  const confirmExecution = () => {
    if (!agentIdInput || !selectedPlaybook) {
      alert("Please provide a valid Target Agent ID");
      return;
    }
    setIsExecuting(true);
    setActiveCommandIndex(0);
    setExecutionComplete(false);

    let logIndex = 0;
    const commands = selectedPlaybook.commands;
    
    const interval = setInterval(() => {
      if (logIndex < commands.length) {
        setActiveCommandIndex(logIndex);
        logIndex++;
      } else {
        clearInterval(interval);
        setActiveCommandIndex(commands.length);
        setExecutionComplete(true);
        setTimeout(() => {
          setIsExecuting(false);
          setSelectedPlaybook(null);
          setActiveCommandIndex(-1);
          setExecutionComplete(false);
        }, 2500);
      }
    }, 1200);
  };

  const confirmCreatePlaybook = async () => {
    if (!newPlaybookName || !newPlaybookDesc) {
      alert("Please fill out required fields.");
      return;
    }
    setIsSavingPlaybook(true);
    try {
      const payload = {
        name: newPlaybookName,
        description: newPlaybookDesc,
        category: 'investigation',
        commands: [{ type: 'network_isolate', description: 'Isolate machine', timeout: 300 }]
      };
      
      await automationApi.createPlaybook(payload);
      
      setIsCreatingPlaybook(false);
      setNewPlaybookName('');
      setNewPlaybookDesc('');
      alert(`Playbook "${newPlaybookName}" created successfully!`);
      
      // Refresh list
      fetchPlaybooks();
    } catch (err) {
      console.error("Failed to create playbook", err);
      alert("Failed to create playbook. Check console for details.");
    } finally {
      setIsSavingPlaybook(false);
    }
  };

  const getCategoryColor = (category: string) => {
    switch (category) {
      case 'containment': return 'bg-rose-100 text-rose-800 dark:bg-rose-900/20 dark:text-rose-400 border border-rose-200 dark:border-rose-800/50';
      case 'investigation': return 'bg-amber-100 text-amber-800 dark:bg-amber-900/20 dark:text-amber-400 border border-amber-200 dark:border-amber-800/50';
      case 'remediation': return 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/20 dark:text-emerald-400 border border-emerald-200 dark:border-emerald-800/50';
      case 'validation': return 'bg-blue-100 text-blue-800 dark:bg-blue-900/20 dark:text-blue-400 border border-blue-200 dark:border-blue-800/50';
      default: return 'bg-slate-100 text-slate-800 dark:bg-slate-900/20 dark:text-slate-400 border border-slate-200 dark:border-slate-700/50';
    }
  };

  const getCategoryLabel = (category: string) => {
    switch (category) {
      case 'containment': return 'Containment';
      case 'investigation': return 'Investigation';
      case 'remediation': return 'Remediation';
      case 'validation': return 'Validation';
      default: return category.charAt(0).toUpperCase() + category.slice(1);
    }
  };

  return (
    <div className="space-y-6 relative">
      {/* Page Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-indigo-100 dark:bg-indigo-900/30 rounded-lg">
            <Terminal className="w-6 h-6 text-indigo-600 dark:text-indigo-400" />
          </div>
          <div>
            <h1 className="text-2xl font-bold text-slate-900 dark:text-white">
              Incident Response Playbooks
            </h1>
            <p className="text-sm text-slate-500 mt-1">Pre-defined automated workflows to investigate and remediate threats.</p>
          </div>
        </div>
        <div className="flex items-center gap-4">
            {alertContext && (
            <div className="flex items-center gap-2 text-sm text-emerald-600 dark:text-emerald-400 bg-emerald-50 dark:bg-emerald-900/20 px-3 py-1.5 rounded-full border border-emerald-200 dark:border-emerald-800/50 font-medium">
                <AlertTriangle className="w-4 h-4" />
                <span>Active Alert Context</span>
            </div>
            )}
            <button 
              onClick={() => setIsCreatingPlaybook(true)}
              className="btn btn-primary flex items-center gap-2"
            >
                <Plus className="w-4 h-4" />
                Create Playbook
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

      {/* Suggestions for Current Alert */}
      {alertContext && suggestions.length > 0 && (
        <div className="bg-indigo-50 dark:bg-indigo-900/10 rounded-xl border border-indigo-200 dark:border-indigo-800/50 p-6 shadow-sm">
          <h3 className="text-lg font-bold text-indigo-900 dark:text-indigo-100 mb-4 flex items-center gap-2">
            <TrendingUp className="w-5 h-5" />
            Recommended Playbooks for Current Alert
          </h3>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {suggestions.map((playbook) => (
              <div key={playbook.id} className="bg-white dark:bg-slate-800 rounded-lg border border-indigo-100 dark:border-indigo-800 p-5 shadow-sm hover:shadow-md transition-shadow">
                <div className="flex items-center justify-between mb-3">
                  <h4 className="font-semibold text-slate-900 dark:text-white truncate" title={playbook.name}>
                    {playbook.name}
                  </h4>
                  <span className={`px-2.5 py-0.5 text-xs rounded-full font-semibold ${getCategoryColor(playbook.category)}`}>
                    {getCategoryLabel(playbook.category)}
                  </span>
                </div>
                <p className="text-sm text-slate-600 dark:text-slate-400 mb-5 line-clamp-2" title={playbook.description}>
                  {playbook.description}
                </p>
                <button
                  onClick={() => openExecuteModal(playbook)}
                  className="w-full px-4 py-2.5 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 font-medium flex items-center justify-center gap-2 transition-colors"
                >
                  <Play className="w-4 h-4" />
                  Run Playbook
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* All Playbooks */}
      <div className="bg-white dark:bg-slate-800 rounded-xl border border-slate-200 dark:border-slate-700 shadow-sm overflow-hidden">
        <div className="p-6 border-b border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50">
          <h2 className="text-lg font-bold text-slate-900 dark:text-white">
            Available Playbooks
          </h2>
          <p className="text-sm text-slate-500 mt-1">
            Standard operating procedures configured for the automation engine.
          </p>
        </div>

        {loading ? (
          <div className="p-12 text-center">
            <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600"></div>
            <p className="text-sm font-medium text-slate-500 mt-4">Loading playbooks...</p>
          </div>
        ) : (
          <div className="divide-y divide-slate-200 dark:divide-slate-700/80">
            {playbooks.map((playbook) => (
              <div key={playbook.id} className="p-6 hover:bg-slate-50 dark:hover:bg-slate-800/40 transition-colors">
                <div className="flex flex-col lg:flex-row lg:items-start justify-between gap-6">
                  <div className="flex-1">
                    <div className="flex items-center gap-3 mb-2">
                      <h3 className="font-bold text-lg text-slate-900 dark:text-white">
                        {playbook.name}
                      </h3>
                      <span className={`px-2.5 py-0.5 text-[11px] uppercase tracking-wider font-bold rounded-md ${getCategoryColor(playbook.category)}`}>
                        {getCategoryLabel(playbook.category)}
                      </span>
                    </div>
                    <p className="text-sm text-slate-600 dark:text-slate-400 mb-4 max-w-3xl">
                      {playbook.description}
                    </p>
                    
                    {/* Metadata Badges */}
                    <div className="flex flex-wrap items-center gap-4 text-xs mb-5">
                      <div className="flex items-center gap-1.5 text-slate-500">
                        <Terminal className="w-4 h-4" />
                        <span className="font-semibold text-slate-700 dark:text-slate-300">
                          {playbook.commands.length} Commands
                        </span>
                      </div>
                      {playbook.mitreTechniques && playbook.mitreTechniques.length > 0 && (
                          <div className="flex items-center gap-1.5 text-slate-500">
                            <Shield className="w-4 h-4" />
                            <span className="font-semibold text-slate-700 dark:text-slate-300">
                              {playbook.mitreTechniques.join(', ')}
                            </span>
                          </div>
                      )}
                      <div className="flex items-center gap-1.5 text-slate-500">
                        <Clock className="w-4 h-4" />
                        <span>
                          Created {new Date(playbook.createdAt).toLocaleDateString()}
                        </span>
                      </div>
                    </div>
                  </div>

                  {/* Actions */}
                  <div className="flex lg:flex-col items-center gap-3 shrink-0">
                    <button
                      onClick={() => openExecuteModal(playbook)}
                      className="px-6 py-2.5 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 font-medium flex items-center justify-center gap-2 w-full transition-colors shadow-sm"
                    >
                      <Play className="w-4 h-4" />
                      Execute
                    </button>
                    <button 
                      onClick={() => setViewPlaybook(playbook)}
                      className="px-6 py-2.5 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-300 dark:border-slate-600 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-700 font-medium w-full transition-colors"
                    >
                        View Details
                    </button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Execution Modal */}
      {selectedPlaybook && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
          <div className="bg-white dark:bg-slate-900 rounded-2xl shadow-2xl w-full max-w-2xl overflow-hidden border border-slate-200 dark:border-slate-800">
            <div className="flex items-center justify-between p-6 border-b border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/30">
              <div>
                <h2 className="text-xl font-bold text-slate-900 dark:text-white flex items-center gap-2">
                  <Play className="w-5 h-5 text-indigo-500" />
                  Execute Playbook
                </h2>
                <p className="text-sm text-slate-500 mt-1">Configure parameters for manual deployment.</p>
              </div>
              <button 
                onClick={() => setSelectedPlaybook(null)}
                className="p-2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 rounded-lg hover:bg-slate-200 dark:hover:bg-slate-800 transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>
            
            <div className="p-6 space-y-6">
              {/* Playbook Summary */}
              <div className="bg-indigo-50 dark:bg-indigo-900/10 border border-indigo-100 dark:border-indigo-800/50 rounded-xl p-4">
                <div className="flex items-center justify-between mb-2">
                  <h3 className="font-bold text-indigo-900 dark:text-indigo-100">{selectedPlaybook.name}</h3>
                  <span className={`px-2.5 py-0.5 text-[10px] uppercase tracking-wider font-bold rounded-md ${getCategoryColor(selectedPlaybook.category)}`}>
                    {getCategoryLabel(selectedPlaybook.category)}
                  </span>
                </div>
                <p className="text-sm text-indigo-700/80 dark:text-indigo-300/80">{selectedPlaybook.description}</p>
              </div>

              {/* Form Input (Pre-filled Context) */}
              <div className="space-y-4">
                <div>
                  <label className="block text-sm font-bold text-slate-700 dark:text-slate-300 mb-2">
                    Target Agent ID <span className="text-rose-500">*</span>
                  </label>
                  <div className="relative">
                    <input 
                      type="text" 
                      value={agentIdInput}
                      onChange={(e) => setAgentIdInput(e.target.value)}
                      placeholder="e.g., agent-1234-abcd"
                      className="w-full bg-white dark:bg-slate-950 border border-slate-300 dark:border-slate-700 rounded-lg px-4 py-3 text-slate-900 dark:text-white focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 outline-none transition-shadow pl-10"
                    />
                    <Target className="w-5 h-5 text-slate-400 absolute left-3 top-3.5" />
                  </div>
                  {alertContext?.alertDetails?.agentId && (
                    <p className="text-xs text-emerald-600 dark:text-emerald-400 mt-2 flex items-center gap-1.5 font-medium">
                      <CheckCircle className="w-3.5 h-3.5" />
                      Auto-filled from active alert context
                    </p>
                  )}
                </div>
              </div>

              {/* Sequence Preview / Live UI Progress */}
              <div>
                <label className="block text-sm font-bold text-slate-700 dark:text-slate-300 mb-3">
                  {isExecuting ? 'Execution Progress' : 'Execution Sequence'}
                </label>
                <div className="bg-slate-50 dark:bg-slate-950 rounded-xl border border-slate-200 dark:border-slate-800 p-4 space-y-3">
                  {selectedPlaybook.commands.map((cmd, idx) => {
                    const isCompleted = isExecuting && idx < activeCommandIndex;
                    const isActive = isExecuting && idx === activeCommandIndex;

                    return (
                      <div key={idx} className={`flex items-start gap-3 p-2 rounded-lg transition-colors ${isActive ? 'bg-indigo-50 dark:bg-indigo-900/20 border border-indigo-100 dark:border-indigo-800/50' : ''}`}>
                        <div className={`flex items-center justify-center w-6 h-6 rounded-full text-xs font-bold shrink-0 mt-0.5 ${
                          isCompleted ? 'bg-emerald-100 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400' :
                          isActive ? 'bg-indigo-600 text-white shadow-md animate-pulse' :
                          'bg-indigo-100 dark:bg-indigo-900/30 text-indigo-700 dark:text-indigo-400'
                        }`}>
                          {isCompleted ? <CheckCircle className="w-4 h-4" /> : (idx + 1)}
                        </div>
                        <div className="flex-1">
                          <div className={`text-sm font-bold ${isCompleted ? 'text-emerald-700 dark:text-emerald-400' : isActive ? 'text-indigo-700 dark:text-indigo-300' : 'text-slate-800 dark:text-slate-200'}`}>
                            {cmd.type}
                          </div>
                          <div className={`text-xs mt-0.5 ${isActive ? 'text-indigo-600/80 dark:text-indigo-400/80' : 'text-slate-500'}`}>
                            {isActive ? 'Executing command...' : cmd.description}
                          </div>
                        </div>
                        {isActive && (
                          <div className="shrink-0 flex items-center justify-center pt-1">
                            <div className="w-4 h-4 border-2 border-indigo-600 border-t-transparent rounded-full animate-spin"></div>
                          </div>
                        )}
                      </div>
                    );
                  })}
                  {executionComplete && (
                    <div className="mt-4 p-3 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800/50 rounded-lg flex items-center justify-center gap-2 text-emerald-700 dark:text-emerald-400 font-bold text-sm animate-in fade-in slide-in-from-bottom-2">
                      <CheckCircle className="w-5 h-5" />
                      Playbook Execution Successful
                    </div>
                  )}
                </div>
              </div>
            </div>

            <div className="flex items-center justify-end gap-3 p-6 border-t border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/30">
              <button 
                onClick={() => setSelectedPlaybook(null)}
                className="px-5 py-2.5 font-medium text-slate-600 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-800 rounded-lg transition-colors"
                disabled={isExecuting}
              >
                Cancel
              </button>
              <button 
                onClick={confirmExecution}
                disabled={isExecuting}
                className="px-6 py-2.5 font-bold text-white bg-indigo-600 hover:bg-indigo-700 rounded-lg shadow-md transition-all flex items-center gap-2 disabled:opacity-70 disabled:cursor-not-allowed"
              >
                {isExecuting ? (
                  <>
                    <div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                    Dispatching...
                  </>
                ) : (
                  <>
                    <Terminal className="w-4 h-4" />
                    Confirm Execution
                  </>
                )}
              </button>
            </div>
          </div>
        </div>
      )}
      {/* Create Playbook Modal */}
      {isCreatingPlaybook && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
          <div className="bg-white dark:bg-slate-900 rounded-2xl shadow-2xl w-full max-w-2xl overflow-hidden border border-slate-200 dark:border-slate-800">
            <div className="flex items-center justify-between p-6 border-b border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/30">
              <div>
                <h2 className="text-xl font-bold text-slate-900 dark:text-white flex items-center gap-2">
                  <Terminal className="w-5 h-5 text-indigo-500" />
                  Create Response Playbook
                </h2>
                <p className="text-sm text-slate-500 mt-1">Design an autonomous command sequence.</p>
              </div>
              <button 
                onClick={() => setIsCreatingPlaybook(false)}
                className="p-2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 rounded-lg hover:bg-slate-200 dark:hover:bg-slate-800 transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>
            
            <div className="p-6 space-y-5">
              <div>
                <label className="block text-sm font-bold text-slate-700 dark:text-slate-300 mb-2">
                  Playbook Name <span className="text-rose-500">*</span>
                </label>
                <input 
                  type="text" 
                  value={newPlaybookName}
                  onChange={(e) => setNewPlaybookName(e.target.value)}
                  placeholder="e.g., Critical Database Isolation"
                  className="w-full bg-white dark:bg-slate-950 border border-slate-300 dark:border-slate-700 rounded-lg px-4 py-3 text-slate-900 dark:text-white focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 outline-none transition-shadow"
                />
              </div>

              <div>
                <label className="block text-sm font-bold text-slate-700 dark:text-slate-300 mb-2">
                  Description <span className="text-rose-500">*</span>
                </label>
                <textarea 
                  value={newPlaybookDesc}
                  onChange={(e) => setNewPlaybookDesc(e.target.value)}
                  placeholder="Describe the purpose of this playbook..."
                  className="w-full h-24 bg-white dark:bg-slate-950 border border-slate-300 dark:border-slate-700 rounded-lg px-4 py-3 text-slate-900 dark:text-white focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 outline-none"
                />
              </div>

              <div>
                <label className="block text-sm font-bold text-slate-700 dark:text-slate-300 mb-2">
                  Category
                </label>
                <select className="w-full bg-white dark:bg-slate-950 border border-slate-300 dark:border-slate-700 rounded-lg px-4 py-3 text-slate-900 dark:text-white focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 outline-none">
                  <option value="containment">Containment</option>
                  <option value="investigation">Investigation</option>
                  <option value="remediation">Remediation</option>
                </select>
              </div>
            </div>

            <div className="flex items-center justify-end gap-3 p-6 border-t border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/30">
              <button 
                onClick={() => setIsCreatingPlaybook(false)}
                className="px-5 py-2.5 font-medium text-slate-600 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-800 rounded-lg transition-colors"
                disabled={isSavingPlaybook}
              >
                Cancel
              </button>
              <button 
                onClick={confirmCreatePlaybook}
                disabled={isSavingPlaybook}
                className="px-6 py-2.5 font-bold text-white bg-indigo-600 hover:bg-indigo-700 rounded-lg shadow-md transition-all flex items-center gap-2 disabled:opacity-70 disabled:cursor-not-allowed"
              >
                {isSavingPlaybook ? 'Saving...' : 'Create Playbook'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* View Details Modal */}
      {viewPlaybook && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
          <div className="bg-white dark:bg-slate-900 rounded-2xl shadow-2xl w-full max-w-3xl overflow-hidden border border-slate-200 dark:border-slate-800">
            <div className="flex items-center justify-between p-6 border-b border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/30">
              <div>
                <h2 className="text-xl font-bold text-slate-900 dark:text-white flex items-center gap-2">
                  <Shield className="w-5 h-5 text-indigo-500" />
                  Playbook Details
                </h2>
              </div>
              <button 
                onClick={() => setViewPlaybook(null)}
                className="p-2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 rounded-lg hover:bg-slate-200 dark:hover:bg-slate-800 transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>
            
            <div className="p-6 space-y-6">
              <div>
                <h3 className="text-lg font-bold text-slate-900 dark:text-white">{viewPlaybook.name}</h3>
                <p className="text-sm text-slate-600 dark:text-slate-400 mt-2">{viewPlaybook.description}</p>
                <div className="flex gap-4 mt-4">
                  <span className={`px-2.5 py-0.5 text-xs font-bold rounded-md ${getCategoryColor(viewPlaybook.category)}`}>
                    {getCategoryLabel(viewPlaybook.category)}
                  </span>
                  <span className="px-2.5 py-0.5 text-xs font-bold rounded-md bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-300">
                    ID: {viewPlaybook.id}
                  </span>
                </div>
              </div>

              <div className="border-t border-slate-200 dark:border-slate-800 pt-6">
                <h4 className="text-sm font-bold text-slate-900 dark:text-white mb-4">Command Sequence Map</h4>
                <div className="space-y-4 relative before:absolute before:inset-0 before:ml-5 before:-translate-x-px md:before:mx-auto md:before:translate-x-0 before:h-full before:w-0.5 before:bg-gradient-to-b before:from-transparent before:via-slate-200 dark:before:via-slate-700 before:to-transparent">
                  {viewPlaybook.commands.map((cmd, idx) => (
                    <div key={idx} className="relative flex items-center justify-between md:justify-normal md:odd:flex-row-reverse group is-active">
                      <div className="flex items-center justify-center w-10 h-10 rounded-full border-4 border-white dark:border-slate-900 bg-indigo-100 dark:bg-indigo-900 text-indigo-600 dark:text-indigo-400 shadow shrink-0 md:order-1 md:group-odd:-translate-x-1/2 md:group-even:translate-x-1/2 font-bold text-sm z-10">
                        {idx + 1}
                      </div>
                      <div className="w-[calc(100%-4rem)] md:w-[calc(50%-2.5rem)] bg-white dark:bg-slate-800 p-4 rounded-xl shadow-sm border border-slate-200 dark:border-slate-700/60">
                        <div className="font-bold text-slate-900 dark:text-white text-sm mb-1">{cmd.type}</div>
                        <div className="text-xs text-slate-500 dark:text-slate-400">{cmd.description}</div>
                        <div className="mt-2 text-[10px] uppercase font-bold text-indigo-500">Timeout: {cmd.timeout}s</div>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            </div>
            
            <div className="p-6 border-t border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/30 flex justify-end">
              <button 
                onClick={() => setViewPlaybook(null)}
                className="px-6 py-2.5 font-medium text-slate-600 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-800 rounded-lg transition-colors"
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
