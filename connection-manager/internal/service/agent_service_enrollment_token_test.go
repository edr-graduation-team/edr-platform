package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/edr-platform/connection-manager/pkg/models"
)

type fakeAgentRepo struct{}

func (r *fakeAgentRepo) Create(ctx context.Context, agent *models.Agent) error { return nil }
func (r *fakeAgentRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Agent, error) {
	return &models.Agent{ID: id}, nil
}
func (r *fakeAgentRepo) GetByHostname(ctx context.Context, hostname string) (*models.Agent, error) {
	return nil, repository.ErrNotFound
}
func (r *fakeAgentRepo) Update(ctx context.Context, agent *models.Agent) error { return nil }
func (r *fakeAgentRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string, lastSeen time.Time) error {
	return nil
}
func (r *fakeAgentRepo) UpdateMetrics(ctx context.Context, id uuid.UUID, cpuUsage float64, memoryUsedMB int64,
	memoryTotalMB int64, queueDepth int, eventsGenerated, eventsSent, eventsDropped int64,
	agentVersion string, ipAddresses []string, cpuCount int, healthScore float64,
	sysmonInstalled, sysmonRunning bool,
) error {
	return nil
}
func (r *fakeAgentRepo) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (r *fakeAgentRepo) List(ctx context.Context, filter repository.AgentFilter) ([]*models.Agent, error) {
	return nil, nil
}
func (r *fakeAgentRepo) Count(ctx context.Context, filter repository.AgentFilter) (int64, error) {
	return 0, nil
}
func (r *fakeAgentRepo) GetOnlineAgents(ctx context.Context) ([]*models.Agent, error) {
	return nil, nil
}
func (r *fakeAgentRepo) GetAgentsNeedingCertRenewal(ctx context.Context, within time.Duration) ([]*models.Agent, error) {
	return nil, nil
}
func (r *fakeAgentRepo) MarkStaleOffline(ctx context.Context, threshold time.Duration) (int64, error) {
	return 0, nil
}
func (r *fakeAgentRepo) SetIsolation(ctx context.Context, id uuid.UUID, isolated bool) error {
	return nil
}
func (r *fakeAgentRepo) UpsertByHostname(ctx context.Context, agent *models.Agent) error { return nil }
func (r *fakeAgentRepo) UpdateBusinessContext(ctx context.Context, id uuid.UUID, ctxFields repository.AgentBusinessContext) error {
	return nil
}
func (r *fakeAgentRepo) UpdateDeviceInfo(ctx context.Context, id uuid.UUID, profile, loggedInUser, signatureServerVersion string) error {
	return nil
}

type fakeTokenRepo struct{}

func (r *fakeTokenRepo) Create(ctx context.Context, token *models.InstallationToken) error {
	return nil
}
func (r *fakeTokenRepo) GetByValue(ctx context.Context, value string) (*models.InstallationToken, error) {
	return nil, repository.ErrNotFound
}
func (r *fakeTokenRepo) MarkUsed(ctx context.Context, id uuid.UUID, agentID uuid.UUID) error {
	return nil
}
func (r *fakeTokenRepo) Delete(ctx context.Context, id uuid.UUID) error   { return nil }
func (r *fakeTokenRepo) DeleteExpired(ctx context.Context) (int64, error) { return 0, nil }

type fakeAuditRepo struct{}

func (r *fakeAuditRepo) Create(ctx context.Context, log *models.AuditLog) error { return nil }
func (r *fakeAuditRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.AuditLog, error) {
	return nil, repository.ErrNotFound
}
func (r *fakeAuditRepo) List(ctx context.Context, filter repository.AuditLogFilter) ([]*models.AuditLog, error) {
	return nil, nil
}
func (r *fakeAuditRepo) Count(ctx context.Context, filter repository.AuditLogFilter) (int64, error) {
	return 0, nil
}

type fakeEnrollmentTokenRepo struct {
	t            *models.EnrollmentToken
	consumptions map[string]bool // key: tokenID + ":" + hardwareID
}

func (r *fakeEnrollmentTokenRepo) Create(ctx context.Context, token *models.EnrollmentToken) error {
	r.t = token
	return nil
}
func (r *fakeEnrollmentTokenRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.EnrollmentToken, error) {
	if r.t == nil || r.t.ID != id {
		return nil, repository.ErrNotFound
	}
	return r.t, nil
}
func (r *fakeEnrollmentTokenRepo) GetByToken(ctx context.Context, token string) (*models.EnrollmentToken, error) {
	if r.t == nil || r.t.Token != token {
		return nil, repository.ErrNotFound
	}
	return r.t, nil
}
func (r *fakeEnrollmentTokenRepo) List(ctx context.Context) ([]*models.EnrollmentToken, error) {
	if r.t == nil {
		return []*models.EnrollmentToken{}, nil
	}
	return []*models.EnrollmentToken{r.t}, nil
}
func (r *fakeEnrollmentTokenRepo) IncrementUsage(ctx context.Context, id uuid.UUID) error {
	if r.t == nil || r.t.ID != id {
		return repository.ErrNotFound
	}
	r.t.UseCount++
	return nil
}
func (r *fakeEnrollmentTokenRepo) Revoke(ctx context.Context, id uuid.UUID) error {
	if r.t == nil || r.t.ID != id {
		return repository.ErrNotFound
	}
	r.t.IsActive = false
	return nil
}
func (r *fakeEnrollmentTokenRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if r.t == nil || r.t.ID != id {
		return repository.ErrNotFound
	}
	r.t = nil
	return nil
}

func (r *fakeEnrollmentTokenRepo) HasConsumption(ctx context.Context, tokenID uuid.UUID, hardwareID string) (bool, error) {
	if r.consumptions == nil {
		return false, nil
	}
	return r.consumptions[tokenID.String()+":"+hardwareID], nil
}

func (r *fakeEnrollmentTokenRepo) RecordConsumption(ctx context.Context, tokenID uuid.UUID, hardwareID string, agentID uuid.UUID) (bool, error) {
	if r.consumptions == nil {
		r.consumptions = map[string]bool{}
	}
	k := tokenID.String() + ":" + hardwareID
	if r.consumptions[k] {
		return false, nil
	}
	r.consumptions[k] = true
	return true, nil
}

type fakeCertService struct{}

func (s *fakeCertService) Issue(ctx context.Context, agentID uuid.UUID, csrPEM []byte) (*IssuedCertificate, error) {
	return &IssuedCertificate{
		Certificate: []byte("cert"),
		CACert:      []byte("ca"),
	}, nil
}
func (s *fakeCertService) Renew(ctx context.Context, agentID uuid.UUID, csrPEM []byte, oldFingerprint string) (*IssuedCertificate, error) {
	return nil, errors.New("not implemented")
}
func (s *fakeCertService) Revoke(ctx context.Context, certID uuid.UUID, revokedBy uuid.UUID, reason string) error {
	return errors.New("not implemented")
}
func (s *fakeCertService) GetActive(ctx context.Context, agentID uuid.UUID) (*models.Certificate, error) {
	return nil, errors.New("not implemented")
}
func (s *fakeCertService) IsRevoked(ctx context.Context, fingerprint string) (bool, error) {
	return false, errors.New("not implemented")
}

func TestRegister_MultiUseEnrollmentToken_ExhaustsAndRejectsFurtherUse(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()

	maxUses := 2
	tokenStr := "test-token"
	enrollRepo := &fakeEnrollmentTokenRepo{
		t: &models.EnrollmentToken{
			ID:       uuid.New(),
			Token:    tokenStr,
			IsActive: true,
			UseCount: 0,
			MaxUses:  &maxUses,
		},
	}

	svc := NewAgentService(
		&fakeAgentRepo{},
		&fakeTokenRepo{},
		enrollRepo,
		&fakeAuditRepo{},
		nil,
		logger,
		&fakeCertService{},
	)

	// Consume exactly 2 seats with two distinct devices.
	for i, hwid := range []string{"hwid-1", "hwid-2"} {
		_, err := svc.Register(ctx, &RegisterAgentRequest{
			InstallationToken: tokenStr,
			Hostname:          uuid.New().String(),
			OSType:            "windows",
			OSVersion:         "test",
			HardwareID:        hwid,
			CSRData:           []byte("csr"),
		})
		if err != nil {
			t.Fatalf("registration %d failed: %v", i+1, err)
		}
	}

	// Third attempt with same token must fail.
	_, err := svc.Register(ctx, &RegisterAgentRequest{
		InstallationToken: tokenStr,
		Hostname:          uuid.New().String(),
		OSType:            "windows",
		OSVersion:         "test",
		HardwareID:        "hwid-3",
		CSRData:           []byte("csr"),
	})
	if !errors.Is(err, ErrExpiredToken) && !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrExpiredToken/ErrInvalidToken on 3rd use, got: %v", err)
	}
}

func TestRegister_MultiUseEnrollmentToken_IdempotentPerHardwareID(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()

	maxUses := 2
	tokenStr := "test-token"
	enrollRepo := &fakeEnrollmentTokenRepo{
		t: &models.EnrollmentToken{
			ID:       uuid.New(),
			Token:    tokenStr,
			IsActive: true,
			UseCount: 0,
			MaxUses:  &maxUses,
		},
	}

	svc := NewAgentService(
		&fakeAgentRepo{},
		&fakeTokenRepo{},
		enrollRepo,
		&fakeAuditRepo{},
		nil,
		logger,
		&fakeCertService{},
	)

	// Device A enrolls twice: should only consume 1 seat.
	for i := 0; i < 2; i++ {
		_, err := svc.Register(ctx, &RegisterAgentRequest{
			InstallationToken: tokenStr,
			Hostname:          uuid.New().String(),
			OSType:            "windows",
			OSVersion:         "test",
			HardwareID:        "hwid-A",
			CSRData:           []byte("csr"),
		})
		if err != nil {
			t.Fatalf("repeat enrollment %d failed: %v", i+1, err)
		}
	}

	if enrollRepo.t == nil || enrollRepo.t.UseCount != 1 {
		t.Fatalf("expected use_count=1 after idempotent re-enrollment; got %+v", enrollRepo.t)
	}

	// Device B consumes the 2nd seat.
	_, err := svc.Register(ctx, &RegisterAgentRequest{
		InstallationToken: tokenStr,
		Hostname:          uuid.New().String(),
		OSType:            "windows",
		OSVersion:         "test",
		HardwareID:        "hwid-B",
		CSRData:           []byte("csr"),
	})
	if err != nil {
		t.Fatalf("device B enrollment failed: %v", err)
	}

	// Device C should be rejected.
	_, err = svc.Register(ctx, &RegisterAgentRequest{
		InstallationToken: tokenStr,
		Hostname:          uuid.New().String(),
		OSType:            "windows",
		OSVersion:         "test",
		HardwareID:        "hwid-C",
		CSRData:           []byte("csr"),
	})
	if !errors.Is(err, ErrExpiredToken) && !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrExpiredToken/ErrInvalidToken for device C, got: %v", err)
	}
}
