# الجزء الثالث: Confidence + التنبيهات + Suppression + المثال الكامل + أسئلة المناقشة

---

## 11. حساب الثقة (Confidence Scoring)

> المرجع: [detection_engine.go](file:///d:/EDR_Platform/sigma_engine_go/internal/application/detection/detection_engine.go)

### 11.1 الصيغة الكاملة

```
confidence = baseConf × fieldFactor × contextScore

حيث:
  confidence ∈ [0.0, 1.0]
  إذا confidence < MinConfidence (0.6) → رفض المطابقة
```

### 11.2 `baseConf` — من مستوى القاعدة

```go
func getLevelConfidence(level string) float64 {
    switch level {
    case "critical":      return 1.0
    case "high":          return 0.8
    case "medium":        return 0.6
    case "low":           return 0.4
    case "informational": return 0.2
    default:              return 0.5
    }
}
```

**لماذا هذه القيم؟**
- `critical = 1.0`: القواعد الحرجة (مثل Mimikatz) نادراً ما تكون false positive
- `high = 0.8`: احتمال عالي لكن ليس مؤكد
- `medium = 0.6`: يساوي بالضبط MinConfidence — يعني أي عامل سلبي واحد يرفضها
- `low = 0.4`: أقل من MinConfidence افتراضياً — تحتاج fieldFactor = 1.0 + contextScore مرتفع لتمرر

### 11.3 `fieldFactor` — نسبة الحقول المطابقة

```
fieldFactor = matchedPositiveFields / totalPositiveFields

"Positive" = كل selection لا يبدأ اسمها بـ "filter"

مثال:
  selection_img: Image|endswith matched ✓ (1 field)
  selection_cli: CommandLine|contains|all matched ✓ (1 field — CommandLine)
  filter_system: User matched ✓ (لا تُحسب — filter)
  
  matchedPositiveFields = 2 (Image + CommandLine)
  totalPositiveFields = 2
  fieldFactor = 2/2 = 1.0
```

**لماذا نستبعد filter selections؟** الـ filter هو شرط استبعاد (not). مطابقته تعني أن الحدث **غير خبيث**. لا نريد أن يزيد الثقة.

### 11.4 `contextScore` — التحقق من السياق

```go
func validateContext(rule, event, matchedFields) float64 {
    score := 1.0
    
    // إذا القاعدة تتوقع ParentImage لكنه غائب
    if ruleReferences("ParentImage") && !fieldExists(event, "ParentImage") {
        score *= 0.8    // ← سياق ناقص = ثقة أقل
    }
    
    // إذا القاعدة تتوقع CommandLine لكنه غائب
    if ruleReferences("CommandLine") && !fieldExists(event, "CommandLine") {
        score *= 0.85
    }
    
    // إذا القاعدة تتوقع User لكنه غائب
    if ruleReferences("User") && !fieldExists(event, "User") {
        score *= 0.9
    }
    
    return score
}
```

**لماذا؟** قاعدة تكشف `powershell.exe` بدون `CommandLine` = تطابق جزئي. ربما العملية هي PowerShell ISE وليس هجوم. تقليل الثقة يقلل False Positives.

### 11.5 MinConfidence Gate — لماذا 0.6؟

```
0.6 = الحد الذي يوازن بين:
  ← أقل: تنبيهات كثيرة لا قيمة لها (alert fatigue)
  → أعلى: يفوِّت هجمات حقيقية (missed detections)

مثال حسابي:
  قاعدة high (0.8) × fieldFactor(1.0) × context(1.0) = 0.8 ≥ 0.6 ✓ تمرر
  قاعدة medium (0.6) × fieldFactor(0.5) × context(1.0) = 0.3 < 0.6 ✗ ترفض
  قاعدة medium (0.6) × fieldFactor(1.0) × context(0.8) = 0.48 < 0.6 ✗ ترفض

القيمة 0.6 قابلة للتعديل في config.yaml: detection.min_confidence
```

---

## 12. توليد التنبيهات (Alert Generation)

> المرجع: [alert_generator.go](file:///d:/EDR_Platform/sigma_engine_go/internal/application/alert/alert_generator.go)

### 12.1 لماذا Atomic Event Aggregation؟

**المشكلة:** حدث واحد (مثل PowerShell مشفر) يمكن أن يطابق **5-10 قواعد** مختلفة:
- "Suspicious PowerShell Encoded Command"
- "PowerShell with -NoProfile"
- "Hidden Window PowerShell Execution"
- "Base64 Encoded PowerShell"
- "PowerShell Execution from Explorer"

بدون تجميع: **5 تنبيهات** لنفس الحدث = **alert fatigue**

مع تجميع ذري: **تنبيه واحد** يقول "5 قواعد طابقت هذا الحدث" + قائمة القواعد المرتبطة

### 12.2 `GenerateAggregatedAlert()` — الخوارزمية

```
═══ الخطوة 1: اختيار القاعدة الأساسية (Primary Rule) ═══

primary = matchResult.HighestSeverityMatch()
  → يختار القاعدة ذات أعلى Severity
  → إذا تساوت: يختار الأعلى Confidence

مثال: 5 matches
  Rule A: severity=Medium, confidence=0.7
  Rule B: severity=High, confidence=0.8    ← PRIMARY
  Rule C: severity=Medium, confidence=0.9
  Rule D: severity=High, confidence=0.6
  Rule E: severity=Low, confidence=0.5

═══ الخطوة 2: جمع MITRE من كل القواعد ═══

allTechniques = unique([T1059.001, T1059, T1055, T1078, T1059.001])
             = [T1059.001, T1059, T1055, T1078]

tactics = techniqueToTactic لكل technique
        = [Execution, Defense Evasion]

═══ الخطوة 3: حساب Severity الأصلي ═══

originalSeverity = primary.Rule.Severity()  // High = 4

═══ الخطوة 4: Severity Promotion (ترقية) ═══

3 قواعد ترقية:

القاعدة 1: matchCount > 3 AND severity < High → promote to High
  5 > 3 ✓ AND severity=High (ليس أقل) ✗ → لا ترقية

القاعدة 2: matchCount > 5 AND confidence > 0.8 → promote to Critical
  5 ≯ 5 ✗ → لا ترقية

القاعدة 3: combinedConfidence > 0.9 → +1 level
  combined = max(0.8, 0.9, 0.7...) + min(0.2, (5-1)×0.05) = 0.9 + 0.2 = 1.1 → cap 1.0
  1.0 > 0.9 ✓ → severity++ = Critical!

finalSeverity = Critical (was High), severityPromoted = true

═══ الخطوة 5: بناء Alert ═══

alert = {
    ID:                 "alert-uuid-v4",
    RuleID:             "rule-B-id",           // Primary rule
    RuleTitle:          "PowerShell with Hidden Window",
    Severity:           Critical,               // ← promoted!
    Confidence:         0.8,
    MatchCount:         5,
    RelatedRules:       ["Rule A", "Rule C", "Rule D", "Rule E"],
    RelatedRuleIDs:     ["id-a", "id-c", "id-d", "id-e"],
    CombinedConfidence: 1.0,
    OriginalSeverity:   High,
    SeverityPromoted:   true,
    MITRETactics:       ["Execution", "Defense Evasion"],
    MITRETechniques:    ["T1059.001", "T1055", "T1078"],
    MatchedFields:      {Image: "...", CommandLine: "..."},
    EventData:          {sanitized raw data},     // بدون password/token/apikey
}
```

### 12.3 `sanitizeEventData()` — لماذا؟

```go
sensitiveFields := map[string]bool{
    "password": true, "passwd": true, "pwd": true,
    "secret": true, "token": true, "api_key": true, "apikey": true,
}
```

**التبرير:** التنبيهات تُخزن في PostgreSQL وتُعرض في Dashboard. لا نريد كلمات مرور أو API keys في قاعدة البيانات أو على الشبكة. هذا يتوافق مع **GDPR** و **PCI-DSS** compliance.

---

## 13. Alert Suppression — منع طوفان التنبيهات

> المرجع: [event_loop.go سطر 82-139](file:///d:/EDR_Platform/sigma_engine_go/internal/infrastructure/kafka/event_loop.go#L82-L139)

### 13.1 المشكلة

```
مهاجم يشغّل loop ينفذ powershell.exe كل ثانية
→ 60 حدث/دقيقة × 5 قواعد مطابقة = 300 تنبيه/دقيقة
→ 18,000 تنبيه/ساعة ← alert fatigue كارثي
```

### 13.2 الخوارزمية

```go
type suppressionCache struct {
    entries map[string]time.Time    // key → first-seen timestamp
    ttl     time.Duration           // 60 seconds
}

func shouldSuppress(key string) bool {
    // Phase 1: Read lock (fast path — 90% of cases)
    sc.mu.RLock()
    if ts, exists := entries[key]; exists && now-ts < ttl {
        sc.mu.RUnlock()
        return true     // ← suppressed (already seen within 60s)
    }
    sc.mu.RUnlock()

    // Phase 2: Write lock (slow path)
    sc.mu.Lock()
    // Double-check after acquiring write lock
    if ts, exists := entries[key]; exists && now-ts < ttl {
        return true     // ← another goroutine wrote it
    }
    entries[key] = now  // ← record first occurrence
    return false        // ← NOT suppressed (first time)
}
```

### 13.3 Suppression Key — لماذا هذا التصميم؟

```go
// event_loop.go سطر 403
suppressKey = fmt.Sprintf("%s|%s|%s|%d", 
    baseAlert.RuleID,    // أي قاعدة
    agentStr,            // أي جهاز
    processName,         // أي عملية
    pidVal,              // أي PID
)
```

**S5 FIX:** النسخة القديمة كانت `ruleID|agentID` فقط. المشكلة: مهاجم يشغل `powershell -enc AAA` ثم `powershell -enc BBB` — نفس القاعدة ونفس الـ agent لكن **هجومان مختلفان**. بإضافة `processName|pid` نفرّق بينهما.

### 13.4 لماذا TTL = 60 ثانية؟

| القيمة | الإيجابيات | السلبيات |
|--------|-----------|----------|
| 10s | تنبيهات أكثر، فقدان أقل | alert fatigue |
| **60s** ✅ | توازن جيد | يفقد تنبيهات في نافذة 60s |
| 300s | تنبيهات أقل بكثير | يفقد هجمات حقيقية |

60 ثانية = نافذة كافية لتجميع burst من نفس الـ process بدون فقدان هجمات جديدة.

---

## 14. Risk Scoring — السياق الذكي

### 14.1 LineageCache (Redis)

يحفظ سلسلة نسب العمليات (process ancestry) لآخر 12 دقيقة:
```
agent-abc123:pid:5544 → {
    PID: 5544,
    PPID: 1234,
    Name: "powershell.exe",
    Executable: "C:\...\powershell.exe",
    ParentName: "explorer.exe",
    Timestamp: "2026-04-08T07:00:00Z"
}
```

**Hydration:** كل حدث process يُكتب لـ Redis **قبل** تقييم القواعد (fire-and-forget عبر قناة غير متزامنة):

```go
// event_loop.go سطر 726-735
select {
case el.lineageWriteCh <- entry:    // non-blocking enqueue
default:
    // channel full → skip (best-effort)
}
```

### 14.2 حساب RiskScore

```
RiskScore = base_score + lineage_bonus + burst_bonus - fp_discount

base_score:   من severity (High=80, Critical=100, Medium=50, Low=30)
lineage_bonus: +5 إذا سلسلة الأجداد مشبوهة (explorer→cmd→powershell)
burst_bonus:   +10 إذا نفس الـ agent أنتج 5+ تنبيهات في 5 دقائق
fp_discount:   -10 إذا FP risk > 0.5

النتيجة تُقيَّد في [0, 100]
```

### 14.3 Graceful Degradation بدون Redis

```go
// event_loop.go سطر 334-336
if el.lineageCache != nil {
    el.hydrateLineageCache(event)   // only if Redis available
}
// ...
if el.riskScorer != nil {
    scoreOut, err := el.riskScorer.Score(...)
    if err != nil {
        logger.Warnf("RiskScorer error — using base score")
        // alert goes through with RiskScore=0
    }
}
```

**التبرير:** Redis هو **optional enrichment**. إذا تعطل، المحرك يعمل بكامل وظائفه بدون risk scoring. التنبيهات ستظهر بـ `RiskScore=0` فقط.

---

## 15. نشر التنبيهات

### 15.1 Kafka Producer — Batched Publishing

```go
// producer.go سطر 122-186
writer := kafka.Writer{
    Addr:         kafka.TCP(brokers...),
    Topic:        "alerts",
    Balancer:     &kafka.Hash{},        // Partition by ruleID
    BatchSize:    50,                    // يجمع 50 رسالة قبل الإرسال
    BatchTimeout: 100 * time.Millisecond, // أو كل 100ms
    RequiredAcks: kafka.RequiredAcks(-1), // acks=all → كل replicas تؤكد
    Compression:  kafka.Snappy,          // ضغط ~60% أقل حجم
}
```

**لماذا `acks=-1`?** التنبيهات الأمنية لا يمكن فقدانها. `acks=all` يضمن أن كل نسخ Kafka حفظت الرسالة قبل التأكيد.

### 15.2 PostgreSQL — AlertWriter

```go
// alertPublisher في event_loop.go سطر 440-455
for alert := range el.alertChan {
    // 1. نشر إلى Kafka
    el.producer.Publish(alert)
    
    // 2. حفظ في PostgreSQL
    if el.alertWriter != nil {
        el.alertWriter.Write(alert)
    }
}
```

**لماذا قناتين (Kafka + PostgreSQL)؟**
- **Kafka**: للخدمات الأخرى التي تريد الاشتراك (SOAR, تنبيهات Slack, archiving)
- **PostgreSQL**: للـ Dashboard والـ REST API — استعلام سريع مع فلترة وترتيب

---

## 16. مثال عملي END-TO-END — كل function call

### السيناريو: مهاجم ينفذ PowerShell مشفر من Desktop

```
الوقت: 2026-04-08 07:00:00
المهاجم: فتح explorer.exe → نقر على ملف خبيث → ينفذ powershell.exe -nop -w hidden -enc <base64>
```

---

```
════════ الخطوة 1: الـ Agent يلتقط العملية ════════

ETW Provider: Microsoft-Windows-Kernel-Process
→ Agent collector → gRPC stream → Connection Manager → Kafka (events-raw)

Kafka Message (JSON):
{
  "agent_id": "agent-7f3b2",
  "event_type": "process",
  "source": {"hostname": "WORKSTATION-01", "os_type": "windows"},
  "timestamp": "2026-04-08T07:00:00.123Z",
  "data": {
    "pid": 5544,
    "ppid": 1234,
    "name": "powershell.exe",
    "executable": "C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe",
    "command_line": "powershell.exe -nop -w hidden -enc SQBFAFgAIAAoAE4AZQB3AC0ATwBiAGoAZQBjAHQA...",
    "parent_executable": "C:\\Windows\\explorer.exe",
    "parent_name": "explorer.exe",
    "user_name": "CORP\\john.doe",
    "working_directory": "C:\\Users\\john.doe\\Desktop"
  }
}

════════ الخطوة 2: EventConsumer.consumeLoop() ════════

→ reader.ReadMessage(ctx)               [consumer.go:167]
→ json.Unmarshal(msg.Value, &rawData)   [consumer.go:218]
→ rawData["_kafka_partition"] = 1       [consumer.go:224]
→ domain.NewLogEvent(rawData)           [consumer.go:230]
  → extractEventID() → nil             [event.go:257]
  → inferCategory():
      event_type = "process" → process_creation  [event.go:280]
  → extractProduct():
      source.os_type = "windows"                 [event.go:320]
→ eventChan <- event                    [consumer.go:202]

════════ الخطوة 3: Detection Worker picks up event ════════

→ processOneEvent(event)                [event_loop.go:319]

═══ 3a. Lineage Cache Hydration ═══
→ hydrateLineageCache(event)            [event_loop.go:686]
  → event_type = "process" ✓
  → agent_id = "agent-7f3b2"
  → entry = {PID:5544, PPID:1234, Name:"powershell.exe", ...}
  → lineageWriteCh <- entry (non-blocking) [event_loop.go:729]
  
→ (background) lineageWriteWorker:
  → lineageCache.WriteEntry(ctx, entry)  [event_loop.go:757]
  → Redis SET "lineage:agent-7f3b2:5544" → JSON → TTL 12min

═══ 3b. UEBA Baseline ═══
→ baselines.ShouldRecord(rawData) → true
→ baselineAggregator.Record(input) (fire-and-forget)

════════ الخطوة 4: DetectAggregated(event) ════════

→ isWhitelistedEvent(event):             [detection_engine.go]
  Image = "C:\...\powershell.exe" → NOT in whitelist ✓

→ getCandidateRules(event):              [detection_engine.go]
  product="windows", category="process_creation", service=""
  → ruleIndex.GetRules("windows","process_creation","")
  → categoryIndex["windows:process_creation"] → 150 rules

→ for each of 150 rules:
  → evaluateRuleForAggregation(rule, event)

═══ Rule: "Suspicious Encoded PowerShell" (HIGH) ═══

  الخطوة 4.1 — Selection Evaluation:

  selection_img: {Image|endswith: ['\powershell.exe', '\pwsh.exe']}
    → resolveFieldValue("Image", event)
      → fieldMapper.ResolveField(rawData, "Image")
        المرحلة 1: rawData["Image"] → nil
        المرحلة 5: sigmaToAgentData["Image"] = "data.executable"
          → getNested(rawData, "data.executable")
            rawData["data"]["executable"] = "C:\...\powershell.exe" ✓
      → value = "C:\...\powershell.exe"
    → endswith(value, '\powershell.exe') → true ✓
    → endswith(value, '\pwsh.exe') → false
    → OR: true ✓
  selectionResults["selection_img"] = true

  selection_cli: {CommandLine|contains|all: ['-nop', '-w hidden', '-enc']}
    → resolveFieldValue("CommandLine", event)
      → sigmaToAgentData["CommandLine"] = "data.command_line"
      → value = "powershell.exe -nop -w hidden -enc SQBFAFgA..."
    → modifier "all" → AND logic:
      contains(value, "-nop")      → true ✓
      contains(value, "-w hidden") → true ✓
      contains(value, "-enc")      → true ✓
      All true ✓
  selectionResults["selection_cli"] = true
  matchedFields = {"Image": "C:\...\powershell.exe", "CommandLine": "...-enc..."}

  الخطوة 4.2 — Condition:
  condition = "selection_img and selection_cli"
  Parse → AndNode(SelectionNode("selection_img"), SelectionNode("selection_cli"))
  Evaluate → true AND true = true ✓

  الخطوة 4.3 — Filter check:
  لا يوجد filter selections → pass

  الخطوة 4.4 — Confidence:
  baseConf = 0.8 (high)
  fieldFactor = 2/2 = 1.0
  contextScore = 1.0 (ParentImage exists, CommandLine exists, User exists)
  confidence = 0.8 × 1.0 × 1.0 = 0.8 ≥ 0.6 ✓

  الخطوة 4.5 → RuleMatch{Rule, Confidence:0.8, MatchedFields, Techniques:["T1059.001"]}

═══ (4 more rules also match — similar process) ═══

→ eventResult = EventMatchResult{
    Event: event,
    Matches: [5 RuleMatch objects],
    Timestamp: now,
  }

════════ الخطوة 5: Alert Generation ════════

→ alertGenerator.GenerateAggregatedAlert(eventResult)  [event_loop.go:358]
  → primary = HighestSeverityMatch() → Rule "Encoded PowerShell" (High, 0.8)
  → allTechniques = [T1059.001, T1059, ...]
  → severity promotion: 5 matches, combined_conf=1.0 > 0.9 → High → Critical!
  → return Alert{Severity:Critical, MatchCount:5, ...}

════════ الخطوة 6: Risk Scoring ════════

→ riskScorer.Score(input)                [event_loop.go:378]
  → lineageCache.LookupAncestry("agent-7f3b2", 5544)
    → [explorer.exe → powershell.exe] ← مشبوه
  → base=100 (critical) + lineage=5 + burst=0 = 105 → cap(100)
  → alert.RiskScore = 100

════════ الخطوة 7: Suppression ════════

→ suppressKey = "rule-xxx|agent-7f3b2|powershell.exe|5544"
→ suppression.shouldSuppress(key) → false (first time)
→ alertChan <- alert                     [event_loop.go:411]

════════ الخطوة 8: Publishing ════════

→ alertPublisher goroutine:
  ← alert from alertChan                 [event_loop.go:440]
  → producer.Publish(alert)              [event_loop.go:442]
    → alertToMessage(alert) → kafka.Message{Key:"rule-xxx", Value:JSON}
    → batch = append(batch, msg)
    → len(batch) >= 50? → flush → writer.WriteMessages(batch...)
    → Kafka topic "alerts" ← message published
  
  → alertWriter.Write(alert)             [event_loop.go:451]
    → INSERT INTO sigma_alerts (id, rule_id, title, severity, risk_score,
         confidence, match_count, matched_fields, event_data, context_snapshot,
         mitre_tactics, mitre_techniques, timestamp)
       VALUES (...)

════════ الخطوة 9: Dashboard يعرض التنبيه ════════

→ Dashboard → GET /api/v1/sigma/alerts?severity=critical&limit=50
→ PostgreSQL → SELECT * FROM sigma_alerts ORDER BY timestamp DESC
→ JSON Response:
{
  "id": "alert-a1b2c3d4",
  "rule_title": "Suspicious Encoded PowerShell Command",
  "severity": "critical",
  "risk_score": 100,
  "match_count": 5,
  "related_rules": ["PowerShell NoProfile", "Hidden Window PS", ...],
  "mitre_tactics": ["Execution"],
  "mitre_techniques": ["T1059.001"],
  "matched_fields": {"Image": "...\\powershell.exe", "CommandLine": "...-enc..."},
  "event_data": {"user_name": "CORP\\john.doe", "hostname": "WORKSTATION-01", ...},
  "severity_promoted": true,
  "original_severity": "high"
}

→ يظهر في Dashboard:
  🔴 CRITICAL | Risk: 100/100
  "Suspicious Encoded PowerShell Command"
  5 rules matched | Promoted from HIGH
  MITRE: Execution / T1059.001
  Host: WORKSTATION-01 | User: CORP\john.doe
```

---

## 17. أسئلة المناقشة المتوقعة مع الإجابات

### س1: "لماذا Go وليس Python أو Rust؟"

**الإجابة:**

| المعيار | Go ✅ | Python | Rust |
|---------|-------|--------|------|
| Concurrency | goroutines مدمجة — مثالية لـ multi-worker | GIL يمنع true parallelism | ممتازة لكن أكثر تعقيداً |
| سرعة التطوير | سريعة — compiled + simple syntax | أسرع | أبطأ — lifetime/borrow checker |
| الأداء | ~5-10x أسرع من Python | بطيء للـ regex heavy workloads | الأسرع لكن بتكلفة تطوير |
| Deployment | binary واحد — لا dependencies | يحتاج Python runtime | binary واحد |
| Ecosystem | segmentio/kafka-go, pgx, redis | confluent-kafka, psycopg2 | rdkafka, tokio |

Go يعطي **التوازن الأفضل** بين الأداء وسرعة التطوير لمحرك real-time detection.

---

### س2: "لماذا لم تستخدم Elasticsearch بدل PostgreSQL؟"

**الإجابة:** PostgreSQL يكفي لحجم عملنا (آلاف التنبيهات/يوم). Elasticsearch مصمم لملايين السجلات/ثانية وهو:
- أكثر تعقيداً في التشغيل (cluster، shards، replicas)
- يحتاج 4+ GB RAM minimum
- أبطأ في الكتابة الفردية (optimized for bulk)

PostgreSQL يوفر: ACID transactions، JSON queries مع `jsonb`، وKafka يعمل كـ message bus بين المحرك والخدمات الأخرى. **لو احتجنا scale**: نضيف Elasticsearch كـ consumer إضافي من Kafka topic "alerts" بدون تغيير المحرك.

---

### س3: "ما الحد الأقصى لعدد الأحداث/ثانية؟"

**الإجابة:** مع الإعدادات الحالية (4 workers, 150 candidate rules):
```
Average matching latency: ~0.5ms per event
4 workers × (1000ms / 0.5ms) = ~8,000 events/sec theoretical max
Practical (with I/O + serialization): ~3,000-5,000 events/sec
```
**لزيادة الأداء:** زيادة workers إلى 8-16، إضافة Kafka partitions، أو horizontal scaling (multiple instances بنفس consumer group).

---

### س4: "كيف تتعامل مع False Positives؟"

**الإجابة:** 7 طبقات حماية:
1. **QualityFilter**: يرفض قواعد low/informational و experimental
2. **Rule Filters**: `filter` selections تستبعد أنشطة مشروعة
3. **Global Whitelist**: عمليات نظام معروفة (svchost.exe)
4. **Confidence Gate**: يرفض مطابقات بثقة < 0.6
5. **Context Validation**: يقلل الثقة عند غياب سياق مهم
6. **Severity Promotion**: يرفع الأهمية للتنبيهات المتعددة (الأكثر يقيناً)
7. **Suppression**: يمنع تكرار نفس التنبيه

---

### س5: "ما الفرق بين محركك ومحرك Sigma الرسمي (sigma-cli/pySigma)؟"

| المعيار | محركنا | sigma-cli/pySigma |
|---------|--------|-------------------|
| اللغة | Go (compiled, concurrent) | Python (interpreted) |
| الوضع | Real-time stream processing | Batch/offline conversion |
| الهدف | **Detection engine** (matches events) | **Rule converter** (Sigma → Splunk/Elastic query) |
| Confidence | ✅ حساب ثقة ديناميكي | ❌ لا يوجد |
| Aggregation | ✅ تنبيه واحد لقواعد متعددة | ❌ ليس محرك كشف |
| Risk Scoring | ✅ مع lineage/burst context | ❌ |

**التبرير:** pySigma يحوّل القواعد إلى queries لأنظمة أخرى. محركنا هو **المحرك ذاته** الذي يطابق الأحداث مباشرة — لا يحتاج Splunk أو Elastic.

---

### س6: "لماذا 3 طبقات فهرسة وليس Inverted Index مثل Elasticsearch؟"

**الإجابة:** Elasticsearch يستخدم Inverted Index لأن الـ query يمكن أن يكون أي حقل. في حالتنا، الـ "query" هو **دائماً** logsource (product, category, service). لذلك:
- **HashMap ثلاثي** = O(1) lookup — أبسط وأسرع
- **Inverted Index** = O(log n) to O(1) — أعقد بكثير ولا يقدم ميزة إضافية

الفهرس الثلاثي يحقق نفس النتيجة بـ 50 سطر كود بدل 5000.

---

### س7: "ماذا يحدث إذا تعطل المحرك أثناء المعالجة؟"

**الإجابة:** Kafka Consumer Group يحفظ الـ offset. عند إعادة التشغيل:
- Consumer يستأنف من آخر committed offset
- الأحداث التي لم تُعالج تُعاد معالجتها تلقائياً
- Suppression cache فارغ = بعض التنبيهات قد تتكرر مؤقتاً (acceptable)

---

### س8: "لماذا async lineage writes وليس sync؟"

**الإجابة:** في النسخة الأولى كانت sync. المشكلة: Redis latency = 1-5ms per write. مع 3000 event/sec:
```
Sync:  3000 × 2ms Redis = 6 seconds CPU blocked/sec ← pipeline stalled
Async: 0ms blocking + background writers drain the channel
```
**S2 FIX** حلّها بقناة غير متزامنة (buffer=4096) + 2 worker goroutines مخصصة لـ Redis writes.
