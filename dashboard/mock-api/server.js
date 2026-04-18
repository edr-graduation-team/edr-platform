/**
 * Mock API Server for EDR Dashboard Development
 * This provides sample data for testing the dashboard without running actual backends
 */

import http from 'http';
import { URL } from 'url';

const PORT = 8080;

// Sample mock data
const mockAlerts = [
    {
        id: "alert-001",
        timestamp: new Date().toISOString(),
        agent_id: "agent-001",
        rule_id: "win-proc-suspicious-image",
        rule_title: "Suspicious Process Creation",
        severity: "critical",
        category: "process_creation",
        event_count: 3,
        status: "open",
        confidence: 0.95,
        mitre_tactics: ["execution", "defense-evasion"],
        mitre_techniques: ["T1059.001", "T1055"],
        matched_fields: { "Image": "C:\\Windows\\System32\\cmd.exe", "CommandLine": "powershell -enc..." },
        created_at: new Date(Date.now() - 3600000).toISOString(),
        updated_at: new Date().toISOString()
    },
    {
        id: "alert-002",
        timestamp: new Date(Date.now() - 1800000).toISOString(),
        agent_id: "agent-002",
        rule_id: "win-proc-lsass-access",
        rule_title: "LSASS Memory Access - Credential Dumping",
        severity: "high",
        category: "process_access",
        event_count: 1,
        status: "acknowledged",
        confidence: 0.87,
        mitre_tactics: ["credential-access"],
        mitre_techniques: ["T1003.001"],
        matched_fields: { "TargetImage": "C:\\Windows\\System32\\lsass.exe" },
        created_at: new Date(Date.now() - 7200000).toISOString(),
        updated_at: new Date(Date.now() - 1800000).toISOString()
    },
    {
        id: "alert-003",
        timestamp: new Date(Date.now() - 900000).toISOString(),
        agent_id: "agent-001",
        rule_id: "win-net-lateral-movement",
        rule_title: "Lateral Movement via SMB",
        severity: "medium",
        category: "network_connection",
        event_count: 5,
        status: "open",
        confidence: 0.72,
        mitre_tactics: ["lateral-movement"],
        mitre_techniques: ["T1021.002"],
        matched_fields: { "DestinationPort": "445" },
        created_at: new Date(Date.now() - 3600000).toISOString(),
        updated_at: new Date().toISOString()
    },
    {
        id: "alert-004",
        timestamp: new Date(Date.now() - 600000).toISOString(),
        agent_id: "agent-003",
        rule_id: "win-file-ransomware-indicator",
        rule_title: "Ransomware File Modification Pattern",
        severity: "critical",
        category: "file_event",
        event_count: 50,
        status: "open",
        confidence: 0.98,
        mitre_tactics: ["impact"],
        mitre_techniques: ["T1486"],
        matched_fields: { "TargetFilename": "*.encrypted" },
        created_at: new Date(Date.now() - 1200000).toISOString(),
        updated_at: new Date().toISOString()
    },
    {
        id: "alert-005",
        timestamp: new Date(Date.now() - 300000).toISOString(),
        agent_id: "agent-002",
        rule_id: "win-proc-powershell-encoded",
        rule_title: "Encoded PowerShell Execution",
        severity: "high",
        category: "process_creation",
        event_count: 1,
        status: "open",
        confidence: 0.82,
        mitre_tactics: ["execution"],
        mitre_techniques: ["T1059.001"],
        matched_fields: { "CommandLine": "powershell.exe -encodedCommand" },
        created_at: new Date(Date.now() - 600000).toISOString(),
        updated_at: new Date().toISOString()
    }
];

const mockRules = [
    { id: "win-proc-suspicious-image", title: "Suspicious Process Creation", severity: "critical", category: "process_creation", enabled: true, status: "stable", description: "Detects suspicious process creation patterns" },
    { id: "win-proc-lsass-access", title: "LSASS Memory Access - Credential Dumping", severity: "high", category: "process_access", enabled: true, status: "stable", description: "Detects access to LSASS memory for credential theft" },
    { id: "win-net-lateral-movement", title: "Lateral Movement via SMB", severity: "medium", category: "network_connection", enabled: true, status: "stable", description: "Detects lateral movement using SMB protocol" },
    { id: "win-file-ransomware-indicator", title: "Ransomware File Modification Pattern", severity: "critical", category: "file_event", enabled: true, status: "experimental", description: "Detects ransomware file encryption patterns" },
    { id: "win-proc-powershell-encoded", title: "Encoded PowerShell Execution", severity: "high", category: "process_creation", enabled: true, status: "stable", description: "Detects encoded PowerShell commands" }
];

// Generate timeline data
function generateTimeline() {
    const timeline = [];
    for (let i = 24; i >= 0; i--) {
        const time = new Date(Date.now() - i * 3600000);
        timeline.push({
            time: time.toISOString(),
            critical: Math.floor(Math.random() * 3),
            high: Math.floor(Math.random() * 5),
            medium: Math.floor(Math.random() * 8),
            low: Math.floor(Math.random() * 10),
            informational: Math.floor(Math.random() * 15)
        });
    }
    return timeline;
}

// CORS headers
function setCORSHeaders(res) {
    res.setHeader('Access-Control-Allow-Origin', '*');
    res.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, PATCH, DELETE, OPTIONS');
    res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization');
    res.setHeader('Content-Type', 'application/json');
}

// Parse request body
function parseBody(req) {
    return new Promise((resolve) => {
        let body = '';
        req.on('data', chunk => body += chunk);
        req.on('end', () => {
            try {
                resolve(JSON.parse(body));
            } catch {
                resolve({});
            }
        });
    });
}

// Route handler
async function handleRequest(req, res) {
    setCORSHeaders(res);

    if (req.method === 'OPTIONS') {
        res.writeHead(200);
        res.end();
        return;
    }

    const parsedUrl = new URL(req.url, `http://51.21.199.229:${PORT}`);
    const path = parsedUrl.pathname;

    console.log(`${new Date().toISOString()} ${req.method} ${path}`);

    // === Health Check ===
    if (path === '/health') {
        res.writeHead(200);
        res.end(JSON.stringify({ status: 'healthy' }));
        return;
    }

    // === Alerts Endpoints ===
    if (path === '/api/v1/sigma/alerts' && req.method === 'GET') {
        res.writeHead(200);
        res.end(JSON.stringify({
            count: mockAlerts.length,
            total: mockAlerts.length,
            limit: 100,
            offset: 0,
            alerts: mockAlerts
        }));
        return;
    }

    if (path === '/api/v1/sigma/alerts/stream') {
        // SSE endpoint for live alerts
        res.writeHead(200, {
            'Content-Type': 'text/event-stream',
            'Cache-Control': 'no-cache',
            'Connection': 'keep-alive',
            'Access-Control-Allow-Origin': '*'
        });
        res.write(`data: ${JSON.stringify({ type: 'connected' })}\n\n`);
        // Keep connection alive
        const interval = setInterval(() => {
            res.write(`data: ${JSON.stringify({ type: 'heartbeat', time: new Date().toISOString() })}\n\n`);
        }, 30000);
        req.on('close', () => clearInterval(interval));
        return;
    }

    // Match PATCH /api/v1/sigma/alerts/:id/status
    const statusMatch = path.match(/^\/api\/v1\/sigma\/alerts\/([^/]+)\/status$/);
    if (statusMatch && req.method === 'PATCH') {
        const alertId = statusMatch[1];
        const alert = mockAlerts.find(a => a.id === alertId);
        if (alert) {
            const body = await parseBody(req);
            alert.status = body.status || alert.status;
            alert.updated_at = new Date().toISOString();
            res.writeHead(200);
            res.end(JSON.stringify(alert));
        } else {
            res.writeHead(404);
            res.end(JSON.stringify({ error: 'Alert not found' }));
        }
        return;
    }

    // Match GET /api/v1/sigma/alerts/:id
    const alertMatch = path.match(/^\/api\/v1\/sigma\/alerts\/([^/]+)$/);
    if (alertMatch && req.method === 'GET') {
        const alertId = alertMatch[1];
        const alert = mockAlerts.find(a => a.id === alertId);
        if (alert) {
            res.writeHead(200);
            res.end(JSON.stringify(alert));
        } else {
            res.writeHead(404);
            res.end(JSON.stringify({ error: 'Alert not found' }));
        }
        return;
    }

    // === Rules Endpoints ===
    if (path === '/api/v1/sigma/rules' && req.method === 'GET') {
        res.writeHead(200);
        res.end(JSON.stringify({
            count: mockRules.length,
            total: mockRules.length,
            rules: mockRules
        }));
        return;
    }

    // === Stats Endpoints ===
    if (path === '/api/v1/sigma/stats' || path === '/api/v1/sigma/stats/alerts') {
        const stats = {
            total_alerts: mockAlerts.length,
            alerts_by_severity: {
                critical: mockAlerts.filter(a => a.severity === 'critical').length,
                high: mockAlerts.filter(a => a.severity === 'high').length,
                medium: mockAlerts.filter(a => a.severity === 'medium').length,
                low: mockAlerts.filter(a => a.severity === 'low').length,
                informational: 0
            },
            alerts_by_status: {
                open: mockAlerts.filter(a => a.status === 'open').length,
                acknowledged: mockAlerts.filter(a => a.status === 'acknowledged').length,
                resolved: mockAlerts.filter(a => a.status === 'resolved').length,
                closed: 0
            },
            total_rules: mockRules.length,
            active_rules: mockRules.filter(r => r.enabled).length,
            avg_confidence: mockAlerts.reduce((sum, a) => sum + a.confidence, 0) / mockAlerts.length,
            events_processed_24h: 12543,
            detection_rate: 0.003
        };
        res.writeHead(200);
        res.end(JSON.stringify(stats));
        return;
    }

    if (path === '/api/v1/sigma/stats/rules') {
        res.writeHead(200);
        res.end(JSON.stringify({
            total: mockRules.length,
            enabled: mockRules.filter(r => r.enabled).length,
            by_severity: {
                critical: mockRules.filter(r => r.severity === 'critical').length,
                high: mockRules.filter(r => r.severity === 'high').length,
                medium: mockRules.filter(r => r.severity === 'medium').length,
                low: 0
            }
        }));
        return;
    }

    if (path === '/api/v1/sigma/stats/timeline') {
        res.writeHead(200);
        res.end(JSON.stringify({ data: generateTimeline() }));
        return;
    }

    // === 404 ===
    res.writeHead(404);
    res.end(JSON.stringify({ error: 'Not found', path: path, method: req.method }));
}

// Create server
const server = http.createServer(handleRequest);

server.listen(PORT, () => {
    console.log(`========================================`);
    console.log(`Mock API Server running on http://51.21.199.229:${PORT}`);
    console.log(`========================================`);
    console.log(`Endpoints:`);
    console.log(`  GET  /health                       - Health check`);
    console.log(`  GET  /api/v1/sigma/alerts          - List alerts`);
    console.log(`  GET  /api/v1/sigma/alerts/:id      - Get alert`);
    console.log(`  PATCH /api/v1/sigma/alerts/:id/status - Update status`);
    console.log(`  GET  /api/v1/sigma/alerts/stream   - SSE live alerts`);
    console.log(`  GET  /api/v1/sigma/rules           - List rules`);
    console.log(`  GET  /api/v1/sigma/stats           - Get stats`);
    console.log(`  GET  /api/v1/sigma/stats/alerts    - Alert stats`);
    console.log(`  GET  /api/v1/sigma/stats/rules     - Rule stats`);
    console.log(`  GET  /api/v1/sigma/stats/timeline  - Timeline data`);
    console.log(`========================================`);
});
