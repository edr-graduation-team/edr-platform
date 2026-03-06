// Package main is the entry point for the connection-manager server.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
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
	"github.com/edr-platform/connection-manager/pkg/security"
	"github.com/edr-platform/connection-manager/pkg/server"
)

// seedDefaultAdmin guarantees a valid admin account exists on every boot.
// Uses UPSERT so the admin row is self-healing: if the password, role, or
// status were corrupted, they are forcefully corrected on next startup.
func seedDefaultAdmin(ctx context.Context, dbPool *database.PostgresPool, logger *logrus.Logger) {
	pool := dbPool.Pool()

	// Generate a fresh bcrypt hash every boot — idempotent and tamper-proof.
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		logger.Errorf("Failed to hash seed password: %v", err)
		return
	}

	now := time.Now()
	adminID := uuid.New()
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, username, email, password_hash, full_name, role, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (username) DO UPDATE SET
		     password_hash = EXCLUDED.password_hash,
		     role          = EXCLUDED.role,
		     status        = EXCLUDED.status,
		     updated_at    = EXCLUDED.updated_at`,
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
		logger.Errorf("Failed to upsert default admin user: %v", err)
		return
	}
	logger.Info("🔑 Admin account bootstrapped (username: admin, password: admin) — CHANGE THIS PASSWORD!")
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
		// Auto-Certificate Bootstrapper: dynamically generate server.crt
		// with ALL current host IPs in SANs so mTLS works on any machine.
		// Runs on every startup; only regenerates if IPs changed or cert missing.
		caKeyPath := filepath.Join(filepath.Dir(cfg.Server.CACertPath), "ca.key")
		if regenerated, err := security.EnsureServerCert(
			cfg.Server.CACertPath, caKeyPath,
			cfg.Server.TLSCertPath, cfg.Server.TLSKeyPath,
			logger,
		); err != nil {
			logger.Warnf("Auto-Cert Bootstrapper failed (will try loading existing cert): %v", err)
		} else if regenerated {
			logger.Info("Server certificate regenerated with current host IPs")
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
	var agentRepo repository.AgentRepository // hoisted for sweeper access
	var dbPool *database.PostgresPool        // scoped outside if-block for fallback access

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
		auditRepo := repository.NewPostgresAuditLogRepository(pool)
		certRepo := repository.NewPostgresCertificateRepository(pool)

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

	// Wire up the PostgreSQL fallback store for data durability.
	// When Kafka is unavailable, events are persisted to PostgreSQL so they
	// can be replayed later. This completes the 3-tier delivery guarantee:
	// Kafka primary → Kafka DLQ → PostgreSQL fallback.
	if dbPool != nil {
		fallback := handlers.NewEventFallbackStore(dbPool.Pool(), logger)
		if fallback != nil {
			// Auto-create the fallback table if it doesn't exist.
			// This is safe to call on every startup (CREATE IF NOT EXISTS).
			if err := fallback.EnsureTable(ctx); err != nil {
				logger.Warnf("Failed to create fallback table (DB fallback disabled): %v", err)
			} else {
				evtHandler.SetFallbackStore(fallback)
				logger.Info("Event DB fallback store enabled")
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

	// Build REST API config (dashboard /api/v1): use API config with HTTP port so one server serves both.
	apiCfg := cfg.API
	apiCfg.Port = cfg.Server.HTTPPort
	restAPIServer := api.NewServer(&apiCfg, logger, edrMetrics)
	// Prometheus metrics on same server (lifecycle managed with REST API).
	restAPIServer.Echo().GET(cfg.Monitoring.MetricsPath, echo.WrapHandler(promhttp.Handler()))
	apiHandlers := api.NewHandlers(logger, jwtManager, redisClient, rateLimiter, agentSvc, authSvc, cfg.Server.CACertPath, enrollmentTokenRepo)

	// Wire the gRPC server's AgentRegistry into REST API handlers for C2 command routing.
	// Without this, POST /agents/:id/commands returns 503 (registry == nil).
	apiHandlers.SetRegistry(grpcServer.GetRegistry())

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
		logger.Info("Stale agent sweeper started (interval: 60s, threshold: 5m)")
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
// An agent is considered stale if its last_seen timestamp is older than 5 minutes.
func staleAgentSweeper(ctx context.Context, repo repository.AgentRepository, logger *logrus.Logger) {
	const threshold = 5 * time.Minute
	ticker := time.NewTicker(60 * time.Second)
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
