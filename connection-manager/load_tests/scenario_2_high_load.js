// Scenario 2: High Load Test (10000 EPS)
// Target: Push system to 2x baseline and verify graceful handling

import http from 'k6/http';
import { check } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

const eventsSent = new Counter('events_sent');
const batchLatency = new Trend('batch_latency', true);
const errorRate = new Rate('error_rate');

export const options = {
    scenarios: {
        high_load: {
            executor: 'ramping-arrival-rate',
            startRate: 100,
            timeUnit: '1s',
            preAllocatedVUs: 200,
            maxVUs: 500,
            stages: [
                { duration: '2m', target: 500 },   // Ramp to 5000 EPS
                { duration: '3m', target: 1000 },  // Push to 10000 EPS
                { duration: '3m', target: 1000 },  // Sustain 10000 EPS
                { duration: '2m', target: 100 },   // Ramp down
            ],
        },
    },
    thresholds: {
        http_req_duration: ['p(50)<200', 'p(95)<500', 'p(99)<1000'],
        http_req_failed: ['rate<0.005'],  // Allow 0.5% errors under high load
        error_rate: ['rate<0.005'],
    },
};

const API_HOST = __ENV.API_HOST || 'http://localhost:8080';
const EVENTS_PER_BATCH = 10;

function generateBatch(vuId) {
    return JSON.stringify({
        batch_id: `hb-${Date.now()}-${vuId}`,
        agent_id: `agent-${String(vuId % 1000).padStart(5, '0')}`, // 1000 unique agents
        events: Array(EVENTS_PER_BATCH).fill(null).map((_, i) => ({
            type: 'process_create',
            timestamp: new Date().toISOString(),
            data: { index: i }
        })),
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
            timeout: '15s',
        }
    );

    batchLatency.add(Date.now() - start);

    const success = check(response, {
        'status is 2xx': (r) => r.status >= 200 && r.status < 300,
    });

    if (success) {
        eventsSent.add(EVENTS_PER_BATCH);
        errorRate.add(0);
    } else {
        errorRate.add(1);
    }
}

export function handleSummary(data) {
    const totalEvents = data.metrics.events_sent?.values?.count || 0;
    const duration = data.state.testRunDurationMs / 1000;
    const peakEPS = totalEvents / duration;

    return {
        'stdout': `
================================================================================
HIGH LOAD TEST RESULTS (10000 EPS Target)
================================================================================
Duration:           ${duration.toFixed(0)} seconds
Total Events:       ${totalEvents.toLocaleString()}
Avg EPS:            ${peakEPS.toFixed(0)}

Latency:
  - p50:            ${data.metrics.http_req_duration?.values?.['p(50)']?.toFixed(2) || 'N/A'} ms
  - p99:            ${data.metrics.http_req_duration?.values?.['p(99)']?.toFixed(2) || 'N/A'} ms

Error Rate:         ${((data.metrics.error_rate?.values?.rate || 0) * 100).toFixed(3)}%

Result: ${peakEPS >= 9000 ? '✅ PASSED' : '⚠️ DEGRADED'} (Target: 90% of 10000 = 9000 EPS)
================================================================================
`,
        'results/high_load.json': JSON.stringify(data, null, 2),
    };
}
