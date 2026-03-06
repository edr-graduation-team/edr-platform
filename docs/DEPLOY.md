# EDR Platform - Full Stack Deployment

This guide covers running the **entire EDR stack** with a single `docker-compose` from the repository root (e.g. `d:\EDR_Server`).

---

## Prerequisites

- **Docker Desktop** (with Compose)
- **Certificates** for Connection Manager (gRPC mTLS and JWT) — see below

---

## 1. Generate development certificates

Connection Manager requires TLS and JWT keys. Generate them once before the first run:

```powershell
# From repo root (e.g. d:\EDR_Server)
cd connection-manager

# Option A: Git Bash or WSL (OpenSSL available)
bash scripts/generate_certs.sh

# Option B: If you have OpenSSL in PATH (e.g. from Git for Windows)
# Run the same commands as in scripts/generate_certs.sh from connection-manager/certs/
# See connection-manager/certs/README.md for manual steps.
```

This creates `connection-manager/certs/` with `ca.crt`, `server.crt`, `server.key`, `jwt_private.pem`, `jwt_public.pem`. **Do not use these in production.**

---

## 2. Start the full stack

```powershell
# From repo root
cd d:\EDR_Server

docker-compose up -d
```

**Services started:**

| Service             | Port(s)   | Description                    |
|---------------------|-----------|--------------------------------|
| PostgreSQL          | 5432      | DB `sigma` / user `sigma`     |
| Kafka               | 9092      | events-raw, alerts topics      |
| Zookeeper           | 2181      | For Kafka                      |
| Redis               | 6379      | Cache                          |
| Sigma Engine        | 8080      | Detection (Kafka mode + /health) |
| Connection Manager   | 8082, 50051 | gRPC + REST, health at /healthz |
| Dashboard           | 8088→80   | Web UI                         |

- **Dashboard:** http://localhost:8088  
- **Connection Manager health:** http://localhost:8082/healthz  
- **Sigma Engine health:** http://localhost:8080/health  

Database tables are created automatically from `connection-manager/internal/database/migrations` on first Postgres start.

---

## 3. Verify

```powershell
docker-compose ps
curl http://localhost:8082/healthz
curl http://localhost:8080/health
```

Open http://localhost:8088 and log in (create a user via Connection Manager API or seed script if required).

---

## 4. Manual / component-wise run

For running components **outside Docker** (e.g. local Go/Node), use:

- [RUN_GUIDE.md](RUN_GUIDE.md) — step-by-step (infra from `connection-manager`, then Sigma Engine, Connection Manager, Dashboard)
- [PRODUCTION_GUIDE.md](PRODUCTION_GUIDE.md) — production-style steps and checks
- [VM_DEPLOYMENT_GUIDE.md](VM_DEPLOYMENT_GUIDE.md) — Host + Windows VM agent

When using **root** `docker-compose`, DB is `sigma`/`sigma`. When using **connection-manager** `docker-compose` only, DB is `edr`/`edr` — keep env and docs consistent with the compose you use.
