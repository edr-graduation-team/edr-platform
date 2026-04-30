// Package main is the entry point for the connection-manager server.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	"github.com/edr-platform/connection-manager/config"
	"github.com/edr-platform/connection-manager/internal/cache"
	"github.com/edr-platform/connection-manager/internal/database"
	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/edr-platform/connection-manager/internal/service"
	"github.com/edr-platform/connection-manager/pkg/api"
	"github.com/edr-platform/connection-manager/pkg/handlers"
	"github.com/edr-platform/connection-manager/pkg/kafka"
	"github.com/edr-platform/connection-manager/pkg/metrics"
	"github.com/edr-platform/connection-manager/pkg/models"
	"github.com/edr-platform/connection-manager/pkg/playbook"
	"github.com/edr-platform/connection-manager/pkg/security"
	"github.com/edr-platform/connection-manager/pkg/server"
)

// seedDefaultAdmin creates a default admin account if one does not already exist.
// Uses INSERT ... ON CONFLICT DO NOTHING so that existing admin credentials
// (including password changes made by operators) are NEVER overwritten on reboot.
func seedDefaultAdmin(ctx context.Context, dbPool *database.PostgresPool, logger *logrus.Logger) {
	pool := dbPool.Pool()

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		logger.Errorf("Failed to hash seed password: %v", err)
		return
	}

	now := time.Now()
	adminID := uuid.New()
	tag, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, password_hash, full_name, role, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (username) DO NOTHING`,
		adminID,
		"admin",
		"admin@edr.local",
		string(hashedPassword),
		"Administrator",
		models.UserRoleAdmin,
		models.UserStatusActive,
		now, now,
	)
	if err != nil {
		logger.Errorf("Failed to seed default admin user: %v", err)
		return
	}
	if tag.RowsAffected() > 0 {
		logger.Info("🔑 Default admin account created (username: admin, password: admin) — CHANGE THIS PASSWORD!")
	} else {
		logger.Debug("Admin account already exists — skipping seed")
	}
}

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config/config.yaml", "Path to configuration file")
	flag.Parse()

	// Initialize logger
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	})
	logger.SetOutput(os.Stdout)

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}

	// Set log level
	level, err := logrus.ParseLevel(cfg.Logging.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	logger.Info("Starting Connection Manager Server...")
	logger.WithFields(logrus.Fields{
		"grpc_port": cfg.Server.GRPCPort,
		"http_port": cfg.Server.HTTPPort,
	}).Info("Configuration loaded")

	// Initialize Redis client (optional — server runs in degraded mode if unavailable)
	var redisClient *cache.RedisClient
	if c, err := cache.NewRedisClient(&cache.RedisConfig{
		Addr:         cfg.Redis.Addr,
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		PoolSize:     cfg.Redis.PoolSize,
		PoolTimeout:  cfg.Redis.PoolTimeout,
		ReadTimeout:  cfg.Redis.ReadTimeout,
		WriteTimeout: cfg.Redis.WriteTimeout,
	}, logger); err != nil {
		logger.Warnf("Redis unavailable, running in degraded mode (dedup, rate limit, agent status, and JWT blocklist disabled): %v", err)
		redisClient = nil
	} else {
		redisClient = c
		defer redisClient.Close()
	}

	// TLS configuration: skip when GRPC_INSECURE=1 or true (plaintext gRPC for debugging / Host-VM connectivity)
	var tlsConfig *tls.Config
	if grpcInsecure() {
		logger.Warn("GRPC_INSECURE is set — gRPC server will use PLAINTEXT (no TLS). Use only for debugging.")
		tlsConfig = nil
	} else {
		// ── Full PKI Bootstrap ──────────────────────────────────────────
		// Auto-generates ALL crypto material on first run:
		//   1. CA cert + key       (if missing)
		//   2. Server cert + key   (if missing or IPs changed)
		//   3. JWT signing keys    (if missing)
		// Safe to call on every startup — only generates what is missing.
		caKeyPath := filepath.Join(filepath.Dir(cfg.Server.CACertPath), "ca.key")
		if err := security.EnsureFullPKI(
			cfg.Server.CACertPath, caKeyPath,
			cfg.Server.TLSCertPath, cfg.Server.TLSKeyPath,
			cfg.JWT.PrivateKeyPath, cfg.JWT.PublicKeyPath,
			logger,
		); err != nil {
			logger.Fatalf("PKI Bootstrap failed: %v", err)
		}

		var err error
		tlsConfig, err = security.LoadServerTLSConfig(&security.TLSConfig{
			CertPath:   cfg.Server.TLSCertPath,
			KeyPath:    cfg.Server.TLSKeyPath,
			CACertPath: cfg.Server.CACertPath,
		})
		if err != nil {
			logger.Fatalf("Failed to load TLS configuration: %v", err)
		}
	}

	// Initialize JWT Manager (optional - mTLS-only if keys not available)
	var jwtManager *security.JWTManager
	if cfg.JWT.PrivateKeyPath != "" && cfg.JWT.PublicKeyPath != "" {
		jwtManager, err = security.NewJWTManager(
			cfg.JWT.PrivateKeyPath,
			cfg.JWT.PublicKeyPath,
			cfg.JWT.Issuer,
			cfg.JWT.Audience,
			cfg.JWT.AccessTTL,
			cfg.JWT.RefreshTTL,
		)
		if err != nil {
			logger.Warnf("JWT Manager initialization failed (mTLS-only mode): %v", err)
			jwtManager = nil
		} else {
			logger.Info("JWT Manager initialized - dual auth (mTLS + JWT) enabled")
		}
	} else {
		logger.Warn("JWT key paths not configured - running in mTLS-only mode")
	}

	// Initialize PostgreSQL database (optional - registration disabled without it)
	// We keep the pool reference alive beyond this block so the EventFallbackStore
	// can use it for durable event storage when Kafka is unavailable.
	ctx := context.Background()
	var agentSvc service.AgentService
	var authSvc service.AuthService
	var enrollmentTokenRepo repository.EnrollmentTokenRepository
	var agentRepo repository.AgentRepository     // hoisted for sweeper access
	var commandRepo repository.CommandRepository // hoisted for C2 injection
	var forensicRepo repository.ForensicRepository
	var quarantineRepo repository.QuarantineRepository
	var auditRepo repository.AuditLogRepository // hoisted for audit log API
	var alertRepo repository.AlertRepository    // hoisted for alert stats API
	var playbookRepo repository.ResponsePlaybookRepository
	var automationRuleRepo repository.AutomationRuleRepository
	var executionRepo repository.PlaybookExecutionRepository
	var automationMetricsRepo repository.AutomationMetricsRepository
	var dbPool *database.PostgresPool // scoped outside if-block for fallback access

	dbPoolInst, dbErr := database.NewPostgresPool(ctx, &database.PostgresConfig{
		Host:            cfg.Database.Host,
		Port:            cfg.Database.Port,
		User:            cfg.Database.User,
		Password:        cfg.Database.Password,
		Database:        cfg.Database.Name,
		SSLMode:         cfg.Database.SSLMode,
		MaxConns:        int32(cfg.Database.MaxOpenConns),
		MinConns:        int32(cfg.Database.MaxIdleConns),
		MaxConnLifetime: cfg.Database.ConnMaxLifetime,
		MaxConnIdleTime: cfg.Database.ConnMaxIdleTime,
	}, logger)
	if dbErr != nil {
		logger.Warnf("Failed to connect to PostgreSQL (agent registration disabled): %v", dbErr)
	} else {
		dbPool = dbPoolInst
		defer dbPool.Close()

		// ── Auto-migrate: apply any pending SQL migrations ──
		dbCfg := &database.PostgresConfig{
			Host:     cfg.Database.Host,
			Port:     cfg.Database.Port,
			User:     cfg.Database.User,
			Password: cfg.Database.Password,
			Database: cfg.Database.Name,
			SSLMode:  cfg.Database.SSLMode,
		}
		if err := database.RunMigrations(dbCfg, logger); err != nil {
			logger.Fatalf("Auto-migration failed: %v", err)
		}

		pool := dbPool.Pool()
		agentRepo = repository.NewPostgresAgentRepository(pool)
		tokenRepo := repository.NewPostgresInstallationTokenRepository(pool)
		enrollmentTokenRepo = repository.NewPostgresEnrollmentTokenRepository(pool)
		auditRepo = repository.NewPostgresAuditLogRepository(pool)
		certRepo := repository.NewPostgresCertificateRepository(pool)
		commandRepo = repository.NewPostgresCommandRepository(pool)
		forensicRepo = repository.NewPostgresForensicRepository(pool)
		alertRepo = repository.NewPostgresAlertRepository(pool)
		quarantineRepo = repository.NewPostgresQuarantineRepository(pool)
		playbookRepo = repository.NewPostgresResponsePlaybookRepository(pool)
		automationRuleRepo = repository.NewPostgresAutomationRuleRepository(pool)
		executionRepo = repository.NewPostgresPlaybookExecutionRepository(pool)
		automationMetricsRepo = repository.NewPostgresAutomationMetricsRepository(pool)

		// CA paths for signing agent certificates (ca.key next to ca.crt)
		caCertPath := cfg.Server.CACertPath
		caKeyPath := filepath.Join(filepath.Dir(caCertPath), "ca.key")

		certSvc := service.NewCertificateService(
			certRepo, agentRepo, auditRepo, redisClient, logger,
			caCertPath, caKeyPath,
		)

		// Create agent service (with cert service for auto-issuance on Register)
		agentSvc = service.NewAgentService(agentRepo, tokenRepo, enrollmentTokenRepo, auditRepo, redisClient, logger, certSvc)

		// Create user repository and auth service for dashboard login
		userRepo := repository.NewPostgresUserRepository(pool)
		authSvc = service.NewAuthService(userRepo, auditRepo, jwtManager, redisClient, logger)

		logger.Info("Database connected - agent registration enabled")

		// Seed default admin user if no users exist
		seedDefaultAdmin(ctx, dbPool, logger)
	}

	// =========================================================================
	// PHASE 1: Initialize Kafka Producer, Metrics, and Handlers
	// These are the components that were missing from the original code,
	// causing StreamEvents and Heartbeat RPCs to be dead (no-op stubs).
	// =========================================================================

	// Initialize Prometheus metrics for observability
	edrMetrics := metrics.NewMetrics("edr")
	logger.Info("Prometheus metrics initialized")

	// Initialize Kafka producer (graceful: skip if disabled or unreachable)
	// When Kafka is unavailable, EventHandler will fall back to PostgreSQL
	// storage so no telemetry data is lost.
	var kafkaProducer *kafka.EventProducer
	if cfg.Kafka.Enabled {
		producerCfg := &kafka.ProducerConfig{
			Brokers:     cfg.Kafka.Brokers,
			Topic:       cfg.Kafka.Topic,
			DLQTopic:    cfg.Kafka.DLQTopic,
			Compression: cfg.Kafka.Compression,
			Acks:        cfg.Kafka.Acks,
			MaxRetries:  cfg.Kafka.MaxRetries,
			BatchSize:   cfg.Kafka.BatchSize,
			Timeout:     cfg.Kafka.Timeout,
		}

		var kafkaErr error
		kafkaProducer, kafkaErr = kafka.NewEventProducer(producerCfg, edrMetrics, logger)
		if kafkaErr != nil {
			// Kafka failure is NOT fatal — EventHandler can fall back to DB.
			// This is a deliberate design choice: an EDR system must never
			// refuse telemetry just because Kafka is temporarily down.
			logger.Warnf("Kafka producer init failed (events will fall back to DB): %v", kafkaErr)
			kafkaProducer = nil
		} else {
			defer kafkaProducer.Close()
			logger.Info("Kafka producer connected")
		}
	} else {
		logger.Warn("Kafka disabled in config — events will be stored via DB fallback")
	}

	// Initialize rate limiter for event ingestion
	// 1000 events/sec with 2x burst allows healthy agents to send batches
	// without being throttled, while protecting the server from floods.
	rateLimiter := cache.NewRateLimiter(redisClient, 1000, 2.0)

	// Create EventHandler — the core telemetry pipeline.
	// Depends on: Redis (dedup, agent-status), RateLimiter, Kafka (primary), Metrics.
	evtHandler := handlers.NewEventHandler(logger, redisClient, rateLimiter, kafkaProducer, edrMetrics)
	var fallbackStore *handlers.EventFallbackStore

	// Wire up the PostgreSQL fallback store for data durability.
	// When Kafka is unavailable, events are persisted to PostgreSQL so they
	// can be replayed later. This completes the 3-tier delivery guarantee:
	// Kafka primary → Kafka DLQ → PostgreSQL fallback.
	if dbPool != nil {
		fallback := handlers.NewEventFallbackStore(dbPool.Pool(), edrMetrics, logger)
		if fallback != nil {
			// Auto-create the fallback table if it doesn't exist.
			// This is safe to call on every startup (CREATE IF NOT EXISTS).
			if err := fallback.EnsureTable(ctx); err != nil {
				logger.Warnf("Failed to create fallback table (DB fallback disabled): %v", err)
			} else {
				fallbackStore = fallback
				evtHandler.SetFallbackStore(fallback)
				defer fallback.Close() // drain async writer workers on shutdown
				logger.Info("Event DB fallback store enabled (async workers started)")

				// Start the replay worker that re-publishes unreplayed events
				// from the fallback table back to Kafka when connectivity is restored.
				replayWorker := handlers.NewFallbackReplayWorker(dbPool.Pool(), kafkaProducer, logger)
				if replayWorker != nil {
					replayCtx, replayCancel := context.WithCancel(context.Background())
					defer replayCancel()
					go replayWorker.Start(replayCtx)
				}
			}
		}
	} else {
		logger.Warn("No DB available — event DB fallback disabled (data loss risk if Kafka is also down)")
	}
	logger.Info("EventHandler created")

	// Create HeartbeatHandler — agent health reporting.
	// Depends on: Redis (real-time status), AgentService (DB persistence).
	// agentSvc may be nil if DB is unavailable — HeartbeatHandler handles this gracefully.
	hbHandler := handlers.NewHeartbeatHandler(logger, redisClient, agentSvc)
	logger.Info("HeartbeatHandler created")

	// Create gRPC server with ALL handlers injected
	grpcServer, err := server.NewServer(cfg, logger, redisClient, tlsConfig, jwtManager, agentSvc, evtHandler, hbHandler)
	if err != nil {
		logger.Fatalf("Failed to create gRPC server: %v", err)
	}
	if forensicRepo != nil {
		grpcServer.SetForensicRepo(forensicRepo)
	}

	// Build REST API config (dashboard /api/v1): use API config with HTTP port so one server serves both.
	apiCfg := cfg.API
	apiCfg.Port = cfg.Server.HTTPPort
	restAPIServer := api.NewServer(&apiCfg, logger, edrMetrics)
	// Prometheus metrics on same server (lifecycle managed with REST API).
	restAPIServer.Echo().GET(cfg.Monitoring.MetricsPath, echo.WrapHandler(promhttp.Handler()))

	var automationHandlers *api.AutomationHandlers
	if dbPool != nil {
		metricsService := service.NewMetricsService(logger, automationMetricsRepo)
		commandService := service.NewCommandService(logger, commandRepo, executionRepo, automationMetricsRepo)
		mlOptimizer := service.NewMLOptimizer(logger, automationMetricsRepo, automationRuleRepo)
		notificationService := service.NewNotificationService(logger)
		automationService := service.NewAutomationService(logger, alertRepo, playbookRepo, automationRuleRepo, executionRepo, commandService, notificationService, metricsService, mlOptimizer)
		automationHandlers = api.NewAutomationHandlers(logger, automationService, metricsService)
	}

	apiHandlers := api.NewHandlers(logger, jwtManager, redisClient, rateLimiter, agentSvc, authSvc, cfg.Server.CACertPath, enrollmentTokenRepo, automationHandlers)
	// Wire fallback store stats into REST API reliability endpoint.
	// Safe even if nil (endpoint returns enabled=false + reason).
	apiHandlers.SetFallbackStore(fallbackStore)

	// Wire the gRPC server's AgentRegistry into REST API handlers for C2 command routing.
	// Without this, POST /agents/:id/commands returns 503 (registry == nil).
	apiHandlers.SetRegistry(grpcServer.GetRegistry())

	// Wire the gRPC address so isolate commands automatically inject server_address
	// into their parameters. The agent needs this to build correct ALLOW firewall rules.
	// NOTE: The address here must match what the agent uses to connect.
	//       Env var C2_GRPC_ADDRESS overrides the default "host:port" construct.
	c2GRPCAddress := os.Getenv("C2_GRPC_ADDRESS")
	if c2GRPCAddress == "" {
		c2GRPCAddress = fmt.Sprintf("localhost:%d", cfg.Server.GRPCPort)
	}
	apiHandlers.SetGRPCAddress(c2GRPCAddress)
	logger.Infof("[C2] Isolation server_address configured as %s", c2GRPCAddress)

	// Wire CommandRepository into REST handlers and gRPC server for C2 persistence.
	// Also wire into EventHandler for pending command re-delivery on agent reconnect.
	if commandRepo != nil {
		apiHandlers.SetCommandRepo(commandRepo)
		grpcServer.SetCommandRepo(commandRepo)
		evtHandler.SetCommandRepo(commandRepo) // enables re-delivery on reconnect
		logger.Info("C2 command persistence enabled (commands table + pending re-delivery)")
	}
	if quarantineRepo != nil {
		evtHandler.SetQuarantineRepo(quarantineRepo)
		apiHandlers.SetQuarantineRepo(quarantineRepo)
		grpcServer.SetQuarantineRepo(quarantineRepo)
		logger.Info("Quarantine inventory API enabled (agent_quarantine_items)")
	}

	// Wire AuditLogRepository into REST handlers for the Audit Logs page.
	if auditRepo != nil {
		apiHandlers.SetAuditRepo(auditRepo)
		logger.Info("Audit log querying enabled")
	}

	// Wire AlertRepository into REST handlers for Alert Stats and querying.
	if alertRepo != nil {
		apiHandlers.SetAlertRepo(alertRepo)
		logger.Info("Alert querying and stats enabled")
	}

	// Wire UserRepository + RoleRepository into REST handlers for RBAC.
	if dbPool != nil {
		pool := dbPool.Pool()
		apiHandlers.SetUserRepo(repository.NewPostgresUserRepository(pool))
		apiHandlers.SetRoleRepo(repository.NewPostgresRoleRepository(pool))
		apiHandlers.SetContextPolicyRepo(repository.NewPostgresContextPolicyRepository(pool))
		apiHandlers.SetAgentPackageRepo(repository.NewPostgresAgentPackageRepository(pool))
		apiHandlers.SetAgentPatchProfileRepo(repository.NewPostgresAgentPatchProfileRepository(pool))
		// Event storage/search for dashboard investigations
		eventRepo := repository.NewPostgresEventRepository(pool)
		apiHandlers.SetEventRepo(eventRepo)
		evtHandler.SetEventRepo(eventRepo)
		apiHandlers.SetForensicRepo(forensicRepo)
		logger.Info("User management and RBAC enabled")

		// ── Post-Isolation Pipeline ──────────────────────────────────────
		// Wire incident repository + playbook engine.
		incidentRepo := repository.NewPostgresIncidentRepository(pool)
		apiHandlers.SetIncidentRepo(incidentRepo)
		grpcServer.SetIncidentRepo(incidentRepo)

		vulnRepo := repository.NewPostgresVulnerabilityRepository(pool)
		apiHandlers.SetVulnRepo(vulnRepo)
		apiHandlers.SetVulnScannerIngest(service.NewVulnScannerIngestService(logger))
		evtHandler.SetVulnerabilityRepo(vulnRepo)
		logger.Info("Vulnerability findings API enabled (vulnerability_findings)")

		// CISA KEV catalog sync — runs once at startup, then daily.
		kevSync := service.NewKEVSyncService(vulnRepo, logger)
		apiHandlers.SetKEVSync(kevSync)
		go kevSync.Run(context.Background())
		logger.Info("CISA KEV sync service enabled (24h interval)")

		apiHandlers.SetSiemRepo(repository.NewPostgresSiemConnectorRepository(pool))
		logger.Info("SIEM connectors API enabled (siem_connectors)")

		if commandRepo != nil {
			pb := playbook.NewEngine(logger, incidentRepo, commandRepo, grpcServer.GetRegistry())
			grpcServer.SetPlaybookEngine(pb)
			logger.Info("Post-isolation playbook engine enabled")
		}
	}

	restAPIServer.RegisterRoutes(apiHandlers)

	// Start REST API server (healthz, readyz, /api/v1/*, metrics) on HTTP port
	go func() {
		if err := restAPIServer.Start(); err != nil && err != http.ErrServerClosed {
			logger.Errorf("REST API server error: %v", err)
		}
	}()

	// =========================================================================
	// STALE AGENT SWEEPER: mark agents offline if last_seen > 5 minutes
	// =========================================================================
	sweepCtx, sweepCancel := context.WithCancel(context.Background())
	defer sweepCancel()
	if agentRepo != nil {
		go staleAgentSweeper(sweepCtx, agentRepo, logger)
		logger.Info("Stale agent sweeper started (interval: 15s, threshold: 1m)")
	}

	// Start gRPC server in goroutine
	go func() {
		if err := grpcServer.Start(); err != nil {
			logger.Fatalf("gRPC server failed: %v", err)
		}
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("Shutdown signal received, initiating graceful shutdown...")

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	// Shutdown REST API (HTTP) server first
	if err := restAPIServer.Shutdown(shutdownCtx); err != nil {
		logger.Warnf("REST API server shutdown error: %v", err)
	}

	// Shutdown gRPC server
	if err := grpcServer.Shutdown(shutdownCtx); err != nil {
		logger.Warnf("gRPC server shutdown error: %v", err)
	}

	logger.Info("Server stopped")
}

// startHTTPServer starts the HTTP server for health checks and metrics.
func startHTTPServer(cfg *config.Config, logger *logrus.Logger, redis *cache.RedisClient) *http.Server {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc(cfg.Monitoring.HealthPath, func(w http.ResponseWriter, r *http.Request) {
		health := checkHealth(r.Context(), redis)

		status := http.StatusOK
		if health.Status == "unhealthy" {
			status = http.StatusServiceUnavailable
		} else if health.Status == "degraded" {
			status = http.StatusOK // Still return 200 for degraded
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write([]byte(health.ToJSON()))
	})

	// Prometheus metrics endpoint
	if cfg.Monitoring.Enabled {
		mux.Handle(cfg.Monitoring.MetricsPath, promhttp.Handler())
	}

	// Ready endpoint (for K8s readiness probe)
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		// Check if all dependencies are ready
		health := checkHealth(r.Context(), redis)
		if health.Status == "unhealthy" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ready"))
	})

	addr := ":" + strconv.Itoa(cfg.Server.HTTPPort)

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		logger.Infof("HTTP server listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Errorf("HTTP server error: %v", err)
		}
	}()

	return server
}

// HealthStatus represents the health check response.
type HealthStatus struct {
	Status     string            `json:"status"`
	Components map[string]string `json:"components"`
	Timestamp  string            `json:"timestamp"`
}

// ToJSON converts HealthStatus to a JSON string.
func (h *HealthStatus) ToJSON() string {
	data, err := json.Marshal(h)
	if err != nil {
		return `{"status":"error","message":"failed to serialize health status"}`
	}
	return string(data)
}

// checkHealth checks the health of all components.
func checkHealth(ctx context.Context, redis *cache.RedisClient) *HealthStatus {
	health := &HealthStatus{
		Status:     "healthy",
		Components: make(map[string]string),
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	// Check Redis (nil when running in degraded mode)
	if redis == nil {
		health.Components["redis"] = "unavailable (degraded mode)"
		health.Status = "degraded"
	} else if err := redis.Client().Ping(ctx).Err(); err != nil {
		health.Components["redis"] = "unhealthy"
		health.Status = "degraded"
	} else {
		health.Components["redis"] = "ok"
	}

	// TODO: Check database connection
	health.Components["database"] = "ok"

	// gRPC server is always ok if we're responding
	health.Components["grpc"] = "ok"

	return health
}

// grpcInsecure returns true when GRPC_INSECURE env is set (plaintext gRPC for debugging).
func grpcInsecure() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("GRPC_INSECURE")))
	return v == "1" || v == "true"
}

// staleAgentSweeper runs a periodic sweep to mark zombie agents as offline.
// An agent is considered stale if its last_seen timestamp is older than 1 minute.
// Sweep interval: 15s — ensures offline detection within ~75 seconds.
func staleAgentSweeper(ctx context.Context, repo repository.AgentRepository, logger *logrus.Logger) {
	const threshold = 1 * time.Minute
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Stale agent sweeper stopped")
			return
		case <-ticker.C:
			sweepCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			affected, err := repo.MarkStaleOffline(sweepCtx, threshold)
			cancel()
			if err != nil {
				logger.Errorf("Stale agent sweep failed: %v", err)
			} else if affected > 0 {
				logger.Infof("Stale agent sweep: marked %d zombie agents as offline", affected)
			}
		}
	}
}
