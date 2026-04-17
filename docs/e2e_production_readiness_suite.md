# EDR Full E2E Production Readiness Suite

This file is the mandatory end-to-end validation suite to decide if the EDR is production ready.

It is designed to:
- cover all collector families,
- cover all major detection categories and SOC response capabilities,
- validate context-aware scoring propagation end-to-end,
- produce a final auditable `READY / NOT READY` decision.

---

## 1) Readiness Principle

`NOT READY` unless all critical collector/category/capability checkpoints are executed and passed with evidence.

---

## 2) Coverage Requirements (Mandatory)

## 2.1 Collector Coverage Matrix

Every row must be tested at least once.

| Collector Family | Required Event Types | Minimum Required Fields | Validation Method | Status |
|---|---|---|---|---|
| Process collector | process start/exec | `agent_id`, `event_type`, `timestamp`, `data.name`, `data.executable`, `data.command_line`, `data.pid`, `data.ppid`, `data.user_name` | CP1/CP2 + API payload checks | TODO |
| Network collector | connection/discovery activity | `agent_id`, `event_type`, `timestamp`, `data.name`, `data.command_line`, network keys if present | CP1/CP2 + Sigma match/drop reason | TODO |
| Persistence collector | scheduled task / autorun behavior | process payload + persistence command-line markers | CP1/CP2/CP3 + alert correctness | TODO |
| Privilege collector | elevated/privileged behavior | process payload + `data.integrity_level`, `data.is_elevated`, `data.user_sid` | CP3 + score breakdown privilege component | TODO |
| Defense evasion collector | tamper/evasion behavior | process payload + evasion command markers | CP3 + high/critical risk expectation | TODO |
| Heartbeat/health telemetry | heartbeat metrics | `events_generated`, `events_sent`, `events_dropped`, `queue_depth`, resource usage | endpoint page/API consistency checks | TODO |

## 2.2 Detection Category Coverage Matrix

| Category | Example Trigger | Expected Outcome | Status |
|---|---|---|---|
| Reconnaissance | `whoami /all`, `ipconfig /all` | alert generated, expected severity/risk band | TODO |
| Network Discovery | `netstat -ano`, `arp -a` | alert generated or explicit explainable drop | TODO |
| Persistence | `schtasks /create ...` | persistence alert with valid payload | TODO |
| Privilege/Elevation | `whoami /groups`, `whoami /priv` | privilege bonus appears when applicable | TODO |
| Defense Evasion | `vssadmin list shadows` (lab only) | high-risk alert path validated | TODO |

## 2.3 Capability Coverage Matrix

| Capability | Required Validation | Status |
|---|---|---|
| Ingestion durability | no silent loss across CP1->CP2 | TODO |
| Sigma decisioning | CP3 has clear match/drop reasoning | TODO |
| Alert persistence | alerts visible via API + DB-backed pages | TODO |
| Realtime dashboard behavior | alerts visible without manual refresh | TODO |
| Context-aware scoring | `context_multiplier` and factors present in payload | TODO |
| Endpoint risk aggregation | endpoint risk KPIs consistent with API | TODO |
| Reliability telemetry | fallback/drops counters render and refresh | TODO |
| Response readiness | endpoint action surfaces reachable/consistent | TODO |

---

## 3) Deterministic Scenario Pack (Execute All)

## S1 - Process Execution (Recon)
- Trigger:
  - `whoami /all`
  - `ipconfig /all`
- Expected:
  - process event ingested
  - Sigma rule match (or explainable drop)
  - alert appears in API/UI

## S2 - Network Behavior
- Trigger:
  - `netstat -ano`
  - `arp -a`
- Expected:
  - network/discovery behavioral detection path exercised
  - context snapshot populated

## S3 - Persistence
- Trigger:
  - `schtasks /create /sc onlogon /tn EDRTestTask /tr "cmd.exe /c echo test" /f`
- Expected:
  - persistence-related rule path evaluated
  - risk score and breakdown present

## S4 - Privilege Context
- Trigger:
  - elevated shell + `whoami /groups`
  - `whoami /priv`
- Expected:
  - privilege context field presence
  - privilege-related score component non-zero when applicable

## S5 - Defense Evasion (Lab)
- Trigger:
  - `vssadmin list shadows`
- Expected:
  - evasion path detection exercised
  - high-risk scoring behavior verified

Cleanup:
- `schtasks /delete /tn EDRTestTask /f`

---

## 4) Four Checkpoints Per Scenario (Mandatory)

For each S1..S5 record PASS/FAIL for:

1. **CP1 Ingestion (connection-manager)**  
   Command: `docker compose logs --since 3m connection-manager`

2. **CP2 Raw payload in Kafka events-raw**  
   Command: `docker exec -it edr_platform-kafka-1 kafka-console-consumer --bootstrap-server localhost:9092 --topic events-raw --property print.timestamp=true --timeout-ms 15000 --max-messages 20`

3. **CP3 Sigma match decision**  
   Command: `docker compose logs --since 3m sigma-engine | Select-String "Risk scored alert|suppressed|Published|Failed|Stats"`

4. **CP4 API/UI correctness**  
   Command: `curl -s http://localhost:30080/api/v1/sigma/alerts`

---

## 5) Context-Aware Mandatory Assertions

For matched alerts, payload must include:
- `risk_score`
- `false_positive_risk`
- `context_snapshot`
- `score_breakdown`
- `score_breakdown.context_multiplier`
- `score_breakdown.user_role_weight`
- `score_breakdown.device_criticality_weight`
- `score_breakdown.network_anomaly_factor`
- `score_breakdown.context_quality_score`
- `score_breakdown.quality_factor`
- `context_snapshot.missing_context_fields` (when source event is partial)

Missing-field behavior policy must be validated:
- In soft mode, alert is still persisted with reduced `quality_factor`.
- In strict mode (if explicitly enabled), event may be dropped before scoring.
- For partial context events in soft mode, `quality_factor` must be `< 1.0` and warning evidence should appear in `context_snapshot.warnings`.

Context policy path must be validated:
- Settings page can create/edit policy
- `/api/v1/context-policies` responds correctly
- policy effects visible in newly generated alerts

---

## 6) Performance & Reliability Assertions

Validate Statistics + Reliability surfaces:
- `avg_event_latency_ms`
- `avg_rule_matching_ms`
- `avg_database_query_ms`
- `error_rate`
- `alert_fallback_used`
- `alerts_dropped`
- reliability fallback counters and headline state
- context quality degradation evidence in alerts (`quality_factor`, missing fields, warnings)

Important:
- metrics may be zero on idle system; verify under active scenario load.

---

## 7) Required Final Report Template

Copy/fill this exactly:

```md
# EDR Full E2E Production Readiness Report

## A) Metadata
- Tester:
- Date (UTC):
- Environment:
- Build/Tag:
- Dashboard URL:
- Sigma API URL:
- Connection Manager URL:

## B) Service Health
- All required containers healthy: PASS/FAIL
- Any restart loop observed: YES/NO
- Notes:

## C) Collector Coverage Result
| Collector | PASS/FAIL | Evidence | Notes |
|---|---|---|---|
| Process |  |  |  |
| Network |  |  |  |
| Persistence |  |  |  |
| Privilege |  |  |  |
| Evasion |  |  |  |
| Heartbeat |  |  |  |

## D) Category Coverage Result
| Category | PASS/FAIL | Evidence | Notes |
|---|---|---|---|
| Recon |  |  |  |
| Network Discovery |  |  |  |
| Persistence |  |  |  |
| Privilege |  |  |  |
| Defense Evasion |  |  |  |

## E) Scenario Checkpoints
### S1
- CP1:
- CP2:
- CP3:
- CP4:
- Context-aware assertions:

### S2
- CP1:
- CP2:
- CP3:
- CP4:
- Context-aware assertions:

### S3
- CP1:
- CP2:
- CP3:
- CP4:
- Context-aware assertions:

### S4
- CP1:
- CP2:
- CP3:
- CP4:
- Context-aware assertions:

### S5
- CP1:
- CP2:
- CP3:
- CP4:
- Context-aware assertions:

## F) Performance & Reliability
- Avg Latency behavior:
- Rule Match Time behavior:
- DB Query Time behavior:
- Error Rate behavior:
- Alert Fallback Used behavior:
- Alerts Dropped behavior:
- Reliability page consistency:

## G) Gap Classification
| Gap ID | Class (collector/mapping/category_inference/rule_coverage/dashboard_render) | Severity (P0/P1/P2) | Summary | Owner | Status |
|---|---|---|---|---|---|
| G1 |  |  |  |  |  |

## H) Final Decision
- READY / NOT READY:
- Blocking P0 items:
- Recommended fixes:
- Retest required:
- Sign-off:
```

---

## 8) Production Gate Criteria

Mark `READY` only if:
- All collector rows completed and passed.
- All 5 scenarios executed.
- All 4 checkpoints passed for every scenario.
- Context-aware assertions passed on matched alerts.
- No unresolved P0 gaps.
- Realtime dashboard behavior confirmed.

Else mark `NOT READY`.

---

## 9) الشرح العربي الكامل (Full Arabic Guide)

هذا القسم يشرح نفس خطة الاختبار السابقة لكن باللغة العربية وبأسلوب تنفيذي واضح، مع الحفاظ على المصطلحات التقنية بالإنجليزية حتى يبقى التقرير مناسبًا للفريق التقني وفرق SOC.

### 9.1 هدف هذا الملف

الهدف من هذا الملف هو إصدار قرار واضح وقابل للتدقيق:
- `READY` إذا كانت المنظومة تعمل End-to-End بشكل صحيح وموثوق.
- `NOT READY` إذا وُجدت فجوات حرجة (خصوصًا P0) أو نقص في التغطية.

لماذا هذا مهم؟
- لأن نجاح اختبار جزئي لا يكفي للإنتاج.
- المطلوب هو إثبات سلامة السلسلة كاملة: جمع الحدث -> ingestion -> Kafka -> Sigma decision -> alert persistence -> dashboard rendering.

---

### 9.2 الـ E2E Flow الذي نتحقق منه

لكل سيناريو (S1..S5) يجب إثبات المسار التالي:
1. **Agent/Collector** ينتج event صحيح.
2. **Connection Manager** يستقبله بدون فقدان (CP1).
3. **Kafka events-raw** يحتوي payload المتوقع (CP2).
4. **Sigma Engine** يتخذ قرار match/drop مع سبب مفهوم (CP3).
5. **API/Dashboard** يعرض alert النهائي بشكل صحيح (CP4).

إذا انكسر أي جزء من هذه السلسلة، فالنتيجة التشغيلية في الإنتاج غير موثوقة حتى لو كانت أجزاء أخرى سليمة.

---

### 9.3 شرح Coverage Requirements

#### A) Collector Coverage Matrix
هذا الجدول يضمن أننا لم نختبر نوعًا واحدًا من البيانات فقط، بل جميع عائلات الجمع الأساسية:
- Process
- Network
- Persistence
- Privilege
- Defense Evasion
- Heartbeat/Health

لكل Collector:
- **Required Event Types**: ما نوع الأحداث التي يجب أن تظهر.
- **Minimum Required Fields**: الحد الأدنى من الحقول لتشغيل detection وscoring بشكل صحيح.
- **Validation Method**: كيف نثبت المرور عبر checkpoints.
- **Status**: نتيجة التنفيذ.

#### B) Detection Category Coverage Matrix
هذا الجدول يضمن تغطية فئات كشف مختلفة (Recon, Persistence, Evasion...)، لأن نظام EDR في الإنتاج يجب أن يتعامل مع طيف هجمات متنوع.

#### C) Capability Coverage Matrix
هذا الجدول يتحقق من قدرات المنصة نفسها، مثل:
- Ingestion durability
- Sigma decisioning
- Realtime dashboard
- Context-aware scoring
- Reliability telemetry

أي نقص هنا يعني أن النظام قد "يكشف" لكنه غير قابل للتشغيل المؤسسي بثقة.

---

### 9.4 شرح Deterministic Scenario Pack

#### S1 - Process Execution (Recon)
أوامر مثل `whoami /all` و `ipconfig /all`:
- تختبر مسار process collection الأساسي.
- مفيدة لقياس صحة الإدخال والتصنيف الأولي.

#### S2 - Network Behavior
أوامر مثل `netstat -ano` و `arp -a`:
- تختبر network/discovery telemetry.
- تساعد على التأكد أن events الشبكية تصل وتُقيّم.

#### S3 - Persistence
أمر `schtasks /create ...`:
- يختبر سلوك persistence الفعلي.
- يجب أن يظهر في scoring مع `risk_score` و`score_breakdown`.

#### S4 - Privilege Context
أوامر privilege (`whoami /groups`, `whoami /priv`) من shell مرفوع:
- تختبر وجود حقول privilege (`integrity_level`, `is_elevated`, `user_sid`).
- يجب أن يظهر أثر privilege component في scoring عند انطباق الشروط.

#### S5 - Defense Evasion (Lab)
أمر `vssadmin list shadows` (في بيئة اختبار فقط):
- يختبر مسار evasion.
- نتوقع سلوك scoring أعلى خطورة عند المطابقة.

---

### 9.5 شرح Four Checkpoints (CP1..CP4)

#### CP1 - Ingestion (connection-manager logs)
نتحقق أن الحدث دخل المنصة ولم يُفقد مبكرًا.

#### CP2 - Raw Payload in Kafka (`events-raw`)
نتحقق أن payload الخام موجود وبالحقول المتوقعة.

#### CP3 - Sigma Match Decision
نتحقق من قرار Sigma:
- هل حصل match؟
- أم drop؟
- وما السبب؟

#### CP4 - API/UI Correctness
نتحقق أن مخرجات detection وصلت لنقطة العرض:
- موجودة في API.
- معروضة في dashboard بشكل صحيح.

هذه النقاط الأربعة تمنع false confidence الناتج عن اختبار طبقة واحدة فقط.

---

### 9.6 شرح Context-Aware Mandatory Assertions

عند وجود alert مطابق، يجب أن تظهر حقول أساسية:
- `risk_score`
- `false_positive_risk`
- `context_snapshot`
- `score_breakdown`
- `score_breakdown.context_multiplier`
- `score_breakdown.user_role_weight`
- `score_breakdown.device_criticality_weight`
- `score_breakdown.network_anomaly_factor`
- `score_breakdown.context_quality_score`
- `score_breakdown.quality_factor`

وعند وجود نقص في البيانات المصدر:
- يجب ظهور `context_snapshot.missing_context_fields`.

#### سياسة الحقول الناقصة (Missing-field behavior)
- **Soft mode**: لا نسقط التنبيه، لكن نخفض الثقة عبر `quality_factor`.
- **Strict mode** (إذا فُعّل صراحة): يمكن إسقاط الحدث قبل scoring.

#### لماذا هذا best practice؟
- يمنع فقدان الرؤية الأمنية في حالات partial telemetry.
- يمنع المبالغة في الثقة عندما السياق ناقص.
- يعطي أثرًا audit-ready داخل payload.

---

### 9.7 شرح Performance & Reliability Assertions

المؤشرات المطلوب التحقق منها:
- `avg_event_latency_ms`
- `avg_rule_matching_ms`
- `avg_database_query_ms`
- `error_rate`
- `alert_fallback_used`
- `alerts_dropped`

تفسير تشغيلي:
- `latency/match/db` تقيس الأداء.
- `error_rate` يقيس الاستقرار.
- `fallback_used` يقيس الضغط أو أعطال المسار الأساسي.
- `alerts_dropped` مؤشر فقدان بيانات محتمل (خطير إذا غير صفري).

ملاحظة مهمة:
- القيم قد تكون صفر في idle state؛ يجب التحقق أثناء تحميل سيناريوهات فعلية.

---

### 9.8 كيفية تعبئة التقرير النهائي بشكل صحيح

القالب في القسم 7 إلزامي لأنه يوحد طريقة الحكم بين المختبرين.

أفضل ممارسة تعبئة:
- لكل PASS/FAIL أضف evidence واضح (log snippet / API output / screenshot reference).
- لا تضع PASS بدون دليل.
- أي gap يجب تصنيفه ضمن:
  - `collector`
  - `mapping`
  - `category_inference`
  - `rule_coverage`
  - `dashboard_render`

الأولوية:
- `P0`: مانع إنتاج.
- `P1`: مهم لكن قد يُقبل مؤقتًا بخطة زمنية.
- `P2`: تحسينات غير مانعة.

---

### 9.9 قواعد القرار النهائي (Production Gate)

ضع `READY` فقط إذا:
- كل collector rows تم تنفيذها ونجحت.
- كل السيناريوهات S1..S5 تم تنفيذها.
- كل CP1..CP4 نجحت لكل سيناريو.
- كل context-aware assertions نجحت.
- لا توجد فجوات P0 مفتوحة.
- real-time dashboard behavior مؤكد.

خلاف ذلك القرار الإجباري:
- `NOT READY`

---

### 9.10 Checklist تنفيذ سريع (للمختبر)

قبل الاختبار:
- تأكد من صحة health للخدمات والحاويات.
- حدد window زمني واضح لكل سيناريو.
- جهز commands ونافذة logs.

أثناء الاختبار:
- نفّذ trigger.
- سجّل CP1 -> CP4 مباشرة.
- وثّق أي drop reason.
- تحقق من context-aware fields.

بعد الاختبار:
- املأ الجداول الثلاثة (Collector/Category/Capability).
- املأ template النهائي حرفيًا.
- أصدر قرار READY/NOT READY مع تبرير مختصر.

---

### 9.11 ما الذي يعتبر نجاحًا حقيقيًا؟

النجاح الحقيقي ليس "ظهور alert واحد"، بل:
- ثبات السلسلة كاملة End-to-End.
- اتساق النتيجة بين logs وAPI وdashboard.
- قابلية تفسير القرار عبر `score_breakdown/context_snapshot`.
- عدم وجود فقدان صامت أو تعارض في مصادر البيانات.

