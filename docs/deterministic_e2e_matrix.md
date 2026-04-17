# Deterministic E2E Detection Matrix

Use this file as the single execution sheet for deterministic end-to-end validation.

## 0) Baseline Freeze (before tests)

- Commit SHA:
- Deployed image tags:
  - connection-manager:
  - sigma-engine:
  - dashboard:
- Sigma ruleset hash/version:
- Environment:
  - Kafka brokers:
  - DB endpoint:
  - Dashboard URL:
- Start time (UTC):

---

## 1) Core Scenario Matrix (5 scenarios)

Fill one row per scenario before running tests.

| Scenario ID | Detection Family | Trigger command/action | Expected `event_type` | Required payload keys | Expected Sigma rule(s) | Expected dashboard outcome |
|---|---|---|---|---|---|---|
| S1 | Process execution (recon) | `whoami /all` ثم `ipconfig /all` | `process` | `agent_id`, `event_type`, `timestamp`, `data.name`, `data.executable`, `data.command_line`, `data.pid`, `data.ppid`, `data.user_name` | `edr-test-recon-commands-001` (أو rule recon مكافئة) | ظهور Alert خلال <= 10s، severity `high`, و`risk_score` غالبا `85-100` |
| S2 | Network behavior | `netstat -ano` ثم `arp -a` | `process` (مع سلوك network discovery) | نفس مفاتيح S1 + `data.parent_name` | Rule discovery/network enumeration (حسب ruleset المحمّل) | Alert واحد على الأقل، مع `context_snapshot` يتضمن parent/process details |
| S3 | Persistence behavior | `schtasks /create /sc onlogon /tn EDRTestTask /tr "cmd.exe /c echo test" /f` | `process` | نفس مفاتيح S1 + command line الكامل | Rule persistence (scheduled task / autorun pattern) | Alert persistence واضح، `risk_score >= 70` |
| S4 | Privilege / elevation | تشغيل PowerShell كمسؤول ثم `whoami /groups` و`whoami /priv` | `process` | نفس مفاتيح S1 + `data.integrity_level`, `data.is_elevated`, `data.user_sid` | Rule privilege/elevation أو نفس rule recon مع bonus سياقي | Alert مع `score_breakdown.privilege_bonus > 0` |
| S5 | Defense evasion | `vssadmin list shadows` أو `wevtutil cl Security` (بيئة اختبار فقط) | `process` | نفس مفاتيح S1 + executable/cmdline | Rule defense-evasion / tampering | Alert عالي/حرج، `risk_score` مرتفع، وعدم فقدان event عبر pipeline |

### Recommended required payload keys (minimum)

- `agent_id`
- `event_type`
- `timestamp`
- `data.name`
- `data.executable`
- `data.command_line` (if process scenario)
- `data.pid`
- `data.ppid`
- `data.user_name`

### Scenario cleanup commands (after tests)

- S3 cleanup:
  - `schtasks /delete /tn EDRTestTask /f`
- If you used log clearing/tampering tests in S5, execute only in isolated lab and restore baseline image/snapshot afterward.

### Standard evidence commands (copy/paste)

- Connection-manager ingestion:
  - `docker compose logs --since 3m connection-manager`
- Kafka raw events:
  - `docker exec -it edr_platform-kafka-1 kafka-console-consumer --bootstrap-server localhost:9092 --topic events-raw --property print.timestamp=true --timeout-ms 15000 --max-messages 20`
- Kafka alerts:
  - `docker exec -it edr_platform-kafka-1 kafka-console-consumer --bootstrap-server localhost:9092 --topic alerts --property print.timestamp=true --timeout-ms 15000 --max-messages 20`
- Sigma decision logs:
  - `docker compose logs --since 3m sigma-engine | Select-String "Stats | Alerts:|Published|suppressed|Risk scored alert|Failed"`
- API payload verification:
  - `curl -s http://localhost:30088/api/v1/sigma/alerts | jq ".alerts[0] | {id,rule_id,severity,risk_score,false_positive_risk,context_snapshot,score_breakdown}"`

---

## 2) Execution Checkpoints (run per scenario)

For each scenario, capture all 4 checkpoints with evidence.

### Scenario: S__

#### Checkpoint 1: Ingestion in connection-manager

- Evidence command/log:
- Expected:
- Actual:
- Status: PASS / FAIL
- Notes:

#### Checkpoint 2: Payload in Kafka `events-raw`

- Evidence command/log:
- Expected:
- Actual:
- Status: PASS / FAIL
- Notes:

#### Checkpoint 3: Sigma match decision

- Expected decision: MATCH / DROP
- Expected reason:
- Actual decision:
- Actual reason:
- Evidence log/API:
- Status: PASS / FAIL

#### Checkpoint 4: Dashboard/API alert correctness

- API endpoint checked:
- Expected fields:
- Actual fields:
- Severity/risk correctness:
- Realtime render correctness:
- Status: PASS / FAIL
- Notes:

---

## 3) Coverage Gap Analysis

Classify every failure into exactly one primary bucket.

| Gap ID | Scenario | Failure summary | Primary class | Root cause hypothesis | Owner | Target file/path | Priority | ETA | Status |
|---|---|---|---|---|---|---|---|---|---|
| G1 |  |  | collector |  |  |  | P0/P1/P2 |  | open |
| G2 |  |  | mapping |  |  |  | P0/P1/P2 |  | open |
| G3 |  |  | category_inference |  |  |  | P0/P1/P2 |  | open |
| G4 |  |  | rule_coverage |  |  |  | P0/P1/P2 |  | open |
| G5 |  |  | dashboard_render |  |  |  | P0/P1/P2 |  | open |

---

## 4) Deterministic Evidence Log

Record raw commands and outputs (or links/artifacts) used as proof.

| Timestamp (UTC) | Scenario | Step | Command / Endpoint | Key output / reference |
|---|---|---|---|---|
|  |  |  |  |  |

---

## 5) Acceptance Gate

Mark final gate only after all scenarios are executed.

### Pass Criteria

- All 5 scenarios executed end-to-end.
- All 4 checkpoints completed for each scenario.
- No unresolved P0 gaps.
- Realtime dashboard updates verified without manual refresh for alert-producing scenarios.
- Alert payload fields semantically correct in API and UI.

### Final Decision

- Overall status: READY / NOT READY
- Blocking gaps:
- Rollback plan validated: YES / NO
- Sign-off by:
- Sign-off date (UTC):

---

## 6) Latest Execution Snapshot (Hybrid Context-Aware)

- Date: 2026-04-15
- Code status:
  - Hybrid context policy CRUD implemented (`/api/v1/context-policies`)
  - Scoring factors implemented (`user_role_weight`, `device_criticality_weight`, `network_anomaly_factor`, `context_multiplier`)
  - Dashboard surfaces implemented (Settings/Alerts/EndpointRisk/Stats)
- Local verification:
  - Go tests for changed packages passed.
  - Frontend static lint diagnostics clean for changed files.
- Environment limitation:
  - Full 5-scenario runtime E2E execution is pending on deployed stack.

### Interim Gate

- Overall status: **NOT READY**
- Blocking reason: deterministic runtime matrix checkpoints are not fully executed yet in the live environment.

