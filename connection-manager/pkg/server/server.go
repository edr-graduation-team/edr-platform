// Package server provides the gRPC server implementation.
package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/edr-platform/connection-manager/config"
	"github.com/edr-platform/connection-manager/internal/cache"
	"github.com/edr-platform/connection-manager/internal/service"
	"github.com/edr-platform/connection-manager/pkg/handlers"
	"github.com/edr-platform/connection-manager/pkg/security"
	edrv1 "github.com/edr-platform/connection-manager/proto/v1"
)

// Server represents the gRPC server.
// It explicitly implements all RPCs from EventIngestionServiceServer.
// Handlers are injected via NewServer — nil handlers degrade gracefully
// rather than silently discarding data (which the old stub behavior did).
type Server struct {
	edrv1.UnimplementedEventIngestionServiceServer

	cfg              *config.Config
	grpcServer       *grpc.Server
	logger           *logrus.Logger
	redis            *cache.RedisClient
	agentService     service.AgentService
	eventHandler     *handlers.EventHandler
	heartbeatHandler *handlers.HeartbeatHandler
	registry         *handlers.AgentRegistry
}

// NewServer creates a new gRPC server with all handler dependencies injected.
// Every handler is optional — the server will log warnings and return proper
// gRPC error codes when a handler is nil, rather than silently succeeding
// with no-op stubs (which caused total data loss in the previous design).
func NewServer(
	cfg *config.Config,
	logger *logrus.Logger,
	redis *cache.RedisClient,
	tlsConfig *tls.Config,
	jwtManager *security.JWTManager,
	agentSvc service.AgentService,
	evtHandler *handlers.EventHandler,
	hbHandler *handlers.HeartbeatHandler,
) (*Server, error) {
	opts := []grpc.ServerOption{}

	// Credentials: TLS when tlsConfig is set, otherwise plaintext (for GRPC_INSECURE / debugging)
	if tlsConfig != nil {
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
	} else {
		logger.Warn("gRPC server running in PLAINTEXT (no TLS)")
	}

	// Keepalive: use config; avoid aggressive MinTime on Windows (use 30s if not set)
	kaTime := cfg.Server.KeepaliveTime
	if kaTime <= 0 {
		kaTime = 30 * time.Second
	}
	kaTimeout := cfg.Server.KeepaliveTimeout
	if kaTimeout <= 0 {
		kaTimeout = 10 * time.Second
	}
	kaParams := keepalive.ServerParameters{
		Time:    kaTime,
		Timeout: kaTimeout,
	}
	kaPolicy := keepalive.EnforcementPolicy{
		MinTime:             30 * time.Second, // Relaxed for Windows; was 5s
		PermitWithoutStream: true,
	}
	opts = append(opts,
		grpc.KeepaliveParams(kaParams),
		grpc.KeepaliveEnforcementPolicy(kaPolicy),
		grpc.MaxConcurrentStreams(cfg.Server.MaxConcurrentStreams),
	)

	// Add interceptors (middleware)
	interceptor := NewInterceptor(cfg, logger, redis, jwtManager)
	opts = append(opts,
		grpc.ChainUnaryInterceptor(
			interceptor.LoggingUnaryInterceptor,
			interceptor.AuthUnaryInterceptor,
			interceptor.RateLimitUnaryInterceptor,
		),
		grpc.ChainStreamInterceptor(
			interceptor.LoggingStreamInterceptor,
			interceptor.AuthStreamInterceptor,
			interceptor.RateLimitStreamInterceptor,
		),
	)

	grpcServer := grpc.NewServer(opts...)

	s := &Server{
		cfg:              cfg,
		grpcServer:       grpcServer,
		logger:           logger,
		redis:            redis,
		agentService:     agentSvc,
		eventHandler:     evtHandler,
		heartbeatHandler: hbHandler,
	}

	// Create and wire the AgentRegistry for real-time presence and C2
	registry := handlers.NewAgentRegistry(logger)
	s.registry = registry
	if evtHandler != nil {
		evtHandler.SetAgentRegistry(registry)
		evtHandler.SetAgentService(agentSvc)
	}

	// Log handler availability at startup so operators know which RPCs are live
	if evtHandler != nil {
		logger.Info("StreamEvents RPC: ENABLED (EventHandler injected)")
	} else {
		logger.Warn("StreamEvents RPC: DISABLED (no EventHandler — agents will receive Unavailable)")
	}
	if hbHandler != nil {
		logger.Info("Heartbeat RPC: ENABLED (HeartbeatHandler injected)")
	} else {
		logger.Warn("Heartbeat RPC: DISABLED (no HeartbeatHandler — agents will receive Unavailable)")
	}
	logger.Info("AgentRegistry: ENABLED (real-time presence & command routing)")
	logger.Info("SendCommandResult RPC: ENABLED")

	// Register services
	edrv1.RegisterEventIngestionServiceServer(grpcServer, s)

	return s, nil
}

// Start starts the gRPC server.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.cfg.Server.GRPCPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s.logger.Infof("gRPC server listening on %s", addr)
	return s.grpcServer.Serve(lis)
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Initiating graceful shutdown...")

	// Create channel to signal shutdown completion
	done := make(chan struct{})

	go func() {
		s.grpcServer.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("gRPC server stopped gracefully")
		return nil
	case <-ctx.Done():
		s.logger.Warn("Graceful shutdown timed out, forcing stop")
		s.grpcServer.Stop()
		return ctx.Err()
	}
}

// GetGRPCServer returns the underlying gRPC server.
func (s *Server) GetGRPCServer() *grpc.Server {
	return s.grpcServer
}

// ============================================================================
// RPC IMPLEMENTATIONS
// Each method delegates to the injected handler. A nil handler returns
// codes.Unavailable — this is the correct gRPC semantics because it tells
// the client "this service exists but the server can't serve it right now",
// which is exactly what happens when a dependency (Kafka, DB) is missing.
// This replaces the old UnimplementedEventIngestionServiceServer stubs that
// returned nil, which agents interpreted as success (silently losing data).
// ============================================================================

// StreamEvents implements bidirectional streaming for event telemetry.
// This is the primary data pipeline: Agent → gRPC stream → EventHandler → Kafka.
// If the EventHandler is nil (e.g., Kafka/DB not configured), we return
// codes.Unavailable so the agent knows to retry with backoff.
func (s *Server) StreamEvents(stream edrv1.EventIngestionService_StreamEventsServer) error {
	if s.eventHandler == nil {
		s.logger.Warn("StreamEvents called but EventHandler is not configured")
		return status.Error(codes.Unavailable, "event ingestion is not available")
	}
	return s.eventHandler.StreamEvents(stream)
}

// Heartbeat implements the unary heartbeat RPC.
// Agents send periodic health reports; the handler persists metrics to both
// Redis (for real-time dashboards) and PostgreSQL (source of truth).
// Returning codes.Unavailable on nil handler tells the agent that the
// server is temporarily unable to process heartbeats.
func (s *Server) Heartbeat(ctx context.Context, req *edrv1.HeartbeatRequest) (*edrv1.HeartbeatResponse, error) {
	if s.heartbeatHandler == nil {
		s.logger.Warn("Heartbeat called but HeartbeatHandler is not configured")
		return nil, status.Error(codes.Unavailable, "heartbeat service is not available")
	}
	return s.heartbeatHandler.Heartbeat(ctx, req)
}

// RegisterAgent implements the gRPC RegisterAgent RPC with database persistence.
func (s *Server) RegisterAgent(ctx context.Context, req *edrv1.AgentRegistrationRequest) (*edrv1.AgentRegistrationResponse, error) {
	if s.agentService == nil {
		s.logger.Warn("RegisterAgent called but AgentService is not configured")
		return &edrv1.AgentRegistrationResponse{
			Status:  edrv1.RegistrationStatus_REGISTRATION_STATUS_REJECTED,
			Message: "Agent registration is not available (database not configured)",
		}, nil
	}

	// Map gRPC request to service request
	svcReq := &service.RegisterAgentRequest{
		InstallationToken: req.InstallationToken,
		Hostname:          req.Hostname,
		OSType:            req.OsType,
		OSVersion:         req.OsVersion,
		CPUCount:          int(req.CpuCount),
		MemoryMB:          req.MemoryMb,
		AgentVersion:      req.AgentVersion,
		CSRData:           req.Csr,
		IPAddresses:       req.IpAddresses,
		Tags:              req.Tags,
	}

	svcResp, err := s.agentService.Register(ctx, svcReq)
	if err != nil {
		s.logger.WithError(err).Warn("Agent registration failed")
		return &edrv1.AgentRegistrationResponse{
			Status:  edrv1.RegistrationStatus_REGISTRATION_STATUS_REJECTED,
			Message: err.Error(),
		}, nil
	}

	status := edrv1.RegistrationStatus_REGISTRATION_STATUS_PENDING
	message := "Agent registration pending admin approval"
	if svcResp.Status == "approved" {
		status = edrv1.RegistrationStatus_REGISTRATION_STATUS_APPROVED
		message = "Agent registered and certificate issued"
	}

	return &edrv1.AgentRegistrationResponse{
		AgentId:     svcResp.AgentID.String(),
		Status:      status,
		Message:     message,
		Certificate: svcResp.Certificate,
		CaChain:     svcResp.CACert,
		AccessToken: svcResp.AccessToken,
	}, nil
}

// SendCommandResult receives the execution result of a command from the agent.
// This closes the C2 feedback loop: Dashboard → Server → Agent → Execute → Result → Server.
func (s *Server) SendCommandResult(ctx context.Context, res *edrv1.CommandResult) (*emptypb.Empty, error) {
	if res == nil {
		return &emptypb.Empty{}, nil
	}

	s.logger.WithFields(logrus.Fields{
		"command_id": res.CommandId,
		"agent_id":   res.AgentId,
		"status":     res.Status,
		"output":     res.Output,
		"error":      res.Error,
	}).Info("Command result received from agent")

	// TODO: Persist to database (commands table) and notify dashboard via WebSocket
	return &emptypb.Empty{}, nil
}

// GetRegistry returns the server's AgentRegistry for use by the REST API.
func (s *Server) GetRegistry() *handlers.AgentRegistry {
	return s.registry
}
