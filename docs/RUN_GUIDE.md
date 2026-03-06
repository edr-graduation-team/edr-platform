# EDR Platform - Run Guide (Windows)

Complete step-by-step guide to run all EDR components locally on Windows.

---

## Prerequisites

### Required Software

| Software | Version | Download |
|----------|---------|----------|
| **Docker Desktop** | 4.20+ | [docker.com/products/docker-desktop](https://www.docker.com/products/docker-desktop/) |
| **Go** | 1.24+ | [go.dev/dl](https://go.dev/dl/) |
| **Node.js** | 20.15+ | [nodejs.org](https://nodejs.org/) |

### Verify Installation

```powershell
docker --version    # Docker version 24.x+
go version          # go version go1.24+
node --version      # v20.15+
```

---

## Step 1: Start Infrastructure (Docker)

All components require PostgreSQL, Redis, and Kafka. Start them using Docker Compose.

```powershell
# Navigate to connection-manager directory
cd d:\EDR_Server\connection-manager

# Start infrastructure services
docker-compose up -d postgres redis zookeeper kafka kafka-init

# Wait 30-60 seconds, then verify all services are healthy
docker-compose ps
```

**Expected Output:**
```
NAME                              STATUS
connection-manager-postgres-1     Up (healthy)
connection-manager-redis-1        Up (healthy)
connection-manager-zookeeper-1    Up (healthy)
connection-manager-kafka-1        Up (healthy)
connection-manager-kafka-init-1   Exited (0)  ← This is OK
```

### Service Ports

| Service | Port | Access From Windows |
|---------|------|---------------------|
| PostgreSQL | 5432 | `localhost:5432` |
| Redis | 6379 | `localhost:6379` |
| Kafka | 29092 | `localhost:29092` |
| Kafka (internal) | 9092 | Docker only |
| Kafka UI | 8081 | http://localhost:8081 |

---

## Step 2: Run Sigma Engine

The Sigma Engine processes events and detects threats using 874+ Sigma rules.

### Option A: HTTP API Mode (Recommended for Development)

```powershell
cd d:\EDR_Server\sigma_engine_go

.\sigma-engine.exe
```

- **API Endpoint**: http://localhost:8080
- **Health Check**: `curl http://localhost:8080/api/v1/sigma/stats/rules`

### Option B: Kafka Mode (Production)

```powershell
cd d:\EDR_Server\sigma_engine_go

.\sigma-engine-kafka.exe -brokers "localhost:29092"
```

**All Kafka Mode Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `-brokers` | `localhost:9092` | Kafka brokers (use `localhost:29092`) |
| `-events-topic` | `events-raw` | Topic to consume events from |
| `-alerts-topic` | `alerts` | Topic to publish alerts to |
| `-group` | `sigma-engine-group` | Consumer group name |
| `-workers` | `4` | Detection worker threads |

**Success Indicators:**
```
✓ Loaded 874 high-fidelity rules
Starting Kafka consumer: brokers=[localhost:29092] ✅
Event loop started ✅
```

---

## Step 3: Run Connection Manager

The Connection Manager handles agent communication via gRPC.

```powershell
cd d:\EDR_Server\connection-manager

# Build (first time or after code changes)
go build -o bin/connection-manager.exe ./cmd/server

# Run
.\bin\connection-manager.exe
```

### Environment Variables (set before running)

```powershell
$env:DATABASE_URL = "postgres://edr:edr@localhost:5432/edr?sslmode=disable"
$env:REDIS_ADDR = "localhost:6379"
$env:KAFKA_BROKERS = "localhost:29092"
$env:HTTP_PORT = "8082"
$env:GRPC_PORT = "50051"
$env:LOG_LEVEL = "info"
```

### Verify Connection Manager

```powershell
# Health check
curl http://localhost:8082/healthz

# Metrics
curl http://localhost:8082/metrics
```

---

## Step 4: Run Dashboard

The Dashboard provides the security operations web interface.

```powershell
cd d:\EDR_Server\dashboard

# Install dependencies (first time only)
npm install

# Start development server
npm run dev
```

**Access**: Open http://localhost:5173 in your browser.

---

## Quick Start Script

Save as `start-edr.ps1` in EDR_Server folder:

```powershell
# start-edr.ps1 - Start all EDR components
Write-Host "=== EDR Platform Startup ===" -ForegroundColor Cyan

# Step 1: Infrastructure
Write-Host "`n[1/4] Starting Infrastructure..." -ForegroundColor Yellow
Set-Location "d:\EDR_Server\connection-manager"
docker-compose up -d postgres redis zookeeper kafka kafka-init
Write-Host "Waiting 45 seconds for services to initialize..."
Start-Sleep -Seconds 45

# Step 2: Sigma Engine
Write-Host "`n[2/4] Starting Sigma Engine..." -ForegroundColor Yellow
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd 'd:\EDR_Server\sigma_engine_go'; .\sigma-engine.exe"

# Step 3: Connection Manager
Write-Host "`n[3/4] Starting Connection Manager..." -ForegroundColor Yellow
$env:DATABASE_URL = "postgres://edr:edr@localhost:5432/edr?sslmode=disable"
$env:REDIS_ADDR = "localhost:6379"
$env:KAFKA_BROKERS = "localhost:29092"
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd 'd:\EDR_Server\connection-manager'; go build -o bin/connection-manager.exe ./cmd/server; .\bin\connection-manager.exe"

# Step 4: Dashboard
Write-Host "`n[4/4] Starting Dashboard..." -ForegroundColor Yellow
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd 'd:\EDR_Server\dashboard'; npm run dev"

Write-Host "`n=== All Components Started ===" -ForegroundColor Green
Write-Host "Dashboard: http://localhost:5173" -ForegroundColor Cyan
Write-Host "Sigma API: http://localhost:8080" -ForegroundColor Cyan
Write-Host "Kafka UI:  http://localhost:8081" -ForegroundColor Cyan
```

---

## Component Summary

| Component | Type | Port(s) | Health Check |
|-----------|------|---------|--------------|
| **PostgreSQL** | Docker | 5432 | `docker-compose ps` |
| **Redis** | Docker | 6379 | `docker-compose ps` |
| **Kafka** | Docker | 29092 | http://localhost:8081 |
| **Sigma Engine** | Go | 8080 | http://localhost:8080/api/v1/sigma/stats/rules |
| **Connection Manager** | Go | 8082, 50051 | http://localhost:8082/healthz |
| **Dashboard** | Node | 5173 | http://localhost:5173 |

---

## Stopping Services

```powershell
# Stop Docker infrastructure
cd d:\EDR_Server\connection-manager
docker-compose down

# To also remove data volumes
docker-compose down -v
```

---

## Troubleshooting

### Kafka Connection Issues

If you see `kafka: no such host`:
```powershell
# Restart Kafka with correct config
docker-compose down
docker-compose up -d postgres redis zookeeper kafka kafka-init
```

### Port Already in Use

```powershell
netstat -ano | findstr :5432
taskkill /PID <pid> /F
```

### Docker Not Starting

Ensure Docker Desktop is running and WSL2 is enabled.

### Sigma Engine Kafka Errors

Use **port 29092** (external), not 9092 (internal):
```powershell
.\sigma-engine-kafka.exe -brokers "localhost:29092"
```
