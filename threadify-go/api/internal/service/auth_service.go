package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"threadify-go/api/internal/models"
	"threadify-go/api/internal/repository"
	"threadify-go/api/internal/utils"
	"threadify-go/api/internal/validation"
	sharedauth "threadify-go/shared/auth"

	"go.uber.org/zap"
)

const (
	userRole     = "user"
	standardRole = "standard"

	operationTimeout = 10 * time.Second
)

type OutboxWorkerTrigger interface {
	Trigger()
}

type AuthService struct {
	db            *sql.DB
	userRepo      *repository.UserRepository
	companyRepo   *repository.CompanyRepository
	userRoleRepo  *repository.UserRoleRepository
	outboxRepo    *repository.OutboxRepository
	emailSvc      *EmailService
	authClient    sharedauth.AuthClient
	jwksVerifier  *sharedauth.JWKSVerifier
	outboxWorker  OutboxWorkerTrigger
	encryptionKey []byte
	logger        *zap.Logger
}

func NewAuthService(
	db *sql.DB,
	emailSvc *EmailService,
	authClient sharedauth.AuthClient,
	outboxRepo *repository.OutboxRepository,
	outboxWorker OutboxWorkerTrigger,
	encryptionKey string,
	logger *zap.Logger,
) *AuthService {
	return &AuthService{
		db:            db,
		userRepo:      repository.NewUserRepository(db),
		companyRepo:   repository.NewCompanyRepository(db),
		userRoleRepo:  repository.NewUserRoleRepository(db),
		outboxRepo:    outboxRepo,
		emailSvc:      emailSvc,
		authClient:    authClient,
		outboxWorker:  outboxWorker,
		encryptionKey: []byte(encryptionKey),
		logger:        logger,
	}
}

func (s *AuthService) SetJWKSVerifier(verifier *sharedauth.JWKSVerifier) {
	s.jwksVerifier = verifier
}

func (s *AuthService) Signup(ctx context.Context, req *models.SignupRequest) error {
	if err := validation.ValidateSignupRequest(req); err != nil {
		return err
	}

	email := normalizeEmail(req.Email)
	if email == "" {
		return ErrInvalidCredentials
	}
	req.Email = email

	existing, err := s.userRepo.FindByEmail(email)
	if err != nil {
		return fmt.Errorf("check existing user: %w", err)
	}
	if existing != nil {
		return ErrUserAlreadyExists
	}

	now := time.Now()
	company := &models.Company{
		ID:        utils.GenerateID(),
		Name:      strings.TrimSpace(req.CompanyName),
		Industry:  req.Industry,
		Size:      req.CompanySize,
		UseCase:   req.UseCase,
		CreatedAt: now,
		UpdatedAt: now,
	}
	user := &models.User{
		ID:                       utils.GenerateID(),
		CompanyID:                company.ID,
		Email:                    email,
		FullName:                 normalizeOptionalString(req.FullName),
		JobRole:                  normalizeOptionalString(req.JobRole),
		EmailVerified:            false,
		OnboardingCompleted:      false,
		FirstInstrumentationDone: false,
		CreatedAt:                now,
		UpdatedAt:                now,
	}

	outboxEvent, err := s.buildRegisterAuthUserEvent(user, company, req.Password, req.FullName)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := s.companyRepo.CreateTx(tx, company); err != nil {
		return fmt.Errorf("create company: %w", err)
	}
	if err := s.userRepo.CreateTx(tx, user); err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	if err := s.userRoleRepo.AssignRoleToUserTx(tx, user.ID, "standard_account", "system"); err != nil {
		return fmt.Errorf("assign default role: %w", err)
	}
	if err := s.outboxRepo.CreateTx(tx, outboxEvent); err != nil {
		return fmt.Errorf("create outbox event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	if s.outboxWorker != nil {
		s.outboxWorker.Trigger()
	}

	return nil
}

func (s *AuthService) buildRegisterAuthUserEvent(
	user *models.User,
	company *models.Company,
	password, fullName string,
) (*models.OutboxEvent, error) {
	payload, err := json.Marshal(map[string]string{
		"email":        user.Email,
		"password":     password,
		"full_name":    fullName,
		"user_id":      user.ID,
		"company_id":   company.ID,
		"company_name": company.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal outbox payload: %w", err)
	}

	encrypted, err := utils.Encrypt(payload, s.encryptionKey)
	for i := range payload {
		payload[i] = 0
	}
	if err != nil {
		return nil, fmt.Errorf("encrypt outbox payload: %w", err)
	}

	return &models.OutboxEvent{
		ID:         utils.GenerateID(),
		Type:       models.EventTypeRegisterAuthUser,
		Payload:    encrypted,
		Status:     models.OutboxStatusPending,
		MaxRetries: 5,
		NextRunAt:  time.Now(),
	}, nil
}

func (s *AuthService) Login(ctx context.Context, req *models.LoginRequest, clientIP string) (*models.AuthResponse, error) {
	if err := validation.ValidateLoginRequest(req); err != nil {
		return nil, err
	}

	email := normalizeEmail(req.Email)
	if email == "" {
		return nil, ErrInvalidCredentials
	}

	localUser, err := s.userRepo.FindByEmail(email)
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}
	if localUser != nil && (localUser.AuthUserID == nil || strings.TrimSpace(*localUser.AuthUserID) == "") {
		return nil, ErrAccountStillProvisioning
	}

	authCtx, cancel := timeoutContext(ctx)
	defer cancel()

	accessToken, userInfo, err := s.authClient.LoginWithPassword(authCtx, email, req.Password, clientIP)
	if err != nil {
		if errors.Is(err, sharedauth.ErrAuthInvalidCredentials) {
			return nil, err
		}
		return nil, fmt.Errorf("authenticate: %w", err)
	}

	user, err := s.resolveUserFromAuthIdentity(email, userInfo)
	if err != nil {
		return nil, err
	}

	if err := s.userRepo.UpdateLastLogin(user.ID); err != nil {
		return nil, fmt.Errorf("update last login: %w", err)
	}

	if userInfo.EmailVerified && !user.EmailVerified {
		if err := s.userRepo.UpdateEmailVerified(user.ID, true); err == nil {
			user.EmailVerified = true
		}
	}

	return &models.AuthResponse{Token: accessToken, User: user}, nil
}

func (s *AuthService) ForgotPassword(ctx context.Context, req *models.ForgotPasswordRequest) error {
	if err := validation.ValidateForgotPasswordRequest(req); err != nil {
		return err
	}

	email := normalizeEmail(req.Email)
	if email == "" {
		return nil
	}

	user, err := s.userRepo.FindByEmail(email)
	if err != nil {
		return fmt.Errorf("find user: %w", err)
	}
	if user == nil {
		return nil
	}

	authCtx, cancel := timeoutContext(ctx)
	defer cancel()

	token, err := s.authClient.GeneratePasswordResetLink(authCtx, email)
	if err != nil {
		if errors.Is(err, sharedauth.ErrAuthInvalidEmail) {
			return ErrInvalidEmail
		}
		return nil
	}
	if token == "" {
		return nil
	}

	if err := s.emailSvc.SendPasswordResetEmail(ctx, email, token); err != nil {
		return fmt.Errorf("send password reset email: %w", err)
	}

	return nil
}

func (s *AuthService) ResetPassword(ctx context.Context, req *models.ResetPasswordRequest) error {
	authCtx, cancel := timeoutContext(ctx)
	defer cancel()

	if err := s.authClient.ResetPasswordWithToken(authCtx, req.Token, req.Password); err != nil {
		return fmt.Errorf("reset password: %w", err)
	}
	return nil
}

func (s *AuthService) VerifyEmail(ctx context.Context, req *models.VerifyEmailRequest) error {
	authCtx, cancel := timeoutContext(ctx)
	defer cancel()

	authUserID, err := s.authClient.VerifyEmailWithToken(authCtx, req.Token)
	if err != nil {
		return fmt.Errorf("verify email token: %w", err)
	}

	user, err := s.userRepo.FindByAuthUserID(authUserID)
	if err != nil {
		return fmt.Errorf("find user by auth ID: %w", err)
	}
	if user == nil {
		return ErrInvalidEmail
	}
	if user.EmailVerified {
		return nil
	}

	if err := s.userRepo.UpdateEmailVerified(user.ID, true); err != nil {
		return fmt.Errorf("update verification status: %w", err)
	}

	name := ""
	if user.FullName != nil {
		name = *user.FullName
	}
	go func() {
		sendCtx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		if err := s.emailSvc.SendWelcomeEmail(sendCtx, user.Email, name); err != nil {
			s.logger.Error("failed to send welcome email",
				zap.String("email", user.Email),
				zap.Error(err),
			)
		}
	}()

	return nil
}

func (s *AuthService) VerifyToken(ctx context.Context, tokenString string) (*sharedauth.TokenClaims, error) {
	if s.jwksVerifier == nil {
		return nil, ErrJwtVerificationNotConfigured
	}
	return s.jwksVerifier.Verify(ctx, tokenString)
}

func (s *AuthService) GetUserRoles(_ context.Context, userID, principalType string) ([]string, error) {
	switch principalType {
	case userRole:
		return s.userRoleRepo.GetUserRoles(userID)
	case standardRole:
		return s.userRoleRepo.GetServiceAccountRoles(userID)
	default:
		return nil, fmt.Errorf("invalid principal type: %s", principalType)
	}
}

func (s *AuthService) resolveUserFromAuthIdentity(emailHint string, info *sharedauth.AuthUserInfo) (*models.User, error) {
	email := normalizeEmail(emailHint)
	if info != nil && strings.TrimSpace(info.Email) != "" {
		email = normalizeEmail(info.Email)
	}

	var user *models.User
	var err error

	if email != "" {
		user, err = s.userRepo.FindByEmail(email)
		if err != nil {
			return nil, fmt.Errorf("find user by email: %w", err)
		}
	}

	if user == nil && info != nil && strings.TrimSpace(info.Sub) != "" {
		user, err = s.userRepo.FindByAuthUserID(strings.TrimSpace(info.Sub))
		if err != nil {
			return nil, fmt.Errorf("find user by auth ID: %w", err)
		}
	}

	if user == nil {
		return nil, ErrInvalidCredentials
	}

	if info != nil && strings.TrimSpace(info.Sub) != "" &&
		(user.AuthUserID == nil || strings.TrimSpace(*user.AuthUserID) == "") {
		sub := strings.TrimSpace(info.Sub)
		if err := s.userRepo.UpdateAuthUserID(user.ID, sub); err == nil {
			user.AuthUserID = &sub
		}
	}

	return user, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func normalizeOptionalString(value string) *string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return &trimmed
	}
	return nil
}

func timeoutContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, operationTimeout)
}
