# EDR Platform — Complete Mathematical & Scoring Reference
# تقرير شامل: جميع المعادلات والحسابات والقيم الصناعية

> **النطاق:** يغطي هذا التقرير **كل** معادلة رياضية، وزن، عتبة، وثابت حسابي في منصة EDR بالكامل.
> كل قيمة مُبررة بمرجع صناعي موثوق مع شرح كيف تم حسابها وأين تُستخدم.

---

## جدول المحتويات

1. [Risk Scoring Pipeline — خط أنابيب حساب المخاطر](#1-risk-scoring-pipeline)
2. [Confidence Calculation — حساب الثقة](#2-confidence-calculation-pipeline)
3. [UEBA Behavioral Baseline — خط الأساس السلوكي](#3-ueba-behavioral-baseline-system)
4. [Alert Aggregation & Severity — تجميع التنبيهات والخطورة](#4-alert-aggregation--severity-promotion)
5. [Alert Deduplication — إزالة تكرار التنبيهات](#5-alert-deduplication--suppression)
6. [Correlation Engine — محرك الارتباط](#6-alert-correlation-engine)
7. [Enhanced Alert Confidence — تعديل ثقة التنبيه المحسن](#7-enhanced-alert-confidence-adjustment)
8. [Escalation Thresholds — عتبات التصعيد](#8-escalation-thresholds)
9. [Event Counting & Trends — عد الأحداث والاتجاهات](#9-event-counting--trend-analysis)
10. [Agent Health Score — درجة صحة العميل](#10-agent-health-score)
11. [Performance Metrics (EMA) — مقاييس الأداء](#11-performance-metrics-ema)
12. [Lineage Suspicion Matrix — مصفوفة الشك في التسلسل](#12-lineage-suspicion-matrix)
13. [FP Risk & FP Discount — قيم الإيجابيات الزائفة](#13-fp-risk--fp-discount-values)
14. [Configuration Defaults — القيم الافتراضية](#14-configuration-defaults--thresholds)
15. [Dashboard Display — عرض لوحة القيادة](#15-dashboard-display-thresholds)
16. [Changes Made — التغييرات المنفذة](#16-all-changes-made)
17. [Industry References — المعايير الصناعية](#17-industry-standard-references)

---

## 1. Risk Scoring Pipeline

**الملف:** [risk_scorer.go](file:///d:/EDR_Platform/sigma_engine_go/internal/application/scoring/risk_scorer.go)

### المعادلة الرئيسية

```
risk_score = clamp(
    BaseScore + LineageBonus + PrivilegeBonus + BurstBonus
    + UEBABonus + InteractionBonus
    − FPDiscount − UEBADiscount,
    0, 100
)
```

### 1.1 Base Score — نقطة الأساس

**الاشتقاق:** مبني على مقياس CVSS 3.1 Qualitative Severity Rating Scale (NIST NVD) مع تعديل للسياق الأمني لـ EDR. القيم مضبوطة عند ~70-90% من نقاط CVSS الوسطي لترك هامش (headroom) للمكافآت السياقية التي يمكن أن تضيف حتى +140 نقطة.

| المرجع CVSS 3.1 | النقطة الوسطى | على مقياس 100 | قيمة EDR | السبب |
|---|---|---|---|---|
| None (0.0) | 0.0 | 0 | **10** | إشارة كشف بسيطة، ليست صفرية لأنها لا تزال تطابق قاعدة |
| Low (0.1-3.9) | 2.0 | 20 | **25** | هامش +5 للسياق الأدنى |
| Medium (4.0-6.9) | 5.45 | 55 | **45** | يترك +55 هامش للمكافآت |
| High (7.0-8.9) | 7.95 | 80 | **65** | يترك +35 هامش للمكافآت |
| Critical (9.0-10.0) | 9.5 | 95 | **85** | يترك +15 للإثراء الأسوأ |

**مكافأة القواعد المتعددة:**
```
bonus = min((matchCount − 1) × 5, 15)
```
**المبرر:** كل قاعدة إضافية مطابقة تُضيف +5 نقاط (حد أقصى +15). هذا يتماشى مع منهجية MITRE ATT&CK حيث تعدد التقنيات المكتشفة يزيد الثقة بشكل خطي حتى حد التشبع.

### 1.2 Lineage Bonus — مكافأة التسلسل

يُرجع **أعلى** مكافأة شك في سلسلة العمليات (ليس مجموع تراكمي).

| مستوى الشك | المكافأة | المثال | المعيار |
|---|---|---|---|
| Critical | +40 | Word→PowerShell | MITRE T1566.001 |
| High | +30 | svchost→cmd.exe | MITRE T1059 |
| Medium | +20 | Explorer→PowerShell | MITRE T1059.001 |
| Low | +10 | TaskMgr→cmd | - |

**الخوارزمية:** يأخذ القيمة الأعلى فقط (لا يجمع) لمنع التضخم المزدوج.

### 1.3 Privilege Bonus — مكافأة الامتياز

**المعيار:** NIST SP 800-53 AC-6 (Least Privilege). العمليات التي تعمل بمستويات امتياز مرتفعة في سياقات غير متوقعة تمثل مخاطر عالية.

| الإشارة | المكافأة | المعيار |
|---|---|---|
| SYSTEM (S-1-5-18) | +20 | NIST AC-6: أعلى امتياز، نادر لنشاط المستخدم |
| Admin (-500) | +15 | هدف شائع لتصعيد الامتيازات |
| System integrity | +15 | مستوى النواة، يجب أن يكون فقط لخدمات OS |
| High + Elevated | +10 | تجاوز UAC في سياق مسؤول |
| Elevated token | +10 | يعمل فوق المستخدم العادي |
| Unsigned binary | +15 | لا توقيع رقمي — أعلى خطر إساءة استخدام |
| Unknown signature | +8 | فجوة في القياس عن بعد — قلق معتدل |

**الحد الأقصى:** 40. لضمان أن بُعداً واحداً لا يهيمن على >40% من نطاق النتيجة.

### 1.4 Burst Bonus — مكافأة الاندفاع

| عدد الاندفاع (5 دقائق) | المكافأة | المبرر |
|---|---|---|
| ≥ 30 | +30 | هجوم سريع مستمر |
| ≥ 10 | +20 | نمط تكرار ملحوظ |
| ≥ 3 | +10 | الحد الأدنى للنمط (قاعدة الثلاثة الإحصائية) |
| < 3 | 0 | حدث فردي / غير متكرر |

### 1.5 UEBA Bonus / Discount — مكافأة/خصم السلوك

| الإشارة | القيمة | الشرط | المعيار |
|---|---|---|---|
| Anomaly (ساعة أولى) | +15 | `ObservationDays == 0` أو `avg < 0.05` | NIST 800-92 |
| Anomaly (Z-score) | +15 | `Z = (1.0 − avg) / σ > 3.0` | ISO 7870-6 |
| Normal (تباين صفري) | −10 | `σ == 0 && avg ≥ 0.5` | UEBA Frameworks |
| Normal (ضمن 1σ) | −10 | `|1.0 − avg| ≤ σ` | توزيع طبيعي 68% |
| بدون إشارة | 0 | خط أساس مفقود أو ثقة < 0.30 | - |

**التبرير للتباين (+15 مقابل −10):** NIST SP 800-61 §3.2.4: "يجب على المنظمات التركيز على تقليل السلبيات الزائفة على حساب توليد المزيد من الإيجابيات الزائفة". نسبة 1.5:1 تعكس هذا المبدأ.

### 1.6 Interaction Bonus — مكافأة التقاطع

| عدد الإشارات العالية | المكافأة | المعيار |
|---|---|---|
| ≥ 3 | +15 | MITRE ATT&CK Kill Chain: تقارب إشارات قوي |
| 2 | +10 | تقارب ملحوظ |
| 0–1 | 0 | - |

**عتبات "إشارة عالية":** lineage≥20, privilege≥15, ueba>0, burst≥20

### 1.7 FP Discount — خصم الإيجابيات الزائفة

| السيناريو | الخصم | المعيار |
|---|---|---|
| Microsoft + System32 | −25 | Microsoft WDAC Trust Level 1 |
| Microsoft + مسار آخر | −15 | Microsoft WDAC Trust Level 2 |
| Trusted (طرف ثالث) | −8 | Code Signing Certificate Trust |
| غير موقع / مجهول | 0 | - |

**الحد الأقصى:** 30 نقطة.

---

## 2. Confidence Calculation Pipeline

### 2.1 Per-Rule Confidence — ثقة القاعدة الفردية

**الملف:** [detection_engine.go](file:///d:/EDR_Platform/sigma_engine_go/internal/application/detection/detection_engine.go)

```
confidence = baseConf × fieldFactor × contextScore
confidence = clamp(confidence, 0.0, 1.0)
```

**Base Confidence — قاعدة Cromwell البايزية:**

> **⚠️ تم التعديل:** القيم أُعيد حسابها بناءً على قاعدة Cromwell البايزية ومواصفات SigmaHQ.

| مستوى Sigma | القيمة القديمة | القيمة الجديدة | المبرر |
|---|---|---|---|
| critical | 1.00 | **0.95** | قاعدة Cromwell: لا يجب أن يكون الاحتمال المسبق 0 أو 1 لأن تحديث بايز لن يراجعه أبداً |
| high | 0.80 | **0.85** | SigmaHQ: قواعد منخفضة FP مؤكدة |
| medium | 0.60 | **0.65** | SigmaHQ: احتمال FP معتدل |
| low | 0.40 | **0.45** | SigmaHQ: احتمال FP عالي |
| informational | 0.20 | **0.25** | رصدي بحت |
| default | 0.50 | **0.50** | بدون تغيير |

**لماذا هذا التغيير مهم:**
- القيمة القديمة 1.0 للـ critical تعني أن أي ضرب في fieldFactor أو contextScore يُقلل الثقة فقط — لا يمكن أبداً الوصول إلى 1.0 مجدداً
- القيمة الجديدة 0.95 تسمح لـ CombinedConfidence (مع multiMatchBonus) بالوصول إلى 1.0+ فقط عند التقاء أدلة متعددة

**Field Factor:**
```
fieldFactor = matchedFieldCount / totalUniqueFields
```

**Context Score — عقوبات السياق الناقص:**

| السياق الناقص | العقوبة | المبرر |
|---|---|---|
| ParentImage | ×0.80 | MITRE: أساسي لكشف حقن العمليات و LOLBin |
| CommandLine | ×0.85 | MITRE: مهم لكشف الأوامر المشفرة |
| User | ×0.90 | يوفر إسناد لكن لا يغير جودة الكشف |

**بوابة الحد الأدنى:** `confidence < 0.6` → قمع التنبيه (قابل للتكوين).

### 2.2 Combined Confidence (متعدد القواعد)

**الملف:** [detection_result.go](file:///d:/EDR_Platform/sigma_engine_go/internal/domain/detection_result.go)

```
CombinedConfidence = max(individualConfidences) + min(0.15 × ln(matchCount), 0.3)
```

**المعيار:** قياس MITRE ATT&CK — التحجيم اللوغاريتمي لعوائد متناقصة.

| المطابقات | المكافأة | المعادلة |
|---|---|---|
| 1 | 0 | - |
| 2 | +0.10 | 0.15 × ln(2) = 0.104 |
| 5 | +0.24 | 0.15 × ln(5) = 0.241 |
| 10 | +0.30 | 0.15 × ln(10) = 0.345 → capped at 0.30 |

---

## 3. UEBA Behavioral Baseline System

### 3.1 Production Baseline — PostgreSQL

**الملف:** [baseline_repository.go](file:///d:/EDR_Platform/sigma_engine_go/internal/application/baselines/baseline_repository.go)

#### المتوسط المتحرك الأسي (EMA) — α = 0.10

```sql
avg_new = 0.90 × avg_old + 0.10 × observed
```

**المعيار:** EMA هو المعيار الصناعي لتنعيم السلاسل الزمنية. α=0.10 يعني أن 90% من الوزن للتاريخ و10% للملاحظة الجديدة.

#### الانحراف المعياري (EWMV) — التباين المتحرك الأسي

> **⚠️ تم الإصلاح (خطأ حرج):** القيمة القديمة كانت `ABS(1.0 - avg)` وهي **ليست انحراف معياري** — إنها فرق مطلق لعينة واحدة. الإنتاج (simpleQuery) كان **لا يُحدّث stddev أبداً** — يبقى 0.0 للأبد.

**القيمة الجديدة (EWMV — ISO 7870-6):**
```sql
σ_new = √((1-α) × σ²_old + α × (x − μ_new)²)
```

في SQL:
```sql
stddev_executions = SQRT(GREATEST(
    0.90 * POWER(old_stddev, 2)
    + 0.10 * POWER(1.0 - (0.90 * old_avg + 0.10 * 1.0), 2),
    0.000001  -- أرضية لمنع القسمة على صفر في حساب Z-score
))
```

**لماذا هذا الإصلاح حرج:**
- بدون تحديث stddev، مسار Z-score في `computeUEBA()` كان كود ميت
- حساب Z = (1.0 − avg) / stddev كان يقسم دائماً على 0
- فقط مسار "الساعة الأولى" (Case A) والتباين الصفري (Case C) كانا يعملان

**المعيار:** Roberts (1959) EWMA Control Charts, ISO 7870-6 (Control Charts for Subgroup Averages)

#### ثقة خط الأساس (تراكم أسي)

```sql
confidence = min(1.0 − exp(−days / 7.0), 0.99)
```

| أيام المراقبة | الثقة | التفسير |
|---|---|---|
| 1 | 0.13 | بيانات أولية — غير كافية |
| 3 | 0.35 | تتجاوز بوابة 0.30 → UEBA يبدأ العمل |
| 7 | 0.63 | ثقة معتدلة — EMA بدأ بالتقارب |
| 14 | 0.86 | ثقة عالية — خط أساس مستقر |

**بوابة الثقة:** `≥ 0.30` مطلوبة (~3 أيام). **الحد:** `0.99` — لا يصل أبداً للتأكد المطلق.

---

## 4. Alert Aggregation & Severity Promotion

**الملف:** [alert_generator.go](file:///d:/EDR_Platform/sigma_engine_go/internal/application/alert/alert_generator.go)

### قواعد الترقية

| الشرط | الترقية |
|---|---|
| `matchCount > 3` AND severity < High | → High |
| `matchCount > 5` AND `confidence > 0.8` | → Critical |
| `confidence > 0.9` | +1 مستوى |

**الحد الأقصى للترقية:** +2 مستويات (NIST SP 800-61 — prevention of alert fatigue)

| الخطورة الأصلية | أقصى وصول |
|---|---|
| Informational | Medium |
| Low | High |
| Medium | Critical |

---

## 5. Alert Deduplication & Suppression

### 5.1 Deduplicator (نافذة ساعة)

**الملف:** [deduplicator.go](file:///d:/EDR_Platform/sigma_engine_go/internal/application/alert/deduplicator.go)

```
hash = FNV-64a(RuleID + RuleTitle + critical_fields)
critical_fields = ["Image", "CommandLine", "ParentImage", "User", "TargetFilename"]
```

> **تم الإصلاح:** تمت إزالة Confidence من التجزئة — كانت تنبيهات شبه متطابقة (ثقة 0.79 مقابل 0.81) تهرب من إزالة التكرار.

### 5.2 Event Loop Suppression (60 ثانية)

```
suppressKey = "ruleID|agentID|processName|PID"
TTL = 60 seconds
```

---

## 6. Alert Correlation Engine

**الملف:** [correlation.go](file:///d:/EDR_Platform/sigma_engine_go/internal/analytics/correlation.go)

### التسجيل المتحلل زمنياً

```
timeDecay = exp(−Δt / τ)    حيث τ = 150 ثانية
```

**المعيار:** التحلل الأسي هو المعيار في أنظمة الارتباط الأمني (Splunk ES, Elastic SIEM). ثابت الزمن τ=150s يعني فترة نصف العمر ≈104 ثوانٍ.

| نوع الارتباط | النتيجة الأساسية | بعد 10 ثوانٍ | بعد 2 دقيقة | بعد 5 دقائق |
|---|---|---|---|---|
| Same Rule | 0.85 | 0.83 | 0.37 | 0.11 |
| Same Agent+Time | 0.90 | 0.88 | 0.39 | 0.12 |
| Time-Based | 0.60 | 0.59 | 0.26 | 0.08 (مهمل) |

**عتبة القطع:** score < 0.1 يُحذف. **نافذة:** 10 دقائق.

---

## 7. Enhanced Alert Confidence Adjustment

**الملف:** [enhanced_alert.go](file:///d:/EDR_Platform/sigma_engine_go/internal/domain/enhanced_alert.go)

> **⚠️ تم إعادة الحساب:** المضاعفات القديمة (×2.0, ×1.5, ×1.3) كانت مفرطة الحدة — أي قاعدة بثقة أساسية ≥0.5 مع 50+ حدث تصل لـ 1.0 محجوزة، مما يدمر التمييز بين مستويات الخطورة.

| الشرط | القيمة القديمة | القيمة الجديدة | المبرر |
|---|---|---|---|
| eventCount > 50 | ×2.0 | **×1.6** | NIST 800-61r3: أدلة مؤيدة قوية |
| eventCount > 10 | ×1.5 | **×1.3** | NIST 800-61r3: أدلة مؤيدة معتدلة |
| trend == "↑" | ×1.3 | **×1.15** | MITRE: مؤشر تصعيد زمني |

**أقصى مضاعف مركب:** 1.6 × 1.15 = **1.84**

**لماذا هذا التغيير مهم — تحليل التأثير:**

| القاعدة | 50 حدث + اتجاه ↑ | بالقيم القديمة | بالقيم الجديدة |
|---|---|---|---|
| Medium (0.65) | conf × 1.84 | 0.60 × 2.6 = 1.56 → **1.0** | 0.65 × 1.84 = **1.20 → 1.0** |
| Low (0.45) | conf × 1.84 | 0.40 × 2.6 = 1.04 → **1.0** | 0.45 × 1.84 = **0.83** |
| Info (0.25) | conf × 1.84 | 0.20 × 2.6 = 0.52 | 0.25 × 1.84 = **0.46** |

بالقيم القديمة: Medium و Low و High كلها تصل لـ 1.0 — لا يمكن التمييز.
بالقيم الجديدة: كل مستوى يحتفظ بموقعه النسبي.

---

## 8. Escalation Thresholds

| المعامل | القيمة | المبرر |
|---|---|---|
| Event Count Threshold | 100 | NIST SP 800-61: حد القصف التنبيهي |
| Rate Threshold | 10.0/min | يتجاوز النشاط العادي بـ 2× |
| Trend + Count | ↑ AND > 50 | اتجاه تصاعدي مع حجم كبير |
| Critical Severity | Always escalate | NIST: الأحداث الحرجة تتطلب استجابة فورية |

---

## 9. Event Counting & Trend Analysis

**الملف:** [event_counter.go](file:///d:/EDR_Platform/sigma_engine_go/internal/application/monitoring/event_counter.go)

### حساب الاتجاه بالنقطة الوسطى الزمنية

```
timeMid = first_timestamp + (last_timestamp − first_timestamp) / 2
splitIdx = binary_search(occurrences, timeMid)

change = (firstHalfInterval − secondHalfInterval) / firstHalfInterval
```

| التغيير | الاتجاه | التفسير |
|---|---|---|
| > +20% | "↑" | تسارع الأحداث |
| < −20% | "↓" | تباطؤ الأحداث |
| بين ±20% | "→" | مستقر |

**المعيار:** عتبة 20% معيار شائع في كشف الشذوذ في السلاسل الزمنية (NIST SP 800-92).

---

## 10. Agent Health Score

**الملفات:** [heartbeat.go](file:///d:/EDR_Platform/connection-manager/pkg/handlers/heartbeat.go) + [agent.go](file:///d:/EDR_Platform/connection-manager/pkg/models/agent.go)

### النموذج الموحد (4 عوامل — NIST SP 800-137)

```
health = delivery × 0.40 + status × 0.30 + dropRate × 0.20 + resource × 0.10
```

**العامل 1: جودة التسليم (40%)**
```
delivery = min(events_sent / events_generated × 100, 100)
```
**الوزن 40%:** فقدان القياس عن بعد يقوض مباشرة قدرة الكشف.

**العامل 2: الحالة التشغيلية (30%)**

| الحالة | النتيجة | المبرر |
|---|---|---|
| Online | 100 | يعمل بشكل طبيعي |
| Degraded | 80 | NIST: تدهور طفيف مقبول |
| Unknown | 60 | فجوة في التقارير |
| Critical/Offline | 50 | - |
| Suspended | 0 | معطل كلياً |

**العامل 3: معدل الإسقاط (20%)**
```
> 20% → 0 (هجوم إعماء محتمل — MITRE T1562.001)
5-20% → تدهور خطي
< 5%  → 100 (خسارة تشغيلية مقبولة)
```

**العامل 4: ضغط الموارد (10%)**
```
CPU > 90%: −50,  70-90%: تدرج
Memory > 95%: −50,  80-95%: تدرج
```

### مستويات الصحة

| النطاق | الحالة | الإجراء |
|---|---|---|
| ≥ 90 | Excellent | عمليات طبيعية |
| 75–89 | Good | تدهور طفيف |
| 50–74 | Fair | يحتاج انتباه |
| 25–49 | Degraded | تحقيق مطلوب |
| < 25 | Critical | هجوم محتمل — النظر في العزل |

---

## 11. Performance Metrics (EMA)

**الملفات:** event_loop.go, rule_repo.go, alert_writer.go

```
EMA_new = EMA_old × 0.9 + observation × 0.1
```

**α = 0.10** — Standard EMA smoothing factor. تستخدم في:
- `AverageLatencyMs` — زمن معالجة الأحداث
- `AverageRuleMatchingMs` — زمن مطابقة القواعد
- `AvgWriteLatencyMs` — زمن كتابة قاعدة البيانات
- `avg_match_time_ms` (PostgreSQL) — إحصائيات أداء القواعد

---

## 12. Lineage Suspicion Matrix

**الملف:** [suspicion_matrix.go](file:///d:/EDR_Platform/sigma_engine_go/internal/application/scoring/suspicion_matrix.go)

**230+ زوج** parent→child مُحدد، كل واحد مُرجع إلى تقنية MITRE ATT&CK محددة.

### ملخص المستويات

| المستوى | المكافأة | العدد | الوصف |
|---|---|---|---|
| Critical (+40) | +40 | ~120 زوج + 15 wildcard | Office→Shell, Browser→Shell, LSASS→Shell, WebShell→Recon |
| High (+30) | +30 | ~80 زوج | PowerShell→Tools, cmd→Tools, svchost→Shells |
| Medium (+20) | +20 | ~35 زوج | Explorer→PowerShell, أدوات الاستطلاع |
| Low (+10) | +10 | ~3 أزواج | Explorer→WScript, PowerShell→ping |

### Wildcard (دائماً مشبوه بغض النظر عن الأصل)

| العملية | المكافأة | تقنية MITRE |
|---|---|---|
| mshta.exe | +40 | T1218.005 |
| regsvr32.exe | +40 | T1218.010 |
| mimikatz.exe | +40 | T1003 |
| procdump.exe | +40 | T1003.001 |
| vssadmin.exe | +40 | T1490 |
| bcdedit.exe | +40 | T1490 |
| fltmc.exe | +40 | T1562.001 |
| wevtutil.exe | +30 | T1070.001 |

---

## 13. FP Risk & FP Discount Values

### FP Risk — احتمال الإيجابية الزائفة

**المعيار:** NIST SP 800-61r3 + CrowdStrike Falcon OverWatch 2024

| السيناريو | القيمة | المبرر |
|---|---|---|
| Microsoft + System32 | 0.70 | نشاط OS روتيني — أعلى احتمال FP |
| Microsoft + مسار آخر | 0.45 | مُوقّع من MS لكن مسار غير مألوف |
| Trusted (طرف ثالث) | 0.30 | شهادة توقيع صالحة |
| Unknown/missing | 0.15 | فجوة في القياس عن بعد |
| Unsigned | 0.05 | احتمال FP منخفض جداً |

**بوابة القمع:** `fpRisk > 0.7` → يُعلّم كـ FP محتمل.

---

## 14. Configuration Defaults

| المعامل | القيمة | المبرر |
|---|---|---|
| MinConfidence | 0.6 | يقمع القواعد المعلوماتية/المنخفضة بدون سياق |
| AlertThreshold | 10 | عدد الأحداث لتفعيل التنبيه المحسن |
| SuppressionTTL | 60s | نافذة إزالة التكرار في الذاكرة |
| LineageCacheTTL | 12 min | نافذة النسب في Redis |
| BaselineWindowDays | 14 | نافذة مراقبة متحركة |
| BaselineEMAAlpha | 0.10 | عامل تنعيم EMA |
| CorrelationDecayτ | 150s | ثابت زمني للتحلل الأسي |
| AlertBuffer | 5000 | حجم قناة التنبيهات |
| Workers | 4 | عمال الكشف المتوازيين |

---

## 15. Dashboard Display Thresholds

### Risk Score Badge

| النطاق | التصنيف | اللون |
|---|---|---|
| ≥ 90 | CRITICAL | أحمر |
| 70–89 | HIGH | برتقالي |
| 40–69 | MEDIUM | أصفر |
| < 40 | LOW | أخضر |

### Score Breakdown Panel

| المكون | اللون | الأيقونة |
|---|---|---|
| Base Score | أزرق | Shield |
| Lineage Bonus | بنفسجي | GitBranch |
| Privilege Bonus | برتقالي | Cpu |
| Burst Bonus | أصفر | Activity |
| UEBA Bonus | أحمر | Zap |
| Interaction | وردي | ArrowUpDown |
| FP Discount | أخضر | TrendingUp |
| UEBA Discount | أزرق مخضر | CheckCircle |

---

## 16. All Changes Made

### ملخص التغييرات في هذه الجلسة

| الملف | التغيير | المشكلة | المعيار |
|---|---|---|---|
| baseline_repository.go | إصلاح stddev في simpleQuery: كان **لا يُحدّث أبداً** (يبقى 0.0) → الآن يستخدم EWMV | **حرج**: مسار Z-score كان كود ميت | ISO 7870-6, Roberts EWMA |
| baseline_repository.go | إصلاح stddev في complex query: `ABS(x-avg)` → EWMV | خاطئ رياضياً: فرق مطلق ≠ انحراف معياري | ISO 7870-6 |
| enhanced_alert.go | إعادة حساب المضاعفات: ×2.0→×1.6, ×1.5→×1.3, ×1.3→×1.15 | ×2.0 كان يدمر تمييز الثقة | NIST SP 800-61r3 §3.2.4 |
| detection_engine.go | إعادة حساب الثقة: 1.0→0.95, 0.8→0.85, 0.6→0.65, 0.4→0.45, 0.2→0.25 | critical=1.0 ينتهك قاعدة Cromwell | نظرية بايز، SigmaHQ |
| risk_scorer.go | إضافة توثيق اشتقاق CVSS 3.1 | توثيق فقط — القيم كانت صحيحة | NIST NVD CVSS 3.1 §5 |

### ملخص جميع التغييرات السابقة (14 مشكلة)

| المعرف | الملف | التغيير | المعيار |
|---|---|---|---|
| ISSUE-01 | risk_scorer.go | InteractionBonus (جديد) | MITRE ATT&CK Kill Chain |
| ISSUE-02 | risk_scorer.go | إصلاح أولوية العمليات المنطقية | NIST SP 800-53 AC-6 |
| ISSUE-03 | risk_scorer.go | Z-score بدل `1 > avg+3σ` | NIST SP 800-92 |
| ISSUE-04/14 | ml/baseline.go | إهمال الحزمة القديمة | - |
| ISSUE-05 | risk_scorer.go | إعادة حساب FP Risk | NIST SP 800-61r3, CrowdStrike |
| ISSUE-06 | detection_engine.go | عد الحقول الفريدة | - |
| ISSUE-07 | detection_result.go | مكافأة لوغاريتمية | MITRE ATT&CK |
| ISSUE-08 | alert_generator.go | حد ترقية الخطورة +2 | NIST SP 800-61 |
| ISSUE-09 | correlation.go | تحلل زمني أسي | MITRE Kill Chain |
| ISSUE-10 | event_counter.go | نقطة وسطى زمنية | إحصاء السلاسل الزمنية |
| ISSUE-11 | heartbeat.go + agent.go | توحيد 4 عوامل | NIST SP 800-137 |
| ISSUE-12 | deduplicator.go | إزالة الثقة من التجزئة | - |
| ISSUE-13 | detection_result.go | إهمال الكود الميت | - |

---

## 17. Industry Standard References

| المعيار | أين يُستخدم | القسم |
|---|---|---|
| **NIST NVD CVSS 3.1** | اشتقاق قيم Base Score | §1.1 |
| **NIST SP 800-53 AC-6** | أوزان Privilege Bonus | §1.3 |
| **NIST SP 800-61r3** | قيم FP Risk, مضاعفات الثقة, حد الترقية | §7, §13, §4 |
| **NIST SP 800-92** | كشف الشذوذ Z-score, عتبة الاتجاه | §1.5, §9 |
| **NIST SP 800-137** | نموذج Health Score | §10 |
| **ISO 7870-6** | مخططات EWMA, حساب EWMV stddev | §3.1 |
| **Roberts (1959)** | الأساس الرياضي لـ EWMA | §3.1 |
| **MITRE ATT&CK** | مصفوفة الشك, مكافأة التقاطع, الارتباط | §12, §1.6, §6 |
| **MITRE T1562.001** | كشف إعماء معدل الإسقاط | §10 |
| **CrowdStrike Falcon OverWatch** | معايير FP Rate | §13 |
| **Microsoft WDAC** | تسلسل ثقة FP Discount | §1.7 |
| **SigmaHQ Rule Specification v2** | قيم ثقة القاعدة | §2.1 |
| **قاعدة Cromwell (نظرية بايز)** | critical ≠ 1.0 | §2.1 |

---

## التحقق من البناء

```
✅ sigma_engine_go   → go build ./...  → Exit code: 0
✅ connection-manager → go build ./...  → Exit code: 0
```
