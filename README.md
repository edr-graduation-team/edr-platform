# EDR Platform — Deployment Runbook

> **Audience**: Graduation project team  
> **Prerequisites**: Docker Desktop, Go 1.24+, Git

---

## Architecture Overview

All backend services run in Docker. The Windows Agent runs natively on the host.

```
  Windows Agent (ETW + gRPC)
        │
        │  gRPC :47051
        ▼
  ┌─────────────────────────┐      ┌──────────────┐
  │   Connection Manager    │─────►│    Kafka      │
  │   gRPC :47051           │      │  :9092 / :29092
  │   REST :30082           │      └──────┬───────┘
  └─────────────────────────┘             │
        │                                 ▼
        │                        ┌──────────────────┐
  ┌─────┴─────┐                  │  Sigma Engine    │
  │ PostgreSQL│◄─────────────────│  :30080          │
  │  :6100    │                  └──────────────────┘
  └───────────┘                          │
        │                                ▼
  ┌─────┴─────┐                  ┌──────────────────┐
  │   Redis   │                  │   Dashboard      │
  │  :6379    │                  │   :30088         │
  └───────────┘                  └──────────────────┘
```

---

## Step 1 — Start Backend Infrastructure

```powershell
cd D:\EDR_Platform
docker compose up -d --build
```

This starts **8 services**: Connection Manager, Sigma Engine, Dashboard (Nginx), PostgreSQL, Redis, Kafka, Zookeeper, and a one-shot Kafka topic initializer (`kafka-init`).

**Verify all services are healthy :**

```powershell
docker compose ps
```

| Service | Ports | Purpose |
|---------|-------|---------|
| `connection-manager` | `47051` (gRPC), `30082` (REST) | Agent ingestion + C2 routing |
| `sigma-engine` | `30080` | Detection engine (Kafka consumer) |
| `dashboard` | `30088` | Web UI (Nginx SPA) |
| `postgres` | `6100` | PostgreSQL 16 |
| `redis` | `6379` | Redis 7 (lineage cache) |
| `kafka` | `9092` / `29092` | Event bus |
| `zookeeper` | `2181` | Kafka coordination |

---

## Step 2 — Run the Agent


```powershell
PS C:\> .\edr-agent.exe -install -server-ip "192.168.152.1" -server-domain "edr.local" -server-port "47051" -token "0082f405074366da46291ec74eb37c0927490d55e1280abbd8a2bf1ce22a6361"
```
replace with your IP and Token.

---

## Step 3 — Access the Dashboard

Open **http://localhost:30088** in your browser.

---

## Useful Commands

| Task | Command |
|------|---------|
| View all logs | `docker compose logs -f` |
| View Sigma Engine logs | `docker compose logs -f sigma-engine` |
| Check Kafka topics | `docker compose exec kafka kafka-topics --bootstrap-server localhost:9092 --list` |
| Rebuild a single service | `docker compose up -d --build sigma-engine` |
| Stop everything | `docker compose down` |
| Full reset (delete data) | `docker compose down -v` |

---
