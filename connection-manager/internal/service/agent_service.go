// Package service provides the business logic layer.
package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/edr-platform/connection-manager/internal/cache"
	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/edr-platform/connection-manager/pkg/models"
)

// AgentService provides business logic for agent operations.
type AgentService interface {
	// Register registers a new agent with the system.
	Register(ctx context.Context, req *RegisterAgentRequest) (*RegisterAgentResponse, error)

	// GetByID retrieves an agent by its ID.
	GetByID(ctx context.Context, id uuid.UUID) (*models.Agent, error)

	// UpdateStatus updates agent status and optionally metrics.
	UpdateStatus(ctx context.Context, id uuid.UUID, status string, metrics *AgentMetrics) error

	// UpdateMetrics updates agent metrics from a heartbeat.
	UpdateMetrics(ctx context.Context, id uuid.UUID, metrics *AgentMetrics) error

	// GetOnlineAgents returns all currently online agents.
	GetOnlineAgents(ctx context.Context) ([]*models.Agent, error)

	// ListAgents returns agents matching the given filter.
	ListAgents(ctx context.Context, filter repository.AgentFilter) ([]*models.Agent, error)

	// CountAgents returns the count of agents matching the given filter.
	CountAgents(ctx context.Context, filter repository.AgentFilter) (int64, error)

	// Suspend suspends an agent.
	Suspend(ctx context.Context, id uuid.UUID, reason string) error

	// Approve approves a pending agent.
	Approve(ctx context.Context, id uuid.UUID, approvedBy uuid.UUID) error

	// SetIsolation updates the is_isolated flag for an agent.
	SetIsolation(ctx context.Context, id uuid.UUID, isolated bool) error
}

// RegisterAgentRequest contains registration request data.
type RegisterAgentRequest struct {
	InstallationToken string
	Hostname          string
	OSType            string
	OSVersion         string
	CPUCount          int
	MemoryMB          int64
	AgentVersion      string
	CSRData           []byte
	IPAddresses       []string
	Tags              map[string]string
}

// RegisterAgentResponse contains registration response data.
type RegisterAgentResponse struct {
	AgentID     uuid.UUID
	Status      string
	Certificate []byte
	CACert      []byte
	AccessToken string
}

// AgentMetrics contains agent performance metrics.
type AgentMetrics struct {
	CPUUsage        float64
	MemoryUsedMB    int64
	MemoryTotalMB   int64
	QueueDepth      int
	EventsGenerated int64
	EventsSent      int64
	EventsDropped   int64
	AgentVersion    string
	IPAddresses     []string
	CpuCount        int
}

// RegistrationStatusApproved is the status string for approved (cert-issued) registration.
const RegistrationStatusApproved = "approved"

// agentServiceImpl implements AgentService.
type agentServiceImpl struct {
	agentRepo           repository.AgentRepository
	tokenRepo           repository.InstallationTokenRepository
	enrollmentTokenRepo repository.EnrollmentTokenRepository
	auditRepo           repository.AuditLogRepository
	redis               *cache.RedisClient
	logger              *logrus.Logger
	certService         CertificateService
}

// NewAgentService creates a new AgentService.
// certService may be nil; if so, Register will succeed but return pending status with no certificate.
func NewAgentService(
	agentRepo repository.AgentRepository,
	tokenRepo repository.InstallationTokenRepository,
	enrollmentTokenRepo repository.EnrollmentTokenRepository,
	auditRepo repository.AuditLogRepository,
	redis *cache.RedisClient,
	logger *logrus.Logger,
	certService CertificateService,
) AgentService {
	return &agentServiceImpl{
		agentRepo:           agentRepo,
		tokenRepo:           tokenRepo,
		enrollmentTokenRepo: enrollmentTokenRepo,
		auditRepo:           auditRepo,
		redis:               redis,
		logger:              logger,
		certService:         certService,
	}
}

// Register registers a new agent using First-Touch Provisioning.
//
// This method implements auto-enrollment: when a valid installation token and
// CSR are presented, the agent is immediately activated (status = 'online'),
// its CSR is signed by the CA, and the certificate is returned in the first
// response. The token is only burned after successful cert issuance, preventing
// the "Enrollment Catch-22" where a burned token + pending status left the
// agent unable to obtain its certificate.
func (s *agentServiceImpl) Register(ctx context.Context, req *RegisterAgentRequest) (*RegisterAgentResponse, error) {
	// 1. Validate token: try dynamic enrollment tokens first, then fall back to legacy installation tokens.
	var legacyToken *models.InstallationToken
	var enrollmentToken *models.EnrollmentToken

	if s.enrollmentTokenRepo != nil {
		if et, err := s.enrollmentTokenRepo.GetByToken(ctx, req.InstallationToken); err == nil {
			if !et.IsValid() {
				s.logger.Warnf("Enrollment token %s is invalid (revoked/expired/max-uses)", et.ID)
				return nil, ErrExpiredToken
			}
			enrollmentToken = et
		}
	}

	// Fall back to legacy one-time installation token if enrollment token not found
	if enrollmentToken == nil {
		token, err := s.tokenRepo.GetByValue(ctx, req.InstallationToken)
		if err != nil {
			return nil, ErrInvalidToken
		}
		if !token.IsValid() {
			return nil, ErrExpiredToken
		}
		legacyToken = token
	}

	// 2. Check for duplicate hostname
	existing, err := s.agentRepo.GetByHostname(ctx, req.Hostname)
	if err == nil && existing != nil {
		return nil, ErrDuplicateAgent
	}

	// 3. Generate agent ID
	agentID := uuid.New()
	now := time.Now()

	// 4. Create agent record as PENDING first.
	//    The certificates table has FK → agents(id), so the agent row MUST
	//    exist before CertificateService.Issue inserts the signed cert.
	agent := &models.Agent{
		ID:            agentID,
		Hostname:      req.Hostname,
		Status:        models.AgentStatusPending,
		OSType:        req.OSType,
		OSVersion:     req.OSVersion,
		CPUCount:      req.CPUCount,
		MemoryMB:      req.MemoryMB,
		AgentVersion:  req.AgentVersion,
		InstalledDate: &now,
		LastSeen:      now,
		Tags:          req.Tags,
		HealthScore:   100.0,
	}

	if err := s.agentRepo.Create(ctx, agent); err != nil {
		s.logger.WithError(err).Error("Failed to create agent")
		return nil, ErrInternal
	}

	// 5. Attempt certificate issuance (agent row now exists → FK satisfied).
	var issued *IssuedCertificate
	certIssued := false
	if s.certService != nil && len(req.CSRData) > 0 {
		issued, err = s.certService.Issue(ctx, agentID, req.CSRData)
		if err != nil {
			s.logger.WithError(err).Warn("Certificate issuance failed; agent stays pending, token NOT burned (retryable)")
		} else {
			certIssued = true
		}
	}

	// 6. Post-issuance: activate agent and burn token only on cert success.
	if certIssued {
		// Promote agent from 'pending' → 'online'
		if err := s.agentRepo.UpdateStatus(ctx, agentID, models.AgentStatusOnline, now); err != nil {
			s.logger.WithError(err).Error("Failed to activate agent after cert issuance")
			// Non-fatal: cert was issued, agent can still connect; status will
			// be corrected on first heartbeat.
		}

		// Burn/increment token ONLY after successful cert issuance.
		if enrollmentToken != nil {
			if err := s.enrollmentTokenRepo.IncrementUsage(ctx, enrollmentToken.ID); err != nil {
				s.logger.WithError(err).Error("Failed to increment enrollment token usage")
			}
		} else if legacyToken != nil {
			if err := s.tokenRepo.MarkUsed(ctx, legacyToken.ID, agentID); err != nil {
				s.logger.WithError(err).Error("Failed to mark token as used")
			}
		}
	}
	// If cert failed: agent stays 'pending', token stays valid → agent can retry.

	// 7. Create audit log
	audit := models.NewAuditLog(uuid.Nil, "system", models.AuditActionAgentRegistered, "agent", agentID)
	audit.WithDetails("hostname: " + req.Hostname)
	s.auditRepo.Create(ctx, audit)

	// 8. Notify dashboard via Redis pub/sub (skip when Redis unavailable)
	if s.redis != nil {
		channel := "agents:registered"
		if certIssued {
			channel = "agents:activated"
		}
		s.redis.Publish(ctx, channel, agentID.String())
	}

	// 9. Build response
	resp := &RegisterAgentResponse{
		AgentID: agentID,
	}

	if certIssued {
		resp.Status = RegistrationStatusApproved
		resp.Certificate = issued.Certificate
		resp.CACert = issued.CACert
		s.logger.WithFields(logrus.Fields{
			"agent_id": agentID,
			"hostname": req.Hostname,
			"status":   models.AgentStatusOnline,
		}).Info("Agent registered and certificate issued (First-Touch Provisioning)")
	} else {
		resp.Status = models.AgentStatusPending
		s.logger.WithFields(logrus.Fields{
			"agent_id": agentID,
			"hostname": req.Hostname,
			"status":   models.AgentStatusPending,
		}).Info("Agent registered (pending — certificate not issued)")
	}

	return resp, nil
}

// GetByID retrieves an agent by ID.
func (s *agentServiceImpl) GetByID(ctx context.Context, id uuid.UUID) (*models.Agent, error) {
	return s.agentRepo.GetByID(ctx, id)
}

// UpdateStatus updates agent status.
func (s *agentServiceImpl) UpdateStatus(ctx context.Context, id uuid.UUID, status string, metrics *AgentMetrics) error {
	// Update database
	if err := s.agentRepo.UpdateStatus(ctx, id, status, time.Now()); err != nil {
		return err
	}

	// Update Redis cache (skip when Redis unavailable)
	if s.redis != nil {
		s.redis.SetAgentStatus(ctx, id.String(), status, 5*time.Minute)
	}

	// Update metrics if provided
	if metrics != nil {
		return s.UpdateMetrics(ctx, id, metrics)
	}

	return nil
}

// UpdateMetrics updates agent metrics.
func (s *agentServiceImpl) UpdateMetrics(ctx context.Context, id uuid.UUID, metrics *AgentMetrics) error {
	return s.agentRepo.UpdateMetrics(ctx, id,
		metrics.CPUUsage,
		metrics.MemoryUsedMB,
		metrics.MemoryTotalMB,
		metrics.QueueDepth,
		metrics.EventsGenerated,
		metrics.EventsSent,
		metrics.EventsDropped,
		metrics.AgentVersion,
		metrics.IPAddresses,
		metrics.CpuCount,
	)
}

// GetOnlineAgents returns all online agents.
func (s *agentServiceImpl) GetOnlineAgents(ctx context.Context) ([]*models.Agent, error) {
	return s.agentRepo.GetOnlineAgents(ctx)
}

// ListAgents returns agents matching the given filter.
func (s *agentServiceImpl) ListAgents(ctx context.Context, filter repository.AgentFilter) ([]*models.Agent, error) {
	return s.agentRepo.List(ctx, filter)
}

// CountAgents returns the count of agents matching the given filter.
func (s *agentServiceImpl) CountAgents(ctx context.Context, filter repository.AgentFilter) (int64, error) {
	return s.agentRepo.Count(ctx, filter)
}

// Suspend suspends an agent.
func (s *agentServiceImpl) Suspend(ctx context.Context, id uuid.UUID, reason string) error {
	if err := s.agentRepo.UpdateStatus(ctx, id, models.AgentStatusSuspended, time.Now()); err != nil {
		return err
	}

	// Audit log
	audit := models.NewAuditLog(uuid.Nil, "system", models.AuditActionAgentSuspended, "agent", id)
	audit.WithDetails(reason)
	s.auditRepo.Create(ctx, audit)

	return nil
}

// Approve approves a pending agent.
func (s *agentServiceImpl) Approve(ctx context.Context, id uuid.UUID, approvedBy uuid.UUID) error {
	if err := s.agentRepo.UpdateStatus(ctx, id, models.AgentStatusOnline, time.Now()); err != nil {
		return err
	}

	// Audit log
	audit := models.NewAuditLog(approvedBy, "", models.AuditActionAgentApproved, "agent", id)
	s.auditRepo.Create(ctx, audit)

	return nil
}

// SetIsolation updates the is_isolated flag for an agent.
func (s *agentServiceImpl) SetIsolation(ctx context.Context, id uuid.UUID, isolated bool) error {
	return s.agentRepo.SetIsolation(ctx, id, isolated)
}
