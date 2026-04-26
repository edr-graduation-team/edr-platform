<div dir="rtl">

# تقرير التحقق والتوثيق لمنصة EDR (AWS) — نسخة الإنتاج

**التاريخ**: 2026-04-24  
**الجهة**: Protocol Soft  
**نطاق الاختبار**: بيئة معملية على AWS (Region: `eu-central-1`)  

---

## 1) ملخص تنفيذي

**هدف التحقق**: التأكد عمليًا (End-to-End) أن المنصة تعمل كـ EDR عبر:

- ظهور الأجهزة الثلاثة كـ **Endpoints/Agents** داخل الـ Dashboard.
- وصول **Telemetry/Events** من الوكيل (وليس مجرد تثبيت).
- تحميل وتشغيل **Rules** (Sigma) وإظهار **Alerts** عند تحقق شروط الكشف (قدر الإمكان).
- (اختياري قوي) تنفيذ **Response Action** من لوحة التحكم وتوثيق نتيجته.

**النتيجة النهائية**: *(املأ بعد التنفيذ)*  
- الحالة: **ناجح / جزئي / غير ناجح**
- سبب الحكم (3 نقاط مختصرة): *(مثال: Agents ظاهرة + Events تصل + Alerts ظهرت / أو سبب الفشل)*

---

## 2) معلومات البيئة (Environment)

### 2.1 الدومين والسيرفر
- **Dashboard**: `https://edr.maztc.com/`
- **DNS**: `edr.maztc.com` → `13.49.244.151`
- **سيرفر AWS**:
  - Public IP: `13.49.244.151`
  - Private IP: `172.31.34.133`
  - OS: Ubuntu 24.04.4 LTS

### 2.2 خدمات Docker (لقطة واحدة مطلوبة)
الخدمات بحسب `docker compose ps` (متوقع أنها Healthy):
- dashboard: `0.0.0.0:30088 -> 80`
- connection-manager (REST): `0.0.0.0:30082 -> 8082`
- connection-manager (gRPC للوكيل): `0.0.0.0:47051 -> 50051`
- sigma-engine: `0.0.0.0:30080 -> 8080`
- postgres: `31432 -> 5432`
- redis: `31379 -> 6379`
- kafka: `9092` + `31292 -> 29092`
- zookeeper: `31181 -> 2181`

### 2.3 الأجهزة التي عليها الوكيل (Windows Agents)
| الاسم | InstanceId | Private IP | Public IP | الحالة |
|---|---|---:|---:|---|
| win-edr-lab-eu-central-1 | i-0ca3a9fda1ebd197d | 172.31.42.177 | 3.72.37.208 | running |
| win-edr-lab-eu-central-2 | i-0503ef450a2a9b549 | 172.31.44.91 | 3.127.151.241 | running |
| win-edr-lab-eu-central-3 | i-086bbd0f3b5c7833e | 172.31.35.85 | 35.157.158.135 | running |

---

## 3) الأدلة (Evidence Index) — ضع اللقطات هنا

> **مهم**: اكتب رقم الدليل على اسم ملف الصورة (مثال: `E4-endpoints-list.png`) وضعها في مجلد `report_images/` ثم اربطها هنا.

| ID | الدليل | الوصف | رابط/اسم ملف اللقطة |
|---|---|---|---|
| E1 | DNS | `nslookup -q=A edr.maztc.com` يثبت الربط | `report_images/E1-nslookup.png` |
| E2 | تشغيل الخدمات | ناتج `docker compose ps` (healthy) | `report_images/E2-docker-compose-ps.png` |
| E3 | تسجيل الدخول | صفحة Login + نجاح الدخول | `report_images/E3-login-success.png` |
| E4 | Endpoints | قائمة Endpoints تظهر 3 أجهزة | `report_images/E4-endpoints-list.png` |
| E5 | Endpoint Detail | تفاصيل Endpoint واحد (status/last_seen/IP) | `report_images/E5-endpoint-detail.png` |
| E6 | نشاط على الوكيل | PowerShell على جهاز Windows (hostname/commands) | `report_images/E6-agent-powershell.png` |
| E7 | Events | صفحة Events تُظهر ingestion من نفس الجهاز | `report_images/E7-events-ingestion.png` |
| E8 | Rules | صفحة Rules تثبت أن Sigma rules محمّلة | `report_images/E8-rules-list.png` |
| E9 | Alerts | صفحة Alerts + تفاصيل Alert (إن وُجد) | `report_images/E9-alerts.png` |
| E10 | Response Action (اختياري) | تنفيذ أمر (مثال kill_process) ونتيجته | `report_images/E10-response-action.png` |

---

## 4) خطوات التحقق (Validation Steps)

### 4.1 التحقق من DNS وربط الدومين
نفّذ من جهازك (أو أي جهاز إداري):

```powershell
nslookup -q=A edr.maztc.com
```

- **معيار النجاح**: يظهر `Address: 13.49.244.151`
- **لقطة مطلوبة**: E1

---

### 4.2 التحقق من تشغيل الخدمات على السيرفر (Docker)
على السيرفر:

```bash
cd ~/edr-platform
docker compose ps
```

- **معيار النجاح**: كل الخدمات الأساسية **Up (healthy)**
- **لقطة مطلوبة**: E2

---

### 4.3 الدخول إلى لوحة التحكم
افتح:
- `https://edr.maztc.com/`
- ثم `https://edr.maztc.com/login` (إن تم تحويلك للـ login)

**بيانات الدخول**: *(املأ وفق حساب الإنتاج لديك)*  
- Username: `__________`
- Password: `__________`

- **معيار النجاح**: دخول ناجح وظهور الواجهة الرئيسية بدون أخطاء 401/403
- **لقطة مطلوبة**: E3

> ملاحظة تشغيلية: بعد التحقق، يفضّل تغيير كلمة مرور admin وتوثيق ذلك ضمن سياسة التشغيل (اختياري).

---

### 4.4 التحقق من ظهور الأجهزة الثلاثة (Endpoints / Agents)
من القائمة الجانبية في الـ Dashboard:
- افتح صفحة **Endpoints** (أو Agents)
- ابحث عن: `win-edr-lab-eu-central-1/2/3`

- **معايير النجاح**:
  - ظهور **3 Endpoints**
  - حالة الاتصال **online** أو **Last Seen** حديث (خلال 1–2 دقيقة)
  - OS = Windows، مع IPs/Agent Version إن توفرت

- **لقطات مطلوبة**:
  - E4: قائمة Endpoints الثلاثة
  - E5: تفاصيل Endpoint واحد

---

### 4.5 التحقق من وصول Events (Telemetry Ingestion)
#### 4.5.1 توليد نشاط على جهاز Windows (لإجبار Telemetry)
على أحد الأجهزة (RDP) شغّل PowerShell كمسؤول:

```powershell
hostname
whoami
Get-Process | Select-Object -First 10
```

- **لقطة مطلوبة**: E6 (نافذة PowerShell تُظهر `hostname`)

#### 4.5.2 التأكد من ظهور Events في الـ Dashboard
من الـ Dashboard:
- افتح صفحة **Events**
- اجعل الزمن: آخر 15 دقيقة
- فلتر Endpoint = نفس الجهاز الذي نفذت عليه الأوامر

- **معيار النجاح**:
  - ظهور Events مرتبطة بالنشاط (مثل تشغيل `powershell.exe`/عمليات/شبكة… حسب collectors)
  - timestamps قريبة من وقت التنفيذ

- **لقطة مطلوبة**: E7

---

### 4.6 التحقق من Sigma Engine (Rules + Alerts)
#### 4.6.1 Rules
من الـ Dashboard:
- افتح صفحة **Rules**

- **معيار النجاح**: وجود قواعد محمّلة (قائمة/عدد) وقابلة للتفعيل/التعطيل حسب الصلاحيات
- **لقطة مطلوبة**: E8

#### 4.6.2 Alerts
من الـ Dashboard:
- افتح صفحة **Alerts**
- اضبط الزمن: آخر 24 ساعة (أو آخر ساعة أثناء الاختبار)

- **معيار النجاح (ممتاز)**: ظهور Alert مرتبط بأحد الأجهزة الثلاثة
- **بديل مقبول**: عدم وجود Alerts لكن إثبات أن:
  - Events تصل (E7)
  - Rules محمّلة (E8)
  - Sigma Engine يعمل (من health/الواجهة)

- **لقطة مطلوبة**: E9

---

### 4.7 (اختياري قوي) التحقق من Response Action (Command)
> هذا القسم يثبت أن المنصة لا تكتفي بالكشف، بل تنفّذ استجابة على الجهاز.

على جهاز Windows:
1) شغّل `notepad.exe`
2) من Endpoint detail نفّذ أمر (إن متاح) مثل:
   - `kill_process` / `terminate_process`

- **معيار النجاح**: حالة الأمر تتحول إلى completed والـ notepad يغلق فعليًا
- **لقطة مطلوبة**: E10

---

## 5) النتائج (Results)

املأ بعد التجربة:

- **عدد الأجهزة المرصودة**: ___ / 3
- **عدد Events المستلمة آخر 15 دقيقة** (تقريبي): ___
- **وجود Alerts**: نعم / لا
- **تم تنفيذ Response Action**: نعم / لا

---

## 6) ملاحظات ومشاكل واجهت الاختبار (Issues / Gaps)

اكتب أي شيء ظهر أثناء الاختبار:
- Agent يظهر offline رغم أنه شغال
- Events لا تظهر (telemetry gap)
- Alerts لا تظهر رغم وجود events (rules mismatch/disabled)
- صلاحيات RBAC تمنع الوصول لصفحات معينة (403)

---

## 7) الخلاصة (Conclusion)

**الحكم النهائي على تحقيق هدف المشروع كـ EDR**: *(ناجح/جزئي/غير ناجح)*  
**الدليل الأقوى**: *(اذكر E4 + E7 + E9/E10)*  
**توصيات قصيرة**:
- **إعداد سيناريو اختبار “مضمون” لتوليد Alert (Rule-based)**:
  - اختر جهاز واحد للاختبار (يفضّل `win-edr-lab-eu-central-1`) ونفّذ نشاطًا “غير ضار” لكن شائع أن ترصده قواعد Windows/Sigma.
  - نفّذ على الجهاز (PowerShell):

    ```powershell
    Start-Process notepad.exe
    powershell -NoProfile -WindowStyle Hidden -Command "Start-Sleep 2"
    ```

  - ثم راقب في الـ Dashboard:
    - **Events**: تأكد أن أحداث `powershell.exe` ظهرت (E7).
    - **Alerts**: راقب آخر 15–60 دقيقة لظهور Alert جديد (E9).
  - إذا لم يظهر Alert رغم وصول Events: اعتبرها إشارة أن القواعد الحالية لا تطابق نشاط الاختبار—انتقل للتوصية التالية.

- **تفعيل/ضبط مجموعة Rules مناسبة لبيئة Windows Lab**:
  - من صفحة **Rules** (E8) فعّل/أكد تفعيل قواعد Windows الأساسية ذات العلاقة بـ:
    - تشغيل PowerShell بخصائص مريبة (hidden/noProfile/encoded…)
    - Process creation / suspicious parent-child lineage
    - Network + DNS patterns (حسب collectors المفعّلة)
  - وثّق أي تغيير قمت به في القواعد (اسم القاعدة + enabled/disabled) داخل قسم “ملاحظات” مع لقطة شاشة إضافية عند الحاجة.

- **تعزيز دليل الـ EDR عبر Response Action واحد على الأقل (مستحسن)**:
  - نفّذ أمر من Endpoint detail مثل `terminate_process` لإغلاق `notepad.exe` واحتفظ بدليل قبل/بعد (E10).
  - هذا يرفع قوة التقرير لأنه يثبت “Detection + Response” وليس فقط “Monitoring”.

---

## 8) ملحق: سيناريوهات اختبار واقعية (Realistic Test Cases)

> ملاحظتان مهمتان:
> - هذه السيناريوهات **غير ضارة** (لا تحتوي malware)، لكنها “سلوكيات” شائعة في الاختبارات الأمنية وقد تطابق قواعد Sigma في بيئات Windows.
> - الهدف هو إنتاج **Events مؤكدة** + محاولة إنتاج **Alerts**. إذا لم تظهر Alerts فوثّق ذلك واذكر أن السبب غالبًا “rules mismatch/disabled” مع بقاء ingestion مثبتًا.

### TC-01: تشغيل PowerShell بخصائص مريبة (Hidden/NoProfile)
**الهدف**: إثبات telemetry لعمليات PowerShell وربما تنبيه “Suspicious PowerShell”.

**الخطوات (على جهاز Windows)**:

```powershell
hostname
Start-Process notepad.exe
powershell -NoProfile -WindowStyle Hidden -Command "Start-Sleep 2"
```

**المتوقع في Events**:
- Event عن إنشاء عملية `powershell.exe` (وأحيانًا command line)
- Event عن `notepad.exe`

**المتوقع في Alerts (إن كانت القواعد مفعّلة)**:
- Alert متعلق بـ PowerShell “hidden/noProfile” أو سلوك مشابه.

**اللقطات المطلوبة**:
- E6 (PowerShell)
- E7 (Events)
- E9 (Alerts إن ظهرت)

---

### TC-02: تنزيل ملف عبر PowerShell (WebRequest) إلى مجلد Temp
**الهدف**: سلوك شائع جدًا في الهجمات/الـ living-off-the-land، وقد يطابق قواعد تنزيل عبر PowerShell.

**الخطوات (على جهاز Windows)**:
> استخدم رابطًا آمنًا (ملف نصي) حتى لا يعتبر محتوى ضار.

```powershell
$out = "$env:TEMP\edr-test-download.txt"
Invoke-WebRequest -Uri "https://example.com/" -OutFile $out
Get-Item $out | Select-Object FullName,Length,LastWriteTime
```

**المتوقع في Events**:
- Process event لـ `powershell.exe`
- Network/DNS events (حسب collectors)
- File write event لمسار `%TEMP%`

**المتوقع في Alerts (إن كانت القواعد مفعّلة)**:
- Alert عن “PowerShell download / IWR” أو “Suspicious WebRequest”.

**اللقطات المطلوبة**:
- E6 (PowerShell)
- E7 (Events)
- E9 (Alerts إن ظهرت)

---

### TC-03: فك ضغط أرشيف وتشغيل ملف منه (Archive → Execute)
**الهدف**: سلسلة سلوك واقعية (تنزيل/فك/تشغيل) بدون أي malware.

**الخطوات (على جهاز Windows)**:

```powershell
$zip = "$env:TEMP\edr-zip-test.zip"
$dir = "$env:TEMP\edr-zip-test"
Invoke-WebRequest -Uri "https://github.com/github/training-kit/archive/refs/heads/main.zip" -OutFile $zip
Remove-Item -Recurse -Force $dir -ErrorAction SilentlyContinue
Expand-Archive -Path $zip -DestinationPath $dir -Force
Get-ChildItem $dir -Recurse | Select-Object -First 5 FullName
```

**المتوقع في Events**:
- Download + file write للـ zip
- Expand/archive activity (file creation كثيرة)

**المتوقع في Alerts**:
- ممكن Alert “Archive dropped many files” أو “Suspicious unzip” (قد لا تظهر حسب rules).

**اللقطات المطلوبة**:
- E6
- E7
- E9 (إن ظهرت)

---

### TC-04: اختبار اتصال خارجي + DNS (Network/DNS visibility)
**الهدف**: إثبات أن collectors الخاصة بالشبكة/DNS فعّالة.

**الخطوات (على جهاز Windows)**:

```powershell
Resolve-DnsName example.com
Test-NetConnection example.com -Port 443
```

**المتوقع في Events**:
- DNS query events (إذا `dns_enabled: true`)
- Network connection events (إذا `network_enabled: true`)

**اللقطات المطلوبة**:
- E6
- E7

---

### TC-05 (اختياري قوي): Response Action — إنهاء notepad من الـ Dashboard
**الهدف**: إثبات الاستجابة (Response) عبر C2 commands.

**الخطوات**:
1) على الجهاز شغّل `notepad.exe`.
2) من Endpoint detail في الـ Dashboard نفّذ أمر:
   - `terminate_process` / `kill_process` مستهدفًا notepad (حسب واجهة المنصة).
3) راقب “Action Center / Commands” حتى تصبح الحالة `completed`.

**النجاح**:
- notepad يغلق فعليًا + command status completed

**اللقطات المطلوبة**:
- E10


