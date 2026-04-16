<div dir="rtl">

# شرح Context-Aware Scoring (مركّز وعميق)

هذا المستند يشرح فقط جزء **Context-Aware** في نظام التقييم، مع:

- المعادلات الأساسية
- مصدر كل قيمة (كود + قاعدة البيانات)
- تسلسل الـ Pipeline من الحدث حتى التخزين
- كيف تنعكس النتيجة في الـ Dashboard
- سيناريو عملي كامل
- خارطة إضافة عامل سياقي جديد بأمان

</div>

---

<div dir="rtl">

## 1) المعادلات الأساسية

</div>

<div dir="ltr">

### 1.1 Raw Score

\[
raw\_score = baseScore + lineageBonus + privilegeBonus + burstBonus + uebaBonus + interactionBonus - fpDiscount - uebaDiscount
\]

</div>

<div dir="rtl">

**مكانها في الكود:**

- `sigma_engine_go/internal/application/scoring/risk_scorer.go`
- داخل `RiskScorer.Score(...)`:
  - `raw := baseScore + lineageBonus + privilegeBonus + burstBonus + uebaBonus + interactionBonus - fpDiscount - uebaDiscount`

**شرح كل حقل (من أين يأتي وكيف يُحسب):**

- `baseScore`: من `computeBaseScore(...)` حسب severity وعدد الـ matches.
- `lineageBonus`: من `SuspicionMatrix.ComputeBonus(lineageChain)`.
- `privilegeBonus`: من `computePrivilegeBonus(event.RawData)` عبر `user_sid`, `integrity_level`, `is_elevated`, `signature_status`.
- `burstBonus`: من `computeBurstBonus(burstCount)` بعد `burstTracker.IncrAndGet(...)`.
- `uebaBonus`/`uebaDiscount`: من `computeUEBA(...)`.
- `interactionBonus`: من `computeInteractionBonus(...)`.
- `fpDiscount`: من `computeFPDiscount(signature_status, executable)`.

**دوره في الـ context-aware:**

هذا يمثل **النتيجة الخام** قبل تطبيق عوامل السياق (`ContextMultiplier`) وقبل عامل الجودة (`QualityFactor`).

</div>

<div dir="ltr">

### 1.2 Context Factors

`ContextFactors` fields:

- `UserRoleWeight`
- `DeviceCriticalityWeight`
- `NetworkAnomalyFactor`

</div>

<div dir="rtl">

**مكانها في الكود:**

- `sigma_engine_go/internal/application/scoring/context_policy_provider.go`
- النوع: `ContextFactors`
- يتم إنتاجها بواسطة: `ContextPolicyProvider.Resolve(ctx, agentID, userName, sourceIP)`

**كيف تتكوّن القيم:**

1. تحميل سياسات `context_policies` من Postgres.
2. تطبيق الترتيب: `global -> agent -> user`.
3. لكل policy مطابقة:
   - ضرب كل عامل بعامل policy المقابل.
   - Clamping لكل عامل ضمن `[0.5, 2.0]`.
4. تعديل عامل الشبكة حسب CIDR:
   - داخل trusted network => `*0.9`
   - خارج trusted network => `*1.1`
   - ثم clamp `[0.5, 2.0]`.

</div>

<div dir="ltr">

### 1.3 Context Multiplier

\[
context\_multiplier = clamp(UserRoleWeight \times DeviceCriticalityWeight \times NetworkAnomalyFactor,\ 0.25,\ 4.0)
\]

</div>

<div dir="rtl">

**مكانها في الكود:**

- `ContextFactors.Multiplier()` في `context_policy_provider.go`.

**الدور:**

يحوّل السياق إلى معامل مضاعِف/مخفِّض للـ raw score.

</div>

<div dir="ltr">

### 1.4 Quality Factor

\[
quality\_factor = f(context\_quality\_score,\ missing\_context\_fields)
\]

Range: `[0.85 .. 1.00]`

</div>

<div dir="rtl">

**مكانها في الكود:**

- `computeContextQualityFactor(...)` في `risk_scorer.go`.

**المنطق الحالي:**

- `score >= 80` => `1.00`
- `>= 60` => `0.97`
- `>= 40` => `0.93`
- `> 0` => `0.90`
- `score == 0`:
  - بدون missing fields => `1.00`
  - مع missing fields => `0.85`

**الدور:**

تقليل الثقة عند نقص السياق (uncertainty handling).

</div>

<div dir="ltr">

### 1.5 Context Adjusted Score

\[
context\_adjusted\_score = round(raw\_score \times context\_multiplier \times quality\_factor)
\]

</div>

<div dir="rtl">

**مكانها في الكود:**

- `risk_scorer.go`:
  - `contextAdjusted := int(math.Round(float64(raw) * contextFactors.Multiplier() * qualityFactor))`

</div>

<div dir="ltr">

### 1.6 Final Score

\[
final\_score = clamp(context\_adjusted\_score,\ 0,\ 100)
\]

</div>

<div dir="rtl">

**مكانها في الكود:**

- `risk_scorer.go`:
  - `finalScore := clamp(contextAdjusted, 0, 100)`

**هذا هو الرقم النهائي الذي تراه في الواجهة.**

</div>

---

<div dir="rtl">

## 2) ربط المعادلات بالـ Pipeline (Agent -> Scorer -> DB -> UI)

</div>

<div dir="ltr">

1. Agent sends event telemetry (process/network/user context).
2. Sigma engine performs rule matching.
3. `RiskScorer.Score(...)` is called in event loop.
4. Scorer computes:
   - raw score
   - context factors + multiplier
   - quality factor
   - context-adjusted + final score
5. Scorer builds:
   - `ScoreBreakdown`
   - `ContextSnapshot`
6. Persist to DB (`sigma_alerts`):
   - `risk_score`
   - `context_snapshot`
   - `score_breakdown`
7. Dashboard fetches and renders.

</div>

<div dir="rtl">

**نقطة الاستدعاء الأساسية:**

- `sigma_engine_go/internal/infrastructure/kafka/event_loop.go`
- بعد توليد alert مباشرة يتم استدعاء `riskScorer.Score(...)`.

**نقطة التخزين:**

- `sigma_engine_go/internal/infrastructure/database/alert_repo.go`
- migration: `sigma_engine_go/internal/infrastructure/database/migrations/014_add_risk_scoring.up.sql`

</div>

---

<div dir="rtl">

## 3) أين تظهر النتائج في الـ Dashboard؟

</div>

<div dir="ltr">

### Alerts API and Pages

- API types and endpoints: `dashboard/src/api/client.ts`
  - `GET /api/v1/sigma/alerts`
- UI: `dashboard/src/pages/Alerts.tsx`
  - Table risk column (`risk_score`)
  - Context modal:
    - `context_snapshot`
    - `score_breakdown`
    - context weights + multiplier + quality factor

### Endpoint Risk

- API: `GET /api/v1/alerts/endpoint-risk` (connection-manager aggregate)
- UI: `dashboard/src/pages/EndpointRisk.tsx`
  - Shows per-endpoint `peak_risk_score` and `avg_risk_score`
  - Ranking reflects final scored alerts.

### Context Policies Settings

- UI: `dashboard/src/pages/settings/ContextPolicies.tsx`
- API: `/api/v1/context-policies`

</div>

---

<div dir="rtl">

## 4) سيناريو عملي كامل (من Dashboard إلى final score)

### خطوة A: إنشاء Policy من الـ Dashboard

نفترض أنك حفظت Policy:

- `scope_type = user`
- `scope_value = me`
- `user_role_weight = 0.8`
- `device_criticality_weight = 1.2`
- `network_anomaly_factor = 1.1`
- `trusted_networks = ["10.10.0.0/16"]`

### خطوة B: التخزين

- تُخزن في جدول `context_policies` عبر:
  - handler: `connection-manager/pkg/api/handlers_other.go`
  - repo: `connection-manager/internal/repository/context_policy_repo.go`

### خطوة C: وصول Event جديد

نفترض event يحتوي:

- `agent_id = A1`
- `user_name = "me"`
- `source_ip` مرة داخل الشبكة الموثوقة ومرة خارجها.

نفترض السياسات المطابقة:

- Global: `(1.0, 1.0, 1.0)`
- Agent A1: `(1.0, 1.3, 1.0)`
- User me: `(0.8, 1.2, 1.1)`

قبل CIDR adjustment:

- `UserRoleWeight = 1.0 * 1.0 * 0.8 = 0.8`
- `DeviceCriticalityWeight = 1.0 * 1.3 * 1.2 = 1.56`
- `NetworkAnomalyFactor = 1.0 * 1.0 * 1.1 = 1.1`

#### حالة trusted IP

- `NetworkAnomalyFactor = 1.1 * 0.9 = 0.99`
- `ContextMultiplier = 0.8 * 1.56 * 0.99 = 1.23552`

#### حالة untrusted IP

- `NetworkAnomalyFactor = 1.1 * 1.1 = 1.21`
- `ContextMultiplier = 0.8 * 1.56 * 1.21 = 1.51008`

نفترض:

- `raw_score = 62`
- `quality_factor = 0.97`

trusted:

- `context_adjusted_score = round(62 * 1.23552 * 0.97) = 74`
- `final_score = 74`

untrusted:

- `context_adjusted_score = round(62 * 1.51008 * 0.97) = 91`
- `final_score = 91`

### خطوة D: الانعكاس في الواجهة

- `Alerts` table: يظهر الرقم النهائي 74 أو 91.
- Alert detail modal: يظهر breakdown + context factors + multiplier + quality.
- `EndpointRisk`: يتأثر عبر تجميع `risk_score` النهائي (peak/avg).

</div>

---

<div dir="rtl">

## 5) خارطة إضافة عامل سياقي جديد (مثال: GeoLocationRiskFactor)

### Backend

1. إضافة الحقل في:
   - `ContextFactors` (`context_policy_provider.go`)
2. إدخاله في:
   - `ContextFactors.Multiplier()`
3. إضافة عمود DB في:
   - migration لـ `context_policies`
4. تحديث mapping في:
   - provider scan + connection-manager model + CRUD handlers
5. تحديث explainability:
   - `ScoreBreakdown` + `ContextSnapshot`

### Dashboard

1. تحديث types في `dashboard/src/api/client.ts`
2. عرض العامل الجديد في `Alerts.tsx` context panel
3. إضافة input في `ContextPolicies.tsx`

### Safety Checklist

- default DB value = `1.0`
- clamping واضح ومختبر
- tests لـ:
  - precedence
  - multiplier math
  - trusted/untrusted branch
  - final clamp

</div>

---

<div dir="rtl">

## 6) Q&A توضيحي (أسئلتك العملية)

### 6.1 هل `final_score` نسبة مئوية أم مجرد رقم؟

- `final_score` هو **رقم scalar من 0 إلى 100**.
- ليس Percentage رسميًا في التخزين (لا يوجد `%` في DB).
- عمليًا يمكن قراءته كتدرّج مشابه للنسبة، لكن النظام يتعامل معه كـ integer score.

### 6.2 هل يوجد Threshold في Backend لإخفاء Alerts؟

- في التطبيق الحالي: **لا يوجد شرط backend عام يقول “إذا risk_score أقل من X لا تنشئ alert”**.
- الـ alert يظهر لأنه matched rule، ثم يتم إرفاق `risk_score`.
- في الـ Sigma alert API يتم إرجاع alerts حسب الفلاتر المعتادة، مع sorting (`risk_score` أو `timestamp`).

### 6.3 من يحدد Badges/ألوان الخطورة؟

- يوجد مستويان مختلفان:
  1. `severity` (critical/high/...) قادم من rule logic.
  2. `risk_score` (0..100) context-aware score.
- واجهة `Alerts.tsx` تعرض كلاهما:
  - Badge للـ severity
  - Risk badge للـ risk score
- عتبات تصنيف risk (مثل 90+ Critical) تظهر في الـ UI كتصنيف بصري.

### 6.4 ما وظيفة `ScoreBreakdown` عمليًا؟

- `ScoreBreakdown` = **أثر حسابي/تحليلي explainability artifact**:
  - يشرح لماذا خرجت الدرجة النهائية بهذا الرقم.
  - يفيد SOC في triage.
  - يفيد Engineering في tuning.
- حاليًا ليس policy engine مستقل لاتخاذ قرار suppression إضافي من breakdown نفسه (القرار النهائي للعرض لا يعتمد على breakdown بل على وجود alert واسترجاعه).

### 6.5 ما وظيفة `ContextSnapshot`؟

- `ContextSnapshot` = حاوية forensic:
  - process/lineage/privilege/rule metadata
  - score breakdown
  - context factors
  - quality + warnings
- الهدف: إعادة بناء سبب القرار لاحقًا بشكل قابل للتدقيق.

</div>

<div dir="ltr">

#### Main breakdown fields shown in UI

- `base_score`
- `lineage_bonus`
- `privilege_bonus`
- `burst_bonus`
- `ueba_bonus`
- `ueba_discount`
- `interaction_bonus`
- `fp_discount`
- `user_role_weight`
- `device_criticality_weight`
- `network_anomaly_factor`
- `context_multiplier`
- `context_quality_score`
- `quality_factor`
- `raw_score`
- `context_adjusted_score`
- `final_score`

</div>

<div dir="rtl">

### 6.6 لماذا هذه المكوّنات موجودة (قيمة أمنية)؟

- `base_score`: يعطي نقطة البداية من خطورة القاعدة نفسها.
- `lineage_bonus`: يلتقط علاقات parent-child المشبوهة.
- `privilege_bonus`: يرفع الخطورة مع صلاحيات عالية/سياقات حساسة.
- `burst_bonus`: يلتقط الهجمات المتكررة بزمن قصير.
- `ueba_bonus/discount`: يوازن بين الشذوذ والسلوك الطبيعي.
- `interaction_bonus`: يلتقط “تلاقي إشارات خطرة” (non-linear risk).
- `fp_discount`: يخفض الضوضاء من binaries موثوقة.
- context factors: يخصص المخاطر حسب المستخدم/الجهاز/الشبكة.
- quality factor: يمنع overconfidence عند نقص السياق.

</div>

---

<div dir="rtl">

### 6.7 شرح متغيرات `raw_score` بأمثلة رقمية

#### مثال 1: Alert بسيط

افتراض:

- rule severity = medium -> `base_score = 45`
- `lineage_bonus = 0`
- `privilege_bonus = 0`
- `burst_bonus = 0`
- `ueba_bonus = 0`
- `interaction_bonus = 0`
- `fp_discount = 8` (trusted signature)
- `ueba_discount = 0`

</div>

<div dir="ltr">

\[
raw\_score = 45 + 0 + 0 + 0 + 0 + 0 - 8 - 0 = 37
\]

</div>

<div dir="rtl">

#### مثال 2: Alert قوي

افتراض:

- rule severity = high -> `base_score = 65`
- `lineage_bonus = 30` (chain مشبوهة)
- `privilege_bonus = 25` (elevated/system/unsigned signals)
- `burst_bonus = 20`
- `ueba_bonus = 15` (anomaly)
- `interaction_bonus = 15`
- `fp_discount = 0`
- `ueba_discount = 0`

</div>

<div dir="ltr">

\[
raw\_score = 65 + 30 + 25 + 20 + 15 + 15 - 0 - 0 = 170
\]

</div>

---

<div dir="rtl">

### 6.8 `quality_factor`: كيف يُستخدم؟ وهل `context_quality_score` محسوب هنا؟

- في `risk_scorer.go`:
  - `context_quality_score` يتم **قراءته من event data** (`extractFloat64(...)`).
  - `missing_context_fields` أيضًا تُقرأ من event data.
- إذن: الـ scorer لا يبني `context_quality_score` من الصفر؛ يتعامل معه كدخل من upstream pipeline.

#### أمثلة:

1) سياق مكتمل:
- `context_quality_score = 85` -> `quality_factor = 1.00`

2) سياق متوسط:
- `context_quality_score = 55` -> `quality_factor = 0.93`

3) أسوأ حالة:
- `context_quality_score = 0` + missing fields موجودة -> `quality_factor = 0.85`

مثال تأثير:

- إذا `raw=80`, `multiplier=1.2`:
  - مع `quality=1.00` -> `adjusted=96`
  - مع `quality=0.85` -> `adjusted=82`

الفرق كبير ويقلل التصعيد غير الموثوق.

### 6.9 دور `trusted_networks`

- `trusted_networks` = قائمة CIDR تعتبر شبكات موثوقة.
- إذا `source_ip` داخل أي CIDR منها:
  - يتم ضرب `NetworkAnomalyFactor` في `0.9` (تخفيض بسيط للمخاطر).
- خارجها:
  - يتم ضربه في `1.1` (رفع بسيط للمخاطر).

**نعم**: إدخال `10.10.0.0/16` يعني هذه الشبكة trusted، وما عداها (إن لم يطابق CIDR آخر) يُعامل untrusted في هذا المنطق.

### 6.10 ماذا لو أنشأت Policy ثانية لنفس `agent A1` بقيم مختلفة؟

- الجدول عليه Unique Index:
  - `(scope_type, scope_value)` فريد.
- بالتالي لا يمكن وجود سجلين `agent + A1` معًا.
- السلوك الصحيح:
  - تحديث policy القديمة أو استبدالها، وليس إضافة ثانية بنفس المفتاح.
- إذا حاولت create بنفس المفتاح غالبًا سيحدث conflict من DB.

### 6.11 من أين جاءت قيم 0.9 و 1.1؟ وهل يمكن التحكم بها؟

- هذه القيم **hard-coded في الكود** داخل:
  - `context_policy_provider.go` (`Resolve(...)`)
- ليست مخزنة الآن في DB كإعدادات مستقلة.
- التحكم الحالي يكون بتعديل الكود، أو مستقبلاً يمكن نقلها لأعمدة/config قابلة للإدارة.

</div>
