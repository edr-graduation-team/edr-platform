# Context-aware scoring pipeline overview
_Exported on 4/16/2026 at 19:50:10 UTC from Cursor (2.6.20)_

---

**User**

@c:\Users\Administrator\Desktop\edr-platform\ 




You are an expert security engineer and senior Go/TypeScript developer.  
I have an in‑house EDR platform repository on my local machine:

```text
C:\Users\Me\edr-platform
├─ agent-builder/
├─ connection-manager/
├─ dashboard/
│  └─ src/
├─ sigma_engine_go/
│  ├─ cmd/
│  ├─ internal/
│  │  ├─ application/
│  │  │  └─ scoring/
│  │  ├─ analytics/
│  │  ├─ automation/
│  │  ├─ domain/
│  │  ├─ handlers/
│  │  ├─ infrastructure/
│  │  ├─ integrations/
│  │  ├─ metrics/
│  │  ├─ ml/
│  │  ├─ optimization/
│  │  ├─ rules/
│  │  └─ user/
│  ├─ pkg/
│  └─ ...
├─ win_edrAgent/
└─ scripts/
```

Your job:  
1) Discover and explain the full **context‑aware scoring pipeline** end‑to‑end in this project (backend + agent + dashboard), with focus on the “hybrid context‑aware controls” we discussed earlier (user role, device criticality, network context, multipliers, etc.).  
2) Explain it to me as if I am **new to context‑aware risk scoring**, but already comfortable with Go, SQL, and TypeScript/React.

### What I specifically want:

1. **Code navigation plan**
   - Scan the repo and locate all components related to:
     - risk scoring / alert scoring,
     - context policies / context factors,
     - user/agent/network context,
     - storage of context snapshots,
     - UI display of risk/context in the dashboard.
   - Point to the exact folders/files you will focus on (paths relative to repo root).

2. **Back‑end context model (Go)**
   - Start from `sigma_engine_go/internal/application/scoring/` (especially:
     - `context_policy_provider.go`
     - `context_snapshot.go`
     - any other scoring‑related files).
   - For each key type/function:
     - Explain what it does, and how it fits into the risk scoring model.
     - Show the **mathematical formula** used (for example, show how `UserRoleWeight`, `DeviceCriticalityWeight`, and `NetworkAnomalyFactor` combine into a `ContextMultiplier` and then into the final `risk_score`).
   - Trace the flow:
     - Where is `ContextPolicyProvider.Resolve(...)` called?
     - How are `ContextFactors` and `ContextMultiplier` plugged into `ScoreBreakdown` and the final `risk_score`?
     - How are `ContextSnapshot` and `ScoreBreakdown` persisted (which DB tables / columns)?
   - I want an explicit, human‑readable equation like:
     - `raw_score = ...`
     - `context_multiplier = ...`
     - `context_adjusted_score = raw_score * context_multiplier`
     - `final_score = clamp(context_adjusted_score, 0, 100)`
     and then show exactly where this happens in code (file + function).

3. **Context policy storage & retrieval**
   - Inspect how `context_policies` are stored in Postgres:
     - which Go structs map to that table (e.g., `policyRecord`),
     - what columns exist (`scope_type`, `scope_value`, `user_role_weight`, `device_criticality_weight`, `network_anomaly_factor`, `trusted_networks`, etc.).
   - Explain the **precedence model**:
     - global → agent → user
     - how policies are combined by multiplying factors and then clamped.
   - Explain how trusted networks (CIDR ranges) are used to adjust `NetworkAnomalyFactor` depending on `sourceIP`.

4. **Agent side (win_edrAgent)**
   - Show how the Windows agent collects:
     - user info (SID, user name, elevation),
     - device/host info,
     - network context (source IP, interface),
     - process lineage and other telemetry used later in scoring.
   - Explain how this data is sent to the backend and how it becomes part of the context used in scoring.

5. **ContextSnapshot & forensic view**
   - In `ContextSnapshot` and `ScoreBreakdown`, explain:
     - How all fields (lineage, privileges, burst, rule severity, UEBA, plus context weights) are combined into the final score.
     - How `UserRoleWeight`, `DeviceCriticalityWeight`, `NetworkAnomalyFactor`, `ContextMultiplier`, `ContextQualityScore`, and `QualityFactor` are populated and used.
   - Give at least 2–3 **concrete numeric examples**:
     - Example 1: low‑risk user on trusted network with non‑critical device.
     - Example 2: high‑risk admin on untrusted network with critical server.
     - For each, start from base score and show step‑by‑step how the final score is computed using the actual formulas in this project.

6. **Dashboard side (React/TypeScript)**
   - In `dashboard/src/`:
     - Find where alerts / scores / context snapshots are displayed.
     - Explain how the UI presents:
       - risk_score / severity,
       - context factors (user role, device criticality, network),
       - any visualizations for context‑aware scoring (badges, colors, tooltips).
   - Show which API endpoints are called from the dashboard to fetch alerts/context, and how these endpoints relate back to the Go backend.

7. **Design intent & how to extend**
   - From the current implementation, infer the **design intent** of the context‑aware model:
     - Why it was built as a multiplicative `ContextMultiplier` instead of a fully additive formula.
     - Why there is a `context_policies` table with scopes (global/agent/user).
   - Then, explain in **simple terms**:
     - If I want to add a new context factor (for example: “GeoLocationRiskFactor” or “TimeOfDayFactor”), where exactly should I:
       - modify Go structs & formulas,
       - modify DB schemas / `context_policies`,
       - adjust the dashboard to show the new factor.
   - Give concrete code pointers and high‑level steps to safely extend the system.

### Output style:

- Write the explanation as a structured technical document, but easy to read:
  - Use section headings (e.g., “1. Context Factors and Multiplier”, “2. How Policies Are Applied”, …).
  - Include short code snippets with file paths and function names.
  - For every formula, write it in plain math notation and then show the corresponding Go code.
- Assume I will **read your explanation, then open the files myself**. Your goal is that after reading your doc, I can:
  - navigate the codebase confidently,
  - understand exactly how context‑aware scoring works end‑to‑end,
  - safely modify or extend it.

You have full read access to the repo on my local machine at `C:\Users\Me\edr-platform`.  
Start by listing the key files you will inspect, then go layer by layer from agent → backend → database → dashboard → final risk score.  

---

**Cursor**

I’ll map the scoring flow first by locating the key backend, agent, database, and dashboard files, then I’ll trace how context factors move from telemetry to persisted score breakdown and UI rendering. I’m starting with a focused repo scan for scoring/context symbols and related endpoints.

I found the main hotspots: `sigma_engine_go/internal/application/scoring/*`, risk persistence in `sigma_engine_go/internal/infrastructure/database/*`, policy CRUD in `connection-manager/*`, and dashboard pages including `Alerts`, `EndpointRisk`, and `settings/ContextPolicies`. Next I’m reading these files to extract the exact formulas and call chain.

I now have the scoring core and call sites. Next I’m pulling DB schema/migrations, policy CRUD, agent telemetry collectors, and dashboard API/pages so I can trace the full end-to-end path you requested.

## Key Files To Inspect (Navigation Plan)

Start here, in this order:

- **Scoring core (Go)**  
  - `sigma_engine_go/internal/application/scoring/risk_scorer.go`  
  - `sigma_engine_go/internal/application/scoring/context_policy_provider.go`  
  - `sigma_engine_go/internal/application/scoring/context_snapshot.go`  
  - `sigma_engine_go/internal/application/scoring/suspicion_matrix.go`  
  - `sigma_engine_go/internal/application/scoring/burst_tracker.go`

- **Where scoring is invoked + persisted**  
  - `sigma_engine_go/internal/infrastructure/kafka/event_loop.go`  
  - `sigma_engine_go/cmd/sigma-engine-kafka/main.go`  
  - `sigma_engine_go/internal/infrastructure/database/alert_writer.go`  
  - `sigma_engine_go/internal/infrastructure/database/alert_repo.go`  
  - `sigma_engine_go/internal/handlers/alerts.go`  
  - `sigma_engine_go/internal/infrastructure/database/migrations/014_add_risk_scoring.up.sql`

- **Context policy storage/API (Postgres + connection-manager)**  
  - `connection-manager/internal/database/migrations/017_create_context_policies.up.sql`  
  - `connection-manager/internal/repository/context_policy_repo.go`  
  - `connection-manager/pkg/models/context_policy.go`  
  - `connection-manager/pkg/api/handlers_other.go`  
  - `connection-manager/internal/repository/alert_repo.go` (endpoint risk aggregate)

- **Agent-side context telemetry (Windows)**  
  - `win_edrAgent/internal/event/types.go`  
  - `win_edrAgent/internal/collectors/etw.go`  
  - `win_edrAgent/internal/collectors/network.go`  
  - `win_edrAgent/internal/event/batcher.go`  
  - `win_edrAgent/internal/grpc/client.go`  
  - `win_edrAgent/internal/grpc/stream.go`

- **Dashboard rendering (React/TS)**  
  - `dashboard/src/api/client.ts`  
  - `dashboard/src/pages/Alerts.tsx`  
  - `dashboard/src/pages/EndpointRisk.tsx`  
  - `dashboard/src/pages/settings/ContextPolicies.tsx`  
  - `dashboard/src/pages/Stats.tsx`

---

## 1) End-to-End Pipeline (Agent → Backend → DB → UI)

1. Agent collectors produce enriched events (`process`, `network`, etc.) with context fields (`user_sid`, `user_name`, `is_elevated`, `integrity_level`, `source_ip`, lineage/process metadata).
2. Agent batcher stamps host source metadata and sends compressed batches over gRPC.
3. Sigma engine Kafka loop detects rule matches, generates alert, then calls `RiskScorer.Score(...)`.
4. `RiskScorer` computes:
   - additive raw score components (base, lineage, privilege, burst, UEBA, interaction, FP discounts)
   - multiplicative context factors from policies (`user/device/network`)
   - quality factor from context completeness
5. Final `risk_score` + `context_snapshot` + `score_breakdown` are persisted in `sigma_alerts`.
6. Dashboard `Alerts` page fetches `/api/v1/sigma/alerts`, shows risk/context detail modal.
7. Dashboard `EndpointRisk` page fetches `/api/v1/alerts/endpoint-risk` (connection-manager aggregate), ranks endpoints by peak risk.

---

## 2) Backend Context Model + Exact Formula

### Core types

- `ContextFactors` in `context_policy_provider.go`:  
  `UserRoleWeight`, `DeviceCriticalityWeight`, `NetworkAnomalyFactor`
- `ContextSnapshot` + `ScoreBreakdown` in `context_snapshot.go`: forensic and explainability payload.
- `DefaultRiskScorer.Score(...)` in `risk_scorer.go`: orchestrates everything.

### Actual formula in code

In `risk_scorer.go` (`Score`):

- `raw = base + lineage + privilege + burst + uebaBonus + interaction - fpDiscount - uebaDiscount`
- `contextMultiplier = clamp(UserRoleWeight * DeviceCriticalityWeight * NetworkAnomalyFactor, 0.25, 4.0)`
- `qualityFactor = computeContextQualityFactor(context_quality_score, missing_context_fields_count)` where result is in `[0.85..1.00]`
- `contextAdjusted = round(raw * contextMultiplier * qualityFactor)`
- `final = clamp(contextAdjusted, 0, 100)`

So, human-readable:

- `raw_score = base_score + lineage_bonus + privilege_bonus + burst_bonus + ueba_bonus + interaction_bonus - fp_discount - ueba_discount`
- `context_multiplier = clamp(user_role_weight * device_criticality_weight * network_anomaly_factor, 0.25, 4.0)`
- `context_adjusted_score = round(raw_score * context_multiplier * quality_factor)`
- `final_score = clamp(context_adjusted_score, 0, 100)`

### Where `ContextPolicyProvider.Resolve(...)` is called

In `risk_scorer.go`, inside `Score(...)`, after UEBA and interaction are computed:
- extracts `user_name`
- extracts `ip_address` (or `source.ip_address` fallback)
- calls `rs.contextProvider.Resolve(ctx, agentID, userName, sourceIP)`

Provider is wired in `cmd/sigma-engine-kafka/main.go`:
- `riskScorer.SetContextPolicyProvider(scoring.NewPostgresContextPolicyProvider(...))`

---

## 3) Policy Storage, Precedence, and Trusted Networks

### Table + struct mapping

`context_policies` columns come from migration `017_create_context_policies.up.sql`:

- `scope_type` (`global|agent|user`)
- `scope_value`
- `user_role_weight`
- `device_criticality_weight`
- `network_anomaly_factor`
- `trusted_networks` (`JSONB`, CIDR list)
- plus `enabled`, `notes`, timestamps

Mapped by:

- `policyRecord` in `sigma_engine_go/internal/application/scoring/context_policy_provider.go`
- `models.ContextPolicy` in `connection-manager/pkg/models/context_policy.go`

### Precedence and combination model

`PostgresContextPolicyProvider.Resolve(...)` applies policies in SQL order:

- global → agent → user (then `id ASC`)

For each matched policy:
- multiply each factor independently
- clamp each per-step to `[0.5, 2.0]`

Then multiplier is final product clamped `[0.25, 4.0]`.

### Trusted network behavior

After policy aggregation, if `sourceIP` exists:

- if `sourceIP ∈ trusted_networks CIDR set`: `network_factor *= 0.9` (downweight risk)
- else: `network_factor *= 1.1` (upweight risk)
- then clamp network factor to `[0.5, 2.0]`

---

## 4) Agent-Side Context Collection

### User/privilege/process context

`win_edrAgent/internal/collectors/etw.go` (`processStart`):
- `getPrivileges(pid)` extracts:
  - SID
  - user account
  - elevation
  - integrity level
- event data includes:
  - `user_sid`, `user_name`, `is_elevated`, `integrity_level`
  - `pid`, `ppid`, `name`, `executable`, `command_line`
  - parent executable/name

### Network context

`win_edrAgent/internal/collectors/network.go`:
- kernel TCP/UDP tables via IP Helper API
- emits `source_ip`, `source_port`, `destination_ip`, `destination_port`, protocol, PID/process

### Device/host context

`win_edrAgent/internal/event/batcher.go`:
- `fillEventSource(...)` adds:
  - hostname
  - first non-loopback IP
  - OS type/version placeholder
  - agent version

### Transport to backend

- batched/compressed payloads via gRPC stream (`internal/grpc/stream.go`, `internal/grpc/client.go`)
- sigma side eventually receives events, detects, scores, persists.

---

## 5) ContextSnapshot + ScoreBreakdown Forensics

Both are populated in `risk_scorer.go`:
- `breakdown := ScoreBreakdown{...}` stores every component plus context weights/multiplier/quality.
- `buildContextSnapshot(...)` embeds process lineage, privilege fields, rule metadata, burst, and breakdown.
- later assigns:
  - `snapshot.UserRoleWeight`
  - `snapshot.DeviceCriticalityWeight`
  - `snapshot.NetworkAnomalyFactor`
  - `snapshot.ContextMultiplier`
  - `snapshot.ContextQualityScore`
  - `snapshot.QualityFactor`

Persisted via:
- `alert_writer.go` maps domain alert fields
- `alert_repo.go` writes `risk_score`, `context_snapshot`, `score_breakdown` into `sigma_alerts`.

---

## 6) Numeric Examples (Using Current Formula)

### Example 1: Low-risk user, trusted network, non-critical device

Assume:
- additive raw components:  
  `base=45, lineage=10, privilege=0, burst=0, uebaBonus=0, interaction=0, fpDiscount=8, uebaDiscount=10`  
  `raw = 45+10+0+0+0+0-8-10 = 37`
- context factors from policies:
  - user `0.9`, device `0.9`, network base `1.0`
  - trusted CIDR hit applies `*0.9` → network `0.9`
  - multiplier `= 0.9*0.9*0.9 = 0.729`
- quality factor: `1.0`

Then:
- `context_adjusted = round(37 * 0.729 * 1.0) = round(26.97)=27`
- `final = clamp(27,0,100)=27`

### Example 2: High-risk admin, untrusted network, critical server

Assume:
- additive raw components:  
  `base=65, lineage=30, privilege=25, burst=20, uebaBonus=15, interaction=15, fpDiscount=0, uebaDiscount=0`  
  `raw = 170`
- context factors:
  - user `1.4`, device `1.5`, network base `1.2`
  - untrusted IP adjustment `*1.1` => network `1.32`
  - multiplier `= 1.4*1.5*1.32 = 2.772`
- quality factor `0.97`

Then:
- `context_adjusted = round(170 * 2.772 * 0.97) = round(457.1)=457`
- `final = clamp(457,0,100)=100`

### Example 3: Moderate raw score but partial context quality

Assume:
- `raw=70`
- multiplier `=1.2`
- quality score = 0 with missing fields > 0 => quality factor `0.85`

Then:
- `context_adjusted = round(70 * 1.2 * 0.85)=71`
- `final=71`

So uncertainty handling reduces overconfident escalation.

---

## 7) Dashboard: Where risk/context appears

### Alerts page

`dashboard/src/pages/Alerts.tsx`:
- table sorted by `risk_score` by default
- `RiskScoreBadge` color tiers
- detail modal “Context” tab renders:
  - process lineage tree
  - UEBA signal badge
  - score breakdown panel
  - context multiplier + component factors
  - quality factor + missing context warnings

API source in `dashboard/src/api/client.ts`:
- `alertsApi.list()` → `GET /api/v1/sigma/alerts`
- includes `risk_score`, `context_snapshot`, `score_breakdown`

### Endpoint risk page

`dashboard/src/pages/EndpointRisk.tsx`:
- `alertsApi.endpointRisk()` → `GET /api/v1/alerts/endpoint-risk`
- shows per-endpoint peak/avg risk, critical/high counts, open counts
- merges in agent metadata and displays role/device/network badges from agent metadata

### Context policy UI

`dashboard/src/pages/settings/ContextPolicies.tsx`:
- CRUD over `context_policies`
- fields map directly to backend factors and trusted CIDRs
- API via `contextPoliciesApi.*` in `client.ts` (`/api/v1/context-policies`)

---

## 8) Design Intent (Why multiplicative context + scoped policies)

- Multiplicative context (`raw * context_multiplier`) gives **proportional amplification/suppression**:
  - same context has stronger effect on truly dangerous raw events than weak ones
  - avoids brittle “+/- fixed points” that can overwhelm low-signal alerts
- Scoped policy table (`global/agent/user`) gives **operational control layering**:
  - global baseline
  - endpoint-specific hardening
  - user-specific sensitivity (e.g., admins/service accounts)
- Trusted CIDR adjustment captures real SOC intuition: “same behavior from trusted subnet vs unknown subnet” should score differently.

---

## 9) How To Add a New Factor (e.g., `GeoLocationRiskFactor` or `TimeOfDayFactor`)

### Go/backend changes

1. Extend factor model:
   - `ContextFactors` in `context_policy_provider.go`
   - add field + include in `Multiplier()`
2. Add DB policy column:
   - migration for `context_policies` new numeric column
   - update `policyRecord` scan + repo CRUD structs (`models.ContextPolicy`, requests/validation)
3. Add resolve logic:
   - in `Resolve(...)`, combine scoped policy values
   - optional dynamic adjustment (like trusted networks logic)
4. Add explainability output:
   - set field in `ScoreBreakdown` and `ContextSnapshot` in `risk_scorer.go`
5. Update persistence (already JSONB-friendly):
   - no major table change needed for breakdown/snapshot JSONB unless you want indexed querying on new key.

### Dashboard changes

1. API typings:
   - add optional field in `ScoreBreakdown`/`ContextSnapshot` interfaces in `dashboard/src/api/client.ts`
2. Alerts context UI:
   - show factor in formula text and context chips in `Alerts.tsx`
3. Policies settings page:
   - add input field in `ContextPolicies.tsx`
4. Endpoint risk (optional):
   - expose aggregate/metadata badge if desired.

### Safe rollout steps

- add migration + backward-compatible defaults (`1.0`)
- keep factor optional in TS/Go JSON structs
- add unit tests around:
  - policy precedence
  - clamp boundaries
  - final score determinism
- monitor distributions (elevated/neutral/discounted buckets on `Stats.tsx`) after deployment.

---

If you want, next I can produce a **single-page “code-reading checklist”** with exact function call order (including `DetectAggregated` → `GenerateAggregatedAlert` → `Score` → DB write) so you can step through in debugger quickly.

---

**User**

أنا أعمل حاليًا على فهم جزء **context‑aware** في منصة الـ EDR الخاصة بي، وأريد منك شرحًا تفصيليًا لكن مركزًا فقط على هذا الجزء، وليس على المشروع كامل.

أريد منك ما يلي بالتحديد:

## 1) شرح كل معادلة بالتفصيل

- خذ جميع المعادلات المتعلقة بـ context‑aware scoring، مثل:
  - حساب `raw_score`
  - حساب `ContextFactors` (UserRoleWeight, DeviceCriticalityWeight, NetworkAnomalyFactor)
  - حساب `context_multiplier`
  - حساب `quality_factor`
  - حساب `context_adjusted_score`
  - حساب `final_score`
- لكل معادلة:
  1. اكتب المعادلة بصيغة رياضية واضحة.
  2. اشرح **كل حقل** فيها:
     - من أين تأتي قيمته (من أي ملف/دالة/جدول DB؟).
     - كيف يتم حسابه بالضبط في الكود.
     - في أي مرحلة من الـ pipeline يتم حسابه.
  3. اشرح دور هذه المعادلة في عملية **context‑aware**:
     - كيف تؤثر على نتيجة الـ risk؟
     - كيف تنعكس لاحقًا في الـ dashboard؟

أريد أن يكون الشرح بأسلوب مبسط، شبيه بالشرح السابق الذي استخدمنا فيه أمثلة رقمية واشتقينا القيم خطوة بخطوة.

## 2) ربط المعادلات بخط الأنابيب (Pipeline) والـ Dashboard

- وضّح لي بالتسلسل:
  1. في أي مرحلة يتم حساب كل جزء من الـ score:
     - من لحظة وصول الـ event من الـ agent،
     - مرورًا بتطبيق الـ rules،
     - ثم استدعاء `RiskScorer.Score(...)`,
     - ثم حفظ `risk_score` و `context_snapshot` و `score_breakdown` في DB.
  2. وضّح أين يتم استخدام هذه النتائج في الـ Dashboard:
     - ما هي المعادلة النهائية التي تؤدي إلى الرقم الذي أراه في واجهة الـ dashboard؟  
       (مثلاً: `final_score` في أي حقل/كولمن).
     - في أي صفحة بالضبط يظهر هذا الرقم؟ (مثال: صفحة `Alerts`, صفحة `EndpointRisk`, إلخ).
     - كيف تُعرض عوامل السياق (user/device/network) والـ context_multiplier والـ quality_factor في الواجهة؟

## 3) التركيز على context‑aware فقط

- لا أحتاج شرحًا عن كامل الـ EDR أو كل الموديلات.
- ركّز فقط على:
  - `ContextPolicyProvider`
  - `ContextFactors`
  - `ContextSnapshot`
  - `ScoreBreakdown`
  - المعادلات التي تربطها ببعضها.
- إشرح العلاقات بينهم بحيث أستطيع بعد ذلك أن أعدّل أو أضيف عوامل سياقية جديدة وأنا فاهم (مثلاً إضافة `GeoLocationRiskFactor` أو غيره).

## 4) سيناريو عملي كامل (من الـ Dashboard إلى السيرفر)

أريد سيناريو متكامل يشرح كأنني قمت بهذا التسلسل:

1. من الـ Dashboard:
   - دخلت إلى صفحة إعدادات سياسات السياق (Context Policies).
   - أنشأت Policy جديدة مثلاً:
     - scope_type = "user"
     - scope_value = "me"
     - user_role_weight = 0.8
     - device_criticality_weight = 1.2
     - network_anomaly_factor = 1.1
     - trusted_networks فيها CIDR لشبكة معينة.
2. اشرح ماذا يحدث بعد الحفظ في الـ DB:
   - أين تُخزن هذه السياسة (أي جدول/أعمدة).
   - كيف يقرأها `PostgresContextPolicyProvider` ويجمعها مع سياسات أخرى (global/agent/user).
3. ثم مثلاً:
   - يحدث event من جهاز يحمل `agent_id` معين و `user_name = "me"` و `source_ip` إما ضمن trusted network أو خارجها.
   - وضّح خطوة بخطوة:
     - كيف يتم حساب `UserRoleWeight`, `DeviceCriticalityWeight`, `NetworkAnomalyFactor` لهذه الحالة بالذات.
     - ما هو `ContextMultiplier` الناتج.
     - كيف يُضرب هذا الـ multiplier في `raw_score` مع `quality_factor` ليعطي `final_score`.
4. أخيرًا:
   - وضّح كيف يظهر هذا الـ `final_score` وعوامل السياق في الـ Dashboard:
     - في جدول الـ alerts،
     - وفي modal التفاصيل،
     - وفي صفحة EndpointRisk (إن كانت متأثرة).

أريد أن يكون الشرح متسلسل، كامل، وواضح،  (مع أمثلة رقمية واقعية قدر الإمكان) حتى أستطيع بعد ذلك تعديل أو تطوير جزء الـ context‑aware في المشروع وأنا فاهم كل خطوة.

---

---

**Cursor**

سأركّز فقط على جزء `context-aware` كما طلبت، وأبني لك شرحًا مرتبطًا مباشرة بالمعادلات، مصادر القيم في الكود/قاعدة البيانات، ثم سيناريو عملي كامل من إعداد الـ policy في الـ Dashboard حتى ظهور النتيجة في واجهة التنبيهات. بعدها سأقدّمه بشكل مرجعي واضح يمكنك استخدامه أثناء التعديل.

---

**User**

Implement the plan as specified, it is attached for your reference. Do NOT edit the plan file itself.

To-do's from the plan have already been created. Do not create them again. Mark them as in_progress as you work, starting with the first one. Don't stop until you have completed all the to-dos.

---

**Cursor**

سأبدأ بتنفيذ البنود واحدًا واحدًا كما في الخطة، مع تحديث حالة المهام أثناء العمل، ثم أقدّم لك شرحًا عميقًا ومترابطًا فقط لجزء `context-aware` من المعادلات حتى العرض في الـ Dashboard.

أكملت الآن جزء المعادلات ومصادر القيم، وأنتقل لربطها بخط الأنابيب الكامل ثم ربطها بعناصر الواجهة (Alerts/EndpointRisk/ContextPolicies) قبل السيناريو العملي والتوسعة.

## Context-Aware Scoring Deep Dive

ممتاز. سأركّز فقط على `context-aware` كما طلبت: المعادلات، مصادر القيم، خط الأنابيب، الربط بالـ Dashboard، ثم سيناريو عملي كامل.

---

## 1) المعادلات (Equation by Equation)

## A) معادلة `raw_score`

**الصيغة الرياضية:**

\[
raw\_score = baseScore + lineageBonus + privilegeBonus + burstBonus + uebaBonus + interactionBonus - fpDiscount - uebaDiscount
\]

**أين في الكود؟**  
داخل `RiskScorer.Score(...)` في `sigma_engine_go/internal/application/scoring/risk_scorer.go`:

- `raw := baseScore + lineageBonus + privilegeBonus + burstBonus + uebaBonus + interactionBonus - fpDiscount - uebaDiscount`

### شرح كل حقل:

- `baseScore`  
  - من `computeBaseScore(...)` في نفس الملف.
  - يعتمد على Severity + عدد الـ matches.
  - يُحسب مبكرًا (Step 1) بعد استخراج أعلى rule severity.

- `lineageBonus`  
  - من `rs.matrix.ComputeBonus(lineageChain)`  
  - `lineageChain` يأتي من cache (`lineageCache.GetLineageChain(...)`).
  - يُحسب بعد base (Step 2).

- `privilegeBonus`  
  - من `computePrivilegeBonus(event.RawData)`.
  - يعتمد على `user_sid`, `integrity_level`, `is_elevated`, `signature_status`.

- `burstBonus`  
  - من `computeBurstBonus(burstCount)` بعد `burstTracker.IncrAndGet(...)`.
  - يزيد مع التكرار الزمني لنفس category.

- `uebaBonus` / `uebaDiscount`  
  - من `computeUEBA(...)`.
  - Anomaly يعطي Bonus، Normal يعطي Discount.

- `interactionBonus`  
  - من `computeInteractionBonus(...)`.
  - يلتقط التآزر بين عدة إشارات عالية.

- `fpDiscount`  
  - من `computeFPDiscount(signature_status, executable)`.

**دور المعادلة context-aware؟**  
هذه ليست المرحلة السياقية النهائية بعد؛ هي **النتيجة الخام قبل ضرب عوامل السياق**.

---

## B) عوامل السياق `ContextFactors`

**النوع:** `ContextFactors` في `sigma_engine_go/internal/application/scoring/context_policy_provider.go`:

- `UserRoleWeight`
- `DeviceCriticalityWeight`
- `NetworkAnomalyFactor`

### كيف تتحدد القيم؟

من `ContextPolicyProvider.Resolve(ctx, agentID, userName, sourceIP)`.

في implementation الفعلي `PostgresContextPolicyProvider.Resolve(...)`:

1. يقرأ `context_policies` من Postgres (`getPolicies`).
2. يطبّق policies المطابقة بترتيب:
   - global → agent → user
3. لكل policy match:
   - `UserRoleWeight *= policy.user_role_weight`
   - `DeviceCriticalityWeight *= policy.device_criticality_weight`
   - `NetworkAnomalyFactor *= policy.network_anomaly_factor`
   - كل عامل يُclamp إلى `[0.5, 2.0]`
4. ثم trusted network logic:
   - إذا `sourceIP` داخل CIDR trusted: `NetworkAnomalyFactor *= 0.9`
   - خارجها: `NetworkAnomalyFactor *= 1.1`
   - ثم clamp مرة أخرى `[0.5, 2.0]`

**مصدر DB:**
- جدول `context_policies` في:
  - `connection-manager/internal/database/migrations/017_create_context_policies.up.sql`
- الأعمدة:
  - `scope_type`, `scope_value`
  - `user_role_weight`
  - `device_criticality_weight`
  - `network_anomaly_factor`
  - `trusted_networks`

---

## C) معادلة `context_multiplier`

**الصيغة الرياضية:**

\[
context\_multiplier = clamp(UserRoleWeight \times DeviceCriticalityWeight \times NetworkAnomalyFactor,\ 0.25,\ 4.0)
\]

**أين؟**
- method `ContextFactors.Multiplier()` في `context_policy_provider.go`.

**دوره:**
- يضخّم/يخفض `raw_score` حسب السياق.
- Multiplicative model يجعل تأثير السياق نسبيًا مع خطورة الحدث الخام.

---

## D) معادلة `quality_factor`

**الصيغة:**
\[
quality\_factor = f(context\_quality\_score,\ missing\_context\_fields)
\]
بمدى `[0.85 .. 1.00]`.

**أين؟**
- `computeContextQualityFactor(...)` في `risk_scorer.go`.

**المنطق الحالي:**
- score >= 80 => 1.00
- >= 60 => 0.97
- >= 40 => 0.93
- > 0 => 0.90
- score = 0:
  - missingCount = 0 => 1.00
  - otherwise => 0.85

**مصدر المدخلات:**
- `context_quality_score` من event raw data
- `missing_context_fields` من event raw data

**الدور:**
- يعالج عدم اكتمال السياق (uncertainty penalty).

---

## E) معادلة `context_adjusted_score`

**الصيغة:**

\[
context\_adjusted\_score = round(raw\_score \times context\_multiplier \times quality\_factor)
\]

**أين؟**
- `contextAdjusted := int(math.Round(float64(raw) * contextFactors.Multiplier() * qualityFactor))` في `risk_scorer.go`.

---

## F) معادلة `final_score`

**الصيغة:**

\[
final\_score = clamp(context\_adjusted\_score,\ 0,\ 100)
\]

**أين؟**
- `finalScore := clamp(contextAdjusted, 0, 100)` في `risk_scorer.go`.

**هذا هو الرقم الذي يظهر في الواجهة.**

---

## 2) ربط المعادلات بالـ Pipeline والـ Dashboard

```mermaid
flowchart LR
agentEvent["Agent Event (process/network...)"] --> sigmaDetect["Sigma DetectAggregated"]
sigmaDetect --> alertGen["GenerateAggregatedAlert"]
alertGen --> riskScore["RiskScorer.Score(...)"]
riskScore --> breakdown["Build ScoreBreakdown + ContextSnapshot"]
breakdown --> persist["Persist sigma_alerts: risk_score/context_snapshot/score_breakdown"]
persist --> apiAlerts["GET /api/v1/sigma/alerts"]
persist --> apiEndpointRisk["GET /api/v1/alerts/endpoint-risk"]
apiAlerts --> alertsUi["Dashboard Alerts + Context Modal"]
apiEndpointRisk --> endpointRiskUi["Dashboard EndpointRisk"]
```

### التسلسل الفعلي:

1. event يدخل للـ Sigma event loop (`event_loop.go`).
2. rule matching (`DetectAggregated`).
3. alert generation.
4. مباشرة بعده:
   - `riskScorer.Score(...)` يُستدعى في `event_loop.go`.
5. scorer يرجع:
   - `RiskScore`
   - `Snapshot` (contains `ScoreBreakdown`)
6. map إلى alert fields:
   - `alert.RiskScore`
   - `alert.ContextSnapshot`
   - `alert.ScoreBreakdown`
7. DB persistence في `sigma_alerts` عبر `alert_repo.go`:
   - `risk_score`
   - `context_snapshot`
   - `score_breakdown`

### أين يظهر `final_score` في Dashboard؟

- في `dashboard/src/api/client.ts` ضمن `Alert.risk_score`.
- يُجلب من endpoint:
  - `/api/v1/sigma/alerts`
- يعرض في:
  - صفحة `Alerts` جدول التنبيهات (`RiskScoreBadge`)
  - Modal التفاصيل في تبويب Context
  - أيضًا يدخل ضمن endpoint summaries في `/api/v1/alerts/endpoint-risk` (peak/avg risk).

### عرض عوامل السياق في الواجهة:

في `Alerts.tsx`:
- يظهر `context_multiplier`
- يظهر decomposition:
  - user weight
  - device weight
  - network factor
- يظهر `quality_factor` + `context_quality_score`
- يظهر `score_breakdown` كامل (formula visual panel)

---

## 3) العلاقات بين `ContextPolicyProvider` و `ContextFactors` و `ContextSnapshot` و `ScoreBreakdown`

- `ContextPolicyProvider`: مصدر policy-driven factors (من DB + CIDR logic).
- `ContextFactors`: القيم الثلاث + method multiplier.
- `ScoreBreakdown`: يسجل كل مكونات الحساب بما فيها context weights/multiplier/quality.
- `ContextSnapshot`: forensic envelope يحتوي breakdown + lineage/privilege/burst + warnings.

**العلاقة الجوهرية:**
- Provider يحسب factors → factors تدخل في multiplier → multiplier + quality تؤثر على final score → كل ذلك يُوثَّق داخل breakdown/snapshot → UI يقرأه ويعرضه.

---

## 4) سيناريو عملي كامل من Dashboard إلى السيرفر

## الخطوة 1: إنشاء policy من Dashboard

في `ContextPolicies` page:
- POST إلى `/api/v1/context-policies` مع payload مثل:
  - `scope_type = "user"`
  - `scope_value = "me"`
  - `user_role_weight = 0.8`
  - `device_criticality_weight = 1.2`
  - `network_anomaly_factor = 1.1`
  - `trusted_networks = ["10.10.0.0/16"]`

## الخطوة 2: التخزين

- handler في `connection-manager/pkg/api/handlers_other.go`
- repo في `connection-manager/internal/repository/context_policy_repo.go`
- table: `context_policies`

## الخطوة 3: حدث جديد يصل

نفترض event مع:
- `agent_id = A1`
- `user_name = "me"`
- `source_ip` مرة trusted ومرة untrusted

### 3.1 تجميع factors (مثال)

نفترض السياسات المطابقة:
- global: (1.0, 1.0, 1.0)
- agent A1: (1.0, 1.3, 1.0)
- user me: (0.8, 1.2, 1.1)

النتيجة قبل CIDR:
- user = `1.0 * 1.0 * 0.8 = 0.8`
- device = `1.0 * 1.3 * 1.2 = 1.56`
- network = `1.0 * 1.0 * 1.1 = 1.1`

#### حالة Trusted IP
- network = `1.1 * 0.9 = 0.99`
- context multiplier = `0.8 * 1.56 * 0.99 = 1.23552`

#### حالة Untrusted IP
- network = `1.1 * 1.1 = 1.21`
- context multiplier = `0.8 * 1.56 * 1.21 = 1.51008`

### 3.2 حساب score

نفترض:
- `raw_score = 62`
- `quality_factor = 0.97`

#### Trusted
- context_adjusted = `round(62 * 1.23552 * 0.97) = round(74.3)=74`
- final = 74

#### Untrusted
- context_adjusted = `round(62 * 1.51008 * 0.97) = round(90.8)=91`
- final = 91

**نفس الحدث الخام، سياق الشبكة غيّر القرار من High إلى Critical.**

## الخطوة 4: كيف يظهر في Dashboard

- `Alerts` table:
  - عمود Risk يعرض 74 أو 91.
- Alert details modal:
  - يعرض factors (user/device/network)
  - multiplier
  - quality factor
  - breakdown formula
- `EndpointRisk`:
  - يأخذ `peak_risk_score` و `avg_risk_score` من تجميع alerts،
  - فتزيد رتبة endpoint عند ارتفاع final score.

---

## 5) كيف تضيف عامل جديد (مثل `GeoLocationRiskFactor`)

## Backend (Go + DB)

1. أضف field في `ContextFactors` (`context_policy_provider.go`)
2. أدخله في `Multiplier()`
3. أضف column في `context_policies` migration
4. حدث:
   - `policyRecord` scan
   - connection-manager model + request validation + CRUD repo
5. في `Resolve(...)` احسبه (policy + dynamic adjustment)
6. في `risk_scorer.go`:
   - أضفه إلى `ScoreBreakdown`
   - أضفه إلى `ContextSnapshot`

## Dashboard

1. حدث TS types في `dashboard/src/api/client.ts`
2. اعرض العامل في:
   - `Alerts` context summary
   - score panel (إن أردت)
3. أضف input في `ContextPolicies.tsx`

## Safety checklist

- migration default = `1.0` (backward-compatible)
- clamping boundaries واضحة
- unit tests:
  - precedence
  - multiplier math
  - trusted/untrusted branching
  - final clamp behavior

---

إذا أردت، الخطوة التالية أقدر أجهز لك **cheat sheet مختصر جدًا** (صفحة واحدة) بعنوان:  
`How to debug context-aware score for one alert`  
مع ترتيب نقاط breakpoints الدقيقة في `Score(...)` وقائمة مفاتيح JSON التي يجب تتبعها.

---

**User**

اعمله في ملف md واعمل لي الكلام العربي داخل <div dir="rtl"> والانجليزي في ltr

---

**Cursor**

سأحوّل الشرح بالكامل إلى ملف `md` داخل المشروع، مع تنسيق الاتجاه كما طلبت: العربي داخل `<div dir="rtl">` والإنجليزي داخل `<div dir="ltr">`. سأضعه بصيغة منظمة قابلة للقراءة المباشرة في GitHub/Cursor.

تم التنفيذ كما طلبت.

- أنشأت ملف Markdown جديد:
  - `context-aware-scoring-deep-dive.md`
- نسّقت المحتوى بحيث:
  - العربي داخل `<div dir="rtl">`
  - الإنجليزي داخل `<div dir="ltr">`

إذا تريد، أقدر في خطوة ثانية أعمل **نسخة مختصرة** من نفس الملف (Quick Reference) بنفس تنسيق RTL/LTR.

---

**User**

الآن بناءً على الشرح السابق عندي مجموعة أسئلة توضيحية على الجزء الخاص بـ context‑aware scoring، أرجو الإجابة عليها بنفس أسلوبك التعليمي:

***

## 1) منطق تصنيف الـ Final Score والـ Alerts

في المثال الذي وضحته أنت:

- في حالة الشبكة الموثوقة (trusted) كان:
  - `final_score = 74`
- في حالة الشبكة غير الموثوقة (untrusted) كان:
  - `final_score = 91`

سؤالي:

1. هل `final_score` هنا يُفسَّر كنسبة مئوية (مثلاً 74%) أم مجرد رقم بين 0 و 100 بدون علامة %؟
2. هل هناك منطق في الكود أو في الـ backend يقوم بالمقارنة مع عتبات (thresholds) ثابتة مثلاً:
   - إذا كانت `final_score < 60` يتم تجاهل الـ alert أو لا يُعرض في الـ dashboard؟
   - إذا كانت:
     - من 0 إلى 60 = Low أو يتم إخفاؤها
     - من 61 إلى 70 = Medium
     - من 71 إلى 89 = High
     - من 90 إلى 100 = Critical
3. هل هذا هو المنطق المطبق فعليًا الآن في مشروع EDR  لتحديد:
   - مستوى الخطورة (Low/Medium/High/Critical)
   - وما إذا كان الـ alert يظهر من الأساس في الـ dashboard أو يتم فلترته؟

وبشكل أوضح:  
هل المنطق الحالي في الكود/الـ backend هو ما يتحكم في عرض الـ alerts في الـ dashboard على شكل شارات (Badges) وألوان severity مبنيًا على final_score بهذه العتبات، أم هناك منطق آخر في الـ UI فقط (front‑end)؟

***

## 2) ما هو الـ Breakdown؟ وما الذي يعرضه بالضبط؟

أحتاج توضيح أكثر لجزء:

- `ScoreBreakdown`
- `ContextSnapshot`

أسئلتي:

1. ما هي **وظيفة ScoreBreakdown** بالضبط؟
   - هل هو مجرد log/trace رياضي يبيّن مكونات المعادلة؟
   - أم يُستخدم أيضًا في اتخاذ قرار آخر (مثلاً suppression، tuning، أو أي logic إضافي)؟
2. ما هي **الحقول/التفاصيل** التي يعرضها الـ dashboard للمستخدم من ScoreBreakdown و ContextSnapshot؟
   - مثلاً:
     - base_score
     - lineage_bonus
     - privilege_bonus
     - burst_bonus
     - ueba_bonus / ueba_discount
     - interaction_bonus
     - fp_discount
     - UserRoleWeight
     - DeviceCriticalityWeight
     - NetworkAnomalyFactor
     - ContextMultiplier
     - QualityFactor
     - RawScore
     - FinalScore
3. هل يمكن أن تشرح:
   - ما هي المعادلات/القيم التي يعتمد عليها الـ dashboard ليعرض هذه التفاصيل؟
   - ولماذا تم اختيار هذه المعادلات تحديدًا (من ناحية تحليل أمني)؟
   - ما فائدة كل مكوّن (bonus/discount/factor) في كل خطوة من خطوات حساب الـ risk؟

بمعنى آخر: أريد فهم **الدور العملي** لـ breakdown: ليس فقط “كيف يُحسب رياضيًا”، بل “لماذا هذا الجزء موجود؟” وكيف يساعد محلل الـ SOC أو يساعد في tuning.

***

## 3) شرح معادلات ومتغيرات raw_score بالتفصيل مع أمثلة رقمية

أريد نفس أسلوب الشرح الذي استخدمناه سابقًا  لكن هنا على معادلة:

\[
raw\_score = base\_score + lineage\_bonus + privilege\_bonus + burst\_bonus + ueba\_bonus + interaction\_bonus - fp\_discount - ueba\_discount
\]

أحتاج:

1. **شرح كل متغير**:

   - `base_score`:
     - كيف يتم حسابه؟ (من rule severity + match_count؟ كيف بالضبط؟)
   - `lineage_bonus`:
     - من أين يأتي؟ أي دالة؟ وما المنطق: متى يزيد؟ (مثلاً parent مشبوه، grandparent أنتي فيرس معطّل، الخ)
   - `privilege_bonus`:
     - ما الذي يدخل فيه؟ (SYSTEM, High integrity, is_elevated, unsigned, إلخ)
   - `burst_bonus`:
     - كيف يتم حسابه عبر `burstTracker`؟ ما هي القيم التقريبية؟ متى يزيد؟ (عدد hits في نافذة زمنية معينة...)
   - `ueba_bonus` و `ueba_discount`:
     - ما هو الـ UEBA signal؟ (anomaly/normal/none)
     - كيف يحوّل signal “anomaly” إلى +X وكيف يحوّل “normal” إلى -Y؟
   - `interaction_bonus`:
     - متى يتم إعطاء هذا bonus؟ (مثلاً تزامن أكثر من high signal معًا؟)
   - `fp_discount`:
     - متى يُخصم؟ (مثلاً توقيع Microsoft موثوق، binary معروف، مسار system شرعي...)

2. أريد **أمثلة رقمية بسيطة**:
   - مثال 1: alert بسيط (rule medium severity، لا lineage مشبوه، لا privileges عالية، لا burst، لا UEBA).
   - مثال 2: alert قوي (rule high، parent مشبوه، process elevated، burst عالي، UEBA anomaly، binary unsigned).

في كل مثال:

- ضع أرقام تقريبية لكل متغير.
- احسب `raw_score` خطوة بخطوة (مثل ما فعلت 

***

## 4) نفس الشيء مع معادلة quality_factor

أريد شرحًا شبيهًا لكن لمعادلـة:

\[
quality\_factor = f(context\_quality\_score,\ missing\_context\_fields)
\]

مع أن المنطق الحالي تقريبًا:

- `score >= 80` → 1.00
- `>= 60` → 0.97
- `>= 40` → 0.93
- `> 0` → 0.90
- `score == 0` مع حقول ناقصة → 0.85، ومع عدم وجود حقول ناقصة → 1.00

أحتاج:

1. **شرح كيف يتم حساب `context_quality_score` نفسه**:
   - ما هي الحقول التي تزيد score؟
   - هل يعتمد على completeness حقول مثل user, network, lineage, signature, إلخ؟
2. لماذا تم اختيار هذه الدرجات (0.85, 0.90, 0.93, 0.97, 1.00) بالذات؟
   - هل هي نتيجة tuning؟
   - أم مجرد قيم مبدئية منطقية؟
3. مثالين أو ثلاثة:
   - حالة سياق مكتمل تمامًا → quality_factor = 1.0
   - حالة سياق ناقص (مثلاً missing user_name و source_ip) → كيف ينخفض factor؟
   - حالة `context_quality_score = 0` + missing fields → quality_factor = 0.85، كيف يؤثر ذلك على final_score؟

***
خطوة A: إنشاء Policy من الـ Dashboard
نفترض أنك حفظت Policy:

scope_type = user
scope_value = me
user_role_weight = 0.8
device_criticality_weight = 1.2
network_anomaly_factor = 1.1
trusted_networks = ["10.10.0.0/16"]


ما دور trusted_networks في context
وعندما تم ادخال هذه الشبكة ["10.10.0.0/16"] يعني تعتبر trusted وباقي الشبكات الاخرى untrusted



عندي هذه القاعدة Agent A1: (1.0, 1.3, 1.0) عملتها سابقا 
ثم قمت بعمل نفس القاعدة ب قيم مختلفة Agent A1: (2.0, 0.3, 1.0) ما الذي سوف يحدث القاعدة السابقة


قبل CIDR adjustment:

UserRoleWeight = 1.0 * 1.0 * 0.8 = 0.8
DeviceCriticalityWeight = 1.0 * 1.3 * 1.2 = 1.56
NetworkAnomalyFactor = 1.0 * 1.0 * 1.1 = 1.1
حالة trusted IP
NetworkAnomalyFactor = 1.1 * 0.9 = 0.99
ContextMultiplier = 0.8 * 1.56 * 0.99 = 1.23552
حالة untrusted IP
NetworkAnomalyFactor = 1.1 * 1.1 = 1.21
ContextMultiplier = 0.8 * 1.56 * 1.21 = 1.51008


في حالة trusted IP تم استخدام القيمة 0.9 في معادلة NetworkAnomalyFactor

في حالة untrusted IPتم استخدام القيمة 1.1 في معادلة NetworkAnomalyFactor

على اي اساس تم تحديد هذه القيم ومن اين يتم التحكم بها او اين تخزن 



الهدف من كل هذا أن أفهم:

- كيف يتم بناء `raw_score` من مكوّنات متعددة.
- كيف يطبَّق عليه `context_multiplier` وعامل الجودة.
- وكيف تتحول هذه السلسلة إلى `final_score` الذي أراه في الـ dashboard، مع فهم دور كل خطوة وكل متغير في الـ context‑aware model.@context-aware-scoring-deep-dive.md 

---

**Cursor**

سأضيف لك إجابة مفصلة مباشرة داخل الملف `context-aware-scoring-deep-dive.md` كقسم **Q&A توضيحي** بنفس تنسيق RTL/LTR، ثم ألخّص لك أهم النقاط هنا أيضًا.

تم، وأضفت إجابات أسئلتك بالكامل داخل الملف نفسه:

- `context-aware-scoring-deep-dive.md`

وأضافت فيه قسم جديد:
- **`6) Q&A توضيحي (أسئلتك العملية)`**

ويحتوي تحديدًا على:

- هل `final_score` نسبة أم رقم 0..100
- هل يوجد threshold في backend لإخفاء alerts
- الفرق بين `severity` و `risk_score` ومن يلوّن الـ badges
- الدور العملي لـ `ScoreBreakdown` و `ContextSnapshot`
- شرح أعمق لكل مكوّن في `raw_score` مع أمثلة رقمية
- شرح `quality_factor` وأثره بالأرقام
- دور `trusted_networks` ومعنى CIDR trusted/untrusted
- ماذا يحدث عند إنشاء policy ثانية لنفس `agent A1` (unique scope constraint)
- مصدر قيم `0.9` و`1.1` (hard-coded) وأين تُتحكَّم

إذا تريد، الخطوة التالية أقدر أضيف لك **جدول Cheatsheet سريع** في آخر الملف:
- عمود `Field`
- عمود `Source`
- عمود `Used in Formula`
- عمود `Shown in UI`
بحيث يكون مرجع سريع أثناء التعديل.

---

**User**

طيب بصفتك مهندس امن معلومات وامن سيبراني ومحلل بيانات هل من وجهة نظرك الطريقة المستخدمة الحالية هي افضل ولا يوجد طريقة افضل منها وغير معقدة ويمكن تطبيقها بشكل صحيح وموثق من مصادر ومراجع 

---

**Cursor**

### 1. هل النموذج الحالي “جيد” من منظور أمن معلومات؟

برأيي كنموذج **جيل أول** لـ context‑aware scoring، ما عندك شيء “خاطئ” جوهريًا:

- **فكرة ممتازة** أن:
  - تبني `raw_score` من إشارات أمنية واضحة (severity, lineage, privilege, burst, UEBA, FP).
  - ثم تطبّق `context_multiplier` لعوامل (user / device / network).
  - ثم تعالج جودة السياق بـ `quality_factor`.
- هذا النمط (Base score + Context weights + Quality) **متناسق** مع أدبيات:
  - NIST SP 800‑61 (incident handling)
  - UEBA frameworks (behavioral baselines)
  - ممارسات EDR تجارية (CrowdStrike, Defender ATP، إلخ).

بمعنى آخر: **المنهجية العامة سليمة، مفهومة، وقابلة للتبرير أمام SOC و Audit.**

---

### 2. هل هي “أفضل طريقة ممكنة”؟ لا، لكنها توازن بين:

- **البساطة**:  
  - Multiplicative `ContextMultiplier` أسهل بكثير من نموذج ML كامل أو weighted logistic model.
  - يمكن تفسيره بسهولة لمحلل SOC:  
    “هذا الحدث مضروب ×1.5 لأنه admin على server حساس وخارج شبكة موثوقة”.

- **القابلية للتنفيذ**:
  - بسيط بما يكفي ليُطبق بشكل صحيح بدون بنية ML ثقيلة، ولا يحتاج بيانات تاريخية ضخمة لكل عامل.
  - كل شيء deterministic وموجود في Go/SQL؛ لا يعتمد على “صندوق أسود”.

- **الشفافية**:
  - `ScoreBreakdown` و`ContextSnapshot` يعطونك Explainability جيدة جدًا مقارنةً بكثير من الحلول التجارية.

من هذه الزاوية: **النموذج الحالي “جيد جدًا” كنقطة انطلاق عملية**.

---

### 3. أين يمكن أن يتطور/يتحسن بدون تعقيد زائد؟

إذا أردت تحسينه *بدون* تحويله إلى مشروع بحثي معقد، فهذه بعض النقاط المنطقية:

1. **معايرة العتبات والوزنات (Calibration) من بيانات حقيقية**
   - حاليًا كثير من القيم (15, 20, 30, 0.9, 1.1, ...) هي **tuned by design** وليست مبنية على إحصائيات محلية.
   - يمكن تحسينها لاحقًا عبر:
     - تحليل توزيع `risk_score` على Alerts حقيقية.
     - مقارنة true positive vs false positive.
     - ضبط الأوزان تدريجيًا (مثلاً: privilege bonus أقل أو burst أقل...).

2. **تحسين نموذج `ContextQuality`**
   - الآن مجرد buckets ثابتة.
   - يمكن لاحقًا:
     - جعل الحقول التي تفقدها weighted (فقدان `source_ip` ≠ فقدان `user_name`).
     - بناء score quality = sum(weights) بدل single scalar input.

3. **إضافة طبقة “Risk Level” موحدة**
   - حاليًا لديك:
     - `severity` من rule.
     - `risk_score` 0‑100.
   - يمكن إضافة mapping موحد:
     - 0–39 → Low
     - 40–69 → Medium
     - 70–89 → High
     - 90–100 → Critical  
   - بحيث يستخدم في triage، reports، وفلترة الـ alerts، ويكون موثق في مكان واحد.

4. **Guardrails أقوى للـ policies**
   - مثلاً:
     - منع مجموع السياق من تجاوز ×2 إلا لو كان هناك justification معينة.
     - UI يوضح dangerous configs (مثل user weight = 3.0 لو سمحت بها مستقبلاً).

---

### 4. هل توجد طرق “أفضل” جذريًا؟

نظريًا: نعم، ممكن تعمل:

- **نموذج إحصائي / ML** (logistic regression, gradient boosting) يتعلم:
  - weights المثلى لكل عامل.
  - interactions بينهم.
- **Bayesian risk model**:
  - يحسب احتمال compromise مقابل false positive بشكل أكثر دقة.

لكن:

- هذه النماذج:
  - أعقد في التنفيذ والصيانة.
  - تحتاج بيانات تاريخية موثوقة، labeling من SOC، pipeline تدريب، مراقبة drift...
  - أصعب في التفسير أمام المدققين.

لمنصة in‑house في هذه المرحلة، نموذجك الحالي **أفضل compromise** بين:
- الوضوح
- البساطة
- قابلية الضبط (tuning)
- الشفافية

---

### 5. الخلاصة العملية

- **لا أرى حاجة فورية لتغيير المنهجية الأساسية** (base + bonuses + discounts + multiplicative context + quality).
- ما تحتاجه مع الوقت هو:
  - **Calibration تدريجي** للأوزان والعتبات بناءً على بياناتك الحقيقية.
  - توثيق رسمي:
    - مستند شبيه بما كتبناه (وتحويله إلى internal design doc).
    - ربطه بمراجع (NIST, UEBA best practices, EDR vendor papers) كـ rationale.
  - إضافة بعض الــ guardrails في UI وbackend لمنع misconfiguration.

إذا أحببت، أقدر أقترح لك “Roadmap صغير” من 3–4 خطوات لتحويل النموذج الحالي إلى **v2 أكثر نضجًا** بدون أن تتحول لمنظومة ML كاملة.

---

**User**

 إذا أحببت، أقدر أقترح لك “Roadmap صغير” من 3–4 خطوات لتحويل النموذج الحالي إلى v2 أكثر نضجًا بدون أن تتحول لمنظومة ML كاملة.

نعم ارغب بهذا ,ولو اردت استخدام مخططات توضحية اكتبها ك اكواد mermaid

---

**Cursor**

<div dir="rtl">

## Roadmap مختصر لتطوير النموذج إلى v2 (بدون ML معقّد)

سأقسمها إلى 4 مراحل متتالية، كل مرحلة يمكن تنفيذها واستغلال نتائجها قبل الانتقال للمرحلة التالية.

---

### المرحلة 1: توحيد وتصميم طبقة Risk Level + Thresholds واضحة

**الهدف:** يكون عندك خريطة رسمية تربط `final_score` بمستويات خطورة (Low/Medium/High/Critical) وبمعاملة واضحة في الـ UI والـ Backend.

- عرّف mapping رسمي مثلًا:
  - 0–39 → `risk_level = low`
  - 40–69 → `risk_level = medium`
  - 70–89 → `risk_level = high`
  - 90–100 → `risk_level = critical`
- أضِف هذا الـ level:
  - كحقل محسوب في الـ backend (أو كـ computed في الـ API).
  - واستخدمه في:
    - ترتيب الـ alerts الافتراضي.
    - فلترة سريعة في الـ Dashboard (Buttons أو Chips).
- اترك suppression/الإخفاء لمرحلة لاحقة (لا تغلق alerts فقط لأن score منخفض في v2).

```mermaid
flowchart LR
  rawScore["raw_score"] --> finalScore["final_score (0..100)"]
  finalScore --> riskLevel["risk_level (low/med/high/crit)"]
  riskLevel --> uiBadges["UI Badges & Filters"]
```

**الفائدة:**  
توحيد فهم المخاطر بين الفريق (SOC / Engineering) وتبسيط triage بدون تغيير معادلاتك الجوهرية.

---

### المرحلة 2: جمع Telemetry عن التوزيع (Calibration بدون تغيير المنطق)

**الهدف:** قبل ما تغيّر الأوزان، اجمع بيانات عن سلوكها.

- أضف Logging/metrics بسيطة:
  - عدد alerts في كل bucket (`risk_level` + `severity`).
  - معدّل تصنيف SOC (مثلاً: قام المستخدم بتعليم alert كـ false_positive أو escalated).
- خزّن هذه الإحصائيات في جدول بسيط أو في metrics system (Prometheus/Grafana).

```mermaid
flowchart LR
  alerts["Alerts + risk_score"] --> statsCollector["Stats Collector"]
  statsCollector --> histo["Histograms per level"]
  statsCollector --> socFeedback["SOC Feedback (FP/TP tags)"]
```

**ما تغيّره هنا:**
- لا تغيّر المعادلات؛ فقط راقب:
  - هل معظم الـ true positives تقع في >70؟
  - هل أغلب الـ false positives في <50 مثلًا؟
- سجّل بعض الأمثلة يدويًا مع الفريق لفهم patterns.

---

### المرحلة 3: Tuning خفيف للأوزان والـ Context Factors بناءً على البيانات

**الهدف:** ضبط الأوزان الرقمية بحيث تعكس واقع بيئتك، بدون إدخال ML.

1. اختر 2–3 أماكن فقط للتعديل:
   - مثلًا:
     - `privilege_bonus` max من 40 → 30
     - `burst_bonus` thresholds
     - تأثير `UserRoleWeight` على `context_multiplier` (clamp من [0.5..2.0] إلى [0.7..1.8] مثلاً)
2. بعد كل تعديل:
   - راقب histogram السابق/اللاحق.
   - تأكّد أن:
     - نسبة alerts المصنفة High/Critical ليست عالية جدًا (تسبب noise).
     - ولا منخفضة جدًا (تفقد الحساسية).

```mermaid
flowchart LR
  metrics["Collected Metrics"] --> tuningWorkshop["Tuning Session"]
  tuningWorkshop --> newParams["Adjusted Weights & Thresholds"]
  newParams --> scorer["RiskScorer v2"]
  scorer --> metrics
```

**نصيحة عملية:**  
ابدأ بـ parameters التي لها أكبر تأثير (مثل interaction/privilege/burst)، واترك context_multiplier كما هو في الدورة الأولى، ثم عد له لاحقًا.

---

### المرحلة 4: تحسين Model الجودة (Context Quality v2) + Guardrails للسياسات

**الهدف:** جعل `quality_factor` و `context_policies` أكثر دقة بدون تعقيد كبير.

1. **Context Quality v2:**
   - بدل scalar وحيد، فكّره كجمع لوزنات حقول:
     - كل حقل سياقي مهم (user, ip, lineage, signature, ...) له وزن.
     - quality_score = sum(weights_of_present_fields) / sum(all_weights) × 100
   - ثم استخدم نفس buckets تقريبًا (0.85..1.0)، لكن الآن مبنية على completeness حقيقية.

2. **Guardrails للـ Policies:**
   - على مستوى UI + Backend:
     - منع قيم خارج نطاقات آمنة (مثلاً weight > 2.0 أو < 0.5 إلا بمود developer).
     - تحذير في الـ UI إذا net effect لمجموعة السياسات يمكن أن يتجاوز `multiplier > 2.5` بانتظام.
   - إضافة صفحة صغيرة في Dashboard تريك:
     - توزيع `ContextMultiplier` (كم مرة 0.5–0.8، 0.8–1.2، 1.2–2.0، إلخ).

```mermaid
flowchart LR
  fields["Context Fields Presence"] --> qualityCalc["QualityScore v2 (weighted)"]
  qualityCalc --> qualityFactor["quality_factor"]
  qualityFactor --> finalScore
```

```mermaid
flowchart LR
  policies["Context Policies"] --> effectSim["Multiplier Simulation"]
  effectSim --> uiWarnings["UI Warnings / Guardrails"]
```

**الفائدة:**  
- context-aware يصبح “أذكى” في التعامل مع نقص البيانات.  
- تقل احتمالية misconfiguration من السياسات التي تضخم أو تخفض المخاطر بشكل مفرط.

---

### خلاصة سريعة

- **المرحلة 1:** ثبّت خريطة `final_score → risk_level` واستخدمها في واجهتك.  
- **المرحلة 2:** اجمع بيانات عن توزيع الـ scores والـ feedback من SOC بدون أن تغيّر المنطق.  
- **المرحلة 3:** عدّل الأوزان والعتبات بشكل محدود مبني على البيانات (tuning خفيف).  
- **المرحلة 4:** طوّر نموذج الجودة وguardrails للسياسات لتحسين الدقة ومنع الأخطاء البشرية.

بهذا الشكل، تتحرك من v1 → v2 **منهجية، قابلة للتنفيذ، ومتصاعدة** بدون القفز إلى ML معقد أو إعادة كتابة النموذج بالكامل.

إذا أحببت، أقدر في رسالة لاحقة أقترح لك **Checklist تنفيذية** لكل مرحلة (بنود TODO على مستوى ملفات Go/SQL/React) بحيث تنفذها خطوة خطوة في الكود.

---

**User**

تمام , اريد تنفيذ جميع التحسينات كاملة بشكل موثوق وصحيح , ويجب ان تعطيني توثيق كامل لجميع الpipline for algorith  كاملة وكيف تتم العملية كاملة مع التبرير , يجب ان توثق كل شي مع كتابة مسار الملف الموجود فيه الكود المسول عن ذالك ومع كتابتة جزء الكود المسوؤل, وايضا اريد ان تون الاوازن والقيم والمتغيرات التي توجد حاليا في الكود يتم ادارتها والتحكم بها من ملف واحد من ملف config لكي يسهل ادارتها واغييرها في المستقبل , ايضا يجب ان توثق كل المعادلات وكل حقولها وكيف تحسب واين ومتى للمعادلة ولكل حقل فيها, مع التبريرات وايضا اريد ان توضح لي الناتج النهائي الذي يظهر درجة او خطورة الاشعار في الdashboard وكيف تمت حسابه وكيف تحسب كل قيمة للمعادلة , واذا كانت هناك قيم ثابتة اذكرها واين هي وكيف تم تحدييها بناء على ماذا , من البداية الى النهاية مع سيناريو ومثال للتوضيح مع تبريرات كل خطوة والمراجع الذي اعتمدت عليها في اجراء كل العمليات, 

توثيق رسمي:
مستند شبيه بما كتبناه (وتحويله إلى internal design doc).
ربطه بمراجع (NIST, UEBA best practices, EDR vendor papers) كـ rationale.

Implement the plan as specified, it is attached for your reference. Do NOT edit the plan file itself.

To-do's from the plan have already been created. Do not create them again. Mark them as in_progress as you work, starting with the first one. Don't stop until you have completed all the to-dos.

@riskscoring-v2-design-and-tuning_ece69617.plan.md  buildit

