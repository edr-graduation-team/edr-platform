# Context-Aware Readiness and Test Plan

This document answers:

1. What context-aware capabilities are already implemented.
2. What math is used, where, and how outputs are propagated.
3. What is still missing relative to a full context-aware EDR model.
4. Exactly how to test readiness in a deterministic way.

## Update (2026-04-15): Hybrid Context-Aware Completion

- Hybrid policy input model is now implemented:
  - global / agent / user scoped context policies in `connection-manager`
  - CRUD API exposed at `/api/v1/context-policies`
- Scoring v2 factors are now active in `sigma-engine`:
  - `user_role_weight`
  - `device_criticality_weight`
  - `network_anomaly_factor`
  - derived `context_multiplier`
- Output propagation is completed:
  - persisted to alert `context_snapshot` and `score_breakdown`
  - available in alerts API payloads
  - rendered in dashboard (Settings Context Policies, Alerts summary, Endpoint Risk tags, Stats context counters)
- Current gate: **NOT READY** until deterministic runtime execution matrix is completed in the deployed environment.

---

## 1) Current Implementation Status

## Implemented and wired end-to-end

- **Process/Application context**
  - Parent-child lineage, ancestor chain, command line, executable path.
  - Files:
    - `sigma_engine_go/internal/infrastructure/cache/lineage_cache.go`
    - `sigma_engine_go/internal/application/scoring/suspicion_matrix.go`
    - `sigma_engine_go/internal/application/scoring/context_snapshot.go`
- **Privilege context**
  - Integrity/elevation/user SID/signature-based privilege and trust effects.
  - File:
    - `sigma_engine_go/internal/application/scoring/risk_scorer.go`
- **Historical/temporal context**
  - Burst tracking (rule-category frequency) + UEBA process/hour baseline.
  - Files:
    - `sigma_engine_go/internal/application/scoring/burst_tracker.go`
    - `sigma_engine_go/internal/application/baselines/baseline_repository.go`
- **Adaptive risk scoring**
  - Final `risk_score`, `false_positive_risk`, `context_snapshot`, `score_breakdown`.
  - Files:
    - `sigma_engine_go/internal/application/scoring/risk_scorer.go`
    - `sigma_engine_go/internal/domain/alert.go`
- **Persistence + API + UI reflection**
  - Stored in `sigma_alerts` and rendered in Alerts + Endpoint Risk.
  - Files:
    - `sigma_engine_go/internal/infrastructure/database/alert_writer.go`
    - `dashboard/src/pages/Alerts.tsx`
    - `dashboard/src/pages/EndpointRisk.tsx`
    - `dashboard/src/api/client.ts`

## Partially implemented

- **User context**
  - Present at event level (`user_name`, `user_sid`), used in scoring context snapshot.
  - Not yet enriched with business role semantics (finance/developer/admin role model).
- **Device context**
  - Basic endpoint identity/status/health available.
  - No explicit device criticality weighting yet in risk formula.
- **Network/environment context**
  - Limited via process/network indicators in rules.
  - No geo-anomaly, subnet profiling, or VPN behavior model in scorer.

## Not implemented yet (relative to full model)

- Graph-based user/device/process/network anomaly modeling.
- ML embeddings / multi-dimensional anomaly model.
- Threat-intel weighted contextual joins inside scoring engine (beyond rule/TTP mapping already in Sigma content).

---

## 2) Math and Where It Runs

## Detection confidence

- `confidence = clamp(baseSeverityConfidence * fieldMatchFactor * contextScore, 0, 1)`
- File:
  - `sigma_engine_go/internal/application/detection/detection_engine.go`

## Aggregated confidence

- `combined_confidence = min(max_confidence + min(0.15 * ln(match_count), 0.3), 1.0)`
- File:
  - `sigma_engine_go/internal/domain/detection_result.go`

## Context-aware risk score

- `risk_score = clamp(base + lineage + privilege + burst + ueba_bonus + interaction - fp_discount - ueba_discount, 0, 100)`
- File:
  - `sigma_engine_go/internal/application/scoring/risk_scorer.go`

## FP risk / suppression

- Signature/path-mapped probability `false_positive_risk ∈ [0,1]`
- Suppression gate:
  - `false_positive_risk >= 0.70`
- File:
  - `sigma_engine_go/internal/domain/alert.go`

## UEBA baseline math

- EMA mean:
  - `new_avg = 0.90*old + 0.10*1.0`
- EWM variance:
  - `sigma_new = sqrt((1-a)*sigma_old^2 + a*(x-mu_new)^2), a=0.10`
- Confidence:
  - `min(1 - exp(-observation_days/7), 0.99)`
- Day-count semantics:
  - `observation_days` increments on day transitions (not per event burst).
- File:
  - `sigma_engine_go/internal/application/baselines/baseline_repository.go`

## Agent health score

- `health = delivery*0.40 + status*0.30 + dropPenalty*0.20 + resource*0.10`
- Files:
  - `connection-manager/pkg/handlers/heartbeat.go`
  - `connection-manager/pkg/models/agent.go`
  - `connection-manager/internal/repository/agent_repo.go`

---

## 3) Output Propagation Across System

1. Event enters `connection-manager` -> Kafka `events-raw`.
2. `sigma-engine` evaluates rules, computes context-aware outputs.
3. Alert persisted (`risk_score`, `false_positive_risk`, `context_snapshot`, `score_breakdown`).
4. APIs expose enriched alerts and endpoint risk aggregates.
5. Dashboard renders:
   - Alerts list/details with score breakdown.
   - Endpoint Risk ranking by `peak_risk_score`.
   - Reliability and stats pages for runtime health/operability.

---

## 4) Ready-to-Test Checklist

- Services healthy:
  - `connection-manager`, `sigma-engine`, `dashboard`, `kafka`, `postgres`, `redis`.
- Dashboard can query:
  - `/api/v1/sigma/alerts`
  - `/api/v1/alerts/endpoint-risk`
  - `/api/v1/reliability`
- Auth token valid in dashboard session.
- Use `docs/deterministic_e2e_matrix.md` as execution worksheet.

---

## 5) Deterministic Validation Commands

- Ingestion:
  - `docker compose logs --since 3m connection-manager`
- Raw events:
  - `docker exec -it edr_platform-kafka-1 kafka-console-consumer --bootstrap-server localhost:9092 --topic events-raw --property print.timestamp=true --timeout-ms 15000 --max-messages 20`
- Alerts topic:
  - `docker exec -it edr_platform-kafka-1 kafka-console-consumer --bootstrap-server localhost:9092 --topic alerts --property print.timestamp=true --timeout-ms 15000 --max-messages 20`
- Sigma scoring decision logs:
  - `docker compose logs --since 3m sigma-engine | Select-String "Risk scored alert|suppressed|Published|Failed|Stats"`
- API payload spot-check:
  - `curl -s http://localhost:30088/api/v1/sigma/alerts | jq ".alerts[0] | {id,rule_id,severity,risk_score,false_positive_risk,context_snapshot,score_breakdown}"`

---

## 6) Go/No-Go Criteria for Context-Aware Readiness

Mark **READY** only if all are true:

- 5/5 deterministic scenarios executed.
- 4/4 checkpoints passed per scenario.
- `risk_score` + `context_snapshot` + `score_breakdown` present for matched alerts.
- No unresolved P0 gaps in:
  - collector / mapping / category_inference / rule_coverage / dashboard_render.
- Realtime alert visibility works without manual refresh.

If any fail, mark **NOT READY** and fix by gap class before additional feature expansion.

