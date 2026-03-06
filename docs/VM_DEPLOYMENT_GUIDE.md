# EDR Platform - VM Deployment Guide
# Agent على VM Windows 10 + Components على Host Windows 11

## 📋 المتطلبات

### على Host (Windows 11)
- Docker Desktop
- Go 1.24+
- Node.js 18+
- Git

### على VM (Windows 10)
- Go 1.24+ (لبناء الـ Agent)
- اتصال شبكة مع الـ Host

---

## 🔧 الخطوة 1: تحديد عناوين IP

### على Host (Windows 11):
```powershell
# احصل على IP الـ Host
ipconfig
# ابحث عن IPv4 Address (مثال: 192.168.1.100)
```

### على VM (Windows 10):
```powershell
# تأكد أن VM يمكنه ping الـ Host
ping 192.168.1.100
```

> ⚠️ **مهم:** استبدل `192.168.1.100` بعنوان IP الفعلي للـ Host

---

## 🐳 الخطوة 2: تشغيل Infrastructure على Host

### افتح PowerShell كـ Administrator على Host:

```powershell
# انتقل لمجلد Connection Manager
cd d:\EDR_Server\connection-manager

# شغّل البنية التحتية (PostgreSQL, Redis, Kafka, Zookeeper)
docker-compose -f docker-compose.infra.yml up -d

# تحقق من أن كل الخدمات تعمل
docker ps
```

### النتيجة المتوقعة:
```
CONTAINER ID   IMAGE                    STATUS    PORTS
xxx            postgres:15-alpine       Up        0.0.0.0:5432->5432/tcp
xxx            redis:7-alpine           Up        0.0.0.0:6379->6379/tcp
xxx            bitnami/kafka:3.6        Up        0.0.0.0:29092->29092/tcp
xxx            bitnami/zookeeper:3.8    Up        0.0.0.0:2181->2181/tcp
```

---

## 🔗 الخطوة 3: تشغيل Connection Manager على Host

### Terminal جديد على Host:

```powershell
cd d:\EDR_Server\connection-manager

# تعيين متغيرات البيئة
$env:DATABASE_URL = "postgres://postgres:postgres@localhost:5432/edr?sslmode=disable"
$env:REDIS_ADDR = "localhost:6379"
$env:KAFKA_BROKERS = "localhost:29092"
$env:HTTP_PORT = "8082"
$env:GRPC_PORT = "50051"

# بناء وتشغيل
go build -o bin/connection-manager.exe ./cmd/server
.\bin\connection-manager.exe
```

### النتيجة المتوقعة:
```
INFO: Starting Connection Manager...
INFO: HTTP API listening on :8082
INFO: gRPC server listening on :50051
INFO: Connected to PostgreSQL
INFO: Connected to Redis
INFO: Connected to Kafka
```

> ✅ **Connection Manager يعمل على:**
> - HTTP: `http://<HOST_IP>:8082`
> - gRPC: `<HOST_IP>:50051`

---

## 🎯 الخطوة 4: تشغيل Sigma Engine (أو Mock API) على Host

### الخيار A: Mock API (أسهل للاختبار)

```powershell
cd d:\EDR_Server\dashboard\mock-api

# تثبيت dependencies
npm install

# تشغيل
node server.js
```

### الخيار B: Sigma Engine الحقيقي

```powershell
cd d:\EDR_Server\sigma_engine_go

$env:DATABASE_URL = "postgres://postgres:postgres@localhost:5432/edr?sslmode=disable"
$env:KAFKA_BROKERS = "localhost:29092"
$env:API_PORT = "8080"

go build -o bin/sigma-engine.exe ./cmd/engine
.\bin\sigma-engine.exe api
```

> ✅ **Sigma Engine/Mock API يعمل على:** `http://<HOST_IP>:8080`

---

## 🖥️ الخطوة 5: تشغيل Dashboard على Host

### Terminal جديد:

```powershell
cd d:\EDR_Server\dashboard

# تثبيت dependencies
npm install

# تشغيل في وضع التطوير
npm run dev -- --host 0.0.0.0
```

### النتيجة المتوقعة:
```
  VITE v5.x.x  ready in xxx ms

  ➜  Local:   http://localhost:5173/
  ➜  Network: http://192.168.1.100:5173/
```

> ✅ **Dashboard متاح على:** `http://<HOST_IP>:5173`

---

## 🛡️ الخطوة 6: بناء Agent على VM

### على VM (Windows 10):

```powershell
# انسخ مجلد win_edrAgent إلى VM
# أو استخدم shared folder

cd C:\win_edrAgent  # أو المسار الذي نسخت إليه

# بناء الـ Agent
go build -o bin/agent.exe ./cmd/agent
```

---

## ⚙️ الخطوة 7: تكوين Agent للاتصال بـ Host

### أنشئ ملف `config.yaml` على VM:

```powershell
mkdir C:\ProgramData\EDR\config
notepad C:\ProgramData\EDR\config\config.yaml
```

### محتوى الملف (استبدل HOST_IP):

```yaml
server:
  # استبدل بعنوان IP الـ Host
  address: "192.168.1.100:50051"
  timeout: 30s
  reconnect_delay: 1s
  max_reconnect_delay: 30s
  heartbeat_interval: 30s

agent:
  id: ""
  hostname: ""
  batch_size: 50
  batch_interval: 1s
  buffer_size: 5000
  compression: "snappy"

collectors:
  etw_enabled: true
  wmi_enabled: true

logging:
  level: "DEBUG"
  file_path: "C:\\ProgramData\\EDR\\logs\\agent.log"
  max_size_mb: 100
  max_age_days: 7
```

---

## 🚀 الخطوة 8: تشغيل Agent على VM

```powershell
# تشغيل في وضع Debug
.\bin\agent.exe -debug -config "C:\ProgramData\EDR\config\config.yaml"
```

### النتيجة المتوقعة:
```
[2026-01-21 22:30:00] INFO: EDR Windows Agent v1.0.0
[2026-01-21 22:30:00] INFO: Agent ID: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
[2026-01-21 22:30:00] INFO: Hostname: WINDOWS10-VM
[2026-01-21 22:30:00] INFO: Connecting to server: 192.168.1.100:50051
[2026-01-21 22:30:01] INFO: Agent started successfully
```

---

## ✅ الخطوة 9: التحقق من النظام

### على Host - تحقق من Dashboard:

1. افتح المتصفح: `http://localhost:5173`
2. انتقل إلى صفحة **Endpoints**
3. يجب أن ترى الـ Agent من VM

### على Host - تحقق من Connection Manager:

```powershell
# قائمة الـ Agents المتصلين
curl http://localhost:8082/api/v1/agents
```

### على VM - تحقق من Logs:

```powershell
Get-Content C:\ProgramData\EDR\logs\agent.log -Tail 20
```

---

## 📊 ملخص المنافذ

| الخدمة | المنفذ | الموقع |
|--------|--------|--------|
| PostgreSQL | 5432 | Host (Docker) |
| Redis | 6379 | Host (Docker) |
| Kafka | 29092 | Host (Docker) |
| Zookeeper | 2181 | Host (Docker) |
| Connection Manager HTTP | 8082 | Host |
| Connection Manager gRPC | 50051 | Host |
| Sigma Engine / Mock API | 8080 | Host |
| Dashboard | 5173 | Host |
| Agent | - | VM (client) |

---

## 🔥 Windows Firewall

### على Host - افتح المنافذ:

```powershell
# افتح PowerShell كـ Administrator
netsh advfirewall firewall add rule name="EDR gRPC" dir=in action=allow protocol=tcp localport=50051
netsh advfirewall firewall add rule name="EDR HTTP" dir=in action=allow protocol=tcp localport=8082
netsh advfirewall firewall add rule name="EDR Sigma" dir=in action=allow protocol=tcp localport=8080
netsh advfirewall firewall add rule name="EDR Dashboard" dir=in action=allow protocol=tcp localport=5173
```

---

## 🛑 إيقاف النظام

### على VM:
```powershell
# Ctrl+C لإيقاف Agent
```

### على Host:
```powershell
# إيقاف كل العمليات (Ctrl+C في كل terminal)

# إيقاف Docker containers
cd d:\EDR_Server\connection-manager
docker-compose -f docker-compose.infra.yml down
```

---

## 🔧 استكشاف الأخطاء

### Agent لا يتصل بـ Connection Manager:

1. **تحقق من الـ IP:**
   ```powershell
   # من VM
   ping 192.168.1.100
   Test-NetConnection -ComputerName 192.168.1.100 -Port 50051
   ```

2. **تحقق من Firewall:**
   ```powershell
   # على Host
   netsh advfirewall firewall show rule name="EDR gRPC"
   ```

3. **تحقق من Connection Manager يعمل:**
   ```powershell
   # على Host
   curl http://localhost:8082/health
   ```

### Dashboard لا يظهر البيانات:

1. تحقق أن Mock API يعمل على port 8080
2. تحقق من Network tab في المتصفح
3. تحقق من CORS settings

---

## 📌 Quick Reference

```powershell
# === على HOST ===

# 1. Infrastructure
cd connection-manager
docker-compose -f docker-compose.infra.yml up -d

# 2. Connection Manager
.\bin\connection-manager.exe

# 3. Mock API
cd dashboard\mock-api
node server.js

# 4. Dashboard
cd dashboard
npm run dev -- --host 0.0.0.0

# === على VM ===

# 5. Agent
.\bin\agent.exe -debug
```
