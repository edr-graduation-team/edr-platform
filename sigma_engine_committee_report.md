# Sigma Detection Engine — تقرير تقني شامل للجنة المناقشة

<div style="text-align: center; margin: 40px 0;">

**مشروع منصة الكشف والاستجابة لنقاط النهاية (EDR)**

**مكون: محرك الكشف المبني على قواعد Sigma**

</div>

---

## 1. الملخص التنفيذي

يُعد محرك Sigma Detection Engine المكوّن المركزي في منصة EDR، وهو المسؤول عن تحليل أحداث الأمان الواردة من أجهزة المستخدمين في الوقت الحقيقي ومطابقتها مع قواعد الكشف وفق معيار Sigma المفتوح. يعالج المحرك آلاف الأحداث في الثانية عبر بنية متوازية متعددة العمال، ويولّد تنبيهات أمنية مُثراة بمعلومات MITRE ATT&CK وتقييم السياق.

**المدخلات:** أحداث أمان خام من Kafka (مصدرها EDR Agent)

**المخرجات:** تنبيهات أمنية مُثراة إلى Kafka + PostgreSQL → Dashboard

**الإحصائيات التشغيلية:**
- معالجة 3,000-5,000 حدث/ثانية
- زمن استجابة متوسط < 1 مللي ثانية لكل حدث
- دعم 2,000+ قاعدة Sigma
- كشف 34 فئة مختلفة من أحداث الأمان

---

## 2. الهندسة المعمارية

### 2.1 البنية الطبقية (Layered Architecture)

المحرك مبني وفق نمط Hexagonal Architecture (Ports & Adapters) الذي يفصل المنطق الأساسي عن البنية التحتية، مما يسمح باستبدال أي مكون خارجي (Kafka، PostgreSQL، Redis) دون تعديل منطق الكشف.

```mermaid
block-beta
    columns 3
    
    block:entry:3
        columns 3
        main["cmd/sigma-engine-kafka\nmain.go\n(Entry Point)"]
    end
    
    space:3
    
    block:app:3
        columns 5
        det["detection/\nDetectionEngine\nSelectionEvaluator\nModifierRegistry"]
        map["mapping/\nFieldMapper\n40+ field mappings"]
        rul["rules/\nRuleLoader\nRuleParser\nRuleIndexer\nConditionParser"]
        alt["alert/\nAlertGenerator\nAtomic Aggregation"]
        scr["scoring/\nRiskScorer\nBurstTracker"]
    end
    
    space:3
    
    block:infra:3
        columns 4
        kaf["kafka/\nEventConsumer\nAlertProducer\nEventLoop"]
        db["database/\nAlertWriter\nMigrations"]
        cch["cache/\nFieldResolution\nRegexCache"]
        red["redis/\nLineageCache\nBurstTracker"]
    end

    entry --> app
    app --> infra

    style entry fill:#1a365d,color:#fff
    style app fill:#2d5016,color:#fff
    style infra fill:#7c2d12,color:#fff
```

| الطبقة | الدور | قاعدة التبعية |
|--------|-------|--------------|
| **Domain** (`internal/domain/`) | النماذج الأساسية: `SigmaRule`, `LogEvent`, `Alert` | لا تعتمد على أي طبقة أخرى |
| **Application** (`internal/application/`) | منطق الكشف والمطابقة والتنبيه | تعتمد على Domain فقط |
| **Infrastructure** (`internal/infrastructure/`) | Kafka, PostgreSQL, Redis, Caches | تعتمد على Domain + Application |

**السبب:** هذا الفصل يضمن أن تغيير Kafka إلى RabbitMQ مثلاً لا يتطلب تعديل أي سطر في محرك الكشف أو منطق المطابقة.

### 2.2 مخطط المكونات وتدفق البيانات

```mermaid
C4Context
    title مخطط مكونات محرك Sigma Detection Engine

    Person(agent, "EDR Agent", "يجمع أحداث الأمان من Windows عبر ETW")
    
    System_Boundary(sigma, "Sigma Detection Engine") {
        Container(consumer, "EventConsumer", "Go / segmentio/kafka-go", "يستهلك الأحداث من Kafka ويحولها إلى LogEvent")
        Container(loop, "EventLoop", "Go goroutines", "يوزع الأحداث على 4 عمال كشف")
        Container(engine, "DetectionEngine", "Go", "يطابق الأحداث مع القواعد المفهرسة")
        Container(alertgen, "AlertGenerator", "Go", "يولد تنبيهات مجمعة مع MITRE")
        Container(producer, "AlertProducer", "Go / segmentio/kafka-go", "ينشر التنبيهات إلى Kafka بدفعات مضغوطة")
    }

    SystemDb(kafka_in, "Kafka Topic\nevents-raw", "أحداث خام")
    SystemDb(kafka_out, "Kafka Topic\nalerts", "تنبيهات")
    SystemDb(pg, "PostgreSQL", "sigma_alerts table")
    SystemDb(redis, "Redis", "Lineage Cache + Burst Tracker")
    Person(analyst, "Security Analyst", "يراقب التنبيهات عبر Dashboard")

    Rel(agent, kafka_in, "gRPC → Kafka")
    Rel(kafka_in, consumer, "ReadMessage()")
    Rel(consumer, loop, "eventChan")
    Rel(loop, engine, "DetectAggregated()")
    Rel(engine, alertgen, "EventMatchResult")
    Rel(alertgen, producer, "alertChan")
    Rel(producer, kafka_out, "WriteMessages()")
    Rel(loop, pg, "AlertWriter.Write()")
    Rel(loop, redis, "LineageCache.WriteEntry()")
    Rel(pg, analyst, "REST API → Dashboard")
```

---

## 3. دورة حياة الحدث (Event Lifecycle)

المخطط التالي يوضح التسلسل الزمني الكامل لمعالجة حدث واحد من لحظة وصوله إلى لحظة ظهوره كتنبيه:

```mermaid
sequenceDiagram
    autonumber
    participant A as EDR Agent
    participant K1 as Kafka<br/>(events-raw)
    participant C as EventConsumer
    participant EL as EventLoop<br/>(Detection Worker)
    participant LC as LineageCache<br/>(Redis)
    participant DE as DetectionEngine
    participant RI as RuleIndexer
    participant SE as SelectionEvaluator
    participant CP as ConditionParser
    participant AG as AlertGenerator
    participant SC as SuppressionCache
    participant K2 as Kafka<br/>(alerts)
    participant DB as PostgreSQL
    participant D as Dashboard

    A->>K1: Publish(process event JSON)
    K1->>C: ReadMessage()
    C->>C: json.Unmarshal() → NewLogEvent()
    C->>EL: eventChan ← LogEvent
    
    Note over EL: processOneEvent()
    
    EL->>LC: hydrateLineageCache(event)<br/>[async, non-blocking]
    
    EL->>DE: DetectAggregated(event)
    DE->>DE: isWhitelistedEvent() → false
    DE->>RI: GetRules("windows", "process_creation", "")
    RI-->>DE: 150 candidate rules [O(1) lookup]
    
    loop لكل قاعدة مرشحة
        DE->>SE: Evaluate(selection, event)
        SE->>SE: ResolveField() → ApplyModifiers()
        SE-->>DE: selection matched: true/false
        DE->>CP: Parse(condition) → AST
        CP-->>DE: AST.Evaluate(selectionResults)
        DE->>DE: calculateConfidence()
    end
    
    DE-->>EL: EventMatchResult (5 rules matched)
    
    EL->>AG: GenerateAggregatedAlert(matchResult)
    AG->>AG: HighestSeverityMatch() → Primary Rule
    AG->>AG: Severity Promotion + MITRE Enrichment
    AG-->>EL: Alert (severity=Critical, risk=85)
    
    EL->>SC: shouldSuppress("ruleID|agentID|proc|pid")
    SC-->>EL: false (first occurrence)
    
    par نشر متوازي
        EL->>K2: producer.Publish(alert)
        EL->>DB: alertWriter.Write(alert)
    end
    
    D->>DB: GET /api/v1/sigma/alerts
    DB-->>D: Alert JSON
```

---

## 4. نموذج البيانات (Domain Model)

```mermaid
classDiagram
    direction TB
    
    class SigmaRule {
        +string ID
        +string Title
        +string Status
        +string Level
        +string Description
        +LogSource LogSource
        +Detection Detection
        +string[] Tags
        +string Author
        +Validate() error
        +Severity() Severity
        +IndexKey() string
    }
    
    class LogSource {
        +*string Product
        +*string Category
        +*string Service
        +IndexKey() string
    }
    
    class Detection {
        +map~string,Selection~ Selections
        +string Condition
        +*string Timeframe
    }
    
    class Selection {
        +string Name
        +SelectionField[] Fields
        +bool IsKeywordSelection
        +string[] Keywords
    }
    
    class SelectionField {
        +string FieldName
        +interface[] Values
        +string[] Modifiers
        +bool IsNegated
        +Regexp[] CompiledRegex
    }
    
    class LogEvent {
        +map RawData
        +*int EventID
        +EventCategory Category
        +string Product
        +time.Time Timestamp
        +map fieldCache
        +GetField(path) interface
        +ComputeHash() string
    }
    
    class Alert {
        +string ID
        +string RuleID
        +string RuleTitle
        +Severity Severity
        +float64 Confidence
        +int MatchCount
        +string[] RelatedRules
        +float64 CombinedConfidence
        +Severity OriginalSeverity
        +bool SeverityPromoted
        +string[] MITRETactics
        +string[] MITRETechniques
        +map MatchedFields
        +map EventData
        +int RiskScore
    }
    
    class EventMatchResult {
        +LogEvent Event
        +RuleMatch[] Matches
        +time.Time Timestamp
        +HasMatches() bool
        +HighestSeverityMatch() RuleMatch
        +AllMITRETechniques() string[]
        +CombinedConfidence() float64
    }
    
    class RuleMatch {
        +SigmaRule Rule
        +float64 Confidence
        +map MatchedFields
        +string[] MatchedSelections
    }
    
    class Severity {
        <<enumeration>>
        Informational = 1
        Low = 2
        Medium = 3
        High = 4
        Critical = 5
    }

    SigmaRule *-- LogSource
    SigmaRule *-- Detection
    Detection *-- Selection
    Selection *-- SelectionField
    EventMatchResult *-- RuleMatch
    EventMatchResult o-- LogEvent
    RuleMatch o-- SigmaRule
    Alert ..> EventMatchResult : generated from
    Alert ..> Severity
    SigmaRule ..> Severity
```

---

## 5. فهرسة القواعد — بنية HashMap الثلاثية

الفهرس يستخدم ثلاث خرائط Hash متداخلة لتحقيق بحث O(1) عن القواعد المناسبة لكل حدث:

```mermaid
graph TB
    subgraph "RuleIndexer — Three-Tier HashMap"
        direction TB
        
        subgraph "Tier 1: Exact Match Index"
            E1["windows:process_creation:sysmon → [Rule 1, Rule 2]"]
            E2["windows:dns_query:* → [Rule 5]"]
            E3["linux:process_creation:* → [Rule 8]"]
        end
        
        subgraph "Tier 2: Category Index"
            C1["windows:process_creation → [Rule 1, 2, 3, 4]"]
            C2["windows:dns_query → [Rule 5, 6]"]
            C3["windows:network_connection → [Rule 7]"]
        end
        
        subgraph "Tier 3: Product Index"
            P1["windows → [Rule 1...150]"]
            P2["linux → [Rule 151...180]"]
        end
        
        subgraph "Tier 4: Fallback"
            F1["allRules → [Rule 1...2000]"]
        end
    end
    
    Q["GetRules(product, category, service)"] --> E1
    E1 -. "miss" .-> C1
    C1 -. "miss" .-> P1
    P1 -. "miss" .-> F1

    style Q fill:#1a365d,color:#fff
```

**خوارزمية البحث:**

| المحاولة | المفتاح | التعقيد | مثال |
|----------|---------|---------|------|
| 1. Exact | `product:category:service` | O(1) | `windows:process_creation:sysmon` |
| 2. Category | `product:category` | O(1) | `windows:process_creation` |
| 3. Product | `product` | O(1) | `windows` |
| 4. Fallback | كل القواعد | O(1) | 2000 قاعدة |

**الأثر على الأداء:** بدلاً من تقييم 2000 قاعدة لكل حدث، يتم تقييم ~150 قاعدة فقط (القواعد المتوافقة مع نوع الحدث). هذا يحقق تحسين 13x في الأداء.

---

## 6. خوارزمية حل الحقول (Field Resolution Pipeline)

المحرك يدعم 4 تنسيقات مختلفة لأسماء الحقول. خوارزمية `ResolveField` تبحث عبر 7 مراحل متسلسلة:

```mermaid
stateDiagram-v2
    direction LR
    
    [*] --> DirectAccess: ResolveField(event, "Image")
    
    DirectAccess: المرحلة 1<br/>البحث المباشر<br/>eventData["Image"]
    NestedAccess: المرحلة 2<br/>البحث المتداخل<br/>getNested("Image")
    SigmaToECS: المرحلة 3<br/>تحويل Sigma→ECS<br/>"Image" → "process.name"
    Alternatives: المرحلة 4<br/>الحقول البديلة<br/>process.executable, TargetImage
    AgentData: المرحلة 5<br/>مسارات Agent<br/>"Image" → "data.executable"
    FallbackChain: المرحلة 5b<br/>سلسلة الاحتياط<br/>data.executable → data.name
    SysmonPaths: المرحلة 6<br/>مسارات Sysmon<br/>Event.EventData.Image
    BroadSearch: المرحلة 7<br/>بحث شامل<br/>كل البدائل الممكنة
    Found: تم العثور<br/>على القيمة ✓
    NotFound: الحقل غير<br/>موجود (nil)
    
    DirectAccess --> Found: وُجد
    DirectAccess --> NestedAccess: لم يُوجد
    NestedAccess --> Found: وُجد
    NestedAccess --> SigmaToECS: لم يُوجد
    SigmaToECS --> Found: وُجد
    SigmaToECS --> Alternatives: لم يُوجد
    Alternatives --> Found: وُجد
    Alternatives --> AgentData: لم يُوجد
    AgentData --> Found: وُجد
    AgentData --> FallbackChain: لم يُوجد
    FallbackChain --> Found: وُجد
    FallbackChain --> SysmonPaths: لم يُوجد
    SysmonPaths --> Found: وُجد
    SysmonPaths --> BroadSearch: لم يُوجد
    BroadSearch --> Found: وُجد
    BroadSearch --> NotFound: لم يُوجد
```

**السبب:** قواعد Sigma العالمية تستخدم أسماء Sysmon (مثل `Image`)، لكن الـ Agent الخاص بنا يرسل البيانات في مسار `data.executable`. بدون هذه الترجمة، لن تعمل أي قاعدة Sigma.

---

## 7. خوارزمية المطابقة (Detection Algorithm)

### 7.1 مخطط حالة المطابقة

```mermaid
stateDiagram-v2
    direction TB
    
    [*] --> WhitelistCheck: DetectAggregated(event)
    
    WhitelistCheck: فحص القائمة البيضاء
    CandidateRules: جلب القواعد المرشحة<br/>O(1) index lookup
    RuleEvaluation: تقييم القاعدة
    SelectionEval: تقييم Selections
    ConditionEval: تقييم الشرط المنطقي<br/>(AST Evaluation)
    FilterCheck: فحص الفلاتر<br/>(False Positive Prevention)
    ConfidenceGate: بوابة الثقة<br/>confidence ≥ 0.6?
    AddMatch: إضافة RuleMatch<br/>إلى EventMatchResult
    NextRule: القاعدة التالية
    ReturnResult: إرجاع EventMatchResult<br/>(كل القواعد المطابقة)
    Discard: رفض (لا تنبيه)
    
    WhitelistCheck --> Discard: حدث مدرج في whitelist
    WhitelistCheck --> CandidateRules: غير مدرج
    CandidateRules --> RuleEvaluation: 150 قاعدة مرشحة
    
    RuleEvaluation --> SelectionEval
    SelectionEval --> ConditionEval: selectionResults
    ConditionEval --> Discard: الشرط = false
    ConditionEval --> FilterCheck: الشرط = true
    FilterCheck --> Discard: فلتر مطابق (false positive)
    FilterCheck --> ConfidenceGate: لا فلتر مطابق
    ConfidenceGate --> Discard: confidence < 0.6
    ConfidenceGate --> AddMatch: confidence ≥ 0.6
    AddMatch --> NextRule
    NextRule --> RuleEvaluation: قواعد متبقية
    NextRule --> ReturnResult: انتهت القواعد
```

### 7.2 تقييم Selection — المنطق الداخلي

```mermaid
stateDiagram-v2
    direction LR
    
    [*] --> CheckType: Evaluate(selection, event)
    
    CheckType: فحص نوع Selection
    
    state "Keyword Selection" as KW {
        KW1: تحويل الحدث إلى JSON
        KW2: البحث عن أي keyword
        KW1 --> KW2
    }
    
    state "Field-Based Selection" as FB {
        FB1: لكل حقل في Selection
        FB2: ResolveField() — حل الحقل
        FB3: ApplyModifiers() — تطبيق المعدّلات
        FB4: فحص all modifier
        FB1 --> FB2
        FB2 --> FB3
        FB3 --> FB4
    }
    
    CheckType --> KW: IsKeywordSelection
    CheckType --> FB: Field-Based
    
    KW --> [*]: OR بين الكلمات المفتاحية
    FB --> [*]: AND بين الحقول<br/>(early exit عند أول فشل)
```

**المنطق:**

| المستوى | العملية | التوضيح |
|---------|---------|---------|
| بين **الحقول** في Selection واحد | **AND** | كل الحقول يجب أن تطابق |
| بين **القيم** لحقل واحد | **OR** (افتراضي) | أي قيمة تكفي |
| بين **القيم** مع modifier `all` | **AND** | كل القيم يجب أن تطابق |
| بين **Selections** | حسب **Condition** | `and`, `or`, `not` |

---

## 8. محلل الشروط (Condition Parser)

المحلل يستخدم خوارزمية Recursive Descent Parser لبناء شجرة صياغة مجردة (AST):

```mermaid
graph TB
    subgraph "المدخل: condition string"
        Input["selection_img and selection_cli and not filter_admin"]
    end
    
    subgraph "الخطوة 1: Tokenization"
        T1["Identifier:<br/>selection_img"]
        T2["AND"]
        T3["Identifier:<br/>selection_cli"]
        T4["AND"]
        T5["NOT"]
        T6["Identifier:<br/>filter_admin"]
        T1 --- T2 --- T3 --- T4 --- T5 --- T6
    end
    
    subgraph "الخطوة 2: AST Construction"
        AND1["AndNode"]
        AND2["AndNode"]
        SEL1["SelectionNode<br/>selection_img"]
        SEL2["SelectionNode<br/>selection_cli"]
        NOT1["NotNode"]
        SEL3["SelectionNode<br/>filter_admin"]
        
        AND1 --> |Left| SEL1
        AND1 --> |Right| AND2
        AND2 --> |Left| SEL2
        AND2 --> |Right| NOT1
        NOT1 --> |Child| SEL3
    end
    
    subgraph "الخطوة 3: Evaluation"
        direction LR
        R["selectionResults =<br/>{selection_img: true,<br/>selection_cli: true,<br/>filter_admin: false}"]
        E["true AND (true AND NOT(false))<br/>= true AND (true AND true)<br/>= true ✓"]
    end
    
    Input --> T1
    T6 --> AND1
    AND1 --> R
    R --> E
```

**الأنماط المدعومة:**

| النمط | المعنى | مثال |
|-------|--------|------|
| `A and B` | كلا الشرطين | `selection and not filter` |
| `A or B` | أحد الشرطين | `selection1 or selection2` |
| `not A` | نفي | `not filter_admin` |
| `(A or B) and C` | تجميع بأقواس | `(sel1 or sel2) and not filter` |
| `1 of selection_*` | واحد من نمط | أي selection يبدأ بـ `selection_` |
| `all of them` | كل الـ selections | جميعها يجب أن تطابق |

---

## 9. حساب الثقة (Confidence Scoring)

```mermaid
graph TB
    subgraph "الصيغة: confidence = baseConf × fieldFactor × contextScore"
        direction TB
        
        subgraph "baseConf — من مستوى القاعدة"
            B1["critical → 1.0"]
            B2["high → 0.8"]
            B3["medium → 0.6"]
            B4["low → 0.4"]
            B5["informational → 0.2"]
        end
        
        subgraph "fieldFactor — نسبة الحقول المطابقة"
            F1["matchedFields / totalPositiveFields"]
            F2["يستبعد filter selections من الحساب"]
        end
        
        subgraph "contextScore — وجود السياق"
            C1["ParentImage غائب → × 0.80"]
            C2["CommandLine غائب → × 0.85"]
            C3["User غائب → × 0.90"]
        end
    end
    
    subgraph "بوابة القرار"
        G{"confidence ≥ 0.6?"}
        PASS["✅ إنشاء تنبيه"]
        REJECT["❌ رفض المطابقة"]
    end
    
    B1 & B2 & B3 & B4 & B5 --> MUL["×"]
    F1 --> MUL
    C1 & C2 & C3 --> MUL2["×"]
    MUL --> MUL2
    MUL2 --> G
    G -->|"نعم"| PASS
    G -->|"لا"| REJECT
```

**مثال:** قاعدة `high` مع كل الحقول مطابقة لكن بدون `ParentImage`:
```
0.8 × (2/2) × 0.8 = 0.64 ≥ 0.6 → ✅ تنبيه
```

**مثال:** قاعدة `medium` مع حقل واحد من اثنين:
```
0.6 × (1/2) × 1.0 = 0.30 < 0.6 → ❌ رفض
```

---

## 10. التجميع الذري للتنبيهات (Atomic Alert Aggregation)

```mermaid
sequenceDiagram
    participant E as حدث واحد<br/>(PowerShell)
    participant DE as DetectionEngine
    participant R1 as Rule 1<br/>(Encoded PS)
    participant R2 as Rule 2<br/>(Hidden Window)
    participant R3 as Rule 3<br/>(NoProfile)
    participant AG as AlertGenerator
    participant A as تنبيه واحد مُجمَّع

    E->>DE: DetectAggregated()
    
    par تقييم متوازي
        DE->>R1: evaluate → match (High, 0.8)
        DE->>R2: evaluate → match (Medium, 0.7)
        DE->>R3: evaluate → match (Medium, 0.6)
    end
    
    DE->>DE: EventMatchResult{Matches: [R1, R2, R3]}
    DE->>AG: GenerateAggregatedAlert()
    
    Note over AG: 1. Primary = R1 (أعلى severity)
    Note over AG: 2. MatchCount = 3
    Note over AG: 3. Severity: High → ترقية؟
    Note over AG: 4. CombinedConfidence = 0.8 + 0.10 = 0.90
    Note over AG: 5. Rule 3: conf > 0.9 → severity++
    
    AG->>A: Alert{<br/>Severity: Critical ↑<br/>MatchCount: 3<br/>RelatedRules: [R2, R3]<br/>MITRE: merged<br/>}
    
    Note over A: تنبيه واحد بدلاً من 3<br/>يقلل Alert Fatigue بنسبة 67%
```

**قواعد ترقية الخطورة (Severity Promotion):**

| الشرط | النتيجة | السبب |
|-------|---------|-------|
| `matchCount > 3` و `severity < High` | ترقية إلى **High** | تعدد القواعد يزيد اليقين |
| `matchCount > 5` و `confidence > 0.8` | ترقية إلى **Critical** | حدث خطير جداً |
| `combinedConfidence > 0.9` | +1 مستوى | ثقة عالية جداً |

---

## 11. منع طوفان التنبيهات (Alert Suppression)

```mermaid
sequenceDiagram
    participant E1 as حدث 1<br/>(powershell, PID 5544)
    participant E2 as حدث 2<br/>(powershell, PID 5544)<br/>بعد 10 ثوان
    participant E3 as حدث 3<br/>(cmd.exe, PID 6000)<br/>بعد 20 ثانية
    participant SC as SuppressionCache<br/>(TTL = 60s)
    participant AC as alertChan

    E1->>SC: shouldSuppress("rule1|agent1|powershell|5544")
    SC-->>E1: false (أول مرة)
    SC->>SC: entries["rule1|agent1|powershell|5544"] = now
    E1->>AC: ✅ alert published

    E2->>SC: shouldSuppress("rule1|agent1|powershell|5544")
    SC-->>E2: true (موجود، عمره 10s < 60s)
    Note over E2: ❌ suppressed (مكرر)

    E3->>SC: shouldSuppress("rule1|agent1|cmd|6000")
    SC-->>E3: false (مفتاح مختلف!)
    E3->>AC: ✅ alert published (عملية مختلفة)
```

**المفتاح = `ruleID|agentID|processName|PID`** — يمنع تكرار نفس التنبيه لنفس العملية، لكن يسمح بتنبيهات لعمليات مختلفة على نفس الجهاز.

---

## 12. نظام المعدّلات (Modifiers)

| المعدّل | الخوارزمية | الاستخدام الأمني |
|---------|-----------|-----------------|
| `contains` | `strings.Contains` (case-insensitive) | كشف كلمات مشبوهة في سطر الأوامر |
| `startswith` | `strings.HasPrefix` | كشف ملفات من مسارات مشبوهة |
| `endswith` | `strings.HasSuffix` | كشف أنواع الملفات التنفيذية |
| `re` / `regex` | `regexp.MatchString` | أنماط معقدة (يُجمَّع مسبقاً لتحسين الأداء) |
| `all` | AND logic بين كل القيم | كشف تركيبة أوامر محددة |
| `base64` | فك ترميز + مقارنة | كشف أوامر مشفرة |
| `windash` | استبدال `-` و `/` | كشف التهرب بتبديل محارف الأوامر |
| `cidr` | `net.ParseCIDR` + `Contains` | كشف اتصالات لشبكات مشبوهة |
| `gt`/`lt`/`gte`/`lte` | مقارنات رقمية | فحص أرقام المنافذ أو معرفات العمليات |

---

## 13. ملخص القرارات التصميمية

| القرار | البديل | السبب |
|--------|--------|-------|
| **Go** كلغة تطوير | Python, Rust | توازن بين الأداء (10x أسرع من Python) وسرعة التطوير (أبسط من Rust)، مع goroutines مدمجة |
| **Hexagonal Architecture** | Monolithic, Clean Architecture | فصل المنطق عن البنية التحتية — أبسط من Clean Architecture لحجم المشروع |
| **HashMap ثلاثي** للفهرسة | Inverted Index, Brute Force | O(1) lookup كافٍ لحالتنا — أبسط من Inverted Index بـ 50 سطر بدل 5000 |
| **Kafka** كمكون رسائل | RabbitMQ, Redis Streams | المعيار الصناعي في SIEM/EDR، يدعم replay ومرونة عالية |
| **PostgreSQL** للتنبيهات | Elasticsearch | كافٍ لحجم التنبيهات (آلاف/يوم)، أبسط تشغيلاً، يدعم `jsonb` |
| **التجميع الذري** | تنبيه لكل قاعدة | يقلل Alert Fatigue بنسبة 60-80% |
| **Confidence Gate 0.6** | عتبة ثابتة per-rule | يوازن بين الكشف وتقليل False Positives — قابل للتعديل |
| **Pre-compiled Regex** | Compile at match time | يوفر ~100,000 compile/sec من وقت المعالج |
| **Async Lineage Writes** | Sync Redis writes | يمنع Redis latency من إبطاء pipeline الكشف |

---

## 14. مقاييس الأداء

| المقياس | القيمة |
|---------|--------|
| معدل المعالجة | 3,000-5,000 حدث/ثانية |
| زمن المطابقة | < 0.5 مللي ثانية/حدث |
| عدد القواعد المدعومة | 2,000+ قاعدة Sigma |
| زمن تحميل القواعد (من كاش) | ~200 مللي ثانية |
| زمن تحميل القواعد (من ملفات) | ~2-5 ثوان |
| عدد عمال الكشف | 4 (قابل للتعديل) |
| نافذة كبح التكرار | 60 ثانية |
| فئات الأحداث المدعومة | 34 فئة |
| تنسيقات الحقول المدعومة | 4 (Sigma, ECS, Sysmon, Agent) |
| حقول المطابقة المسجّلة | 40+ حقل |
