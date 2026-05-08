package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/edr-platform/connection-manager/pkg/models"
)

// ============================================================================
// REPORTS — single-call report bundle endpoint
// ============================================================================

type reportTemplate string

const (
	reportTemplateExecutive  reportTemplate = "executive"
	reportTemplateTechnical  reportTemplate = "technical"
	reportTemplateCompliance reportTemplate = "compliance"
	reportTemplateOperations reportTemplate = "operations"
	reportTemplateCustom     reportTemplate = "custom"
)

type reportScope string

const (
	reportScopeAll      reportScope = "all_endpoints"
	reportScopeEndpoint reportScope = "specific_endpoint"
)

type generateReportRequest struct {
	Template reportTemplate `json:"template"` // executive|technical|compliance|operations|custom
	Scope    reportScope    `json:"scope"`    // all_endpoints|specific_endpoint
	AgentID  string         `json:"agent_id,omitempty"`

	// Optional time-range: used for Sigma alerts / timeline and audit/vuln filters.
	DateFrom *string `json:"date_from,omitempty"` // RFC3339
	DateTo   *string `json:"date_to,omitempty"`   // RFC3339

	// Optional limits (sane defaults are applied)
	Limits struct {
		Agents    int `json:"agents,omitempty"`    // default 500
		Commands  int `json:"commands,omitempty"`  // default 500
		Vuln      int `json:"vuln,omitempty"`      // default 200
		AuditLogs int `json:"audit_logs,omitempty"` // default 200
		Sigma     int `json:"sigma_alerts,omitempty"` // default 1000
	} `json:"limits,omitempty"`

	// Custom template: allow selecting which sections to include.
	Include struct {
		SigmaAlerts       bool `json:"sigma_alerts,omitempty"`
		SigmaAlertStats   bool `json:"sigma_alert_stats,omitempty"`
		SigmaPerformance  bool `json:"sigma_performance,omitempty"`
		Agents            bool `json:"agents,omitempty"`
		AgentStats        bool `json:"agent_stats,omitempty"`
		Commands          bool `json:"commands,omitempty"`
		CommandStats      bool `json:"command_stats,omitempty"`
		Vulnerability     bool `json:"vulnerability,omitempty"`
		AuditLogs         bool `json:"audit_logs,omitempty"`
		EndpointRisk      bool `json:"endpoint_risk,omitempty"`
	} `json:"include,omitempty"`
}

type reportSectionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// GenerateReportBundle
// POST /api/v1/reports/generate
//
// This endpoint exists to avoid the dashboard issuing 8–10 API calls to assemble a report.
// It returns a single JSON payload containing the requested sections, optionally filtered
// by agent_id and time range.
func (h *Handlers) GenerateReportBundle(c echo.Context) error {
	var req generateReportRequest
	if err := c.Bind(&req); err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
	}

	// Defaults
	if req.Template == "" {
		req.Template = reportTemplateExecutive
	}
	if req.Scope == "" {
		req.Scope = reportScopeAll
	}
	if req.Limits.Agents <= 0 {
		req.Limits.Agents = 500
	}
	if req.Limits.Commands <= 0 {
		req.Limits.Commands = 500
	}
	if req.Limits.Vuln <= 0 {
		req.Limits.Vuln = 200
	}
	if req.Limits.AuditLogs <= 0 {
		req.Limits.AuditLogs = 200
	}
	if req.Limits.Sigma <= 0 {
		req.Limits.Sigma = 1000
	}

	// Validate template
	switch req.Template {
	case reportTemplateExecutive, reportTemplateTechnical, reportTemplateCompliance, reportTemplateOperations, reportTemplateCustom:
	default:
		return errorResponse(c, http.StatusBadRequest, "INVALID_TEMPLATE", "template must be one of: executive, technical, compliance, operations, custom")
	}

	// Validate scope
	switch req.Scope {
	case reportScopeAll:
		// ok
	case reportScopeEndpoint:
		if strings.TrimSpace(req.AgentID) == "" {
			return errorResponse(c, http.StatusBadRequest, "INVALID_SCOPE", "agent_id is required when scope=specific_endpoint")
		}
	default:
		return errorResponse(c, http.StatusBadRequest, "INVALID_SCOPE", "scope must be one of: all_endpoints, specific_endpoint")
	}

	var agentUUID *uuid.UUID
	if strings.TrimSpace(req.AgentID) != "" {
		id, err := uuid.Parse(req.AgentID)
		if err != nil {
			return errorResponse(c, http.StatusBadRequest, "INVALID_AGENT_ID", "agent_id must be a UUID")
		}
		agentUUID = &id
	}

	var fromTime, toTime *time.Time
	if req.DateFrom != nil && strings.TrimSpace(*req.DateFrom) != "" {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.DateFrom)); err == nil {
			fromTime = &t
		} else {
			return errorResponse(c, http.StatusBadRequest, "INVALID_DATE_FROM", "date_from must be RFC3339")
		}
	}
	if req.DateTo != nil && strings.TrimSpace(*req.DateTo) != "" {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.DateTo)); err == nil {
			toTime = &t
		} else {
			return errorResponse(c, http.StatusBadRequest, "INVALID_DATE_TO", "date_to must be RFC3339")
		}
	}

	// Apply template include defaults.
	// If caller provided explicit include flags, merge them on top so the
	// frontend can request extra sections (e.g. sigma_alerts for timeline in
	// executive preview) without forcing template=custom.
	requestedInclude := req.Include
	if req.Template != reportTemplateCustom {
		req.Include = templateDefaultIncludes(req.Template)
	}
	req.Include = mergeIncludeFlags(req.Include, requestedInclude)

	ctx := c.Request().Context()

	type section struct {
		Data  any               `json:"data,omitempty"`
		Error *reportSectionError `json:"error,omitempty"`
	}

	out := map[string]section{}

	// ─────────────────────────────────────────────────────────────────────
	// Local (connection-manager) sections
	// ─────────────────────────────────────────────────────────────────────

	if req.Include.Agents {
		if h.agentSvc == nil {
			out["agents"] = section{Error: &reportSectionError{Code: "DB_UNAVAILABLE", Message: "Agent service is not available"}}
		} else {
			filter := repository.AgentFilter{Limit: req.Limits.Agents, Offset: 0}
			agents, err := h.agentSvc.ListAgents(ctx, filter)
			if err != nil {
				out["agents"] = section{Error: &reportSectionError{Code: "FETCH_FAILED", Message: "Failed to list agents"}}
			} else {
				out["agents"] = section{Data: agents}
			}
		}
	}

	if req.Include.AgentStats {
		if h.agentSvc == nil {
			out["agent_stats"] = section{Error: &reportSectionError{Code: "DB_UNAVAILABLE", Message: "Agent service is not available"}}
		} else {
			stats, err := computeAgentStats(ctx, h.agentSvc)
			if err != nil {
				out["agent_stats"] = section{Error: &reportSectionError{Code: "FETCH_FAILED", Message: err.Error()}}
			} else {
				out["agent_stats"] = section{Data: stats}
			}
		}
	}

	if req.Include.Commands {
		if h.commandRepo == nil {
			out["commands"] = section{Error: &reportSectionError{Code: "DB_UNAVAILABLE", Message: "Command repository is not available"}}
		} else {
			f := repository.CommandListFilter{
				Limit:     req.Limits.Commands,
				Offset:    0,
				SortBy:    "issued_at",
				SortOrder: "desc",
			}
			if agentUUID != nil {
				f.AgentID = agentUUID
			}
			items, total, err := h.commandRepo.ListAll(ctx, f)
			if err != nil {
				out["commands"] = section{Error: &reportSectionError{Code: "FETCH_FAILED", Message: "Failed to list commands"}}
			} else {
				out["commands"] = section{Data: map[string]any{"data": items, "total": total, "limit": f.Limit, "offset": f.Offset}}
			}
		}
	}

	if req.Include.CommandStats {
		if h.commandRepo == nil {
			out["command_stats"] = section{Error: &reportSectionError{Code: "DB_UNAVAILABLE", Message: "Command repository is not available"}}
		} else {
			stats, err := h.commandRepo.GetStats(ctx)
			if err != nil {
				out["command_stats"] = section{Error: &reportSectionError{Code: "FETCH_FAILED", Message: "Failed to get command stats"}}
			} else {
				out["command_stats"] = section{Data: stats}
			}
		}
	}

	if req.Include.Vulnerability {
		if h.vulnRepo == nil {
			out["vuln_findings"] = section{Error: &reportSectionError{Code: "DB_UNAVAILABLE", Message: "Vulnerability repository is not available"}}
		} else {
			filter := repository.VulnerabilityFindingFilter{Limit: req.Limits.Vuln, Offset: 0}
			if agentUUID != nil {
				filter.AgentID = agentUUID
			}
			items, total, err := h.vulnRepo.ListWithHost(ctx, filter)
			if err != nil {
				out["vuln_findings"] = section{Error: &reportSectionError{Code: "FETCH_FAILED", Message: "Failed to list vulnerability findings"}}
			} else {
				out["vuln_findings"] = section{Data: map[string]any{"data": items, "total": total, "limit": filter.Limit, "offset": filter.Offset}}
			}
		}
	}

	if req.Include.AuditLogs {
		if h.auditRepo == nil {
			out["audit_logs"] = section{Error: &reportSectionError{Code: "DB_UNAVAILABLE", Message: "Audit repository is not available"}}
		} else {
			filter := repository.AuditLogFilter{Limit: req.Limits.AuditLogs, Offset: 0}
			if fromTime != nil {
				filter.StartTime = fromTime
			}
			if toTime != nil {
				filter.EndTime = toTime
			}
			logs, err := h.auditRepo.List(ctx, filter)
			if err != nil {
				out["audit_logs"] = section{Error: &reportSectionError{Code: "FETCH_FAILED", Message: "Failed to list audit logs"}}
			} else {
				out["audit_logs"] = section{Data: map[string]any{"data": logs, "limit": filter.Limit, "offset": filter.Offset}}
			}
		}
	}

	if req.Include.EndpointRisk {
		if h.alertRepo == nil {
			out["endpoint_risk"] = section{Error: &reportSectionError{Code: "DB_UNAVAILABLE", Message: "Alert repository is not available"}}
		} else {
			summaries, err := h.alertRepo.GetEndpointRiskSummary(ctx)
			if err != nil {
				out["endpoint_risk"] = section{Error: &reportSectionError{Code: "FETCH_FAILED", Message: "Failed to compute endpoint risk summary"}}
			} else {
				// Optional filter to a single endpoint for specific reports.
				if agentUUID != nil {
					filtered := make([]any, 0, 1)
					for _, s := range summaries {
						if s != nil && s.AgentID == agentUUID.String() {
							filtered = append(filtered, s)
							break
						}
					}
					out["endpoint_risk"] = section{Data: map[string]any{"data": filtered, "total": len(filtered)}}
				} else {
					out["endpoint_risk"] = section{Data: map[string]any{"data": summaries, "total": len(summaries)}}
				}
			}
		}
	}

	// ─────────────────────────────────────────────────────────────────────
	// Sigma (via reverse-proxy) sections — optional
	// NOTE: connection-manager does not directly host /api/v1/sigma/* routes.
	// We call back through the same public host so the reverse proxy can route
	// those requests to the Sigma Engine, while reusing the caller's JWT.
	// ─────────────────────────────────────────────────────────────────────

	baseURL := publicBaseURL(c)
	authHeader := c.Request().Header.Get("Authorization")

	if req.Include.SigmaAlerts {
		path := "/api/v1/sigma/alerts"
		q := url.Values{}
		q.Set("limit", fmt.Sprintf("%d", req.Limits.Sigma))
		q.Set("sort", "timestamp")
		q.Set("order", "desc")
		if agentUUID != nil {
			q.Set("agent_id", agentUUID.String())
		}
		if fromTime != nil {
			q.Set("date_from", fromTime.UTC().Format(time.RFC3339Nano))
		}
		if toTime != nil {
			q.Set("date_to", toTime.UTC().Format(time.RFC3339Nano))
		}
		var payload any
		if err := fetchJSON(ctx, baseURL, path, q, authHeader, &payload); err != nil {
			out["sigma_alerts"] = section{Error: &reportSectionError{Code: "SIGMA_FETCH_FAILED", Message: err.Error()}}
		} else {
			out["sigma_alerts"] = section{Data: payload}
		}
	}

	if req.Include.SigmaAlertStats {
		var payload any
		if err := fetchJSON(ctx, baseURL, "/api/v1/sigma/stats/alerts", nil, authHeader, &payload); err != nil {
			out["sigma_stats_alerts"] = section{Error: &reportSectionError{Code: "SIGMA_FETCH_FAILED", Message: err.Error()}}
		} else {
			out["sigma_stats_alerts"] = section{Data: payload}
		}
	}

	if req.Include.SigmaPerformance {
		var payload any
		if err := fetchJSON(ctx, baseURL, "/api/v1/sigma/stats/performance", nil, authHeader, &payload); err != nil {
			out["sigma_stats_performance"] = section{Error: &reportSectionError{Code: "SIGMA_FETCH_FAILED", Message: err.Error()}}
		} else {
			out["sigma_stats_performance"] = section{Data: payload}
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"input": req,
		"data":  out,
		"meta":  responseMeta(c),
	})
}

func templateDefaultIncludes(t reportTemplate) (out struct {
	SigmaAlerts       bool `json:"sigma_alerts,omitempty"`
	SigmaAlertStats   bool `json:"sigma_alert_stats,omitempty"`
	SigmaPerformance  bool `json:"sigma_performance,omitempty"`
	Agents            bool `json:"agents,omitempty"`
	AgentStats        bool `json:"agent_stats,omitempty"`
	Commands          bool `json:"commands,omitempty"`
	CommandStats      bool `json:"command_stats,omitempty"`
	Vulnerability     bool `json:"vulnerability,omitempty"`
	AuditLogs         bool `json:"audit_logs,omitempty"`
	EndpointRisk      bool `json:"endpoint_risk,omitempty"`
}) {
	switch t {
	case reportTemplateExecutive:
		out.SigmaAlertStats = true
		out.SigmaPerformance = true
		out.AgentStats = true
		out.EndpointRisk = true
	case reportTemplateTechnical:
		out.SigmaAlerts = true
		out.SigmaAlertStats = true
		out.SigmaPerformance = true
		out.Agents = true
		out.AgentStats = true
		out.Commands = true
		out.CommandStats = true
		out.Vulnerability = true
		out.AuditLogs = true
		out.EndpointRisk = true
	case reportTemplateCompliance:
		out.Agents = true
		out.Vulnerability = true
		out.AuditLogs = true
		out.EndpointRisk = true
	case reportTemplateOperations:
		out.AgentStats = true
		out.Commands = true
		out.CommandStats = true
		out.AuditLogs = true
		out.SigmaAlertStats = true
	default: // custom
		// caller decides
	}
	return out
}

func mergeIncludeFlags(base, extra struct {
	SigmaAlerts       bool `json:"sigma_alerts,omitempty"`
	SigmaAlertStats   bool `json:"sigma_alert_stats,omitempty"`
	SigmaPerformance  bool `json:"sigma_performance,omitempty"`
	Agents            bool `json:"agents,omitempty"`
	AgentStats        bool `json:"agent_stats,omitempty"`
	Commands          bool `json:"commands,omitempty"`
	CommandStats      bool `json:"command_stats,omitempty"`
	Vulnerability     bool `json:"vulnerability,omitempty"`
	AuditLogs         bool `json:"audit_logs,omitempty"`
	EndpointRisk      bool `json:"endpoint_risk,omitempty"`
}) (out struct {
	SigmaAlerts       bool `json:"sigma_alerts,omitempty"`
	SigmaAlertStats   bool `json:"sigma_alert_stats,omitempty"`
	SigmaPerformance  bool `json:"sigma_performance,omitempty"`
	Agents            bool `json:"agents,omitempty"`
	AgentStats        bool `json:"agent_stats,omitempty"`
	Commands          bool `json:"commands,omitempty"`
	CommandStats      bool `json:"command_stats,omitempty"`
	Vulnerability     bool `json:"vulnerability,omitempty"`
	AuditLogs         bool `json:"audit_logs,omitempty"`
	EndpointRisk      bool `json:"endpoint_risk,omitempty"`
}) {
	out.SigmaAlerts = base.SigmaAlerts || extra.SigmaAlerts
	out.SigmaAlertStats = base.SigmaAlertStats || extra.SigmaAlertStats
	out.SigmaPerformance = base.SigmaPerformance || extra.SigmaPerformance
	out.Agents = base.Agents || extra.Agents
	out.AgentStats = base.AgentStats || extra.AgentStats
	out.Commands = base.Commands || extra.Commands
	out.CommandStats = base.CommandStats || extra.CommandStats
	out.Vulnerability = base.Vulnerability || extra.Vulnerability
	out.AuditLogs = base.AuditLogs || extra.AuditLogs
	out.EndpointRisk = base.EndpointRisk || extra.EndpointRisk
	return out
}

func publicBaseURL(c echo.Context) string {
	// Prefer proxy-aware scheme.
	scheme := c.Scheme()
	if v := c.Request().Header.Get("X-Forwarded-Proto"); v != "" {
		scheme = strings.Split(v, ",")[0]
	}
	host := c.Request().Host
	if host == "" {
		host = "localhost"
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

func fetchJSON(ctx context.Context, baseURL, path string, q url.Values, authHeader string, into any) error {
	u := baseURL + path
	if q != nil && len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	if strings.TrimSpace(authHeader) != "" {
		req.Header.Set("Authorization", authHeader)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 5<<20)) // 5MB cap to avoid runaway
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// try to surface an error message from JSON
		msg := strings.TrimSpace(string(body))
		if len(msg) > 500 {
			msg = msg[:500] + "…"
		}
		return fmt.Errorf("upstream %s returned %d: %s", path, resp.StatusCode, msg)
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("invalid JSON from upstream %s: %w", path, err)
	}
	return nil
}

// computeAgentStats mirrors GET /api/v1/agents/stats without needing an echo.Context.
func computeAgentStats(ctx context.Context, agentSvc interface {
	CountAgents(context.Context, repository.AgentFilter) (int64, error)
	GetOnlineAgents(context.Context) ([]*models.Agent, error)
	ListAgents(context.Context, repository.AgentFilter) ([]*models.Agent, error)
}) (any, error) {
	// NOTE: We intentionally return a map with the same keys the dashboard expects
	// (online/offline/avg_health/by_os_type/by_version) so the report consumer can
	// reuse existing rendering logic.

	statusKeys := []string{"online", "offline", "degraded", "pending", "suspended"}
	counts := make(map[string]int)
	var totalCount int64
	for _, s := range statusKeys {
		status := s
		cnt, err := agentSvc.CountAgents(ctx, repository.AgentFilter{Status: &status})
		if err != nil {
			continue
		}
		counts[s] = int(cnt)
		totalCount += cnt
	}

	avgHealth := 0.0
	onlineAgents, err := agentSvc.GetOnlineAgents(ctx)
	if err == nil && len(onlineAgents) > 0 {
		sum := 0.0
		for _, a := range onlineAgents {
			sum += a.HealthScore
		}
		avgHealth = sum / float64(len(onlineAgents))
	}

	byOS := make(map[string]int)
	byVersion := make(map[string]int)
	allAgents, err := agentSvc.ListAgents(ctx, repository.AgentFilter{Limit: 10000})
	if err == nil {
		for _, a := range allAgents {
			byOS[a.OSType]++
			byVersion[a.AgentVersion]++
		}
	}

	return map[string]any{
		"total":      int(totalCount),
		"online":     counts["online"],
		"offline":    counts["offline"],
		"degraded":   counts["degraded"],
		"pending":    counts["pending"],
		"suspended":  counts["suspended"],
		"by_os_type": byOS,
		"by_version": byVersion,
		"avg_health": avgHealth,
	}, nil
}

