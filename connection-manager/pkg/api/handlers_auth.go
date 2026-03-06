// Package api provides auth handler implementations.
package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// Login handles user login — authenticates against the database and issues
// a JWT with the user's real role from the users table.
func (h *Handlers) Login(c echo.Context) error {
	if h.authSvc == nil {
		// Fallback when DB/AuthService is unavailable: use jwtManager directly
		// with a placeholder (should not happen in production).
		if h.jwtManager == nil {
			return errorResponse(c, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication service is not configured")
		}
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Database is unavailable — cannot authenticate")
	}

	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
	}

	// Authenticate via AuthService (bcrypt password check, DB user lookup)
	loginResp, err := h.authSvc.Login(c.Request().Context(), req.Username, req.Password)
	if err != nil {
		h.logger.WithField("username", req.Username).WithError(err).Warn("Login failed")
		return errorResponse(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid username or password")
	}

	return c.JSON(http.StatusOK, LoginResponse{
		AccessToken:  loginResp.AccessToken,
		RefreshToken: loginResp.RefreshToken,
		ExpiresIn:    int64(time.Until(loginResp.AccessExp).Seconds()),
		TokenType:    "Bearer",
		User: UserResponse{
			ID:       loginResp.User.ID,
			Username: loginResp.User.Username,
			Email:    loginResp.User.Email,
			FullName: loginResp.User.FullName,
			Role:     loginResp.User.Role,
			Status:   loginResp.User.Status,
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

	// RefreshAccessToken(refreshToken) returns (string, time.Time, error)
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
	// Extract token and add to blacklist
	authHeader := c.Request().Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 {
			token := parts[1]
			if h.redis != nil && h.jwtManager != nil {
				// Get JTI from token
				jti, err := h.jwtManager.GetTokenID(token)
				if err != nil {
					h.logger.WithError(err).Warn("Failed to get token ID")
				} else {
					// BlacklistToken(ctx, jti, expiresAt, reason)
					expiresAt := time.Now().Add(24 * time.Hour)
					if err := h.redis.BlacklistToken(c.Request().Context(), jti, expiresAt, "logout"); err != nil {
						h.logger.WithError(err).Warn("Failed to blacklist token")
					}
				}
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Logged out successfully",
	})
}

// GetCurrentUser returns the current authenticated user.
func (h *Handlers) GetCurrentUser(c echo.Context) error {
	user := getCurrentUser(c)
	if user == nil {
		return errorResponse(c, http.StatusUnauthorized, "AUTH_REQUIRED", "Authentication required")
	}

	// Return role from the JWT claims (which was set from DB on login)
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
