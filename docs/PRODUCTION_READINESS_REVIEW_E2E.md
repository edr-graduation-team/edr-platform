# Production Readiness Review (PRR) — E2E Integration

**Date:** 2025-02-22  
**Scope:** win_edrAgent (client) and Connection Manager (server) after architectural hardening.  
**Purpose:** Strictly objective assessment for running the live End-to-End (E2E) integration test.

---

## 1. End-to-End Telemetry Pipeline & Data Integrity

### 1.1 Agent: Offline Disk Queue (WAL) and Sender Flow

**Verified:**

- **WAL integration:** Batches are written to disk before any send. In `internal/agent/agent.go`:
  - `processBatch(batch)` builds `pb.EventBatch` from the batcher output, including **Payload** (`batch.Payload`) and **Checksum** (`batch.Checksum`).
  - It then calls `a.diskQueue.Enqueue(pbBatch)`. No send occurs in `processBatch`.
- **Queue processor:** `runQueueProcessor()` runs in a dedicated goroutine: it calls `a.diskQueue.PeekOldest()`, then `a.grpcClient.SendBatchSync(a.ctx, pbBatch)`. On success it calls `a.diskQueue.Remove(filename)` and resets backoff. On failure it backs off (exponential, cap 30s) and retries without removing the file.
- **Batch content:** `internal/event/batcher.go` computes the checksum as SHA256 of the **compressed payload** and sets `batch.Checksum` (hex). `processBatch` assigns `Payload: batch.Payload` and `Checksum: batch.Checksum`, so the same bytes are used for checksum and payload end-to-end.

**Conclusion:** Batches are safely stored to the WAL first and sent synchronously by the queue processor. Payload and checksum are aligned (checksum is SHA256 of the exact payload bytes sent).

### 1.2 Server: Telemetry Ingestion and Checksum Validation

**Verified:**

- **Validation:** `pkg/handlers/event_ingestion.go` — `validateBatch(batch)` requires non-empty `batch_id`, `agent_id`, positive `event_count`, non-empty `payload`, and payload size ≤ 10MB.
- **Checksum:** When `batch.Checksum != ""`, the handler calls `verifyChecksum(batch.Payload, batch.Checksum)`, which computes SHA256 of the raw payload, hex-encodes it, and compares to `batch.Checksum`. Mismatch returns `codes.InvalidArgument` with "checksum mismatch".
- **Order:** Checksum is verified on the **compressed** payload (before decompression for Kafka/DB), matching the agent’s contract (checksum of the bytes in `EventBatch.Payload`).

**Conclusion:** Server correctly validates structure and, when present, verifies payload integrity via SHA256 checksum. No data integrity blockers for E2E.

---

## 2. Context & Identity Alignment

### 2.1 Shared Context Key and “Unknown” Agent ID Fix

**Verified:**

- **Shared package:** `connection-manager/pkg/contextkeys/keys.go` defines `type ContextKey string` and `const AgentIDKey ContextKey = "agent_id"`.
- **Interceptors** (`pkg/server/interceptors.go`): Both `AuthUnaryInterceptor` and `AuthStreamInterceptor` set the agent ID with `context.WithValue(ctx, contextkeys.AgentIDKey, agentID)` (no local `ContextKeyAgentID`; the shared key is used).
- **Event handler** (`pkg/handlers/event_ingestion.go`): `extractAgentIDFromContext` uses `ctx.Value(contextkeys.AgentIDKey).(string)`. Same key type as the interceptors.
- **SendCommandResult** (`pkg/server/server.go`): Reads agent ID with `ctx.Value(contextkeys.AgentIDKey).(string)`.

**Conclusion:** The previous bug (handler using string `"agent_id"` while interceptors used a different key type) is resolved. Stream and unary paths use `contextkeys.AgentIDKey` consistently. The “unknown” agent ID issue in StreamEvents is fixed.

---

## 3. Command & Control (C2) Feedback Loop

### 3.1 CommandResult and SendCommandResult Alignment

**Verified:**

- **Proto (both sides):** Agent `internal/proto/v1/edr.proto` and Connection Manager `proto/v1/edr.proto` both define `CommandResult` with `command_id`, `agent_id`, `status`, `output`, `error`, `duration` (Duration), `timestamp` (Timestamp), and `rpc SendCommandResult(CommandResult) returns (google.protobuf.Empty)`.
- **Server implementation:** `pkg/server/server.go` implements `SendCommandResult(ctx, req *edrv1.CommandResult) (*edrv1.Empty, error)`. Connection Manager stub (`proto/v1/stub.go`) includes `CommandResult`, `Empty`, and `SendCommandResult` in the service interface and unimplemented stub.

### 3.2 Server Anti-Spoofing Check

**Verified:**

- **Logic:** `SendCommandResult` reads `ctxAgentID` from `ctx.Value(contextkeys.AgentIDKey)`. If empty, it returns `Unauthenticated`. If `req.AgentId != ""` and `req.AgentId != ctxAgentID`, it logs a warning (context vs payload agent ID) and returns `PermissionDenied` with message "agent ID in payload does not match authenticated agent". Otherwise it logs success and returns `&edrv1.Empty{}`.
- **Auth:** SendCommandResult is a unary RPC; it is **not** in the RegisterAgent auth-bypass list, so it runs through `AuthUnaryInterceptor` and gets the agent ID from mTLS.

**Conclusion:** Anti-spoofing is implemented and correctly enforced.

### 3.3 Agent: Sending Command Results

**Verified:**

- **Command loop:** In `internal/agent/agent.go`, `runCommandLoop()` does `result := a.commandHandler.Execute(a.ctx, c)` and, if `result != nil`, calls `a.grpcClient.SendCommandResult(a.ctx, result, a.cfg.Agent.ID)`; on error it logs a warning.
- **Client:** `internal/grpc/client.go` — `SendCommandResult(ctx, res *command.Result, agentID string)` builds the request with `pb.NewCommandResultProto(res.CommandID, agentID, res.Status, res.Output, res.Error, res.Duration, res.Timestamp)` and calls `conn.Invoke(ctx, pb.EventIngestionService_SendCommandResult_FullMethodName, req, out)` with `out = &emptypb.Empty{}`.
- **Dynamic proto:** `internal/pb/command_result.go` builds a `CommandResult` message at runtime (descriptor built with `descriptorpb`/`protodesc`, message with `dynamicpb`), sets all seven fields including `durationpb.New(duration)` and `timestamppb.New(timestamp)`, and returns a `proto.Message` suitable for gRPC Invoke.

**Conclusion:** The agent correctly maps `*command.Result` to the RPC request and sends it over the wire via the dynamic protobuf implementation. No type or wiring gaps for E2E.

---

## 4. Resilience & Hardening

### 4.1 Panic Recovery in Agent

**Verified:**

- **Pipeline goroutines** (`internal/agent/agent.go`): Each of the following is wrapped in a goroutine with `defer a.wg.Done()` and `defer func() { if r := recover(); ... a.logger.Errorf("Panic recovered in <name>: %v", r) }()`:
  - `RunReconnector`
  - `RunStream`
  - `runCommandLoop`
  - `RunSender` (gRPC client sender)
  - `runQueueProcessor`
- **Platform collectors** (`internal/agent/agent_windows.go`): ETW, Registry, Network, and WMI each run in a goroutine with `defer func() { if r := recover(); ... logger.Errorf("<Collector> panicked and was safely recovered: %v", r) }()` before calling `collector.Start(ctx)`.

**Gap (acceptable for v1.0):** `runBatcher` and the agent’s ticker-driven `runSender()` (which only calls `processBatch` and does not touch the network) are started without panic recovery. They were not in the explicit hardening list. Risk is limited to batcher/sender logic; recommend adding recovery in a follow-up.

### 4.2 Registry and Network Collectors with Global Filter

**Verified:**

- **Filter creation:** In `startPlatformCollectors`, `evtFilter := collectors.NewFilter(collectors.FilterConfig{...})` is built from `cfg.Filtering` (ExcludeProcesses, ExcludeIPs, ExcludeRegistry, ExcludePaths, IncludePaths).
- **Registry:** `collectors.NewRegistryCollector(eventChan, evtFilter, logger)` — filter is passed (not nil).
- **Network:** `collectors.NewNetworkCollector(eventChan, evtFilter, logger)` — filter is passed (not nil).
- **ETW:** Still no filter parameter in the current API; ETW does not accept a filter. Documented as known limitation.

**Conclusion:** Registry and Network are wired with the global event filter. Noise reduction is in place for those collectors.

---

## 5. Final Production Readiness (Go/No-Go)

### 5.1 Architectural Blockers and Type Safety

- **Builds:** Both `win_edrAgent` and `connection-manager` have been built successfully (`go build ./...`) in recent changes. No missing imports or type mismatches were identified for the E2E path.
- **Agent → Server telemetry:** EventBatch (batch_id, agent_id, payload, checksum, compression, etc.) is produced by the agent and consumed by the server with consistent semantics. Checksum is optional on the server (validated when present).
- **Server → Agent commands:** CommandBatch/Command are received by the agent; the agent executes and sends back CommandResult. Server accepts CommandResult and enforces agent identity.
- **Identity:** Shared context key is used end-to-end on the server; no remaining “unknown” agent ID in the StreamEvents path for authenticated streams.

### 5.2 Acceptable Stubs / Technical Debt for v1.0

- **C2 command implementations:** The agent’s `command.Handler` implements several command types (e.g. TERMINATE_PROCESS, QUARANTINE_FILE, COLLECT_FORENSICS, etc.). Some server-issued command types (e.g. UPDATE_AGENT, UPDATE_CONFIG, RESTART_SERVICE, ADJUST_RATE) may be partially implemented or return “not implemented” style results. Acceptable for v1.0 as long as the agent always returns a Result and sends it via SendCommandResult.
- **RequestCertificateRenewal:** Connection Manager handler is still a stub (returns placeholder cert fields). Does not block E2E for registration + StreamEvents + SendCommandResult.
- **Agent proto generation:** Agent uses a runtime-built CommandResult (`internal/pb/command_result.go`) instead of generated code. Functional for E2E; replacing with `make proto`-generated types is technical debt.

### 5.3 Go/No-Go Decision

- **E2E telemetry:** WAL → queue processor → SendBatchSync; server validates and verifies checksum. **Go.**
- **Identity:** contextkeys.AgentIDKey used consistently; “unknown” bug resolved. **Go.**
- **C2 feedback:** SendCommandResult implemented on server with anti-spoofing; agent sends result via dynamic proto and Invoke. **Go.**
- **Resilience:** Panic recovery on critical pipeline and collector goroutines; Registry/Network filtered. **Go.**
- **No unresolved blockers** were found for running the live integration test (registration, StreamEvents, command receive, SendCommandResult, checksum validation).

**Decision: Go** — Proceed with the live E2E integration test. The current state of both codebases supports a successful test of the hardened telemetry pipeline, identity handling, and C2 feedback loop, with known technical debt and minor hardening follow-ups (runBatcher/runSender recovery, ETW filter if desired) acceptable for v1.0.
