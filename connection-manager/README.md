# Antigravity EDR - Connection Manager

Core gRPC communication infrastructure for the Antigravity EDR platform. Provides secure agent-to-server communication with mTLS authentication, JWT tokens, and real-time event ingestion.

## Features

- **gRPC Server** - Bidirectional streaming for event ingestion
- **mTLS Authentication** - Mutual TLS with certificate validation
- **JWT Authorization** - RS256 signed tokens with blacklist support
- **Event Ingestion** - High-throughput event processing (5000+ EPS)
- **Heartbeat Monitoring** - Agent health tracking and status updates
- **Certificate Management** - Automatic certificate renewal
- **Agent Registration** - Secure onboarding with installation tokens

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    CONNECTION MANAGER                        │
├─────────────────────────────────────────────────────────────┤
│  gRPC Server (Port 50051)                                   │
│  ├─ StreamEvents (bidirectional)                            │
│  ├─ Heartbeat (unary)                                       │
│  ├─ RequestCertificateRenewal (unary)                       │
│  └─ RegisterAgent (unary)                                   │
├─────────────────────────────────────────────────────────────┤
│  HTTP Server (Port 8090)                                    │
│  ├─ /healthz (health check)                                 │
│  └─ /metrics (Prometheus)                                   │
├─────────────────────────────────────────────────────────────┤
│  Security Layer                                             │
│  ├─ mTLS Certificate Validation                             │
│  ├─ JWT Token Verification                                  │
│  └─ Rate Limiting (10,000 events/sec per agent)             │
├─────────────────────────────────────────────────────────────┤
│  Data Layer                                                 │
│  ├─ PostgreSQL (persistent storage)                         │
│  └─ Redis (caching, rate limiting, token blacklist)         │
└─────────────────────────────────────────────────────────────┘
```

## Quick Start

### Prerequisites

- Go 1.21+
- PostgreSQL 14+
- Redis 7+
- protoc (Protocol Buffers compiler)

### Installation

```bash
# Clone the repository
cd connection-manager

# Install dependencies
make deps

# Generate protobuf files
make proto

# Run database migrations
export DATABASE_URL="postgres://user:pass@localhost:5432/edr?sslmode=disable"
make migrate-up

# Build and run
make run
```

### Configuration

Create a `config.yaml` file or use environment variables:

```yaml
server:
  grpc_port: 50051
  http_port: 8090
  tls_cert_path: "./certs/server.crt"
  tls_key_path: "./certs/server.key"
  ca_cert_path: "./certs/ca.crt"

database:
  host: "localhost"
  port: 5432
  user: "edr"
  password: "${DATABASE_PASSWORD}"
  name: "edr"
  ssl_mode: "require"

redis:
  addr: "localhost:6379"
  password: "${REDIS_PASSWORD}"
  db: 0

jwt:
  private_key_path: "./certs/jwt_private.pem"
  public_key_path: "./certs/jwt_public.pem"
  access_ttl: "24h"
  refresh_ttl: "2160h"  # 90 days

rate_limit:
  events_per_second: 10000
  burst_multiplier: 1.2
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `GRPC_PORT` | gRPC server port | 50051 |
| `HTTP_PORT` | HTTP server port (health/metrics) | 8090 |
| `DATABASE_URL` | PostgreSQL connection string | - |
| `REDIS_ADDR` | Redis address | localhost:6379 |
| `LOG_LEVEL` | Log level (debug, info, warn, error) | info |

## Development

### Generate Protobuf Files

```bash
# Install protoc plugins (first time only)
make proto-install

# Generate Go code from .proto files
make proto
```

### Run Tests

```bash
# Unit tests
make test-unit

# Integration tests (requires PostgreSQL and Redis)
make test-integration

# Coverage report
make test-coverage

# Load tests
make test-load
```

### Code Quality

```bash
# Run linter
make lint

# Format code
make fmt

# Vet code
make vet
```

## API Documentation

### gRPC Services

#### StreamEvents (Bidirectional Streaming)
```protobuf
rpc StreamEvents(stream EventBatch) returns (stream CommandBatch) {}
```

#### Heartbeat (Unary)
```protobuf
rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse) {}
```

#### RequestCertificateRenewal (Unary)
```protobuf
rpc RequestCertificateRenewal(CertRenewalRequest) returns (CertificateResponse) {}
```

#### RegisterAgent (Unary)
```protobuf
rpc RegisterAgent(AgentRegistrationRequest) returns (AgentRegistrationResponse) {}
```

## Security

- **TLS 1.3** - Only TLS 1.3 connections accepted
- **mTLS** - Client certificates required and validated
- **JWT RS256** - 2048-bit RSA signed tokens
- **Certificate Validity** - 90 days with auto-renewal
- **Rate Limiting** - 10,000 events/sec per agent

## Monitoring

### Health Check

```bash
curl http://localhost:8090/healthz
```

### Prometheus Metrics

```bash
curl http://localhost:8090/metrics
```

Key metrics:
- `grpc_requests_total` - Total gRPC requests
- `grpc_request_duration_seconds` - Request latency histogram
- `events_received_total` - Total events received
- `agents_online` - Current online agents count
- `rate_limits_triggered_total` - Rate limit violations

## License

Copyright © 2025 Antigravity EDR Platform
