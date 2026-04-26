<div dir="rtl">

## 8) ملحق: سيناريوهات اختبار متقدمة (Advanced Realistic Test Cases)

> ملاحظتان مهمتان:  
> - كل السيناريوهات أدناه **غير ضارة** (لا تحتوي malware)، لكنها تمثّل **سلوكيات حقيقية** شائعة في الهجمات (PowerShell مشبوه، LOLBins، persistence، lateral، ransomware-like). [github](https://github.com/gl0bal01/intel-codex/blob/main/Security/Pentesting/sop-detection-evasion-testing.md)
> - الهدف هو إنتاج **Events مؤكدة** + محاولة توليد **Alerts** (حسب قواعد Sigma/Rules المفعّلة). إذا لم تظهر Alerts فوثّق ذلك واذكر أن السبب غالبًا **rules mismatch/disabled** مع استمرار الـ ingestion بشكل سليم. [docs.limacharlie](https://docs.limacharlie.io/3-detection-response/tutorials/dr-rule-building-guidebook/)

***

### TC-06: PowerShell EncodedCommand (سلوك Red Team شائع)

**الهدف**: إثبات أن المنصة ترصد استخدام `-EncodedCommand` في PowerShell حتى لو كان الكود نفسه غير ضار.

**الخطوات (على جهاز Windows)**:

```powershell
$cmd   = 'Write-Output "EDR Test - EncodedCommand"'
$bytes = [System.Text.Encoding]::Unicode.GetBytes($cmd)
$base64 = [Convert]::ToBase64String($bytes)

powershell.exe -NoLogo -NoProfile -WindowStyle Hidden -EncodedCommand $base64
```

**المتوقع في Events**:
- Event عن إنشاء عملية `powershell.exe` مع command line يحتوي `-EncodedCommand`.  
- Parent process مناسب (explorer.exe أو powershell.exe أخرى) حسب سياق التشغيل. [docs.stellarcyber](https://docs.stellarcyber.ai/6.3.x/Using/ML/Alert-Rule-Based-Process-Creation_CLI.htm)

**المتوقع في Alerts (إن كانت القواعد مفعّلة)**:
- Alert عن “Encoded PowerShell command” أو “Suspicious PowerShell CommandLine”.

**اللقطات المطلوبة**:
- لقطة PowerShell على الوكيل (مثال: E11).  
- لقطة Events مفلترة على نفس الجهاز و`process_name = powershell.exe` (مثال: E12).  
- لقطة Alert إن ظهرت (مثال: E13).

***

### TC-07: تنزيل ملف عبر LOLBin `certutil.exe`

**الهدف**: اختبار قدرة المنصة على رصد استخدام أدوات النظام (LOLBins) لتنزيل ملفات، وهي تقنية شائعة في الهجمات لتجاوز السياسات البسيطة. [deepstrike](https://deepstrike.io/blog/what-is-living-off-the-land-binaries-lolbins)

**الخطوات (على جهاز Windows)**:

```powershell
$dest = "$env:TEMP\edr-certutil-test.txt"
certutil.exe -urlcache -split -f "https://example.com/" $dest
Get-Item $dest | Select FullName,Length,LastWriteTime
```

**المتوقع في Events**:
- Process event لـ `certutil.exe` مع arguments مثل `-urlcache -split -f`.  
- File write event في `%TEMP%` (إن كان File telemetry مفعّل).

**المتوقع في Alerts (إن كانت القواعد مفعّلة)**:
- Alert عن “Suspicious certutil download” أو Rule مشابه لـ LOLBins usage.

**اللقطات المطلوبة**:
- لقطة PowerShell (مثال: E14).  
- لقطة Events تظهر `certutil.exe` مع الـ command line (مثال: E15).  
- لقطة Alert (إن ظهرت) (مثال: E16).

***

### TC-08: إنشاء Persistence عبر Registry Run Key

**الهدف**: إثبات أن المنصة تلتقط محاولات **Persistence** عبر مفاتيح التشغيل التلقائي (autorun) في الـ Registry.

**الخطوات (على جهاز Windows)**:

```powershell
$regPath = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run"
New-ItemProperty -Path $regPath -Name "EDRTestPersist" -Value "notepad.exe" -PropertyType String -Force
Get-ItemProperty -Path $regPath | Select EDRTestPersist
```

**المتوقع في Events**:
- Registry modification event لمسار `...CurrentVersion\Run` مع قيمة جديدة باسم `EDRTestPersist`. [ibm](https://www.ibm.com/support/pages/qradar-edr-formerly-reaqta-troubleshooting-registration-errors-occur-during-client-installation)

**المتوقع في Alerts (إن كانت القواعد مفعّلة)**:
- Alert عن “New autorun registry entry” أو “Persistence via Run key”.

**اللقطات المطلوبة**:
- لقطة PowerShell تظهر القيمة (مثال: E17).  
- لقطة Events تبين تعديل الـ Registry (مثال: E18).  
- لقطة Alert (إن ظهرت) (مثال: E19).

> يفضّل بعد الاختبار حذف القيمة لإرجاع الجهاز لحالته الأصلية:
> ```powershell
> Remove-ItemProperty -Path $regPath -Name "EDRTestPersist" -ErrorAction SilentlyContinue
> ```

***

### TC-09: نشاط شبيه بـ Lateral Movement (إنشاء Local Admin + RDP)

**الهدف**: محاكاة جزء من سيناريو حركة جانبية (lateral) عبر إنشاء مستخدم محلي له صلاحيات Administrators ثم محاولة RDP.

**الخطوات**:

1) على جهاز Windows الأول (مثلاً Win10-Agent1):

```powershell
net user edrtest P@ssw0rd! /add
net localgroup Administrators edrtest /add
```

2) من جهاز Windows آخر (مثلاً Win10-Agent2):

- افتح RDP إلى Win10-Agent1 وحاول تسجيل الدخول بالمستخدم `edrtest`.

**المتوقع في Events**:
- على الجهاز الهدف (Agent1):
  - Events إنشاء مستخدم جديد (إن كان جمع Security logs/LSA مفعّل).  
  - Logon events لـ RDP (نوع تسجيل دخول Network/RemoteInteractive). [techdocs.broadcom](https://techdocs.broadcom.com/us/en/symantec-security-software/endpoint-security-and-management/endpoint-protection/all/symantec-endpoint-protection-client-for-windows-help/Dialog-Overview-Client/cs-help-troubleshooting-and-client-mgmt-settings-c-v23492611-d13e4507/troubleshooting-edr-connection-status-v115547620-d13e5790.html)

**المتوقع في Alerts (إن كانت القواعد مفعّلة)**:
- Alert عن “New local admin account created”.  
- أو Alert عن “Suspicious RDP logon / possible lateral movement”.

**اللقطات المطلوبة**:
- لقطة PowerShell لإنشاء المستخدم (مثال: E20).  
- لقطة Events تظهر user creation و RDP logon (مثال: E21).  
- لقطة Alert (إن ظهرت) (مثال: E22).

> بعد الاختبار يفضّل حذف المستخدم:
> ```powershell
> net user edrtest /delete
> ```

***

### TC-10: سلوك شبيه بـ Ransomware (Mass File Writes) بدون تشفير

**الهدف**: اختبار كيفية تعامل المنصة مع **Burst كبير من تعديلات الملفات** في مسار واحد يشبه تصرّف ransomware، لكن بمحتوى نصّي سليم. [infosecwriteups](https://infosecwriteups.com/from-threat-intelligence-to-detection-a-practitioners-guide-2d930b168426)

**الخطوات (على جهاز Windows — في مجلد اختبار فقط)**:

```powershell
$root = "C:\EDR-Ransomware-Sim"
Remove-Item -Recurse -Force $root -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Path $root | Out-Null

1..200 | ForEach-Object {
    $path = Join-Path $root ("file_{0}.txt" -f $_)
    "EDR test content $_" * 50 | Out-File -FilePath $path -Encoding ASCII
}
```

**المتوقع في Events**:
- عدد كبير من file write events في مجلد واحد خلال فترة زمنية قصيرة.

**المتوقع في Alerts (إن كانت القواعد مفعّلة)**:
- Alert عن “Ransomware-like behavior” أو “Mass file modifications في زمن قصير”.

**اللقطات المطلوبة**:
- لقطة PowerShell (مثال: E23).  
- لقطة Events تبين spike في activity على المسار `C:\EDR-Ransomware-Sim` (مثال: E24).  

> بعد الاختبار يمكنك حذف المجلد بالكامل:
> ```powershell
> Remove-Item -Recurse -Force "C:\EDR-Ransomware-Sim"
> ```

***

### TC-R1: عزل الشبكة (Network Isolation) كاستجابة متقدمة

**الهدف**: التأكد أن المنصة تستطيع **عزل endpoint شبكيًا** مع الحفاظ على قناة التحكم (C2) مع الـ agent. [securview](https://www.securview.com/ai-security-essentials/endpoint-quarantine)

**الخطوات**:

1) من الـ Dashboard:
   - اختر Endpoint مستهدف (مثلاً Win10-Agent1).  
   - نفّذ Action: **Isolate network** (أو ما يعادله في الواجهة).

2) من داخل الجهاز المعزول (RDP/Console):

```powershell
Test-NetConnection google.com -Port 443
Resolve-DnsName example.com
```

**المتوقع**:
- فشل الاتصالات الخارجية (HTTPS/DNS) حسب تصميم العزل.  
- استمرار قدرة الـ agent على الاتصال بسيرفر EDR (لإرسال heartbeat/commands).

**اللقطات المطلوبة**:
- لقطة Action isolation في الـ Dashboard (مثال: E25).  
- لقطة PowerShell تظهر فشل الاتصال الخارجي (مثال: E26).

***

### TC-R2: حجر ملف (File Quarantine) غير ضار

**الهدف**: إثبات أن ميزة **Quarantine** تعمل وتؤثر على الملفات في الـ endpoint. [manageengine](https://www.manageengine.com/products/desktop-central/help/edr/quarantine-infected-devices.html)

**الخطوات**:

1) على جهاز Windows مستهدف (مثلاً Win10-Agent2):

```powershell
$path = "C:\Users\Public\edr-test-file.txt"
"EDR test file" | Out-File -FilePath $path -Encoding ASCII
Get-Item $path
```

2) من الـ Dashboard:
   - افتح Endpoint نفسه.  
   - نفّذ Action: **quarantine_file** على المسار `C:\Users\Public\edr-test-file.txt`.

3) بعد تنفيذ الأمر، على نفس الجهاز:

```powershell
Test-Path $path
```

**المتوقع**:
- الملف لم يعد موجودًا في المسار الأصلي (أو أصبح غير قابل للوصول).  
- ظهور سجل في صفحة/جدول Quarantine في المنصة. [securview](https://www.securview.com/ai-security-essentials/endpoint-quarantine)

**اللقطات المطلوبة**:
- قبل الحجر: الملف موجود في PowerShell/Explorer (مثال: E27).  
- بعد الحجر: `Test-Path` = False + صفحة Quarantine في الـ Dashboard (مثال: E28).

***

### TC-R3: إلغاء تثبيت الوكيل عن بُعد (Remote Uninstall)

**الهدف**: التأكد من أن المنصة تستطيع **إزالة الوكيل** بشكل آمن ومراقب من Endpoint محدد.

**الخطوات**:

1) من الـ Dashboard:
   - اختر Endpoint معين (مثلاً Win10-Agent3).  
   - نفّذ Action: `uninstall_agent`.

2) راقب:

   - في الـ Dashboard: حالة الـ command حتى تصبح `completed`.  
   - على الجهاز:

```powershell
sc query EDRAgent
Test-Path "C:\ProgramData\EDR"
```

**المتوقع**:
- خدمة `EDRAgent` يتم إيقافها ثم إزالتها (sc query لا يجد الخدمة).  
- مجلدات البرنامج (`C:\ProgramData\EDR` وغيرها) تُحذف أو تُنظف حسب الكود.  
- الـ Endpoint يظهر كـ “uninstalled” أو حالة مناسبة في الواجهة. [ibm](https://www.ibm.com/support/pages/qradar-edr-formerly-reaqta-troubleshooting-registration-errors-occur-during-client-installation)

**اللقطات المطلوبة**:
- لقطة حالة الـ command في الـ Dashboard.  
- لقطة PowerShell تبين اختفاء الخدمة والمجلد (Evidence جديد مثل E29).

***

### TC-S1: انقطاع مؤقت في سيرفر EDR (Resilience)

**الهدف**: اختبار **قدرة الوكيل على التعامل مع انقطاع السيرفر** ثم استعادة الاتصال بدون تدخل يدوي. [forums.docker](https://forums.docker.com/t/docker-desktop-must-restart-to-restore-communication/142896)

**الخطوات**:

1) على سيرفر EDR (Ubuntu):

```bash
cd ~/edr-platform
docker compose stop connection-manager
sleep 60
docker compose start connection-manager
```

2) على Agent (مثلاً Win10-Agent1):

- راقب `C:\ProgramData\EDR\logs\agent.log` خلال فترة التوقف وما بعدها.

**المتوقع**:
- ظهور رسائل retry/backoff بدون crash.  
- بعد عودة service `connection-manager` يعود الـ agent للاتصال وإرسال heartbeat/events تلقائيًا.

**اللقطات المطلوبة**:
- مقطع من agent.log قبل/بعد يوضح سلوك إعادة المحاولة.

***

### TC-S2: إعادة تشغيل خدمة الوكيل عدة مرات

**الهدف**: التحقق من **استقرار خدمة EDRAgent** تحت إعادة تشغيل متكررة.

**الخطوات (على كل Agent)**:

```powershell
for ($i=1; $i -le 3; $i++) {
    Restart-Service -Name EDRAgent -Force
    Start-Sleep -Seconds 5
}
```

**المتوقع**:
- الخدمة ترجع لحالة Running بعد كل إعادة تشغيل.  
- لا يحدث فقدان دائم في الاتصال بالـ Dashboard أو فساد في config/enrollment.

**اللقطات المطلوبة**:
- مقطع من `sc query EDRAgent` و/أو agent.log يثبت استقرار الخدمة.

***

### TC-S3: Burst Events (ضغط بسيط على الـ pipeline)

**الهدف**: رؤية تصرف المنصة عند **ارتفاع مفاجئ في عدد الـ Events** (process creation) من Endpoint واحد.

**الخطوات (على جهاز Windows)**:

```powershell
for ($i=1; $i -le 50; $i++) {
    Start-Process notepad.exe
}
Start-Sleep 5
Stop-Process -Name notepad -Force
```

**المتوقع في Events**:
- عدد كبير من events لإنشاء/إنهاء `notepad.exe` خلال فترة قصيرة.

**المتوقع في المنصة**:
- عدم وجود أخطاء كبيرة في connection-manager/Kafka/Postgres تحت هذا الحمل البسيط. [techdocs.broadcom](https://techdocs.broadcom.com/us/en/symantec-security-software/endpoint-security-and-management/endpoint-protection/all/symantec-endpoint-protection-client-for-windows-help/Dialog-Overview-Client/cs-help-troubleshooting-and-client-mgmt-settings-c-v23492611-d13e4507/troubleshooting-edr-connection-status-v115547620-d13e5790.html)

**اللقطات المطلوبة**:
- لقطة Events تبين الـ spike (يمكن استخدام نفس E23/E24 أو لقطات جديدة).

***

### TC-D1: استعلام تحليلي مركّب على الـ Events (Analytics)

**الهدف**: إثبات أن الـ SOC analyst يستطيع إجراء **pivots معقولة** على بيانات الـ EDR (مثل process, host, time). [docs.limacharlie](https://docs.limacharlie.io/3-detection-response/tutorials/dr-rule-building-guidebook/)

**الخطوات (من الـ Dashboard)**:

1) افتح صفحة **Events**.  
2) اضبط الفلاتر على سبيل المثال:
   - Host = `WIN10-AGENT1`.  
   - Process name = `powershell.exe`.  
   - Time range = آخر ساعة.

**المتوقع**:
- ظهور نتائج تمثل سيناريوهات مثل TC-01, TC-02, TC-06, TC-07.  
- زمن استجابة معقول للـ query.

**اللقطات المطلوبة**:
- لقطة لسطر الفلاتر + النتائج (Evidence جديد مثل E30).

***

### TC-D2: الانتقال من Alert إلى Events (Triage Flow)

**الهدف**: التحقق من أن المنصة تدعم **تتبّع (drill-down)** من Alert إلى Events المرتبطة به لعمل التحقيق (triage). [docs.limacharlie](https://docs.limacharlie.io/3-detection-response/tutorials/dr-rule-building-guidebook/)

**الخطوات**:

1) افتح Alert ناتج من أحد السيناريوهات السابقة (مثلاً TC-06 أو TC-07).  
2) استخدم خيار “View related events” أو ما يعادله في الواجهة.  

**المتوقع**:
- فتح صفحة Events بفلاتر auto-applied (agent_id / hostname / time range) تبين الأحداث المرتبطة بالـ Alert.  
- هذا يثبت أن المنصة تدعم مسار التحقيق الكامل: Alert → Context → Events خام.

**اللقطات المطلوبة**:
- لقطة لصفحة Alert.  
- لقطة لصفحة Events بعد الضغط على “related events”.

***

يمكنك الآن إضافة هذا القسم (8) كما هو إلى تقريرك، ثم ربط كل سيناريو بـ Evidence IDs التي تلتقطها أثناء الاختبار. بهذه الطريقة السيناريوهات تصبح **مرجع رسمي** و**قابلة لإعادة التنفيذ** في أي جولة تحقق مستقبلية.