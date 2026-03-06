# EDR Platform - End-to-End Testing Guide

اختبار شامل للنظام بدون Agent - محاكاة بيئة الإنتاج

---

## نظرة عامة على تدفق البيانات

```
[Test Events] → [Kafka/API] → [Sigma Engine] → [Alerts] → [Dashboard]
     ↑                              ↓
  نرسل أحداث              يكتشف التهديدات
   اختبارية              ويولد تنبيهات
```

---

## الخطوة 1: تشغيل جميع المكونات

### 1.1 البنية التحتية (Docker)

```powershell
cd d:\1-EDR-GRUD-PROJECT\EDR_Platform\EDR_Server\connection-manager
docker-compose up -d postgres redis zookeeper kafka kafka-init kafka-ui
```

انتظر 60 ثانية ثم تحقق:
```powershell
docker-compose ps
```

### 1.2 Sigma Engine (HTTP Mode)

```powershell
cd d:\1-EDR-GRUD-PROJECT\EDR_Platform\EDR_Server\sigma_engine_go
.\sigma-engine.exe
```

### 1.3 Dashboard

```powershell
cd d:\1-EDR-GRUD-PROJECT\EDR_Platform\EDR_Server\dashboard
npm run dev
```

---

## الخطوة 2: إرسال أحداث اختبارية

### الطريقة 1: عبر Sigma Engine API (الأسهل)

افتح PowerShell جديد وأرسل حدث مشبوه:

```powershell
# حدث 1: تنفيذ PowerShell مشبوه (يجب أن يولد تنبيه)
$event1 = @{
    EventType = "ProcessCreate"
    EventCode = 1
    Image = "C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe"
    CommandLine = "powershell.exe -enc UwB0AGEAcgB0AC0AUAByAG8AYwBlAHMAcwA="
    ParentImage = "C:\Windows\System32\cmd.exe"
    User = "SYSTEM"
    ProcessId = 1234
    Timestamp = (Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ")
} | ConvertTo-Json

Invoke-RestMethod -Uri "http://localhost:8080/api/v1/sigma/events" -Method POST -Body $event1 -ContentType "application/json"
```

```powershell
# حدث 2: محاولة dump لـ LSASS (تهديد عالي الخطورة)
$event2 = @{
    EventType = "ProcessAccess"
    EventCode = 10
    SourceImage = "C:\Temp\mimikatz.exe"
    TargetImage = "C:\Windows\System32\lsass.exe"
    GrantedAccess = "0x1410"
    User = "Administrator"
    ProcessId = 5678
    Timestamp = (Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ")
} | ConvertTo-Json

Invoke-RestMethod -Uri "http://localhost:8080/api/v1/sigma/events" -Method POST -Body $event2 -ContentType "application/json"
```

```powershell
# حدث 3: إنشاء scheduled task مشبوه
$event3 = @{
    EventType = "ProcessCreate"
    EventCode = 1
    Image = "C:\Windows\System32\schtasks.exe"
    CommandLine = "schtasks /create /tn backdoor /tr C:\temp\evil.exe /sc onlogon"
    ParentImage = "C:\Windows\System32\cmd.exe"
    User = "Administrator"
    ProcessId = 9012
    Timestamp = (Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ")
} | ConvertTo-Json

Invoke-RestMethod -Uri "http://localhost:8080/api/v1/sigma/events" -Method POST -Body $event3 -ContentType "application/json"
```

### الطريقة 2: عبر Kafka (محاكاة الإنتاج)

1. افتح Kafka UI: http://localhost:8081
2. اذهب إلى Topics → `events-raw`
3. اضغط "Produce Message"
4. أدخل JSON التالي:

```json
{
  "EventType": "ProcessCreate",
  "EventCode": 1,
  "Image": "C:\\Windows\\System32\\whoami.exe",
  "CommandLine": "whoami /all",
  "ParentImage": "C:\\Windows\\System32\\cmd.exe",
  "User": "Administrator",
  "ProcessId": 3456,
  "Timestamp": "2026-01-21T09:00:00Z"
}
```

---

## الخطوة 3: التحقق من النتائج

### 3.1 فحص Sigma Engine

```powershell
# عرض إحصائيات القواعد
Invoke-RestMethod -Uri "http://localhost:8080/api/v1/sigma/stats/rules" | ConvertTo-Json

# عرض التنبيهات الأخيرة
Invoke-RestMethod -Uri "http://localhost:8080/api/v1/sigma/alerts?limit=10" | ConvertTo-Json -Depth 5
```

### 3.2 فحص Dashboard

افتح المتصفح على: http://localhost:5173

يجب أن ترى:
- ✅ KPI cards تعرض إحصائيات
- ✅ Live Alerts feed
- ✅ Severity distribution chart

### 3.3 فحص Kafka (اختياري)

افتح http://localhost:8081 وتحقق من:
- Topic `events-raw`: الأحداث المرسلة
- Topic `alerts`: التنبيهات المولدة

---

## الخطوة 4: سيناريوهات اختبار شاملة

### سيناريو 1: هجوم Ransomware

```powershell
# محاكاة سلسلة هجوم ransomware
$ransomwareEvents = @(
    @{
        EventType = "ProcessCreate"
        EventCode = 1
        Image = "C:\Users\victim\Downloads\invoice.exe"
        CommandLine = "invoice.exe"
        ParentImage = "C:\Windows\explorer.exe"
        User = "victim"
        ProcessId = 1001
    },
    @{
        EventType = "ProcessCreate"
        EventCode = 1
        Image = "C:\Windows\System32\vssadmin.exe"
        CommandLine = "vssadmin delete shadows /all /quiet"
        ParentImage = "C:\Users\victim\Downloads\invoice.exe"
        User = "SYSTEM"
        ProcessId = 1002
    },
    @{
        EventType = "ProcessCreate"
        EventCode = 1
        Image = "C:\Windows\System32\bcdedit.exe"
        CommandLine = "bcdedit /set {default} recoveryenabled No"
        ParentImage = "C:\Users\victim\Downloads\invoice.exe"
        User = "SYSTEM"
        ProcessId = 1003
    }
)

foreach ($event in $ransomwareEvents) {
    $event.Timestamp = (Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ")
    $json = $event | ConvertTo-Json
    Invoke-RestMethod -Uri "http://localhost:8080/api/v1/sigma/events" -Method POST -Body $json -ContentType "application/json"
    Start-Sleep -Milliseconds 500
}

Write-Host "Ransomware attack simulation complete!" -ForegroundColor Red
```

### سيناريو 2: Credential Dumping

```powershell
$credDumpEvents = @(
    @{
        EventType = "ProcessCreate"
        EventCode = 1
        Image = "C:\Tools\procdump.exe"
        CommandLine = "procdump.exe -ma lsass.exe lsass.dmp"
        User = "Administrator"
        ProcessId = 2001
    },
    @{
        EventType = "FileCreate"
        EventCode = 11
        TargetFilename = "C:\Temp\lsass.dmp"
        Image = "C:\Tools\procdump.exe"
        User = "Administrator"
        ProcessId = 2001
    }
)

foreach ($event in $credDumpEvents) {
    $event.Timestamp = (Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ")
    $json = $event | ConvertTo-Json
    Invoke-RestMethod -Uri "http://localhost:8080/api/v1/sigma/events" -Method POST -Body $json -ContentType "application/json"
}

Write-Host "Credential dump simulation complete!" -ForegroundColor Yellow
```

### سيناريو 3: Lateral Movement

```powershell
$lateralEvents = @(
    @{
        EventType = "ProcessCreate"
        EventCode = 1
        Image = "C:\Windows\System32\net.exe"
        CommandLine = "net use \\\\192.168.1.100\\c$ /user:admin password123"
        User = "attacker"
        ProcessId = 3001
    },
    @{
        EventType = "ProcessCreate"
        EventCode = 1
        Image = "C:\Windows\System32\wmic.exe"
        CommandLine = "wmic /node:192.168.1.100 process call create 'cmd.exe /c whoami'"
        User = "attacker"
        ProcessId = 3002
    },
    @{
        EventType = "ProcessCreate"
        EventCode = 1
        Image = "C:\Windows\System32\psexec.exe"
        CommandLine = "psexec \\\\192.168.1.100 -u admin -p pass cmd.exe"
        User = "attacker"
        ProcessId = 3003
    }
)

foreach ($event in $lateralEvents) {
    $event.Timestamp = (Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ")
    $json = $event | ConvertTo-Json
    Invoke-RestMethod -Uri "http://localhost:8080/api/v1/sigma/events" -Method POST -Body $json -ContentType "application/json"
}

Write-Host "Lateral movement simulation complete!" -ForegroundColor Magenta
```

---

## الخطوة 5: التحقق النهائي

### قائمة الفحص

| المكون | الفحص | الحالة المتوقعة |
|--------|-------|-----------------|
| PostgreSQL | `docker-compose ps` | healthy |
| Redis | `docker-compose ps` | healthy |
| Kafka | http://localhost:8081 | Topics visible |
| Sigma Engine | http://localhost:8080/api/v1/sigma/stats/rules | JSON response |
| Dashboard | http://localhost:5173 | UI loads |
| Alerts | Dashboard → Alerts page | Shows test alerts |

### مثال على الإخراج المتوقع

```json
{
  "total_rules": 874,
  "active_rules": 874,
  "rules_by_severity": {
    "critical": 45,
    "high": 312,
    "medium": 517
  }
}
```

---

## استكشاف الأخطاء

### لا تظهر تنبيهات؟

1. تحقق أن Sigma Engine يعمل وأنه حمّل القواعد
2. تأكد من أن الأحداث تحتوي على `EventCode` و `EventType`
3. راجع سجلات Sigma Engine للتفاصيل

### Dashboard فارغ؟

1. تأكد من تشغيل Sigma Engine أولاً
2. أرسل بعض الأحداث الاختبارية
3. اضغط Refresh في المتصفح

### Kafka لا يعمل؟

```powershell
docker-compose down
docker-compose up -d postgres redis zookeeper kafka kafka-init
```

---

## ملخص

بعد تنفيذ هذا الدليل، تكون قد اختبرت:

✅ تدفق البيانات من الأحداث إلى التنبيهات
✅ محرك الكشف Sigma مع 874+ قاعدة
✅ واجهة Dashboard لعرض التنبيهات
✅ سيناريوهات هجوم متعددة (Ransomware, Credential Dump, Lateral Movement)

النظام جاهز للإنتاج! 🚀
