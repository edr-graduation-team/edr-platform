// Package server provides the gRPC server implementation.
package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/edr-platform/connection-manager/config"
	"github.com/edr-platform/connection-manager/internal/cache"
	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/edr-platform/connection-manager/internal/service"
	"github.com/edr-platform/connection-manager/pkg/handlers"
	"github.com/edr-platform/connection-manager/pkg/models"
	"github.com/edr-platform/connection-manager/pkg/security"
	edrv1 "github.com/edr-platform/connection-manager/proto/v1"
)

type forensicBundle struct {
	Version   int `json:"version"`
	CommandID string `json:"command_id"`
	AgentID   string `json:"agent_id"`
	TimeRange string `json:"time_range,omitempty"`
	LogTypes  string `json:"log_types,omitempty"`
	Summary   map[string]any `json:"summary,omitempty"`
	Events    []forensicBundleEvent `json:"events,omitempty"`
}

type forensicBundleEvent struct {
	Timestamp string          `json:"timestamp,omitempty"`
	LogType   string          `json:"log_type"`
	EventID   string          `json:"event_id,omitempty"`
	Level     string          `json:"level,omitempty"`
	Provider  string          `json:"provider,omitempty"`
	Message   string          `json:"message,omitempty"`
	Raw       json.RawMessage `json:"raw,omitempty"`
}

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
	commandRepo      repository.CommandRepository
	quarantineRepo   repository.QuarantineRepository
	forensicRepo     repository.ForensicRepository
}

// SetCommandRepo injects the command repository for C2 result persistence.
func (s *Server) SetCommandRepo(repo repository.CommandRepository) {
	s.commandRepo = repo
}

// SetQuarantineRepo injects quarantine inventory persistence (optional).
func (s *Server) SetQuarantineRepo(repo repository.QuarantineRepository) {
	s.quarantineRepo = repo
}

func (s *Server) SetForensicRepo(repo repository.ForensicRepository) {
	s.forensicRepo = repo
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
		// Match the 10MB payload limit in validateBatch(). The extra 1MB
		// covers Protobuf envelope overhead (batch_id, metadata, etc.).
		grpc.MaxRecvMsgSize(11*1024*1024),
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

	// Persist result to commands table
	if s.commandRepo != nil {
		// Map agent status to DB status (agent sends UPPERCASE: "SUCCESS", "FAILED")
		dbStatus := models.CommandStatusCompleted
		agentStatus := strings.ToLower(res.Status)
		if agentStatus == "failed" || agentStatus == "error" {
			dbStatus = models.CommandStatusFailed
		} else if agentStatus == "timeout" {
			dbStatus = models.CommandStatusTimeout
		}

		result := map[string]any{
			"output": res.Output,
		}
		if cmdID, err := uuid.Parse(res.CommandId); err == nil {
			if err := s.commandRepo.UpdateStatus(ctx, cmdID, dbStatus, result, res.Error); err != nil {
				s.logger.WithError(err).Warn("Failed to persist command result to DB")
			} else {
				s.logger.Infof("Command %s result persisted: status=%s", res.CommandId, dbStatus)
			}
		}
	}

	// Persist forensic bundles (collect_logs / collect_forensics) if present in output JSON.
	if s.forensicRepo != nil && s.commandRepo != nil {
		s.persistForensicBundleBestEffort(ctx, res)
	}

	// ── Status side-effects for successful commands ────────────────────────────
	// Look up the command from DB (to get command_type) and apply side-effects:
	//   - isolate_network   → set is_isolated=true
	//   - restore_network   → set is_isolated=false
	//   - stop_agent        → set status='suspended' (distinguishes from offline!)
	if s.commandRepo != nil && strings.ToLower(res.Status) == "success" {
		if cmdID, err := uuid.Parse(res.CommandId); err == nil {
			if cmd, err := s.commandRepo.GetByID(ctx, cmdID); err == nil {
				cmdType := strings.ToLower(string(cmd.CommandType))
				agentID, _ := uuid.Parse(res.AgentId)
				switch {
				case cmdType == "isolate_network":
					s.updateAgentIsolation(ctx, agentID, true)
					s.logger.Infof("[Isolation] Agent %s is now ISOLATED", agentID)
				case cmdType == "restore_network" || cmdType == "unisolate_network":
					s.updateAgentIsolation(ctx, agentID, false)
					s.logger.Infof("[Isolation] Agent %s isolation RESTORED", agentID)
				case cmdType == "stop_agent" || cmdType == "stop_service":
					// Mark suspended so frontend shows 'Start Agent' enabled.
					// The stream-close defer checks current status and skips
					// the 'offline' overwrite when already 'suspended'.
					s.updateAgentStatus(ctx, agentID, models.AgentStatusSuspended)
					s.logger.Infof("[Control] Agent %s marked SUSPENDED after stop_agent ACK", agentID)
				case cmdType == "quarantine_file" || cmdType == "restore_quarantine_file" || cmdType == "delete_quarantine_file":
					s.applyQuarantineInventoryOnSuccess(ctx, res, cmd, agentID)
				case cmdType == "uninstall_agent":
					// Final confirmation: agent has cleaned itself up and is about to exit.
					// Latch the status so no further commands are ever dispatched.
					s.updateAgentStatus(ctx, agentID, models.AgentStatusUninstalled)
					s.logger.Infof("[Uninstall] Agent %s confirmed uninstall (command %s) — status=uninstalled", agentID, res.CommandId)
				}
			}
		}
	}

	return &emptypb.Empty{}, nil
}

func (s *Server) persistForensicBundleBestEffort(ctx context.Context, res *edrv1.CommandResult) {
	cmdID, err := uuid.Parse(res.CommandId)
	if err != nil {
		return
	}
	cmd, err := s.commandRepo.GetByID(ctx, cmdID)
	if err != nil {
		return
	}
	cmdType := strings.ToLower(string(cmd.CommandType))
	if cmdType != "collect_logs" && cmdType != "collect_forensics" {
		return
	}

	// Output may either be raw string or JSON; only parse when it looks like JSON.
	out := strings.TrimSpace(res.Output)
	if out == "" || !(strings.HasPrefix(out, "{") && strings.HasSuffix(out, "}")) {
		return
	}

	var bundle forensicBundle
	if err := json.Unmarshal([]byte(out), &bundle); err != nil || bundle.Version != 1 {
		return
	}

	agentID, err := uuid.Parse(res.AgentId)
	if err != nil {
		return
	}

	issuedAt := cmd.IssuedAt
	completedAt := cmd.CompletedAt

	sum := bundle.Summary
	if sum == nil {
		sum = map[string]any{}
	}

	_ = s.forensicRepo.UpsertCollection(ctx, repository.ForensicCollectionSummary{
		CommandID:   cmdID,
		AgentID:     agentID,
		CommandType: cmdType,
		IssuedAt:    issuedAt,
		CompletedAt: completedAt,
		TimeRange:   bundle.TimeRange,
		LogTypes:    bundle.LogTypes,
		Summary:     sum,
	})

	// Replace events per log_type (simple MVP).
	byType := map[string][]repository.ForensicEventRow{}
	for _, e := range bundle.Events {
		r := repository.ForensicEventRow{
			LogType:  strings.ToLower(strings.TrimSpace(e.LogType)),
			EventID:  e.EventID,
			Level:    e.Level,
			Provider: e.Provider,
			Message:  e.Message,
			Raw:      e.Raw,
		}
		if r.LogType == "" {
			continue
		}
		if e.Timestamp != "" {
			if ts, err := time.Parse(time.RFC3339Nano, e.Timestamp); err == nil {
				r.Timestamp = &ts
			} else if ts, err := time.Parse(time.RFC3339, e.Timestamp); err == nil {
				r.Timestamp = &ts
			}
		}
		byType[r.LogType] = append(byType[r.LogType], r)
	}
	for logType, events := range byType {
		_ = s.forensicRepo.ReplaceEvents(ctx, agentID, cmdID, logType, events)
	}
}

// updateAgentIsolation updates the agents.is_isolated column in PostgreSQL.
func (s *Server) updateAgentIsolation(ctx context.Context, agentID uuid.UUID, isolated bool) {
	if s.agentService == nil {
		return
	}
	if err := s.agentService.SetIsolation(ctx, agentID, isolated); err != nil {
		s.logger.WithError(err).Warnf("Failed to update agent %s isolation state", agentID)
	}
}

// updateAgentStatus updates the agent status column in PostgreSQL.
// Used for deliberate state transitions: suspended, online, offline.
func (s *Server) updateAgentStatus(ctx context.Context, agentID uuid.UUID, status string) {
	if s.agentService == nil {
		return
	}
	// AgentService.UpdateStatus(ctx, id, status, metrics) — pass nil metrics to skip metric update
	if err := s.agentService.UpdateStatus(ctx, agentID, status, nil); err != nil {
		s.logger.WithError(err).Warnf("Failed to update agent %s status to %s", agentID, status)
	}
}

// GetRegistry returns the server's AgentRegistry for use by the REST API.
func (s *Server) GetRegistry() *handlers.AgentRegistry {
	return s.registry
}
