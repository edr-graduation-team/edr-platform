import type { ProcessCategory } from './types';

// ────────────────────────────────────────────────────────────────────────────
// Process categorisation heuristics.
//
// This maps well-known Windows executable names to semantic categories.
// The classifier is intentionally broad — it runs client-side on
// aggregated process events and is designed for visibility, not blocking.
// ────────────────────────────────────────────────────────────────────────────

const SCRIPTING: readonly string[] = [
    'powershell.exe', 'pwsh.exe', 'cmd.exe', 'wscript.exe', 'cscript.exe',
    'mshta.exe', 'python.exe', 'python3.exe', 'pythonw.exe', 'node.exe',
    'ruby.exe', 'perl.exe', 'bash.exe', 'wsl.exe',
];

const ADMIN: readonly string[] = [
    'regedit.exe', 'mmc.exe', 'taskmgr.exe', 'systeminfo.exe', 'sc.exe',
    'net.exe', 'net1.exe', 'netstat.exe', 'ipconfig.exe', 'whoami.exe',
    'arp.exe', 'route.exe', 'nslookup.exe', 'ping.exe', 'tracert.exe',
    'certutil.exe', 'reg.exe', 'bcdedit.exe', 'diskpart.exe', 'fsutil.exe',
    'cipher.exe', 'icacls.exe', 'takeown.exe', 'shutdown.exe', 'gpupdate.exe',
    'schtasks.exe', 'wmic.exe', 'dism.exe', 'eventvwr.exe',
];

const REMOTE_ACCESS: readonly string[] = [
    'mstsc.exe', 'ssh.exe', 'putty.exe', 'psexec.exe', 'psexec64.exe',
    'teamviewer.exe', 'anydesk.exe', 'vnc.exe', 'winrm.exe',
    'ngrok.exe', 'chisel.exe',
];

const BROWSER: readonly string[] = [
    'chrome.exe', 'msedge.exe', 'firefox.exe', 'opera.exe', 'brave.exe',
    'iexplore.exe', 'msedgewebview2.exe',
];

const SECURITY: readonly string[] = [
    'sysmon.exe', 'sysmon64.exe', 'edr-agent.exe', 'trivy.exe',
    'smartscreen.exe', 'mrt.exe', 'msmpeng.exe', 'nissrv.exe',
    'malwarebytes.exe',
];

const SYSTEM_SERVICES: readonly string[] = [
    'svchost.exe', 'csrss.exe', 'lsass.exe', 'smss.exe', 'services.exe',
    'wininit.exe', 'winlogon.exe', 'dwm.exe', 'explorer.exe', 'spoolsv.exe',
    'searchindexer.exe', 'wmiprvse.exe', 'wmiadap.exe', 'taskhost.exe',
    'taskhostw.exe', 'runtimebroker.exe', 'fontdrvhost.exe', 'audiodg.exe',
    'conhost.exe', 'dllhost.exe', 'sihost.exe', 'ctfmon.exe',
    'mousocoreworker.exe', 'wermgr.exe', 'sppsvc.exe',
];

// Build a single lookup map for O(1) classification.
const LOOKUP = new Map<string, ProcessCategory>();
for (const n of SCRIPTING) LOOKUP.set(n, 'scripting');
for (const n of ADMIN) LOOKUP.set(n, 'admin');
for (const n of REMOTE_ACCESS) LOOKUP.set(n, 'remote_access');
for (const n of BROWSER) LOOKUP.set(n, 'browser');
for (const n of SECURITY) LOOKUP.set(n, 'security');
for (const n of SYSTEM_SERVICES) LOOKUP.set(n, 'system');

/**
 * Classify a process name into a semantic category.
 * Falls back to `'unknown'` for unrecognised executables.
 */
export function classifyProcess(processName: string): ProcessCategory {
    const lower = processName.toLowerCase();
    return LOOKUP.get(lower) ?? 'unknown';
}

/**
 * Returns `true` for processes that security teams typically want
 * highlighted (scripting, admin tools, remote access).
 */
export function isHighAttention(cat: ProcessCategory): boolean {
    return cat === 'scripting' || cat === 'admin' || cat === 'remote_access';
}
