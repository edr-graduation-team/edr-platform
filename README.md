# EDR Platform - Quick Start Guide

This guide provides step-by-step instructions for pulling and running the pre-packaged EDR Platform using Docker Compose.

## Prerequisites

1. **Docker**: Ensure Docker Desktop (Windows/Mac) or Docker Engine (Linux) is installed and running.
2. **Hardware Requirements**: At least 4GB of free RAM is recommended to run the services smoothly (Postgres, Kafka, Zookeeper, Redis, and custom GO/Node.js services).
3. **Network**: Ensure the required host ports are available (e.g., `30088`, `47051`, `30082`, `30080`, `31292`, `31432`, `31379`, `31181`).

---

## Step-by-Step Setup

### Step 1: Prepare the Bundle
Ensure you have project folder. This folder must include:
* `docker-compose.yml`
* This `README-Run.md`
* Any required configuration directories (`certs`, `config`, `sigma_rules`, etc.) if they were bundled.

**Open a Terminal / PowerShell window and navigate (`cd`) into this directory.**

### Step 2: (Optional) Docker Hub Login  => you can skip this step , لانه عام مايحتاج تسحيل دخول
it is public, you can skip this step.
```bash
docker login
```

### Step 3: Pull the Latest Images
Fetch the pre-built images from Docker Hub, which avoids building them from source on your local machine:
```bash
docker compose pull
```

### Step 4: Start the Services
Start the entire infrastructure and microservices stack in "detached" mode (running in the background):
```bash
docker compose up -d
```

### Step 5: Verify Deployment
Wait ~30-60 seconds for infrastructure services (Kafka, Zookeeper, Postgres) to fully initialize, after which the main application services will automatically become healthy.

Check the status of your containers:
```bash
docker compose ps
```
You should see states like `Up` and `healthy` for `connection-manager`, `sigma-engine`, and `dashboard`.

---

## Accessing the Platform

Once the stack is healthy, you can access the following services:

* **EDR Dashboard (Web UI):** [http://localhost:30088](http://localhost:30088)

###  Testing with the Windows Agent
To test agent enrollment and telemetry:
1. When installing the Windows EDR Agent on your VM or host, provide the gRPC address of the machine currently running Docker.
2. The gRPC endpoint is published on port `47051`.
3. Example payload for agent builder/config: `YOUR_SERVER_IP:47051`

---

##  Troubleshooting

**1. Viewing Service Logs**
If a service is constantly restarting or acting strangely, view its last 100 log lines:
```bash
docker compose logs --tail 100 connection-manager
docker compose logs --tail 100 sigma-engine
```
*(To follow logs live, use `-f` instead of `--tail 100`)*.

**2. Kafka or DB Started Too Slowly**
Sometimes on older CPUs, Kafka or Postgres takes too long to boot, causing connection manager to crash. It should auto-restart, but if it doesn't, force restart the backend services:
```bash
docker compose restart connection-manager sigma-engine
```

**3. Port Conflicts**
If `docker compose up -d` complains that a port is "already allocated", you will need to map it to a different port by modifying the left side of the `ports:` definition in the `docker-compose.yml` file.

**4. Complete Teardown (Data Wipe)**
If you want to completely destroy the environment, delete all databases, alerts, and agents to start fresh, run:
```bash
# WARNING: THIS DELETES ALL VOLUMES AND PERSISTENT DATA
docker compose down -v
```
