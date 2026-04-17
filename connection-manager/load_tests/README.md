# Load Testing Suite

EDR Connection Manager load testing using [k6](https://k6.io/).

## Prerequisites

Install k6:
```bash
# Windows (chocolatey)
choco install k6

# macOS
brew install k6

# Docker
docker pull grafana/k6
```

## Test Scenarios

| Scenario | Target | Duration | Description |
|----------|--------|----------|-------------|
| 1. Baseline | 5000 EPS | 10 min | Standard sustained load |
| 2. High Load | 10000 EPS | 10 min | Stress test 2x capacity |
| 3. Burst | 2x spike | 8 min | Sudden traffic spike |
| 4. API | 50 VUs | 5 min | REST API endpoints |
| 5. Soak | 5000 EPS | 1 hour | Long-running stability |

## Quick Start

```bash
# Start the server stack
docker-compose up -d

# Run baseline test
k6 run load_tests/scenario_1_baseline.js

# Run with custom API host
k6 run -e API_HOST=http://localhost:8080 load_tests/scenario_1_baseline.js
```

## Running All Scenarios

```bash
# Create results directory
mkdir -p results

# Run all scenarios
k6 run load_tests/scenario_1_baseline.js
k6 run load_tests/scenario_2_high_load.js
k6 run load_tests/scenario_3_burst.js
k6 run load_tests/scenario_4_api.js

# Soak test (run separately - 1 hour)
k6 run load_tests/scenario_5_soak.js
```

## Performance Targets

| Metric | Target | Threshold |
|--------|--------|-----------|
| Events/Second | 5000+ | Minimum |
| p50 Latency | < 100ms | Pass |
| p99 Latency | < 500ms | Pass |
| Error Rate | < 0.1% | Pass |

## Results

Results are output to:
- `stdout` - Summary report
- `results/*.json` - Detailed metrics

## Monitoring During Tests

Watch metrics in real-time:
```bash
# Prometheus metrics
curl http://localhost:8090/metrics

# Kafka-UI
open http://localhost:8081
```
