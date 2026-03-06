// Scenario 3: Burst Load Test
// Target: Handle sudden 2x traffic spike for 5 minutes

import http from 'k6/http';
import { check } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

const eventsSent = new Counter('events_sent');
const errorRate = new Rate('error_rate');

export const options = {
    scenarios: {
        burst_load: {
            executor: 'ramping-arrival-rate',
            startRate: 500,          // Start at 5000 EPS
            timeUnit: '1s',
            preAllocatedVUs: 200,
            maxVUs: 400,
            stages: [
                { duration: '1m', target: 500 },   // Baseline
                { duration: '10s', target: 1000 }, // Sudden spike to 10000 EPS
                { duration: '5m', target: 1000 },  // Sustain burst
                { duration: '30s', target: 500 },  // Return to baseline
                { duration: '2m', target: 500 },   // Stabilize
            ],
        },
    },
    thresholds: {
        http_req_duration: ['p(50)<500', 'p(99)<2000'],
        error_rate: ['rate<0.01'],  // Allow 1% errors during burst
    },
};

const API_HOST = __ENV.API_HOST || 'http://localhost:8080';

function generateBatch() {
    return JSON.stringify({
        batch_id: `burst-${Date.now()}-${Math.random().toString(36).substr(2, 6)}`,
        agent_id: `agent-${String(__VU % 100).padStart(5, '0')}`,
        events: Array(10).fill({ type: 'burst_event', ts: Date.now() }),
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
            timeout: '20s',
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
    const errorPct = ((data.metrics.error_rate?.values?.rate || 0) * 100).toFixed(3);
    return {
        'stdout': `
================================================================================
BURST LOAD TEST RESULTS
================================================================================
Total Events:       ${(data.metrics.events_sent?.values?.count || 0).toLocaleString()}
Error Rate:         ${errorPct}%
p50 Latency:        ${data.metrics.http_req_duration?.values?.['p(50)']?.toFixed(0) || 'N/A'} ms
p99 Latency:        ${data.metrics.http_req_duration?.values?.['p(99)']?.toFixed(0) || 'N/A'} ms

Result: ${parseFloat(errorPct) < 1 ? '✅ PASSED' : '❌ FAILED'} (Error rate < 1%)
================================================================================
`,
        'results/burst_load.json': JSON.stringify(data, null, 2),
    };
}
