package scoring

import (
	"strings"

	infracache "github.com/edr-platform/sigma-engine/internal/infrastructure/cache"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
)

// =============================================================================
// Suspicion Matrix -- Parent->Child Process Lineage Risk Table
// =============================================================================
//
// The SuspicionMatrix encodes the curated "threat intelligence" of this scorer:
// which parent->child process relationships are suspicious and by how much.
//
// Design rationale:
//   - Hardcoded (not runtime-configurable) for explainability and academic defence.
//     Every entry can be cited to a real attack technique from the MITRE ATT&CK
//     framework or a known malware family.
//   - Case-insensitive matching on normalised lowercase process names.
//   - Wildcard parent "*" matches any parent -- used for processes that are
//     suspicious regardless of their spawner (e.g., mshta.exe commanding anything).
//   - Uses additive tiers (critical=40, high=30, medium=20, low=10) so the
//     lineage bonus is meaningful but cannot dominate the score alone.

// suspicionLevel classifies the severity of a parent->child pair.
type suspicionLevel string

const (
	suspicionCritical suspicionLevel = "critical" // confirmed high-value LOLBin chain
	suspicionHigh     suspicionLevel = "high"     // strong indicator of abuse
	suspicionMedium   suspicionLevel = "medium"   // moderate concern
	suspicionLow      suspicionLevel = "low"      // worth noting, not alarming
	suspicionNone     suspicionLevel = "none"     // no entry found
)

// suspicionEntry holds the score bonus and human-readable rationale for
// a specific parent->child pair.
type suspicionEntry struct {
	Level     suspicionLevel
	Bonus     int
	Rationale string
}

// matrixKey is the lookup key: "parent_name|child_name" (both lowercase).
// A wildcard parent uses "*" to match any parent.
type matrixKey struct {
	parent string
	child  string
}

// SuspicionMatrix is a pre-built lookup table for parent->child suspicion.
type SuspicionMatrix struct {
	// exact maps "parent|child" -> suspicionEntry for precise lookups.
	exact map[matrixKey]suspicionEntry
	// wildcardChild maps "child_name" -> suspicionEntry for entries where
	// the child is always suspicious regardless of its parent (parent = "*").
	wildcardChild map[string]suspicionEntry
}

// NewSuspicionMatrix constructs and pre-populates the suspicion matrix.
// All entries are cited to real MITRE ATT&CK techniques or CVEs.
func NewSuspicionMatrix() *SuspicionMatrix {
	m := &SuspicionMatrix{
		exact:         make(map[matrixKey]suspicionEntry),
		wildcardChild: make(map[string]suspicionEntry),
	}

	// =========================================================================
	// CRITICAL (+40) -- Almost exclusively attacker activity
	// =========================================================================

	// -------------------------------------------------------------------------
	// Microsoft Office -- Macro / Phishing Initial Access (T1566.001)
	// -------------------------------------------------------------------------

	m.addExact("winword.exe", "powershell.exe", suspicionCritical, 40,
		"T1566.001/T1059.001: Word macro spawning PowerShell -- classic macro-based initial access")
	m.addExact("winword.exe", "cmd.exe", suspicionCritical, 40,
		"T1566.001: Word macro spawning cmd.exe via AutoOpen/AutoClose VBA")
	m.addExact("winword.exe", "wscript.exe", suspicionCritical, 40,
		"T1566.001/T1059.005: Word macro dropping and executing a VBScript payload")
	m.addExact("winword.exe", "cscript.exe", suspicionCritical, 40,
		"T1566.001/T1059.005: Word macro spawning CScript for VBScript execution")
	m.addExact("winword.exe", "mshta.exe", suspicionCritical, 40,
		"T1218.005: Word macro spawning mshta for HTA payload execution")
	m.addExact("winword.exe", "certutil.exe", suspicionCritical, 40,
		"T1105: Word macro using certutil as a download cradle")
	m.addExact("winword.exe", "bitsadmin.exe", suspicionCritical, 40,
		"T1197: Word macro invoking bitsadmin for background file download")
	m.addExact("winword.exe", "rundll32.exe", suspicionCritical, 40,
		"T1218.011: Word macro spawning rundll32 to side-load a malicious DLL")
	m.addExact("winword.exe", "regsvr32.exe", suspicionCritical, 40,
		"T1218.010: Word macro Squiblydoo via regsvr32 COM scriptlet")
	m.addExact("winword.exe", "cmstp.exe", suspicionCritical, 40,
		"T1218.003: Word macro spawning cmstp for UAC bypass and code execution")
	m.addExact("winword.exe", "installutil.exe", suspicionCritical, 40,
		"T1218.004: Word macro spawning InstallUtil for .NET whitelisted execution")
	m.addExact("winword.exe", "msbuild.exe", suspicionCritical, 40,
		"T1127.001: Word macro invoking MSBuild for inline .NET payload execution")
	m.addExact("winword.exe", "forfiles.exe", suspicionCritical, 40,
		"T1202: Word macro spawning forfiles for indirect command execution")
	m.addExact("winword.exe", "pcalua.exe", suspicionCritical, 40,
		"T1202: Word macro using Program Compatibility Assistant for indirect execution")
	m.addExact("winword.exe", "hh.exe", suspicionCritical, 40,
		"T1218.001: Word macro invoking HTML Help for CHM-based code execution")
	m.addExact("winword.exe", "csc.exe", suspicionCritical, 40,
		"T1027.004: Word macro invoking csc.exe -- compile-and-execute C# payload")

	m.addExact("excel.exe", "powershell.exe", suspicionCritical, 40,
		"T1566.001/T1059.001: Excel macro spawning PowerShell -- common initial access")
	m.addExact("excel.exe", "cmd.exe", suspicionCritical, 40,
		"T1566.001: Excel macro spawning cmd.exe via XLM/VBA macro")
	m.addExact("excel.exe", "wscript.exe", suspicionCritical, 40,
		"T1566.001: Excel macro executing VBScript via WScript")
	m.addExact("excel.exe", "cscript.exe", suspicionCritical, 40,
		"T1566.001: Excel macro executing script via CScript")
	m.addExact("excel.exe", "mshta.exe", suspicionCritical, 40,
		"T1218.005: Excel macro spawning mshta for HTA execution")
	m.addExact("excel.exe", "certutil.exe", suspicionCritical, 40,
		"T1105: Excel macro download cradle via certutil")
	m.addExact("excel.exe", "rundll32.exe", suspicionCritical, 40,
		"T1218.011: Excel DDE/macro spawning rundll32")
	m.addExact("excel.exe", "regsvr32.exe", suspicionCritical, 40,
		"T1218.010: Excel macro Squiblydoo via regsvr32")
	m.addExact("excel.exe", "msbuild.exe", suspicionCritical, 40,
		"T1127.001: Excel macro invoking MSBuild for .NET inline task execution")
	m.addExact("excel.exe", "bitsadmin.exe", suspicionCritical, 40,
		"T1197: Excel macro using bitsadmin for BITS download")
	m.addExact("excel.exe", "csc.exe", suspicionCritical, 40,
		"T1027.004: Excel macro invoking csc.exe to compile and execute payload")

	m.addExact("powerpnt.exe", "powershell.exe", suspicionCritical, 40,
		"T1566.001: PowerPoint macro spawning PowerShell -- phishing via PPTM file")
	m.addExact("powerpnt.exe", "cmd.exe", suspicionCritical, 40,
		"T1566.001: PowerPoint macro spawning cmd.exe")
	m.addExact("powerpnt.exe", "wscript.exe", suspicionCritical, 40,
		"T1566.001: PowerPoint macro executing VBScript")
	m.addExact("powerpnt.exe", "mshta.exe", suspicionCritical, 40,
		"T1218.005: PowerPoint spawning mshta for HTA code execution")
	m.addExact("powerpnt.exe", "certutil.exe", suspicionCritical, 40,
		"T1105: PowerPoint macro using certutil as download cradle")

	// eqnedt32.exe -- Microsoft Equation Editor (CVE-2017-11882 / CVE-2018-0802)
	m.addExact("eqnedt32.exe", "powershell.exe", suspicionCritical, 40,
		"T1203/CVE-2017-11882: Equation Editor RCE spawning PowerShell")
	m.addExact("eqnedt32.exe", "cmd.exe", suspicionCritical, 40,
		"T1203/CVE-2017-11882: Equation Editor RCE spawning cmd.exe")
	m.addExact("eqnedt32.exe", "wscript.exe", suspicionCritical, 40,
		"T1203/CVE-2017-11882: Equation Editor RCE dropping VBScript")
	m.addExact("eqnedt32.exe", "mshta.exe", suspicionCritical, 40,
		"T1203/CVE-2017-11882: Equation Editor RCE executing HTA payload")
	m.addExact("eqnedt32.exe", "certutil.exe", suspicionCritical, 40,
		"T1105/CVE-2017-11882: Equation Editor spawning certutil download cradle")

	// Outlook phishing
	m.addExact("outlet.exe", "powershell.exe", suspicionCritical, 40,
		"T1566.001: Mail client (outlet.exe) spawning PowerShell")
	m.addExact("outlook.exe", "cmd.exe", suspicionCritical, 40,
		"T1566.001: Outlook spawning cmd.exe -- malicious email attachment")
	m.addExact("outlook.exe", "powershell.exe", suspicionCritical, 40,
		"T1566.001: Outlook spawning PowerShell -- phishing payload execution")
	m.addExact("outlook.exe", "wscript.exe", suspicionCritical, 40,
		"T1566.001: Outlook spawning WScript -- phishing VBScript execution")
	m.addExact("outlook.exe", "mshta.exe", suspicionCritical, 40,
		"T1218.005/T1566.001: Outlook spawning mshta for HTA payload via email")
	m.addExact("outlook.exe", "certutil.exe", suspicionCritical, 40,
		"T1105/T1566.001: Outlook spawning certutil download cradle")
	m.addExact("outlook.exe", "rundll32.exe", suspicionCritical, 40,
		"T1218.011/T1566.001: Outlook spawning rundll32 for DLL-based payload")
	m.addExact("outlook.exe", "cscript.exe", suspicionCritical, 40,
		"T1566.001: Outlook spawning CScript for VBScript execution")

	// -------------------------------------------------------------------------
	// Browsers -- Drive-by Download / Exploitation (T1203, T1189)
	// -------------------------------------------------------------------------

	m.addExact("chrome.exe", "cmd.exe", suspicionCritical, 40,
		"T1189/T1203: Chrome spawning cmd.exe -- browser exploitation or malicious extension")
	m.addExact("chrome.exe", "powershell.exe", suspicionCritical, 40,
		"T1189: Chrome spawning PowerShell -- drive-by exploit payload")
	m.addExact("chrome.exe", "wscript.exe", suspicionCritical, 40,
		"T1189: Chrome spawning WScript -- JavaScript-dropped VBScript")
	m.addExact("chrome.exe", "certutil.exe", suspicionCritical, 40,
		"T1105/T1189: Chrome spawning certutil -- browser exploit download cradle")

	m.addExact("msedge.exe", "cmd.exe", suspicionCritical, 40,
		"T1189: Microsoft Edge spawning cmd.exe -- browser exploit")
	m.addExact("msedge.exe", "powershell.exe", suspicionCritical, 40,
		"T1189: Microsoft Edge spawning PowerShell -- drive-by exploit execution")
	m.addExact("msedge.exe", "wscript.exe", suspicionCritical, 40,
		"T1189: Microsoft Edge spawning WScript -- JavaScript-dropped VBScript")

	m.addExact("firefox.exe", "cmd.exe", suspicionCritical, 40,
		"T1189: Firefox spawning cmd.exe -- browser exploitation")
	m.addExact("firefox.exe", "powershell.exe", suspicionCritical, 40,
		"T1189: Firefox spawning PowerShell -- drive-by exploit payload")
	m.addExact("firefox.exe", "wscript.exe", suspicionCritical, 40,
		"T1189: Firefox spawning WScript -- JavaScript-dropped VBScript")

	m.addExact("iexplore.exe", "powershell.exe", suspicionCritical, 40,
		"T1189/T1203: IE spawning PowerShell -- common CVE exploitation vector")
	m.addExact("iexplore.exe", "cmd.exe", suspicionCritical, 40,
		"T1189/T1203: IE spawning cmd.exe -- exploit-based execution")
	m.addExact("iexplore.exe", "wscript.exe", suspicionCritical, 40,
		"T1189: IE dropping and executing VBScript payload")
	m.addExact("iexplore.exe", "mshta.exe", suspicionCritical, 40,
		"T1218.005/T1189: IE spawning mshta -- exploit-triggered HTA execution")

	// -------------------------------------------------------------------------
	// PDF Readers -- Acrobat / Reader Exploits (T1203)
	// -------------------------------------------------------------------------

	m.addExact("acrord32.exe", "powershell.exe", suspicionCritical, 40,
		"T1203: Acrobat Reader spawning PowerShell -- PDF exploit execution")
	m.addExact("acrord32.exe", "cmd.exe", suspicionCritical, 40,
		"T1203: Acrobat Reader spawning cmd.exe -- malicious PDF payload")
	m.addExact("acrord32.exe", "wscript.exe", suspicionCritical, 40,
		"T1203: Acrobat Reader spawning WScript -- embedded JS dropping VBS")
	m.addExact("acrord32.exe", "certutil.exe", suspicionCritical, 40,
		"T1105/T1203: Acrobat Reader using certutil as download cradle")
	m.addExact("acrord32.exe", "mshta.exe", suspicionCritical, 40,
		"T1218.005/T1203: Acrobat Reader spawning mshta for HTA execution")

	m.addExact("acrobat.exe", "powershell.exe", suspicionCritical, 40,
		"T1203: Acrobat Pro spawning PowerShell -- exploit-based code execution")
	m.addExact("acrobat.exe", "cmd.exe", suspicionCritical, 40,
		"T1203: Acrobat Pro spawning cmd.exe -- malicious PDF execution")
	m.addExact("acrobat.exe", "wscript.exe", suspicionCritical, 40,
		"T1203: Acrobat Pro spawning WScript -- embedded JavaScript payload")

	// -------------------------------------------------------------------------
	// Core System Processes -- Anomalous Spawning (T1055, T1543, T1547)
	// -------------------------------------------------------------------------

	m.addExact("lsass.exe", "cmd.exe", suspicionCritical, 40,
		"T1003.001/T1055: LSASS spawning cmd -- process injection or credential dump")
	m.addExact("lsass.exe", "powershell.exe", suspicionCritical, 40,
		"T1003.001/T1055: LSASS spawning PowerShell -- indicative of process injection")
	m.addExact("lsass.exe", "wscript.exe", suspicionCritical, 40,
		"T1055: LSASS spawning WScript -- very anomalous injection indicator")

	m.addExact("smss.exe", "cmd.exe", suspicionCritical, 40,
		"T1055: Session Manager spawning cmd -- process hollowing or trojan smss")
	m.addExact("smss.exe", "powershell.exe", suspicionCritical, 40,
		"T1055: Session Manager spawning PowerShell -- anomalous injection")

	m.addExact("wininit.exe", "cmd.exe", suspicionCritical, 40,
		"T1055: Windows Initialization spawning cmd -- process injection or trojan")
	m.addExact("wininit.exe", "powershell.exe", suspicionCritical, 40,
		"T1055: Windows Initialization spawning PowerShell -- very anomalous")

	m.addExact("winlogon.exe", "cmd.exe", suspicionCritical, 40,
		"T1055/T1546.002: Winlogon spawning cmd -- Winlogon hijack or injection")
	m.addExact("winlogon.exe", "powershell.exe", suspicionCritical, 40,
		"T1055/T1546.002: Winlogon spawning PowerShell -- credential interception")
	m.addExact("winlogon.exe", "wscript.exe", suspicionCritical, 40,
		"T1055: Winlogon spawning WScript -- very anomalous execution chain")

	m.addExact("spoolsv.exe", "cmd.exe", suspicionCritical, 40,
		"T1547.012/CVE-2021-34527: Print Spooler spawning cmd -- PrintNightmare")
	m.addExact("spoolsv.exe", "powershell.exe", suspicionCritical, 40,
		"T1547.012/CVE-2021-34527: Print Spooler spawning PowerShell -- PrintNightmare LPE")
	m.addExact("spoolsv.exe", "rundll32.exe", suspicionCritical, 40,
		"T1547.012/CVE-2021-34527: Print Spooler spawning rundll32 -- PrintNightmare DLL load")

	// -------------------------------------------------------------------------
	// Web Servers -- Web Shell Exploitation (T1505.003, T1190)
	// -------------------------------------------------------------------------

	m.addExact("w3wp.exe", "cmd.exe", suspicionCritical, 40,
		"T1505.003/T1190: IIS Worker spawning cmd.exe -- web shell command execution")
	m.addExact("w3wp.exe", "powershell.exe", suspicionCritical, 40,
		"T1505.003/T1190: IIS Worker spawning PowerShell -- web shell pivot")
	m.addExact("w3wp.exe", "wscript.exe", suspicionCritical, 40,
		"T1505.003: IIS Worker spawning WScript -- ASPX web shell script execution")
	m.addExact("w3wp.exe", "cscript.exe", suspicionCritical, 40,
		"T1505.003: IIS Worker spawning CScript -- ASPX web shell execution")
	m.addExact("w3wp.exe", "whoami.exe", suspicionCritical, 40,
		"T1505.003/T1033: IIS Worker spawning whoami -- web shell reconnaissance")
	m.addExact("w3wp.exe", "net.exe", suspicionCritical, 40,
		"T1505.003/T1087: IIS Worker spawning net.exe -- post-exploitation enumeration")
	m.addExact("w3wp.exe", "certutil.exe", suspicionCritical, 40,
		"T1105/T1505.003: IIS Worker using certutil download cradle")
	m.addExact("w3wp.exe", "bitsadmin.exe", suspicionCritical, 40,
		"T1197/T1505.003: IIS Worker using BITS download -- web shell payload retrieval")
	m.addExact("w3wp.exe", "nltest.exe", suspicionCritical, 40,
		"T1505.003/T1482: IIS Worker spawning nltest -- domain trust enum from web shell")
	m.addExact("w3wp.exe", "mshta.exe", suspicionCritical, 40,
		"T1218.005/T1505.003: IIS Worker spawning mshta -- web shell HTA execution")
	m.addExact("w3wp.exe", "rundll32.exe", suspicionCritical, 40,
		"T1218.011/T1505.003: IIS Worker spawning rundll32 -- web shell DLL payload")

	m.addExact("httpd.exe", "cmd.exe", suspicionCritical, 40,
		"T1505.003: Apache httpd spawning cmd.exe -- PHP/CGI web shell execution")
	m.addExact("httpd.exe", "powershell.exe", suspicionCritical, 40,
		"T1505.003: Apache httpd spawning PowerShell -- web shell pivot")
	m.addExact("httpd.exe", "whoami.exe", suspicionCritical, 40,
		"T1505.003/T1033: Apache httpd spawning whoami -- web shell recon")

	m.addExact("nginx.exe", "cmd.exe", suspicionCritical, 40,
		"T1505.003: Nginx spawning cmd.exe -- web shell exploitation")
	m.addExact("nginx.exe", "powershell.exe", suspicionCritical, 40,
		"T1505.003: Nginx spawning PowerShell -- web shell pivot")

	m.addExact("php-cgi.exe", "cmd.exe", suspicionCritical, 40,
		"T1505.003: PHP CGI spawning cmd.exe -- PHP web shell execution")
	m.addExact("php-cgi.exe", "powershell.exe", suspicionCritical, 40,
		"T1505.003: PHP CGI spawning PowerShell -- PHP web shell pivot")
	m.addExact("php-cgi.exe", "whoami.exe", suspicionCritical, 40,
		"T1505.003/T1033: PHP CGI spawning whoami -- web shell user discovery")

	// =========================================================================
	// HIGH (+30) -- Strong indicator of abuse
	// =========================================================================

	// -------------------------------------------------------------------------
	// PowerShell spawning suspicious tools
	// -------------------------------------------------------------------------

	m.addExact("powershell.exe", "certutil.exe", suspicionHigh, 30,
		"T1105: PowerShell download cradle using certutil for payload retrieval")
	m.addExact("powershell.exe", "bitsadmin.exe", suspicionHigh, 30,
		"T1197: BITS abuse -- PowerShell spawning bitsadmin for background download")
	m.addExact("powershell.exe", "wmic.exe", suspicionHigh, 30,
		"T1047: PowerShell spawning WMIC -- WMI lateral movement or persistence")
	m.addExact("powershell.exe", "schtasks.exe", suspicionHigh, 30,
		"T1053.005: Scheduled task creation via PowerShell for persistence")
	m.addExact("powershell.exe", "reg.exe", suspicionHigh, 30,
		"T1112: Registry modification via PowerShell for persistence or evasion")
	m.addExact("powershell.exe", "csc.exe", suspicionHigh, 30,
		"T1027.004: PowerShell invoking csc.exe -- compile-and-execute C# payload")
	m.addExact("powershell.exe", "msbuild.exe", suspicionHigh, 30,
		"T1127.001: PowerShell spawning MSBuild for inline .NET task execution")
	m.addExact("powershell.exe", "installutil.exe", suspicionHigh, 30,
		"T1218.004: PowerShell spawning InstallUtil for AppLocker bypass")
	m.addExact("powershell.exe", "regsvr32.exe", suspicionHigh, 30,
		"T1218.010: PowerShell invoking regsvr32 Squiblydoo bypass")
	m.addExact("powershell.exe", "rundll32.exe", suspicionHigh, 30,
		"T1218.011: PowerShell spawning rundll32 for DLL side-loading or COM abuse")
	m.addExact("powershell.exe", "mshta.exe", suspicionHigh, 30,
		"T1218.005: PowerShell spawning mshta for HTA execution")
	m.addExact("powershell.exe", "forfiles.exe", suspicionHigh, 30,
		"T1202: PowerShell using forfiles for indirect command execution")
	m.addExact("powershell.exe", "vssadmin.exe", suspicionHigh, 30,
		"T1490/T1003.003: PowerShell invoking vssadmin -- shadow delete or NTDS copy")
	m.addExact("powershell.exe", "procdump.exe", suspicionHigh, 30,
		"T1003.001: PowerShell spawning procdump for LSASS memory dump")
	m.addExact("powershell.exe", "ntdsutil.exe", suspicionHigh, 30,
		"T1003.003: PowerShell invoking ntdsutil for NTDS credential extraction")
	m.addExact("powershell.exe", "wscript.exe", suspicionHigh, 30,
		"T1059.005: PowerShell spawning WScript -- stage 2 script execution")
	m.addExact("powershell.exe", "esentutl.exe", suspicionHigh, 30,
		"T1003.003: PowerShell invoking esentutl -- NTDS.dit ESE database extraction")
	m.addExact("powershell.exe", "pcalua.exe", suspicionHigh, 30,
		"T1202: PowerShell using Program Compat. Assistant for indirect execution")

	// -------------------------------------------------------------------------
	// cmd.exe spawning suspicious tools
	// -------------------------------------------------------------------------

	m.addExact("cmd.exe", "powershell.exe", suspicionHigh, 30,
		"T1059.001: cmd.exe spawning PowerShell -- staged payload execution")
	m.addExact("cmd.exe", "certutil.exe", suspicionHigh, 30,
		"T1105: cmd.exe using certutil as download cradle")
	m.addExact("cmd.exe", "bitsadmin.exe", suspicionHigh, 30,
		"T1197: BITS abuse launched from cmd.exe")
	m.addExact("cmd.exe", "wmic.exe", suspicionHigh, 30,
		"T1047: WMI execution via cmd for lateral movement or persistence")
	m.addExact("cmd.exe", "mshta.exe", suspicionHigh, 30,
		"T1218.005: cmd.exe spawning mshta for HTA payload execution")
	m.addExact("cmd.exe", "regsvr32.exe", suspicionHigh, 30,
		"T1218.010: cmd.exe invoking regsvr32 Squiblydoo bypass")
	m.addExact("cmd.exe", "procdump.exe", suspicionHigh, 30,
		"T1003.001: cmd.exe spawning procdump for LSASS credential dump")
	m.addExact("cmd.exe", "vssadmin.exe", suspicionHigh, 30,
		"T1490: cmd.exe invoking vssadmin to delete shadow copies (ransomware)")
	m.addExact("cmd.exe", "ntdsutil.exe", suspicionHigh, 30,
		"T1003.003: cmd.exe invoking ntdsutil for NTDS database extraction")
	m.addExact("cmd.exe", "forfiles.exe", suspicionHigh, 30,
		"T1202: cmd.exe using forfiles for indirect command execution")
	m.addExact("cmd.exe", "msbuild.exe", suspicionHigh, 30,
		"T1127.001: cmd.exe invoking MSBuild for inline task code execution")
	m.addExact("cmd.exe", "installutil.exe", suspicionHigh, 30,
		"T1218.004: cmd.exe spawning InstallUtil for .NET whitelisted execution bypass")
	m.addExact("cmd.exe", "csc.exe", suspicionHigh, 30,
		"T1027.004: cmd.exe invoking csc.exe -- compile-and-execute C# payload")
	m.addExact("cmd.exe", "esentutl.exe", suspicionHigh, 30,
		"T1003.003: cmd.exe invoking esentutl -- NTDS.dit / ESE database extraction")
	m.addExact("cmd.exe", "pcalua.exe", suspicionHigh, 30,
		"T1202: cmd.exe using pcalua for indirect Program Compat. execution")
	m.addExact("cmd.exe", "diskshadow.exe", suspicionHigh, 30,
		"T1003.003/T1218: cmd.exe using diskshadow for VSS-based NTDS extraction")

	// -------------------------------------------------------------------------
	// svchost / services -- core service anomalies (T1543.003, T1059)
	// -------------------------------------------------------------------------

	m.addExact("svchost.exe", "cmd.exe", suspicionHigh, 30,
		"T1059: svchost spawning cmd -- malicious service, DLL injection, or hollowing")
	m.addExact("svchost.exe", "powershell.exe", suspicionHigh, 30,
		"T1059.001: svchost spawning PowerShell -- service-based persistence")
	m.addExact("svchost.exe", "wscript.exe", suspicionHigh, 30,
		"T1059.005: svchost spawning WScript -- service-based VBScript execution")
	m.addExact("svchost.exe", "cscript.exe", suspicionHigh, 30,
		"T1059.005: svchost spawning CScript -- malicious service executing script")
	m.addExact("svchost.exe", "certutil.exe", suspicionHigh, 30,
		"T1105: svchost using certutil download cradle -- malicious service payload")
	m.addExact("svchost.exe", "mshta.exe", suspicionHigh, 30,
		"T1218.005: svchost spawning mshta -- HTA execution from service context")
	m.addExact("svchost.exe", "rundll32.exe", suspicionHigh, 30,
		"T1218.011: svchost spawning rundll32 -- DLL side-load or COM abuse from service")

	m.addExact("services.exe", "cmd.exe", suspicionHigh, 30,
		"T1543.003: Malicious Windows service spawning cmd.exe")
	m.addExact("services.exe", "powershell.exe", suspicionHigh, 30,
		"T1543.003: Malicious Windows service spawning PowerShell")
	m.addExact("services.exe", "wscript.exe", suspicionHigh, 30,
		"T1543.003: Malicious service spawning WScript for VBScript execution")
	m.addExact("services.exe", "mshta.exe", suspicionHigh, 30,
		"T1543.003/T1218.005: Malicious service spawning mshta for HTA execution")

	// -------------------------------------------------------------------------
	// taskhostw -- Scheduled Task Host anomalies (T1053.005)
	// -------------------------------------------------------------------------

	m.addExact("taskhostw.exe", "cmd.exe", suspicionHigh, 30,
		"T1053.005: Task Host spawning cmd -- scheduled task persistence execution")
	m.addExact("taskhostw.exe", "powershell.exe", suspicionHigh, 30,
		"T1053.005: Task Host spawning PowerShell -- scheduled task persistence")
	m.addExact("taskhostw.exe", "wscript.exe", suspicionHigh, 30,
		"T1053.005/T1059.005: Task Host spawning WScript -- scheduled VBScript payload")
	m.addExact("taskhostw.exe", "certutil.exe", suspicionHigh, 30,
		"T1105/T1053.005: Task Host using certutil download cradle")
	m.addExact("taskhostw.exe", "mshta.exe", suspicionHigh, 30,
		"T1218.005/T1053.005: Task Host spawning mshta -- scheduled HTA payload")

	// -------------------------------------------------------------------------
	// Scripting Engines -- WScript / CScript / hh.exe
	// -------------------------------------------------------------------------

	m.addExact("wscript.exe", "powershell.exe", suspicionHigh, 30,
		"T1059.001: WScript spawning PowerShell -- staged script execution")
	m.addExact("wscript.exe", "cmd.exe", suspicionHigh, 30,
		"T1059.003: WScript spawning cmd.exe -- VBScript shell execution")
	m.addExact("wscript.exe", "certutil.exe", suspicionHigh, 30,
		"T1105: WScript using certutil as download cradle")
	m.addExact("wscript.exe", "bitsadmin.exe", suspicionHigh, 30,
		"T1197: WScript using BITS for background download")
	m.addExact("wscript.exe", "mshta.exe", suspicionHigh, 30,
		"T1218.005: WScript spawning mshta -- stage 2 HTA execution")
	m.addExact("wscript.exe", "regsvr32.exe", suspicionHigh, 30,
		"T1218.010: WScript spawning regsvr32 Squiblydoo")
	m.addExact("wscript.exe", "csc.exe", suspicionHigh, 30,
		"T1027.004: WScript invoking csc.exe for compile-and-execute C# payload")

	m.addExact("cscript.exe", "powershell.exe", suspicionHigh, 30,
		"T1059.001: CScript spawning PowerShell -- stage 2 execution")
	m.addExact("cscript.exe", "cmd.exe", suspicionHigh, 30,
		"T1059.003: CScript spawning cmd.exe -- script shell execution")
	m.addExact("cscript.exe", "certutil.exe", suspicionHigh, 30,
		"T1105: CScript using certutil as download cradle")
	m.addExact("cscript.exe", "mshta.exe", suspicionHigh, 30,
		"T1218.005: CScript spawning mshta for HTA stage 2")
	m.addExact("cscript.exe", "regsvr32.exe", suspicionHigh, 30,
		"T1218.010: CScript spawning regsvr32 Squiblydoo")
	m.addExact("cscript.exe", "bitsadmin.exe", suspicionHigh, 30,
		"T1197: CScript using BITS for background download")

	m.addExact("hh.exe", "cmd.exe", suspicionHigh, 30,
		"T1218.001: HTML Help spawning cmd.exe -- CHM-based code execution")
	m.addExact("hh.exe", "powershell.exe", suspicionHigh, 30,
		"T1218.001: HTML Help spawning PowerShell -- CHM payload execution")
	m.addExact("hh.exe", "wscript.exe", suspicionHigh, 30,
		"T1218.001: HTML Help spawning WScript -- CHM-embedded VBScript")
	m.addExact("hh.exe", "mshta.exe", suspicionHigh, 30,
		"T1218.001/T1218.005: HTML Help spawning mshta -- dual LOLBin chain")
	m.addExact("hh.exe", "certutil.exe", suspicionHigh, 30,
		"T1105/T1218.001: HTML Help using certutil download cradle")

	// -------------------------------------------------------------------------
	// rundll32 / COM-based execution chains
	// -------------------------------------------------------------------------

	m.addExact("rundll32.exe", "cmd.exe", suspicionHigh, 30,
		"T1218.011: rundll32 spawning cmd -- LOLBin evasion technique")
	m.addExact("rundll32.exe", "powershell.exe", suspicionHigh, 30,
		"T1218.011: rundll32 spawning PowerShell -- fileless execution via COM/DLL")
	m.addExact("rundll32.exe", "wscript.exe", suspicionHigh, 30,
		"T1218.011: rundll32 spawning WScript -- COM scriptlet stage 2")
	m.addExact("rundll32.exe", "certutil.exe", suspicionHigh, 30,
		"T1105/T1218.011: rundll32 using certutil as download cradle")
	m.addExact("rundll32.exe", "mshta.exe", suspicionHigh, 30,
		"T1218.011/T1218.005: rundll32 spawning mshta -- dual LOLBin chain")
	m.addExact("rundll32.exe", "csc.exe", suspicionHigh, 30,
		"T1027.004/T1218.011: rundll32 spawning csc.exe for C# compile-and-execute")

	// -------------------------------------------------------------------------
	// WMI / WMIC lateral movement and execution (T1047)
	// -------------------------------------------------------------------------

	m.addExact("wmic.exe", "powershell.exe", suspicionHigh, 30,
		"T1047: WMIC spawning PowerShell -- WMI-based lateral movement or persistence")
	m.addExact("wmic.exe", "cmd.exe", suspicionHigh, 30,
		"T1047: WMIC spawning cmd -- WMI command execution")
	m.addExact("wmic.exe", "certutil.exe", suspicionHigh, 30,
		"T1105/T1047: WMIC using certutil as download cradle")
	m.addExact("wmic.exe", "mshta.exe", suspicionHigh, 30,
		"T1218.005/T1047: WMIC spawning mshta for HTA execution")

	m.addExact("wmiprvse.exe", "powershell.exe", suspicionHigh, 30,
		"T1047: WMI Provider Host spawning PowerShell -- remote WMI lateral movement")
	m.addExact("wmiprvse.exe", "cmd.exe", suspicionHigh, 30,
		"T1047: WMI Provider Host spawning cmd -- remote WMI command execution")
	m.addExact("wmiprvse.exe", "certutil.exe", suspicionHigh, 30,
		"T1105/T1047: WMI Provider Host using certutil download cradle")
	m.addExact("wmiprvse.exe", "regsvr32.exe", suspicionHigh, 30,
		"T1218.010/T1047: WMI Provider Host spawning regsvr32 Squiblydoo")
	m.addExact("wmiprvse.exe", "mshta.exe", suspicionHigh, 30,
		"T1218.005/T1047: WMI Provider Host spawning mshta for HTA execution")
	m.addExact("wmiprvse.exe", "wscript.exe", suspicionHigh, 30,
		"T1059.005/T1047: WMI Provider Host spawning WScript -- lateral movement VBS")

	// -------------------------------------------------------------------------
	// LOLBin chaining: pcalua / forfiles / mmc / at / cmstp
	// -------------------------------------------------------------------------

	m.addExact("pcalua.exe", "cmd.exe", suspicionHigh, 30,
		"T1202: pcalua.exe spawning cmd -- Program Compat. Assistant indirect execution")
	m.addExact("pcalua.exe", "powershell.exe", suspicionHigh, 30,
		"T1202: pcalua.exe spawning PowerShell -- AppLocker bypass via compat layer")
	m.addExact("forfiles.exe", "cmd.exe", suspicionHigh, 30,
		"T1202: forfiles spawning cmd -- indirect command execution / AppLocker bypass")
	m.addExact("forfiles.exe", "powershell.exe", suspicionHigh, 30,
		"T1202: forfiles spawning PowerShell -- indirect LOLBin code execution")
	m.addExact("mmc.exe", "cmd.exe", suspicionMedium, 20,
		"T1218: MMC spawning cmd -- mmc.exe LOLBin abuse for AppLocker bypass")
	m.addExact("mmc.exe", "powershell.exe", suspicionMedium, 20,
		"T1218: MMC spawning PowerShell -- LOLBin code execution chain")
	m.addExact("at.exe", "cmd.exe", suspicionMedium, 20,
		"T1053.002: Legacy Task Scheduler spawning cmd -- scheduled persistence")
	m.addExact("at.exe", "powershell.exe", suspicionMedium, 20,
		"T1053.002: Legacy Task Scheduler spawning PowerShell -- scheduled persistence")
	m.addExact("cmstp.exe", "cmd.exe", suspicionCritical, 40,
		"T1218.003: cmstp spawning cmd -- CMSTP INF file code execution")
	m.addExact("cmstp.exe", "powershell.exe", suspicionCritical, 40,
		"T1218.003: cmstp spawning PowerShell -- UAC bypass auto-elevation chain")
	m.addExact("msdeploy.exe", "cmd.exe", suspicionHigh, 30,
		"T1218: MsDeploy spawning cmd -- signed binary used for AppLocker bypass")
	m.addExact("msconfig.exe", "cmd.exe", suspicionMedium, 20,
		"T1218: MSConfig spawning cmd.exe -- LSP/startup config abuse")

	// -------------------------------------------------------------------------
	// Web server recon tools (High because parent context is already critical)
	// -------------------------------------------------------------------------

	m.addExact("w3wp.exe", "ping.exe", suspicionHigh, 30,
		"T1505.003/T1016: IIS Worker spawning ping -- web shell network recon")
	m.addExact("w3wp.exe", "ipconfig.exe", suspicionHigh, 30,
		"T1505.003/T1016: IIS Worker spawning ipconfig -- web shell network discovery")
	m.addExact("w3wp.exe", "systeminfo.exe", suspicionHigh, 30,
		"T1505.003/T1082: IIS Worker spawning systeminfo -- web shell host discovery")
	m.addExact("w3wp.exe", "tasklist.exe", suspicionHigh, 30,
		"T1505.003/T1057: IIS Worker spawning tasklist -- web shell process recon")

	// =========================================================================
	// MEDIUM (+20) -- Moderate concern; context determines severity
	// =========================================================================

	// -------------------------------------------------------------------------
	// Explorer / User interaction anomalies
	// -------------------------------------------------------------------------

	m.addExact("explorer.exe", "powershell.exe", suspicionMedium, 20,
		"T1059.001: Explorer spawning PowerShell -- suspicious without user context")
	m.addExact("explorer.exe", "cmd.exe", suspicionMedium, 15,
		"Shell opened from Explorer -- context-dependent; lower risk")
	m.addExact("explorer.exe", "mshta.exe", suspicionMedium, 20,
		"T1218.005: Explorer spawning mshta -- likely phishing HTA payload")
	m.addExact("explorer.exe", "certutil.exe", suspicionMedium, 20,
		"T1105: Explorer spawning certutil -- user-initiated or phishing download")
	m.addExact("wermgr.exe", "cmd.exe", suspicionMedium, 20,
		"T1036: Masquerading via Windows Error Reporting manager spawning cmd")

	// -------------------------------------------------------------------------
	// Post-exploitation reconnaissance
	// -------------------------------------------------------------------------

	m.addExact("powershell.exe", "net.exe", suspicionMedium, 20,
		"T1087: Domain or local account enumeration via net.exe from PowerShell")
	m.addExact("powershell.exe", "whoami.exe", suspicionMedium, 20,
		"T1033: System owner/user discovery from PowerShell")
	m.addExact("powershell.exe", "ipconfig.exe", suspicionMedium, 15,
		"T1016: Network configuration discovery from PowerShell")
	m.addExact("powershell.exe", "nslookup.exe", suspicionMedium, 15,
		"T1018: Remote system discovery via DNS lookup from PowerShell")
	m.addExact("powershell.exe", "nltest.exe", suspicionMedium, 20,
		"T1482: Domain trust discovery via nltest from PowerShell")
	m.addExact("powershell.exe", "arp.exe", suspicionMedium, 15,
		"T1016: ARP cache enumeration -- network mapping from PowerShell")
	m.addExact("powershell.exe", "tasklist.exe", suspicionMedium, 15,
		"T1057: Process discovery from PowerShell -- pre-attack reconnaissance")
	m.addExact("powershell.exe", "systeminfo.exe", suspicionMedium, 20,
		"T1082: System information discovery from PowerShell")
	m.addExact("powershell.exe", "netstat.exe", suspicionMedium, 15,
		"T1049: Network connections enumeration from PowerShell")

	m.addExact("cmd.exe", "net.exe", suspicionMedium, 20,
		"T1087: Account/share enumeration via net commands from cmd.exe")
	m.addExact("cmd.exe", "whoami.exe", suspicionMedium, 20,
		"T1033: User identity discovery from cmd.exe -- post-exploitation recon")
	m.addExact("cmd.exe", "nltest.exe", suspicionMedium, 20,
		"T1482: Domain trust discovery via nltest from cmd.exe")
	m.addExact("cmd.exe", "systeminfo.exe", suspicionMedium, 20,
		"T1082: System information discovery from cmd.exe")
	m.addExact("cmd.exe", "ipconfig.exe", suspicionMedium, 15,
		"T1016: Network config discovery from cmd.exe")
	m.addExact("cmd.exe", "tasklist.exe", suspicionMedium, 15,
		"T1057: Process discovery from cmd.exe -- pre-attack reconnaissance")
	m.addExact("cmd.exe", "netstat.exe", suspicionMedium, 15,
		"T1049: Active network connections enumeration from cmd.exe")
	m.addExact("cmd.exe", "arp.exe", suspicionMedium, 15,
		"T1016: ARP cache enumeration from cmd.exe")

	// -------------------------------------------------------------------------
	// Data Staging / Archiving (T1560)
	// -------------------------------------------------------------------------

	m.addExact("powershell.exe", "7z.exe", suspicionMedium, 20,
		"T1560.001: PowerShell invoking 7-Zip -- data archive staging for exfiltration")
	m.addExact("cmd.exe", "7z.exe", suspicionMedium, 20,
		"T1560.001: cmd.exe invoking 7-Zip -- data archive staging")
	m.addExact("powershell.exe", "rar.exe", suspicionMedium, 20,
		"T1560.001: PowerShell invoking WinRAR -- data staging for exfiltration")
	m.addExact("cmd.exe", "rar.exe", suspicionMedium, 20,
		"T1560.001: cmd.exe invoking WinRAR -- data staging and compression")

	// =========================================================================
	// LOW (+10) -- Worth noting; low signal individually
	// =========================================================================

	m.addExact("explorer.exe", "wscript.exe", suspicionLow, 10,
		"T1059.005: Explorer launching VBScript -- phishing or legit script")
	m.addExact("powershell.exe", "ping.exe", suspicionLow, 10,
		"T1016: Connectivity check from PowerShell -- common but counted in burst")
	m.addExact("taskmgr.exe", "cmd.exe", suspicionLow, 10,
		"Task Manager spawning cmd -- uncommon but not necessarily malicious")

	// =========================================================================
	// WILDCARD entries -- always suspicious regardless of parent
	// =========================================================================

	// LOLBins -- any child they spawn is a red flag
	m.addWildcard("mshta.exe", suspicionCritical, 40,
		"T1218.005: mshta.exe spawning any child -- HTA LOLBin abuse (AppLocker bypass)")
	m.addWildcard("regsvr32.exe", suspicionCritical, 40,
		"T1218.010: regsvr32 Squiblydoo -- spawning child via COM scriptlet")
	m.addWildcard("cmstp.exe", suspicionCritical, 40,
		"T1218.003: cmstp.exe spawning child -- UAC bypass auto-elevation abuse")
	m.addWildcard("installutil.exe", suspicionCritical, 40,
		"T1218.004: InstallUtil.exe spawning child -- .NET code execution bypass")
	m.addWildcard("msbuild.exe", suspicionCritical, 40,
		"T1127.001: MSBuild.exe spawning child -- inline .NET task code execution")
	m.addWildcard("csc.exe", suspicionCritical, 40,
		"T1027.004: csc.exe spawning child -- compile-and-execute .NET payload (fileless)")
	m.addWildcard("diskshadow.exe", suspicionCritical, 40,
		"T1003.003/T1218: diskshadow.exe -- VSS LOLBin for NTDS.dit extraction or code exec")
	m.addWildcard("fltmc.exe", suspicionCritical, 40,
		"T1562.001: fltMC.exe spawning child -- minifilter driver unload (EDR blinding)")
	m.addWildcard("bcdedit.exe", suspicionCritical, 40,
		"T1490: bcdedit.exe spawning child -- boot config modification (ransomware prep)")

	// Credential dumping tools -- always critical
	m.addWildcard("procdump.exe", suspicionCritical, 40,
		"T1003.001: procdump.exe spawning child -- LSASS credential dumping tool")
	m.addWildcard("mimikatz.exe", suspicionCritical, 40,
		"T1003: mimikatz.exe spawning child -- credential dumping tool")
	m.addWildcard("ntdsutil.exe", suspicionCritical, 40,
		"T1003.003: ntdsutil.exe spawning child -- NTDS credential database manipulation")
	m.addWildcard("esentutl.exe", suspicionCritical, 40,
		"T1003.003: esentutl.exe spawning child -- ESE database copy (NTDS.dit exfil)")
	m.addWildcard("vssadmin.exe", suspicionCritical, 40,
		"T1490/T1003.003: vssadmin.exe spawning child -- shadow copy deletion or NTDS access")

	// Anti-forensics / log clearing
	m.addWildcard("wevtutil.exe", suspicionHigh, 30,
		"T1070.001: wevtutil.exe spawning child -- Windows Event Log clearing (anti-forensics)")

	return m
}

// addExact registers an exact parent->child pair in the matrix.
func (m *SuspicionMatrix) addExact(parent, child string, level suspicionLevel, bonus int, rationale string) {
	m.exact[matrixKey{
		parent: strings.ToLower(parent),
		child:  strings.ToLower(child),
	}] = suspicionEntry{Level: level, Bonus: bonus, Rationale: rationale}
}

// addWildcard registers a child process that is always suspicious, regardless
// of the parent that spawned it.
func (m *SuspicionMatrix) addWildcard(child string, level suspicionLevel, bonus int, rationale string) {
	m.wildcardChild[strings.ToLower(child)] = suspicionEntry{
		Level: level, Bonus: bonus, Rationale: rationale,
	}
}

// ComputeBonus walks the lineage chain (from target -> ancestors) and returns
// the highest suspicion bonus found plus the corresponding level string.
//
// Algorithm:
//  1. Check wildcard table for the target process (chain[0])
//  2. For each adjacent pair (chain[i] = child, chain[i+1] = parent), check
//     the exact matrix. Use the highest bonus found across all pairs.
//
// We return only the HIGHEST bonus (not additive) to avoid double-counting
// when the same chain is suspicious at multiple levels.
func (m *SuspicionMatrix) ComputeBonus(chain []*infracache.ProcessLineageEntry) (bonus int, level string) {
	if len(chain) == 0 {
		return 0, string(suspicionNone)
	}

	maxBonus := 0
	maxLevel := suspicionNone

	// Check wildcard for the target process (index 0)
	targetName := strings.ToLower(chain[0].Name)
	if entry, ok := m.wildcardChild[targetName]; ok {
		if entry.Bonus > maxBonus {
			maxBonus = entry.Bonus
			maxLevel = entry.Level
		}
	}

	// Check exact pairs along the chain
	for i := 0; i < len(chain)-1; i++ {
		childName := strings.ToLower(chain[i].Name)
		parentName := strings.ToLower(chain[i+1].Name)

		key := matrixKey{parent: parentName, child: childName}
		if entry, ok := m.exact[key]; ok {
			if entry.Bonus > maxBonus {
				maxBonus = entry.Bonus
				maxLevel = entry.Level
			}
		}
	}

	if maxBonus == 0 && len(chain) >= 2 {
		// Gap detection: no matrix entry exists for this parent→child pair.
		// Emitted at Debug level (not Warn) to avoid log spam in production.
		// Tune to Warn during matrix review sessions.
		logger.Debugf("[SUSPICION-GAP] no matrix entry: parent=%s child=%s",
			strings.ToLower(chain[1].Name), strings.ToLower(chain[0].Name))
	}

	return maxBonus, string(maxLevel)
}

// Lookup returns the suspicion entry for a specific parent->child pair, or
// (false, empty entry) if the pair is not in the matrix. Exported for tests.
func (m *SuspicionMatrix) Lookup(parent, child string) (suspicionEntry, bool) {
	key := matrixKey{
		parent: strings.ToLower(parent),
		child:  strings.ToLower(child),
	}
	e, ok := m.exact[key]
	return e, ok
}

// LookupWildcard returns the wildcard entry for a child process.
func (m *SuspicionMatrix) LookupWildcard(child string) (suspicionEntry, bool) {
	e, ok := m.wildcardChild[strings.ToLower(child)]
	return e, ok
}

// Size returns the total number of entries (exact + wildcard).
func (m *SuspicionMatrix) Size() int {
	return len(m.exact) + len(m.wildcardChild)
}
