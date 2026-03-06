// Package service provides the business logic layer.
package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"

	"github.com/edr-platform/connection-manager/internal/cache"
	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/edr-platform/connection-manager/pkg/models"
	"github.com/edr-platform/connection-manager/pkg/security"
)

// AuthService provides authentication and authorization services.
type AuthService interface {
	// Login authenticates a user and returns tokens.
	Login(ctx context.Context, username, password string) (*LoginResponse, error)

	// ValidateToken validates a JWT token.
	ValidateToken(ctx context.Context, tokenString string) (*security.Claims, error)

	// RefreshToken refreshes an access token using a refresh token.
	RefreshToken(ctx context.Context, refreshToken string) (string, time.Time, error)

	// Logout invalidates tokens.
	Logout(ctx context.Context, accessToken, refreshToken string) error

	// CreateUser creates a new user.
	CreateUser(ctx context.Context, req *CreateUserRequest) (*models.User, error)

	// ChangePassword changes a user's password.
	ChangePassword(ctx context.Context, userID uuid.UUID, oldPassword, newPassword string) error
}

// LoginResponse contains authentication tokens.
type LoginResponse struct {
	AccessToken  string
	RefreshToken string
	AccessExp    time.Time
	RefreshExp   time.Time
	User         *models.User
}

// CreateUserRequest contains user creation data.
type CreateUserRequest struct {
	Username string
	Email    string
	Password string
	FullName string
	Role     string
}

// authServiceImpl implements AuthService.
type authServiceImpl struct {
	userRepo   repository.UserRepository
	auditRepo  repository.AuditLogRepository
	jwtManager *security.JWTManager
	redis      *cache.RedisClient
	logger     *logrus.Logger

	maxLoginAttempts int
	lockDuration     time.Duration
}

// NewAuthService creates a new AuthService.
func NewAuthService(
	userRepo repository.UserRepository,
	auditRepo repository.AuditLogRepository,
	jwtManager *security.JWTManager,
	redis *cache.RedisClient,
	logger *logrus.Logger,
) AuthService {
	return &authServiceImpl{
		userRepo:         userRepo,
		auditRepo:        auditRepo,
		jwtManager:       jwtManager,
		redis:            redis,
		logger:           logger,
		maxLoginAttempts: 5,
		lockDuration:     15 * time.Minute,
	}
}

// Login authenticates a user.
func (s *authServiceImpl) Login(ctx context.Context, username, password string) (*LoginResponse, error) {
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		s.logger.WithField("username", username).Warn("Login attempt for unknown user")
		return nil, ErrInvalidPassword
	}

	// Check if account is locked
	if user.IsLocked() {
		s.logger.WithField("username", username).Warn("Login attempt on locked account")
		return nil, ErrAccountLocked
	}

	// Check if account is active
	if !user.IsActive() {
		return nil, ErrAccountLocked
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		// Record failed attempt
		user.RecordFailedLogin(s.maxLoginAttempts, s.lockDuration)
		s.userRepo.Update(ctx, user)

		// Audit log
		audit := models.NewAuditLog(user.ID, username, models.AuditActionLoginFailed, "user", user.ID)
		audit.MarkFailed("invalid password")
		s.auditRepo.Create(ctx, audit)

		return nil, ErrInvalidPassword
	}

	// Generate tokens
	tokenPair, err := s.jwtManager.GenerateTokenPair(user.ID.String(), []string{user.Role})
	if err != nil {
		s.logger.WithError(err).Error("Failed to generate tokens")
		return nil, ErrInternal
	}

	// Record successful login
	user.RecordSuccessfulLogin()
	s.userRepo.Update(ctx, user)

	// Audit log
	audit := models.NewAuditLog(user.ID, username, models.AuditActionLoginSuccess, "user", user.ID)
	s.auditRepo.Create(ctx, audit)

	s.logger.WithField("username", username).Info("User logged in successfully")

	return &LoginResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		AccessExp:    tokenPair.AccessExp,
		RefreshExp:   tokenPair.RefreshExp,
		User:         user,
	}, nil
}

// ValidateToken validates a JWT token.
func (s *authServiceImpl) ValidateToken(ctx context.Context, tokenString string) (*security.Claims, error) {
	// Check if token is blacklisted (skip when Redis unavailable)
	if s.redis != nil {
		jti, err := s.jwtManager.GetTokenID(tokenString)
		if err == nil {
			blacklisted, err := s.redis.IsTokenBlacklisted(ctx, jti)
			if err == nil && blacklisted {
				return nil, ErrInvalidToken
			}
		}
	}

	return s.jwtManager.ValidateToken(tokenString)
}

// RefreshToken refreshes an access token.
func (s *authServiceImpl) RefreshToken(ctx context.Context, refreshToken string) (string, time.Time, error) {
	return s.jwtManager.RefreshAccessToken(refreshToken)
}

// Logout invalidates tokens.
func (s *authServiceImpl) Logout(ctx context.Context, accessToken, refreshToken string) error {
	if s.redis == nil {
		return nil
	}
	// Blacklist access token
	if accessToken != "" {
		claims, err := s.jwtManager.ValidateToken(accessToken)
		if err == nil {
			s.redis.BlacklistToken(ctx, claims.ID, claims.ExpiresAt.Time, "logout")
		}
	}

	// Blacklist refresh token
	if refreshToken != "" {
		claims, err := s.jwtManager.ValidateToken(refreshToken)
		if err == nil {
			s.redis.BlacklistToken(ctx, claims.ID, claims.ExpiresAt.Time, "logout")
		}
	}

	return nil
}

// CreateUser creates a new user.
func (s *authServiceImpl) CreateUser(ctx context.Context, req *CreateUserRequest) (*models.User, error) {
	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, ErrInternal
	}

	user := &models.User{
		ID:           uuid.New(),
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		FullName:     req.FullName,
		Role:         req.Role,
		Status:       models.UserStatusActive,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	// Audit log
	audit := models.NewAuditLog(uuid.Nil, "system", models.AuditActionUserCreated, "user", user.ID)
	audit.WithDetail("username", user.Username)
	s.auditRepo.Create(ctx, audit)

	return user, nil
}

// ChangePassword changes a user's password.
func (s *authServiceImpl) ChangePassword(ctx context.Context, userID uuid.UUID, oldPassword, newPassword string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}

	// Verify old password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)); err != nil {
		return ErrInvalidPassword
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return ErrInternal
	}

	if err := s.userRepo.UpdatePassword(ctx, userID, string(hashedPassword)); err != nil {
		return err
	}

	// Audit log
	audit := models.NewAuditLog(userID, user.Username, models.AuditActionPasswordChanged, "user", userID)
	s.auditRepo.Create(ctx, audit)

	return nil
}
