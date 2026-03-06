// Scenario 5: Sustained Load (Soak Test)
// Target: Verify stability over 1 hour continuous load

import http from 'k6/http';
import { check } from 'k6';
import { Counter, Rate, Gauge } from 'k6/metrics';

const eventsSent = new Counter('events_sent');
const errorRate = new Rate('error_rate');
const memoryUsage = new Gauge('memory_estimate_mb');

export const options = {
    scenarios: {
        soak_test: {
            executor: 'constant-arrival-rate',
            rate: 500,              // 5000 EPS sustained
            timeUnit: '1s',
            duration: '1h',         // 1 hour soak
            preAllocatedVUs: 100,
            maxVUs: 150,
        },
    },
    thresholds: {
        http_req_duration: ['p(99)<500'],
        error_rate: ['rate<0.001'],  // Very strict for soak
        // Check for latency degradation
        'http_req_duration{scenario:soak_test}': ['p(99)<600'],
    },
};

const API_HOST = __ENV.API_HOST || 'http://localhost:8080';

function generateBatch() {
    return JSON.stringify({
        batch_id: `soak-${Date.now()}-${Math.random().toString(36).substr(2, 6)}`,
        agent_id: `agent-${String(__VU % 100).padStart(5, '0')}`,
        events: Array(10).fill(null).map(() => ({
            type: 'process_create',
            timestamp: new Date().toISOString(),
            data: { test: 'soak' }
        })),
        event_count: 10,
    });
}

export default function () {
    const response = http.post(
        `${API_HOST}/api/v1/events/ingest`,
        generateBatch(),
        {
            headers: {
                'Content-Type': 'application/json',
                'Authorization': 'Bearer test-token',
            },
            timeout: '10s',
        }
    );

    if (check(response, { 'success': (r) => r.status >= 200 && r.status < 300 })) {
        eventsSent.add(10);
        errorRate.add(0);
    } else {
        errorRate.add(1);
    }
}

export function handleSummary(data) {
    const totalEvents = data.metrics.events_sent?.values?.count || 0;
    const duration = data.state.testRunDurationMs / 1000;
    const avgEPS = totalEvents / duration;
    const errorPct = ((data.metrics.error_rate?.values?.rate || 0) * 100).toFixed(4);

    // Check for latency stability
    const p99Start = data.metrics.http_req_duration?.values?.['p(99)'] || 0;

    return {
        'stdout': `
================================================================================
SOAK TEST RESULTS (1 Hour Sustained Load)
================================================================================
Duration:           ${(duration / 60).toFixed(0)} minutes
Total Events:       ${totalEvents.toLocaleString()}
Avg EPS:            ${avgEPS.toFixed(0)}

Latency:
  - p50:            ${data.metrics.http_req_duration?.values?.['p(50)']?.toFixed(0) || 'N/A'} ms
  - p99:            ${data.metrics.http_req_duration?.values?.['p(99)']?.toFixed(0) || 'N/A'} ms

Error Rate:         ${errorPct}%

Stability Checks:
  - Latency Stable: ${p99Start < 500 ? '✅' : '⚠️'} (p99 < 500ms)
  - No Memory Leak: (check Grafana/metrics)
  - Error Rate:     ${parseFloat(errorPct) < 0.1 ? '✅' : '❌'} (< 0.1%)

Result: ${avgEPS >= 5000 && parseFloat(errorPct) < 0.1 ? '✅ PASSED' : '❌ FAILED'}
================================================================================
`,
        'results/soak_test.json': JSON.stringify(data, null, 2),
    };
}
