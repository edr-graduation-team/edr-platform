<div dir="rtl">

## وثيقة تصميم داخلية (Internal Design Doc)

### الموضوع

**Context-Aware Risk Scoring v2** في منصة `edr-platform` — من لحظة وصول الـ event حتى ظهور `risk_score` و`risk_level` في الـ Dashboard، مع توثيق المعادلات، مصادر القيم، ملفات الكود المسؤولة، والقيم الثابتة وكيف تتم إدارتها من ملف config واحد.

### الهدف من v2

- **مصدر واحد للثوابت**: كل الأوزان/العتبات/الـ multipliers الخاصة بالـ scoring أصبحت تُدار من:
  - `sigma_engine_go/config/config.yaml` تحت المفتاح `risk_scoring:`
- **تفسير موحّد** لدرجة الخطورة في الواجهة:
  - `risk_level` يُحسب في الـ backend من `risk_score` باستخدام عتبات من `risk_scoring.risk_levels`.
- **سياسات سياق deterministic**:
  - Policy واحدة فعّالة لكل `(scope_type, scope_value)` ويتم **استبدالها (upsert)** بدل stacking.

</div>

<div dir="rtl">

## 8) Quick Reference (Field → Source → Formula → UI)

</div>

<div dir="ltr">

| Field | Source (code/data) | Formula / Computation | UI Surface |
|---|---|---|---|
| `base_score` | `computeBaseScore(...)` in `risk_scorer.go`; severity from `primary.Rule.Severity()` | Severity map + match correlation bonus (`risk_scoring.base_score.*`) | `Alerts` Context modal (`score_breakdown.base_score`) |
| `lineage_bonus` | `rs.matrix.ComputeBonus(lineageChain)` + lineage from cache lookup | Suspicion matrix bonus by process ancestry | `Alerts` Context modal (`score_breakdown.lineage_bonus`) |
| `privilege_bonus` | `computePrivilegeBonus(event.RawData, cfg.Privilege)` | Adds configured bonuses for SYSTEM/admin/elevation/signature states | `Alerts` Context modal (`score_breakdown.privilege_bonus`) |
| `burst_bonus` | `BurstTracker.IncrAndGet(...)` then `computeBurstBonus(...)` | Thresholded bonus from configured burst buckets | `Alerts` Context modal (`score_breakdown.burst_bonus`) |
| `fp_discount` | `computeFPDiscount(sigStatus, executable, cfg.FalsePositive)` | Discount for trusted signed binaries/paths | `Alerts` Context modal (`score_breakdown.fp_discount`) |
| `false_positive_risk` | `computeFPRisk(sigStatus, executable, cfg.FalsePositive)` | Probability bucket by signature/path trust | `Alerts` summary + details |
| `ueba_bonus` / `ueba_discount` | `computeUEBA(...)` using baseline provider | Anomaly bonus or normalcy discount from UEBA thresholds | `Alerts` Context modal (`score_breakdown.ueba_*`) |
| `interaction_bonus` | `computeInteractionBonus(..., cfg.Interaction)` | Extra bonus when multiple high signals co-occur | `Alerts` Context modal (`score_breakdown.interaction_bonus`) |
| `UserRoleWeight` | `ContextPolicyProvider.Resolve(...)` | Multiplicative factor from policy precedence (`global→agent→user`) | `context_snapshot` + details panel |
| `DeviceCriticalityWeight` | `ContextPolicyProvider.Resolve(...)` | Multiplicative factor from matched policies | `context_snapshot` + details panel |
| `NetworkAnomalyFactor` | `ContextPolicyProvider.Resolve(...)` + CIDR trusted check | Policy factor then trusted/untrusted multiplier | `context_snapshot` + details panel |
| `context_multiplier` | `ContextFactors.Multiplier(cfg.ContextPolicy)` | `clamp(user*device*network, min,max)` | `score_breakdown.context_multiplier` |
| `quality_factor` | `computeContextQualityFactor(score, missing, cfg.Quality)` | Bucketed multiplier from context completeness | `score_breakdown.quality_factor` |
| `raw_score` | `Score(...)` in `risk_scorer.go` | Sum bonuses − discounts before multipliers | `score_breakdown.raw_score` |
| `context_adjusted_score` | `Score(...)` in `risk_scorer.go` | `round(raw_score * context_multiplier * quality_factor)` | `score_breakdown.context_adjusted_score` |
| `final_score` / `risk_score` | `Score(...)` then persisted in alerts table | `clamp(context_adjusted_score, 0, 100)` | Alerts table badge + detail modal + endpoint aggregate APIs |
| `risk_level` | `RiskLevelFromScore(...)` in `risk_level.go`; added by `alerts.go` response mapper | Threshold mapping from `risk_scoring.risk_levels.*` | `Alerts` badge styling (prefers backend `risk_level`) |

</div>

<div dir="ltr">

## 1) End-to-End Pipeline (Agent → Sigma Engine → DB → API → Dashboard)

### 1.1 Execution flow (high level)

```mermaid
flowchart LR
  agent[AgentTelemetry] --> cm[ConnectionManagerIngestion]
  cm --> kafka[KafkaEventsRawTopic]
  kafka --> loop[EventLoop]
  loop --> detect[DetectAggregated]
  detect --> gen[GenerateAggregatedAlert]
  gen --> score[RiskScorerScore]
  score --> persist[PersistSigmaAlerts]
  persist --> api[AlertsApi]
  api --> ui[DashboardAlerts]
  persist --> cmApi[EndpointRiskApi]
  cmApi --> epUi[DashboardEndpointRisk]
```

### 1.2 Where each step lives (code map)

- **Event processing and scoring invocation**
  - `sigma_engine_go/internal/infrastructure/kafka/event_loop.go`
    - Calls `RiskScorer.Score(...)` right after alert generation.
- **Scoring algorithm**
  - `sigma_engine_go/internal/application/scoring/risk_scorer.go`
  - `sigma_engine_go/internal/application/scoring/context_policy_provider.go`
  - `sigma_engine_go/internal/application/scoring/context_snapshot.go`
  - `sigma_engine_go/internal/application/scoring/scoring_config.go`
- **Persistence**
  - `sigma_engine_go/internal/infrastructure/database/migrations/014_add_risk_scoring.up.sql`
  - `sigma_engine_go/internal/infrastructure/database/alert_writer.go`
  - `sigma_engine_go/internal/infrastructure/database/alert_repo.go`
- **Alerts API**
  - `sigma_engine_go/internal/handlers/alerts.go`
  - `sigma_engine_go/internal/handlers/server.go`
- **Dashboard**
  - `dashboard/src/api/client.ts`
  - `dashboard/src/pages/Alerts.tsx`
  - `dashboard/src/pages/EndpointRisk.tsx`
  - `dashboard/src/pages/settings/ContextPolicies.tsx`

</div>

<div dir="rtl">

## 2) مصدر الثوابت (Single Source of Truth)

### ملف الإعدادات

- **الملف**: `sigma_engine_go/config/config.yaml`
- **المفتاح**: `risk_scoring:`
- **الكود الذي يحمّله**:
  - `sigma_engine_go/internal/infrastructure/config/config.go`
  - حيث تمت إضافة `RiskScoring scoring.RiskScoringConfig \`yaml:\"risk_scoring\"\``

### لماذا هذا مهم؟

لأن كل الأرقام التي كانت موزعة داخل `risk_scorer.go` و`context_policy_provider.go` أصبحت الآن:
- قابلة للتغيير من YAML دون إعادة بناء منطق الكود.
- موثقة في مكان واحد.
- قابلة للتدقيق (Audit) والتبرير (Rationale).

</div>

<div dir="ltr">

## 3) Core Algorithm: Equations, Field Provenance, and Timing

All equations below are computed inside:
- `DefaultRiskScorer.Score(...)` in `sigma_engine_go/internal/application/scoring/risk_scorer.go`

### 3.0 Canonical code snippet (score assembly)

```go
// sigma_engine_go/internal/application/scoring/risk_scorer.go
raw := baseScore + lineageBonus + privilegeBonus + burstBonus + uebaBonus + interactionBonus - fpDiscount - uebaDiscount
contextAdjusted := int(math.Round(float64(raw) * contextFactors.Multiplier(rs.cfg.ContextPolicy) * qualityFactor))
finalScore := clamp(contextAdjusted, 0, 100)
```

### 3.1 Raw score equation

\[
raw\_score =
base\_score
 + lineage\_bonus
 + privilege\_bonus
 + burst\_bonus
 + ueba\_bonus
 + interaction\_bonus
 - fp\_discount
 - ueba\_discount
\]

**Where computed**
- `risk_scorer.go` inside `Score(...)`:
  - `raw := baseScore + lineageBonus + privilegeBonus + burstBonus + uebaBonus + interactionBonus - fpDiscount - uebaDiscount`

**Field provenance**
- `base_score`: `computeBaseScore(severity, matchCount, cfg.BaseScore)`
  - Severity comes from Sigma rule: `primary.Rule.Severity()`
  - Match count: `input.MatchResult.MatchCount()`
  - Constants: `risk_scoring.base_score.*`
- `lineage_bonus`: `rs.matrix.ComputeBonus(lineageChain)`
  - Lineage chain from cache: `rs.lineageCache.GetLineageChain(...)`
  - Matrix: `sigma_engine_go/internal/application/scoring/suspicion_matrix.go`
- `privilege_bonus`: `computePrivilegeBonus(event.RawData, cfg.Privilege)`
  - Fields: `user_sid`, `integrity_level`, `is_elevated`, `signature_status`, `executable`
  - Constants: `risk_scoring.privilege.*`
- `burst_bonus`: `computeBurstBonus(burstCount, cfg.Burst)`
  - Count from `BurstTracker.IncrAndGet(agentID, ruleCategory)`
  - Constants: `risk_scoring.burst.threshold_*` and `risk_scoring.burst.bonus_*`
- `fp_discount` and `false_positive_risk`:
  - `computeFPDiscount(sigStatus, executable, cfg.FalsePositive)`
  - `computeFPRisk(sigStatus, executable, cfg.FalsePositive)`
  - Constants: `risk_scoring.false_positive.*`
- `ueba_bonus / ueba_discount`:
  - `rs.computeUEBA(...)` uses baseline provider
  - Constants: `risk_scoring.ueba.*`
- `interaction_bonus`:
  - `computeInteractionBonus(..., cfg.Interaction)`
  - Constants: `risk_scoring.interaction.*`

### 3.2 Context factors and multiplier

Context factors are returned by:
- `ContextPolicyProvider.Resolve(ctx, agentID, userName, sourceIP)`
  - file: `sigma_engine_go/internal/application/scoring/context_policy_provider.go`

Factors:
- `UserRoleWeight`
- `DeviceCriticalityWeight`
- `NetworkAnomalyFactor`

Multiplier:
\[
context\_multiplier = clamp(UserRoleWeight \times DeviceCriticalityWeight \times NetworkAnomalyFactor,\ min,\ max)
\]

**Where computed**
- `ContextFactors.Multiplier(cfg.ContextPolicy)`

Canonical code:

```go
// sigma_engine_go/internal/application/scoring/context_policy_provider.go
func (f ContextFactors) Multiplier(cfg ContextPolicyConfig) float64 {
    cfg = defaultContextPolicyConfig(cfg)
    return clampFloat(
        f.UserRoleWeight*f.DeviceCriticalityWeight*f.NetworkAnomalyFactor,
        cfg.MultiplierClampMin, cfg.MultiplierClampMax,
    )
}
```

**Constants**
- `risk_scoring.context_policy.per_factor_clamp_min/max`
- `risk_scoring.context_policy.multiplier_clamp_min/max`
- `risk_scoring.context_policy.trusted_network_multiplier`
- `risk_scoring.context_policy.untrusted_network_multiplier`

### 3.3 Quality factor

\[
quality\_factor = f(context\_quality\_score,\ missing\_context\_fields)
\]

**Where computed**
- `computeContextQualityFactor(contextQualityScore, missingCount, cfg.Quality)`

**Constants**
- `risk_scoring.quality.*`

### 3.4 Context-adjusted score and final score

\[
context\_adjusted = round(raw\_score \times context\_multiplier \times quality\_factor)
\]
\[
final\_score = clamp(context\_adjusted, 0, 100)
\]

**Where computed**
- `contextAdjusted := round(raw * contextFactors.Multiplier(...) * qualityFactor)`
- `finalScore := clamp(contextAdjusted, 0, 100)`

</div>

<div dir="rtl">

## 4) `risk_level` وكيف يظهر في الـ Dashboard

### من أين يأتي؟

- `risk_level` يُحسب في الـ Sigma Engine API layer وليس في UI.
- الكود:
  - `sigma_engine_go/internal/application/scoring/risk_level.go`
  - `sigma_engine_go/internal/handlers/alerts.go` (يضيفه إلى response)
  - `sigma_engine_go/internal/handlers/server.go` (يمرر config thresholds إلى `AlertHandler`)

### العتبات (Thresholds)

من `sigma_engine_go/config/config.yaml`:
- `risk_scoring.risk_levels.low_max`
- `risk_scoring.risk_levels.medium_max`
- `risk_scoring.risk_levels.high_max`
- `risk_scoring.risk_levels.critical_min`

### أين يظهر؟

- صفحة **Alerts**:
  - `dashboard/src/pages/Alerts.tsx`
  - `RiskScoreBadge` يفضل `alert.risk_level` إن وُجد.

</div>

<div dir="ltr">

Canonical code (server → handler → response):

```go
// sigma_engine_go/cmd/sigma-engine-kafka/main.go
apiServer = handlers.NewServer(apiCfg, ruleRepo, alertRepo, auditLogger, cfg.RiskScoring.RiskLevels)
```

```go
// sigma_engine_go/internal/handlers/alerts.go
RiskLevel: scoring.RiskLevelFromScore(alert.RiskScore, h.riskLevels),
```

## 5) Context Policy Model (Deterministic Replacement, not stacking)

### Storage

- Table: `context_policies`
  - migration: `connection-manager/internal/database/migrations/017_create_context_policies.up.sql`
  - uniqueness: `UNIQUE(scope_type, scope_value)`

### CRUD hardening (replacement semantics)

- `connection-manager/internal/repository/context_policy_repo.go`
  - `Create(...)` is implemented as **UPSERT** on `(scope_type, scope_value)`
- `connection-manager/pkg/api/handlers_other.go`
  - validation enforces `global` must use `scope_value='*'`

### Resolution precedence

In Sigma Engine provider:
- `global -> agent -> user`
- multiply factors and clamp each step
- apply trusted/untrusted network adjustment based on `sourceIP` CIDR membership

Provider code:
- `sigma_engine_go/internal/application/scoring/context_policy_provider.go`

</div>

<div dir="rtl">

## 6) سيناريو عملي موثّق بالأرقام (Trusted vs Untrusted)

### سياسة تم إنشاؤها

نفترض أنك أنشأت Policy (scope=user, scope_value=me) مع:\n- `user_role_weight=0.8`\n- `device_criticality_weight=1.2`\n- `network_anomaly_factor=1.1`\n- `trusted_networks=[\"10.10.0.0/16\"]`\n\nوسياسة agent (A1):\n- `device_criticality_weight=1.3`\n\n### حساب الـ factors\n\nقبل CIDR:\n- user = 0.8\n- device = 1.3 * 1.2 = 1.56\n- network = 1.1\n\nTrusted:\n- network *= `trusted_network_multiplier` (default 0.9) => 0.99\n- multiplier = 0.8 * 1.56 * 0.99 = 1.23552\n\nUntrusted:\n- network *= `untrusted_network_multiplier` (default 1.1) => 1.21\n- multiplier = 0.8 * 1.56 * 1.21 = 1.51008\n\n### افترض raw_score=62 وquality_factor=0.97\n\nTrusted:\n- adjusted = round(62 * 1.23552 * 0.97) = 74\n- final_score=74 → risk_level=high\n\nUntrusted:\n- adjusted = round(62 * 1.51008 * 0.97) = 91\n- final_score=91 → risk_level=critical\n\n**نفس raw_score، لكن عامل الشبكة نقل القرار من High إلى Critical.**\n\n</div>

<div dir="ltr">

## 7) Rationale and References (for audit)

This design follows widely used security engineering patterns:\n\n- **NIST SP 800-61**: incident triage and prioritization concepts (severity, context, response urgency).\n- **NIST SP 800-92**: log management and statistical anomaly rationale (referenced in UEBA Z-score logic).\n- **UEBA best practices**: confidence gating, baseline-driven anomaly vs normalcy discount.\n\nImplementation rationale:\n- Additive raw score captures independent security signals.\n- Multiplicative context multiplier provides proportional amplification/suppression.\n- Quality factor prevents overconfident scoring under missing context.\n- Risk level tiering provides consistent analyst triage semantics.\n\n</div>

