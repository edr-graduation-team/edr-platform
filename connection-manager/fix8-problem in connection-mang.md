# Connection Manager — Audit Remediation Walkthrough

## Summary

Remediated 7 of 8 audit findings (Finding #4 — `VerifyClientCertIfGiven` — kept as-is per user decision to support current enrollment flow).

## Changes by File

### [server.go](file:///d:/EDR_Platform/connection-manager/pkg/server/server.go) — Finding #6

Added `grpc.MaxRecvMsgSize(11 * 1024 * 1024)` to match the 10MB payload limit in [validateBatch()](file:///d:/EDR_Platform/connection-manager/pkg/handlers/event_ingestion.go#702-726). Previously, gRPC's default 4MB limit silently rejected valid batches.

---

### [event_ingestion.go](file:///d:/EDR_Platform/connection-manager/pkg/handlers/event_ingestion.go) — Findings #1, #3

**Per-batch rate limiting (#1)**: Added `rateLimiter.Allow(ctx, agentID, eventCount)` inside the `stream.Recv()` loop, consuming `batch.EventCount` tokens per batch. Exceeding the limit drops the batch (logged, metrics recorded) but keeps the stream alive.

**JSON schema validation (#3)**: Added [validateEventSchema()](file:///d:/EDR_Platform/connection-manager/pkg/handlers/event_ingestion.go#746-781) that enforces:
- Required fields: `event_type`, `timestamp`, `severity` (all strings)
- `event_type` must be non-empty
- Per-event size limit: 1 MB
- Invalid events are dropped individually; valid events proceed to Kafka

**Async fallback (#2)**: [storeToFallback()](file:///d:/EDR_Platform/connection-manager/pkg/handlers/event_ingestion.go#786-823) now calls the non-blocking [Store()](file:///d:/EDR_Platform/connection-manager/pkg/handlers/event_fallback.go#88-110) which enqueues to a bounded channel.

---

### [event_fallback.go](file:///d:/EDR_Platform/connection-manager/pkg/handlers/event_fallback.go) — Findings #2, #7

**Async fallback (#2)**: Rewrote fundamentally:
- [Store()](file:///d:/EDR_Platform/connection-manager/pkg/handlers/event_fallback.go#88-110) is now **non-blocking** — pushes to a bounded channel (4096 items)
- 4 worker goroutines drain the channel and perform DB INSERTs
- When channel is full, batches are dropped (vs. blocking the gRPC stream)
- [Close()](file:///d:/EDR_Platform/win_edrAgent/internal/logging/logger.go#128-137) signals shutdown and waits for workers to drain

**Replay worker (#7)**: New [FallbackReplayWorker](file:///d:/EDR_Platform/connection-manager/pkg/handlers/event_fallback.go#197-204):
- Runs every 30s, reads 100 unreplayed rows per cycle
- Re-publishes each event to Kafka with `replay=true` header
- Marks successfully published rows as `replayed=true`
- Uses `FOR UPDATE SKIP LOCKED` to prevent concurrent replay conflicts

---

### [interceptors.go](file:///d:/EDR_Platform/connection-manager/pkg/server/interceptors.go) — Finding #5

**Cert revocation fail-closed (#5)**: Replaced fail-open with layered approach:
1. **Redis available** → live check + update local `sync.Map` cache
2. **Redis down, cache fresh (<5 min)** → use local cache
3. **Redis down, cache stale (>5 min)** → **REJECT** connection (fail-closed)

Added [AddToRevocationCache()](file:///d:/EDR_Platform/connection-manager/pkg/server/interceptors.go#354-360) for immediate cache updates on REST API cert revocation.

---

### [main.go](file:///d:/EDR_Platform/connection-manager/cmd/server/main.go) — Finding #8

**Admin seed fix (#8)**: Changed from `ON CONFLICT DO UPDATE SET password_hash=...` to `ON CONFLICT DO NOTHING`. Admin password changes now persist across reboots.

Also wired: `fallback.Close()` for graceful shutdown and [FallbackReplayWorker](file:///d:/EDR_Platform/connection-manager/pkg/handlers/event_fallback.go#197-204) goroutine startup.

## Verification

- ✅ `go build ./cmd/server/` — exit code 0
- ✅ `go test ./config/` — PASS
