# 🛡️ Enterprise EDR Platform

## 📖 Executive Summary

Welcome to our **Enterprise Endpoint Detection and Response (EDR) Platform**, developed as a comprehensive senior graduation project. This platform provides robust, real-time threat detection and response capabilities for Windows environments. 

At its core, the system is designed to provide **real-time endpoint telemetry**, identify malicious activities using industry-standard **Sigma rule detection**, and execute rapid incident response through a powerful **C2 (Command and Control) response mechanism**. It is built with enterprise-grade resilience, security, and scalability in mind.

---

## 🏗️ Key Architectural Features

Our EDR platform is engineered leveraging modern system design patterns to ensure high availability and security:

- 🔬 **Microservices Architecture:** Independently scalable components communicating seamlessly, ensuring fault isolation and rapid feature iteration.
- 🔐 **Zero Trust Security:** Enforced strict mutual TLS (mTLS) for agent-server communication and JWT-based authentication for all dashboard and API interactions.
- ⚡ **Event-Driven Processing:** Powered by **Apache Kafka**, allowing the system to handle massive streams of endpoint telemetry asynchronously with high throughput and low latency.
- 🛡️ **Self-Healing & Zero Data Loss:** Built-in resilience mechanisms including Write-Ahead Logging (WAL) and local fallback queues on the agent, guaranteeing that no vital security telemetry is lost even during unexpected network outages.

---

## 🧩 Component Overview

The architecture is divided into 6 main, highly-specialized components:

1. **Windows Agent (Go):** A lightweight, highly-optimized sensor running on Windows endpoints to collect telemetry and execute active response commands.
2. **Connection Manager (Go):** The secure gateway that handles thousands of concurrent mTLS agent connections and routes telemetry securely.
3. **Sigma Engine (Go):** The real-time streaming analytics engine that evaluates incoming telemetry against Sigma rules to generate actionable alerts.
4. **Data Layer (PostgreSQL/Redis):** Reliable persistence for configuration, alerts, and state management (PostgreSQL), paired with high-speed caching (Redis).
5. **Message Broker (Kafka/Zookeeper):** The central nervous system for event-driven telemetry distribution.
6. **SOC Dashboard (React/TypeScript):** A modern, responsive unified Security Operations Center interface for threat hunting, agent management, and alert triage.

---

## 📋 Prerequisites

Before deploying the platform, ensure your environments meet the following requirements:

### Server Environment
- **OS:** Linux or Windows with WSL2 enabled
- **Containerization:** Docker installed
- **Orchestration:** Docker Compose installed

### Agent Environment
- **OS:** Windows Operating System
- **Compiler:** Go 1.24 or higher (if building from source)
- **Permissions:** **Administrator privileges** are strictly required to collect system-level telemetry and execute response actions.

---

## 🚀 Server Deployment (Step-by-Step)

Deploying the backend infrastructure is fully containerized for simplicity:

1. **Clone the repository:**
   ```bash
   git clone https://github.com/your-org/edr-platform.git
   cd edr-platform
   ```

2. **Spin up the infrastructure:**
   ```bash
   docker compose up -d --build
   ```
   > *Note: This will provision the PostgreSQL database, Redis, Zookeeper, Kafka broker, Connection Manager, Sigma Engine, and the React Dashboard.*

---

## 💻 Agent Deployment & Configuration

To deploy the agent on a Windows endpoint, follow these steps carefully. 

### 1. Configuration (`config.yaml`)

You must configure the agent before running it. Create a file named `config.yaml` in your agent directory using the template below.

> [!CAUTION]
> **CRITICAL CONFIGURATION REQUIRED**
> Before starting the agent, you **MUST** modify the following two fields in the `config.yaml` below:
> 1. `server.address` ➡️ Change this to your Server's actual IP address.
> 2. `certs.bootstrap_token` ➡️ Change this to your Server's generated enrollment token.

```yaml
server:
    address: 192.168.152.1:50051  # ⚠️ CHANGE ME: Update to your server's IP
    insecure: false
    timeout: 30s
    reconnect_delay: 1s
    max_reconnect_delay: 30s
    heartbeat_interval: 30s

agent:
    id: 599d30c7-3ba5-48b3-8d11-c61e5486d683
    hostname: WINDOWS-DBD6CUS
    batch_size: 50
    batch_interval: 1s
    buffer_size: 5000
    compression: snappy
    queue_dir: C:\ProgramData\EDR\queue
    max_queue_size_mb: 500

collectors:
    etw_enabled: true
    etw_session_name: EDRAgentSession
    wmi_enabled: true
    wmi_interval: 1h0m0s
    registry_enabled: true
    file_enabled: true
    network_enabled: true

filtering:
    exclude_processes:
        - svchost.exe
        - csrss.exe
        - services.exe
        - smss.exe
        - wininit.exe
        - winlogon.exe
        - dwm.exe
        - taskhostw.exe
        - RuntimeBroker.exe
        - SearchIndexer.exe
        - MsMpEng.exe
        - agent.exe
    exclude_ips:
        - 127.0.0.1
        - ::1
        - 0.0.0.0
        - 169.254.0.0/16
    exclude_registry:
        - HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing
        - HKLM\SYSTEM\CurrentControlSet\Services\bam\State
    exclude_paths:
        - C:\Windows\Temp
        - C:\Users\*\AppData\Local\Temp
        - C:\Windows\SoftwareDistribution
    include_paths:
        - C:\Windows\System32
        - C:\Program Files
        - C:\Program Files (x86)

logging:
    level: DEBUG
    file_path: C:\ProgramData\EDR\logs\agent.log
    max_size_mb: 100
    max_age_days: 7

certs:
    cert_path: C:\ProgramData\EDR\certs\client.crt
    key_path: C:\ProgramData\EDR\certs\private.key
    ca_path: C:\ProgramData\EDR\certs\ca-chain.crt
    bootstrap_token: a6ad5917452929ad733d3d956bf8e866c324865d17581b25533af46e06f6c4e8 # ⚠️ CHANGE ME: Update to your actual token
```

### 2. Compilation and Execution

1. Build the agent binary using Go:
   ```cmd
   go build -o agent.exe .
   ```
2. **Open an Administrator Command Prompt** or PowerShell.
3. Run the agent:
   ```cmd
   .\agent.exe -config config.yaml
   ```

---

## 🧪 Verification & Usage

Once the server and agent are deployed, you can verify the system is fully operational via the SOC Dashboard:

1. **Access the Dashboard:** Open your web browser and navigate to `http://localhost` (or the server's IP address).
2. **Verify Agent Status:** Navigate to the **Agents** tab. You should see your Windows endpoint listed with an **"Online"** status indicator.
3. **Execute C2 Commands:** 
   - Click on the active agent to open its details.
   - Use the command interface to issue an active response command, such as **"Restart Machine"** or **"Isolate Network"**.
   - Verify the action is successfully executed on the target Windows VM.

---

*Developed  by the EDR Graduation Project Team.*
