# 🚀 دليل تشغيل منصة EDR - Production Mode

## المتطلبات الأساسية

| الأداة | الإصدار المطلوب | التحقق |
|--------|----------------|--------|
| Docker Desktop | أي إصدار حديث | `docker --version` |
| Go | 1.21+ | `go version` |
| Node.js | 20+ | `node --version` |
| Git | أي إصدار | `git --version` |

---

## 🔷 الخطوة 1: تشغيل البنية التحتية (Docker)

```powershell
# افتح PowerShell كـ Administrator
cd d:\EDR_Server\connection-manager

# تشغيل PostgreSQL, Redis, Kafka, Zookeeper
docker-compose up -d

# انتظر 30 ثانية للتأكد من بدء الخدمات
Start-Sleep -Seconds 30

# تحقق من الحالة
docker-compose ps
```

**النتيجة المتوقعة:**
```
NAME                    STATUS    PORTS
connection-manager-db-1       Up    0.0.0.0:5432->5432/tcp
connection-manager-redis-1    Up    0.0.0.0:6379->6379/tcp
connection-manager-kafka-1    Up    0.0.0.0:29092->29092/tcp
connection-manager-zookeeper-1 Up   0.0.0.0:2181->2181/tcp
```

---

## 🔷 الخطوة 2: تهيئة قاعدة البيانات

> **ملاحظة:** إذا كنت تستخدم `docker-compose` من مجلد connection-manager مع volume الـ migrations، أو الـ stack الكامل من الجذر (`docker-compose` في EDR_Server)، فإن الجداول تُنشأ تلقائياً ويمكنك تخطي هذه الخطوة.

```powershell
# إنشاء الجداول اللازمة (فقط عند تشغيل Postgres بدون migrations)
docker exec -it connection-manager-db-1 psql -U edr -d edr -c "
CREATE TABLE IF NOT EXISTS agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hostname VARCHAR(255) NOT NULL,
    ip_address VARCHAR(45),
    os_type VARCHAR(50),
    os_version VARCHAR(100),
    agent_version VARCHAR(50),
    status VARCHAR(20) DEFAULT 'pending',
    last_heartbeat TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id VARCHAR(255),
    rule_id VARCHAR(255),
    rule_title VARCHAR(500),
    severity VARCHAR(20),
    category VARCHAR(100),
    status VARCHAR(20) DEFAULT 'open',
    confidence FLOAT DEFAULT 0,
    event_count INT DEFAULT 1,
    matched_fields JSONB,
    mitre_tactics TEXT[],
    mitre_techniques TEXT[],
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS commands (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID REFERENCES agents(id),
    command_type VARCHAR(50),
    parameters JSONB,
    status VARCHAR(20) DEFAULT 'pending',
    result TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    executed_at TIMESTAMP
);
"
```

---

## 🔷 الخطوة 3: تشغيل Connection Manager

```powershell
# افتح PowerShell جديد (Terminal 2)
cd d:\EDR_Server\connection-manager

# تعيين متغيرات البيئة
$env:DATABASE_URL = "postgres://edr:edr@localhost:5432/edr?sslmode=disable"
$env:REDIS_ADDR = "localhost:6379"
$env:KAFKA_BROKERS = "localhost:29092"
$env:HTTP_PORT = "8082"
$env:GRPC_PORT = "50051"

# بناء وتشغيل
go build -o bin/connection-manager.exe ./cmd/server
.\bin\connection-manager.exe
```

**النتيجة المتوقعة:**
```
INFO  Starting Connection Manager...
INFO  HTTP server listening on :8082
INFO  gRPC server listening on :50051
INFO  Connected to PostgreSQL
INFO  Connected to Redis
INFO  Connected to Kafka
```

---

## 🔷 الخطوة 4: تشغيل Sigma Engine (HTTP API Mode)

```powershell
# افتح PowerShell جديد (Terminal 3)
cd d:\EDR_Server\sigma_engine_go

# بناء API server
go build -o bin/sigma-engine-api.exe ./cmd/sigma-engine

# تعيين المتغيرات وتشغيل
$env:API_PORT = "8080"
$env:RULES_DIR = "rules"
$env:DATABASE_URL = "postgres://edr:edr@localhost:5432/edr?sslmode=disable"

.\bin\sigma-engine-api.exe
```

> ⚠️ **ملاحظة**: إذا لم يوجد HTTP API mode، استخدم Mock API:
```powershell
cd d:\EDR_Server\dashboard
node mock-api/server.js
```

---

## 🔷 الخطوة 5: تشغيل Dashboard (Production Build)

```powershell
# افتح PowerShell جديد (Terminal 4)
cd d:\EDR_Server\dashboard

# بناء production
npm run build

# تشغيل بـ preview server
npm run preview
```

**أو للتطوير:**
```powershell
npm run dev
```

---

## 🔷 الخطوة 6: التحقق من الخدمات

### تحقق من Connection Manager:
```powershell
# Health check
curl http://localhost:8082/healthz

# قائمة Agents
curl http://localhost:8082/api/v1/agents
```

### تحقق من Sigma Engine API:
```powershell
# Health check
curl http://localhost:8080/health

# قائمة Alerts
curl http://localhost:8080/api/v1/sigma/alerts

# الإحصائيات
curl http://localhost:8080/api/v1/sigma/stats
```

### تحقق من Dashboard:
افتح المتصفح: **http://localhost:5173** (dev) أو **http://localhost:4173** (preview)

---

## 🔷 الخطوة 7: إضافة بيانات تجريبية

### إضافة Agent وهمي:
```powershell
$body = @{
    hostname = "WORKSTATION-001"
    ip_address = "192.168.1.100"
    os_type = "windows"
    os_version = "Windows 11 Pro"
    agent_version = "1.0.0"
    status = "online"
} | ConvertTo-Json

Invoke-RestMethod -Uri "http://localhost:8082/api/v1/agents" -Method POST -Body $body -ContentType "application/json"
```

### إرسال أحداث تجريبية للـ Sigma Engine:
```powershell
# حدث مشبوه - Process Creation
$event = @{
    timestamp = (Get-Date).ToString("o")
    agent_id = "agent-001"
    event_type = "process_creation"
    data = @{
        Image = "C:\Windows\System32\cmd.exe"
        CommandLine = "powershell.exe -encodedCommand JAB..."
        ParentImage = "C:\Windows\explorer.exe"
        User = "SYSTEM"
    }
} | ConvertTo-Json -Depth 10

Invoke-RestMethod -Uri "http://localhost:8080/api/v1/events" -Method POST -Body $event -ContentType "application/json"
```

---

## 🔷 الخطوة 8: اختبار السيناريوهات

### سيناريو 1: Credential Dumping
```powershell
$event = @{
    timestamp = (Get-Date).ToString("o")
    agent_id = "agent-001"
    event_type = "process_access"
    data = @{
        SourceImage = "C:\Tools\mimikatz.exe"
        TargetImage = "C:\Windows\System32\lsass.exe"
        GrantedAccess = "0x1010"
    }
} | ConvertTo-Json -Depth 10

Invoke-RestMethod -Uri "http://localhost:8080/api/v1/events" -Method POST -Body $event -ContentType "application/json"
```

### سيناريو 2: Ransomware Activity
```powershell
$event = @{
    timestamp = (Get-Date).ToString("o")
    agent_id = "agent-002"
    event_type = "file_event"
    data = @{
        TargetFilename = "C:\Users\Documents\important.docx.encrypted"
        Image = "C:\Temp\malware.exe"
        EventType = "FileCreate"
    }
} | ConvertTo-Json -Depth 10

Invoke-RestMethod -Uri "http://localhost:8080/api/v1/events" -Method POST -Body $event -ContentType "application/json"
```

---

## 📊 المنافذ المستخدمة

| الخدمة | المنفذ | البروتوكول |
|--------|--------|-----------|
| PostgreSQL | 5432 | TCP |
| Redis | 6379 | TCP |
| Kafka | 29092 | TCP |
| Zookeeper | 2181 | TCP |
| Kafka UI | 8081 | HTTP |
| Connection Manager HTTP | 8082 | HTTP |
| Connection Manager gRPC | 50051 | gRPC |
| Sigma Engine API | 8080 | HTTP |
| Dashboard (dev) | 5173 | HTTP |
| Dashboard (preview) | 4173 | HTTP |

---

## 🛑 إيقاف الخدمات

```powershell
# إيقاف Docker containers
cd d:\EDR_Server\connection-manager
docker-compose down

# إيقاف Go processes (في كل terminal: Ctrl+C)
# إيقاف Node processes (في كل terminal: Ctrl+C)
```

---

## ⚠️ استكشاف الأخطاء

### خطأ: Port already in use
```powershell
# البحث عن العملية التي تستخدم المنفذ
netstat -ano | findstr :8080

# إيقاف العملية
taskkill /PID <PID> /F
```

### خطأ: Docker containers not starting
```powershell
# إعادة تشغيل Docker Desktop
# أو
docker-compose down -v
docker-compose up -d
```

### خطأ: Cannot connect to database
```powershell
# تحقق من تشغيل PostgreSQL
docker logs connection-manager-db-1

# تحقق من الاتصال
docker exec -it connection-manager-db-1 psql -U edr -d edr -c "\dt"
```

---

## ✅ قائمة التحقق النهائية

- [ ] Docker containers تعمل (docker-compose ps)
- [ ] قاعدة البيانات جاهزة (الجداول موجودة)
- [ ] Connection Manager يعمل على 8082
- [ ] Sigma Engine/Mock API يعمل على 8080
- [ ] Dashboard يعمل على 5173
- [ ] يمكن تسجيل الدخول للـ Dashboard
- [ ] صفحة Alerts تعرض البيانات
- [ ] صفحة Dashboard تعرض الإحصائيات
