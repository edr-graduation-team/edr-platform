<div dir="rtl" lang="ar">

# تقرير آلية الاستجابة الكاملة — منصة EDR
### السيرفر والـ Agent: الاستجابة المؤتمتة والاستجابة اليدوية

> **المشروع:** EDR Platform | **الإصدار:** 6_version | **التاريخ:** 19 أبريل 2026

---

## نظرة عامة على المعمارية

منصة EDR تعمل بنموذج **ثنائي الطرف** — السيرفر والـ Agent — وكلٌّ منهما يملك نوعين من الاستجابة:

```
┌─────────────────────────────────────────────────────────────────┐
│                       EDR Platform                              │
│                                                                 │
│  ┌──────────────────────────┐    ┌──────────────────────────┐   │
│  │        SERVER            │    │        AGENT             │   │
│  │  ┌──────────────────┐    │    │  ┌──────────────────┐    │   │
│  │  │    مؤتمتة        │    │◄──►│  │    مؤتمتة        │    │   │
│  │  │  (Automated)     │    │gRPC│  │  [مخططة]         │    │   │
│  │  └──────────────────┘    │    │  └──────────────────┘    │   │
│  │  ┌──────────────────┐    │    │  ┌──────────────────┐    │   │
│  │  │    يدوية         │    │    │  │    يدوية         │    │   │
│  │  │  (Manual)        │    │    │  │  (تنفيذ أوامر)   │    │   │
│  │  └──────────────────┘    │    │  └──────────────────┘    │   │
│  └──────────────────────────┘    └──────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

---

## القسم الأول: الاستجابة على جانب السيرفر

### 1.1 الاستجابة المؤتمتة على السيرفر

الـ Server يتكون من مكوّنَين رئيسيَّين يعملان معاً:
- **Sigma Engine** — محرك التحليل والكشف
- **Connection Manager** — مدير الاتصالات وإصدار الأوامر

#### أ) آلية Playbook (التشغيل التلقائي للمهام)
**الملف:** `sigma_engine_go/internal/automation/playbook.go`

عند وصول تنبيه (Alert)، يقوم **Playbook Manager** تلقائياً بتنفيذ سلسلة من الخطوات المُعرَّفة مسبقاً:

```
Alert يصل من Agent
      │
      ▼
PlaybookManager.ExecuteForAlert()
      │
      ├─► هل يطابق Severity المُعرَّف؟ (critical/high/medium/low)
      ├─► هل يطابق Rule ID المُحدَّد؟
      ├─► هل يطابق Agent ID المُستهدَف؟
      │
      ▼ (عند التطابق — يُطلَق في goroutine مستقل)
      │
      ├── Step 1: notify_slack  → إرسال إشعار Slack
      ├── Step 2: notify_teams  → إرسال إشعار Microsoft Teams
      ├── Step 3: notify_email  → إرسال بريد إلكتروني
      ├── Step 4: create_ticket → إنشاء تذكرة (Ticket)
      ├── Step 5: escalate      → تصعيد للمستوى الأعلى
      ├── Step 6: wait          → انتظار مدة محددة
      ├── Step 7: conditional   → تنفيذ شرطي
      └── Step 8: webhook       → استدعاء رابط خارجي
```

**خصائص تقنية الـ Playbook:**
- يُنفَّذ في goroutine مستقل مع Timeout 5 دقائق
- كل خطوة تدعم إعادة المحاولة (`max_retries`) وسياسة الخطأ (`on_error: continue/stop/retry`)
- يحتفظ بأحدث 1000 سجل تنفيذ في الذاكرة
- الشروط تدعم: `equals | contains | matches (regex)`

#### ب) آلية Escalation (التصعيد التلقائي الزمني)
**الملف:** `sigma_engine_go/internal/automation/escalation.go`

إذا بقي التنبيه دون معالجة لفترة محددة، يُفعَّل التصعيد تلقائياً:

```
Alert يُسجَّل كـ "open"
      │
      ▼ (فحص دوري في الخلفية)
EscalationManager.CheckEscalations()
      │
      ├─► هل مضت N دقيقة على التنبيه؟ (TimeThreshold)
      ├─► هل حالته لا تزال "open"؟
      ├─► هل تطابقت درجة الخطورة؟
      │
      ▼ (عند توفر الشروط)
      │
      ├── Level 1: notify      → إشعار فريق الأمن (مثلاً بعد 15 دقيقة)
      ├── Level 2: create_ticket → إنشاء تذكرة رسمية (بعد 30 دقيقة)
      └── Level 3: page_oncall → استدعاء المناوب على الفور (بعد 60 دقيقة)
```

#### ج) سجل تتبع أوامر التنفيذ
**الملف:** `connection-manager/internal/repository/command_policy_repo.go`

كل أمر يُصدَر (سواء يدوي أو مؤتمت) يُسجَّل في PostgreSQL بالحالة الكاملة:

| الحالة | المعنى |
|---|---|
| `pending` | الأمر أُنشئ وينتظر الإرسال |
| `sent` | أُرسل للـ Agent عبر gRPC |
| `acknowledged` | الـ Agent استلمه |
| `executing` | الـ Agent يُنفِّذه الآن |
| `completed` | اكتمل بنجاح + النتيجة مُحفوظة |
| `failed` | فشل + رسالة الخطأ مُحفوظة |
| `timeout` | انتهت مدة صلاحيته قبل التنفيذ |
| `cancelled` | أُلغي يدوياً |

---

### 1.2 الاستجابة اليدوية على السيرفر

تأتي من المشغِّل (Analyst/Admin) عبر **Dashboard** أو عبر **API** مباشرة.

#### أ) إصدار أوامر مباشرة للـ Agent من الـ Dashboard

| الأمر | النوع | ما يفعله |
|---|---|---|
| `TERMINATE_PROCESS` | يدوي | اختيار عملية بعينها وإنهاؤها |
| `QUARANTINE_FILE` | يدوي | تحديد ملف ونقله للحجر |
| `ISOLATE_NETWORK` | يدوي | عزل الجهاز شبكياً بالكامل |
| `UNISOLATE_NETWORK` | يدوي | رفع عزل الجهاز |
| `COLLECT_FORENSICS` | يدوي | طلب جمع سجلات أحداث محددة |
| `RUN_CMD` | يدوي | تشغيل أمر تشخيصي (من قائمة مُعتمَدة) |
| `RESTART_SERVICE` | يدوي | إعادة تشغيل خدمة الـ Agent |
| `UPDATE_CONFIG` | يدوي | تحديث إعدادات الـ Agent (Hot-reload) |
| `RESTART` | يدوي | إعادة تشغيل الجهاز (يتطلب confirm=true) |
| `SHUTDOWN` | يدوي | إيقاف الجهاز (يتطلب confirm=true) |

#### ب) إدارة Playbooks والسياسات

| الفعل | النوع | التفاصيل |
|---|---|---|
| إنشاء Playbook جديد | يدوي | تعريف خطوات وشروط التشغيل |
| تفعيل/تعطيل Playbook | يدوي | التحكم في الـ Playbooks النشطة |
| إنشاء Escalation Rule | يدوي | تحديد حدود زمنية ومستويات تصعيد |
| تحديث سياسة الفلترة | يدوي | رفع/خفض حساسية رصد الأحداث |

#### ج) إدارة حالة التنبيهات

| الفعل | المعنى |
|---|---|
| `acknowledge` | المشغِّل يُؤكد استلام التنبيه ويبدأ التحقيق |
| `resolve` | المشغِّل يُغلق التنبيه بعد معالجته |
| `escalate` | رفع التنبيه لفريق أو شخص آخر |

---

### ملخص الاستجابة على السيرفر

```
┌─────────────────────────────────────────────────────────────┐
│                   SERVER — Response Map                     │
├─────────────────────────┬───────────────────────────────────┤
│   مؤتمتة (Automated)    │   يدوية (Manual)                  │
├─────────────────────────┼───────────────────────────────────┤
│ ✅ Playbook Execution    │ ✅ Agent Commands (Dashboard)      │
│   - Slack/Teams/Email   │   - Terminate, Quarantine         │
│   - Create Ticket       │   - Isolate, Forensics            │
│   - Webhook / Wait      │   - Restart, Shutdown             │
│   - Conditional Steps   │   - Update Config                 │
│                         │                                   │
│ ✅ Auto Escalation       │ ✅ Playbook Management             │
│   - Level 1: Notify     │   - Create / Edit / Delete        │
│   - Level 2: Ticket     │   - Enable / Disable              │
│   - Level 3: Page OnCall│                                   │
│                         │ ✅ Alert Management                │
│ ✅ Command Lifecycle DB  │   - Acknowledge / Resolve         │
│   (pending→completed)   │   - Manual Escalate               │
└─────────────────────────┴───────────────────────────────────┘
```

---

## القسم الثاني: الاستجابة على جانب الـ Agent

### 2.1 الاستجابة المؤتمتة على الـ Agent

> **الوضع الحالي:** غائبة تقريباً — كل الاستجابة تأتي من السيرفر.
> **المخطط إضافته:** نظام استجابة ذاتي كامل بدون الحاجة للسيرفر.

#### أ) ما هو موجود حالياً (Tamper Protection فقط)

الـ Agent يملك استجابة ذاتية **واحدة فقط** حالياً: **الحماية من التلاعب بنفسه:**

| الطبقة | ما يحدث تلقائياً |
|---|---|
| Process DACL | يمنع تلقائياً أي محاولة إنهاء العملية من غير SYSTEM |
| Service DACL | يمنع تلقائياً `sc stop EDRAgent` من المشغِّل |
| Registry Lock | يمنع تلقائياً تعديل مفاتيح الـ Registry الخاصة به |
| Reconnector | يُعيد الاتصال تلقائياً عند انقطاع gRPC |
| Isolation Watchdog | يُحدِّث قواعد الـ Firewall تلقائياً إذا تغيّر IP السيرفر أثناء العزل |

#### ب) ما هو مخطط إضافته — الاستجابة الذاتية الكاملة

**الإضافة 1: قاعدة بيانات التوقيعات المحلية**

```
التوقيعات (SHA-256 hashes)
محفوظة محلياً في bbolt DB
بدون أي اتصال بالسيرفر
```

**الإضافة 2: فحص الملفات وحجرها تلقائياً**

```
ETW File I/O Event (FileCreate / FileWrite)
            │
            ▼
    هل المسار ذو أولوية؟
    (Downloads, Desktop, Temp, USB)
            │
    ┌───────┴────────┐
    │ لا             │ نعم
    ▼                ▼
  تمرير          حساب SHA-256
  للسيرفر              │
                       ▼
               بحث في Signature DB
                       │
              ┌────────┴─────────┐
              │ لا تطابق        │ تطابق
              ▼                  ▼
         إرسال للسيرفر    حجر فوري تلقائي
                         + إرسال إشعار للسيرفر
                         + تسجيل في Audit Log
```

**الإضافة 3: مراقب USB التلقائي**

```
توصيل USB جديد (WMI VolumeChange)
            │
            ▼
تسجيل مسار الـ USB في Auto-Responder
            │
            ▼
كل ملف يُنسخ من/إلى USB → فحص فوري
            │
      ┌─────┴──────┐
      │ آمن        │ خطير
      ▼            ▼
   تمرير       حجر فوري +
   للسيرفر    تسجيل بيانات
              الجهاز (Serial)
```

**الإضافة 4: Process Tree Termination**

عند تلقي أمر `TERMINATE_PROCESS` بـ `kill_tree=true`:
```
يقرأ بيانات كل العمليات (Snapshot)
→ يبني شجرة Parent-Child
→ يقتل كل أبناء العملية المستهدفة
→ يقتل العملية نفسها أخيراً
→ يُرجع قائمة بكل PID تم إنهاؤه
```

**الإضافة 5: الحجب الشبكي الانتقائي**

بدلاً من العزل الكامل، يمكن حجب IP أو Domain محدد:
```
BLOCK_IP    → قاعدة Firewall لـ IP بعينه
BLOCK_DOMAIN → إضافة لملف hosts (DNS Sinkhole)
```

---

### 2.2 الاستجابة اليدوية على الـ Agent

الـ Agent لا يملك واجهة يدوية مباشرة — كل الأوامر اليدوية **تأتي من الـ Server** عبر gRPC. الـ Agent يستقبلها وينفذها.

#### آلية التنفيذ

```
السيرفر يُرسل Command عبر gRPC
            │
            ▼
runCommandLoop() يستقبله
            │
            ▼
يُحوَّل لـ command.Command{}
            │
            ▼
يُوزَّع لـ Worker Goroutine (حد: 8 متزامنة)
            │
            ▼
commandHandler.Execute() ينفذه
            │
            ▼
النتيجة تُرسَل للسيرفر عبر SendCommandResult()
(Status: SUCCESS / FAILED / TIMEOUT)
```

#### الأوامر اليدوية التي يُنفِّذها الـ Agent

| الأمر | آلية التنفيذ | الحماية المدمجة |
|---|---|---|
| `TERMINATE_PROCESS` | Win32 API مباشر | قائمة حظر لعمليات النظام الحرجة |
| `QUARANTINE_FILE` | os.Rename + metadata | حماية مسارات النظام |
| `ISOLATE_NETWORK` | netsh advfirewall | ACK-before-block (4s) + Watchdog |
| `UNISOLATE_NETWORK` | netsh restore | إلغاء Watchdog أولاً |
| `COLLECT_FORENSICS` | wevtutil.exe | جمع حتى 500 حدث لكل نوع |
| `UPDATE_CONFIG` | Hot-reload | تحقق من صحة الإعدادات أولاً |
| `RESTART_SERVICE` | Detached Script | خروج الـ Agent أولاً ثم الإعادة |
| `RUN_CMD` | exec.Command مباشر | Whitelist صارمة (12 أمر فقط) |
| `RESTART` / `SHUTDOWN` | shutdown.exe /r /t 30 | يتطلب `confirm=true` صريحاً |

---

### ملخص الاستجابة على الـ Agent

```
┌─────────────────────────────────────────────────────────────┐
│                   AGENT — Response Map                      │
├─────────────────────────┬───────────────────────────────────┤
│   مؤتمتة (Automated)    │   يدوية (Manual)                  │
├─────────────────────────┼───────────────────────────────────┤
│ ✅ موجود حالياً:         │ ✅ تنفيذ أوامر السيرفر:           │
│   - Tamper Protection   │   - TERMINATE_PROCESS             │
│   - Isolation Watchdog  │   - QUARANTINE_FILE               │
│   - Auto Reconnect      │   - ISOLATE_NETWORK               │
│                         │   - UNISOLATE_NETWORK             │
│ 🔵 مخطط إضافته:         │   - COLLECT_FORENSICS             │
│   - Local Signature DB  │   - UPDATE_CONFIG                 │
│   - File Auto-Scan      │   - RUN_CMD (whitelist)           │
│   - Auto-Quarantine     │   - RESTART_SERVICE               │
│   - USB Watcher         │   - RESTART / SHUTDOWN            │
│   - Selective Block     │                                   │
│   - Process Tree Kill   │                                   │
└─────────────────────────┴───────────────────────────────────┘
```

---

## القسم الثالث: تدفق الاستجابة الكاملة

### المسار الكامل — من الحدث إلى الإجراء

```
┌──────────────────────────────────────────────────────────────────────┐
│  AGENT                          │  SERVER                            │
│                                 │                                    │
│  1. ETW → File Event             │                                    │
│     (FileCreate: evil.exe)      │                                    │
│            │                    │                                    │
│            ▼                    │                                    │
│  2. [مؤتمت - Agent]             │                                    │
│     Local Scan → SHA256         │                                    │
│     Match in Signature DB?      │                                    │
│     ├── نعم: Auto-Quarantine ─┐ │                                    │
│     │         فوراً           │ │                                    │
│     └── لا: تمرير للسيرفر ───┼─┼──────────────────────────────────►│
│                               │ │  3. Kafka → Sigma Engine           │
│                               │ │     Sigma Rule Match?              │
│                               │ │     Context Score > Threshold?     │
│                               │ │             │                      │
│                               │ │  4. [مؤتمت - Server]              │
│                               │ │     Alert يُنشأ                    │
│                               │ │     Playbook يُطلَق:               │
│                               │ │     → Slack إشعار                  │
│                               │ │     → Email إشعار                  │
│◄──────────────────────────────┼─┼── 5. Command يُصدَر للـ Agent      │
│  6. [يدوي - Agent]            │ │     (ISOLATE_NETWORK مثلاً)        │
│     receives Command          │ │                                    │
│     executes it               │ │  7. [يدوي - Server]               │
│     sends Result ─────────────┼─┼──────────────────────────────────►│
│                               │ │     Analyst يرى النتيجة            │
│                               │ │     يُؤكد: acknowledge/resolve     │
│  8. حجر الـ Agent التلقائي    │ │                                    │
│     يُرسَل كـ Event           │ │     إذا بقي Alert مفتوحاً:        │
│     للسيرفر ──────────────────┼─┼──────────────────────────────────►│
│                               │ │  9. [مؤتمت - Server]              │
│                               │ │     Auto-Escalation بعد N دقيقة:  │
│                               │ │     → Level 1: Team Notify         │
│                               │ │     → Level 2: Create Ticket       │
│                               │ │     → Level 3: Page OnCall         │
└───────────────────────────────┴──────────────────────────────────────┘
```

---

## القسم الرابع: جدول مقارنة شامل

| الجانب | نوع الاستجابة | المكوِّن | الحالة |
|---|---|---|---|
| **Server** | مؤتمتة | Playbook: Slack/Teams/Email | ✅ مكتمل |
| **Server** | مؤتمتة | Playbook: Create Ticket | ✅ مكتمل |
| **Server** | مؤتمتة | Playbook: Webhook / Wait | ✅ مكتمل |
| **Server** | مؤتمتة | Playbook: Conditional Logic | ✅ مكتمل |
| **Server** | مؤتمتة | Auto-Escalation (Level 1-3) | ✅ مكتمل |
| **Server** | مؤتمتة | إصدار أوامر تلقائياً للـ Agent | ⚠️ يحتاج ربط مع Playbook |
| **Server** | يدوية | أوامر Agent من Dashboard | ✅ مكتمل |
| **Server** | يدوية | إدارة Playbooks | ✅ مكتمل |
| **Server** | يدوية | Acknowledge / Resolve Alert | ✅ مكتمل |
| **Agent** | مؤتمتة | Tamper Protection | ✅ مكتمل |
| **Agent** | مؤتمتة | Isolation Watchdog | ✅ مكتمل |
| **Agent** | مؤتمتة | Auto-Reconnect gRPC | ✅ مكتمل |
| **Agent** | مؤتمتة | **Local File Scan + Auto-Quarantine** | 🔵 مخطط |
| **Agent** | مؤتمتة | **USB Device Watcher** | 🔵 مخطط |
| **Agent** | مؤتمتة | **Local Signature DB** | 🔵 مخطط |
| **Agent** | مؤتمتة | **Selective IP/Domain Block** | 🔵 مخطط |
| **Agent** | يدوية | TERMINATE_PROCESS | ✅ مكتمل |
| **Agent** | يدوية | QUARANTINE_FILE | ✅ مكتمل |
| **Agent** | يدوية | ISOLATE / UNISOLATE_NETWORK | ✅ مكتمل |
| **Agent** | يدوية | COLLECT_FORENSICS | ✅ مكتمل |
| **Agent** | يدوية | UPDATE_CONFIG (Hot-reload) | ✅ مكتمل |
| **Agent** | يدوية | RUN_CMD (Whitelist) | ✅ مكتمل |
| **Agent** | يدوية | RESTART_SERVICE | ✅ مكتمل |
| **Agent** | يدوية | RESTART / SHUTDOWN | ✅ مكتمل |
| **Agent** | يدوية | **TERMINATE_PROCESS (Process Tree)** | 🔵 مخطط |
| **Agent** | يدوية | **UPDATE_SIGNATURES** | 🔵 مخطط |

> **مفتاح الألوان:** ✅ مكتمل ومُختبَر | ⚠️ موجود جزئياً | 🔵 مخطط للإضافة

</div>
