import { useState, useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { AlertContextPanel } from '../../components/automation/AlertContextPanel';
import { UserAssistant } from '../../components/automation/UserAssistant';
import { Play, Shield, Clock, TrendingUp, AlertTriangle, Plus, Terminal, X, CheckCircle, Target, Trash2 } from 'lucide-react';
import { automationApi, agentsApi } from '../../api/client';

// Parameters required by each command type.
// All fields set to required:false - every command now carries a DB-level default
// param pre-filled in openExecuteModal. The * only shows when no DB default exists.
const COMMAND_PARAMS: Record<string, Array<{ key: string; label: string; placeholder: string; required: boolean; defaultValue?: string }>> = {
  terminate_process: [{ key: 'process_name', label: 'Process Name / PID', placeholder: 'e.g., vssadmin.exe  or  PID:1234', required: true  }],
  quarantine_file:   [{ key: 'file_path',    label: 'File Path',    placeholder: 'e.g., C:\\Windows\\Temp\\malware.exe', required: true  }],
  run_cmd:           [{ key: 'cmd',          label: 'Command',      placeholder: 'e.g., powershell -Command "Get-Volume | Where-Object {$_.DriveType -eq \'Removable\'} | Dismount-Volume -Confirm:$false"',    required: false }],
  collect_logs:      [{ key: 'log_types',    label: 'Log Types',    placeholder: 'System,Security,Application',        required: false }],
  scan_file:         [{ key: 'file_path',    label: 'Path to Scan', placeholder: 'e.g., C:\\Windows\\Temp',            required: true  }],
  update_signatures: [{ key: 'url',          label: 'Signature URL',placeholder: 'https://...',                        required: false }],
  collect_forensics: [
    { key: 'event_types', label: 'Event Types', placeholder: 'process,file,network,registry', required: false },
    { key: 'max_events',  label: 'Max Events',  placeholder: '1000',                          required: false },
  ],
  filesystem_timeline: [{ key: 'window_hours', label: 'Time Window (hours)', placeholder: '24', required: false }],
  // Legacy type aliases
  process_terminate: [{ key: 'process_name', label: 'Process Name / PID', placeholder: 'e.g., vssadmin.exe  or  PID:1234', required: true  }],
  yara_scan:         [{ key: 'file_path',    label: 'Path to Scan', placeholder: 'C:\\Windows\\Temp',      required: true  }],
};

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
    params?: Record<string, string>;
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
    event_data?: any;
  };
  timestamp: string;
}


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
  const [executionError, setExecutionError] = useState<string | null>(null);
  const [cmdParams, setCmdParams] = useState<Record<string, string>>({});
  const [newPlaybookCategory, setNewPlaybookCategory] = useState('investigation');
  const [agents, setAgents] = useState<{ id: string; hostname: string }[]>([]);

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
    fetchAgents();
  }, [location.state]);

  const fetchAgents = async () => {
    try {
      const res = await agentsApi.list({ limit: 100 });
      if (res && res.data) {
        setAgents(res.data.map((a: any) => ({ id: a.id, hostname: a.hostname || a.id })));
      }
    } catch (error) {
      console.error('Failed to fetch agents:', error);
    }
  };

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
          params: cmd.params || {},
        })),
        mitreTechniques: p.mitre_techniques || [],
        enabled: p.enabled,
        createdAt: p.created_at || new Date().toISOString(),
      }));

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
    } finally {
      setLoading(false);
    }
  };

  const handleSuggestionAction = (action: string) => {
    console.log('Suggestion action:', action);
  };

  const handleDeletePlaybook = async (playbookId: string) => {
    if (!window.confirm("Are you sure you want to delete this playbook?")) return;
    try {
      if (playbookId) {
        await automationApi.deletePlaybook(playbookId);
      }
      setPlaybooks(prev => prev.filter(p => p.id !== playbookId));
      if (suggestions.some(p => p.id === playbookId)) {
        setSuggestions(prev => prev.filter(p => p.id !== playbookId));
      }
    } catch (error) {
      console.error("Failed to delete playbook:", error);
      alert("Failed to delete playbook.");
    }
  };

  const openExecuteModal = (playbook: Playbook) => {
    setSelectedPlaybook(playbook);
    if (alertContext?.alertDetails?.agentId) setAgentIdInput(alertContext.alertDetails.agentId);

    // ?? Extract relevant fields from the alert's event_data ?????????????????
    const ed = alertContext?.alertDetails?.event_data || {};

    // Process info from alert (used for terminate_process)
    const alertProcessName =
      (ed.Image ? ed.Image.split('\\').pop() : '') ||
      ed.ProcessName ||
      ed.OriginalFileName ||
      '';
    const alertPid = ed.ProcessId ? String(ed.ProcessId) : '';
    // If we have a PID, prefer "PID:1234" so the agent can match by PID too.
    const alertProcessValue = alertProcessName || (alertPid ? `PID:${alertPid}` : '');

    // File path from alert (used for quarantine_file / scan_file)
    const alertFilePath =
      ed.TargetFilename ||
      ed.file_path ||
      ed.FilePath ||
      ed.Image ||   // fallback: the process image that triggered the alert
      '';

    const defaults: Record<string, string> = {};

    playbook.commands.forEach((cmd, idx) => {
      const defs = COMMAND_PARAMS[cmd.type] || [];
      defs.forEach(pd => {
        // Step 1: Start with DB default (skip ${template} vars)
        const dbVal = String(cmd.params?.[pd.key] || '');
        defaults[`${idx}_${pd.key}`] = dbVal.startsWith('${') ? '' : dbVal;

        // Step 2: For user-required fields, override with alert context when available
        if (pd.required) {
          if ((cmd.type === 'terminate_process' || cmd.type === 'process_terminate') && pd.key === 'process_name') {
            if (alertProcessValue) defaults[`${idx}_process_name`] = alertProcessValue;
          }
          if ((cmd.type === 'quarantine_file') && pd.key === 'file_path') {
            if (alertFilePath) defaults[`${idx}_file_path`] = alertFilePath;
          }
          if ((cmd.type === 'scan_file' || cmd.type === 'yara_scan') && pd.key === 'file_path') {
            if (alertFilePath) defaults[`${idx}_file_path`] = alertFilePath;
          }
        }
      });
    });

    setCmdParams(defaults);
  };

  const confirmExecution = async () => {
    if (!agentIdInput || !selectedPlaybook) {
      alert("Please provide a valid Target Agent ID");
      return;
    }

    // Pre-flight: check all required fields are filled
    const missing: string[] = [];
    selectedPlaybook.commands.forEach((cmd, idx) => {
      const defs = COMMAND_PARAMS[cmd.type] || [];
      defs.forEach(pd => {
        if (pd.required && !cmdParams[`${idx}_${pd.key}`]?.trim()) {
          missing.push(`Step ${idx + 1} (${cmd.type}) - ${pd.label}`);
        }
      });
    });
    if (missing.length > 0) {
      alert(`Please fill in the required fields before executing:\n\n- ${missing.join("\n- ")}`);
      return;
    }
    setIsExecuting(true);
    setActiveCommandIndex(0);
    setExecutionComplete(false);
    setExecutionError(null);

    const commands = selectedPlaybook.commands;

    for (let logIndex = 0; logIndex < commands.length; logIndex++) {
      setActiveCommandIndex(logIndex);
      const cmd = commands[logIndex];
      let mappedType = cmd.type;
      let params: Record<string, string> = {};

      // Build params from user inputs
      const p = (key: string) => cmdParams[`${logIndex}_${key}`] || '';

      switch (cmd.type) {
        // ── Real DB command types ───────────────────────────────────────
        // For each type: use the user-edited value from state (p(key)),
        // then fall back to the DB-stored default (cmd.params), then a
        // hardcoded safe fallback. This means every command runs without
        // the analyst typing anything when defaults are pre-loaded from DB.
        case 'terminate_process':  params = { process_name: p('process_name') || String(cmd.params?.process_name || 'suspicious.exe'), kill_tree: 'true' }; break;
        case 'quarantine_file':    params = { file_path: p('file_path') || String(cmd.params?.file_path || 'C:\\Windows\\Temp') }; break;
        case 'run_cmd':            params = { cmd: p('cmd') || String(cmd.params?.cmd || '') }; break;
        case 'collect_logs':       params = { log_types: p('log_types') || String(cmd.params?.log_types || 'System,Security') }; break;
        case 'scan_file':          params = { file_path: p('file_path') || String(cmd.params?.file_path || 'C:\\Windows\\Temp') }; break;
        case 'collect_forensics':  params = { event_types: p('event_types') || String(cmd.params?.event_types || 'process,file,network,registry'), max_events: p('max_events') || String(cmd.params?.max_events || '1000') }; break;
        case 'update_signatures':  params = { url: p('url') || String(cmd.params?.url || '') }; break;
        case 'filesystem_timeline':params = { window_hours: p('window_hours') || String(cmd.params?.window_hours || '24') }; break;
        case 'isolate_network':
        case 'unisolate_network':
        case 'memory_dump':
        case 'process_tree_snapshot':
        case 'persistence_scan':
        case 'agent_integrity_check':
        case 'lsass_access_audit':
        case 'network_last_seen':  params = {}; break;
        // ── Legacy type aliases ─────────────────────────────────────────
        case 'network_isolate':   mappedType = 'isolate_network'; break;
        case 'process_terminate': mappedType = 'terminate_process'; params = { process_name: p('process_name') || String(cmd.params?.process_name || 'suspicious.exe'), kill_tree: 'true' }; break;
        case 'forensic_dump':     mappedType = 'collect_forensics'; params = { event_types: 'process,file,network,registry', max_events: '1000' }; break;
        case 'device_unmount':    mappedType = 'run_cmd'; params = { cmd: 'powershell -Command "Get-Volume | Where-Object {$_.DriveType -eq \'Removable\'} | Dismount-Volume -Confirm:$false"', from_playbook: 'true' }; break;
        case 'log_pull':          mappedType = 'collect_logs'; params = { log_types: 'System,Security' }; break;
        case 'yara_scan':         mappedType = 'scan_file'; params = { file_path: p('file_path') || String(cmd.params?.file_path || 'C:\\Windows\\Temp') }; break;
        case 'registry_query':    mappedType = 'run_cmd'; params = { cmd: 'reg query HKLM\\Software\\Microsoft\\Windows\\CurrentVersion\\Run' }; break;
      }

      // Inject playbook context marker so the agent routes through the
      // elevated playbookAllowedCommands whitelist, not the interactive one.
      params.from_playbook = "true";

      try {
        await agentsApi.executeCommand(agentIdInput, {
          command_type: mappedType as any,
          parameters: params,
          timeout: cmd.timeout || 300
        });

        // Add a slight artificial delay for UI feel so the user sees the progress step if it was super fast
        await new Promise(resolve => setTimeout(resolve, 800));
      } catch (err: any) {
        console.error("Command execution failed:", err);
        const errMsg = err.response?.data?.message || err.message || "Unknown error";
        setExecutionError(`Failed at step ${logIndex + 1} (${cmd.type}): ${errMsg}. The agent may be offline or unreachable.`);
        setIsExecuting(false);
        return; // Halt execution of remaining steps
      }
    }

    // If we made it here, all commands executed successfully
    setActiveCommandIndex(commands.length);
    setExecutionComplete(true);
    setTimeout(() => {
      setIsExecuting(false);
      setSelectedPlaybook(null);
      setActiveCommandIndex(-1);
      setExecutionComplete(false);
    }, 3000);
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
        category: newPlaybookCategory,
        commands: [{ type: 'isolate_network', description: 'Isolate machine from network', timeout: 300 }]
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
                    <div className="flex w-full gap-2">
                        <button
                          onClick={() => setViewPlaybook(playbook)}
                          className="flex-1 px-4 py-2 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-300 dark:border-slate-600 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-700 font-medium transition-colors text-sm"
                        >
                          View Details
                        </button>
                        <button
                          onClick={() => handleDeletePlaybook(playbook.id)}
                          className="px-3 py-2 bg-white dark:bg-slate-800 text-rose-600 dark:text-rose-400 border border-slate-300 dark:border-slate-600 rounded-lg hover:bg-rose-50 dark:hover:bg-rose-900/20 font-medium transition-colors flex items-center justify-center"
                          title="Delete Playbook"
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

      {/* Execution Modal */}
      {selectedPlaybook && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
          <div className="bg-white dark:bg-slate-900 rounded-2xl shadow-2xl w-full max-w-2xl flex flex-col border border-slate-200 dark:border-slate-800" style={{ maxHeight: '92vh' }}>
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

            {/* Scrollable body */}
            <div className="overflow-y-auto flex-1 p-6 space-y-6">
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

              {/* Agent selector + dynamic per-command parameter inputs */}
              <div className="space-y-4">
                <div>
                  <label className="block text-sm font-bold text-slate-700 dark:text-slate-300 mb-2">
                    Target Agent <span className="text-rose-500">*</span>
                  </label>
                  <div className="relative">
                    <select
                      value={agentIdInput}
                      onChange={(e) => setAgentIdInput(e.target.value)}
                      className="w-full bg-white dark:bg-slate-950 border border-slate-300 dark:border-slate-700 rounded-lg px-4 py-3 text-slate-900 dark:text-white focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 outline-none appearance-none transition-shadow pl-10"
                    >
                      <option value="" disabled>Select Target Agent</option>
                      {agents.map(a => (
                        <option key={a.id} value={a.id}>{a.hostname} ({a.id})</option>
                      ))}
                      {agentIdInput && !agents.some(a => a.id === agentIdInput) && (
                        <option value={agentIdInput}>{agentIdInput} (From Alert Context)</option>
                      )}
                    </select>
                    <Target className="w-5 h-5 text-slate-400 absolute left-3 top-3.5" />
                  </div>
                  {alertContext?.alertDetails?.agentId && (
                    <p className="text-xs text-emerald-600 dark:text-emerald-400 mt-2 flex items-center gap-1.5 font-medium">
                      <CheckCircle className="w-3.5 h-3.5" />
                      Auto-filled from active alert context
                    </p>
                  )}
                </div>

                {/* Dynamic parameter inputs per command step */}
                {(() => {
                  const inputs: React.ReactNode[] = [];
                  selectedPlaybook.commands.forEach((cmd, idx) => {
                    const defs = COMMAND_PARAMS[cmd.type] || [];
                    defs.forEach(pd => {
                      const currentVal  = cmdParams[`${idx}_${pd.key}`] || '';
                      const dbDefault   = String(cmd.params?.[pd.key] || '');
                      const hasDbValue  = !!dbDefault && !dbDefault.startsWith('${');

                      // "From Alert": required field, filled from alert context (not from DB)
                      const isFromAlert  = pd.required && !!currentVal && currentVal !== dbDefault;
                      // "Auto-filled": non-required field that has a DB default (read-only)
                      const isAutoFilled = !pd.required && hasDbValue && currentVal === dbDefault;
                      // "Empty required": required field with nothing entered yet
                      const isEmpty      = pd.required && !currentVal.trim();

                      inputs.push(
                        <div key={`${idx}-${pd.key}`}>
                          <label className="block text-sm font-bold text-slate-700 dark:text-slate-300 mb-1.5">
                            Step {idx + 1} &mdash; {pd.label}
                            {pd.required && <span className="text-rose-500 ml-1">*</span>}
                            <span className="text-xs font-normal text-slate-400 ml-2">({cmd.type})</span>
                            {isFromAlert && (
                              <span className="ml-2 px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wide bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400 rounded">
                                From Alert
                              </span>
                            )}
                            {isAutoFilled && (
                              <span className="ml-2 px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wide bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400 rounded">
                                Auto-filled
                              </span>
                            )}
                          </label>
                          <div className="relative">
                            <input
                              type="text"
                              value={currentVal}
                              onChange={e => setCmdParams(prev => ({ ...prev, [`${idx}_${pd.key}`]: e.target.value }))}
                              placeholder={pd.placeholder}
                              readOnly={isAutoFilled}
                              className={`w-full bg-white dark:bg-slate-950 border rounded-lg px-4 py-2.5 text-slate-900 dark:text-white focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 outline-none text-sm pl-10 transition-colors ${
                                isAutoFilled
                                  ? "border-emerald-300 dark:border-emerald-700 bg-emerald-50 dark:bg-emerald-900/10 text-slate-600 dark:text-slate-400 cursor-default"
                                  : isEmpty
                                    ? "border-rose-300 dark:border-rose-600 focus:ring-rose-400"
                                    : isFromAlert
                                      ? "border-amber-300 dark:border-amber-600"
                                      : "border-slate-300 dark:border-slate-700"
                              }`}
                            />
                            <Terminal className="w-4 h-4 text-slate-400 absolute left-3 top-3" />
                          </div>
                          {isAutoFilled && (
                            <p className="text-xs text-emerald-600 dark:text-emerald-400 mt-1 flex items-center gap-1">
                              <CheckCircle className="w-3 h-3" />
                              Pre-loaded from playbook - click to override.
                            </p>
                          )}
                          {isFromAlert && (
                            <p className="text-xs text-amber-600 dark:text-amber-400 mt-1 flex items-center gap-1">
                              <AlertTriangle className="w-3 h-3" />
                              Auto-filled from alert context - review and confirm.
                            </p>
                          )}
                          {isEmpty && (
                            <p className="text-xs text-rose-600 dark:text-rose-400 mt-1 flex items-center gap-1">
                              <AlertTriangle className="w-3 h-3" />
                              Required - enter a value to proceed.
                            </p>
                          )}
                        </div>
                      );
                    });
                  });
                  return inputs.length > 0 ? (
                    <div className="space-y-4">
                      <p className="text-xs font-semibold text-slate-500 uppercase tracking-wider">Command Parameters</p>
                      {inputs}
                    </div>
                  ) : null;
                })()}
              </div>

              {/* Sequence Preview / Live UI Progress */}
              <div>
                <label className="block text-sm font-bold text-slate-700 dark:text-slate-300 mb-3">
                  {isExecuting ? 'Execution Progress' : 'Execution Sequence'}
                </label>
                <div className="bg-slate-50 dark:bg-slate-950 rounded-xl border border-slate-200 dark:border-slate-800 p-4 space-y-3">
                  {selectedPlaybook.commands.map((cmd, idx) => {
                    const isCompleted = (isExecuting || executionError) && idx < activeCommandIndex;
                    const isActive = isExecuting && idx === activeCommandIndex && !executionError;
                    const isFailed = executionError && idx === activeCommandIndex;

                    return (
                      <div key={idx} className={`flex items-start gap-3 p-2 rounded-lg transition-colors ${isActive ? 'bg-indigo-50 dark:bg-indigo-900/20 border border-indigo-100 dark:border-indigo-800/50' : ''}`}>
                        <div className={`flex items-center justify-center w-6 h-6 rounded-full text-xs font-bold shrink-0 mt-0.5 ${isFailed ? 'bg-rose-100 text-rose-600 dark:bg-rose-900/30 dark:text-rose-400 border border-rose-200 dark:border-rose-800' :
                            isCompleted ? 'bg-emerald-100 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400' :
                              isActive ? 'bg-indigo-600 text-white shadow-md animate-pulse' :
                                'bg-indigo-100 dark:bg-indigo-900/30 text-indigo-700 dark:text-indigo-400'
                          }`}>
                          {isFailed ? <X className="w-4 h-4" /> : isCompleted ? <CheckCircle className="w-4 h-4" /> : (idx + 1)}
                        </div>
                        <div className="flex-1">
                          <div className={`text-sm font-bold ${isFailed ? 'text-rose-700 dark:text-rose-400' : isCompleted ? 'text-emerald-700 dark:text-emerald-400' : isActive ? 'text-indigo-700 dark:text-indigo-300' : 'text-slate-800 dark:text-slate-200'}`}>
                            {cmd.type}
                          </div>
                          <div className={`text-xs mt-0.5 ${isFailed ? 'text-rose-600/80 dark:text-rose-400/80 font-medium' : isActive ? 'text-indigo-600/80 dark:text-indigo-400/80' : 'text-slate-500'}`}>
                            {isFailed ? 'Execution Failed' : isActive ? 'Executing command...' : cmd.description}
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
                  {executionComplete && !executionError && (
                    <div className="mt-4 p-3 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800/50 rounded-lg flex items-center justify-center gap-2 text-emerald-700 dark:text-emerald-400 font-bold text-sm animate-in fade-in slide-in-from-bottom-2">
                      <CheckCircle className="w-5 h-5" />
                      Playbook Execution Successful
                    </div>
                  )}
                  {executionError && (
                    <div className="mt-4 p-4 bg-rose-50 dark:bg-rose-900/20 border border-rose-200 dark:border-rose-800/50 rounded-lg flex flex-col gap-2 text-rose-700 dark:text-rose-400 text-sm animate-in fade-in slide-in-from-bottom-2">
                      <div className="flex items-center gap-2 font-bold">
                        <AlertTriangle className="w-5 h-5" />
                        Execution Aborted
                      </div>
                      <div className="pl-7 opacity-90">{executionError}</div>
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
                <select
                  value={newPlaybookCategory}
                  onChange={e => setNewPlaybookCategory(e.target.value)}
                  className="w-full bg-white dark:bg-slate-950 border border-slate-300 dark:border-slate-700 rounded-lg px-4 py-3 text-slate-900 dark:text-white focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 outline-none"
                >
                  <option value="containment">Containment</option>
                  <option value="investigation">Investigation</option>
                  <option value="remediation">Remediation</option>
                  <option value="validation">Validation</option>
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
