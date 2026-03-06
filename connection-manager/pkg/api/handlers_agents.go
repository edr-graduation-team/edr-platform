// Package api provides agent handler implementations.
package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/edr-platform/connection-manager/internal/repository"
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

	// Map to API response models
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

	// Parse IP addresses from metadata
	var ipAddresses []string
	if ips, ok := a.Metadata["ip_addresses"]; ok && ips != "" {
		ipAddresses = strings.Split(ips, ",")
	}

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
// If the agent is online, the command is delivered instantly over its gRPC stream.
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

	// Check if agent is online
	if !h.registry.IsOnline(agentID.String()) {
		h.logger.Warnf("[C2] Agent %s is not online", agentID)
		return errorResponse(c, http.StatusNotFound, "AGENT_OFFLINE", "Agent is not online — command cannot be delivered")
	}

	// Map REST command_type to proto CommandType
	cmdType := mapCommandType(req.CommandType)

	// Build proto Command
	commandID := uuid.New().String()
	cmd := &edrv1.Command{
		CommandId:  commandID,
		Timestamp:  timestamppb.Now(),
		Type:       cmdType,
		Parameters: req.Parameters,
		Priority:   5,
	}

	// Push to agent's live stream
	if err := h.registry.Send(agentID.String(), cmd); err != nil {
		h.logger.WithError(err).WithField("agent_id", agentID).Warn("Failed to push command to agent")
		return errorResponse(c, http.StatusConflict, "SEND_FAILED", err.Error())
	}

	h.logger.WithFields(logrus.Fields{
		"agent_id":     agentID,
		"command_id":   commandID,
		"command_type": req.CommandType,
	}).Info("[C2] Command dispatched to agent via live stream")

	return c.JSON(http.StatusAccepted, CommandResponse{
		CommandID: commandID,
		Status:    "dispatched",
		IssuedAt:  time.Now(),
	})
}

// mapCommandType maps REST API command type strings to proto CommandType.
func mapCommandType(cmdType string) edrv1.CommandType {
	switch strings.ToLower(cmdType) {
	case "kill_process", "terminate_process":
		return edrv1.CommandType_COMMAND_TYPE_TERMINATE_PROCESS
	case "collect_logs", "collect_forensics":
		return edrv1.CommandType_COMMAND_TYPE_COLLECT_FORENSICS
	case "isolate", "isolate_network":
		return edrv1.CommandType_COMMAND_TYPE_ISOLATE
	case "unisolate", "unisolate_network", "restore_network":
		return edrv1.CommandType_COMMAND_TYPE_UNISOLATE
	case "restart_agent", "restart_service":
		return edrv1.CommandType_COMMAND_TYPE_RESTART_SERVICE
	case "restart", "restart_machine":
		return 10 // COMMAND_TYPE_RESTART — machine reboot (enum value 10)
	case "shutdown", "shutdown_machine":
		return 11 // COMMAND_TYPE_SHUTDOWN — machine power off (enum value 11)
	case "update_agent":
		return edrv1.CommandType_COMMAND_TYPE_UPDATE_AGENT
	case "update_config", "update_policy":
		return edrv1.CommandType_COMMAND_TYPE_UPDATE_CONFIG
	case "adjust_rate":
		return edrv1.CommandType_COMMAND_TYPE_ADJUST_RATE
	case "run_cmd":
		// RUN_CMD uses numeric value 9; agent handles it by string matching
		return 9
	default:
		return edrv1.CommandType_COMMAND_TYPE_UNSPECIFIED
	}
}
