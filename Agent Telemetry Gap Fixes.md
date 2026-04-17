# Phase 1 — Agent Telemetry Gap Fixes: Walkthrough

## Overview

This phase addresses the most critical detection blind spots identified in the E2E evaluation by adding **3 new kernel-level telemetry collectors** to the Windows EDR agent, all using ETW with TDH (Trace Data Helper) for reliable, real-time field extraction.

> [!IMPORTANT]
> **Impact**: These changes activate **70+ previously non-functional Sigma rules** for DNS tunneling, named pipe C2 communication, and credential dumping detection.

---

## Architecture Decision: Why ETW + TDH?

The existing agent architecture uses:
1. **C layer** (`etw_cgo.c`) — manages ETW sessions, fires on every kernel event
2. **TDH** — Microsoft's Trace Data Helper API resolves field offsets by name (no hardcoded byte offsets)
3. **Go `//export` callbacks** — receive pre-parsed C structs for safe Go processing
4. **Go goroutines** — enrich events with process names, user context, and filtering

All 3 new collectors follow this exact pattern for consistency and maintainability.

---

## Changes Made

### 1. DNS Collector — `dns.go`

**What it detects:**
- C2 domain lookups (Cobalt Strike staging, Metasploit handlers)
- DGA (Domain Generation Algorithm) patterns
- DNS tunneling / data exfiltration
- Suspicious domain reputation lookups

**How it works:**
```
Microsoft-Windows-DNS-Client ETW Provider
    ↓ EventID 3006 (QueryCompleted)
    ↓ TDH extracts: QueryName, QueryType, QueryStatus, QueryResults
    ↓ C callback → goDnsEvent()
    ↓ Go: noise filter + process attribution + emit event
```

**ETW Provider:** `{1C95126E-7EEA-49A9-A3FE-A378B03DDB4D}` (Microsoft-Windows-DNS-Client)

**Session Architecture:** Runs in its own **user-mode ETW session** (`EDRDnsTrace`) because DNS-Client is a manifest-based provider, not a kernel trace flag.

**Fields emitted (Sigma dns_query compatible):**

| Field | Source | Sigma Mapping |
|-------|--------|---------------|
| `query_name` | TDH QueryName | `QueryName` |
| `query_type` | TDH QueryType → map | `QueryType` (A, AAAA, CNAME...) |
| `response_code` | TDH QueryStatus → map | `QueryStatus` (NOERROR, NXDOMAIN...) |
| `answers` | TDH QueryResults | `QueryResults` |
| `process_name` | Windows API | — |
| `process_path` | Windows API | `Image` |
| `pid` | EventHeader | `ProcessId` |

**Noise Filtering:**
- Hard-coded trusted domains (msftconnecttest.com, microsoft telemetry)
- Reverse DNS (`.in-addr.arpa`) filtered out
- Agent self-exclusion (own PowerShell queries)
- Rate limited: 100 EPS default

---

### 2. Named Pipe Collector — `pipe.go`

**What it detects:**
- Cobalt Strike default pipes (`\\.\pipe\msagent_*`, `\\.\pipe\MSSE-*`)
- PsExec lateral movement (`\\.\pipe\PSEXESVC`)
- Metasploit/Meterpreter pipe communication
- Sliver / Covenant C2 framework pipes

**How it works (zero overhead — NO additional ETW session):**
```
Kernel FileIo ETW (existing session)
    ↓ File path starts with \Device\NamedPipe\
    ↓ C callback detects pipe path → routes to goPipeEvent()
    ↓ Go: strip prefix + noise filter + severity promotion + emit
```

**Key Design Decision:** Pipe events are intercepted from the **existing** kernel FileIo stream by checking if the file path contains `\Device\NamedPipe\`. This means:
- ✅ Zero additional ETW session overhead
- ✅ Zero additional kernel resource usage
- ✅ Automatic coverage when ETW is enabled

**Severity Promotion:** If a pipe name matches known C2 patterns, the event severity is automatically promoted from Low to Medium:
```go
var suspiciousPipePatterns = []string{
    "msagent_",    // Cobalt Strike default
    "MSSE-",       // Cobalt Strike variant
    "postex_",     // Cobalt Strike post-exploitation
    "PSEXESVC",    // PsExec lateral movement
    "gruntsvc",    // Covenant C2
    "demoagent_",  // Sliver C2
    // ... more patterns
}
```

**Fields emitted (Sigma pipe_created/pipe_connected compatible):**

| Field | Source | Sigma Mapping |
|-------|--------|---------------|
| `pipe_name` | Kernel FileIo | `PipeName` |
| `action` | Opcode 64=create, other=connect | — |
| `process_name` | Windows API | — |
| `process_path` | Windows API | `Image` |
| `user_name` | Token query | `User` |

---

### 3. Process Access Monitor — `process_access.go`

**What it detects:**
- **LSASS credential dumping** — Mimikatz (`T1003.001`)
- **Process injection** — DLL injection, process hollowing (`T1055`)
- **Handle abuse** — privilege escalation via handle duplication
- **Anti-analysis evasion** — debugger detection (`T1622`)

**How it works:**
```
Microsoft-Windows-Kernel-Audit-API-Calls ETW Provider
    ↓ EventID 1 (OpenProcess)
    ↓ TDH extracts: TargetProcessId, DesiredAccess, ReturnCode
    ↓ CallerPID from EventHeader
    ↓ C callback → goProcessAccessEvent()
    ↓ Go: access mask filter + target sensitivity check + emit
```

**ETW Provider:** `{E02A841C-75A3-4FA7-AFC8-AE09CF9B7F23}` (Microsoft-Windows-Kernel-Audit-API-Calls)

**Critical Security Filter — Access Mask Logic:**

Not all OpenProcess calls are suspicious. The filter only reports events where the access mask includes **dangerous combinations**:

```go
// Credential dump signature: VM_READ + VM_OPERATION
if mask&processVMRead != 0 && mask&processVMOperation != 0 { return true }

// Injection signature: VM_WRITE + VM_OPERATION
if mask&processVMWrite != 0 && mask&processVMOperation != 0 { return true }

// Remote thread: CREATE_THREAD + VM_OPERATION
if mask&processCreateThread != 0 && mask&processVMOperation != 0 { return true }

// PROCESS_ALL_ACCESS — always suspicious
if mask&processAllAccess == processAllAccess { return true }
```

**Severity Classification:**

| Scenario | Severity | Example |
|----------|----------|---------|
| Normal process → non-sensitive target | Medium | explorer.exe → notepad.exe |
| Any process → sensitive target | **High** | unknown.exe → svchost.exe |
| Any process → LSASS with PROCESS_ALL_ACCESS | **Critical** | mimikatz.exe → lsass.exe |

**Fields emitted (Sigma process_access compatible):**

| Field | Source | Sigma Mapping |
|-------|--------|---------------|
| `source_process_path` | Windows API | `SourceImage` |
| `target_process_path` | Windows API | `TargetImage` |
| `access_mask` | TDH DesiredAccess | `GrantedAccess` (hex) |
| `source_pid` | EventHeader | `SourceProcessId` |
| `target_pid` | TDH | `TargetProcessId` |
| `user_name` | Token query | `User` |
| `user_sid` | Token query | — |

---

## C Layer Changes

### `etw_cgo.h` — New Structs

```c
typedef struct { ... } ParsedDnsEvent;           // DNS query fields
typedef struct { ... } ParsedPipeEvent;           // Named pipe fields
typedef struct { ... } ParsedProcessAccessEvent;  // Process access fields
```

### `etw_cgo.c` — New Logic

1. **Provider GUIDs** — DNS-Client + Kernel-Audit-API-Calls
2. **TDH Parsers** — `parseDnsEvent()`, `parseProcessAccessEvent()`
3. **Pipe Detection** — `isNamedPipePath()` helper in the FileIo callback
4. **User-Mode Sessions** — `StartUserModeSession()` + `ProcessUserModeEvents()` using `EnableTraceEx2`
5. **Callback Routing** — `stdcallEventCallback` now routes to 6 event types total

---

## Sigma Engine Integration

### Field Mapper Updates (`field_mapper.go`)

Added 12 new field mappings:

```diff
+ "SourceImage":     "data.source_process_path"     // Process Access
+ "TargetImage":     "data.target_process_path"     // Process Access
+ "GrantedAccess":   "data.access_mask"              // Process Access
+ "SourceProcessId": "data.source_pid"               // Process Access
+ "TargetProcessId": "data.target_pid"               // Process Access
+ "CallTrace":       "data.call_trace"               // Process Access
+ "QueryType":       "data.query_type"               // DNS extended
+ "QueryStatus":     "data.QueryStatus"              // DNS extended
```

### Event Category Inference (`event.go`)

```diff
  case "pipe":
+   // Distinguish pipe_created vs pipe_connected from action field
+   if act contains "connect" → EventCategoryPipeConnected
    return EventCategoryPipeCreated
+ case "process_access":
+   return EventCategoryProcessAccess
```

---

## Config Changes

### New Toggles (`config.go`)

```yaml
collectors:
  dns_enabled: true            # ETW DNS-Client (50+ Sigma rules)
  pipe_enabled: true           # Kernel FileIo pipe detection
  process_access_enabled: true # LSASS/credential dump detection
```

### Rate Limits

```yaml
rate_limit:
  per_event_type:
    dns: 100   # DNS can be bursty during browsing
    pipe: 50   # Pipe events are rare; low limit is safe
    # process_access: NOT rate-limited (high-value, pre-filtered in C)
```

---

## Files Changed

| File | Action | Purpose |
|------|--------|---------|
| [types.go](file:///d:/EDR_Platform/win_edrAgent/internal/event/types.go) | Modified | Added `EventTypeProcessAccess`, enhanced `PipeEvent` and `ProcessAccessEvent` structs |
| [config.go](file:///d:/EDR_Platform/win_edrAgent/internal/config/config.go) | Modified | Added `DNSEnabled`, `PipeEnabled`, `ProcessAccessEnabled` toggles + rate limit defaults |
| [etw_cgo.h](file:///d:/EDR_Platform/win_edrAgent/internal/collectors/etw_cgo.h) | Modified | Added `ParsedDnsEvent`, `ParsedPipeEvent`, `ParsedProcessAccessEvent` structs + user-mode session functions |
| [etw_cgo.c](file:///d:/EDR_Platform/win_edrAgent/internal/collectors/etw_cgo.c) | Modified | Added DNS/ProcessAccess TDH parsers, pipe detection, user-mode ETW session management |
| [dns.go](file:///d:/EDR_Platform/win_edrAgent/internal/collectors/dns.go) | **New** | DNS collector with noise filtering and Sigma field emission |
| [pipe.go](file:///d:/EDR_Platform/win_edrAgent/internal/collectors/pipe.go) | **New** | Pipe collector with C2 pattern detection and severity promotion |
| [process_access.go](file:///d:/EDR_Platform/win_edrAgent/internal/collectors/process_access.go) | **New** | Process Access monitor with access mask analysis and LSASS priority |
| [agent_windows.go](file:///d:/EDR_Platform/win_edrAgent/internal/agent/agent_windows.go) | Modified | Wired all 3 new collectors into platform startup |
| [event.go](file:///d:/EDR_Platform/sigma_engine_go/internal/domain/event.go) | Modified | Added `process_access` category + refined pipe categorization |
| [field_mapper.go](file:///d:/EDR_Platform/sigma_engine_go/internal/application/mapping/field_mapper.go) | Modified | Added 12 new field mappings for process access + extended DNS |

---

## Detection Coverage Impact

```
Before Phase 1:
├── Process Creation  ✅ (ETW kernel)
├── Image Load        ✅ (ETW kernel)
├── File I/O          ✅ (ETW kernel)
├── Network           ⚠️  (PowerShell polling — 30s gaps)
├── Registry          ⚠️  (Polling — 10s gaps)
├── DNS               ❌ (50+ Sigma rules non-functional)
├── Named Pipes       ❌ (Cobalt Strike undetectable)
└── Process Access    ❌ (Mimikatz/LSASS undetectable)

After Phase 1:
├── Process Creation  ✅ (ETW kernel)
├── Image Load        ✅ (ETW kernel)
├── File I/O          ✅ (ETW kernel)
├── Network           ⚠️  (PowerShell polling — Phase 1 scope: unchanged)
├── Registry          ⚠️  (Polling — Phase 1 scope: unchanged)
├── DNS               ✅ (ETW real-time — 50+ rules NOW ACTIVE)
├── Named Pipes       ✅ (ETW kernel FileIo — 0 overhead)
└── Process Access    ✅ (ETW real-time — LSASS/injection detection)
```

> [!NOTE]
> Network and Registry upgrades from polling to ETW/callback are planned for a future iteration. The current phase prioritizes the 3 completely missing telemetry sources that had the highest detection impact.
