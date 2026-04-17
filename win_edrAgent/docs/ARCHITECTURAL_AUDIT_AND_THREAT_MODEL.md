# Architectural Audit and Threat Modeling Assessment

**win_edrAgent** — Engineering Report  
**Scope:** End-to-end execution flow, offline behavior, self-protection, integration gaps, production readiness.  
**Assessment type:** Static analysis and threat modeling; strictly objective.

---

## 1. End-to-End Execution Flow

### 1.1 Entry Points and Modes

| Mode | Trigger | Binary Path | Config Path |
|------|----------|-------------|--------------|
| **Install** | `-install` | N/A (install only) | N/A |
| **Uninstall** | `-uninstall` | N/A | N/A |
| **Version** | `-version` | Any | None (exit before load) |
| **Service** | `-service` | From SCM (e.g. `C:\...\win_edrAgent.exe`) | Hardcoded in Install: `-config C:\ProgramData\EDR\config\config.yaml` |
| **Standalone** | (default) | User-invoked | `-config` flag default `C:\ProgramData\EDR\config\config.yaml` |

**Exact sequence (service or standalone, after flags):**

1. **Logger** — Created with fixed path `C:\ProgramData\EDR\logs\agent.log`; level from config later.
2. **Config load** — `config.Load(*configPath)`; failure exits with status 1.
3. **Log level** — `logger.SetLevel(cfg.Logging.Level)`.
4. **Enrollment** — `enrollment.EnsureEnrolled(cfg, logger, *configPath)`:
   - If `cfg.Certs.CertPath` and `cfg.Certs.KeyPath` exist → return (already enrolled).
   - Else: dial server with **insecure** transport (no TLS for bootstrap), `RegisterAgent` with bootstrap token and CSR; on approval, save cert/CA via `CertManager.SaveCertificate`, set `cfg.Agent.ID = resp.GetAgentId()`, then `cfg.Save(configFilePath)` so AgentID persists.
5. **Dispatch** — If `-service`: `service.Run(cfg, logger)` (fails if not running as SCM service). Else: `runStandalone(cfg, logger)`.

### 1.2 Service Execution Path

- **Run** checks `svc.IsWindowsService()`; then `svc.Run(ServiceName, &edrService{cfg, logger})`.
- **Execute** (SCM callback): report StartPending → `agent.New(s.cfg, s.logger)` → `s.agent.Start(ctx)` in a goroutine → report Running → loop on change requests (Interrogate, Stop, Shutdown, Pause, Continue).
- **Stop/Shutdown:** cancel context → `s.agent.Stop()` in goroutine → wait up to 30s → return.
- **Install:** `m.CreateService(..., exePath, "-service", "-config", "C:\\ProgramData\\EDR\\config\\config.yaml")`; recovery actions (restart on failure); event log source; `MkdirAll` for ProgramData\EDR trees; default config created if missing. No custom ACLs on binary, config, or certs.

### 1.3 Standalone Execution Path

- Create cancelable context; signal handler (SIGINT/SIGTERM) calls cancel.
- `agent.New(cfg, logger)` → `ag.Start(ctx)` → block on `<-ctx.Done()` → `ag.Stop()`.

### 1.4 Agent Start (Unified for Service and Standalone)

1. Context stored; `running` set; batcher created with `BatchSize`, `BatchInterval`, `Compression` from config.
2. **Goroutines started:** runBatcher, runSender, runHealthReporter.
3. **Platform collectors:** Windows only — if `cfg.Collectors.ETWEnabled`, `collectors.NewETWCollector(..., eventChan, logger)` and `etw.Start(ctx)`. No Registry, Network, or WMI collectors are instantiated despite config flags (`registry_enabled`, `network_enabled`, `wmi_enabled`).
4. **gRPC:** One optional `Connect(ctx)`; then always start: RunReconnector, RunStream, runCommandLoop, RunSender (all tied to same context).

### 1.5 Telemetry Pipeline (Continuous)

- **Producers:** ETW collector (process snapshot + simulated network) → non-blocking send to `eventChan`; full buffer → event dropped, `eventsDropped` incremented.
- **eventChan:** Bounded, size `cfg.Agent.BufferSize` (default 5000). Consumed only by runBatcher.
- **runBatcher:** Reads from `eventChan`, `batcher.Add(evt)`; when batch full or Flush/FlushIfReady, `processBatch(batch)`.
- **processBatch:** Builds `pb.EventBatch` with `batch.Payload` and `batch.Checksum` (no re-marshal); `grpcClient.SendBatch(pbBatch)`. On error (e.g. not connected), logs and returns; batch is not queued elsewhere.
- **SendBatch (client):** If `!connected`, returns error immediately. Else non-blocking send to `batchChan`; if full, returns "send queue full".
- **RunSender (client):** Reads `batchChan`, `sendBatchInternal` (stream Send or short-lived stream). On failure, batch is logged and counted as failed; no retry or disk queue.

### 1.6 C2 Command Execution

- **Ingress:** Bidirectional stream `StreamEvents`: server sends `CommandBatch` with `Command` messages. Client recvLoop (and sendBatchInternal fallback) parses commands, maps `ExpiresAt` via `commandExpiresAt(cmd)`, pushes to `commandChan`.
- **runCommandLoop:** Reads from `commandChan`, builds `command.Command` (including `ExpiresAt`), calls `a.commandHandler.Execute(a.ctx, c)` and **discards** the returned `*Result** (`_ = ...`). No send of command result or status back to the server.
- **Handler:** Expiry check; then switch on type (TerminateProcess, QuarantineFile, IsolateNetwork, etc.). Handler uses its own implementations (e.g. `taskkill`, PowerShell, `os.Rename`); **Executor** (with protected PIDs/paths) exists in package but is **not** used by Handler.

---

## 2. Offline Behavior & Resilience

### 2.1 When gRPC Connection Drops

- **Disconnect path:** Stream error or explicit `Disconnect()` → `conn.Close()`, `connected.Store(false)`, stream cleared.
- **SendBatch:** As soon as `!c.connected.Load()`, returns `fmt.Errorf("not connected")` without queuing to `batchChan`.
- **processBatch:** Calls `SendBatch(pbBatch)`; on error, logs "Send batch failed (pipeline continues)" and **returns**. The batch is **not** written to disk or re-queued; it is **dropped**.
- **RunReconnector:** When `!connected`, sleeps delay then `Connect(ctx)`; on success resets delay; on failure increases delay (capped). So connection is eventually re-established, but no batches are buffered during outage.

### 2.2 eventChan and Batcher During Outage

- **eventChan:** Collectors continue to push. If buffer is not full, events are queued; if full, `SubmitEvent` uses `default` and drops the event (debug log). So during outage, events either sit in the in-memory buffer or are dropped at the producer.
- **runBatcher:** Continues to read from `eventChan` and call `batcher.Add(evt)`. When a batch is complete (size or interval), `processBatch(batch)` is called. **processBatch** calls `SendBatch`, which fails when disconnected → batch is dropped. So:
  - Events already in the batch when the connection dropped are sent only if they were queued to `batchChan` before disconnect; once `SendBatch` returns "not connected", that batch is discarded.
  - New events keep filling the batcher; when a new batch is produced, it is passed to processBatch → SendBatch fails → **that batch is lost**.
- **runSender (agent):** Ticker still calls `FlushIfReady()` and `processBatch`; same behavior — batches dropped when not connected.
- **No disk queue:** There is no code path that writes batches to disk or to a persistent queue when send fails. All batches produced while disconnected are **lost** after processBatch returns.

### 2.3 Data Loss Summary

| Scenario | Result |
|----------|--------|
| Connection drops | All batches subsequently produced are dropped at processBatch (SendBatch returns "not connected"). |
| eventChan full | New events dropped at collector/SubmitEvent (debug log). |
| batchChan full | processBatch would get "send queue full" from SendBatch; batch dropped (in practice SendBatch fails earlier due to !connected when disconnected). |
| Process or service stop | In-memory events and any batch not yet sent are lost; no flush-to-disk. |

**Conclusion:** There is **no offline persistent buffering**. During a network outage, all telemetry generated after the disconnect is **lost** (except what remains in the in-memory event buffer until it is batched and then dropped at send). The pipeline is "best effort" only.

---

## 3. Self-Protection (Anti-Tampering)

### 3.1 Process Termination

- **taskkill / PID:** Any process with sufficient privilege (e.g. local admin) can call `taskkill /PID <edr_agent_pid> /F`. The agent is a normal user-mode process; there is **no** kernel driver or user-mode protection (e.g. no ObRegisterCallbacks, no blocking of OpenProcess/TerminateProcess for the agent PID).
- **Handler.terminateProcess:** Only blocks PIDs `"0"` and `"4"` (system). It does **not** protect the agent’s own PID. The separate **Executor** has `protectedPIDs` and `isCriticalProcess`, but the Handler does **not** use the Executor.
- **Service stop:** The service accepts `svc.Stop` and `svc.Shutdown`. There is no custom SCM filter or policy to reject stop requests from non-privileged or unauthorized callers beyond what Windows enforces (e.g. admin). A local administrator can stop the service via `sc stop EDRAgent` or Services MMC.

### 3.2 Configuration and Certificate Files

- **Paths:** Config: `configPath` (e.g. `C:\ProgramData\EDR\config\config.yaml`). Certs: `cfg.Certs.CertPath`, `KeyPath`, `CAPath` (from config; typically under `C:\ProgramData\EDR\certs`).
- **Install:** `MkdirAll(..., 0755)` for directories; default config created with `Save()` (YAML write). **No** explicit ACLs are set on config or cert files to restrict deletion or modification. Permissions are whatever the process (SYSTEM when run as service) and directory inheritance provide.
- **Deletion/modification:** A local admin (or process with write access to those directories) can delete or overwrite config and cert files. The agent does not set "deny" ACLs or use tamper protection. On next connect or restart, load of config/certs would fail or use replaced content.

### 3.3 Kernel-Mode / Additional Hardening

- **Kernel driver:** None. No driver to protect process, service, or files.
- **ETW / callback hardening:** No use of Tamper Protection or other OS hardening APIs specifically to protect the agent binary or its data.

**Conclusion:** The agent has **no** anti-tampering or self-protection mechanisms. A local administrator (or malware with equivalent privilege) can terminate the process, stop the service, and delete or modify config and certificate files without the agent resisting.

---

## 4. Integration Gaps & Incomplete Features

### 4.1 Collectors Not Wired

| Component | Config flag | In code | Wired in agent |
|-----------|-------------|---------|-----------------|
| ETW | `collectors.etw_enabled` | `NewETWCollector`, `Start` | Yes — only collector started in `startPlatformCollectors` |
| Registry | `collectors.registry_enabled` | `NewRegistryCollector` | **No** — never instantiated |
| Network | `collectors.network_enabled` | `NewNetworkCollector` | **No** — never instantiated |
| WMI | `collectors.wmi_enabled`, `wmi_interval` | `NewWMICollector` | **No** — never instantiated |
| File | `collectors.file_enabled` | (no dedicated file collector in code) | N/A |

So only ETW (and its current implementation: process snapshot + simulated network) is active; registry, network, and WMI are present in code but **unused**.

### 4.2 Command Handler Stubs and Unused Executor

- **COLLECT_FORENSICS:** Parses `paths` and returns a message; **TODO** in code: copy/compress, upload to server. No actual collection or upload.
- **UPDATE_CONFIG:** Requires `config` param; **TODO**: parse, validate, apply, persist. Returns "Configuration updated" without changing anything.
- **UPDATE_AGENT:** Requires `version`, `url`, optionally `checksum`; **TODO**: download, verify checksum, replace binary, restart. Returns message only; no download or replace.
- **ADJUST_RATE:** **TODO**: apply to running batcher; returns message only; batcher size/interval not updated from command.
- **Executor:** Package has `Executor` with `TerminateProcess`, `QuarantineFile`, `IsolateNetwork`, `UnisolateNetwork`, `CollectForensics`, `DownloadUpdate` and safety lists (protected PIDs/paths). The **Handler** does **not** call the Executor; it implements its own terminate/quarantine/network logic without those protections.

### 4.3 Command Result Acknowledgment

- **Execute** returns `*Result` (CommandID, Status, Output, Error, Duration, Timestamp). In **runCommandLoop**, the return value is discarded (`_ = a.commandHandler.Execute(...)`).
- **Protocol:** Proto has `ack_batch_id` (e.g. for batch ack); there is **no** RPC or stream message type observed in the agent code that sends command execution results back to the server. So the server cannot know from the agent whether a command succeeded, failed, or expired.

### 4.4 Other Gaps

- **Heartbeat:** `internal/grpc/heartbeat.go` exists; not traced in this report for full integration with StreamEvents (e.g. whether heartbeat is sent on the same stream or a separate channel).
- **Filtering:** Filter type exists and is used in Registry/Network collectors’ constructors, but those collectors are not started; ETW has no filter in the current wiring (events go straight to `eventChan`).
- **Batch ack:** Server may send batch acks; agent does not need to ack for correctness of send path, but there is no observed logic that tracks or reacts to acks (e.g. retry only unacked batches).

---

## 5. Production Readiness Conclusion

### 5.1 Critical Architectural Blockers

These items are **blockers** for deploying this agent in a **strict enterprise production** environment as-is:

1. **No offline persistent buffering**  
   Telemetry is dropped when the connection is down or send fails. Enterprise requirements often mandate no (or minimal) loss of security events during network partitions or server outages.

2. **Data loss when disconnected**  
   All batches produced after disconnect are discarded. Combined with a finite in-memory buffer and drop-when-full behavior, this makes the agent unsuitable for environments that require reliable delivery or auditability of events.

3. **No self-protection**  
   Process killable by admin; service can be stopped; config and certs can be deleted or tampered with. In high-security or regulated environments, EDR agents are expected to resist local tampering (or document accepted residual risk).

4. **Command results not reported**  
   Server cannot confirm success/failure/expiry of commands. Operational and compliance requirements typically need command auditability and status feedback.

5. **Stubbed security-critical commands**  
   UPDATE_AGENT, UPDATE_CONFIG, COLLECT_FORENSICS, ADJUST_RATE are non-functional or placeholders. Deploying with these exposed as “supported” is misleading and can create compliance or operational gaps.

6. **Collectors not wired despite config**  
   Registry, network, and WMI collection are advertised in config but not used. This creates a gap between documented behavior and actual coverage (e.g. no registry/network telemetry).

7. **Executor safety not used**  
   Handler does not use Executor’s protected PIDs/paths; risk of terminating critical processes or quarantining critical files is higher than intended.

8. **Enrollment over insecure transport**  
   Bootstrap uses plain TCP (insecure credentials). Acceptable only in controlled bootstrap networks; must be clearly documented and optionally restricted (e.g. network-level controls).

9. **No panic recovery in critical goroutines**  
   A panic in pipeline or collector can bring down the process; for a long-lived service, recover-and-log or restart policy is expected in production.

10. **Service recovery only restarts process**  
    Recovery actions restart the service on failure but do not address persistent failures (e.g. repeated crash loops, config corruption). No health reporting or circuit breaker to the management plane.

### 5.2 Summary Table

| Domain | Finding | Severity (production) |
|--------|---------|------------------------|
| Offline / resilience | No persistent queue; batches dropped when disconnected | **Critical** |
| Data loss | All post-disconnect batches lost; buffer full → event drop | **Critical** |
| Self-protection | None (process, service, files) | **Critical** (for strict enterprise) |
| Command result ack | Results not sent to server | **High** |
| Stubbed commands | UPDATE_AGENT, UPDATE_CONFIG, COLLECT_FORENSICS, ADJUST_RATE | **High** |
| Collectors unwired | Registry, Network, WMI | **High** |
| Executor unused | Safety checks not applied in Handler | **Medium** |
| Insecure enrollment | Bootstrap without TLS | **Medium** (if not constrained) |
| Panic recovery | None | **Medium** |

**Conclusion:** The agent is **not** ready for deployment in a **strict enterprise production** environment without addressing the critical blockers above (offline buffering, data loss, self-protection, command feedback, and completion of or explicit scoping of stubbed/unwired features). The report is objective and does not assume future work; each item should be explicitly accepted, deferred with documented risk, or resolved before production use.
