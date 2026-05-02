package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/edr-platform/connection-manager/pkg/models"
	edrv1 "github.com/edr-platform/connection-manager/proto/v1"
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

// TriggerSignatureSync runs an immediate MalwareBazaar sync synchronously.
// POST /api/v1/signatures/sync  (JWT protected)
// Returns the number of hashes actually inserted so the UI can show
// "Already up to date" vs "N new hashes added".
func (h *Handlers) TriggerSignatureSync(c echo.Context) error {
	if h.signatureSyncSvc == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Signature sync service not configured")
	}

	ctx := c.Request().Context()
	inserted, err := h.signatureSyncSvc.SyncOnce(ctx)
	if err != nil {
		h.logger.WithError(err).Error("[signatures] Manual sync failed")
		return errorResponse(c, http.StatusInternalServerError, "SYNC_FAILED", fmt.Sprintf("Sync failed: %v", err))
	}

	var message string
	if inserted == 0 {
		message = "Already up to date — no new hashes found"
	} else {
		message = fmt.Sprintf("%d new hash(es) added", inserted)
	}

	maxVer, _ := h.malwareHashRepo.GetMaxVersion(ctx)
	count, _ := h.malwareHashRepo.Count(ctx)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":  message,
		"inserted": inserted,
		"stats": map[string]interface{}{
			"count":       count,
			"max_version": maxVer,
		},
		"meta": responseMeta(c),
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

	var hashes []*models.MalwareHash
	var fetchErr error
	if sinceVersion == 0 {
		// Admin "recent entries" view: show newest hashes first.
		hashes, fetchErr = h.malwareHashRepo.GetLatest(c.Request().Context(), limit)
	} else {
		// Delta query: return entries after the given version cursor (ASC).
		hashes, fetchErr = h.malwareHashRepo.ListSinceVersion(c.Request().Context(), sinceVersion, limit)
	}
	if fetchErr != nil {
		h.logger.WithError(fetchErr).Error("[signatures] ListSignatureHashes failed")
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list signatures")
	}
	count, _ := h.malwareHashRepo.Count(c.Request().Context())

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":  hashes,
		"total": count,
		"meta":  responseMeta(c),
	})
}

type pushSignatureUpdateRequest struct {
	IncludeOffline *bool `json:"include_offline"`
	Limit          int   `json:"limit"`
	Timeout        int   `json:"timeout"`
}

// PushSignatureUpdateAll queues/sends UPDATE_SIGNATURES to all matching agents.
// POST /api/v1/signatures/push-update  (JWT protected)
func (h *Handlers) PushSignatureUpdateAll(c echo.Context) error {
	if h.agentSvc == nil || h.commandRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Agent command pipeline is not available")
	}

	req := pushSignatureUpdateRequest{}
	if err := c.Bind(&req); err != nil && err.Error() != "EOF" {
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
	}

	includeOffline := true
	if req.IncludeOffline != nil {
		includeOffline = *req.IncludeOffline
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 10000
	}
	if limit > 20000 {
		limit = 20000
	}
	timeoutSec := req.Timeout
	if timeoutSec <= 0 {
		timeoutSec = 180
	}
	if timeoutSec > 3600 {
		timeoutSec = 3600
	}

	agents, err := h.agentSvc.ListAgents(c.Request().Context(), repository.AgentFilter{
		Limit:     limit,
		Offset:    0,
		SortBy:    "last_seen",
		SortOrder: "desc",
	})
	if err != nil {
		h.logger.WithError(err).Error("[signatures] ListAgents failed for push-update")
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list agents")
	}

	// Build the NDJSON feed URL that agents will pull from.
	// Scheme: respect X-Forwarded-Proto from nginx, but always default to https
	// because the agent rejects non-HTTPS URLs.
	scheme := c.Scheme()
	if scheme == "" || scheme == "http" {
		scheme = "https"
	}
	feedURL := scheme + "://" + c.Request().Host + "/api/v1/signatures/feed.ndjson"

	sent := 0
	queued := 0
	failed := 0
	skippedOffline := 0
	skippedUninstalled := 0

	for _, a := range agents {
		if a.Status == models.AgentStatusUninstalled {
			skippedUninstalled++
			continue
		}

		online := h.registry != nil && h.registry.IsOnline(a.ID.String())
		if !online && !includeOffline {
			skippedOffline++
			continue
		}

		cmdID := uuid.New()
		meta := map[string]any{
			"bulk_signature_update": true,
		}
		if user := getCurrentUser(c); user != nil {
			meta["issued_by_username"] = user.Username
			if len(user.Roles) > 0 {
				meta["issued_by_role"] = user.Roles[0]
			}
		}

		dbCmd := &models.Command{
			ID:             cmdID,
			AgentID:        a.ID,
			CommandType:    models.CommandType("update_signatures"),
			Parameters:     map[string]any{"url": feedURL},
			Priority:       5,
			Status:         models.CommandStatusPending,
			TimeoutSeconds: timeoutSec,
			IssuedBy:       nil,
			Metadata:       meta,
		}
		if err := h.commandRepo.Create(c.Request().Context(), dbCmd); err != nil {
			failed++
			continue
		}

		if !online {
			queued++
			continue
		}

		cmd := &edrv1.Command{
			CommandId:  cmdID.String(),
			Timestamp:  timestamppb.Now(),
			Type:       mapCommandType("update_signatures"),
			Parameters: map[string]string{"url": feedURL},
			Priority:   5,
			ExpiresAt:  timestamppb.New(time.Now().Add(time.Duration(timeoutSec) * time.Second)),
		}
		if err := h.registry.Send(a.ID.String(), cmd); err != nil {
			_ = h.commandRepo.UpdateStatus(c.Request().Context(), cmdID, models.CommandStatusFailed, nil, err.Error())
			failed++
			continue
		}
		_ = h.commandRepo.UpdateStatus(c.Request().Context(), cmdID, models.CommandStatusSent, nil, "")
		sent++
	}

	return c.JSON(http.StatusAccepted, map[string]interface{}{
		"message": "Bulk signature update dispatch completed",
		"data": map[string]interface{}{
			"processed":           len(agents),
			"sent":                sent,
			"queued":              queued,
			"failed":              failed,
			"skipped_offline":     skippedOffline,
			"skipped_uninstalled": skippedUninstalled,
			"include_offline":     includeOffline,
		},
		"meta": responseMeta(c),
	})
}
