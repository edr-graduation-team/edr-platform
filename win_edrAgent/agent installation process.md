# Zero-Touch Installer & Hot-Reload — Implementation Walkthrough

## Build Verification

```
PS D:\EDR_Platform\win_edrAgent> go build ./...
(no output — zero errors, zero warnings)
```

---

## What Was Implemented

### New Files

| File | Purpose |
|---|---|
| [installer.go](file:///d:/EDR_Platform/win_edrAgent/internal/installer/installer.go) | Zero-touch install logic (hosts, config, dirs) |
| [installer_test.go](file:///d:/EDR_Platform/win_edrAgent/internal/installer/installer_test.go) | 8 unit tests — no admin required |

### Modified Files

| File | What changed |
|---|---|
| [main.go](file:///d:/EDR_Platform/win_edrAgent/cmd/agent/main.go) | New install flags, [runInstall()](file:///d:/EDR_Platform/win_edrAgent/cmd/agent/main.go#159-256), SCM auto-detection |
| [service.go](file:///d:/EDR_Platform/win_edrAgent/internal/service/service.go) | [StartService()](file:///d:/EDR_Platform/win_edrAgent/internal/service/service.go#269-308) added, default-config write removed |
| [agent.go](file:///d:/EDR_Platform/win_edrAgent/internal/agent/agent.go) | [UpdateConfig()](file:///d:/EDR_Platform/win_edrAgent/internal/agent/agent.go#299-351), [SetConfigUpdateHandler()](file:///d:/EDR_Platform/win_edrAgent/internal/agent/agent.go#285-298) added |
| [batcher.go](file:///d:/EDR_Platform/win_edrAgent/internal/event/batcher.go) | [Reconfigure()](file:///d:/EDR_Platform/win_edrAgent/internal/event/batcher.go#206-233) atomic batch params update |
| [handler.go](file:///d:/EDR_Platform/win_edrAgent/internal/command/handler.go) | [SetConfigUpdateCallback()](file:///d:/EDR_Platform/win_edrAgent/internal/command/handler.go#128-136), real YAML-parsing [updateConfig()](file:///d:/EDR_Platform/win_edrAgent/internal/command/handler.go#694-755) |

---

## Zero-Touch Installation Flow

```
agent.exe -install \
  -server-ip   192.168.152.1 \
  -server-domain edr.internal \
  -server-port 50051 \
  -token       ecb8c83b...
```

```
[1/5] Creating EDR directories...         C:\ProgramData\EDR\{config,certs,logs,queue,quarantine}
[2/5] Patching hosts file...              192.168.152.1   edr.internal   # EDR C2
[3/5] Generating config.yaml...           server.address = edr.internal:50051 + fresh UUID
[4/5] Registering Windows Service...      EDRAgent | Automatic | LocalSystem
[5/5] Starting EDRAgent service...        polls svc.Running up to 10s

✓ EDR Agent installed and running successfully.
```

**Each step is idempotent.** Re-running `-install` on an already-installed machine:
- Step 2 — Hosts scan finds existing entry → skips
- Step 4 — Detects `"already exists"` → uninstalls first, then re-registers (clean re-deploy)

---

## SCM Execution Mode Detection

The old `-service` flag required operators to remember it manually. It is now **auto-detected**:

```go
// main.go — replaces the manual -service flag
isScm, _ := svc.IsWindowsService()
if isScm {
    service.Run(cfg, logger)  // SCM-managed path
} else {
    runStandalone(cfg, logger, *configPath)  // Interactive / development
}
```

The `-service` flag is kept as a no-op for backward compatibility.

---

## Hot-Reload Architecture (C2 → config.yaml → Live Batcher)

```
C2 gRPC stream
    │
    │  CommandType = UPDATE_CONFIG
    │  params["config"] = "<full YAML>"
    ▼
agent.runCommandLoop()
    │
    ▼
command.Handler.Execute()
    │
    ▼
handler.updateConfig()          ← YAML parsed into *config.Config
    │
    ▼  (callback, no import cycle)
agent.UpdateConfig(newCfg)
    ├─ 1. Validate()             ← rejects bad values before touching disk
    ├─ 2. newCfg.Save(path)      ← overwrites C:\ProgramData\EDR\config\config.yaml
    ├─ 3. atomic cfg swap        ← a.cfg = newCfg  (under write lock)
    └─ 4. batcher.Reconfigure()  ← new BatchSize/Interval/Compression live immediately
```

**What hot-reloads without service restart:**
- `server.address` (new connections pick up the new address via reconnector)
- `agent.batch_size`, `agent.batch_interval`, `agent.compression`
- `logging.level` (applied to next log call)
- All `filtering.*` fields (consulted on every event)

**What requires a service restart:**
- Collector enable/disable flags (`etw_enabled`, `wmi_enabled`, etc.) — goroutines are already bound to the original context

---

## Sparse Policy Override (C2 → Targeted Fields)

When the C2 sends `UPDATE_CONFIG` without a full YAML payload, individual overrides are supported:

```json
{
  "type": "COMMAND_TYPE_UPDATE_CONFIG",
  "parameters": {
    "log_level": "DEBUG",
    "exclude_process": "suspicious.exe"
  }
}
```

These are acknowledged and logged for the next config reload.

---

## Callback Wiring (No Import Cycle)

The `command` package cannot import `agent` (circular). The callback pattern solves this:

```go
// main.go / service.go — called once after agent.New()
ag.SetConfigUpdateHandler(ag.UpdateConfig)
//   ↑ registers: handler.configUpdateFn = agent.UpdateConfig
```

```go
// agent.go — SetConfigUpdateHandler wires into command.Handler
func (a *Agent) SetConfigUpdateHandler(fn func(*config.Config) error) {
    a.commandHandler.SetConfigUpdateCallback(fn)  // bridge
}
```

---

## Manual Smoke Test Checklist

```powershell
# 1. Install
.\agent.exe -install -server-ip 192.168.152.1 -server-domain edr.internal `
            -server-port 50051 -token ecb8c83b...

# 2. Verify hosts file
Select-String "192.168.152.1" C:\Windows\System32\drivers\etc\hosts

# 3. Verify config
(Get-Content C:\ProgramData\EDR\config\config.yaml | Select-String "address")

# 4. Verify service state
(Get-Service EDRAgent).Status   # → Running

# 5. Push config update from dashboard → "Update Config" command
# Check logs:
Get-Content C:\ProgramData\EDR\logs\agent.log -Tail 20 | Select-String "HotReload"

# 6. Uninstall
.\agent.exe -uninstall
```
