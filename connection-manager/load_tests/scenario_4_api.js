// Scenario 4: API Endpoints Load Test
// Target: Test REST API endpoints under load

import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const errorRate = new Rate('error_rate');
const apiLatency = new Trend('api_latency', true);

export const options = {
    scenarios: {
        api_load: {
            executor: 'constant-vus',
            vus: 50,
            duration: '5m',
        },
    },
    thresholds: {
        http_req_duration: ['p(95)<300'],
        http_req_failed: ['rate<0.01'],
    },
};

const API_HOST = __ENV.API_HOST || 'http://localhost:8080';
const AUTH_HEADER = {
    'Authorization': 'Bearer test-token',
    'Content-Type': 'application/json',
};

export default function () {
    // Health check
    group('Health Endpoints', function () {
        let res = http.get(`${API_HOST}/healthz`);
        check(res, { 'health ok': (r) => r.status === 200 });
    });

    // Agent endpoints
    group('Agent Endpoints', function () {
        // List agents
        let res = http.get(`${API_HOST}/api/v1/agents?limit=50`, { headers: AUTH_HEADER });
        check(res, { 'list agents ok': (r) => r.status === 200 });
        apiLatency.add(res.timings.duration);

        // Agent stats
        res = http.get(`${API_HOST}/api/v1/agents/stats`, { headers: AUTH_HEADER });
        check(res, { 'agent stats ok': (r) => r.status === 200 });
    });

    // Alert endpoints
    group('Alert Endpoints', function () {
        // List alerts
        let res = http.get(`${API_HOST}/api/v1/alerts?limit=50`, { headers: AUTH_HEADER });
        check(res, { 'list alerts ok': (r) => r.status === 200 });

        // Alert stats
        res = http.get(`${API_HOST}/api/v1/alerts/stats`, { headers: AUTH_HEADER });
        check(res, { 'alert stats ok': (r) => r.status === 200 });

        // Search alerts
        res = http.post(
            `${API_HOST}/api/v1/alerts/search`,
            JSON.stringify({ severity: ['high', 'critical'], limit: 20 }),
            { headers: AUTH_HEADER }
        );
        check(res, { 'search alerts ok': (r) => r.status === 200 });
    });

    // Event endpoints
    group('Event Endpoints', function () {
        let res = http.get(`${API_HOST}/api/v1/events/stats`, { headers: AUTH_HEADER });
        check(res, { 'event stats ok': (r) => r.status === 200 });

        res = http.post(
            `${API_HOST}/api/v1/events/search`,
            JSON.stringify({ filters: [], limit: 100 }),
            { headers: AUTH_HEADER }
        );
        check(res, { 'search events ok': (r) => r.status === 200 });
    });

    // Policy endpoints
    group('Policy Endpoints', function () {
        let res = http.get(`${API_HOST}/api/v1/policies`, { headers: AUTH_HEADER });
        check(res, { 'list policies ok': (r) => r.status === 200 });
    });

    sleep(0.5);
}

export function handleSummary(data) {
    return {
        'stdout': `
================================================================================
API ENDPOINTS LOAD TEST RESULTS
================================================================================
Virtual Users:      50
Duration:           5 minutes

Request Stats:
  - Total:          ${data.metrics.http_reqs?.values?.count || 0}
  - p95 Latency:    ${data.metrics.http_req_duration?.values?.['p(95)']?.toFixed(2) || 'N/A'} ms
  - Failed:         ${((data.metrics.http_req_failed?.values?.rate || 0) * 100).toFixed(3)}%

Result: ${(data.metrics.http_req_failed?.values?.rate || 0) < 0.01 ? '✅ PASSED' : '❌ FAILED'}
================================================================================
`,
        'results/api_load.json': JSON.stringify(data, null, 2),
    };
}
