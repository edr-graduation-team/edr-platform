package api

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type ForensicCollectionsResponse struct {
	Data []any `json:"data"`
	Meta ResponseMeta `json:"meta"`
}

// ListForensicCollections returns recent collect_logs/collect_forensics collections for an agent.
func (h *Handlers) ListForensicCollections(c echo.Context) error {
	if h.forensicRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Forensic repository is not available")
	}
	agentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_AGENT_ID", "Invalid agent ID")
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	rows, err := h.forensicRepo.ListCollectionsByAgent(c.Request().Context(), agentID, limit)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list forensic collections")
	}
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, r)
	}
	return c.JSON(http.StatusOK, ForensicCollectionsResponse{
		Data: out,
		Meta: responseMeta(c),
	})
}

type ForensicEventsResponse struct {
	Data []any `json:"data"`
	NextCursor *int64 `json:"next_cursor,omitempty"`
	Meta ResponseMeta `json:"meta"`
}

// ListForensicEvents returns events for a specific collection and log_type.
func (h *Handlers) ListForensicEvents(c echo.Context) error {
	if h.forensicRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Forensic repository is not available")
	}
	agentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_AGENT_ID", "Invalid agent ID")
	}
	cmdID, err := uuid.Parse(c.Param("commandId"))
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_COMMAND_ID", "Invalid command ID")
	}
	logType := c.QueryParam("log_type")
	if logType == "" {
		return errorResponse(c, http.StatusBadRequest, "MISSING_LOG_TYPE", "log_type is required (e.g. security)")
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	var cursor *int64
	if v := c.QueryParam("cursor"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			cursor = &n
		}
	}
	rows, next, err := h.forensicRepo.ListEvents(c.Request().Context(), agentID, cmdID, logType, limit, cursor)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list forensic events")
	}
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, r)
	}
	return c.JSON(http.StatusOK, ForensicEventsResponse{
		Data: out,
		NextCursor: next,
		Meta: responseMeta(c),
	})
}

