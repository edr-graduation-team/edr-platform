import axios, { type AxiosInstance } from 'axios';

// Configuration - all from environment variables
// IMPORTANT: When VITE_* vars are set to "" (empty string) via Docker build args,
// we must use "" as the baseURL so Axios sends RELATIVE requests (same-origin).
// Nginx then proxies /api/v1/* to the correct backend.
// The localhost fallbacks are ONLY for local `npm run dev` outside Docker.
const envOrDefault = (envVal: string | undefined, fallback: string): string => {
    // undefined = env var not set at all (local dev) → use fallback
    // ""        = explicitly set to empty (Docker)   → use "" (same-origin)
    return envVal === undefined ? fallback : envVal;
};

const config = {
    sigmaEngineUrl: envOrDefault(import.meta.env.VITE_API_URL, 'http://localhost:8080'),
    connectionManagerUrl: envOrDefault(import.meta.env.VITE_CONNECTION_MANAGER_URL, 'http://localhost:8082'),
    wsUrl: envOrDefault(import.meta.env.VITE_WS_URL, ''),
};

// Create axios instance for Sigma Engine
const createApiClient = (baseURL: string): AxiosInstance => {
    const client = axios.create({
        baseURL,
        timeout: 10000,
        headers: { 'Content-Type': 'application/json' },
    });

    // Request interceptor for auth
    client.interceptors.request.use((cfg) => {
        const token = localStorage.getItem('auth_token');
        if (token) {
            cfg.headers.Authorization = `Bearer ${token}`;
        }
        return cfg;
    });

    // Response interceptor for error handling
    client.interceptors.response.use(
        (response) => response,
        (error) => {
            if (error.response?.status === 401) {
                localStorage.removeItem('auth_token');
                localStorage.removeItem('user');
                window.location.href = '/login';
            }
            return Promise.reject(error);
        }
    );

    return client;
};

// API clients for each service
export const sigmaApi = createApiClient(config.sigmaEngineUrl);
export const connectionApi = createApiClient(config.connectionManagerUrl);

// For backward compatibility
export const api = sigmaApi;

// ============================================================================
// API Types
// ============================================================================

export type AlertSeverity = 'critical' | 'high' | 'medium' | 'low' | 'informational';
export type AlertStatus = 'open' | 'in_progress' | 'acknowledged' | 'resolved' | 'closed' | 'false_positive';
export type AgentStatus = 'online' | 'offline' | 'degraded' | 'pending' | 'suspended';
export type CommandType = 'kill_process' | 'quarantine_file' | 'collect_logs' | 'update_policy' |
    'restart_agent' | 'isolate_network' | 'restore_network' | 'scan_file' | 'scan_memory' | 'custom' | 'restart_machine' | 'shutdown_machine' | 'update_filter_policy';
export type CommandStatus = 'pending' | 'sent' | 'acknowledged' | 'executing' | 'completed' | 'failed' | 'timeout' | 'cancelled';

export interface Alert {
    id: string;
    timestamp: string;
    agent_id: string;
    rule_id: string;
    rule_title: string;
    severity: AlertSeverity;
    category: string;
    event_count: number;
    status: AlertStatus;
    confidence: number;
    matched_fields?: Record<string, string>;
    mitre_techniques?: string[];
    mitre_tactics?: string[];
    event_data?: Record<string, unknown>;
    tags?: Record<string, string>;
    notes?: string;
    assigned_to?: string;
    acknowledged_at?: string;
    resolved_at?: string;
    created_at: string;
    updated_at: string;
}

export interface Rule {
    id: string;
    title: string;
    description: string;
    severity: string;
    category: string;
    product: string;
    enabled: boolean;
    status: string;
    tags?: string[];
    mitre_attack?: {
        tactics: string[];
        techniques: string[];
    };
    created_at: string;
    updated_at: string;
}

export interface AlertStats {
    total_alerts: number;
    by_severity: Record<string, number>;
    by_status: Record<string, number>;
    alerts_24h: number;
    alerts_7d: number;
    avg_confidence: number;
}

export interface RuleStats {
    total_rules: number;
    enabled_rules: number;
    disabled_rules: number;
    by_severity: Record<string, number>;
}

export interface Agent {
    id: string;
    hostname: string;
    status: AgentStatus;
    os_type: 'windows' | 'linux' | 'macos';
    os_version: string;
    agent_version: string;
    last_seen: string;
    health_score: number;
    events_delivered: number;
    events_collected?: number;
    events_dropped?: number;
    ip_addresses?: string[];
    cpu_count?: number;
    memory_mb?: number;
    cpu_usage?: number;
    memory_used_mb?: number;
    queue_depth?: number;
    current_cert_id?: string;
    cert_expires_at?: string;
    tags?: Record<string, string>;
    metadata?: Record<string, string>;
    installed_date?: string;
    created_at: string;
    updated_at: string;
}

// Filter policy for dynamic agent-side event filtering
export interface FilterPolicy {
    exclude_processes?: string[];
    exclude_ips?: string[];
    exclude_event_ids?: number[];
    trusted_hashes?: string[];
    exclude_registry?: string[];
    exclude_paths?: string[];
    include_paths?: string[];
    rate_limit?: {
        enabled: boolean;
        default_max_eps: number;
        critical_bypass: boolean;
        per_event_type?: Record<string, number>;
    };
}

export interface AgentStats {
    total: number;
    online: number;
    offline: number;
    degraded: number;
    pending: number;
    suspended: number;
    by_os_type: Record<string, number>;
    by_version: Record<string, number>;
    avg_health: number;
}

export interface Command {
    id: string;
    agent_id: string;
    command_type: CommandType;
    parameters: Record<string, string>;
    status: CommandStatus;
    result?: Record<string, unknown>;
    error_message?: string;
    exit_code?: number;
    issued_at: string;
    issued_by: string;
    sent_at?: string;
    acknowledged_at?: string;
    started_at?: string;
    completed_at?: string;
    timeout_seconds: number;
}

export interface CommandRequest {
    command_type: CommandType;
    parameters: Record<string, string>;
    timeout?: number;
}

export interface AuditLog {
    id: string;
    user_id: string;
    username: string;
    action: string;
    resource_type: string;
    resource_id?: string;
    old_value?: Record<string, unknown>;
    new_value?: Record<string, unknown>;
    details?: string;
    result: 'success' | 'failure';
    error_message?: string;
    ip_address?: string;
    user_agent?: string;
    timestamp: string;
}

export interface TimelineDataPoint {
    timestamp: string;
    critical: number;
    high: number;
    medium: number;
    low: number;
    informational: number;
}

export interface User {
    id: string;
    username: string;
    email?: string;
    full_name?: string;
    role: 'admin' | 'security' | 'analyst' | 'operations' | 'viewer';
    status: 'active' | 'inactive' | 'locked';
    last_login?: string;
}

// ============================================================================
// Sigma Engine Alert APIs
// ============================================================================

export const alertsApi = {
    list: async (params?: {
        limit?: number;
        offset?: number;
        severity?: string;
        status?: string;
        agent_id?: string;
        rule_id?: string;
        from?: string;
        to?: string;
        sort?: string;
        order?: 'asc' | 'desc';
    }) => {
        const response = await sigmaApi.get<{ alerts: Alert[]; total: number }>('/api/v1/sigma/alerts', { params });
        return response.data;
    },
    get: async (id: string) => {
        const response = await sigmaApi.get<Alert>(`/api/v1/sigma/alerts/${id}`);
        return response.data;
    },
    updateStatus: async (id: string, status: string, notes?: string) => {
        const response = await sigmaApi.patch(`/api/v1/sigma/alerts/${id}/status`, { status, notes });
        return response.data;
    },
    acknowledge: async (id: string) => {
        const response = await sigmaApi.post(`/api/v1/sigma/alerts/${id}/acknowledge`);
        return response.data;
    },
    delete: async (id: string) => {
        await sigmaApi.delete(`/api/v1/sigma/alerts/${id}`);
    },
    bulkUpdateStatus: async (ids: string[], status: string) => {
        const response = await sigmaApi.patch('/api/v1/sigma/alerts/bulk/status', { ids, status });
        return response.data;
    },
};

// ============================================================================
// Sigma Engine Rules APIs
// ============================================================================

export const rulesApi = {
    list: async (params?: { limit?: number; offset?: number; enabled?: boolean; severity?: string }) => {
        const response = await sigmaApi.get<{ rules: Rule[]; total: number }>('/api/v1/sigma/rules', { params });
        return response.data;
    },
    get: async (id: string) => {
        const response = await sigmaApi.get<Rule>(`/api/v1/sigma/rules/${id}`);
        return response.data;
    },
    create: async (rule: Partial<Rule>) => {
        const response = await sigmaApi.post<Rule>('/api/v1/sigma/rules', rule);
        return response.data;
    },
    update: async (id: string, rule: Partial<Rule>) => {
        const response = await sigmaApi.put<Rule>(`/api/v1/sigma/rules/${id}`, rule);
        return response.data;
    },
    delete: async (id: string) => {
        await sigmaApi.delete(`/api/v1/sigma/rules/${id}`);
    },
    enable: async (id: string) => {
        const response = await sigmaApi.patch(`/api/v1/sigma/rules/${id}/enable`);
        return response.data;
    },
    disable: async (id: string) => {
        const response = await sigmaApi.patch(`/api/v1/sigma/rules/${id}/disable`);
        return response.data;
    },
};

// ============================================================================
// Sigma Engine Stats APIs
// ============================================================================

export const statsApi = {
    alerts: async () => {
        const response = await sigmaApi.get<AlertStats>('/api/v1/sigma/stats/alerts');
        return response.data;
    },
    rules: async () => {
        const response = await sigmaApi.get<RuleStats>('/api/v1/sigma/stats/rules');
        return response.data;
    },
    performance: async () => {
        const response = await sigmaApi.get('/api/v1/sigma/stats/performance');
        return response.data;
    },
    timeline: async (params: { from: string; to: string; granularity?: string }) => {
        const response = await sigmaApi.get<{ data: TimelineDataPoint[] }>('/api/v1/sigma/stats/timeline', { params });
        return response.data;
    },
};

// ============================================================================
// Connection Manager Agents APIs
// ============================================================================

export const agentsApi = {
    list: async (params?: {
        limit?: number;
        offset?: number;
        status?: string;
        os_type?: string;
        search?: string;
        sort_by?: string;
        sort_order?: 'asc' | 'desc';
    }) => {
        const response = await connectionApi.get<{ data: Agent[]; pagination: { total: number; has_more: boolean } }>('/api/v1/agents', { params });
        return response.data;
    },
    get: async (id: string) => {
        const response = await connectionApi.get<{ data: Agent }>(`/api/v1/agents/${id}`);
        return response.data.data;
    },
    stats: async () => {
        const response = await connectionApi.get<AgentStats>('/api/v1/agents/stats');
        return response.data;
    },
    update: async (id: string, data: { tags?: Record<string, string> }) => {
        const response = await connectionApi.patch(`/api/v1/agents/${id}`, data);
        return response.data;
    },
    delete: async (id: string) => {
        await connectionApi.delete(`/api/v1/agents/${id}`);
    },
    health: async () => {
        const response = await connectionApi.get('/health');
        return response.data;
    },
    // Commands
    executeCommand: async (agentId: string, command: CommandRequest) => {
        const response = await connectionApi.post<{ command_id: string; status: string; issued_at: string }>(
            `/api/v1/agents/${agentId}/commands`,
            command
        );
        return response.data;
    },
    getCommands: async (agentId: string, params?: { limit?: number; offset?: number; status?: string }) => {
        const response = await connectionApi.get<{ data: Command[]; pagination: { total: number } }>(
            `/api/v1/agents/${agentId}/commands`,
            { params }
        );
        return response.data;
    },
    getCommand: async (agentId: string, commandId: string) => {
        const response = await connectionApi.get<Command>(`/api/v1/agents/${agentId}/commands/${commandId}`);
        return response.data;
    },
    cancelCommand: async (agentId: string, commandId: string) => {
        const response = await connectionApi.post(`/api/v1/agents/${agentId}/commands/${commandId}/cancel`);
        return response.data;
    },
    // Push a new filter policy to an agent via the C2 command pipeline
    updateFilterPolicy: async (agentId: string, policy: FilterPolicy) => {
        const response = await connectionApi.post<{ command_id: string; status: string; issued_at: string }>(
            `/api/v1/agents/${agentId}/commands`,
            {
                command_type: 'update_filter_policy',
                parameters: { policy: JSON.stringify(policy) },
                timeout: 300,
            }
        );
        return response.data;
    },
};

// ============================================================================
// Audit Logs API
// ============================================================================

export const auditApi = {
    list: async (params?: {
        limit?: number;
        offset?: number;
        user_id?: string;
        action?: string;
        resource_type?: string;
        from?: string;
        to?: string;
    }) => {
        const response = await connectionApi.get<{ data: AuditLog[]; pagination: { total: number } }>(
            '/api/v1/audit/logs',
            { params }
        );
        return response.data;
    },
    get: async (id: string) => {
        const response = await connectionApi.get<AuditLog>(`/api/v1/audit/logs/${id}`);
        return response.data;
    },
};

// ============================================================================
// Enrollment Tokens API
// ============================================================================

export interface EnrollmentToken {
    id: string;
    token: string;
    description: string;
    is_active: boolean;
    expires_at: string | null;
    use_count: number;
    max_uses: number | null;
    created_by: string;
    created_at: string;
    revoked_at: string | null;
}

export const enrollmentTokensApi = {
    list: async () => {
        const response = await connectionApi.get<{ data: EnrollmentToken[] }>(
            '/api/v1/enrollment-tokens'
        );
        return response.data;
    },
    generate: async (data: { description: string; expires_in_hours?: number; max_uses?: number }) => {
        const response = await connectionApi.post<EnrollmentToken>(
            '/api/v1/enrollment-tokens',
            data
        );
        return response.data;
    },
    revoke: async (id: string) => {
        const response = await connectionApi.post(`/api/v1/enrollment-tokens/${id}/revoke`);
        return response.data;
    },
};

// ============================================================================
// WebSocket for real-time alerts
// ============================================================================

export function createAlertStream(
    onMessage: (alert: Alert) => void,
    filters?: { severity?: string[]; agent_id?: string; rule_id?: string }
) {
    // Build WebSocket URL: in Docker mode (empty config), use current browser origin
    let wsUrl = config.wsUrl;
    if (!wsUrl) {
        if (config.sigmaEngineUrl) {
            // Local dev: replace http(s) with ws(s)
            wsUrl = config.sigmaEngineUrl.replace(/^http/, 'ws') + '/api/v1/sigma/alerts/stream';
        } else {
            // Docker/nginx mode: derive from current page origin
            const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            wsUrl = `${proto}//${window.location.host}/api/v1/sigma/alerts/stream`;
        }
    }

    let ws: WebSocket;
    let reconnectAttempts = 0;
    const maxReconnectAttempts = 5;
    let reconnectTimeout: ReturnType<typeof setTimeout>;
    let pingInterval: ReturnType<typeof setInterval>;

    const connect = () => {
        ws = new WebSocket(wsUrl);

        ws.onopen = () => {
            reconnectAttempts = 0;
            console.log('WebSocket connected');

            // Subscribe with filters
            if (filters) {
                ws.send(JSON.stringify({ type: 'subscribe', filters }));
            }

            // Start heartbeat
            pingInterval = setInterval(() => {
                if (ws.readyState === WebSocket.OPEN) {
                    ws.send(JSON.stringify({ type: 'ping' }));
                }
            }, 30000);
        };

        ws.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);
                if (message.type === 'alert') {
                    onMessage(message.data);
                }
            } catch (e) {
                console.error('Failed to parse WebSocket message:', e);
            }
        };

        ws.onerror = (error) => {
            console.error('WebSocket error:', error);
        };

        ws.onclose = () => {
            console.log('WebSocket disconnected');
            clearInterval(pingInterval);

            // Auto-reconnect with exponential backoff
            if (reconnectAttempts < maxReconnectAttempts) {
                const delay = Math.pow(2, reconnectAttempts) * 1000;
                reconnectTimeout = setTimeout(() => {
                    reconnectAttempts++;
                    console.log(`Reconnecting... attempt ${reconnectAttempts}`);
                    connect();
                }, delay);
            }
        };
    };

    connect();

    // Return cleanup function
    return {
        close: () => {
            clearTimeout(reconnectTimeout);
            clearInterval(pingInterval);
            ws.close();
        },
        getState: () => ws.readyState,
    };
}

// ============================================================================
// Auth API
// ============================================================================

export const authApi = {
    login: async (username: string, password: string) => {
        const response = await connectionApi.post<{
            access_token: string;
            refresh_token?: string;
            expires_in: number;
            token_type: string;
            user: User;
        }>('/api/v1/auth/login', { username, password });

        if (response.data.access_token) {
            localStorage.setItem('auth_token', response.data.access_token);
            localStorage.setItem('user', JSON.stringify(response.data.user));
        }
        return response.data;
    },
    logout: () => {
        localStorage.removeItem('auth_token');
        localStorage.removeItem('user');
    },
    getCurrentUser: (): User | null => {
        const user = localStorage.getItem('user');
        return user ? JSON.parse(user) : null;
    },
    isAuthenticated: () => !!localStorage.getItem('auth_token'),
    hasRole: (roles: string[]) => {
        const user = authApi.getCurrentUser();
        return user ? roles.includes(user.role) : false;
    },
    canExecuteCommands: () => {
        return authApi.hasRole(['admin', 'security', 'analyst']);
    },
    canViewAuditLogs: () => {
        return authApi.hasRole(['admin', 'security']);
    },
    canManageUsers: () => {
        return authApi.hasRole(['admin']);
    },
};

