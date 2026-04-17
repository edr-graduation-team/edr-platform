# Production Math and Reliability Validation

This report captures implementation outcomes for the Production Math and Reliability Hardening phase.

## Scope Completed

- Agent health computation persistence and status semantics alignment.
- Risk Intelligence logical deduplication for re-enrolled endpoints.
- Reliability Health routing and response-shape hardening.
- Stats refresh/coherence improvements between backend cadence and dashboard polling.
- Conservative scoring calibration updates (no aggressive recall-impacting threshold changes).
- Full formula contract documentation added.
- Hybrid context-aware scoring control plane (policy CRUD + scoring factor wiring + dashboard reflection).

## Code Validation Performed

- `go test ./internal/... ./pkg/handlers/...` in `connection-manager`: pass
- `go test ./internal/handlers ./internal/infrastructure/kafka ./internal/application/baselines ./internal/domain` in `sigma_engine_go`: pass
- Frontend `npm`/TypeScript validation could not be executed in this environment because `npm` is not installed in PATH.
- `go test ./internal/application/scoring` in `sigma_engine_go`: pass (includes new context factor tests).

## Acceptance Mapping

### 1) No logical duplicate devices in Risk Intelligence

- Backend canonicalization now maps historical `agent_id` to the current endpoint identity using:
  - direct `agents.id` match
  - `agents.metadata.previous_agent_id` bridge for re-enrollment
- UI includes a safety dedupe merge by hostname when available.

### 2) Agent health score reflects latest heartbeat computation

- Heartbeat-calculated health now flows to PostgreSQL `agents.health_score` on each metrics update.
- Status mapping normalized to dashboard-supported operational statuses.

### 3) Reliability Health valid live metrics and no false "not available" from routing

- Nginx now proxies `/api/v1/reliability` to connection-manager.
- Frontend API guard throws explicit error on invalid response shape (misroute detection).

### 4) Stats semantics and refresh coherence

- Backend default event-loop stats interval reduced to 10s for better freshness.
- Dashboard polling intervals aligned and timeline query-key fixed for custom ranges.
- Performance payload now includes metadata fields (`generated_at`, `stats_window`) to clarify semantics.

### 5) Scoring formulas documented and calibrated

- Suppression gate normalized to explicit `>= 0.70`.
- UEBA day-confidence maturation fixed to increment by observed day transitions, not raw event frequency.
- Formula contract documented in `docs/production_math_contract.md`.

## Residual Risks / Follow-ups

- Full frontend typecheck/build validation requires Node/npm on target host.
- End-to-end integration tests in `sigma_engine_go/test/integration` were not used as acceptance gate due pre-existing fixture instability; targeted packages for changed logic are green.
- Production smoke test is still recommended after deployment:
  - heartbeat update -> verify `agents.health_score` changes
  - re-enrollment scenario -> verify single logical row in endpoint risk view
  - fallback path outage simulation -> verify reliability cards and headline behavior
  - context policy changes -> verify downstream `context_multiplier` effects in new alerts
  - run full deterministic E2E matrix before READY sign-off
