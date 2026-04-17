# Production Math Contract (Full)

This document is the single production reference for all quantitative logic in the EDR platform:
- what each formula is,
- why it exists,
- which fields feed it,
- where it is computed,
- where it is stored and rendered,
- and what legacy formulas were replaced.

---

## 0) Conventions

- `clamp(x, a, b)` means `max(a, min(x, b))`.
- Scores in `0..100` are operational risk/health scores.
- Probabilities in `0..1` are statistical likelihood values.
- "Source of truth" for dashboard state is database/API responses, not websocket payloads.

---

## 1) Detection Confidence (rule-event match confidence)

### Why used
To estimate quality of a single rule match before higher-level aggregation and risk scoring.

### Formula
`confidence = clamp(base_confidence(level) * field_match_factor * context_score, 0, 1)`

### Inputs and fields
- `base_confidence(level)` from Sigma rule level:
  - `critical=0.95`, `high=0.85`, `medium=0.65`, `low=0.45`, `informational=0.25`, default `0.50`
- `field_match_factor = matched_unique_fields / total_unique_non_filter_fields`
- `context_score` (optional, when context validation is enabled):
  - parent field missing => `score *= 0.8`
  - command line missing => `score *= 0.85`
  - user missing => `score *= 0.9`

### Where calculated
- `sigma_engine_go/internal/application/detection/detection_engine.go`
  - `calculateConfidence()`
  - `getLevelConfidence()`
  - `validateContext()`

### Where used
- stored in each match object (`RuleMatch.Confidence`)
- used later by aggregated confidence in event-level aggregation.

---

## 2) Aggregated Confidence (multi-rule per event)

### Why used
One event can match multiple rules. This score captures corroboration strength with diminishing returns.

### Formula
`combined_confidence = min(max_confidence + min(0.15 * ln(match_count), 0.3), 1.0)`

### Inputs and fields
- `max_confidence`: highest confidence among matches.
- `match_count`: number of matched rules for same event.

### Where calculated
- `sigma_engine_go/internal/domain/detection_result.go`
  - `EventMatchResult.CombinedConfidence()`

### Where used
- mapped into aggregated alert metadata (`combined_confidence`).

---

## 3) Context-Aware Risk Score (final alert priority)

### Why used
Produces SOC-priority risk score with explainable components.

### Formula (current production)
1) Base raw score:

`raw = base_score + lineage_bonus + privilege_bonus + burst_bonus + ueba_bonus + interaction_bonus - fp_discount - ueba_discount`

2) Hybrid context policy multiplier:

`context_adjusted = round(raw * context_multiplier)`

3) Final score:

`risk_score = clamp(context_adjusted, 0, 100)`

### Context multiplier details
`context_multiplier = clamp(user_role_weight * device_criticality_weight * network_anomaly_factor, 0.25, 4.0)`

Additional network trust adjustment:
- source IP inside trusted CIDR => `network_anomaly_factor *= 0.9`
- otherwise => `network_anomaly_factor *= 1.1`
- then bounded `0.5..2.0` per factor.

### Inputs and fields
- Severity and rule metadata: `rule.level`, `match_count`
- Event/process context:
  - `pid`, `user_sid`, `integrity_level`, `is_elevated`, `signature_status`, `executable`
- Lineage context from cache:
  - parent/ancestor chain
- Temporal burst:
  - per `(agent_id, rule_category)` 5-minute counter
- UEBA baseline:
  - `(agent_id, process_name, hour)` profile
- Policy context:
  - `scope_type/scope_value`, policy weights, trusted networks

### Where calculated
- `sigma_engine_go/internal/application/scoring/risk_scorer.go`
- Policy factor resolution:
  - `sigma_engine_go/internal/application/scoring/context_policy_provider.go`

### Where stored
- `sigma_alerts.risk_score`
- `sigma_alerts.context_snapshot` (JSONB)
- `sigma_alerts.score_breakdown` (JSONB)

### Where used in UI
- `Alerts` details and sorting logic
- `Endpoint Risk Intelligence` aggregation
- stats (`avg_confidence` currently based on normalized `risk_score` mean)

---

## 4) Risk Score Components (Detailed)

### 4.1 Base Score

#### Formula
Severity mapping:
- informational `10`
- low `25`
- medium `45`
- high `65`
- critical `85`
- default `35`

Multi-rule bonus:
- `+5` per additional rule
- capped at `+15`

#### Why
Severity gives prior risk, multi-match gives correlation bonus.

#### Where
- `computeBaseScore()` in `risk_scorer.go`

---

### 4.2 Lineage Bonus

#### Formula
Matrix-driven suspicious parent-child pattern bonus from lineage chain.

#### Why
Execution ancestry often distinguishes benign admin activity from attack chains.

#### Where
- `SuspicionMatrix.ComputeBonus()` and usage in `risk_scorer.go`

---

### 4.3 Privilege Bonus

#### Formula (cumulative, capped at 40)
- SYSTEM SID => `+20`
- builtin admin SID (`-500`) => `+15`
- integrity `system` => `+15`
- integrity `high` + elevated => `+10`
- elevated token (non-system) => `+10`
- signature `unsigned` => `+15`
- signature missing/unknown with executable present => `+8`
- cap total component at `40`

#### Why
Privilege/elevation anomalies increase impact and attacker capability.

#### Where
- `computePrivilegeBonus()` in `risk_scorer.go`

---

### 4.4 Burst Bonus

#### Formula
- count >= 30 => `+30`
- count >= 10 => `+20`
- count >= 3 => `+10`
- else `0`

Window is 5 minutes.

#### Why
Bursting detections in short windows often means active attack phase.

#### Where
- `computeBurstBonus()` in `risk_scorer.go`
- tracker in `burst_tracker.go`

---

### 4.5 False Positive Discount

#### Formula (subtracted from score)
- Microsoft signed => `+15` discount
- Microsoft + system path (`system32/syswow64/sysnative`) => additional `+10`
- trusted third-party signed => `+8`
- cap discount at `30`

#### Why
Reduce triage noise from highly trusted binaries/paths.

#### Where
- `computeFPDiscount()` in `risk_scorer.go`

---

### 4.6 False Positive Risk Probability

#### Formula (stored probability, not score points)
- microsoft + system path => `0.70`
- microsoft non-system path => `0.45`
- trusted => `0.30`
- unsigned => `0.05`
- unknown => `0.15`

Suppression gate:
- `suppress = (false_positive_risk >= 0.70) OR explicit_suppressed_flag`

#### Why
Probabilistic suppression control separate from score arithmetic.

#### Where
- `computeFPRisk()` in `risk_scorer.go`
- suppression check in `sigma_engine_go/internal/domain/alert.go` (`ShouldSuppress`)

---

### 4.7 UEBA Bonus / Discount

#### Behavior
For process-hour baseline:
- anomaly => `ueba_bonus = +15`
- normal => `ueba_discount = +10` (subtracted)
- none => `0`

Anomaly criterion uses z-score; normal criterion uses within 1 sigma (or stable average case).

#### Why
Adds behavioral learning beyond static rules.

#### Where
- `computeUEBA()` in `risk_scorer.go`
- baseline storage in `baseline_repository.go`

---

### 4.8 Interaction Bonus (cross-signal nonlinearity)

#### Formula
Count strong dimensions among:
- lineage bonus >= 20
- privilege bonus >= 15
- ueba bonus > 0
- burst bonus >= 20

Then:
- 3+ strong signals => `+15`
- 2 strong signals => `+10`
- else `0`

#### Why
Multiple high-risk signals together are more meaningful than each in isolation.

#### Where
- `computeInteractionBonus()` in `risk_scorer.go`

---

## 5) UEBA Baseline Mathematics

### Why used
Model expected process behavior per endpoint and hour to classify anomaly vs normal.

### Core formulas
- EMA mean:
  - `new_avg = 0.90 * old_avg + 0.10 * 1.0`
- EWM variance/stddev:
  - `sigma_new = sqrt((1-alpha)*sigma_old^2 + alpha*(x - mu_new)^2), alpha=0.10, x=1.0`
- Confidence maturation:
  - `confidence = min(1 - exp(-observation_days / 7), 0.99)`
- `observation_days` increments only when UTC date advances for that baseline key.

### Fields used
- `agent_id`, `process_name`, `process_path`, `hour_of_day`
- `sig_status`, `integrity_level`, `is_elevated`, `parent_name`, `observed_at`

### Where calculated
- `sigma_engine_go/internal/application/baselines/baseline_repository.go` (UPSERT SQL math)

### Where used
- read by risk scorer to emit UEBA bonus/discount and `ueba_signal`.

---

## 6) Agent Health Score (Endpoint Health)

### Why used
Represents telemetry reliability + agent operational state for endpoint operations.

### Formula
`health_score = delivery*0.40 + status*0.30 + drop*0.20 + resource*0.10`

Components:
- `delivery = min(events_sent / events_generated * 100, 100)`
- `status` mapping:
  - healthy `100`, degraded `80`, critical `50`, default `60`
- `drop`:
  - drop rate > 20% => `0`
  - 5%-20% => linear `(0.20 - rate)/0.15 * 100`
  - <5% => `100`
- `resource`:
  - CPU and memory penalties reduce from 100, floor at 0

### Fields used
- `events_generated`, `events_sent`, `events_dropped`
- heartbeat status
- `cpu_usage`, `memory_used_mb`, `memory_total_mb`

### Where calculated
- `connection-manager/pkg/handlers/heartbeat.go` (`calculateHealthScore`)
- mirrored in model math:
  - `connection-manager/pkg/models/agent.go`

### Where used
- stored in `agents.health_score`
- shown on Endpoints page
- used in operational triage/automation context

### Related field: Queue Depth
- `queue_depth` is not mathematically derived in server.
- It is agent-reported telemetry (`HeartbeatRequest.QueueDepth`) stored/displayed as-is.

---

## 7) Endpoint Risk Intelligence Aggregation Math

### Why used
Provides per-endpoint posture summary for SOC ranking.

### SQL aggregation
For canonical endpoint identity:
- `total_alerts = COUNT(*)`
- `peak_risk_score = MAX(risk_score)`
- `avg_risk_score = ROUND(AVG(risk_score), 1)`
- `critical_count = COUNT(*) WHERE risk_score >= 90`
- `high_count = COUNT(*) WHERE risk_score >= 70 AND risk_score < 90`
- `open_count = COUNT(*) WHERE status = 'open'`
- `last_alert_at = MAX(timestamp)`

Ordered by:
- `peak_risk_score DESC`, then `critical_count DESC`

Canonicalization:
- map historical rows via:
  - `agents.id` direct match,
  - or `agents.metadata.previous_agent_id` bridge,
  - fallback to raw alert `agent_id`.

### Where calculated
- `connection-manager/internal/repository/alert_repo.go`
  - `GetEndpointRiskSummary()`

### Where used
- API: `/api/v1/alerts/endpoint-risk`
- UI: `dashboard/src/pages/EndpointRisk.tsx`

---

## 8) Alert Statistics Math (Dashboard Statistics page)

### Why used
High-level SOC KPI view.

### Formulas
- `total_alerts = COUNT(*)`
- `last_24h = COUNT(*) WHERE timestamp > now - 24h`
- `last_7d = COUNT(*) WHERE timestamp > now - 7d`
- by severity/status/rule = grouped counts
- **current implementation label note**:
  - `avg_confidence = AVG(risk_score) / 100.0`
  - this is normalized average risk, not raw detection confidence.

### Where calculated
- `sigma_engine_go/internal/infrastructure/database/alert_repo.go` (`GetStats`)
- exposed in:
  - `sigma_engine_go/internal/handlers/stats.go` (`AlertStats`)

### Where used
- `dashboard/src/pages/Stats.tsx`

---

## 9) Timeline Statistics Math

### Why used
Trend visualization by period.

### Formula
Grouped by bucket (`hour` or `day`):
- counts by severity per bucket.

### Where calculated
- `sigma_engine_go/internal/infrastructure/database/alert_repo.go` (`GetTimeline`)

### Where used
- API `/api/v1/sigma/stats/timeline`
- UI `Stats.tsx` trend chart

---

## 10) Performance and Reliability Metrics Math (Sigma pipeline)

### Why used
Operational visibility into ingestion/detection pipeline quality.

### Formulas
- `events_per_second = (processed_now - processed_prev) / elapsed_seconds`
- `alerts_per_second = (alerts_published / events_processed) * events_per_second`
- `avg_event_latency_ms`: EWMA-style moving average:
  - `avg = old*0.9 + new*0.1`
- `avg_rule_matching_ms`: EWMA-style moving average
- `avg_database_query_ms`: from alert writer average write latency
- `error_rate = processing_errors / events_processed` (0 if denominator = 0)
- `alert_fallback_used`: monotonic counter
- `alerts_dropped`: monotonic counter

### Where calculated
- `sigma_engine_go/internal/infrastructure/kafka/event_loop.go`
  - `statsReporter()`, metrics getters
- exposed by:
  - `sigma_engine_go/internal/handlers/stats.go` (`PerformanceStats`)

### Where used
- UI `dashboard/src/pages/Stats.tsx`
- UI `ReliabilityHealth.tsx` (connection-manager fallback telemetry)

---

## 11) Reliability Health (Fallback store counters)

### Why used
Measure ingestion durability and loss risk when primary pipeline is degraded.

### Counters
- fallback channel length/capacity
- channel full count
- DB write failure count
- sync write failed drop count

### Important interpretation
- Counters are cumulative since process start.
- Rates need delta over time externally.

### Where used
- API `/api/v1/reliability` in connection-manager
- UI `dashboard/src/pages/settings/ReliabilityHealth.tsx`

---

## 12) Context Policy Math (Hybrid manual + automatic context)

### Why used
Allow SOC-admin control over risk sensitivity by user/device/network scope.

### Effective math
- Merge applicable policies by scope (`global`, `agent`, `user`) using multiplicative factors.
- Bound each factor and final multiplier to avoid runaway scaling.
- Apply trusted-network adjustment (`0.9` inside trusted CIDR, `1.1` outside).
- Apply uncertainty penalty from ingestion quality using bounded `quality_factor`.

### Final context-aware production formula
- `raw_score = base + lineage + privilege + burst + ueba_bonus + interaction - fp_discount - ueba_discount`
- `context_multiplier = user_role_weight * device_criticality_weight * network_anomaly_factor` (clamped by scorer/provider bounds)
- `quality_factor = f(context_quality_score)` where:
  - `>= 80 => 1.00`
  - `60..79 => 0.97`
  - `40..59 => 0.93`
  - `1..39 => 0.90`
  - `0/absent => 0.85`
- `context_adjusted_score = round(raw_score * context_multiplier * quality_factor)`
- `final_score = clamp(context_adjusted_score, 0, 100)`

Why:
- Never invent missing context signals.
- Penalize confidence when required context is missing.
- Keep deterministic and bounded behavior in production.

### Deterministic application order (production)
- Policies are applied in deterministic precedence:
  1. `global`
  2. `agent`
  3. `user`
- Within each scope, policies are applied by ascending policy ID.
- Rationale:
  - stable reproducible scoring,
  - broad baseline first, then endpoint specialization, then per-user specialization.

### Fields
- `user_role_weight`
- `device_criticality_weight`
- `network_anomaly_factor`
- `trusted_networks` (CIDR list)

### Event fields required for policy applicability
- Required for `agent` scope matching:
  - `agent_id`
- Required for `user` scope matching:
  - `user_name`
- Required for trusted-network adjustment:
  - `ip_address` or `source.ip_address`
- If these fields are absent, scoring still runs, but policy matching is partial (reduced context quality).
- `context_quality_score` (0-100) and `missing_context_fields[]` are consumed by scorer to apply `quality_factor` and emit warnings for auditability.

### Where calculated
- `sigma_engine_go/internal/application/scoring/context_policy_provider.go`
- consumed in `risk_scorer.go`

### Where used
- visible in `score_breakdown` and `context_snapshot`
- controlled from dashboard Settings -> Context Policies.

---

## 13) Legacy Math (Previous vs Current)

### 13.1 Alert suppression threshold
- Previous behavior: effectively strict `> 0.7`
- Current production: inclusive `>= 0.70`
- Why changed: deterministic, explicit boundary behavior.

### 13.2 Multi-rule confidence boost
- Previous: linear bonus with low cap (`max + min((n-1)*0.05, 0.2)`)
- Current: logarithmic scaling (`max + min(0.15*ln(n), 0.3)`)
- Why changed: better diminishing-return behavior for high match counts.

### 13.3 Agent health formula consistency
- Previous issue: inconsistent formulas in heartbeat path vs model path.
- Current: unified 4-factor formula in both places.

### 13.4 UEBA maturity progression
- Previous issue: confidence could mature too quickly with event bursts.
- Current: `observation_days` increments by UTC day transitions only.

### 13.5 Avg Risk label semantics
- UI card now labeled "Avg Risk (normalized)" to reflect actual backend metric source (`AVG(risk_score)/100`).

---

## 14) Field-Level Contract (What must exist)

For scored alerts, these fields are mandatory in API payload:
- `risk_score`
- `false_positive_risk`
- `context_snapshot`
- `score_breakdown`

For context-aware v2 specifically:
- `score_breakdown.context_multiplier`
- `score_breakdown.user_role_weight`
- `score_breakdown.device_criticality_weight`
- `score_breakdown.network_anomaly_factor`
- `score_breakdown.context_quality_score`
- `score_breakdown.quality_factor`
- `context_snapshot.missing_context_fields`

---

## 15) Authoritative Usage Surfaces

- Alert list/details source of truth:
  - DB/API (`/api/v1/sigma/alerts`)
- Endpoint risk source of truth:
  - connection-manager aggregation API (`/api/v1/alerts/endpoint-risk`)
- Realtime websocket:
  - refresh signal only; not authoritative state storage.
- Reliability source of truth:
  - connection-manager reliability API.
