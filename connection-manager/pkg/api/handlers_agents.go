// Package api provides agent handler implementations.
package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/edr-platform/connection-manager/pkg/models"
	edrv1 "github.com/edr-platform/connection-manager/proto/v1"
)

// ListAgents returns paginated list of agents from the database.
func (h *Handlers) ListAgents(c echo.Context) error {
	if h.agentSvc == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Agent service is not available")
	}

	var req AgentListRequest
	if err := c.Bind(&req); err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid query parameters")
	}

	// Set defaults
	if req.Limit == 0 {
		req.Limit = 50
	}

	// Build repository filter
	filter := repository.AgentFilter{
		Limit:     req.Limit,
		Offset:    req.Offset,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}
	if req.Status != "" {
		filter.Status = &req.Status
	}
	if req.OSType != "" {
		filter.OSType = &req.OSType
	}
	if req.Search != "" {
		filter.Search = &req.Search
	}

	// Query agents from database
	agents, err := h.agentSvc.ListAgents(c.Request().Context(), filter)
	if err != nil {
		h.logger.WithError(err).Error("Failed to list agents")
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve agents")
	}

	// Get total count for pagination
	total, err := h.agentSvc.CountAgents(c.Request().Context(), filter)
	if err != nil {
		h.logger.WithError(err).Warn("Failed to count agents")
		total = int64(len(agents))
	}

	// Map to API response models — populate ALL fields for InlineAgentDetail panel
	summaries := make([]AgentSummary, 0, len(agents))
	for _, a := range agents {
		summary := AgentSummary{
			ID:              a.ID,
			Hostname:        a.Hostname,
			Status:          a.Status,
			OSType:          a.OSType,
			OSVersion:       a.OSVersion,
			AgentVersion:    a.AgentVersion,
			LastSeen:        a.LastSeen,
			HealthScore:     a.HealthScore,
			EventsDelivered: a.EventsDelivered,
			CPUCount:        a.CPUCount,
			MemoryMB:        a.MemoryMB,
			IPAddresses:     a.IPAddresses,
			IsIsolated:      a.IsIsolated,
			EventsCollected: a.EventsCollected,
			EventsDropped:   a.EventsDropped,
			CPUUsage:        a.CPUUsage,
			MemoryUsedMB:    a.MemoryUsedMB,
			QueueDepth:      a.QueueDepth,
			CurrentCertID:   a.CurrentCertID,
			InstalledDate:   a.InstalledDate,
			CreatedAt:       a.CreatedAt,
			UpdatedAt:       a.UpdatedAt,
			Tags:            a.Tags,
			Metadata:        a.Metadata,
		}
		if a.CertExpiresAt != nil && !a.CertExpiresAt.IsZero() {
			summary.CertExpiresAt = a.CertExpiresAt
		}
		summaries = append(summaries, summary)
	}

	return c.JSON(http.StatusOK, AgentListResponse{
		Data: summaries,
		Pagination: PaginationResponse{
			Total:   int(total),
			Limit:   req.Limit,
			Offset:  req.Offset,
			HasMore: int64(req.Offset+req.Limit) < total,
		},
		Meta: ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// GetAgent returns a single agent by ID from the database.
func (h *Handlers) GetAgent(c echo.Context) error {
	if h.agentSvc == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Agent service is not available")
	}

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid agent ID format")
	}

	a, err := h.agentSvc.GetByID(c.Request().Context(), id)
	if err != nil {
		return errorResponse(c, http.StatusNotFound, "NOT_FOUND", "Agent not found")
	}

	// Use the actual IPAddresses field from the DB (not metadata)
	ipAddresses := a.IPAddresses

	detail := AgentDetail{
		AgentSummary: AgentSummary{
			ID:              a.ID,
			Hostname:        a.Hostname,
			Status:          a.Status,
			OSType:          a.OSType,
			OSVersion:       a.OSVersion,
			AgentVersion:    a.AgentVersion,
			LastSeen:        a.LastSeen,
			HealthScore:     a.HealthScore,
			EventsDelivered: a.EventsDelivered,
			IsIsolated:      a.IsIsolated,
		},
		IPAddresses:     ipAddresses,
		CPUCount:        a.CPUCount,
		MemoryMB:        a.MemoryMB,
		Tags:            a.Tags,
		EventsGenerated: a.EventsCollected,
		EventsSent:      a.EventsDelivered,
		CPUUsage:        a.CPUUsage,
		MemoryUsedMB:    a.MemoryUsedMB,
		QueueDepth:      a.QueueDepth,
	}
	if a.InstalledDate != nil {
		detail.InstalledDate = *a.InstalledDate
	}
	if a.CertExpiresAt != nil && !a.CertExpiresAt.IsZero() {
		detail.CertExpiresAt = a.CertExpiresAt
	}
	if a.CurrentCertID != nil {
		detail.CurrentCertID = a.CurrentCertID
	}

	return c.JSON(http.StatusOK, AgentDetailResponse{
		Data: detail,
		Meta: ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// UpdateAgent updates agent metadata.
func (h *Handlers) UpdateAgent(c echo.Context) error {
	idStr := c.Param("id")
	_, err := uuid.Parse(idStr)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid agent ID format")
	}

	var req AgentUpdateRequest
	if err := c.Bind(&req); err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
	}

	// TODO: Update in AgentRepository

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Agent updated successfully",
		"meta": ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// DeleteAgent removes an agent.
func (h *Handlers) DeleteAgent(c echo.Context) error {
	idStr := c.Param("id")
	_, err := uuid.Parse(idStr)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid agent ID format")
	}

	// TODO: Delete from AgentRepository

	return c.NoContent(http.StatusNoContent)
}

// GetAgentStats returns agent statistics from the database.
func (h *Handlers) GetAgentStats(c echo.Context) error {
	if h.agentSvc == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Agent service is not available")
	}

	ctx := c.Request().Context()

	// Query counts per status
	statusKeys := []string{"online", "offline", "degraded", "pending", "suspended"}
	counts := make(map[string]int)
	var totalCount int64

	for _, s := range statusKeys {
		status := s
		cnt, err := h.agentSvc.CountAgents(ctx, repository.AgentFilter{Status: &status})
		if err != nil {
			h.logger.WithError(err).Warnf("Failed to count %s agents", s)
			continue
		}
		counts[s] = int(cnt)
		totalCount += cnt
	}

	// Calculate average health from online agents
	var avgHealth float64
	onlineAgents, err := h.agentSvc.GetOnlineAgents(ctx)
	if err == nil && len(onlineAgents) > 0 {
		var totalHealth float64
		for _, a := range onlineAgents {
			totalHealth += a.HealthScore
		}
		avgHealth = totalHealth / float64(len(onlineAgents))
	}

	// Build OS type and version breakdown from all agents
	allAgents, err := h.agentSvc.ListAgents(ctx, repository.AgentFilter{Limit: 10000})
	byOS := make(map[string]int)
	byVersion := make(map[string]int)
	if err == nil {
		for _, a := range allAgents {
			byOS[a.OSType]++
			byVersion[a.AgentVersion]++
		}
	}

	return c.JSON(http.StatusOK, AgentStatsResponse{
		Total:     int(totalCount),
		Online:    counts["online"],
		Offline:   counts["offline"],
		Degraded:  counts["degraded"],
		Pending:   counts["pending"],
		Suspended: counts["suspended"],
		ByOSType:  byOS,
		ByVersion: byVersion,
		AvgHealth: avgHealth,
		Meta: ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// GetAgentEvents returns events for an agent.
func (h *Handlers) GetAgentEvents(c echo.Context) error {
	idStr := c.Param("id")
	_, err := uuid.Parse(idStr)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid agent ID format")
	}

	// TODO: Query events for agent

	return c.JSON(http.StatusOK, EventListResponse{
		Data:       []EventSummary{},
		Pagination: PaginationResponse{Total: 0, Limit: 50, Offset: 0},
		Meta: ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// GetAgentCommands returns command history for an agent.
func (h *Handlers) GetAgentCommands(c echo.Context) error {
	idStr := c.Param("id")
	_, err := uuid.Parse(idStr)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid agent ID format")
	}

	// TODO: Query commands for agent

	return c.JSON(http.StatusOK, CommandListResponse{
		Data:       []CommandSummary{},
		Pagination: PaginationResponse{Total: 0, Limit: 50, Offset: 0},
		Meta: ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// ExecuteAgentCommand pushes a command to a live agent via the AgentRegistry.
// Dual-write strategy:
//  1. Persist command to DB with status=pending (durable record created first)
//  2. Push to agent's live gRPC stream — update DB to sent or failed accordingly
//
// This eliminates the race condition where a command is lost if the stream
// disconnects between the Send() call and the actual delivery.
func (h *Handlers) ExecuteAgentCommand(c echo.Context) error {
	h.logger.Info("[C2] ExecuteAgentCommand called")

	idStr := c.Param("id")
	agentID, err := uuid.Parse(idStr)
	if err != nil {
		h.logger.Warnf("[C2] Invalid agent ID: %s", idStr)
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid agent ID format")
	}

	var req CommandRequest
	if err := c.Bind(&req); err != nil {
		h.logger.Warnf("[C2] Bind failed: %v", err)
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
	}
	h.logger.Infof("[C2] Request bound: agent=%s type=%s timeout=%d", agentID, req.CommandType, req.Timeout)

	// Validate registry is available
	if h.registry == nil {
		h.logger.Warn("[C2] Registry is nil")
		return errorResponse(c, http.StatusServiceUnavailable, "C2_UNAVAILABLE", "Command routing is not available")
	}

	// Special case: start_agent works even when agent is offline.
	// The command is stored in DB (status=pending). When the agent reconnects
	// it auto-starts because: (1) Windows service recovery restarts it, or
	// (2) the redeliverPendingCommands goroutine pushes it on next connect.
	if strings.ToLower(req.CommandType) == "start_agent" {
		// Inject mode so agent's restartService handler knows to only start, not stop
		if req.Parameters == nil {
			req.Parameters = map[string]string{}
		}
		req.Parameters["mode"] = "start"
	}

	// Check if agent is online (skip for start_agent — handled above as offline-safe)
	if strings.ToLower(req.CommandType) != "start_agent" && !h.registry.IsOnline(agentID.String()) {
		h.logger.Warnf("[C2] Agent %s is not online", agentID)
		return errorResponse(c, http.StatusNotFound, "AGENT_OFFLINE", "Agent is not online — command cannot be delivered")
	}

	// ── Step 1: Persist to DB FIRST (status=pending) ──────────────────────────
	// Always create a durable record before attempting delivery so commands are
	// never lost if the gRPC stream disconnects during the Send() call.
	// NOTE: issued_by is intentionally left nil (NULL). The JWT UserID is a
	// claims string that may not match the UUID in the users table, causing FK
	// violations. The issuer username is stored in Metadata for audit purposes.
	commandID := uuid.New()
	if h.commandRepo != nil {
		params := make(map[string]any, len(req.Parameters))
		for k, v := range req.Parameters {
			params[k] = v
		}
		timeout := req.Timeout
		if timeout <= 0 {
			timeout = 300
		}
		// Build metadata with issuer info for audit (avoids FK constraint on issued_by)
		meta := map[string]any{}
		if user := getCurrentUser(c); user != nil {
			meta["issued_by_username"] = user.Username
			if len(user.Roles) > 0 {
				meta["issued_by_role"] = user.Roles[0]
			}
		}
		dbCmd := &models.Command{
			ID:             commandID,
			AgentID:        agentID,
			CommandType:    models.CommandType(req.CommandType),
			Parameters:     params,
			Priority:       5,
			Status:         models.CommandStatusPending,
			TimeoutSeconds: timeout,
			IssuedBy:       nil, // always nil to avoid FK violation
			Metadata:       meta,
		}
		if err := h.commandRepo.Create(c.Request().Context(), dbCmd); err != nil {
			h.logger.WithError(err).Error("[C2] Failed to persist command to DB before dispatch — aborting")
			return errorResponse(c, http.StatusInternalServerError, "DB_ERROR", "Failed to persist command")
		}
		h.logger.Infof("[C2] Command %s persisted to DB (status=pending)", commandID)
	}

	// ── Step 1b: Inject mode parameter for agent service control commands ───────
	// The agent's restartService handler checks Parameters["mode"] to decide
	// whether to stop+start (restart), stop only, or start only.
	switch strings.ToLower(req.CommandType) {
	case "stop_agent", "stop_service":
		if req.Parameters == nil {
			req.Parameters = map[string]string{}
		}
		req.Parameters["mode"] = "stop" // agent: sc stop EDRAgent only
	case "restart_agent", "restart_service":
		if req.Parameters == nil {
			req.Parameters = map[string]string{}
		}
		req.Parameters["mode"] = "restart" // agent: sc stop → sc start
		// start_agent mode already injected in the offline-safe block above
	case "isolate", "isolate_network", "unisolate", "unisolate_network", "restore_network":
		// Auto-inject the C2 server address so the agent builds correct ALLOW
		// firewall rules. The agent falls back to config.server.address when
		// server_address is not provided, but explicit injection is more reliable.
		if h.grpcAddress != "" {
			if req.Parameters == nil {
				req.Parameters = map[string]string{}
			}
			if req.Parameters["server_address"] == "" {
				req.Parameters["server_address"] = h.grpcAddress
			}
		}
	}

	// ── Step 2: Map REST command_type to proto and push to gRPC stream ─────────
	cmdType := mapCommandType(req.CommandType)
	cmd := &edrv1.Command{
		CommandId:  commandID.String(),
		Timestamp:  timestamppb.Now(),
		Type:       cmdType,
		Parameters: req.Parameters,
		Priority:   5,
	}

	if err := h.registry.Send(agentID.String(), cmd); err != nil {
		h.logger.WithError(err).WithField("agent_id", agentID).Warn("[C2] Failed to push command to agent — marking as FAILED in DB")
		// R1 FIX: Explicitly update DB to 'failed' and CHECK the error.
		// If this update also fails, the command stays at 'pending' forever
		// (phantom pending) — log it as CRITICAL so it's never missed.
		if h.commandRepo != nil {
			if uErr := h.commandRepo.UpdateStatus(c.Request().Context(), commandID, models.CommandStatusFailed, nil, err.Error()); uErr != nil {
				h.logger.WithError(uErr).Errorf("[C2] CRITICAL: Failed to update command %s to FAILED in DB — phantom pending command!", commandID)
			} else {
				h.logger.Infof("[C2] Command %s marked FAILED in DB (channel full or agent offline)", commandID)
			}
		}
		return errorResponse(c, http.StatusConflict, "SEND_FAILED", err.Error())
	}

	// ── Step 3: Update DB to 'sent' ────────────────────────────────────────────
	if h.commandRepo != nil {
		if err := h.commandRepo.UpdateStatus(c.Request().Context(), commandID, models.CommandStatusSent, nil, ""); err != nil {
			h.logger.WithError(err).Warn("[C2] Failed to update command status to sent (non-fatal)")
		}
	}

	// ── Step 3b: Proactively update isolation state ──────────────────────────
	// Set is_isolated in the DB immediately at dispatch rather than waiting
	// for the agent's asynchronous SendCommandResult ACK. This eliminates the
	// race between the dashboard's next query and the async result, ensuring
	// the UI shows "Restore Network" (or "Isolate Network") right away.
	if h.agentSvc != nil {
		switch strings.ToLower(req.CommandType) {
		case "isolate_network", "isolate":
			if err := h.agentSvc.SetIsolation(c.Request().Context(), agentID, true); err != nil {
				h.logger.WithError(err).Warn("[Isolation] Failed to proactively set is_isolated=true")
			} else {
				h.logger.Infof("[Isolation] Agent %s proactively marked ISOLATED at dispatch", agentID)
			}
		case "restore_network", "unisolate_network", "unisolate":
			if err := h.agentSvc.SetIsolation(c.Request().Context(), agentID, false); err != nil {
				h.logger.WithError(err).Warn("[Isolation] Failed to proactively set is_isolated=false")
			} else {
				h.logger.Infof("[Isolation] Agent %s proactively marked UN-ISOLATED at dispatch", agentID)
			}
		}
	}

	h.logger.WithFields(logrus.Fields{
		"agent_id":     agentID,
		"command_id":   commandID,
		"command_type": req.CommandType,
		"proto_type":   cmdType.String(),
	}).Info("[C2] Command dispatched to agent via live stream")

	// ── Step 4: Write audit log entry (non-blocking) ────────────────────────
	if h.auditRepo != nil {
		ip, ua := auditContext(c)
		username := "unknown"
		userID := uuid.Nil
		if user := getCurrentUser(c); user != nil {
			username = user.Username
			if uid, parseErr := uuid.Parse(user.UserID); parseErr == nil {
				userID = uid
			}
		}
		auditAction := models.AuditActionCommandExecuted
		if req.CommandType == "isolate" || req.CommandType == "isolate_network" {
			auditAction = models.AuditActionIsolate
		} else if req.CommandType == "unisolate" || req.CommandType == "unisolate_network" || req.CommandType == "restore_network" {
			auditAction = models.AuditActionUnisolate
		} else if req.CommandType == "update_filter_policy" {
			auditAction = models.AuditActionDeployPolicy
		}

		auditEntry := models.NewAuditLog(userID, username, auditAction, "command", commandID).
			WithContext(ip, ua).
			WithDetails("agent=" + agentID.String() + " type=" + req.CommandType)
		go func(entry *models.AuditLog) {
			auditCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := h.auditRepo.Create(auditCtx, entry); err != nil {
				h.logger.WithError(err).Warn("[C2] Failed to write audit log entry (non-fatal)")
			}
		}(auditEntry)
	}

	return c.JSON(http.StatusAccepted, CommandResponse{
		CommandID: commandID.String(),
		Status:    "sent",
		IssuedAt:  time.Now(),
	})
}

// AddProcessException pushes a live allow-exception for process auto-response.
// It uses UPDATE_CONFIG sparse override (exclude_process=<name>) so the agent
// hot-reloads immediately and preserves existing exclusions.
func (h *Handlers) AddProcessException(c echo.Context) error {
	idStr := c.Param("id")
	agentID, err := uuid.Parse(idStr)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid agent ID format")
	}
	var req ProcessExceptionRequest
	if err := c.Bind(&req); err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
	}
	procName := strings.TrimSpace(req.ProcessName)
	if procName == "" {
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "process_name is required")
	}
	if h.registry == nil || h.commandRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "C2_UNAVAILABLE", "Command routing is not available")
	}
	if !h.registry.IsOnline(agentID.String()) {
		return errorResponse(c, http.StatusNotFound, "AGENT_OFFLINE", "Agent is not online — exception cannot be delivered")
	}

	commandID := uuid.New()
	meta := map[string]any{
		"exception_type": "process_allow",
		"process_name":   procName,
		"reason":         strings.TrimSpace(req.Reason),
	}
	if user := getCurrentUser(c); user != nil {
		meta["issued_by_username"] = user.Username
	}
	dbCmd := &models.Command{
		ID:          commandID,
		AgentID:     agentID,
		CommandType: models.CommandType("update_config"),
		Parameters: map[string]any{
			"exclude_process": procName,
		},
		Priority:       5,
		Status:         models.CommandStatusPending,
		TimeoutSeconds: 120,
		IssuedBy:       nil,
		Metadata:       meta,
	}
	if err := h.commandRepo.Create(c.Request().Context(), dbCmd); err != nil {
		h.logger.WithError(err).Error("[C2] Failed to persist process exception command")
		return errorResponse(c, http.StatusInternalServerError, "DB_ERROR", "Failed to persist exception command")
	}

	cmd := &edrv1.Command{
		CommandId: commandID.String(),
		Timestamp: timestamppb.Now(),
		Type:      edrv1.CommandType_COMMAND_TYPE_UPDATE_CONFIG,
		Parameters: map[string]string{
			"exclude_process": procName,
		},
		Priority: 5,
	}
	if err := h.registry.Send(agentID.String(), cmd); err != nil {
		_ = h.commandRepo.UpdateStatus(c.Request().Context(), commandID, models.CommandStatusFailed, nil, err.Error())
		return errorResponse(c, http.StatusConflict, "SEND_FAILED", err.Error())
	}
	if err := h.commandRepo.UpdateStatus(c.Request().Context(), commandID, models.CommandStatusSent, nil, ""); err != nil {
		h.logger.WithError(err).Warn("[C2] Failed to update process exception command status to sent")
	}

	h.logger.WithFields(logrus.Fields{
		"agent_id":   agentID,
		"command_id": commandID,
		"process":    procName,
	}).Info("[C2] Process allow-exception dispatched")

	if h.auditRepo != nil {
		ip, ua := auditContext(c)
		username := "unknown"
		userID := uuid.Nil
		if user := getCurrentUser(c); user != nil {
			username = user.Username
			if uid, parseErr := uuid.Parse(user.UserID); parseErr == nil {
				userID = uid
			}
		}
		entry := models.NewAuditLog(userID, username, models.AuditActionDeployPolicy, "agent", agentID).
			WithContext(ip, ua).
			WithDetails("process_exception=" + procName)
		go func() {
			auditCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = h.auditRepo.Create(auditCtx, entry)
		}()
	}

	return c.JSON(http.StatusAccepted, map[string]interface{}{
		"command_id":   commandID.String(),
		"status":       "sent",
		"agent_id":     agentID.String(),
		"process_name": procName,
		"meta":         responseMeta(c),
	})
}

// mapCommandType maps REST API command type strings to proto CommandType.
// Every command type exposed in the dashboard must have a case here;
// missing entries would cause the agent to receive COMMAND_TYPE_UNSPECIFIED
// and log "unknown command type", silently dropping the command.
func mapCommandType(cmdType string) edrv1.CommandType {
	switch strings.ToLower(cmdType) {
	case "kill_process", "terminate_process":
		return edrv1.CommandType_COMMAND_TYPE_TERMINATE_PROCESS
	case "collect_logs", "collect_forensics":
		return edrv1.CommandType_COMMAND_TYPE_COLLECT_FORENSICS
	case "quarantine_file":
		return edrv1.CommandType(13) // COMMAND_TYPE_QUARANTINE_FILE
	case "block_ip":
		return edrv1.CommandType(14)
	case "unblock_ip":
		return edrv1.CommandType(15)
	case "block_domain":
		return edrv1.CommandType(16)
	case "unblock_domain":
		return edrv1.CommandType(17)
	case "update_signatures":
		return edrv1.CommandType(18)
	case "scan_file", "scan_memory":
		// Map to COLLECT_FORENSICS so the agent receives it; params carry sub-type.
		return edrv1.CommandType_COMMAND_TYPE_COLLECT_FORENSICS
	case "isolate", "isolate_network":
		return edrv1.CommandType_COMMAND_TYPE_ISOLATE
	case "unisolate", "unisolate_network", "restore_network":
		return edrv1.CommandType_COMMAND_TYPE_UNISOLATE
	case "restart_agent", "restart_service":
		return edrv1.CommandType_COMMAND_TYPE_RESTART_SERVICE
	case "stop_agent", "stop_service":
		// Stop only — agent checks Parameters["mode"] == "stop"
		return edrv1.CommandType_COMMAND_TYPE_RESTART_SERVICE
	case "start_agent", "start_service":
		// Start only — agent checks Parameters["mode"] == "start"
		return edrv1.CommandType_COMMAND_TYPE_RESTART_SERVICE
	case "restart", "restart_machine":
		return edrv1.CommandType(10)
	case "shutdown", "shutdown_machine":
		return edrv1.CommandType(11)
	case "update_agent":
		return edrv1.CommandType_COMMAND_TYPE_UPDATE_AGENT
	case "update_config", "update_policy":
		return edrv1.CommandType_COMMAND_TYPE_UPDATE_CONFIG
	case "update_filter_policy":
		return edrv1.CommandType(12) // COMMAND_TYPE_UPDATE_FILTER_POLICY
	case "adjust_rate":
		return edrv1.CommandType_COMMAND_TYPE_ADJUST_RATE
	case "run_cmd", "custom":
		return edrv1.CommandType(9) // COMMAND_TYPE_RUN_CMD
	default:
		return edrv1.CommandType_COMMAND_TYPE_UNSPECIFIED
	}
}
