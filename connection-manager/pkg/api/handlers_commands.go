// Package api provides command handler implementations for the Action Center.
package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/edr-platform/connection-manager/internal/repository"
)

// ListCommands returns paginated list of commands for the Action Center.
func (h *Handlers) ListCommands(c echo.Context) error {
	if h.commandRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Command repository is not available")
	}

	// Parse query parameters
	var limit, offset int
	echo.QueryParamsBinder(c).Int("limit", &limit).Int("offset", &offset)
	if limit <= 0 {
		limit = 50
	}

	status := c.QueryParam("status")
	commandType := c.QueryParam("command_type")
	agentIDStr := c.QueryParam("agent_id")
	sortBy := c.QueryParam("sort_by")
	sortOrder := c.QueryParam("sort_order")

	filter := repository.CommandListFilter{
		Limit:     limit,
		Offset:    offset,
		SortBy:    sortBy,
		SortOrder: sortOrder,
	}
	if status != "" {
		filter.Status = &status
	}
	if commandType != "" {
		filter.CommandType = &commandType
	}
	if agentIDStr != "" {
		if id, err := uuid.Parse(agentIDStr); err == nil {
			filter.AgentID = &id
		}
	}

	items, total, err := h.commandRepo.ListAll(c.Request().Context(), filter)
	if err != nil {
		h.logger.WithError(err).Error("Failed to list commands")
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve commands")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": items,
		"pagination": PaginationResponse{
			Total:   int(total),
			Limit:   limit,
			Offset:  offset,
			HasMore: int64(offset+limit) < total,
		},
		"meta": ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// GetCommandStats returns aggregate command statistics for Action Center KPIs.
func (h *Handlers) GetCommandStats(c echo.Context) error {
	if h.commandRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Command repository is not available")
	}

	stats, err := h.commandRepo.GetStats(c.Request().Context())
	if err != nil {
		h.logger.WithError(err).Error("Failed to get command stats")
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve command statistics")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": stats,
		"meta": ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// GetCommand returns one command by ID (same row shape as Action Center / agents/:id/commands).
func (h *Handlers) GetCommand(c echo.Context) error {
	if h.commandRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Command repository is not available")
	}

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid command ID format")
	}

	cmd, err := h.commandRepo.GetByID(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errorResponse(c, http.StatusNotFound, "NOT_FOUND", "Command not found")
		}
		h.logger.WithError(err).Error("GetCommand failed")
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve command")
	}

	hostname := ""
	if h.agentSvc != nil {
		if a, err := h.agentSvc.GetByID(c.Request().Context(), cmd.AgentID); err == nil && a != nil {
			hostname = a.Hostname
		}
	}

	issuedByUser := ""
	if cmd.Metadata != nil {
		if u, ok := cmd.Metadata["issued_by_username"].(string); ok {
			issuedByUser = u
		}
	}

	item := repository.CommandListItem{
		Command:       *cmd,
		AgentHostname: hostname,
		IssuedByUser:  issuedByUser,
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": item,
		"meta": ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}
