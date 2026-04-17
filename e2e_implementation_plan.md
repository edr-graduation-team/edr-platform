# تحليل شامل: خطة الكشف النهائية (Detection + Context-Aware + AI)

## الحالة الحالية بعد Phase 1 + Phase 2

```
✅ تم إنجازه:
├── 8 مصادر telemetry كلها real-time (ETW kernel level)
├── Field mappings كاملة (Sigma ↔ Agent)
├── MITRE mapping صحيح (150+ technique)
├── Image Load hash أصبح async
└── Event category inference محدّث

⚠️ لم يتم اختباره بعد:
├── هل القواعد تطابق فعلاً؟ (E2E rule matching)
├── هل الإشعارات تظهر في Dashboard؟
├── هل الأحداث الجديدة تصل الـ Kafka؟
└── هل الحقول الجديدة تُقرأ بشكل صحيح؟

❌ غير موجود:
├── Context-Aware Detection (الأبعاد الخمسة)
├── AI Scoring
└── E2E Validation
```

---

## الأعمدة الثلاثة المطلوبة

### العمود 1: ضمان الكشف الصحيح والموثوق (E2E Reliability)

> [!IMPORTANT]
> **هذا هو الأهم**. بدون كشف صحيح، كل شيء آخر (AI, Context-Aware) مبني على أساس ضعيف.

#### المشكلة
الأحداث الجديدة (DNS, Pipe, ProcessAccess) والأحداث المُحسّنة (Network, Registry) لم تُختبر
بشكل E2E. يجب التأكد أن:

1. **الحدث يخرج من Agent** بالحقول الصحيحة
2. **يصل Kafka** بالصيغة المتوقعة
3. **Sigma Engine يقرأ الحقول** ويطابقها مع القواعد
4. **Alert يُولّد** ويُحفظ في PostgreSQL
5. **Dashboard يعرضه** في الوقت الحقيقي

#### الحل: E2E Validation Pipeline

لا يحتاج كود جديد — بل **اختبار منهجي** بعد النشر:

| الاختبار | الأمر | الحدث المتوقع | القاعدة |
|----------|-------|---------------|---------|
| DNS C2 | `nslookup evil.com` | `event_type: dns, query_name: evil.com` | `dns_query_*` |
| LSASS dump | `procdump -ma lsass.exe` | `event_type: process_access, target: lsass.exe` | `proc_access_*` |
| Suspicious pipe | `\\.\pipe\msagent_test` | `event_type: pipe, pipe_name: msagent_test` | `pipe_created_*` |
| Registry persist | `reg add HKCU\...\Run /v test` | `event_type: registry, action: value_set` | `registry_set_*` |
| Network C2 | `curl http://suspicious.com` | `event_type: network` | `net_connection_*` |
| Recon burst | `whoami && ipconfig && net user` | 3 process events | severity promotion |

#### لماذا هذا أولاً؟
بدون هذا التحقق، لا نعرف إذا كان النظام يعمل أصلاً. **تنفيذ AI على نظام كشف معطل = 0 قيمة**.

---

### العمود 2: Context-Aware Detection (القيمة الحقيقية الأكبر)

> [!IMPORTANT]
> **هذا هو ما يميز EDR عن Simple SIEM.** الـ Sigma rules وحدها تنظر لحدث واحد فقط.
> Context-Aware ينظر للصورة الكاملة: من فعل ماذا، من أين، متى، ولماذا هذا غريب.

#### الأبعاد الخمسة للسياق

```
                    ┌─────────────┐
                    │   Process   │
                    │   Context   │
                    └──────┬──────┘
                           │
           ┌───────────────┼───────────────┐
           │               │               │
    ┌──────┴──────┐ ┌──────┴──────┐ ┌──────┴──────┐
    │    User     │ │  Temporal   │ │   Device    │
    │   Context   │ │   Context   │ │   Context   │
    └─────────────┘ └─────────────┘ └─────────────┘
                           │
                    ┌──────┴──────┐
                    │   Network   │
                    │   Context   │
                    └─────────────┘
```

#### البُعد 1: Process Context (سلسلة العمليات)

**ماذا يفعل**: ينظر لسلسلة الأب-الابن (parent → child → grandchild).

**لماذا مهم**: `powershell.exe` وحدها ليست مشبوهة. لكن `winword.exe → cmd.exe → powershell.exe → certutil.exe` = **هجوم مؤكد** (T1566 → T1059).

**التنفيذ**: Go code في الـ Sigma Engine
```
عند وصول Alert:
  1. اقرأ PID + PPID من الحدث
  2. ابنِ شجرة الأنساب (3 مستويات max) من Redis cache
  3. قيّم: هل الأب عملية مكتبية/مستعرض؟ (office_spawned_shell = +30 risk)
  4. قيّم: هل السلسلة طويلة بشكل غير طبيعي؟ (depth > 4 = +10 risk)
```

**القيمة المضافة**: يكتشف هجمات **لا تكتشفها Sigma وحدها** — لأن كل حدث بمفرده عادي.

---

#### البُعد 2: User Context (سلوك المستخدم)

**ماذا يفعل**: هل هذا المستخدم يستخدم هذه الأداة عادة؟

**لماذا مهم**: `whoami` من مطور = عادي. `whoami` من حساب خدمة (SYSTEM) في الـ 3 صباحاً = **مشبوه**.

**التنفيذ**:
```
1. احتفظ بـ map في الذاكرة: user → set(processes seen)
2. عند Alert: هل هذه العملية first-seen لهذا المستخدم؟
3. first_seen = true → risk +15 + tag "first_seen_process"
4. المستخدم SYSTEM/Administrator يشغل أدوات recon → risk +25
```

**القيمة المضافة**: يميز بين **سلوك عادي ومشبوه** لنفس الأمر.

---

#### البُعد 3: Temporal Context (الزمن)

**ماذا يفعل**: هل هذا النشاط يحدث في وقت غير معتاد؟ هل هناك burst؟

**لماذا مهم**: 5 أوامر recon في 10 ثوان = **automated reconnaissance** (T1082+T1083+T1087).

**التنفيذ**:
```
Burst Detection:
  1. نافذة زمنية: آخر 60 ثانية لكل PID
  2. عدّ الأحداث المشبوهة في النافذة
  3. count >= 3 في 60s → severity promotion + tag "burst_activity"

Time-of-Day:
  4. النشاط بين 00:00-06:00 → risk +10 + tag "off_hours"
```

**القيمة المضافة**: يكشف **reconnaissance bursts** و **after-hours attacks**.

---

#### البُعد 4: Device Context (حالة الجهاز)

**ماذا يفعل**: ما هو مستوى خطورة هذا الجهاز؟

**لماذا مهم**: alert على جهاز عليه 10 alerts سابقة ≠ alert على جهاز نظيف.

**التنفيذ**:
```
1. عدّ alerts per agent في آخر 24 ساعة (من PostgreSQL)
2. agent_alert_count > 5 → risk +10 + tag "hot_endpoint"
3. agent_alert_count > 15 → risk +20 + tag "compromised_candidate"
```

**القيمة المضافة**: يساعد SOC analyst على **ترتيب الأولويات** — الجهاز الأكثر خطورة أولاً.

---

#### البُعد 5: Network Context (الشبكة)

**ماذا يفعل**: هل العملية تتصل بجهات خارجية مشبوهة؟

**لماذا مهم**: `powershell.exe` تتصل بـ IP خارجي = **C2 callback** محتمل.

**التنفيذ**:
```
1. عند Alert على process: هل له network events حديثة؟
2. ابحث في آخر 60 ثانية عن connections من نفس PID
3. external_connection = true → risk +15 + attach connection details
4. connection to non-standard port (не 80/443) → risk +10
```

**القيمة المضافة**: يربط **الأحداث المنفصلة** (process + network) في صورة واحدة.

---

#### أين يُنفّذ Context-Aware؟

```
الموقع: sigma_engine_go/internal/application/scoring/context_scorer.go [NEW]

يُستدعى من: risk_scorer.go بعد حساب الـ Sigma additive score

Input: Alert + Event data + Redis cache + PostgreSQL
Output: ContextScore {
    process_risk:  +30  (shell spawned from Office)
    user_risk:     +15  (first_seen_process)
    temporal_risk: +20  (burst: 5 events in 30s)
    device_risk:   +10  (hot_endpoint: 8 alerts today)
    network_risk:  +15  (external connection to non-standard port)
    total_adjustment: +90
    tags: ["office_spawned_shell", "first_seen", "burst_activity", ...]
    explanation: "PowerShell spawned from Word with external C2 callback"
}

Final Score = min(sigmaScore + contextAdjustment, 100)
```

> [!NOTE]
> **كل هذا Go code فقط** — بدون Python، بدون Docker إضافي، بدون ML training.
> يعمل من اليوم الأول بدون بيانات تدريب.

---

### العمود 3: AI Alert Scorer (قيمة مضافة حقيقية)

> [!IMPORTANT]
> الـ AI يعمل **فوق** الـ Context-Aware وليس بدلاً منه. يأخذ الـ Alert المُثرى (بعد Context scoring) ويضيف طبقة ذكاء إضافية.

#### ماذا يفعل الـ AI تحديداً؟

```
Alert (بعد Sigma + Context-Aware)
         ↓
    AI Alert Scorer
         ↓
    يجيب على 3 أسئلة:

    Q1: هل هذا الأمر (command line) يشبه أوامر هجومية معروفة؟
        → Feature: command entropy, base64 patterns, obfuscation, LOLBins
        → Model: XGBoost pre-trained على 50K+ malicious commands
        
    Q2: ما احتمال أن يكون False Positive؟
        → Feature: process reputation, signer, common vs rare
        → Model: FP classifier pre-trained على labeled alerts
        
    Q3: ما مستوى الثقة في التصنيف؟
        → Output: model probability (0.0 - 1.0)
```

#### لماذا XGBoost وليس Deep Learning؟

| المعيار | XGBoost | Deep Learning (BERT/Transformer) |
|---------|---------|----------------------------------|
| يحتاج training data؟ | ✅ نعم لكن 10K كافي (متوفر publicly) | يحتاج 1M+ |
| سرعة inference | ~0.5ms per alert | 15-50ms per alert |
| يحتاج GPU؟ | ❌ لا | عملياً نعم |
| Explainable? | ✅ feature importance | ❌ black box |
| حجم النموذج | ~5MB | 200MB-1GB |
| يعمل في Docker خفيف؟ | ✅ python:3.11-slim (50MB) | ❌ يحتاج pytorch/onnx (1GB+) |

#### مصادر الـ Pre-training Data (عامة ومتاحة)

| Dataset | الحجم | ماذا يحتوي |
|---------|-------|------------|
| EMBER | 1.1M samples | PE file features (malicious vs benign) |
| Mordor | 5K+ events | Real attack telemetry (labeled) |
| Atomic Red Team | 100+ techniques | Command lines per MITRE technique |
| CICIDS 2017 | 2.8M flows | Network intrusion (labeled) |
| SecRepo | 50K+ | Security log samples |

**النموذج يُدرّب مسبقاً** على هذه البيانات ويُشحن كملف `.joblib` (5-10MB).
**لا يحتاج تدريب** عند المستخدم.

#### الـ 20-100 سيناريو هجوم = Validation Set

```
Training:   Public datasets (50K+ samples) ← مُدرّب مسبقاً
Validation: سيناريوهاتك الـ 20-100 ← تثبت أنه يعمل
```

النتيجة: جدول مقارنة في التقرير:
```
| السيناريو      | Sigma Score | + Context | + AI  | Final | TP/FP |
|----------------|------------|-----------|-------|-------|-------|
| Mimikatz LSASS | 65         | +30       | +25   | 100   | TP ✅ |
| Normal whoami  | 40         | +0        | -20   | 20    | TN ✅ |
| DNS tunneling  | 55         | +15       | +20   | 90    | TP ✅ |
| Legit PS script| 45         | +5        | -15   | 35    | TN ✅ |
```

---

## خطة التنفيذ المقترحة (مرتبة بالأولوية)

### المرحلة A: Context-Aware Scoring (2-3 أيام)

> **الأعلى أولوية** — يضيف أكبر قيمة كشفية بأقل تعقيد

```
[NEW] context_scorer.go       — الأبعاد الخمسة
[MOD] risk_scorer.go          — دمج context score
[MOD] context_snapshot.go     — حقول جديدة للسياق
[NEW] process_lineage.go      — شجرة الأنساب (Redis cache)
[NEW] baseline_tracker.go     — UEBA first-seen tracking
```

### المرحلة B: AI Alert Scorer (2-3 أيام)

> **ثاني أولوية** — يضيف طبقة AI حقيقية ومحدودة

```
[NEW] ai-scorer/              — Python microservice كامل
[NEW] transformer_client.go   — gRPC client في Sigma Engine  
[MOD] docker-compose.yml      — إضافة الخدمة
[MOD] risk_scorer.go          — hybrid scoring
```

### المرحلة C: Dashboard + Validation (1-2 أيام)

> **أخيراً** — عرض النتائج + إثبات أن كل شيء يعمل

```
[MOD] Alerts.tsx              — عرض AI score + context dimensions
[NEW] AttentionRadar.tsx      — رسم بياني للأبعاد الخمسة
[RUN] 20-100 attack scenarios — validation matrix
```

---

## ملخص القيمة المضافة لكل طبقة

```
الطبقة 1: Sigma Rules (موجودة)
  → تكتشف: أنماط معروفة (known patterns)
  → لا تكتشف: سياق، سلوك غريب، zero-day

الطبقة 2: Context-Aware (المرحلة A)
  → تكتشف: سلاسل هجوم، سلوك غير معتاد، burst activity
  → تقلل: false positives بنسبة 30-50% (تقدير)
  → الأثر: الأكبر من بين الثلاثة

الطبقة 3: AI Scorer (المرحلة B)  
  → تكتشف: command obfuscation، أوامر مشبوهة بدون rule
  → تقلل: false positives بنسبة 10-20% إضافية
  → الأثر: إضافي + يضيف "AI" كعنوان للمشروع
```

---

## الحدود الواضحة (ما لن نفعله)

- ❌ **لن نبني Transformer/BERT** — غير مناسب لحجم البيئة
- ❌ **لن ندرب نموذج من الصفر** — بيانات غير كافية  
- ❌ **لن نعالج كل event بالـ AI** — فقط الـ Alerts
- ❌ **لن نستخدم GPU** — XGBoost يعمل على CPU بـ < 1ms
- ❌ **لن نضيف خدمات معقدة** — Flask خفيف فقط
- ✅ **سنستخدم نموذج pre-trained** — مُدرّب على datasets عامة
- ✅ **سنثبت القيمة** بجدول مقارنة كمي (20-100 سيناريو)

---

## Open Questions

> [!IMPORTANT]
> 1. هل تريد تنفيذ المرحلة A (Context-Aware) أولاً ثم B (AI) أم الاثنتين معاً؟
> 2. هل Dashboard الحالي يعرض context_snapshot بالتفصيل؟ (أحتاج التأكد قبل إضافة حقول جديدة)
> 3. كم المدة المتبقية لتسليم المشروع؟ (لترتيب الأولويات)
