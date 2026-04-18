# EDR Agent — Response Module: Current State & Planned Enhancements

**Project:** EDR Platform (win_edrAgent)
**Document Type:** Internal Technical Report
**Audience:** Project Team

---

## 1. Overview

This report describes the current state of the **Response** component in our EDR agent and outlines the enhancements planned to bring it closer to industry-standard endpoint detection and response systems such as CrowdStrike Falcon, SentinelOne, Carbon Black, and Microsoft Defender for Endpoint (MDE).

The response layer is the component responsible for **acting** on a detected threat — not just recording it. While our agent currently has a solid server-driven response framework, it lacks the ability to act independently on the endpoint without server authorization. The goal of the enhancements is to introduce **autonomous, local response actions** that execute in real time, without depending on the server being reachable.

---

## 2. Current State of Response

### 2.1 What Is Already Implemented

The agent currently handles all response actions through the Command & Control (C2) server. When the server detects a threat via the Sigma engine or context scoring, it sends a command over the gRPC channel and the agent executes it. The following actions are fully implemented:

| Command | Description | Quality |
|---|---|---|
| `TERMINATE_PROCESS` | Kills a process by PID using native Win32 API. Protected processes (csrss, lsass, etc.) are blocked. | ✅ Complete |
| `QUARANTINE_FILE` | Moves a suspicious file to `C:\ProgramData\EDR\quarantine` with a metadata sidecar. | ✅ Complete |
| `ISOLATE_NETWORK` | Blocks all network traffic via Windows Firewall, while keeping the C2 channel open. Includes a watchdog for IP changes and ACK-before-block grace period. | ✅ Complete |
| `UNISOLATE_NETWORK` | Reverses network isolation and restores default firewall policy. | ✅ Complete |
| `COLLECT_FORENSICS` | Collects Windows Event Logs (Security, Sysmon, etc.) for a given time range using `wevtutil`. | ✅ Complete |
| `UPDATE_CONFIG` | Applies a new agent configuration (full YAML or partial JSON policy) with hot-reload, no restart required. | ✅ Complete |
| `RESTART_SERVICE` | Restarts or stops the EDR agent service or standalone process remotely. | ✅ Complete |
| `RUN_CMD` | Executes a diagnostic command from a strict whitelist (ping, netstat, ipconfig, etc.) — no shell interpretation. | ✅ Complete |
| `RESTART` / `SHUTDOWN` | Reboots or shuts down the host OS. Requires explicit `confirm=true` parameter. | ✅ Complete |

The agent also includes advanced **tamper protection** layers:
- **Process DACL hardening** — prevents non-SYSTEM processes from terminating the agent.
- **Service DACL hardening** — prevents `sc stop EDRAgent` from administrator context.
- **Registry key hardening** — locks `HKLM\SOFTWARE\EDR` to SYSTEM-only write access.
- **Uninstall token verification** — SHA-256 hash check required before uninstall.

### 2.2 The Core Limitation

Every response action described above **requires a round-trip to the server**. The agent collects an event → sends it to the server → the server analyzes it → issues a command → the agent executes it. This pipeline introduces inherent latency (typically 5–30 seconds) during which a threat may already have executed its payload.

More critically: **when the network is isolated or unavailable, the agent has no local defense**. It can detect events but cannot act on them.

---

## 3. Planned Enhancements

The following additions introduce **autonomous, server-independent response** capabilities. Each runs entirely within the agent process, uses the existing event pipeline, and is designed with minimal performance impact.

---

### 3.1 Local Signature Database

**What it is:**
A lightweight, embedded key-value database (`bbolt`) bundled inside the agent binary. It stores SHA-256 hashes of known malicious files alongside threat metadata (name, family, severity).

**How it works:**
- On agent startup, the database is loaded from a pre-seeded file containing a curated set of well-known malware signatures sourced from public threat intelligence repositories (MalwareBazaar, abuse.ch).
- The database can be updated remotely via a new server command (`UPDATE_SIGNATURES`) without restarting the agent.
- Lookup time per file is under **1 millisecond** — the database uses a binary B-tree structure optimized for key lookups.

**Database Schema:**
```
Bucket: "malware_hashes"
Key:    SHA-256 hex string
Value:  { name, family, severity, source, added_at }
```

**Why it matters:**
The agent can now make a yes/no threat decision locally, without sending the file to the server for analysis. This is the foundation for all autonomous response actions described below.

---

### 3.2 Real-Time Local File Scanner

**What it is:**
A scanning engine that computes the SHA-256 hash of any file and checks it against the local signature database.

**How it works:**
- The scanner reads only what is necessary to compute the hash — it does not parse file contents or apply behavioral heuristics (keeping CPU usage negligible).
- Files over a configurable size threshold are skipped automatically to prevent I/O saturation.
- The result object returned by the scanner contains: `IsMatched`, `ThreatName`, `Severity`, `Hash`, and a recommended `Action` (`block`, `quarantine`, or `alert`).

**Performance characteristics:**
- Hash computation for a 10 MB file: < 15 ms
- Hash computation for a 1 MB file: < 2 ms
- Database lookup: < 1 ms

---

### 3.3 Automatic File Response (Auto-Quarantine)

**What it is:**
An autonomous responder that intercepts file creation and write events from the ETW collector and triggers the scanner. If a match is found, the file is immediately quarantined without any server interaction.

**How it works:**

```
ETW File I/O Event (FileCreate / FileWrite)
    │
    ▼
Is the file path in a high-priority monitored location?
    │
    ├── No  → Forward event to server as usual (no change)
    │
    └── Yes → Compute SHA-256 hash
                   │
                   ├── No match → Forward event to server as usual
                   │
                   └── Match found →
                           1. Move file to C:\ProgramData\EDR\quarantine
                           2. Write metadata (original path, hash, threat name, timestamp)
                           3. Emit a HIGH severity event to the server with action details
                           4. Log the action locally
```

**Monitored high-priority paths:**

| Path | Reason |
|---|---|
| `C:\Users\*\Downloads\` | Browser-downloaded files |
| `C:\Users\*\Desktop\` | User-facing drop zone |
| `C:\Users\*\AppData\Local\Temp\` | Common malware staging area |
| `D:\`, `E:\`, `F:\` (removable drives) | External storage devices |
| `C:\ProgramData\` | System-wide program data |

> Files in standard OS directories (Windows, System32, WinSxS, etc.) are excluded to avoid false positives and performance impact. This matches the noise-filtering already implemented in the ETW collector.

---

### 3.4 USB / Removable Device Watcher

**What it is:**
A dedicated monitor for removable storage device insertion events. When a new USB drive or external disk is connected, the watcher registers the new drive path and activates file scanning for any file copied from or written to that device.

**How it works:**
- Uses WMI `Win32_VolumeChangeEvent` to detect device arrival and removal events (the WMI infrastructure already exists in `collectors/wmi.go`).
- When a device is inserted, its drive letter is registered as a monitored path in the Auto-Responder.
- When a file is written from that device to any location, it passes through the scanner before being accessible.
- If a threat is detected: the file is quarantined, the event is reported to the server, and an alert includes the USB device's serial number, vendor, and model for forensic traceability.

**Event fields emitted:**
```
action:        "usb_file_quarantined"
file_path:     original path of the file
threat_name:   matched signature name
device_serial: USB device serial (for forensics)
device_vendor: USB device vendor string
```

---

### 3.5 Process Tree Termination

**What it is:**
An extension of the existing `TERMINATE_PROCESS` command that kills not only the targeted process but its entire child process tree.

**Why it matters:**
Malware commonly spawns child processes immediately after execution. Killing only the parent PID (current behavior) allows child processes to continue running or re-launch the parent. Process tree termination eliminates the entire execution chain.

**How it works:**
- The server includes `kill_tree=true` in the command parameters.
- The agent uses the Windows `CreateToolhelp32Snapshot` API to traverse the full process tree starting from the target PID.
- Each child is validated against the critical process list before termination — system processes are always protected.
- The result includes a list of all PIDs terminated.

---

### 3.6 Selective Network Blocking

**What it is:**
A targeted network-blocking capability that blocks communication to a specific IP address or domain **without isolating the entire host**.

**The problem with current isolation:**
The current `ISOLATE_NETWORK` command is binary — it either blocks everything (except C2) or nothing. This is appropriate for severe incidents but is too aggressive for blocking a single malicious C2 domain while keeping the user productive.

**New commands:**

| Command | Parameters | Behavior |
|---|---|---|
| `BLOCK_IP` | `ip`, `direction` (in/out/both) | Adds a Windows Firewall `BLOCK` rule for the specific IP |
| `BLOCK_DOMAIN` | `domain` | Appends the domain to `C:\Windows\System32\drivers\etc\hosts` → `127.0.0.1` (DNS sinkholing) |
| `UNBLOCK_IP` | `ip` | Removes the firewall rule |
| `UNBLOCK_DOMAIN` | `domain` | Removes the hosts file entry |

**Design note:** The hosts-file approach for domain blocking is deliberately lightweight — it requires no kernel driver and works across all Windows versions, while achieving the same result as DNS sinkholing used by enterprise-grade systems.

---

### 3.7 Remote Signature Database Update

**What it is:**
A new server command (`UPDATE_SIGNATURES`) that allows the C2 server to push a fresh batch of malware signatures to the agent's local database without restarting the agent.

**How it works:**
1. The server sends an `UPDATE_SIGNATURES` command with a download URL and SHA-256 checksum of the update file.
2. The agent downloads the file over HTTPS.
3. The checksum is verified before any import begins (integrity guarantee).
4. New signatures are merged into the existing database — existing entries are not overwritten unless the server specifies `force=true`.
5. The agent reports the number of new signatures added and the new database version.

**This allows the security team to:**
- React to newly discovered zero-day malware by pushing its hash within minutes to all agents.
- Distribute custom organizational threat intelligence hashes.
- Keep agents up to date even when full agent updates are not feasible.

---

## 4. Summary of Additions

| # | Addition | Scope | Impact |
|---|---|---|---|
| 1 | Local Signature Database | New module: `internal/signatures/` | Foundation for autonomous response |
| 2 | Real-Time Local File Scanner | New module: `internal/scanner/` | Sub-millisecond threat identification |
| 3 | Auto-Quarantine on File Events | New module: `internal/responder/` + modification to `collectors/file.go` | Autonomous quarantine without server |
| 4 | USB Device Watcher | New: `internal/collectors/usb_watcher.go` | USB threat containment |
| 5 | Process Tree Termination | Modification to `command/handler.go` | Full execution chain elimination |
| 6 | Selective Network Blocking (IP + Domain) | Modification to `command/handler.go` | Precision network response |
| 7 | Remote Signature Update Command | Modification to `command/handler.go` | Continuous IOC distribution |

---

## 5. Industry References

The following table maps each planned addition to its equivalent implementation in leading commercial EDR products.

### 5.1 Local Signature Database

| Platform | Implementation |
|---|---|
| **CrowdStrike Falcon** | Uses a local lightweight threat intelligence database synced from the cloud (part of the "Sensor Intelligence" framework). Operates fully offline. |
| **SentinelOne** | The "Static AI" engine runs entirely on the agent using a trained model + a local hash database; no cloud query required for known malware. |
| **Microsoft Defender for Endpoint** | Signature definitions (`mpdef.bin`) are stored locally and updated via Windows Update or WSUS, enabling offline detection. |
| **Carbon Black (VMware)** | "Reputation Service" queries a local cache before escalating to cloud — minimizes latency for known threats. |

**Reference:** CrowdStrike: *"Falcon Sensor Intelligence — Local Threat Data"*, SentinelOne: *"Static AI and Behavioral AI Engines"* (SentinelOne Platform Architecture Guide)

---

### 5.2 Real-Time File Scanning & Auto-Quarantine

| Platform | Implementation |
|---|---|
| **CrowdStrike Falcon** | The Falcon sensor's "Machine Learning" and "Indicator of Attack" engines evaluate files at write-time using hash + behavioral heuristics. Quarantine is performed locally in milliseconds. |
| **SentinelOne** | "Behavioral AI" runs at file execution and creation time. Automatic remediation (quarantine, rollback) is a core product feature. |
| **Microsoft Defender for Endpoint** | Real-time protection monitors file writes via a kernel minifilter driver. Quarantine is done locally; the event is reported to the Defender portal. |
| **Carbon Black App Control** | File-level allow/deny decisions happen at the kernel level using a local policy database; no cloud round-trip required. |

**Reference:** Microsoft: *"How Microsoft Defender Antivirus works"* (Microsoft Learn), SentinelOne: *"Autonomous Response Actions"* (Product Documentation)

---

### 5.3 USB / Removable Device Monitoring

| Platform | Implementation |
|---|---|
| **CrowdStrike Falcon** | Device Control policies can block, allow, or read-only restrict USB devices by vendor/product ID. File-level scanning is triggered on USB insertion. |
| **SentinelOne** | "Device Control" module monitors USB insertion via kernel events. Policies can be set to quarantine any executable copied from USB. |
| **Microsoft Defender for Endpoint** | "Removable Storage Access Control" policies apply at the driver level and integrate with Intune/MDM for enterprise USB governance. |
| **Symantec Endpoint Protection** | USB monitoring with automatic scan-on-connect and policy-based device blocking by hardware ID. |

**Reference:** CrowdStrike: *"USB Device Control"* (Falcon Platform Administration Guide), Microsoft: *"Control USB devices and other removable media using Microsoft Defender for Endpoint"* (Microsoft Learn)

---

### 5.4 Process Tree Termination

| Platform | Implementation |
|---|---|
| **CrowdStrike Falcon** | When a process is identified as malicious, Falcon terminates the process and its entire spawned process tree, preventing re-launch from child processes. |
| **SentinelOne** | "Kill & Quarantine" action kills the process chain and optionally rolls back all file changes made by the process tree since execution. |
| **Microsoft Defender for Endpoint** | Live Response includes process tree termination; automated investigation terminates all child processes of a malicious parent. |
| **Carbon Black (VMware)** | "Deny" policy on a process hash prevents execution of all child processes spawned from the denied binary. |

**Reference:** SentinelOne: *"Kill, Quarantine, and Remediate Threats"* (SOC Operations Guide), CrowdStrike: *"Real-time Response — Process Termination"* (Falcon Documentation)

---

### 5.5 Selective Network Blocking / DNS Sinkholing

| Platform | Implementation |
|---|---|
| **CrowdStrike Falcon** | "Network Containment" supports both full isolation and selective IP/domain blocking. DNS-level blocking is performed by the sensor before the query leaves the host. |
| **SentinelOne** | "Network Isolation" and "Network Quarantine" modes; per-IP firewall rules can be pushed from the management console without full isolation. |
| **Microsoft Defender for Endpoint** | "Indicators of Compromise" (IoC) can be configured at domain and IP level — Defender intercepts DNS queries and blocks connections to matched entries. |
| **Palo Alto Cortex XDR** | "Network Restrictions" allow granular per-IP, per-port containment actions from the management console without isolating the entire endpoint. |

**Reference:** CrowdStrike: *"Custom Indicators of Attack — Blocking Network IOCs"*, Microsoft: *"Create indicators for IPs and URLs/domains"* (Microsoft Learn — Defender for Endpoint)

---

### 5.6 Remote Signature / IOC Distribution

| Platform | Implementation |
|---|---|
| **CrowdStrike Falcon** | "Custom IOC" management allows security teams to push MD5/SHA256 hashes to all sensors within minutes via the Falcon console. Sensors apply them locally without restarting. |
| **SentinelOne** | "Custom Detection Rules" and "Custom Blacklist Hashes" can be pushed to all agents from the management console with immediate effect. |
| **Microsoft Defender for Endpoint** | "File Indicators" (allow/block/audit by hash) can be configured in the Defender Security Center and are distributed to all enrolled agents via cloud policy sync. |
| **ESET Endpoint Security** | Signature databases are pushed incrementally via ESET Security Management Center (ESMC) without requiring full agent updates. |

**Reference:** CrowdStrike: *"IOC Management — Custom Hash Indicators"* (Falcon Threat Intelligence), Microsoft: *"Create indicators for files"* (Microsoft Learn — Defender for Endpoint)

---
