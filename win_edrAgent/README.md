# EDR Windows Agent

Production-ready Windows endpoint agent for the EDR platform.

## Features

- **Real-time Event Collection**: ETW-based kernel event monitoring
- **Secure Communication**: mTLS encrypted gRPC streaming
- **Response Actions**: 9 command types for incident response
- **Resource Efficient**: < 5% CPU, < 50MB memory
- **Enterprise Ready**: Windows Service with auto-recovery

## Quick Start

### Build

```powershell
# Development build
.\scripts\build.ps1

# Release build
.\scripts\build.ps1 -Release -Version "1.0.0"

# Build with tests
.\scripts\build.ps1 -Test
```

### Install

```powershell
# Install as Windows Service
.\bin\agent.exe -install

# Start service
net start EDRAgent

# Check status
sc query EDRAgent
```

### Run Standalone (Development)

```powershell
# Run with debug logging
.\bin\agent.exe -debug

# Run with custom config
.\bin\agent.exe -config "C:\path\to\config.yaml"
```

### Uninstall

```powershell
# Stop service
net stop EDRAgent

# Remove service
.\bin\agent.exe -uninstall
```

## Configuration

Configuration file: `C:\ProgramData\EDR\config\config.yaml`

```yaml
server:
  address: "cm.example.com:50051"
  timeout: 30s

agent:
  batch_size: 50
  batch_interval: 1s
  buffer_size: 5000

logging:
  level: "INFO"
  file_path: "C:\\ProgramData\\EDR\\logs\\agent.log"
```

See `config/default.yaml` for all options.

## Directory Structure

```
C:\Program Files\EDR\
└── agent.exe           # Main executable

C:\ProgramData\EDR\
├── config\
│   └── config.yaml     # Configuration
├── certs\
│   ├── client.crt      # Agent certificate
│   ├── private.key     # Private key
│   └── ca-chain.crt    # CA certificate
├── logs\
│   └── agent.log       # Log files
└── quarantine\         # Quarantined files
```

## Event Types

| Type | Description |
|------|-------------|
| Process | Process creation/termination |
| Network | TCP/UDP connections |
| File | File operations |
| Registry | Registry modifications |
| DNS | DNS queries |
| Auth | Authentication events |
| Driver | Driver load/unload |
| ImageLoad | DLL/module loading |
| Pipe | Named pipe operations |
| WMI | WMI operations |
| Clipboard | Clipboard access |

## Commands

| Command | Description |
|---------|-------------|
| TERMINATE_PROCESS | Kill process by PID |
| QUARANTINE_FILE | Move file to quarantine |
| ISOLATE_NETWORK | Disable network adapters |
| UNISOLATE_NETWORK | Restore network |
| COLLECT_FORENSICS | Gather evidence files |
| UPDATE_CONFIG | Apply new configuration |
| UPDATE_AGENT | Deploy new version |
| RESTART_SERVICE | Restart agent service |
| ADJUST_RATE | Change batch parameters |

## Requirements

- Windows 10/11 or Windows Server 2016+
- Administrator privileges (for installation)
- Network access to Connection Manager

## Development

```powershell
# Get dependencies
go mod download

# Run tests
go test ./... -v -cover

# Build
go build -o bin/agent.exe ./cmd/agent
```

## Architecture

```
┌─────────────────────────────────────┐
│           Windows Agent             │
├─────────────────────────────────────┤
│  ETW Collector  │  WMI Collector    │
├─────────────────┼───────────────────┤
│         Event Batcher               │
│    (Snappy compression)             │
├─────────────────────────────────────┤
│         gRPC Client                 │
│    (mTLS, bidirectional)            │
├─────────────────────────────────────┤
│       Command Handler               │
│    (9 response actions)             │
└─────────────────────────────────────┘
              ↕ gRPC/mTLS
         Connection Manager
```

## License

Proprietary - All rights reserved.
