package api

import (
	"net/http"
	"strconv"

	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/labstack/echo/v4"
)

// ============================================================================
// APPLICATION CONTROL HANDLERS
// ============================================================================

// GetProcessAnalytics returns server-side aggregated process execution data.
// This avoids the client needing to fetch and aggregate thousands of raw events.
//
// Query params:
//   - hours: lookback window in hours (default 24, max 168)
func (h *Handlers) GetProcessAnalytics(c echo.Context) error {
	if h.eventRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Event repository is not available")
	}

	hours := 24
	if v := c.QueryParam("hours"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 168 {
			hours = n
		}
	}

	repo, ok := h.eventRepo.(*repository.PostgresEventRepository)
	if !ok {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Process analytics requires PostgreSQL repository")
	}

	rows, totalEvents, err := repo.GetProcessAnalytics(c.Request().Context(), hours)
	if err != nil {
		h.logger.WithError(err).Error("Failed to fetch process analytics")
		return errorResponse(c, http.StatusInternalServerError, "FETCH_FAILED", err.Error())
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":         rows,
		"total_events": totalEvents,
		"hours":        hours,
		"meta":         responseMeta(c),
	})
}

// GetSoftwareInventory returns aggregated installed software data from
// software_inventory events emitted by the agent's WMI collector.
func (h *Handlers) GetSoftwareInventory(c echo.Context) error {
	if h.eventRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Event repository is not available")
	}

	repo, ok := h.eventRepo.(*repository.PostgresEventRepository)
	if !ok {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Software inventory requires PostgreSQL repository")
	}

	rows, err := repo.GetSoftwareInventory(c.Request().Context())
	if err != nil {
		h.logger.WithError(err).Error("Failed to fetch software inventory")
		return errorResponse(c, http.StatusInternalServerError, "FETCH_FAILED", err.Error())
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":  rows,
		"total": len(rows),
		"meta":  responseMeta(c),
	})
}
