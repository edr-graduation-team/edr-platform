# Windows EDR Agent — Architectural & Security Audit + Implementation Plan

## Part 1: Architectural Analysis

### 1. Data Collection & Filtering

**What the agent collects:**

| Collector | Source | Mechanism | Interval |
|-----------|--------|-----------|----------|
| **ETW** (Process) | Windows Kernel via `StartKernelProcessSession` (C/CGo) | Real-time callback ([goProcessEvent](file:///d:/EDR_Platform/win_edrAgent/internal/collectors/etw.go#155-185)) for opcode 1 (start) / 2 (end) | Continuous |
| **Registry** | 10 persistence-critical keys (Run, RunOnce, Services, Winlogon, IFEO, AppInit, Tasks) | Polling via `registry.OpenKey` + value diffing | Every 10s |
| **Network** | TCP connections (Established + Listen) | PowerShell `Get-NetTCPConnection` + connection cache diffing | Every 30s |
| **WMI** | Process inventory, system info, network adapters | PowerShell `Get-CimInstance` | Every 60min |

**Local filtering (pre-transmission):**

Yes — the agent applies a **multi-layer filter pipeline** before events ever reach the batcher/network:

1. **In-line noise filter** (ETW [processStart](file:///d:/EDR_Platform/win_edrAgent/internal/collectors/etw.go#209-275)): Drops `conhost.exe`, `wmiprvse.exe`, and self-generated PowerShell commands.
2. **PID deduplication** ([isDuplicate](file:///d:/EDR_Platform/win_edrAgent/internal/collectors/etw.go#139-154)): Suppresses events for the same PID within a 2-second window.
3. **Configurable [Filter](file:///d:/EDR_Platform/win_edrAgent/internal/collectors/filter.go#20-41)** ([filter.go](file:///d:/EDR_Platform/win_edrAgent/internal/collectors/filter.go)):
   - `ExcludeProcesses` — O(1) map lookup (12 default excludes including `svchost.exe`, `csrss.exe`, `MsMpEng.exe`, self)
   - `ExcludeIPs / CIDRs` — Drops localhost, link-local (169.254.0.0/16)
   - `ExcludeRegistry` — Substring match on noisy registry paths
   - `ExcludePaths / IncludePaths` — Regex glob with directory-prefix matching (include overrides exclude)
   - `ExcludeEventIDs` — O(1) map for Sysmon Event IDs
   - `TrustedHashes` — O(1) SHA256 whitelist for known-good binaries
4. **Token Bucket Rate Limiter** ([rate_limiter.go](file:///d:/EDR_Platform/win_edrAgent/internal/collectors/rate_limiter.go)):
   - Per-event-type EPS limits with critical bypass (High/Critical events never dropped)
   - Runtime-reconfigurable via C2 `ADJUST_RATE` command

**Bandwidth savings** are significant — the filter + rate limiter output is serialized to JSON, compressed with **Snappy** (configurable to gzip), and then transmitted in batches of 50 events (configurable).

---

### 2. Event Format & Contract

**Format:** Events are serialized to **JSON** by the batcher, compressed with **Snappy**, wrapped in a **Protobuf** [EventBatch](file:///d:/EDR_Platform/win_edrAgent/internal/grpc/client.go#601-626) envelope, and sent over a **gRPC bidirectional stream** with **mTLS**.

**Contract:**

| Layer | Format | Validation |
|-------|--------|------------|
| Event payload | JSON (`[]*event.Event` → `json.Marshal`) | No JSON Schema validation on agent side |
| Transport envelope | Protobuf [EventBatch](file:///d:/EDR_Platform/win_edrAgent/internal/grpc/client.go#601-626) (batch_id, agent_id, timestamp, compression enum, payload bytes, event_count, checksum) | SHA256 checksum computed on compressed payload |
| Wire protocol | **gRPC** (`StreamEvents` bidirectional RPC) | mTLS with client cert + CA chain; server returns `CommandBatch` |
| Heartbeat | **gRPC** unary [Heartbeat](file:///d:/EDR_Platform/win_edrAgent/internal/grpc/client.go#443-510) RPC | Proto `HeartbeatRequest` with full system metrics |

**Is there a strict validated contract?** **Yes** — the gRPC protobuf definitions in [edr.pb.go](file:///d:/EDR_Platform/win_edrAgent/internal/pb/edr.pb.go) provide a strict binary contract. However, the JSON payload inside `EventBatch.payload` is schemaless — the server must decompress and parse it trusting the agent's field naming conventions (ECS-inspired but not formally validated).

**Data loss vectors:** The agent includes a **disk-backed WAL** ([disk_queue.go](file:///d:/EDR_Platform/win_edrAgent/internal/queue/disk_queue.go)) that survives disconnects. The queue processor retries with exponential backoff. The main loss vector is the in-memory channel overflow (5000 buffer, non-blocking send drops oldest).

---

### 3. Security Efficacy (MITRE ATT&CK Coverage)

| MITRE Tactic | Technique | Coverage | Source |
|--------------|-----------|----------|--------|
| **Execution** (TA0002) | T1059 (Command-Line) | ✅ Full command_line, executable, parent_executable | ETW [processStart](file:///d:/EDR_Platform/win_edrAgent/internal/collectors/etw.go#209-275) |
| **Persistence** (TA0003) | T1547 (Boot/Logon Autostart) | ✅ Run/RunOnce/Services/Winlogon/IFEO/AppInit/Tasks | Registry collector |
| **Privilege Escalation** (TA0004) | T1134 (Token Manipulation) | ✅ `is_elevated`, `integrity_level`, `user_sid` | ETW [getPrivileges](file:///d:/EDR_Platform/win_edrAgent/internal/collectors/etw.go#419-465) |
| **Defense Evasion** (TA0005) | T1036 (Masquerading) | ⚠️ Partial — process name/path captured, but no hash verification on live processes | ETW |
| **Discovery** (TA0007) | T1057 (Process Discovery) | ✅ Full baseline snapshot + live tracking | ETW + WMI |
| **Lateral Movement** (TA0008) | T1021 (Remote Services) | ⚠️ Limited — outbound TCP connections tracked but no SMB/RDP-specific detection | Network collector |
| **Collection** (TA0009) | T1005 (Data from Local System) | ❌ No file access monitoring (file collector defined but not integrated) | — |
| **C&C** (TA0011) | T1071 (Application Layer Protocol) | ✅ Network connections with PID correlation | Network collector |

> [!IMPORTANT]
> **Gap: File monitoring** — The agent defines `EventTypeFile` and [FileEvent](file:///d:/EDR_Platform/win_edrAgent/internal/event/types.go#125-140) types, and the config has `file_enabled: true`, but there is **no file collector implementation**. The network collector and ETW capture processes, but file create/modify/delete events are not collected.

> [!WARNING]
> **Gap: Image/DLL load monitoring** — `EventTypeImageLoad` is defined but no collector implements it. This is critical for detecting DLL injection (T1055), DLL side-loading (T1574), and other evasion techniques.

---

### 4. Context-Awareness Analysis

**Yes, context-aware features are implemented and working correctly:**

1. **Process Lineage** (parent-child relationships):
   - ETW [processStart](file:///d:/EDR_Platform/win_edrAgent/internal/collectors/etw.go#209-275) enriches every event with `ppid`, `parent_executable`, `parent_name` via [getImagePath(ppid)](file:///d:/EDR_Platform/win_edrAgent/internal/collectors/etw.go#353-369)
   - Baseline snapshot captures the full process tree via `Toolhelp32Snapshot`
   - This enables Sigma rules for parent-child anomalies (e.g., `winword.exe` spawning `cmd.exe`)

2. **Privilege Context**:
   - [getPrivileges(pid)](file:///d:/EDR_Platform/win_edrAgent/internal/collectors/etw.go#419-465) extracts: `user_sid`, `user_name` (DOMAIN\User), `is_elevated` (admin token), `integrity_level` (Low/Medium/High/System)
   - Attached to both live events and baseline snapshot events

3. **Correctness**: The enrichment is applied **before** sending to the event channel, so the Sigma Engine receives fully enriched events. One subtlety: for short-lived processes, the API calls ([getImagePath](file:///d:/EDR_Platform/win_edrAgent/internal/collectors/etw.go#353-369), [getCmdLine](file:///d:/EDR_Platform/win_edrAgent/internal/collectors/etw.go#375-418), [getPrivileges](file:///d:/EDR_Platform/win_edrAgent/internal/collectors/etw.go#419-465)) may fail if the process exits before the goroutine runs — the code falls back to ETW event data, which is correct but may have truncated command lines.

---

## Part 2: Implementation Plan

### Component 1: Security Module (`internal/security/`)

---

#### [NEW] [acl_windows.go](file:///d:/EDR_Platform/win_edrAgent/internal/security/acl_windows.go)

**NTFS ACL hardening** — Uses Windows `SetNamedSecurityInfo` via `golang.org/x/sys/windows` to lock down EDR directories.

- Creates a DACL with **only** `SYSTEM` (S-1-5-18) and `Administrators` (S-1-5-32-544) as full-control ACEs
- Disables DACL inheritance so parent permissions don't leak in
- Applied to: `C:\ProgramData\EDR\queue`, `C:\ProgramData\EDR\logs`, `C:\ProgramData\EDR\certs`, `C:\ProgramData\EDR\config`, `C:\ProgramData\EDR\quarantine`
- Exposes `HardenDirectories(dirs []string, logger) error` called from agent startup

#### [NEW] [encryption.go](file:///d:/EDR_Platform/win_edrAgent/internal/security/encryption.go)

**Log & queue encryption** — AES-256-GCM encryption for all data-at-rest.

- Generates a 256-bit key on first run, stores it in Windows DPAPI-protected storage (via `CryptProtectData`) so only SYSTEM can decrypt
- Key file: `C:\ProgramData\EDR\config\.agent.key` (DPAPI-encrypted blob)
- Exposes `Encrypt(plaintext []byte) ([]byte, error)` and `Decrypt(ciphertext []byte) ([]byte, error)`
- The encryption/decryption will be wired into the logger and disk queue

#### [NEW] [retention.go](file:///d:/EDR_Platform/win_edrAgent/internal/security/retention.go)

**48-hour data retention** — Background goroutine that runs every 15 minutes.

- Scans `C:\ProgramData\EDR\queue\*.bin` and `C:\ProgramData\EDR\logs\agent.log.*` for files older than 48 hours (by ModTime)
- Deletes expired files and logs a summary
- Exposes `StartRetentionCleaner(ctx, dirs []string, maxAge time.Duration, logger)` called from agent startup

#### [NEW] [selfprotect_windows.go](file:///d:/EDR_Platform/win_edrAgent/internal/security/selfprotect_windows.go)

**Anti-tampering self-protection** — Foundational protection for the agent process and service.

- **Process DACL**: Uses `SetSecurityInfo` on the agent's own process handle to remove `PROCESS_TERMINATE` and `PROCESS_SUSPEND_RESUME` from all non-SYSTEM users
- **Service protection**: Sets the Windows Service failure action to restart immediately (already done) and configures `SERVICE_SID_TYPE_RESTRICTED` for defense-in-depth
- **File watchdog**: A goroutine that monitors the agent's executable and config files for unauthorized modification (SHA256 hash check every 30s) and alerts if tampering is detected
- Exposes `ProtectProcess(logger) error` and `StartFileWatchdog(ctx, paths []string, logger)` called from agent startup

---

### Component 2: Integration into Agent Startup

---

#### [MODIFY] [agent.go](file:///d:/EDR_Platform/win_edrAgent/internal/agent/agent.go)

- In `Start()`: Call `security.HardenDirectories()`, `security.ProtectProcess()`, `security.StartRetentionCleaner()`, `security.StartFileWatchdog()` before starting collectors
- Add the encryption key loading in `New()`

#### [MODIFY] [logger.go](file:///d:/EDR_Platform/win_edrAgent/internal/logging/logger.go)

- Wire encryption into file write path: `log()` → `encrypt(entry)` → `file.Write(ciphertext)`
- The logger gets an optional `Encryptor` interface injected after construction

#### [MODIFY] [disk_queue.go](file:///d:/EDR_Platform/win_edrAgent/internal/queue/disk_queue.go)

- `Enqueue()`: encrypt the proto-marshaled bytes before writing to disk
- `PeekOldest()`: decrypt the file bytes before proto unmarshal
- The `DiskQueue` gets an optional `Encryptor` interface injected at construction

---

## Part 3: Proactive Recommendations

> [!CAUTION]
> **Critical Gap — No File Monitoring Collector**: The agent has `file_enabled: true` in config and defines `EventTypeFile` but there is NO implementation. This means MITRE T1005, T1074, T1565, and all file-based indicators are invisible. This should be a high-priority follow-up.

> [!WARNING]
> **No DLL/Image Load Monitoring**: `EventTypeImageLoad` exists in types but has no collector. This is essential for detecting DLL injection, side-loading, and reflective loading. ETW can provide this via the `Microsoft-Windows-Kernel-File` provider.

> [!IMPORTANT]
> **Sysmon Dependency**: The agent does NOT use Sysmon — it goes directly to the Windows Kernel ETW trace for process events. The `ExcludeEventIDs` filter config references Sysmon Event IDs, but these will never match because the ETW collector does not emit Sysmon-style event IDs. This is harmless but confusing.

> [!NOTE]
> **WMI JSON Parsing**: The WMI collector uses naive string splitting (`strings.Split(output, "},{")`) instead of `encoding/json`. This will silently corrupt records containing commas or colons in field values (e.g., command lines like `cmd.exe /c echo hello,world`).

**Additional recommendations:**
- **DNS sinkhole detection**: Currently no DNS monitoring is active despite `EventTypeDNS` being defined
- **Parent PID spoofing**: The current `ppid` is taken at face value from ETW — sophisticated attackers can spoof this via `PROC_THREAD_ATTRIBUTE_PARENT_PROCESS`
- **Memory-safe key handling**: Encryption keys should be zeroed after use via `crypto/subtle.ConstantTimeEq` patterns and mlock'd pages
- **Audit logging**: Command execution is logged but not written to a tamper-evident audit trail (currently just `logging.Logger`)

---

## Verification Plan

### Automated Tests

**Build verification** (must pass before implementation is considered complete):
```powershell
cd d:\EDR_Platform\win_edrAgent
$env:CGO_ENABLED="1"; go build ./...
```

**Existing unit tests** (Run to validate no regressions):
```powershell
cd d:\EDR_Platform\win_edrAgent
go test ./internal/collectors/ ./internal/config/ ./internal/event/ ./internal/installer/
```

### Manual Verification

1. **ACL Verification**: After agent startup, open PowerShell as admin and run:
   ```powershell
   icacls "C:\ProgramData\EDR\queue"
   icacls "C:\ProgramData\EDR\logs"
   ```
   Expected: Only `SYSTEM:(OI)(CI)(F)` and `BUILTIN\Administrators:(OI)(CI)(F)` should appear. No entries for `Users` or `Everyone`.

2. **Encryption Verification**: Check that `.bin` files in the queue directory are not readable as raw protobuf:
   ```powershell
   Get-Content "C:\ProgramData\EDR\queue\*.bin" -Raw | Select-Object -First 100
   ```
   Expected: Binary gibberish, not recognizable protobuf structure.

3. **Retention Verification**: Create test files with old timestamps and verify they get cleaned up within 15 minutes.

4. **Self-Protection Verification**: Try to taskkill the agent process from a non-elevated command prompt — it should fail with "Access Denied".
