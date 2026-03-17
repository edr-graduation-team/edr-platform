// Package handlers provides gRPC handler implementations.
package handlers

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/edr-platform/connection-manager/internal/cache"
	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/edr-platform/connection-manager/pkg/models"
	edrv1 "github.com/edr-platform/connection-manager/proto/v1"
)

// RegistrationHandler handles agent registration RPCs.
type RegistrationHandler struct {
	logger    *logrus.Logger
	redis     *cache.RedisClient
	agentRepo repository.AgentRepository
	tokenRepo repository.InstallationTokenRepository
	csrRepo   repository.CSRRepository
}

// NewRegistrationHandler creates a new registration handler.
func NewRegistrationHandler(
	logger *logrus.Logger,
	redis *cache.RedisClient,
	agentRepo repository.AgentRepository,
	tokenRepo repository.InstallationTokenRepository,
	csrRepo repository.CSRRepository,
) *RegistrationHandler {
	return &RegistrationHandler{
		logger:    logger,
		redis:     redis,
		agentRepo: agentRepo,
		tokenRepo: tokenRepo,
		csrRepo:   csrRepo,
	}
}

// RegisterAgent handles new agent registration.
func (h *RegistrationHandler) RegisterAgent(ctx context.Context, req *edrv1.AgentRegistrationRequest) (*edrv1.AgentRegistrationResponse, error) {
	logger := h.logger.WithFields(logrus.Fields{
		"hostname": req.Hostname,
		"agent_id": req.AgentId,
	})

	logger.Info("Agent registration request received")

	// 1. Validate request
	if err := h.validateRegistrationRequest(req); err != nil {
		logger.WithError(err).Warn("Invalid registration request")
		return nil, err
	}

	// 2. Validate installation token from database
	token, err := h.tokenRepo.GetByValue(ctx, req.InstallationToken)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, status.Error(codes.Unauthenticated, "invalid installation token")
		}
		logger.WithError(err).Error("Failed to look up installation token")
		return nil, status.Error(codes.Internal, "failed to validate token")
	}
	if !token.IsValid() {
		return nil, status.Error(codes.Unauthenticated, "installation token is expired or already used")
	}

	// 3. Check for existing registration by hostname.
	//    NEW AGENT    → existingAgent == nil → fresh INSERT via UpsertByHostname.
	//    RE-ENROLLMENT→ existingAgent != nil → UPDATE in place via UpsertByHostname
	//                   (re-image, re-install, or agent corruption scenario).
	existingAgent, err := h.agentRepo.GetByHostname(ctx, req.Hostname)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		logger.WithError(err).Error("Failed to check for existing agent")
		return nil, status.Error(codes.Internal, "failed to check agent registration")
	}

	isReEnrollment := existingAgent != nil
	if isReEnrollment {
		logger.WithFields(logrus.Fields{
			"old_agent_id": existingAgent.ID.String(),
			"hostname":     req.Hostname,
		}).Warn("Re-enrollment detected for existing hostname — updating agent record in place")
	}

	// 4. Generate (or reuse) agent ID.
	//    On re-enrollment we always mint a new UUID so the old ID is fully
	//    invalidated server-side (heartbeats / streams with the stale UUID will
	//    get Unauthenticated and the new agent will re-connect with the new ID).
	agentUUID := uuid.New()
	if !isReEnrollment && req.AgentId != "" {
		if parsed, err := uuid.Parse(req.AgentId); err == nil {
			agentUUID = parsed
		}
	}

	// 5. Build the agent model. For re-enrollments we carry over the old agent
	//    UUID as metadata so the audit trail is preserved.
	now := time.Now()
	metadata := map[string]string{
		"ip_addresses": joinStrings(req.IpAddresses),
		"mac_address":  req.MacAddress,
	}
	if isReEnrollment {
		metadata["previous_agent_id"] = existingAgent.ID.String()
		metadata["re_enrolled_at"] = now.UTC().Format("2006-01-02T15:04:05Z")
	}

	agent := &models.Agent{
		ID:            agentUUID,
		Hostname:      req.Hostname,
		Status:        models.AgentStatusPending,
		OSType:        req.OsType,
		OSVersion:     req.OsVersion,
		CPUCount:      int(req.CpuCount),
		MemoryMB:      req.MemoryMb,
		AgentVersion:  req.AgentVersion,
		InstalledDate: &now,
		LastSeen:      now,
		Tags:          req.Tags,
		Metadata:      metadata,
	}

	// 6. Upsert: single atomic INSERT … ON CONFLICT DO UPDATE.
	//    Works for both first-time registrations and re-enrollments.
	if err := h.agentRepo.UpsertByHostname(ctx, agent); err != nil {
		logger.WithError(err).Error("Failed to upsert agent record")
		return nil, status.Error(codes.Internal, "failed to register agent")
	}

	logFields := logrus.Fields{
		"agent_id":   agentUUID.String(),
		"os_type":    req.OsType,
		"os_version": req.OsVersion,
		"cpu_count":  req.CpuCount,
		"memory_mb":  req.MemoryMb,
		"re_enroll":  isReEnrollment,
	}
	if isReEnrollment {
		logFields["old_agent_id"] = existingAgent.ID.String()
	}
	logger.WithFields(logFields).Info("Agent upserted in database")

	// 7. Retire old CSR (if any) then store the new one.
	if isReEnrollment {
		if oldCSR, err := h.csrRepo.GetByAgentID(ctx, existingAgent.ID); err == nil && oldCSR != nil {
			if delErr := h.csrRepo.Delete(ctx, oldCSR.ID); delErr != nil {
				logger.WithError(delErr).Warn("Failed to delete old CSR during re-enrollment (non-fatal)")
			}
		}
	}

	csr := &models.CSR{
		ID:        uuid.New(),
		AgentID:   agentUUID,
		CSRData:   req.Csr,
		CreatedAt: now,
		ExpiresAt: now.Add(24 * time.Hour),
	}
	if err := h.csrRepo.Create(ctx, csr); err != nil {
		logger.WithError(err).Error("Failed to store CSR")
		// Non-fatal — agent is registered, CSR can be resubmitted
	}

	// 8. Mark installation token as used.
	if err := h.tokenRepo.MarkUsed(ctx, token.ID, agentUUID); err != nil {
		logger.WithError(err).Warn("Failed to mark installation token as used")
	}

	// 9. Notify dashboard via Redis pub/sub.
	channel := "agents:pending"
	if isReEnrollment {
		channel = "agents:re-enrolled"
	}
	if h.redis != nil {
		if err := h.redis.Publish(ctx, channel, agentUUID.String()); err != nil {
			logger.WithError(err).Warn("Failed to notify dashboard (non-fatal)")
		}
	}

	if isReEnrollment {
		logger.WithField("agent_id", agentUUID.String()).Info("Agent re-enrolled successfully (pending approval)")
	} else {
		logger.Info("Agent registered successfully (pending approval)")
	}

	return &edrv1.AgentRegistrationResponse{
		AgentId: agentUUID.String(),
		Status:  edrv1.RegistrationStatus_REGISTRATION_STATUS_PENDING,
		Message: "Agent registration pending admin approval",
	}, nil
}

// joinStrings joins strings with commas for metadata storage.
func joinStrings(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ","
		}
		result += s
	}
	return result
}

// validateRegistrationRequest validates the registration request.
func (h *RegistrationHandler) validateRegistrationRequest(req *edrv1.AgentRegistrationRequest) error {
	if req.InstallationToken == "" {
		return status.Error(codes.InvalidArgument, "installation_token is required")
	}
	if req.Hostname == "" {
		return status.Error(codes.InvalidArgument, "hostname is required")
	}
	if len(req.Csr) == 0 {
		return status.Error(codes.InvalidArgument, "csr is required")
	}
	if req.OsType == "" {
		return status.Error(codes.InvalidArgument, "os_type is required")
	}
	return nil
}

// CertRenewalHandler handles certificate renewal RPCs.
type CertRenewalHandler struct {
	logger *logrus.Logger
	redis  *cache.RedisClient
	// certRepo repository.CertificateRepository
	// caService *security.CAService
}

// NewCertRenewalHandler creates a new certificate renewal handler.
func NewCertRenewalHandler(logger *logrus.Logger, redis *cache.RedisClient) *CertRenewalHandler {
	return &CertRenewalHandler{
		logger: logger,
		redis:  redis,
	}
}

// RequestCertificateRenewal handles certificate renewal requests.
func (h *CertRenewalHandler) RequestCertificateRenewal(ctx context.Context, req *edrv1.CertRenewalRequest) (*edrv1.CertificateResponse, error) {
	agentID := req.AgentId
	logger := h.logger.WithField("agent_id", agentID)

	logger.Info("Certificate renewal request received")

	// 1. Validate request
	if agentID == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}
	if len(req.Csr) == 0 {
		return nil, status.Error(codes.InvalidArgument, "csr is required")
	}

	// 2. Validate current certificate fingerprint
	// TODO: Verify agent has valid current certificate

	// 3. Parse and validate CSR
	// TODO: Parse CSR using x509.ParseCertificateRequest

	// 4. Sign new certificate (90 days validity)
	// TODO: Use CA service to sign certificate

	// 5. Store new certificate in database
	// TODO: Store certificate

	// 6. Mark old certificate as superseded
	// TODO: Update old certificate status

	logger.Info("Certificate renewed successfully")

	// Return placeholder response (actual implementation will return real certificate)
	now := time.Now()
	expiresAt := now.AddDate(0, 0, 90) // 90 days

	return &edrv1.CertificateResponse{
		Certificate:  nil, // TODO: Return actual certificate bytes
		CaChain:      nil, // TODO: Return CA chain
		IssuedAt:     timestamppb.New(now),
		ExpiresAt:    timestamppb.New(expiresAt),
		Fingerprint:  "", // TODO: Calculate fingerprint
		SerialNumber: uuid.New().String(),
	}, nil
}
