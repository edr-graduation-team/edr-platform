# 🖥️ SOC Dashboard — A Microscopic Deep Dive

> **Component:** `dashboard/` (React 19 + TypeScript + Vite)
> **Purpose:** Unified Security Operations Center interface for the Enterprise EDR Platform

---

## 1. Component-Based Architecture (React & TypeScript)

### 1.1 Architectural Layers

The dashboard follows a **four-layer architecture** that cleanly separates concerns:

```
┌─────────────────────────────────────────────┐
│  Pages (Views)      — Route-level screens   │
│  ├── Dashboard, Alerts, Endpoints, Threats  │
│  ├── Stats, Rules, AuditLogs, Settings      │
│  └── Login, EnrollmentTokens                │
├─────────────────────────────────────────────┤
│  Components (UI)    — Reusable primitives   │
│  ├── Modal, Toast, Skeleton, MultiSelect    │
│  └── DateRangePicker                        │
├─────────────────────────────────────────────┤
│  API Service Layer  — client.ts (582 lines) │
│  ├── Dual Axios Clients (sigma / connection)│
│  ├── JWT Interceptors                       │
│  └── WebSocket Stream Factory               │
├─────────────────────────────────────────────┤
│  State Management   — TanStack React Query  │
│  └── QueryClient (staleTime: 30s, retry: 1)│
└─────────────────────────────────────────────┘
```

**Key design decisions:**

- **No Redux, no Context API for server state.** The team opted for **TanStack React Query v5** (`@tanstack/react-query`) as the sole server-state manager. This eliminates the boilerplate of Redux slices while providing built-in caching, automatic refetching, and query invalidation — which are critical for a near-real-time security dashboard.
- **Lazy loading with `React.lazy()`** for every page component, paired with a `<Suspense>` boundary and a [PageLoader](file:///d:/EDR_Platform/dashboard/src/App.tsx#34-42) spinner. This keeps the initial bundle lean while splitting code per route.
- **Barrel exports** via [components/index.ts](file:///d:/EDR_Platform/dashboard/src/components/index.ts) for a clean import surface.

### 1.2 TypeScript Interfaces — End-to-End Type Safety

Every backend domain model is mirrored as a **strongly-typed TypeScript interface** in [client.ts](file:///d:/EDR_Platform/dashboard/src/api/client.ts). This creates a compile-time contract between the frontend and the backend REST API:

```typescript
// Union literal types enforce valid enum values at compile time
export type AlertSeverity = 'critical' | 'high' | 'medium' | 'low' | 'informational';
export type AlertStatus   = 'open' | 'in_progress' | 'acknowledged' | 'resolved' | 'closed' | 'false_positive';
export type CommandType   = 'kill_process' | 'quarantine_file' | 'collect_logs' | 'isolate_network' | ...;
export type CommandStatus = 'pending' | 'sent' | 'acknowledged' | 'executing' | 'completed' | 'failed' | 'timeout';

// Domain model interface — mirrors the PostgreSQL/JSONB structure exactly
export interface Alert {
    id: string;
    timestamp: string;
    agent_id: string;
    rule_title: string;
    severity: AlertSeverity;
    status: AlertStatus;
    confidence: number;
    matched_fields?: Record<string, string>;     // JSONB
    event_data?: Record<string, unknown>;         // JSONB
    mitre_techniques?: string[];
    mitre_tactics?: string[];
    // ... timestamps, notes, assigned_to
}
```

**Why this matters academically:**

- Any backend schema change (e.g., adding a new severity level) that is not reflected in the frontend [AlertSeverity](file:///d:/EDR_Platform/dashboard/src/api/client.ts#64-65) union type will be caught at **compile time**, not runtime.
- The `Record<string, unknown>` type for deeply nested JSONB fields (`event_data`, `matched_fields`) provides type safety at the outer boundary while acknowledging the inherently dynamic inner structure.
- The [Agent](file:///d:/EDR_Platform/dashboard/src/api/client.ts#129-152) interface carries 20+ fields including optional telemetry metrics (`cpu_usage`, `memory_used_mb`, `queue_depth`), demonstrating the depth of endpoint observability surfaced to the SOC analyst.

### 1.3 Dual API Client Architecture

The dashboard communicates with **two distinct backend microservices**, each with its own Axios instance:

```typescript
export const sigmaApi      = createApiClient(config.sigmaEngineUrl);      // :8080
export const connectionApi = createApiClient(config.connectionManagerUrl); // :8082
```

| API Client | Backend Service | Responsibilities |
|---|---|---|
| `sigmaApi` | Sigma Engine | Alerts, Rules, Stats, Timeline |
| `connectionApi` | Connection Manager | Agents, Commands, Auth, Audit Logs, Enrollment Tokens |

Each client is independently configured via `VITE_*` environment variables with intelligent fallback logic for Docker vs. local development:

```typescript
const envOrDefault = (envVal: string | undefined, fallback: string): string => {
    // undefined = env var not set at all (local dev) → use fallback
    // ""        = explicitly set to empty (Docker)   → use "" (same-origin, nginx proxied)
    return envVal === undefined ? fallback : envVal;
};
```

---

## 2. Secure State Management & Authentication (JWT Lifecycle)

### 2.1 JWT Token Storage & Login Flow

The authentication lifecycle is managed through the `authApi` service object in [client.ts](file:///d:/EDR_Platform/dashboard/src/api/client.ts#L542-L580):

```typescript
export const authApi = {
    login: async (username: string, password: string) => {
        const response = await connectionApi.post<{
            access_token: string;
            refresh_token?: string;
            expires_in: number;
            token_type: string;
            user: User;
        }>('/api/v1/auth/login', { username, password });

        // Persist token and user profile to localStorage
        if (response.data.access_token) {
            localStorage.setItem('auth_token', response.data.access_token);
            localStorage.setItem('user', JSON.stringify(response.data.user));
        }
        return response.data;
    },
    // ...
};
```

**Flow:**
1. User submits credentials on the [Login.tsx](file:///d:/EDR_Platform/dashboard/src/pages/Login.tsx) page.
2. The `authApi.login()` function POSTs to `/api/v1/auth/login` on the Connection Manager.
3. On success, the JWT `access_token` and the [User](file:///d:/EDR_Platform/dashboard/src/api/client.ts#215-224) profile object (containing the `role` field) are persisted to `localStorage`.
4. The user is navigated to Dashboard (`/`).

### 2.2 Axios HTTP Interceptors — Automatic Bearer Injection

Every Axios instance created by [createApiClient()](file:///d:/EDR_Platform/dashboard/src/api/client.ts#20-52) is equipped with **two interceptors**: a request interceptor and a response interceptor.

**Request Interceptor — JWT Injection:**

```typescript
client.interceptors.request.use((cfg) => {
    const token = localStorage.getItem('auth_token');
    if (token) {
        cfg.headers.Authorization = `Bearer ${token}`;
    }
    return cfg;
});
```

This ensures that every single HTTP request — whether it's fetching alerts, executing a C2 command, or querying audit logs — automatically carries the `Authorization: Bearer <JWT>` header without any manual intervention from the consuming component.

**Response Interceptor — 401 Auto-Logout:**

```typescript
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
```

When any API response returns HTTP `401 Unauthorized` (i.e., the JWT has expired or been revoked), the interceptor:
1. Clears the stored token and user profile.
2. Forces a full-page redirect to `/login`.

This provides a **seamless, centralized token expiration handling** mechanism — no individual component needs to handle 401 errors.

### 2.3 UI-Level Role-Based Access Control (RBAC)

The [User](file:///d:/EDR_Platform/dashboard/src/api/client.ts#215-224) interface defines five distinct roles:

```typescript
export interface User {
    // ...
    role: 'admin' | 'security' | 'analyst' | 'operations' | 'viewer';
}
```

RBAC is enforced at **three levels**:

**Level 1 — Route Protection ([ProtectedRoute](file:///d:/EDR_Platform/dashboard/src/App.tsx#48-68) component):**

```tsx
function ProtectedRoute({ children, roles }: { children: React.ReactNode; roles?: string[] }) {
    if (!isAuthenticated()) {
        return <Navigate to="/login" replace />;
    }
    if (roles && roles.length > 0 && !authApi.hasRole(roles)) {
        return (
            <div className="card text-center py-12">
                <Shield className="w-12 h-12 text-gray-400 mx-auto mb-4" />
                <h3>Access Denied</h3>
            </div>
        );
    }
    return <>{children}</>;
}
```

Applied at the route level, e.g., the Audit Logs page is restricted to `admin` and `security` roles:
```tsx
<Route path="/audit" element={
    <ProtectedRoute roles={['admin', 'security']}>
        <AuditLogs />
    </ProtectedRoute>
} />
```

**Level 2 — Navigation Visibility:**

```tsx
const navItems = [
    // ... standard items visible to all ...
    ...(authApi.canViewAuditLogs()
        ? [{ path: '/audit', icon: Activity, label: 'Audit Logs' }]
        : []),
];
```

The "Audit Logs" nav link is **conditionally rendered** — it only appears in the sidebar if the user has the `admin` or `security` role. A `viewer` role user will never even see the option.

**Level 3 — Feature-Level Guards (Helper Functions):**

```typescript
export const authApi = {
    canExecuteCommands: () => authApi.hasRole(['admin', 'security', 'analyst']),
    canViewAuditLogs:   () => authApi.hasRole(['admin', 'security']),
    canManageUsers:     () => authApi.hasRole(['admin']),
};
```

These guards can be used anywhere in the UI to conditionally render sensitive elements like C2 action buttons.

---

## 3. The C2 Execution Pipeline (Dashboard to Agent Flow)

This section traces the **complete asynchronous lifecycle** of a Command & Control action from the moment the SOC analyst clicks a button to when the command is queued for the agent.

### 3.1 Step-by-Step Trace: "Isolate Network"

```
  ┌──────────────────────────────────────────────────────────────┐
  │ SOC Analyst clicks "Actions" dropdown on Endpoints page     │
  │          ↓                                                   │
  │ QuickActionsDropdown renders (React Portal to document.body)│
  │          ↓                                                   │
  │ Analyst selects "Isolate Network"                           │
  │          ↓                                                   │
  │ handleCommand(agent, 'isolate_network') is called           │
  │ → setSelectedAgent(agent)                                    │
  │ → setSelectedCommand('isolate_network')                      │
  │          ↓                                                   │
  │ CommandExecutionModal opens with parameter fields            │
  │ (allow_list input for IPs/domains to whitelist)             │
  │          ↓                                                   │
  │ Analyst clicks "Execute"                                     │
  │          ↓                                                   │
  │ handleExecute() → setStatus('executing')                     │
  │ → executeMutation.mutate({                                   │
  │       agentId: agent.id,                                     │
  │       command: {                                              │
  │           command_type: 'isolate_network',                   │
  │           parameters: { allow_list: '10.0.0.1, ...' },      │
  │           timeout: 300                                       │
  │       }                                                      │
  │   })                                                         │
  │          ↓                                                   │
  │ POST /api/v1/agents/{agentId}/commands                       │
  │ (Bearer JWT attached automatically by Axios interceptor)    │
  │          ↓                                                   │
  │ Connection Manager queues command for agent delivery         │
  │          ↓                                                   │
  │ onSuccess: setStatus('completed')                            │
  │ → showToast('Command queued successfully (ID: ...)', 'success')│
  │ → queryClient.invalidateQueries(['agents'])                  │
  └──────────────────────────────────────────────────────────────┘
```

### 3.2 The React Mutation Pattern

The entire C2 pipeline leverages TanStack React Query's `useMutation` hook:

```typescript
const executeMutation = useMutation({
    mutationFn: async ({ agentId, command }: { agentId: string; command: CommandRequest }) => {
        return agentsApi.executeCommand(agentId, command);
    },
    onSuccess: (data) => {
        setStatus('completed');
        showToast(`Command queued successfully (ID: ${data.command_id})`, 'success');
        queryClient.invalidateQueries({ queryKey: ['agents'] });
    },
    onError: (error: Error) => {
        setStatus('failed');
        showToast(`Command failed: ${error.message}`, 'error');
    },
});
```

### 3.3 UI State Machine for Command Execution

The [CommandExecutionModal](file:///d:/EDR_Platform/dashboard/src/pages/Endpoints.tsx#202-485) manages a 4-state finite state machine for visual feedback:

| State | Visual | User Action |
|---|---|---|
| `idle` | Parameter form + "Execute" button visible | Fill parameters, click Execute |
| `executing` | Blue spinner + "Executing command..." + button disabled | Wait |
| `completed` | Green checkmark + "Command queued successfully" | Click Close |
| `failed` | Red X icon + "Command failed" | Click Close or retry |

### 3.4 Portal-Based Dropdown for Overflow Safety

The [QuickActionsDropdown](file:///d:/EDR_Platform/dashboard/src/pages/Endpoints.tsx#83-201) uses `createPortal(menu, document.body)` to render the command menu directly onto `document.body`. This is a **deliberate architectural choice** to escape the `overflow: hidden` clipping of the parent table container, ensuring the dropdown is always visible regardless of the table's scroll state.

---

## 4. Observability & Metrics Visualization

### 4.1 Data Fetching Strategy — React Query as Cache Layer

The dashboard uses a layered polling strategy with `refetchInterval` to achieve a near-real-time feel:

| Data Source | Query Key | Refetch Interval | Justification |
|---|---|---|---|
| Alert Stats | `['alertStats']` | 30 seconds | KPI cards; acceptable staleness |
| Recent Alerts | `['recentAlerts']` | One-shot + WebSocket | Fed by live stream |
| Agent List | `['agents', filters]` | 30 seconds | Endpoint table |
| Alert Timeline | `['alertTimeline', '24h']` | 60 seconds | Chart data; longer window |
| Performance Stats | `['performanceStats']` | **10 seconds** | EPS, latency — critical metrics |
| Alerts Table | `['alerts', filters, ...]` | 15 seconds | Active investigation view |

This tiered approach is designed to **minimize API pressure** while keeping the most operationally critical metrics (EPS, latency) fresh.

**Global QueryClient configuration:**

```typescript
const queryClient = new QueryClient({
    defaultOptions: {
        queries: {
            refetchOnWindowFocus: false,  // Prevent thundering herd on tab switch
            retry: 1,                     // Single retry to avoid DoS on backend errors
            staleTime: 30000,             // 30s cache before refetch
        },
    },
});
```

### 4.2 WebSocket Real-Time Alert Stream

For the **Live Alerts Feed** on the Dashboard page, the system uses a WebSocket connection to the Sigma Engine's `/api/v1/sigma/alerts/stream` endpoint:

```typescript
export function createAlertStream(
    onMessage: (alert: Alert) => void,
    filters?: { severity?: string[]; agent_id?: string; rule_id?: string }
) {
    // ... WebSocket connection with:
    // - Exponential backoff reconnection (2^n * 1000ms, max 5 attempts)
    // - Heartbeat ping every 30 seconds
    // - Filter subscription on connect
    // - JSON message parsing with type discrimination
}
```

In the Dashboard component, the stream is consumed via `useEffect`:

```tsx
useEffect(() => {
    const stream = createAlertStream((alert) => {
        setLiveAlerts((prev) => [alert, ...prev.slice(0, 49)]);
        //                              ^^^^^^^^^^^^^^^^^^^^^^^^
        //                              Sliding window: max 50 alerts in memory
    }, { severity: ['critical', 'high', 'medium'] });

    return () => stream.close();  // Cleanup on unmount
}, []);
```

**Memory management:** The `prev.slice(0, 49)` pattern implements a **fixed-size sliding window** of 50 alerts, preventing unbounded memory growth from continuous real-time ingestion. This is essential for long-running SOC sessions.

### 4.3 Recharts Visualization Stack

The dashboard uses [Recharts](https://recharts.org/) for all data visualizations:

| Component | Chart Type | Data Source |
|---|---|---|
| [SeverityDonutChart](file:///d:/EDR_Platform/dashboard/src/pages/Dashboard.tsx#156-236) | `PieChart` (donut) | `alertStats.by_severity` |
| [AlertTimelineChart](file:///d:/EDR_Platform/dashboard/src/pages/Dashboard.tsx#369-440) | `AreaChart` (stacked) | `statsApi.timeline()` — 24h, 1h granularity |
| [TopDetectionRules](file:///d:/EDR_Platform/dashboard/src/pages/Dashboard.tsx#441-497) | `BarChart` (horizontal) | Aggregated from live alert feed |
| [AlertStatusOverview](file:///d:/EDR_Platform/dashboard/src/pages/Dashboard.tsx#319-368) | Custom progress bar | `alertStats.by_status` |
| [EndpointsStatusCard](file:///d:/EDR_Platform/dashboard/src/pages/Dashboard.tsx#237-318) | Custom status cards | `agentStats` + client-side `last_seen` recomputation |

### 4.4 Client-Side Staleness Recomputation

A sophisticated pattern is employed for endpoint status accuracy. Even though the backend has its own sweeper, the **frontend applies an additional safety net**:

```typescript
const STALE_THRESHOLD_MS = 5 * 60 * 1000;  // 5 minutes

function getEffectiveStatus(agent: Agent): Agent['status'] {
    if (agent.status === 'online' || agent.status === 'degraded') {
        const elapsed = Date.now() - new Date(agent.last_seen).getTime();
        if (elapsed > STALE_THRESHOLD_MS) {
            return 'offline';  // Override: stale agent is offline
        }
    }
    return agent.status;
}
```

This ensures that even if the backend sweeper hasn't run yet, the Dashboard will never show a stale agent as "online." The same logic is applied in both the Dashboard's [EndpointsStatusCard](file:///d:/EDR_Platform/dashboard/src/pages/Dashboard.tsx#237-318) and the Endpoints page table.

### 4.5 Performance Metrics Panel (Stats Page)

The [Stats.tsx](file:///d:/EDR_Platform/dashboard/src/pages/Stats.tsx) page fetches real-time performance metrics every **10 seconds**:

```typescript
const { data: perfStats } = useQuery({
    queryKey: ['performanceStats'],
    queryFn: statsApi.performance,
    refetchInterval: 10000,
});
```

Displaying:
- **Avg Event Latency** (ms) — time from event ingestion to processing
- **Rule Match Time** (ms) — Sigma rule evaluation overhead
- **DB Query Time** (ms) — PostgreSQL write latency
- **Error Rate** (%) — processing failure percentage
- **Events/Sec (EPS)** — real-time throughput

---

## 5. Investigation & Forensics UI

### 5.1 Alert Detail Modal — Rendering Deeply Nested JSONB

The [AlertDetailModal](file:///d:/EDR_Platform/dashboard/src/pages/Alerts.tsx#60-244) in [Alerts.tsx](file:///d:/EDR_Platform/dashboard/src/pages/Alerts.tsx#L61-L243) uses a **tabbed interface** to organize complex forensic data:

| Tab | Content | Purpose |
|---|---|---|
| **Summary** | Rule title, severity, status, confidence, agent ID, timestamps | Quick triage overview |
| **Event Details** | Pretty-printed raw JSONB (`event_data` / `matched_fields`) | Deep forensic inspection |
| **MITRE ATT&CK** | Tactics and techniques as badges | Threat intelligence mapping |
| **Actions** | Contextual status transition buttons | Incident response workflow |

The **Event Details** tab renders the arbitrarily nested JSONB data using `JSON.stringify` with indentation:

```tsx
{activeTab === 'event' && (
    <div>
        <label className="text-xs text-gray-500 uppercase tracking-wider">
            Raw Event Data
        </label>
        <pre className="mt-2 p-4 bg-gray-100 dark:bg-gray-900 rounded-lg
                        overflow-auto max-h-96 text-xs font-mono">
            {JSON.stringify(alert.event_data || alert.matched_fields || {}, null, 2)}
        </pre>
    </div>
)}
```

The container uses `overflow-auto` with `max-h-96` (384px) to prevent large payloads from breaking the layout, while still allowing the analyst to scroll through complete event trees.

### 5.2 Incident Response Workflow State Machine

The Actions tab implements a **context-aware state machine** that shows only valid transitions:

```
                    ┌───────────────────────────────────────┐
                    │                                       │
                    ▼                                       │
               ┌─────────┐                                 │
               │  OPEN    │                                 │
               └────┬─────┘                                 │
                    │                                       │
          ┌────────┴────────┐                              │
          ▼                 ▼                              │
  ┌──────────────┐  ┌──────────────┐                      │
  │ ACKNOWLEDGED │  │ IN_PROGRESS  │                      │
  └──────┬───────┘  └──────┬───────┘                      │
         │                 │                               │
         └────────┬────────┘                               │
                  │                                        │
          ┌───────┴───────┐                                │
          ▼               ▼                                │
  ┌────────────┐  ┌───────────────┐                       │
  │  RESOLVED  │  │ FALSE_POSITIVE│                       │
  └──────┬─────┘  └───────┬───────┘                       │
         │                │                                │
         └───── Reopen ───┴────────────────────────────────┘
```

The implementation in code:

```tsx
{/* When status is 'open' — show Acknowledge and Start Investigation */}
{alert.status === 'open' && (
    <>
        <button onClick={() => onStatusChange(alert.id, 'acknowledged')}>
            Acknowledge
        </button>
        <button onClick={() => onStatusChange(alert.id, 'in_progress')}>
            Start Investigation
        </button>
    </>
)}

{/* When acknowledged or in_progress — show Resolve and False Positive */}
{(alert.status === 'acknowledged' || alert.status === 'in_progress') && (
    <>
        <button onClick={() => onStatusChange(alert.id, 'resolved')}>
            Resolve
        </button>
        <button onClick={() => onStatusChange(alert.id, 'false_positive')}>
            False Positive
        </button>
    </>
)}

{/* When resolved or false_positive — show Reopen */}
{(alert.status === 'resolved' || alert.status === 'false_positive') && (
    <button onClick={() => onStatusChange(alert.id, 'open')}>
        Reopen
    </button>
)}
```

Each transition triggers a `useMutation` call that PATCHes `/api/v1/sigma/alerts/{id}/status`, followed by automatic query invalidation of both `['alerts']` and `['alertStats']` to keep the entire dashboard consistent.

### 5.3 Bulk Operations

For mass incident response, the Alerts page supports **bulk status updates**:

```tsx
const bulkUpdateMutation = useMutation({
    mutationFn: ({ ids, status }: { ids: string[]; status: string }) =>
        alertsApi.bulkUpdateStatus(ids, status),
    onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: ['alerts'] });
        queryClient.invalidateQueries({ queryKey: ['alertStats'] });
        setSelectedIds(new Set());
        showToast(`${selectedIds.size} alerts updated`, 'success');
    },
});
```

The [BulkActionsToolbar](file:///d:/EDR_Platform/dashboard/src/pages/Alerts.tsx#245-278) appears contextually when one or more alerts are selected via checkboxes, offering one-click Acknowledge, Resolve, or False Positive actions across the entire selection.

### 5.4 MITRE ATT&CK Threat Intelligence View

The [Threats.tsx](file:///d:/EDR_Platform/dashboard/src/pages/Threats.tsx) page provides a **heatmap matrix** of all 14 MITRE ATT&CK tactics:

- Each tactic cell is color-coded by intensity (low → critical) using a computed gradient.
- Clicking a tactic cell filters the related alerts list to show only alerts matching that tactic.
- A **Top Techniques** bar chart aggregates the most frequently detected techniques across all alerts.

### 5.5 Pagination — Server-Side + Client-Side Hybrid

The Alerts page uses a **hybrid pagination** model:
- **Server-side:** `limit` and `offset` parameters are sent to the API.
- **Client-side:** Multi-value filters (when multiple severities/statuses are selected) are applied locally after the fetch.
- A dedicated [Pagination](file:///d:/EDR_Platform/dashboard/src/pages/Alerts.tsx#279-334) component manages page size (`25 | 50 | 100`) and page navigation.

---

## Summary of Technology Stack

| Layer | Technology | Version |
|---|---|---|
| Framework | React | 19.2.0 |
| Language | TypeScript | 5.9.3 |
| Build Tool | Vite | 7.2.4 |
| Server State | TanStack React Query | 5.90.16 |
| HTTP Client | Axios | 1.13.2 |
| Routing | React Router DOM | 7.12.0 |
| Charts | Recharts | 3.6.0 |
| Icons | Lucide React | 0.562.0 |
| Styling | Tailwind CSS | 4.1.18 |
| Date Utility | date-fns | 4.1.0 |
| Containerization | Docker + Nginx | — |
