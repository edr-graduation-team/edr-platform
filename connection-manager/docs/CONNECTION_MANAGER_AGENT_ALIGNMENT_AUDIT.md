# Connection Manager – Agent Alignment Audit Report

**Date:** 2025-02-21  
**Scope:** Static analysis and architecture audit of the Connection Manager server against the hardened `win_edrAgent` client.  
**Objective:** Identify protobuf alignment, enrollment pipeline completeness, telemetry ingestion behavior, and readiness gaps that would cause the agent to fail connecting or transmitting data.

---

## 1. Protobuf Alignment

### 1.1 Service and RPCs

| RPC | Agent expectation | Connection Manager | Status |
|-----|-------------------|--------------------|--------|
| `RegisterAgent` | Unary, bootstrap (no mTLS) | Defined in `proto/v1/edr.proto`; implemented in `pkg/server/server.go` → `AgentService.Register` | **Aligned** |
| `StreamEvents` | Bidirectional stream: client sends `EventBatch`, server sends `CommandBatch` | Same in `edr.proto`; implemented via `EventHandler.StreamEvents` | **Aligned** |
| `SendCommandResult` | Unary: agent sends `CommandResult`, server returns `google.protobuf.Empty` | **Not defined** in Connection Manager `proto/v1/edr.proto` or `proto/v1/stub.go` | **MISSING** |

The agent’s proto (e.g. `win_edrAgent/internal/proto/v1/edr.proto`) defines:

```protobuf
rpc SendCommandResult(CommandResult) returns (google.protobuf.Empty) {}
message CommandResult {
  string command_id = 1;
  string agent_id = 2;
  string status = 3;
  string output = 4;
  string error = 5;
  google.protobuf.Duration duration = 6;
  google.protobuf.Timestamp timestamp = 7;
}
```

The Connection Manager’s `proto/v1/edr.proto` does **not** include this RPC or the `CommandResult` message. The hand-written `proto/v1/stub.go` implements `EventIngestionServiceServer` with only:

- `StreamEvents`
- `Heartbeat`
- `RequestCertificateRenewal`
- `RegisterAgent`

**Impact:** When the agent calls `SendCommandResult` (e.g. after implementing the real RPC instead of the current log-only stub), the server will respond with **Unimplemented** (or the equivalent for the stub-based setup). The C2 feedback path will fail at the RPC boundary until the server proto and implementation are updated.

### 1.2 Message Types Used by the Agent

- **EventBatch:** Both sides define the same logical fields (`batch_id`, `agent_id`, `timestamp`, `compression`, `payload`, `event_count`, `metadata`, `checksum`). The Connection Manager proto and stub are consistent for the fields used in `event_ingestion.go` (`BatchId`, `AgentId`, `Compression`, `Payload`, `EventCount`, `Metadata`, `Checksum`). **Aligned.**
- **CommandBatch:** Server sends it on the stream; agent consumes it. Structure matches. **Aligned.**
- **AgentRegistrationRequest / AgentRegistrationResponse:** Field names and semantics match (installation_token, csr, hostname, certificate, ca_chain, agent_id, status, etc.). **Aligned.**

---

## 2. Enrollment Pipeline

The agent design assumes:

1. One **insecure** gRPC call to `RegisterAgent` with a bootstrap token.
2. Server validates token, signs the CSR with a local CA, returns `AgentID` and signed mTLS certificates.
3. Agent then uses those certs for all subsequent calls (e.g. `StreamEvents`).

### 2.1 Request Path and Auth Bypass

- **RPC:** `RegisterAgent` is implemented in `pkg/server/server.go` and delegates to `s.agentService.Register(ctx, svcReq)`.
- **Auth:** In `pkg/server/interceptors.go`, `AuthUnaryInterceptor` explicitly skips auth for `RegisterAgent`:
  ```go
  if info.FullMethod == "/edr.v1.EventIngestionService/RegisterAgent" {
      return handler(ctx, req)
  }
  ```
  So the initial registration call does **not** require mTLS or JWT. **Aligned** with the agent’s bootstrap flow.

### 2.2 Token Validation and Agent Record

- **Token:** `internal/service/agent_service.go` → `Register` uses `tokenRepo.GetByValue(ctx, req.InstallationToken)`. Invalid or missing token returns `ErrInvalidToken`. Token validity (e.g. not expired, not already used) is enforced via `token.IsValid()`.
- **Duplicate hostname:** `agentRepo.GetByHostname(ctx, req.Hostname)` is used; if an agent with that hostname exists, registration fails with `ErrDuplicateAgent`.
- **Agent record:** A new agent is created with `uuid.New()` as ID, status `AgentStatusPending`, and hostname/OS/version/CSR metadata. The agent’s proposed `agent_id` from the proto is **not** used; the server always generates a new UUID. The response maps this ID back to the client as `AgentId` in `AgentRegistrationResponse`.

**Note:** The agent may send a proposed `agent_id` in the request; the server ignores it and returns its own generated UUID. This is acceptable as long as the agent persists and uses the returned `agent_id` for all later operations (StreamEvents, SendCommandResult, etc.).

### 2.3 CSR Signing and Certificate Return

- **Certificate service:** `internal/service/certificate_service.go` implements `CertificateService.Issue(ctx, agentID, csrPEM)`.
  - Loads a **local CA** from configurable paths (`caCertPath`, `caKeyPath`) at construction.
  - Parses the PEM CSR, validates the CSR signature, builds an x.509 certificate template (client auth, 90-day validity), and signs it with the CA private key.
  - Returns PEM-encoded certificate and CA chain (`IssuedCertificate.Certificate`, `CACert`).
- **Wiring:** In `cmd/server/main.go`, when the DB is available:
  - `caCertPath := cfg.Server.CACertPath`
  - `caKeyPath := filepath.Join(filepath.Dir(caCertPath), "ca.key")`
  - `certSvc := service.NewCertificateService(..., caCertPath, caKeyPath)`
  - `agentSvc = service.NewAgentService(..., certSvc)`
- **Agent service behavior:** In `agent_service.Register`, after creating the agent and marking the token used, if `certService != nil` and `len(req.CSRData) > 0`, it calls `certService.Issue(ctx, agentID, req.CSRData)`. On success it sets `resp.Status = RegistrationStatusApproved`, `resp.Certificate = issued.Certificate`, `resp.CACert = issued.CACert`, and returns. So the **enrollment pipeline is fully implemented**: token validation, agent record creation, CSR signing with local CA, and return of certs + AgentID.

**Gap:** If the CA files are missing or invalid, `loadCA()` logs a warning and `Issue()` will return an error; the agent service then returns **pending** status without certificates. The agent would then not receive certs and could not establish mTLS for `StreamEvents`. Operators must ensure `cfg.Server.CACertPath` and the adjacent `ca.key` are present and loadable.

### 2.4 Unused Registration Handler

- `pkg/handlers/registration.go` defines `RegistrationHandler.RegisterAgent`, which stores the CSR for “admin approval” and **always** returns `REGISTRATION_STATUS_PENDING` with no certificate. This handler is **not** used by the gRPC server. The server uses `service.AgentService` only. So the “pending approval” path in that file is dead code for the current wiring; the live path is auto-approve + sign CSR when CA is configured.

---

## 3. Telemetry Ingestion

### 3.1 Flow

- **Entry:** `EventHandler.StreamEvents` (in `pkg/handlers/event_ingestion.go`) receives batches via `stream.Recv()`.
- **Per batch:** `processBatch(ctx, agentID, batch)` is called. The `agentID` used here comes from `extractAgentIDFromContext(stream.Context())` (see Section 4.1 for a critical bug).

### 3.2 Validation and Checksum

- **Structural validation** (`validateBatch`): Requires non-empty `batch_id`, `agent_id`, `event_count`, non-empty `payload`, and payload size ≤ 10MB. **Aligned** with the agent sending these fields.
- **Checksum:** When `batch.Checksum != ""`, the server calls `verifyChecksum(batch.Payload, batch.Checksum)`, which computes SHA256 of the **raw payload bytes** (the compressed blob), hex-encodes it, and compares to `batch.Checksum`. The agent is designed to send `EventBatch.Payload` as the compressed bytes and `EventBatch.Checksum` as the SHA256 hash of that same payload (e.g. hex-encoded). So **checksum semantics are aligned**; if the agent sends the checksum, the server validates it correctly.
- **Compression:** Server supports `COMPRESSION_NONE`, `COMPRESSION_GZIP`, and `COMPRESSION_SNAPPY`. Decompression is applied for Kafka/DB only where needed; checksum is verified on the **compressed** payload before decompression, which matches the agent sending SHA256 of the compressed bytes.

### 3.3 Routing and Storage

- **Primary:** If `kafkaProducer != nil`, the batch is serialized (JSON of the proto batch) and sent via `kafkaProducer.SendEventBatch(ctx, batch.AgentId, kafkaPayload, headers)`. So telemetry is routed to **Kafka** with `agent_id` as the partition key.
- **DLQ:** The Kafka producer is configured to use a DLQ on primary send failure (referenced in comments and producer implementation).
- **Fallback:** If Kafka send fails (or Kafka is nil), the handler calls `storeToFallback(ctx, batch, payload)`, which writes to **PostgreSQL** via `EventFallbackStore` when configured. So routing is: Kafka (primary) → Kafka DLQ (on failure) → PostgreSQL fallback when Kafka is unavailable or disabled.

### 3.4 Duplicate Handling and Metrics

- Duplicate detection uses Redis (`redis.IsBatchProcessed(batch.BatchId)`). If already processed, the batch is ignored (idempotent).
- After successful processing, the batch is marked in Redis with `SetBatchProcessed(ctx, batch.BatchId, 24*time.Hour)`.
- Metrics: `RecordEventBatch(eventCount, len(batch.Payload))` is called when the batch is accepted.

---

## 4. Readiness and Gaps

### 4.1 Critical: Agent ID in Stream Context

- **Interceptor** (`pkg/server/interceptors.go`): For streaming RPCs, `AuthStreamInterceptor` sets the agent ID in context with `context.WithValue(ctx, ContextKeyAgentID, agentID)`, where `ContextKeyAgentID` is a **custom type**: `type contextKey string` with value `"agent_id"`.
- **EventHandler** (`pkg/handlers/event_ingestion.go`): Uses `extractAgentIDFromContext(ctx)` which does `ctx.Value("agent_id").(string)`. The key used here is the **string** `"agent_id"`, not the server’s `ContextKeyAgentID`. In Go, context keys are compared by identity; a `contextKey` and a `string` are different keys, so `ctx.Value("agent_id")` will **always** return `nil`. As a result, `agentID` in `StreamEvents` is **always** `"unknown"`.
- **Impact:** Redis status updates (`SetAgentStatus(ctx, agentID, "online"|"offline")`) and any logging that use this `agentID` will see `"unknown"` for every stream. Kafka and fallback storage use `batch.AgentId` from the message, so **telemetry is still stored and partitioned by the batch’s agent_id**. The main failure is loss of correct real-time agent presence and any logic that relies on the stream context’s agent ID.

**Recommendation:** Either export a shared context key (e.g. in a small `pkg/contextkeys` package) and use it in both server and handlers, or have the handler use the same key type as the server (e.g. by importing the server package or duplicating the key type and value). Then use that key in `extractAgentIDFromContext`.

### 4.2 Critical: SendCommandResult Not Implemented

- The agent is designed to call `SendCommandResult` with `CommandResult` after executing each C2 command. The Connection Manager does **not** define this RPC in its proto or stub, and has no handler for it.
- **Impact:** When the agent’s client is updated to call the real RPC (instead of the current log-only stub), the server will return **Unimplemented** (or equivalent). Command execution results will not be received by the server.

**Recommendation:** Add to Connection Manager’s `proto/v1/edr.proto`: the `CommandResult` message (and optionally import `google/protobuf/duration.proto`, `google/protobuf/empty.proto`), and the RPC `rpc SendCommandResult(CommandResult) returns (google.protobuf.Empty);`. Regenerate or update `stub.go` to include the new method and message. Implement a handler (e.g. store results in DB or forward to a queue) and wire it in the server so the method is not Unimplemented.

### 4.3 Optional: Checksum Required vs Optional

- Server currently verifies checksum only when `batch.Checksum != ""`. The agent is expected to always send the SHA256 checksum. If the agent were to send an empty checksum, the server would skip verification and still accept the batch. No change required for current agent design; document that sending the checksum is recommended for integrity.

### 4.4 Certificate Renewal (RequestCertificateRenewal)

- The agent may call `RequestCertificateRenewal` for cert rotation. In Connection Manager, `pkg/handlers/registration.go` implements `CertRenewalHandler.RequestCertificateRenewal` with TODOs: no real CSR parsing, no CA signing, no certificate storage; it returns a response with nil certificate and CA chain. This is a **stub**. It does not block the main enrollment or StreamEvents path but will not support renewal until implemented.

### 4.5 Summary Table: What Will Cause the Hardened Agent to Fail

| Gap | Severity | Effect |
|-----|----------|--------|
| **SendCommandResult RPC missing** | **High** | C2 result feedback will fail once the agent calls the RPC; server returns Unimplemented. |
| **Agent ID in stream context** | **Medium** | Stream context agent_id is always "unknown"; Redis status and any context-based logic wrong; telemetry still stored correctly via `batch.AgentId`. |
| **CA not configured** | **High** | RegisterAgent returns pending without certs; agent cannot do mTLS and cannot open StreamEvents. |
| **RequestCertificateRenewal stub** | Low | Renewal does not work; does not affect initial connect or StreamEvents. |

---

## 5. Conclusion

- **Protobuf:** `StreamEvents` and `RegisterAgent` are aligned. **SendCommandResult** and **CommandResult** are missing on the server and must be added for C2 feedback.
- **Enrollment:** RegisterAgent is fully implemented: token validation, agent creation, CSR signing with local CA, and return of AgentID and certificates. Correct configuration of CA paths is required for issuance.
- **Telemetry:** EventBatch is validated (including optional SHA256 checksum of payload); data is routed to Kafka (with DLQ) and PostgreSQL fallback. Checksum and compression behavior match the agent’s contract.
- **Readiness:** The two changes that are **required** for the hardened agent are: (1) add and implement **SendCommandResult** in the Connection Manager proto and server, and (2) fix **agent ID in stream context** so the handler uses the same context key as the interceptor. Ensuring CA cert and key are present is an operational requirement for enrollment to succeed.
