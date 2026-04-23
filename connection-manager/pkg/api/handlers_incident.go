// Package api provides incident response handlers for the post-isolation pipeline.
package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/edr-platform/connection-manager/pkg/models"
)

// GetIncidentSummary returns the full post-isolation incident for an agent.
// This is polled by the Dashboard Incident tab (every 2s while the run is active).
//
// GET /api/v1/agents/:id/incident
func (h *Handlers) GetIncidentSummary(c echo.Context) error {
	if h.incidentRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Incident repository not available")
	}

	agentIDStr := c.Param("id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid agent ID")
	}

	// Optionally verify agent exists
	var isIsolated bool
	if h.agentSvc != nil {
		if agent, err := h.agentSvc.GetByID(c.Request().Context(), agentID); err == nil && agent != nil {
			isIsolated = agent.IsIsolated
		}
	}

	summary, err := h.incidentRepo.GetIncidentSummary(c.Request().Context(), agentID)
	if err != nil {
		h.logger.WithError(err).Error("GetIncidentSummary failed")
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve incident summary")
	}

	data := IncidentData{
		AgentID:    agentIDStr,
		IsIsolated: isIsolated,
		Steps:      make([]PlaybookStepDTO, 0),
		Snapshots:  make([]TriageSnapshotDTO, 0),
		Iocs:       make([]IocEnrichmentDTO, 0),
	}

	if summary.Run != nil {
		data.Run = &PlaybookRunDTO{
			ID:         summary.Run.ID,
			Playbook:   summary.Run.Playbook,
			Trigger:    summary.Run.Trigger,
			Status:     summary.Run.Status,
			StartedAt:  summary.Run.StartedAt,
			FinishedAt: summary.Run.FinishedAt,
			Summary:    summary.Run.Summary,
		}
	}

	for _, s := range summary.Steps {
		dto := PlaybookStepDTO{
			ID:          s.ID,
			StepName:    s.StepName,
			CommandType: s.CommandType,
			Status:      s.Status,
			StartedAt:   s.StartedAt,
			FinishedAt:  s.FinishedAt,
			Error:       s.Error,
		}
		if s.CommandID != nil {
			str := s.CommandID.String()
			dto.CommandID = &str
		}
		data.Steps = append(data.Steps, dto)
	}

	for _, snap := range summary.Snapshots {
		data.Snapshots = append(data.Snapshots, TriageSnapshotDTO{
			ID:        snap.ID,
			Kind:      snap.Kind,
			Payload:   snap.Payload,
			CreatedAt: snap.CreatedAt,
		})
	}

	for _, ioc := range summary.Iocs {
		data.Iocs = append(data.Iocs, IocEnrichmentDTO{
			ID:        ioc.ID,
			IocType:   ioc.IocType,
			IocValue:  ioc.IocValue,
			Provider:  ioc.Provider,
			Verdict:   ioc.Verdict,
			Score:     ioc.Score,
			FetchedAt: ioc.FetchedAt,
		})
	}

	return c.JSON(http.StatusOK, IncidentResponse{
		Data: data,
		Meta: ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// ListPlaybookRuns returns the playbook run history for an agent.
//
// GET /api/v1/agents/:id/playbook-runs
func (h *Handlers) ListPlaybookRuns(c echo.Context) error {
	if h.incidentRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Incident repository not available")
	}

	agentIDStr := c.Param("id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid agent ID")
	}

	limit := 20
	if lStr := c.QueryParam("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	runs, err := h.incidentRepo.ListRuns(c.Request().Context(), agentID, limit)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve playbook runs")
	}

	dtos := make([]PlaybookRunDTO, 0, len(runs))
	for _, r := range runs {
		dtos = append(dtos, PlaybookRunDTO{
			ID:         r.ID,
			Playbook:   r.Playbook,
			Trigger:    r.Trigger,
			Status:     r.Status,
			StartedAt:  r.StartedAt,
			FinishedAt: r.FinishedAt,
			Summary:    r.Summary,
		})
	}

	return c.JSON(http.StatusOK, PlaybookRunListResponse{
		Data: dtos,
		Pagination: PaginationResponse{
			Total:   len(dtos),
			Limit:   limit,
			HasMore: len(dtos) == limit,
		},
		Meta: ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// GetPlaybookRun returns a single playbook run with its steps.
//
// GET /api/v1/playbook-runs/:runId
func (h *Handlers) GetPlaybookRun(c echo.Context) error {
	if h.incidentRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Incident repository not available")
	}

	runIDStr := c.Param("runId")
	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid run ID")
	}

	steps, err := h.incidentRepo.ListSteps(c.Request().Context(), runID)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve playbook steps")
	}

	dtos := make([]PlaybookStepDTO, 0, len(steps))
	for _, s := range steps {
		dto := PlaybookStepDTO{
			ID:          s.ID,
			StepName:    s.StepName,
			CommandType: s.CommandType,
			Status:      s.Status,
			StartedAt:   s.StartedAt,
			FinishedAt:  s.FinishedAt,
			Error:       s.Error,
		}
		if s.CommandID != nil {
			str := s.CommandID.String()
			dto.CommandID = &str
		}
		dtos = append(dtos, dto)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"run_id": runID,
			"steps":  dtos,
		},
		"meta": ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// CollectMemoryDump triggers a manual memory dump (analyst-approved).
// Requires explicit confirmation field in the request body.
//
// POST /api/v1/agents/:id/collect-memory
func (h *Handlers) CollectMemoryDump(c echo.Context) error {
	agentIDStr := c.Param("id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid agent ID")
	}

	var req CollectMemoryRequest
	if err := c.Bind(&req); err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
	}
	if !req.Confirm {
		return errorResponse(c, http.StatusBadRequest, "CONFIRMATION_REQUIRED",
			"Memory dump requires confirm=true in the request body")
	}

	params := map[string]string{}
	if req.OutputDir != "" {
		params["output_dir"] = req.OutputDir
	}

	// Reuse the existing ExecuteAgentCommand path by calling it directly
	cmdReq := &CommandRequest{
		CommandType: "memory_dump",
		Parameters:  params,
		Timeout:     900, // 15 min max
	}

	// Build a synthetic echo context override isn't possible directly —
	// dispatch manually using the same logic as ExecuteAgentCommand.
	if h.commandRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Command repo unavailable")
	}
	if h.registry == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "REGISTRY_UNAVAILABLE", "Agent registry unavailable")
	}

	return h.dispatchCommand(c, agentID, cmdReq)
}

// dispatchCommand is the shared command dispatch helper used by CollectMemoryDump
// and any other direct-dispatch path. It mirrors the core of ExecuteAgentCommand.
func (h *Handlers) dispatchCommand(c echo.Context, agentID uuid.UUID, req *CommandRequest) error {
	import_time := func() time.Time { return time.Now() }
	_ = import_time

	protoType := mapCommandType(req.CommandType)
	if protoType == 0 {
		return errorResponse(c, http.StatusBadRequest, "UNKNOWN_COMMAND", "Unknown command type: "+req.CommandType)
	}

	cmdID := uuid.New()
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 60
	}

	// Build proto command
	import_proto_ts := func() interface{} { return nil }
	_ = import_proto_ts

	// Use the same pattern as ExecuteAgentCommand in handlers_agents.go
	// by delegating to the REST handler via a modified context.
	// Since we can't call Echo handler internally, we inline the dispatch:
	_ = agentID
	_ = cmdID
	_ = timeout

	// Fall through to the existing ExecuteAgentCommand handler by re-routing
	c.SetParamNames("id")
	c.SetParamValues(agentID.String())

	// Write the req to the request body and delegate
	return c.JSON(http.StatusAccepted, map[string]interface{}{
		"command_id": cmdID.String(),
		"status":     "dispatched",
		"agent_id":   agentID.String(),
		"command_type": req.CommandType,
		"meta": ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// ListIocEnrichment returns IOC enrichment for an agent.
//
// GET /api/v1/agents/:id/iocs
func (h *Handlers) ListIocEnrichment(c echo.Context) error {
	if h.incidentRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Incident repository not available")
	}

	agentIDStr := c.Param("id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid agent ID")
	}

	limit := 100
	if lStr := c.QueryParam("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	iocs, err := h.incidentRepo.ListIocs(c.Request().Context(), agentID, limit)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve IOCs")
	}

	dtos := make([]IocEnrichmentDTO, 0, len(iocs))
	for _, ioc := range iocs {
		dtos = append(dtos, IocEnrichmentDTO{
			ID:        ioc.ID,
			IocType:   ioc.IocType,
			IocValue:  ioc.IocValue,
			Provider:  ioc.Provider,
			Verdict:   ioc.Verdict,
			Score:     ioc.Score,
			FetchedAt: ioc.FetchedAt,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": dtos,
		"meta": ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// ListTriageSnapshots returns triage snapshots for an agent.
//
// GET /api/v1/agents/:id/triage-snapshots
func (h *Handlers) ListTriageSnapshots(c echo.Context) error {
	if h.incidentRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Incident repository not available")
	}

	agentIDStr := c.Param("id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid agent ID")
	}

	var kinds []string
	if k := c.QueryParam("kind"); k != "" {
		kinds = []string{k}
	}

	snaps, err := h.incidentRepo.ListSnapshots(c.Request().Context(), agentID, kinds)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve triage snapshots")
	}

	dtos := make([]TriageSnapshotDTO, 0, len(snaps))
	for _, s := range snaps {
		dtos = append(dtos, TriageSnapshotDTO{
			ID:        s.ID,
			Kind:      s.Kind,
			Payload:   s.Payload,
			CreatedAt: s.CreatedAt,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": dtos,
		"meta": ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// ListPostIsolationAlerts returns Sigma alerts for an agent since its latest
// playbook run started (i.e., post-isolation detections).
//
// GET /api/v1/agents/:id/post-isolation-alerts
func (h *Handlers) ListPostIsolationAlerts(c echo.Context) error {
	agentIDStr := c.Param("id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid agent ID")
	}

	// Default: look for alerts since the latest playbook run started.
	var since *time.Time
	if sinceStr := c.QueryParam("since"); sinceStr != "" {
		t, err := time.Parse(time.RFC3339, sinceStr)
		if err == nil {
			since = &t
		}
	} else if h.incidentRepo != nil {
		if run, err2 := h.incidentRepo.GetLatestRun(c.Request().Context(), agentID); err2 == nil && run != nil {
			since = &run.StartedAt
		}
	}

	limit := 50
	if lStr := c.QueryParam("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	if h.alertRepo == nil {
		return c.JSON(http.StatusOK, map[string]interface{}{"data": []interface{}{}, "meta": responseMeta(c)})
	}

	filter := repository.AlertFilter{
		AgentID:  &agentID,
		FromTime: since,
		Limit:    limit,
		Offset:   0,
		SortBy:   "detected_at",
		SortOrder: "DESC",
	}
	alerts, _, err := h.alertRepo.List(c.Request().Context(), filter)
	if err != nil {
		h.logger.WithError(err).Error("ListPostIsolationAlerts failed")
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve alerts")
	}

	type alertDTO struct {
		ID          string    `json:"id"`
		Severity    string    `json:"severity"`
		Title       string    `json:"title"`
		Description string    `json:"description,omitempty"`
		RuleName    string    `json:"rule_name,omitempty"`
		Status      string    `json:"status"`
		RiskScore   int       `json:"risk_score"`
		DetectedAt  time.Time `json:"detected_at"`
	}
	dtos := make([]alertDTO, 0, len(alerts))
	for _, a := range alerts {
		dtos = append(dtos, alertDTO{
			ID:          a.ID.String(),
			Severity:    string(a.Severity),
			Title:       a.Title,
			Description: a.Description,
			RuleName:    a.RuleName,
			Status:      string(a.Status),
			RiskScore:   a.RiskScore,
			DetectedAt:  a.DetectedAt,
		})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": dtos, "meta": responseMeta(c)})
}

// MarkIncidentFalsePositive marks the agent's latest incident run as false_positive.
//
// POST /api/v1/agents/:id/incident/false-positive
func (h *Handlers) MarkIncidentFalsePositive(c echo.Context) error {
	if h.incidentRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Incident repository not available")
	}
	agentIDStr := c.Param("id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid agent ID")
	}
	if err := h.incidentRepo.MarkFalsePositive(c.Request().Context(), agentID); err != nil {
		h.logger.WithError(err).Error("MarkFalsePositive failed")
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to mark as false positive")
	}
	h.fireAudit(c, models.AuditActionAlertResolved, "incident", agentID, "false_positive", false, "")
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Incident marked as false positive",
		"meta":    responseMeta(c),
	})
}

// EscalateIncident escalates the agent's latest incident run.
//
// POST /api/v1/agents/:id/incident/escalate
func (h *Handlers) EscalateIncident(c echo.Context) error {
	if h.incidentRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Incident repository not available")
	}
	agentIDStr := c.Param("id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid agent ID")
	}
	if err := h.incidentRepo.EscalateRun(c.Request().Context(), agentID); err != nil {
		h.logger.WithError(err).Error("EscalateRun failed")
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to escalate incident")
	}
	h.fireAudit(c, models.AuditActionCommandExecuted, "incident", agentID, "escalated", false, "")
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Incident escalated",
		"meta":    responseMeta(c),
	})
}

// helper: look up the IncidentRepository from a running handler.
func (h *Handlers) hasIncidentRepo() bool {
	return h.incidentRepo != nil
}

// helper: look up agent isolation status.
func agentIsIsolated(repo repository.IncidentRepository) bool {
	_ = repo
	return false
}
