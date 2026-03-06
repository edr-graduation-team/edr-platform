// Scenario 1: Baseline Load Test (5000 EPS)
// Target: Verify system can handle 5000 events per second sustained

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

// Custom metrics
const eventsSent = new Counter('events_sent');
const batchLatency = new Trend('batch_latency', true);
const errorRate = new Rate('error_rate');

// Test configuration
export const options = {
    scenarios: {
        baseline_load: {
            executor: 'constant-arrival-rate',
            rate: 500,              // 500 batches per second
            timeUnit: '1s',
            duration: '10m',
            preAllocatedVUs: 100,
            maxVUs: 200,
        },
    },
    thresholds: {
        http_req_duration: ['p(50)<100', 'p(95)<300', 'p(99)<500'],
        http_req_failed: ['rate<0.001'],
        error_rate: ['rate<0.001'],
        events_sent: ['count>3000000'], // 5000 EPS * 600s = 3,000,000
    },
};

const API_HOST = __ENV.API_HOST || 'http://localhost:8080';
const EVENTS_PER_BATCH = 10;

// Generate event batch
function generateBatch(vuId) {
    const events = [];
    for (let i = 0; i < EVENTS_PER_BATCH; i++) {
        events.push({
            type: ['process_create', 'file_write', 'network_connect'][i % 3],
            timestamp: new Date().toISOString(),
            data: { pid: Math.floor(Math.random() * 65535) }
        });
    }

    return JSON.stringify({
        batch_id: `b-${Date.now()}-${vuId}-${Math.random().toString(36).substr(2, 6)}`,
        agent_id: `agent-${String(vuId % 100).padStart(5, '0')}`,
        events: events,
        event_count: EVENTS_PER_BATCH,
    });
}

export default function () {
    const start = Date.now();

    const response = http.post(
        `${API_HOST}/api/v1/events/ingest`,
        generateBatch(__VU),
        {
            headers: {
                'Content-Type': 'application/json',
                'Authorization': 'Bearer test-token',
            },
            timeout: '10s',
        }
    );

    const latency = Date.now() - start;
    batchLatency.add(latency);

    const success = check(response, {
        'status is 200 or 202': (r) => r.status === 200 || r.status === 202,
        'latency < 500ms': () => latency < 500,
    });

    if (success) {
        eventsSent.add(EVENTS_PER_BATCH);
        errorRate.add(0);
    } else {
        errorRate.add(1);
        console.log(`Error: ${response.status} - ${response.body}`);
    }
}

export function handleSummary(data) {
    const totalEvents = data.metrics.events_sent ? data.metrics.events_sent.values.count : 0;
    const duration = data.state.testRunDurationMs / 1000;
    const eps = totalEvents / duration;

    return {
        'stdout': `
================================================================================
BASELINE LOAD TEST RESULTS
================================================================================
Duration:           ${duration.toFixed(0)} seconds
Total Events:       ${totalEvents.toLocaleString()}
Events/Second:      ${eps.toFixed(0)} EPS
Target EPS:         5000

HTTP Request Stats:
  - p50 Latency:    ${data.metrics.http_req_duration?.values?.['p(50)']?.toFixed(2) || 'N/A'} ms
  - p95 Latency:    ${data.metrics.http_req_duration?.values?.['p(95)']?.toFixed(2) || 'N/A'} ms
  - p99 Latency:    ${data.metrics.http_req_duration?.values?.['p(99)']?.toFixed(2) || 'N/A'} ms
  - Failed:         ${data.metrics.http_req_failed?.values?.rate?.toFixed(4) || '0'}%

Result: ${eps >= 5000 ? '✅ PASSED' : '❌ FAILED'} (${eps.toFixed(0)} >= 5000 EPS)
================================================================================
`,
        'results/baseline_load.json': JSON.stringify(data, null, 2),
    };
}
