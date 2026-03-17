// Package api provides enrollment token management REST handlers.
package api

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/edr-platform/connection-manager/pkg/models"
)

// --------------------------------------------------------------------------
// Request / Response types
// --------------------------------------------------------------------------

// CreateEnrollmentTokenRequest is the JSON body for token generation.
type CreateEnrollmentTokenRequest struct {
	Description string `json:"description"`
	ExpiresInH  *int   `json:"expires_in_hours"` // nil = never expires
	MaxUses     *int   `json:"max_uses"`         // nil = unlimited
}

// EnrollmentTokenResponse is the JSON representation returned to the dashboard.
type EnrollmentTokenResponse struct {
	ID          string     `json:"id"`
	Token       string     `json:"token"`
	Description string     `json:"description"`
	IsActive    bool       `json:"is_active"`
	ExpiresAt   *time.Time `json:"expires_at"`
	UseCount    int        `json:"use_count"`
	MaxUses     *int       `json:"max_uses"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	RevokedAt   *time.Time `json:"revoked_at"`
}

func enrollmentTokenToResponse(t *models.EnrollmentToken) EnrollmentTokenResponse {
	return EnrollmentTokenResponse{
		ID:          t.ID.String(),
		Token:       t.Token,
		Description: t.Description,
		IsActive:    t.IsActive,
		ExpiresAt:   t.ExpiresAt,
		UseCount:    t.UseCount,
		MaxUses:     t.MaxUses,
		CreatedBy:   t.CreatedBy,
		CreatedAt:   t.CreatedAt,
		RevokedAt:   t.RevokedAt,
	}
}

// --------------------------------------------------------------------------
// Handlers
// --------------------------------------------------------------------------

// ListEnrollmentTokens returns all enrollment tokens.
func (h *Handlers) ListEnrollmentTokens(c echo.Context) error {
	if h.enrollmentTokenRepo == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "database unavailable"})
	}

	tokens, err := h.enrollmentTokenRepo.List(c.Request().Context())
	if err != nil {
		h.logger.Errorf("ListEnrollmentTokens: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list tokens"})
	}

	// Never return nil list to frontend
	resp := make([]EnrollmentTokenResponse, 0, len(tokens))
	for _, t := range tokens {
		resp = append(resp, enrollmentTokenToResponse(t))
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": resp,
	})
}

// GenerateEnrollmentToken creates a new enrollment token.
func (h *Handlers) GenerateEnrollmentToken(c echo.Context) error {
	if h.enrollmentTokenRepo == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "database unavailable"})
	}

	var req CreateEnrollmentTokenRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	tokenStr, err := models.GenerateSecureToken()
	if err != nil {
		h.logger.Errorf("GenerateEnrollmentToken: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
	}

	token := &models.EnrollmentToken{
		ID:          uuid.New(),
		Token:       tokenStr,
		Description: req.Description,
		IsActive:    true,
		MaxUses:     req.MaxUses,
		CreatedBy:   "admin", // TODO: extract from JWT claims
	}

	if req.ExpiresInH != nil && *req.ExpiresInH > 0 {
		exp := time.Now().Add(time.Duration(*req.ExpiresInH) * time.Hour)
		token.ExpiresAt = &exp
	}

	if err := h.enrollmentTokenRepo.Create(c.Request().Context(), token); err != nil {
		h.logger.Errorf("GenerateEnrollmentToken: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create token"})
	}

	return c.JSON(http.StatusCreated, enrollmentTokenToResponse(token))
}

// RevokeEnrollmentToken deactivates an enrollment token.
func (h *Handlers) RevokeEnrollmentToken(c echo.Context) error {
	if h.enrollmentTokenRepo == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "database unavailable"})
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid token ID"})
	}

	if err := h.enrollmentTokenRepo.Revoke(c.Request().Context(), id); err != nil {
		if err == repository.ErrNotFound {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "token not found"})
		}
		h.logger.Errorf("RevokeEnrollmentToken: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to revoke token"})
	}

	// Audit: token revoked
	h.fireAudit(c, models.AuditActionTokenRevoked, "token", id, "", false, "")

	return c.JSON(http.StatusOK, map[string]string{"status": "revoked"})
}
