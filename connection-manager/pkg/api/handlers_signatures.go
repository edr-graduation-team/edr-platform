package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

// GetSignatureVersion returns the current max version and total hash count.
// GET /api/v1/signatures/version  (public — agents poll this before downloading)
func (h *Handlers) GetSignatureVersion(c echo.Context) error {
	if h.malwareHashRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Signature store not available")
	}
	ctx := c.Request().Context()

	maxVer, err := h.malwareHashRepo.GetMaxVersion(ctx)
	if err != nil {
		h.logger.WithError(err).Error("[signatures] GetMaxVersion failed")
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to query signature version")
	}
	count, err := h.malwareHashRepo.Count(ctx)
	if err != nil {
		h.logger.WithError(err).Error("[signatures] Count failed")
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to query signature count")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"max_version": maxVer,
		"count":       count,
	})
}

// GetSignatureFeed streams NDJSON delta since since_version.
// GET /api/v1/signatures/feed.ndjson?since_version=N&limit=5000  (public — agents download delta)
func (h *Handlers) GetSignatureFeed(c echo.Context) error {
	if h.malwareHashRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Signature store not available")
	}

	sinceVersion := int64(0)
	if v := c.QueryParam("since_version"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err == nil && n >= 0 {
			sinceVersion = n
		}
	}
	limit := 5000
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50000 {
			limit = n
		}
	}

	hashes, err := h.malwareHashRepo.ListSinceVersion(c.Request().Context(), sinceVersion, limit)
	if err != nil {
		h.logger.WithError(err).Error("[signatures] ListSinceVersion failed")
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch signature delta")
	}

	c.Response().Header().Set(echo.HeaderContentType, "application/x-ndjson")
	c.Response().WriteHeader(http.StatusOK)

	enc := json.NewEncoder(c.Response())
	for _, h2 := range hashes {
		if err := enc.Encode(map[string]interface{}{
			"sha256":   h2.SHA256,
			"name":     h2.Name,
			"family":   h2.Family,
			"severity": h2.Severity,
			"source":   h2.Source,
			"version":  h2.Version,
		}); err != nil {
			break
		}
	}
	c.Response().Flush()
	return nil
}

// GetSignatureStats returns admin-level stats about the hash feed.
// GET /api/v1/signatures/stats  (JWT protected)
func (h *Handlers) GetSignatureStats(c echo.Context) error {
	if h.malwareHashRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Signature store not available")
	}
	ctx := c.Request().Context()

	maxVer, _ := h.malwareHashRepo.GetMaxVersion(ctx)
	count, _ := h.malwareHashRepo.Count(ctx)
	sources, _ := h.malwareHashRepo.SourceBreakdown(ctx)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"max_version": maxVer,
		"count":       count,
		"sources":     sources,
		"meta":        responseMeta(c),
	})
}

// TriggerSignatureSync triggers an immediate MalwareBazaar sync.
// POST /api/v1/signatures/sync  (JWT protected)
func (h *Handlers) TriggerSignatureSync(c echo.Context) error {
	if h.signatureSyncSvc == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Signature sync service not configured")
	}
	h.signatureSyncSvc.TriggerNow()
	return c.JSON(http.StatusAccepted, map[string]interface{}{
		"message": "Signature sync triggered",
		"meta":    responseMeta(c),
	})
}

// ListSignatureHashes returns a paginated list of stored hashes (admin view).
// GET /api/v1/signatures  (JWT protected)
func (h *Handlers) ListSignatureHashes(c echo.Context) error {
	if h.malwareHashRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Signature store not available")
	}

	sinceVersion := int64(0)
	limit := 100
	if v := c.QueryParam("since_version"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			sinceVersion = n
		}
	}
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}

	hashes, err := h.malwareHashRepo.ListSinceVersion(c.Request().Context(), sinceVersion, limit)
	if err != nil {
		h.logger.WithError(err).Error("[signatures] ListSignatureHashes failed")
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list signatures")
	}
	count, _ := h.malwareHashRepo.Count(c.Request().Context())

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":  hashes,
		"total": count,
		"meta":  responseMeta(c),
	})
}
