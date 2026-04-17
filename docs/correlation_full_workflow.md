# Correlation Full Workflow

## Purpose

This document explains the Sigma correlation system end-to-end in plain language:

- what correlation does
- where it runs in the pipeline
- the exact matching/scoring algorithm
- persistence model
- API behavior
- operational notes for production

---

## 1) What Correlation Is

Correlation links alerts that are likely related to the same attack chain.

Instead of handling isolated alerts only, SOC can see relationships like:

- same rule repeated over time
- same host producing multiple suspicious alerts
- same user involved in multiple alerts
- alerts close in time (time proximity)

Each relationship is an **edge** between two alert IDs.

---

## 2) Where It Runs

Primary path:

- `sigma_engine_go/internal/infrastructure/kafka/event_loop.go`

Execution order for matched alerts:

1. `DetectAggregated(...)`
2. `GenerateAggregatedAlert(...)`
3. `RiskScorer.Score(...)`
4. `CorrelationManager.CorrelateAlert(...)`
5. Suppression + publish + DB write

`CorrelateAlert` therefore runs on the final alert after scoring/enrichment.

Optional secondary path:

- `sigma_engine_go/internal/infrastructure/processor/parallel_processor.go`
- Uses `SetCorrelationManager(...)` if that processor is used.

---

## 3) Core Components

### 3.1 Correlation manager

File: `sigma_engine_go/internal/analytics/correlation.go`

Responsibilities:

- candidate discovery
- edge type decision
- score computation
- de-duplication
- in-memory cache/index maintenance
- optional persistence integration
- startup bootstrap

### 3.2 DB repository for edges/candidates

File: `sigma_engine_go/internal/infrastructure/database/correlation_repo.go`

Responsibilities:

- `UpsertEdge(...)`
- `ListRecentEdges(...)`
- `LoadRecentAlertsForCorrelation(...)`
- `LoadCandidateAlertsForCorrelation(...)`
- `PruneEdgesOlderThan(...)`

### 3.3 Correlation table

Migration:

- `sigma_engine_go/internal/infrastructure/database/migrations/016_sigma_alert_correlations.up.sql`
- `sigma_engine_go/internal/infrastructure/database/migrations/016_sigma_alert_correlations.down.sql`

Table:

- `sigma_alert_correlations`

Columns:

- `alert_low_id`
- `alert_high_id`
- `relation_type`
- `correlation_score`
- `created_at`
- `updated_at`

Uniqueness:

- one undirected pair only via unique `(alert_low_id, alert_high_id)`.

---

## 4) Algorithm

For each new alert, the manager evaluates candidate alerts and creates at most one edge per pair.

### 4.1 Time decay

```
timeDecay = exp(-deltaSeconds / 150)
```

- 150 seconds half-life behavior (stronger when closer in time).

### 4.2 Priority (first match wins)

Relationship decision order:

1. `same_rule`
2. `same_agent`
3. `same_user`
4. `time_based`

Only one relationship type is selected per pair.

### 4.3 Score formula per type

- `same_rule`: `0.85 * timeDecay`
- `same_agent`: `0.78 * timeDecay` (requires delta < 10 minutes)
- `same_user`: `0.65 * timeDecay` (requires delta < 10 minutes)
- `time_based`: `0.60 * timeDecay` (requires delta < 10 minutes)

Threshold:

- Keep edge only if score `> 0.1`.

### 4.4 User identity key

User correlation key is normalized as:

- `sid:<user_sid>` if available
- else `user:<user_name>`

---

## 5) Candidate Discovery

Candidates are collected from multiple sources:

1. In-memory index by rule (`byRule`)
2. In-memory index by agent (`byAgent`)
3. In-memory index by user (`byUser`)
4. In-memory alerts within 10-minute time window
5. Optional DB-backed candidate query (cross-instance enhancement)

This improves quality and allows better correlation when multiple engine replicas are running.

---

## 6) De-duplication Model

The system treats pairs as undirected:

- canonical pair = `(minID, maxID)`
- pair key = `low|high`

De-dup layers:

1. In-memory set `seenPairs` to prevent duplicate edge creation in one process
2. DB unique constraint + `ON CONFLICT` upsert

Upsert behavior:

- keeps the maximum score
- updates relation type if the newer score is stronger

---

## 7) Persistence and Bootstrap

In Kafka main:

- create shared `CorrelationManager`
- attach persistence adapter when DB is available
- call `Bootstrap(ctx)` at startup

Bootstrap actions:

1. load recent edges into memory
2. load recent alerts to warm cache/indexes
3. prune stale edges (retention cleanup)

Current retention constant:

- 7 days (`edgeRetention`)

---

## 8) Alert Enrichment Output

When new edges are found, the alert is enriched with:

- `correlation_summary`
  - `edges_added`
  - `primary_type`
  - `strongest_score`

Also mirrored into:

- `context_snapshot["correlation"]`

This allows storage and display without extra coupling.

---

## 9) API Behavior

Routes:

- `GET /api/v1/sigma/alerts/correlation`
  - returns analytics summary (counts)
- `GET /api/v1/sigma/alerts/correlation?alert_id=<id>`
  - returns edges touching that alert
- incident routes under `/api/v1/sigma/incidents...`

Notes:

- correlation edges are persisted
- incidents are currently in-memory handler state

---

## 10) Multi-Replica Behavior

What is solved:

- persisted edges in PostgreSQL
- startup bootstrap
- DB candidate lookups for cross-instance signal enrichment

What is still eventual (not fully shared real-time graph state):

- each process still has its own in-memory cache at runtime
- candidate DB lookup reduces but does not fully eliminate timing gaps between replicas

---

## 11) Operational Tuning (future)

Good next tuning candidates:

- move weights/window/threshold/retention to `config.yaml`
- add periodic scheduled prune job (not only bootstrap)
- add metrics:
  - edges by type
  - average score
  - candidate query latency
  - prune rows deleted

---

## 12) Quick Troubleshooting

If no correlations appear:

1. verify `event_loop.go` calls `SetCorrelationManager(...)`
2. check API route reachable: `/api/v1/sigma/alerts/correlation`
3. check `sigma_alert_correlations` table exists (migration 016 applied)
4. inspect alert data for missing `agent_id` / `user_sid` / `user_name`
5. validate timestamps are reasonable (time window/decay depends on delta)

---

## 13) Summary

Current correlation implementation is production-usable and includes:

- deterministic algorithm
- priority-based edge typing
- score decay over time
- pair de-duplication
- persistence with upsert semantics
- startup bootstrap
- retention pruning
- cross-instance candidate enrichment

This is a strong baseline for SOC triage and attack-chain visibility.

