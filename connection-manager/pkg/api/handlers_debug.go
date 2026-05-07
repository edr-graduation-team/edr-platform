// Package api — handlers_debug.go
//
// TEMPORARY DEBUG ENDPOINT — internal development only.
// Exposes a step-by-step trace of how dashboard statistics are computed
// (raw SQL queries executed, intermediate values, formulas applied,
// and final values returned to the UI). Intended to be deleted once
// the developer finishes investigating server-side stat computation.
//
// Route: GET /api/v1/debug/stats-trace?limit=N
// RBAC : admin role only (the route registration enforces this).
package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/edr-platform/connection-manager/pkg/models"
)

// debugTraceResponse is the top-level payload returned by the trace endpoint.
type debugTraceResponse struct {
	GeneratedAt time.Time          `json:"generated_at"`
	Sections    []debugTraceSect   `json:"sections"`
	Notes       []string           `json:"notes"`
}

// debugTraceSect represents one statistic / metric being traced.
type debugTraceSect struct {
	Key         string                 `json:"key"`           // machine-readable identifier
	Title       string                 `json:"title"`         // human-readable title
	UIRoute     string                 `json:"ui_route"`      // where this number is shown in the dashboard
	HTTPRoute   string                 `json:"http_route"`    // the actual API endpoint that returns this value
	Formula     string                 `json:"formula"`       // formula / explanation
	Steps       []debugTraceStep       `json:"steps"`         // ordered computation steps
	FinalValue  interface{}            `json:"final_value"`   // what the dashboard ends up showing
	DurationMs  int64                  `json:"duration_ms"`
	Error       string                 `json:"error,omitempty"`
}

// debugTraceStep is a single computation step.
type debugTraceStep struct {
	Label  string      `json:"label"`             // step description
	SQL    string      `json:"sql,omitempty"`     // SQL executed (if any)
	Inputs interface{} `json:"inputs,omitempty"`  // input values for this step
	Output interface{} `json:"output,omitempty"`  // output / result of this step
	Note   string      `json:"note,omitempty"`    // optional explanatory note
}

// DebugStatsTrace is the unified handler that returns the full stats trace.
func (h *Handlers) DebugStatsTrace(c echo.Context) error {
	ctx := c.Request().Context()
	limit := 5
	if v, err := strconv.Atoi(c.QueryParam("limit")); err == nil && v > 0 && v <= 50 {
		limit = v
	}

	resp := debugTraceResponse{
		GeneratedAt: time.Now().UTC(),
		Notes: []string{
			"This endpoint is for internal development only — it should be removed before production.",
			"All SQL queries shown here are the EXACT queries the production code paths execute.",
			"Per-agent health is computed in Go (not SQL) using the 4-factor NIST SP 800-137 model.",
		},
	}

	resp.Sections = append(resp.Sections, h.traceAgentStats(ctx))
	resp.Sections = append(resp.Sections, h.traceAgentHealth(ctx, limit))
	resp.Sections = append(resp.Sections, h.traceAlertStats(ctx))
	resp.Sections = append(resp.Sections, h.traceVulnStats(ctx))

	return c.JSON(http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// 1) Agent counts + average health (the 5 cards on Endpoints page header)
// ---------------------------------------------------------------------------
func (h *Handlers) traceAgentStats(ctx context.Context) debugTraceSect {
	start := time.Now()
	sect := debugTraceSect{
		Key:       "agent_stats",
		Title:     "Endpoint Inventory & Average Health",
		UIRoute:   "/management/devices  (header KPI cards)",
		HTTPRoute: "GET /api/v1/agents/stats",
		Formula: "for each status in [online, offline, degraded, pending, suspended]:\n" +
			"    counts[status] = SELECT COUNT(*) FROM agents WHERE status = $1\n" +
			"total = sum(counts)\n" +
			"avg_health = avg(health_score) over rows where status='online' (computed in Go)",
	}
	if h.agentSvc == nil {
		sect.Error = "agent service unavailable"
		sect.DurationMs = time.Since(start).Milliseconds()
		return sect
	}

	statuses := []string{"online", "offline", "degraded", "pending", "suspended"}
	counts := map[string]int64{}
	var total int64
	for _, s := range statuses {
		st := s
		n, err := h.agentSvc.CountAgents(ctx, repository.AgentFilter{Status: &st})
		stp := debugTraceStep{
			Label:  "count agents WHERE status = '" + s + "'",
			SQL:    "SELECT COUNT(*) FROM agents WHERE status = '" + s + "'",
			Output: n,
		}
		if err != nil {
			stp.Note = "ERROR: " + err.Error()
		}
		sect.Steps = append(sect.Steps, stp)
		counts[s] = n
		total += n
	}
	sect.Steps = append(sect.Steps, debugTraceStep{
		Label:  "total = sum of all status counts",
		Output: total,
	})

	online, err := h.agentSvc.GetOnlineAgents(ctx)
	if err != nil {
		sect.Steps = append(sect.Steps, debugTraceStep{
			Label: "fetch online agents for avg health",
			Note:  "ERROR: " + err.Error(),
		})
	} else {
		var sum float64
		for _, a := range online {
			sum += a.HealthScore
		}
		var avg float64
		if len(online) > 0 {
			avg = sum / float64(len(online))
		}
		sect.Steps = append(sect.Steps, debugTraceStep{
			Label: "fetch online agents (for average health)",
			SQL:   "SELECT * FROM agents WHERE status = 'online'",
			Inputs: map[string]interface{}{
				"online_count": len(online),
			},
			Output: map[string]interface{}{
				"sum_health_scores": sum,
				"avg_health":        avg,
			},
			Note: "avg_health is computed in Go: sum(a.health_score) / count(online_agents)",
		})
	}

	sect.FinalValue = map[string]interface{}{
		"total":  total,
		"counts": counts,
	}
	sect.DurationMs = time.Since(start).Milliseconds()
	return sect
}

// ---------------------------------------------------------------------------
// 2) Per-agent Health Score — step by step
// ---------------------------------------------------------------------------
func (h *Handlers) traceAgentHealth(ctx context.Context, limit int) debugTraceSect {
	start := time.Now()
	sect := debugTraceSect{
		Key:       "agent_health",
		Title:     "Per-Agent Health Score (4-factor NIST model)",
		UIRoute:   "/management/devices  (Health column)",
		HTTPRoute: "(stored in agents.health_score, written by gRPC heartbeat handler)",
		Formula: "health_score = delivery×0.40 + status×0.30 + dropRate×0.20 + resource×0.10\n" +
			"where:\n" +
			"  delivery  = (events_delivered / events_collected) × 100, capped at 100\n" +
			"  status    = {online:100, degraded:80, offline:50, suspended:0, other:60}\n" +
			"  dropScore = if dropRate > 20%: 0  (potential blinding attack)\n" +
			"              if dropRate > 5%:  linear 100→0 between 5% and 20%\n" +
			"              else: 100\n" +
			"  resource  = 100 - cpu_penalty - memory_penalty (each ≤50)\n" +
			"  status string for UI: ≥90 excellent, ≥75 good, ≥50 fair, ≥25 degraded, <25 critical",
	}
	if h.agentSvc == nil {
		sect.Error = "agent service unavailable"
		sect.DurationMs = time.Since(start).Milliseconds()
		return sect
	}

	agents, err := h.agentSvc.ListAgents(ctx, repository.AgentFilter{Limit: limit})
	if err != nil {
		sect.Error = err.Error()
		sect.DurationMs = time.Since(start).Milliseconds()
		return sect
	}

	sect.Steps = append(sect.Steps, debugTraceStep{
		Label:  "fetch first N agents",
		SQL:    "SELECT * FROM agents ORDER BY last_seen DESC LIMIT " + strconv.Itoa(limit),
		Output: map[string]interface{}{"rows": len(agents)},
	})

	per := make([]map[string]interface{}, 0, len(agents))
	for _, a := range agents {
		per = append(per, traceOneAgentHealth(a))
	}
	sect.FinalValue = per
	sect.DurationMs = time.Since(start).Milliseconds()
	return sect
}

// traceOneAgentHealth re-implements CalculateHealthScore step-by-step
// returning every intermediate value so the developer can see exactly
// how the final score was arrived at (and why it differs between agents).
func traceOneAgentHealth(a *models.Agent) map[string]interface{} {
	// Factor 1 — Delivery
	deliveryRatio := 100.0
	if a.EventsCollected > 0 {
		deliveryRatio = float64(a.EventsDelivered) / float64(a.EventsCollected) * 100
		if deliveryRatio > 100 {
			deliveryRatio = 100
		}
	}

	// Factor 2 — Status
	statusScore := 60.0
	switch a.Status {
	case models.AgentStatusOnline:
		statusScore = 100
	case models.AgentStatusDegraded:
		statusScore = 80
	case models.AgentStatusOffline:
		statusScore = 50
	case models.AgentStatusSuspended:
		statusScore = 0
	}

	// Factor 3 — Drop Rate
	dropScore := 100.0
	dropRate := 0.0
	if a.EventsCollected > 0 {
		dropRate = float64(a.EventsDropped) / float64(a.EventsCollected)
		switch {
		case dropRate > 0.20:
			dropScore = 0
		case dropRate > 0.05:
			dropScore = (0.20 - dropRate) / 0.15 * 100
		}
	}

	// Factor 4 — Resource pressure
	resourceScore := 100.0
	cpuPenalty := 0.0
	switch {
	case a.CPUUsage > 90:
		cpuPenalty = 50
	case a.CPUUsage > 70:
		cpuPenalty = (a.CPUUsage - 70) / 20.0 * 30.0
	}
	memUsagePercent := 0.0
	memPenalty := 0.0
	if a.MemoryMB > 0 && a.MemoryUsedMB > 0 {
		memUsagePercent = float64(a.MemoryUsedMB) / float64(a.MemoryMB) * 100
		switch {
		case memUsagePercent > 95:
			memPenalty = 50
		case memUsagePercent > 80:
			memPenalty = (memUsagePercent - 80) / 15.0 * 30.0
		}
	}
	resourceScore = 100 - cpuPenalty - memPenalty
	if resourceScore < 0 {
		resourceScore = 0
	}

	final := deliveryRatio*0.40 + statusScore*0.30 + dropScore*0.20 + resourceScore*0.10
	statusLabel := "critical"
	switch {
	case final >= 90:
		statusLabel = "excellent"
	case final >= 75:
		statusLabel = "good"
	case final >= 50:
		statusLabel = "fair"
	case final >= 25:
		statusLabel = "degraded"
	}

	return map[string]interface{}{
		"agent_id":       a.ID,
		"hostname":       a.Hostname,
		"status":         a.Status,
		"raw_inputs": map[string]interface{}{
			"events_collected": a.EventsCollected,
			"events_delivered": a.EventsDelivered,
			"events_dropped":   a.EventsDropped,
			"cpu_usage":        a.CPUUsage,
			"memory_used_mb":   a.MemoryUsedMB,
			"memory_total_mb":  a.MemoryMB,
		},
		"factors": map[string]interface{}{
			"delivery": map[string]interface{}{
				"weight":  0.40,
				"value":   deliveryRatio,
				"formula": "(events_delivered / events_collected) × 100, capped at 100",
			},
			"status": map[string]interface{}{
				"weight":  0.30,
				"value":   statusScore,
				"formula": "lookup table by agent.status",
			},
			"drop_rate": map[string]interface{}{
				"weight":         0.20,
				"value":          dropScore,
				"drop_rate_pct":  dropRate * 100,
				"formula":        "0 if >20%, linear 100→0 between 5–20%, else 100",
			},
			"resource": map[string]interface{}{
				"weight":            0.10,
				"value":             resourceScore,
				"cpu_penalty":       cpuPenalty,
				"mem_usage_pct":     memUsagePercent,
				"mem_penalty":       memPenalty,
				"formula":           "100 - cpuPenalty - memPenalty (clamped ≥0)",
			},
		},
		"final_score":          final,
		"final_status_label":   statusLabel,
		"stored_health_score":  a.HealthScore,
		"matches_stored_value": int(final*10) == int(a.HealthScore*10), // tolerate float jitter
	}
}

// ---------------------------------------------------------------------------
// 3) Alert stats (top nav badge + Alerts page KPIs)
// ---------------------------------------------------------------------------
func (h *Handlers) traceAlertStats(ctx context.Context) debugTraceSect {
	start := time.Now()
	sect := debugTraceSect{
		Key:       "alert_stats",
		Title:     "Alert Stats (top-nav badge & Alerts KPI cards)",
		UIRoute:   "/alerts  (KPI cards)  +  PlatformAppShell top-nav badge",
		HTTPRoute: "GET /api/v1/alerts/stats",
		Formula: "single aggregated query returning total / 24h / avg_risk_score / status counts;\n" +
			"separate query for severity breakdown.",
	}
	if h.alertRepo == nil {
		sect.Error = "alert repository unavailable"
		sect.DurationMs = time.Since(start).Milliseconds()
		return sect
	}

	sect.Steps = append(sect.Steps, debugTraceStep{
		Label: "single aggregate scan over alerts",
		SQL: `SELECT
  COUNT(*),
  COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '24 hours'),
  COALESCE(AVG(risk_score), 0),
  COUNT(*) FILTER (WHERE status = 'open'),
  COUNT(*) FILTER (WHERE status = 'in_progress'),
  COUNT(*) FILTER (WHERE status = 'resolved')
FROM alerts`,
		Note: "FILTER clauses let Postgres compute every counter in one sequential scan.",
	})
	sect.Steps = append(sect.Steps, debugTraceStep{
		Label: "severity breakdown",
		SQL:   "SELECT severity, COUNT(*) FROM alerts GROUP BY severity",
	})

	stats, err := h.alertRepo.GetStats(ctx)
	if err != nil {
		sect.Error = err.Error()
		sect.DurationMs = time.Since(start).Milliseconds()
		return sect
	}
	sect.FinalValue = map[string]interface{}{
		"total":          stats.Total,
		"alerts_24h":     stats.Alerts24h,
		"avg_confidence": stats.AvgConfidence,
		"open":           stats.Open,
		"in_progress":    stats.InProgress,
		"resolved":       stats.Resolved,
		"by_severity":    stats.BySeverity,
		"by_status":      stats.ByStatus,
	}
	sect.DurationMs = time.Since(start).Milliseconds()
	return sect
}

// ---------------------------------------------------------------------------
// 4) Vulnerability stats
// ---------------------------------------------------------------------------
func (h *Handlers) traceVulnStats(ctx context.Context) debugTraceSect {
	start := time.Now()
	sect := debugTraceSect{
		Key:       "vuln_stats",
		Title:     "Vulnerability Stats (KPI cards + top-N lists)",
		UIRoute:   "/soc/vulnerability  (KPI cards & top packages/hosts)",
		HTTPRoute: "GET /api/v1/vuln/stats",
		Formula: "one aggregate scan computes every KPI counter via FILTER clauses;\n" +
			"three follow-up GROUP BYs for severity / status / top-10 packages and hosts.",
	}
	if h.vulnRepo == nil {
		sect.Error = "vulnerability repository unavailable"
		sect.DurationMs = time.Since(start).Milliseconds()
		return sect
	}

	sect.Steps = append(sect.Steps, debugTraceStep{
		Label: "single aggregate scan over vulnerability_findings",
		SQL: `SELECT
  COUNT(*),
  COUNT(*) FILTER (WHERE status = 'open'),
  COUNT(*) FILTER (WHERE kev_listed = true AND status = 'open'),
  COUNT(*) FILTER (WHERE due_at IS NOT NULL AND due_at < NOW() AND status NOT IN ('resolved','risk_accepted')),
  COUNT(*) FILTER (WHERE exploit_available = true AND status = 'open'),
  COUNT(DISTINCT agent_id) FILTER (WHERE status = 'open'),
  COUNT(*) FILTER (
    WHERE status = 'open'
      AND EXISTS (
        SELECT 1 FROM alerts al
        WHERE al.agent_id = vulnerability_findings.agent_id
          AND al.status IN ('open','in_progress')
          AND ( …CVE-text match across alert title/desc/rule_name/metadata,
                or al.severity IN ('critical','high') AND al.detected_at >= NOW() - INTERVAL '7 days' )
      )
  )
FROM vulnerability_findings`,
		Note: "Last counter cross-references alerts by CVE substring + recent severity → 'EDR signal' KPI.",
	})
	sect.Steps = append(sect.Steps, debugTraceStep{
		Label: "severity breakdown (open only)",
		SQL:   "SELECT severity, COUNT(*) FROM vulnerability_findings WHERE status = 'open' GROUP BY severity",
	})
	sect.Steps = append(sect.Steps, debugTraceStep{
		Label: "status breakdown (all rows)",
		SQL:   "SELECT status, COUNT(*) FROM vulnerability_findings GROUP BY status",
	})
	sect.Steps = append(sect.Steps, debugTraceStep{
		Label: "top-10 packages by open finding count",
		SQL: `SELECT package_name, COUNT(*) c
FROM vulnerability_findings
WHERE status = 'open' AND package_name <> ''
GROUP BY package_name
ORDER BY c DESC
LIMIT 10`,
	})
	sect.Steps = append(sect.Steps, debugTraceStep{
		Label: "top-10 hosts by open finding count",
		SQL: `SELECT f.agent_id, a.hostname, COUNT(*) c
FROM vulnerability_findings f
JOIN agents a ON a.id = f.agent_id
WHERE f.status = 'open'
GROUP BY f.agent_id, a.hostname
ORDER BY c DESC
LIMIT 10`,
	})

	stats, err := h.vulnRepo.GetStats(ctx)
	if err != nil {
		sect.Error = err.Error()
		sect.DurationMs = time.Since(start).Milliseconds()
		return sect
	}
	sect.FinalValue = stats
	sect.DurationMs = time.Since(start).Milliseconds()
	return sect
}
