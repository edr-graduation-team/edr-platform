// EDR Load Test Configuration
// Common settings and utilities for load testing

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Counter, Rate, Trend, Gauge } from 'k6/metrics';

// Custom metrics
export const eventsSent = new Counter('events_sent');
export const eventsPerSecond = new Gauge('events_per_second');
export const batchLatency = new Trend('batch_latency', true);
export const apiLatency = new Trend('api_latency', true);
export const errorRate = new Rate('error_rate');

// Configuration
export const config = {
    // Server endpoints
    grpcHost: __ENV.GRPC_HOST || 'localhost:50051',
    apiHost: __ENV.API_HOST || 'http://localhost:8080',
    
    // Default test parameters
    defaultVUs: 100,
    defaultDuration: '10m',
    
    // Rate limits
    targetEPS: 5000,
    maxEPS: 10000,
    
    // Thresholds
    maxP99Latency: 500,  // ms
    maxP50Latency: 100,  // ms
    maxErrorRate: 0.001, // 0.1%
};

// Common options for all tests
export const thresholds = {
    http_req_duration: ['p(50)<100', 'p(95)<300', 'p(99)<500'],
    http_req_failed: ['rate<0.001'],
    error_rate: ['rate<0.001'],
    events_per_second: ['value>5000'],
};

// Helper: Generate a random event batch
export function generateEventBatch(agentId, eventCount = 10) {
    const events = [];
    for (let i = 0; i < eventCount; i++) {
        events.push({
            event_type: ['process', 'file', 'network', 'registry'][Math.floor(Math.random() * 4)],
            timestamp: new Date().toISOString(),
            data: {
                process_name: `process_${Math.floor(Math.random() * 1000)}.exe`,
                pid: Math.floor(Math.random() * 65535),
                user: 'SYSTEM',
                command_line: '/c echo test',
            }
        });
    }
    
    return {
        batch_id: `batch-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`,
        agent_id: agentId,
        events: events,
        event_count: eventCount,
        compression: 'none',
        timestamp: new Date().toISOString(),
    };
}

// Helper: Generate random agent ID
export function generateAgentId(index) {
    return `agent-${String(index).padStart(5, '0')}`;
}

// Helper: Get auth token (mock for now)
export function getAuthToken() {
    return 'mock-jwt-token-for-load-testing';
}

// Helper: Make authenticated request
export function authRequest(method, url, body = null, params = {}) {
    const headers = {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${getAuthToken()}`,
        ...params.headers,
    };
    
    const options = {
        headers: headers,
        ...params,
    };
    
    let response;
    switch (method.toUpperCase()) {
        case 'GET':
            response = http.get(url, options);
            break;
        case 'POST':
            response = http.post(url, JSON.stringify(body), options);
            break;
        case 'PATCH':
            response = http.patch(url, JSON.stringify(body), options);
            break;
        case 'DELETE':
            response = http.del(url, null, options);
            break;
        default:
            response = http.request(method, url, JSON.stringify(body), options);
    }
    
    return response;
}

// Helper: Check response and record metrics
export function checkResponse(response, expectedStatus = 200, name = 'request') {
    const success = check(response, {
        [`${name} status is ${expectedStatus}`]: (r) => r.status === expectedStatus,
        [`${name} response time < 500ms`]: (r) => r.timings.duration < 500,
    });
    
    if (!success) {
        errorRate.add(1);
    } else {
        errorRate.add(0);
    }
    
    return success;
}
