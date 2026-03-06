// Package api provides middleware implementations.
package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"

	"github.com/edr-platform/connection-manager/internal/cache"
	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/edr-platform/connection-manager/internal/service"
	"github.com/edr-platform/connection-manager/pkg/handlers"
	"github.com/edr-platform/connection-manager/pkg/security"
)

// Handlers contains all API handler methods.
type Handlers struct {
	logger              *logrus.Logger
	jwtManager          *security.JWTManager
	redis               *cache.RedisClient
	rateLimiter         *cache.RateLimiter
	agentSvc            service.AgentService                 // optional: nil when DB unavailable
	authSvc             service.AuthService                  // optional: nil when DB unavailable
	caCertPath          string                               // path to the CA certificate for zero-touch provisioning
	enrollmentTokenRepo repository.EnrollmentTokenRepository // optional: nil when DB unavailable
	registry            *handlers.AgentRegistry              // real-time agent command routing
}

// NewHandlers creates a new handlers instance.
func NewHandlers(
	logger *logrus.Logger,
	jwtManager *security.JWTManager,
	redis *cache.RedisClient,
	ratLimiter *cache.RateLimiter,
	agentSvc service.AgentService,
	authSvc service.AuthService,
	caCertPath string,
	enrollmentTokenRepo repository.EnrollmentTokenRepository,
) *Handlers {
	return &Handlers{
		logger:              logger,
		jwtManager:          jwtManager,
		redis:               redis,
		rateLimiter:         ratLimiter,
		agentSvc:            agentSvc,
		authSvc:             authSvc,
		caCertPath:          caCertPath,
		enrollmentTokenRepo: enrollmentTokenRepo,
	}
}

// SetRegistry sets the AgentRegistry for command routing.
func (h *Handlers) SetRegistry(r *handlers.AgentRegistry) {
	h.registry = r
}

// UserClaims represents authenticated user info.
type UserClaims struct {
	UserID   string   `json:"user_id"`
	Username string   `json:"username"`
	Roles    []string `json:"roles"`
}

// ContextKey for context values.
type ContextKey string

const (
	ContextKeyUser ContextKey = "user"
)

// AuthMiddleware validates JWT tokens.
func (h *Handlers) AuthMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if h.jwtManager == nil {
			return errorResponse(c, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "JWT authentication is not configured")
		}

		// Extract token from header
		authHeader := c.Request().Header.Get("Authorization")
		if authHeader == "" {
			return errorResponse(c, http.StatusUnauthorized, "AUTH_REQUIRED", "Authorization header is required")
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			return errorResponse(c, http.StatusUnauthorized, "INVALID_AUTH_FORMAT", "Authorization header must be 'Bearer {token}'")
		}

		token := parts[1]

		// Check if token is blacklisted (by JTI, not raw token string)
		if h.redis != nil && h.jwtManager != nil {
			if jti, err := h.jwtManager.GetTokenID(token); err == nil && jti != "" {
				blacklisted, err := h.redis.IsTokenBlacklisted(c.Request().Context(), jti)
				if err != nil {
					h.logger.WithError(err).Warn("Failed to check token blacklist")
				} else if blacklisted {
					return errorResponse(c, http.StatusUnauthorized, "TOKEN_REVOKED", "Token has been revoked")
				}
			}
		}

		// Validate token
		claims, err := h.jwtManager.ValidateToken(token)
		if err != nil {
			h.logger.WithError(err).WithField("path", c.Request().URL.Path).Warn("JWT validation failed")
			return errorResponse(c, http.StatusUnauthorized, "INVALID_TOKEN", "Invalid or expired token")
		}

		// Extract user claims - Claims has Roles field directly
		user := &UserClaims{
			UserID:   claims.Subject,
			Username: claims.Subject, // Or get from AgentID
			Roles:    claims.Roles,   // Roles field directly on Claims
		}

		// Store user in context
		c.Set(string(ContextKeyUser), user)

		return next(c)
	}
}

// RequireRole checks if user has required role.
func (h *Handlers) RequireRole(roles ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user, ok := c.Get(string(ContextKeyUser)).(*UserClaims)
			if !ok || user == nil {
				return errorResponse(c, http.StatusUnauthorized, "AUTH_REQUIRED", "Authentication required")
			}

			// Check if user has any of the required roles
			for _, requiredRole := range roles {
				for _, userRole := range user.Roles {
					if userRole == requiredRole || userRole == "admin" {
						return next(c)
					}
				}
			}

			return errorResponse(c, http.StatusForbidden, "FORBIDDEN", "Insufficient permissions")
		}
	}
}

// RateLimitMiddleware applies rate limiting per user.
func (h *Handlers) RateLimitMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if h.rateLimiter == nil {
			return next(c)
		}

		// Get user ID or IP for rate limiting
		var key string
		if user, ok := c.Get(string(ContextKeyUser)).(*UserClaims); ok && user != nil {
			key = user.UserID
		} else {
			key = c.RealIP()
		}

		// Allow(ctx, agentID, eventCount) returns (bool, int64, error)
		allowed, _, err := h.rateLimiter.Allow(c.Request().Context(), key, 1)
		if err != nil {
			h.logger.WithError(err).Warn("Rate limit check failed")
			// Continue on error
			return next(c)
		}

		if !allowed {
			return errorResponse(c, http.StatusTooManyRequests, "RATE_LIMITED", "Rate limit exceeded")
		}

		return next(c)
	}
}

// getCurrentUser extracts user from context.
func getCurrentUser(c echo.Context) *UserClaims {
	user, _ := c.Get(string(ContextKeyUser)).(*UserClaims)
	return user
}

// errorResponse returns a standardized error response.
func errorResponse(c echo.Context, status int, code, message string) error {
	return c.JSON(status, ErrorResponse{
		Error:     http.StatusText(status),
		ErrorCode: code,
		Message:   message,
		RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// successResponse returns a standardized success response.
func successResponse(c echo.Context, status int, data interface{}) error {
	return c.JSON(status, data)
}
