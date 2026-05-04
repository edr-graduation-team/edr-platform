// Package api provides auth handler implementations.
package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/edr-platform/connection-manager/internal/service"
	"github.com/edr-platform/connection-manager/pkg/models"
)

// Login handles user login — authenticates against the database and issues
// a JWT with the user's real role from the users table.
func (h *Handlers) Login(c echo.Context) error {
	if h.authSvc == nil {
		if h.jwtManager == nil {
			return errorResponse(c, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication service is not configured")
		}
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Database is unavailable — cannot authenticate")
	}

	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
	}

	ip, ua := auditContext(c)

	// Authenticate via AuthService (bcrypt password check, DB user lookup)
	loginResp, err := h.authSvc.Login(c.Request().Context(), req.Username, req.Password)
	if err != nil {
		h.logger.WithField("username", req.Username).WithError(err).Warn("Login failed")

		// Audit: login failure
		if h.auditRepo != nil {
			audit := models.NewAuditLog(uuid.Nil, req.Username, models.AuditActionLoginFailed, "user", uuid.Nil).
				WithContext(ip, ua).
				MarkFailed(err.Error())
			go h.auditRepo.Create(c.Request().Context(), audit) //nolint:errcheck
		}

		return errorResponse(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid username or password")
	}

	// ── MFA challenge branch ─────────────────────────────────────────────
	// AuthService returned a challenge rather than tokens. We intentionally
	// do NOT emit a login-success audit event here — the user is not yet
	// authenticated; the success event fires after VerifyMFA.
	if loginResp.MFAChallenge != nil {
		return c.JSON(http.StatusOK, LoginResponse{
			MFARequired: true,
			MFAChallenge: &MFAChallengeSummary{
				ID:          loginResp.MFAChallenge.ID,
				MaskedEmail: loginResp.MFAChallenge.MaskedEmail,
				ExpiresAt:   loginResp.MFAChallenge.ExpiresAt,
			},
			User: UserResponse{
				ID:         loginResp.User.ID,
				Username:   loginResp.User.Username,
				FullName:   loginResp.User.FullName,
				MFAEnabled: true,
			},
		})
	}

	// Audit: login success
	if h.auditRepo != nil {
		audit := models.NewAuditLog(loginResp.User.ID, loginResp.User.Username, models.AuditActionLoginSuccess, "user", loginResp.User.ID).
			WithContext(ip, ua)
		go h.auditRepo.Create(c.Request().Context(), audit) //nolint:errcheck
	}

	return c.JSON(http.StatusOK, LoginResponse{
		AccessToken:  loginResp.AccessToken,
		RefreshToken: loginResp.RefreshToken,
		ExpiresIn:    int64(time.Until(loginResp.AccessExp).Seconds()),
		TokenType:    "Bearer",
		User: UserResponse{
			ID:         loginResp.User.ID,
			Username:   loginResp.User.Username,
			Email:      loginResp.User.Email,
			FullName:   loginResp.User.FullName,
			Role:       loginResp.User.Role,
			Status:     loginResp.User.Status,
			MFAEnabled: loginResp.User.MFAEnabled,
		},
	})
}

// VerifyMFA completes a login started by Login() when the user has MFA
// enabled. Accepts {challenge_id, code}; on success returns the same shape
// as a successful Login (tokens + user).
func (h *Handlers) VerifyMFA(c echo.Context) error {
	if h.authSvc == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication service is not configured")
	}

	var req MFAVerifyRequest
	if err := c.Bind(&req); err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
	}
	if req.ChallengeID == "" || req.Code == "" {
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "challenge_id and code are required")
	}

	ip, ua := auditContext(c)

	loginResp, err := h.authSvc.VerifyMFA(c.Request().Context(), req.ChallengeID, req.Code)
	if err != nil {
		h.logger.WithError(err).Warn("MFA verification failed")
		switch {
		case errors.Is(err, service.ErrMFAChallengeNotFound):
			return errorResponse(c, http.StatusUnauthorized, "MFA_EXPIRED", "Verification code expired — please log in again")
		case errors.Is(err, service.ErrMFAAttemptsExceeded):
			return errorResponse(c, http.StatusUnauthorized, "MFA_LOCKED", "Too many incorrect attempts — please log in again")
		case errors.Is(err, service.ErrMFACodeInvalid):
			return errorResponse(c, http.StatusUnauthorized, "MFA_INVALID", "Invalid verification code")
		case errors.Is(err, service.ErrMFAUnavailable):
			return errorResponse(c, http.StatusServiceUnavailable, "MFA_UNAVAILABLE", "MFA service unavailable")
		}
		return errorResponse(c, http.StatusUnauthorized, "MFA_FAILED", "MFA verification failed")
	}

	// Audit: login success (deferred from Login so it reflects the real moment
	// the user became authenticated).
	if h.auditRepo != nil {
		audit := models.NewAuditLog(loginResp.User.ID, loginResp.User.Username, models.AuditActionLoginSuccess, "user", loginResp.User.ID).
			WithContext(ip, ua)
		go h.auditRepo.Create(c.Request().Context(), audit) //nolint:errcheck
	}

	return c.JSON(http.StatusOK, LoginResponse{
		AccessToken:  loginResp.AccessToken,
		RefreshToken: loginResp.RefreshToken,
		ExpiresIn:    int64(time.Until(loginResp.AccessExp).Seconds()),
		TokenType:    "Bearer",
		User: UserResponse{
			ID:         loginResp.User.ID,
			Username:   loginResp.User.Username,
			Email:      loginResp.User.Email,
			FullName:   loginResp.User.FullName,
			Role:       loginResp.User.Role,
			Status:     loginResp.User.Status,
			MFAEnabled: loginResp.User.MFAEnabled,
		},
	})
}

// RefreshToken handles token refresh.
func (h *Handlers) RefreshToken(c echo.Context) error {
	if h.jwtManager == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "JWT authentication is not configured")
	}

	var req RefreshTokenRequest
	if err := c.Bind(&req); err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
	}

	accessToken, expiresAt, err := h.jwtManager.RefreshAccessToken(req.RefreshToken)
	if err != nil {
		return errorResponse(c, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "Invalid or expired refresh token")
	}

	return c.JSON(http.StatusOK, RefreshTokenResponse{
		AccessToken: accessToken,
		ExpiresIn:   int64(time.Until(expiresAt).Seconds()),
	})
}

// Logout handles user logout.
func (h *Handlers) Logout(c echo.Context) error {
	ip, ua := auditContext(c)

	// Extract token and add to blacklist
	authHeader := c.Request().Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 {
			token := parts[1]
			if h.redis != nil && h.jwtManager != nil {
				jti, err := h.jwtManager.GetTokenID(token)
				if err != nil {
					h.logger.WithError(err).Warn("Failed to get token ID")
				} else {
					expiresAt := time.Now().Add(24 * time.Hour)
					if err := h.redis.BlacklistToken(c.Request().Context(), jti, expiresAt, "logout"); err != nil {
						h.logger.WithError(err).Warn("Failed to blacklist token")
					}
				}
			}
		}
	}

	// Audit: logout
	if h.auditRepo != nil {
		user := getCurrentUser(c)
		userID := uuid.Nil
		username := "unknown"
		if user != nil {
			username = user.Username
			if uid, err := uuid.Parse(user.UserID); err == nil {
				userID = uid
			}
		}
		audit := models.NewAuditLog(userID, username, models.AuditActionUserLogout, "user", userID).
			WithContext(ip, ua)
		go h.auditRepo.Create(c.Request().Context(), audit) //nolint:errcheck
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Logged out successfully",
	})
}

// GetCurrentUser returns the current authenticated user (full profile when DB is available).
func (h *Handlers) GetCurrentUser(c echo.Context) error {
	user := getCurrentUser(c)
	if user == nil {
		return errorResponse(c, http.StatusUnauthorized, "AUTH_REQUIRED", "Authentication required")
	}

	if h.userRepo != nil && user.UserID != "" {
		if uid, err := uuid.Parse(user.UserID); err == nil {
			dbUser, err := h.userRepo.GetByID(c.Request().Context(), uid)
			if err == nil && dbUser != nil {
				return c.JSON(http.StatusOK, map[string]interface{}{
					"data": UserResponse{
						ID:         dbUser.ID,
						Username:   dbUser.Username,
						Email:      dbUser.Email,
						FullName:   dbUser.FullName,
						Role:       dbUser.Role,
						Status:     dbUser.Status,
						MFAEnabled: dbUser.MFAEnabled,
						LastLogin:  dbUser.LastLogin,
						CreatedAt:  dbUser.CreatedAt,
						UpdatedAt:  dbUser.UpdatedAt,
					},
					"meta": ResponseMeta{
						RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
						Timestamp: time.Now().UTC().Format(time.RFC3339),
					},
				})
			}
		}
	}

	role := ""
	if len(user.Roles) > 0 {
		role = user.Roles[0]
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": UserResponse{
			Username: user.Username,
			Role:     role,
		},
		"meta": ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}
