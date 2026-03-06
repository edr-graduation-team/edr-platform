// Package api provides the REST API server for dashboard integration.
package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/sirupsen/logrus"

	"github.com/edr-platform/connection-manager/config"
	"github.com/edr-platform/connection-manager/pkg/metrics"
)

// Server represents the REST API server.
type Server struct {
	echo    *echo.Echo
	config  *config.APIConfig
	logger  *logrus.Logger
	metrics *metrics.Metrics
}

// NewServer creates a new REST API server.
func NewServer(cfg *config.APIConfig, logger *logrus.Logger, m *metrics.Metrics) *Server {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Configure middleware stack
	e.Use(middleware.RequestIDWithConfig(middleware.RequestIDConfig{
		Generator: func() string {
			return fmt.Sprintf("req-%d", time.Now().UnixNano())
		},
	}))

	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: `{"time":"${time_rfc3339}","id":"${id}","method":"${method}","uri":"${uri}","status":${status},"latency":"${latency_human}","bytes_out":${bytes_out}}` + "\n",
	}))

	e.Use(middleware.Recover())

	// CORS configuration
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     cfg.CORSAllowOrigins,
		AllowMethods:     cfg.CORSAllowMethods,
		AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
		AllowCredentials: true,
		MaxAge:           3600,
	}))

	// Request timeout
	e.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{
		Timeout: cfg.RequestTimeout,
	}))

	// Body limit
	e.Use(middleware.BodyLimit(fmt.Sprintf("%dM", cfg.MaxRequestBodySize/(1024*1024))))

	// Gzip compression
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Level: 5,
	}))

	return &Server{
		echo:    e,
		config:  cfg,
		logger:  logger,
		metrics: m,
	}
}

// RegisterRoutes registers all API routes.
func (s *Server) RegisterRoutes(handlers *Handlers) {
	// Health check (no auth)
	s.echo.GET("/healthz", s.healthCheck)
	s.echo.GET("/readyz", s.readyCheck)

	// API v1 group
	v1 := s.echo.Group("/api/v1")

	// Zero-touch provisioning: public CA cert endpoint (no auth required)
	v1.GET("/agent/ca", handlers.ServeCA)

	// Auth endpoints (no auth required for login)
	auth := v1.Group("/auth")
	auth.POST("/login", handlers.Login)
	auth.POST("/refresh", handlers.RefreshToken)
	auth.POST("/logout", handlers.Logout, handlers.AuthMiddleware)
	auth.GET("/me", handlers.GetCurrentUser, handlers.AuthMiddleware)

	// Protected endpoints
	protected := v1.Group("")
	protected.Use(handlers.AuthMiddleware)

	// Agent endpoints
	agents := protected.Group("/agents")
	agents.GET("", handlers.ListAgents)
	agents.GET("/stats", handlers.GetAgentStats)
	agents.GET("/:id", handlers.GetAgent)
	agents.PATCH("/:id", handlers.UpdateAgent)
	agents.DELETE("/:id", handlers.DeleteAgent)
	agents.GET("/:id/events", handlers.GetAgentEvents)
	agents.GET("/:id/commands", handlers.GetAgentCommands)
	agents.POST("/:id/commands", handlers.ExecuteAgentCommand)

	// Alert endpoints
	alerts := protected.Group("/alerts")
	alerts.GET("", handlers.ListAlerts)
	alerts.POST("/search", handlers.SearchAlerts)
	alerts.GET("/stats", handlers.GetAlertStats)
	alerts.GET("/:id", handlers.GetAlert)
	alerts.PATCH("/:id", handlers.UpdateAlert)
	alerts.POST("/:id/resolve", handlers.ResolveAlert)
	alerts.POST("/:id/notes", handlers.AddAlertNote)
	alerts.DELETE("/:id", handlers.DeleteAlert)

	// Event endpoints
	events := protected.Group("/events")
	events.POST("/search", handlers.SearchEvents)
	events.GET("/stats", handlers.GetEventStats)
	events.GET("/:id", handlers.GetEvent)
	events.POST("/export", handlers.ExportEvents)

	// Policy endpoints
	policies := protected.Group("/policies")
	policies.GET("", handlers.ListPolicies)
	policies.POST("", handlers.CreatePolicy)
	policies.GET("/:id", handlers.GetPolicy)
	policies.PATCH("/:id", handlers.UpdatePolicy)
	policies.DELETE("/:id", handlers.DeletePolicy)
	policies.GET("/:id/agents", handlers.GetPolicyAgents)

	// User endpoints (admin only)
	users := protected.Group("/users")
	users.GET("", handlers.ListUsers, handlers.RequireRole("admin"))
	users.POST("", handlers.CreateUser, handlers.RequireRole("admin"))
	users.GET("/:id", handlers.GetUser)
	users.PATCH("/:id", handlers.UpdateUser)
	users.DELETE("/:id", handlers.DeleteUser, handlers.RequireRole("admin"))
	users.POST("/:id/password", handlers.ChangePassword)

	// Audit endpoints
	audit := protected.Group("/audit")
	audit.GET("/logs", handlers.ListAuditLogs, handlers.RequireRole("admin", "security"))
	audit.GET("/logs/:id", handlers.GetAuditLog, handlers.RequireRole("admin", "security"))

	// Enrollment token management (admin only)
	tokens := protected.Group("/enrollment-tokens")
	tokens.GET("", handlers.ListEnrollmentTokens, handlers.RequireRole("admin", "security"))
	tokens.POST("", handlers.GenerateEnrollmentToken, handlers.RequireRole("admin"))
	tokens.POST("/:id/revoke", handlers.RevokeEnrollmentToken, handlers.RequireRole("admin"))
}

// healthCheck returns server health status.
func (s *Server) healthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// readyCheck returns server readiness status.
func (s *Server) readyCheck(c echo.Context) error {
	// TODO: Check database, Redis, Kafka connectivity
	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":    "ready",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// Start starts the API server.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.config.Port)
	s.logger.WithField("addr", addr).Info("Starting REST API server")
	return s.echo.Start(addr)
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down REST API server...")
	return s.echo.Shutdown(ctx)
}

// Echo returns the underlying Echo instance.
func (s *Server) Echo() *echo.Echo {
	return s.echo
}
