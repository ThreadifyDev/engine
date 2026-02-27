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
	serror "threadify-go/shared/errors"

	"go.uber.org/zap"
)

const (
	userRole         = "user"
	standardRole     = "standard"
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

	existing, err := s.userRepo.FindByEmail(req.Email)
	if err != nil && !errors.Is(err, serror.ErrUserNotFound) {
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
		ID:        utils.GenerateID(),
		CompanyID: company.ID,
		Email:     req.Email,
		FullName:  normalizeOptionalString(req.FullName),
		JobRole:   normalizeOptionalString(req.JobRole),
		CreatedAt: now,
		UpdatedAt: now,
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

	s.logger.Info("signup: user registered",
		zap.String("user_id", user.ID),
		zap.String("company_id", company.ID),
	)

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
		ID:          utils.GenerateID(),
		Type:        models.EventTypeRegisterAuthUser,
		Payload:     encrypted,
		Status:      models.OutboxStatusPending,
		MaxRetries:  5,
		NextRunAt:   time.Now(),
		ReferenceID: user.ID,
	}, nil
}

func (s *AuthService) Login(ctx context.Context, req *models.LoginRequest, clientIP string) (*models.AuthResponse, error) {
	if err := validation.ValidateLoginRequest(req); err != nil {
		return nil, err
	}

	localUser, err := s.userRepo.FindByEmail(req.Email)
	if err != nil && !errors.Is(err, serror.ErrUserNotFound) {
		return nil, fmt.Errorf("find user: %w", err)
	}
	if localUser != nil && (localUser.AuthUserID == nil || strings.TrimSpace(*localUser.AuthUserID) == "") {
		return nil, ErrAccountStillProvisioning
	}

	authCtx, cancel := timeoutContext(ctx)
	defer cancel()

	_, userInfo, err := s.authClient.LoginWithPassword(authCtx, req.Email, req.Password, clientIP)
	if err != nil {
		switch {
		case errors.Is(err, sharedauth.ErrAuthInvalidCredentials):
			return nil, ErrInvalidCredentials
		case errors.Is(err, sharedauth.ErrAuthRateLimit):
			return nil, ErrRateLimit
		}
		return nil, fmt.Errorf("authenticate: %w", err)
	}

	otpCode, err := s.authClient.GenerateLoginOTP(authCtx, req.Email)
	if err != nil {
		s.logger.Error("login: failed to generate otp", zap.String("email", req.Email), zap.Error(err))
		return nil, fmt.Errorf("failed to generate login verification code")
	}

	if err := s.emailSvc.SendLoginOTPEmail(ctx, req.Email, otpCode); err != nil {
		s.logger.Error("login: failed to send otp email", zap.String("email", req.Email), zap.Error(err))
		return nil, fmt.Errorf("failed to send login verification email")
	}

	user, err := s.resolveUserFromAuthIdentity(req.Email, userInfo)
	if err != nil {
		return nil, err
	}

	s.logger.Info("login: password verified, otp sent",
		zap.String("user_id", user.ID),
		zap.String("email", req.Email),
	)

	return &models.AuthResponse{
		User:        user,
		OTPRequired: true,
		Message:     "A login code has been sent to your email.",
	}, nil
}

func (s *AuthService) ForgotPassword(ctx context.Context, req *models.ForgotPasswordRequest) error {
	if err := validation.ValidateForgotPasswordRequest(req); err != nil {
		return err
	}

	user, err := s.userRepo.FindByEmail(req.Email)
	if err != nil && !errors.Is(err, serror.ErrUserNotFound) {
		return fmt.Errorf("find user: %w", err)
	}
	if user == nil {
		return nil // do not reveal whether the email exists
	}

	authCtx, cancel := timeoutContext(ctx)
	defer cancel()

	token, err := s.authClient.GeneratePasswordResetToken(authCtx, req.Email)
	if err != nil {
		s.logger.Error("forgot password: generate otp failed",
			zap.String("email", req.Email),
			zap.Error(err),
		)
		if errors.Is(err, sharedauth.ErrAuthInvalidEmail) {
			return ErrInvalidEmail
		}
		return nil // do not expose internal errors to caller
	}
	if token == "" {
		return nil
	}

	if err := s.emailSvc.SendPasswordResetEmail(ctx, req.Email, token); err != nil {
		s.logger.Error("forgot password: send email failed",
			zap.String("email", req.Email),
			zap.Error(err),
		)
		return fmt.Errorf("send password reset email: %w", err)
	}

	return nil
}

func (s *AuthService) ResetPassword(ctx context.Context, req *models.ResetPasswordRequest) error {
	authCtx, cancel := timeoutContext(ctx)
	defer cancel()

	if err := s.authClient.ResetPasswordWithOTP(authCtx, req.Token, req.Password); err != nil {
		switch {
		case errors.Is(err, sharedauth.ErrAuthInvalidToken):
			return ErrInvalidToken
		case errors.Is(err, sharedauth.ErrAuthExpiredToken):
			return ErrExpiredToken
		case errors.Is(err, sharedauth.ErrAuthRateLimit):
			return ErrRateLimit
		}
		return fmt.Errorf("reset password: %w", err)
	}

	return nil
}

func (s *AuthService) VerifyEmail(ctx context.Context, req *models.VerifyEmailRequest) (*models.AuthResponse, error) {
	authCtx, cancel := timeoutContext(ctx)
	defer cancel()

	accessToken, userInfo, err := s.authClient.VerifyEmailWithOTP(authCtx, req.Email, req.Token)
	if err != nil {
		switch {
		case errors.Is(err, sharedauth.ErrAuthInvalidToken):
			return nil, ErrInvalidToken
		case errors.Is(err, sharedauth.ErrAuthExpiredToken):
			return nil, ErrExpiredToken
		case errors.Is(err, sharedauth.ErrAuthRateLimit):
			return nil, ErrRateLimit
		}
		return nil, fmt.Errorf("verify email token: %w", err)
	}

	user, err := s.userRepo.FindByAuthUserID(userInfo.Sub)
	if err != nil && !errors.Is(err, serror.ErrUserNotFound) {
		return nil, fmt.Errorf("find user by auth ID: %w", err)
	}
	if user == nil {
		// Fallback to email lookup if auth ID mapping is missing
		user, err = s.userRepo.FindByEmail(req.Email)
		if err != nil {
			return nil, ErrInvalidEmail
		}
	}

	if err := s.userRepo.UpdateEmailVerified(user.ID, true); err != nil {
		return nil, fmt.Errorf("update verification status: %w", err)
	}
	if err := s.userRepo.UpdateLastLogin(user.ID); err != nil {
		s.logger.Warn("verify email: failed to update last login", zap.Error(err))
	}
	user.EmailVerified = true

	s.logger.Info("verify email: email verified",
		zap.String("user_id", user.ID),
	)

	name := ""
	if user.FullName != nil {
		name = *user.FullName
	}
	go func() {
		sendCtx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		if err := s.emailSvc.SendWelcomeEmail(sendCtx, user.Email, name); err != nil {
			s.logger.Error("verify email: failed to send welcome email",
				zap.String("user_id", user.ID),
				zap.Error(err),
			)
		}
	}()

	return &models.AuthResponse{
		Token:   accessToken,
		User:    user,
		Message: "Email verified and logged in.",
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	authCtx, cancel := timeoutContext(ctx)
	defer cancel()

	if err := s.authClient.Logout(authCtx, token); err != nil {
		s.logger.Error("logout: failed", zap.Error(err))
		return fmt.Errorf("logout: %w", err)
	}

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
		if err != nil && !errors.Is(err, serror.ErrUserNotFound) {
			return nil, fmt.Errorf("find user by email: %w", err)
		}
	}

	if user == nil && info != nil && strings.TrimSpace(info.Sub) != "" {
		sub := strings.TrimSpace(info.Sub)
		user, err = s.userRepo.FindByAuthUserID(sub)
		if err != nil && !errors.Is(err, serror.ErrUserNotFound) {
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

func (s *AuthService) ResendVerificationEmail(ctx context.Context, req *models.ResendVerificationEmailRequest) error {
	if err := validation.ValidateResendVerificationEmailRequest(req); err != nil {
		return err
	}

	email := normalizeEmail(req.Email)

	// Find user by email
	user, err := s.userRepo.FindByEmail(email)
	if err != nil {
		if errors.Is(err, serror.ErrUserNotFound) {
			// Don't reveal if email exists - return success anyway for security
			return nil
		}
		return fmt.Errorf("find user: %w", err)
	}

	// If already verified, silently succeed
	if user.EmailVerified {
		return nil
	}

	// Check if verification email already queued (prevent spam)
	alreadyQueued, err := s.outboxRepo.ExistsByReference(models.EventTypeSendVerificationEmail, user.ID)
	if err != nil {
		return fmt.Errorf("check existing verification email event: %w", err)
	}
	if alreadyQueued {
		// Already queued, don't create duplicate
		return nil
	}

	// Create encrypted payload
	payload, err := json.Marshal(map[string]string{"email": email})
	if err != nil {
		return fmt.Errorf("marshal email event payload: %w", err)
	}
	encrypted, err := utils.Encrypt(payload, s.encryptionKey)
	for i := range payload {
		payload[i] = 0 // Zero out plaintext
	}
	if err != nil {
		return fmt.Errorf("encrypt email event payload: %w", err)
	}

	// Queue verification email in outbox
	if err := s.outboxRepo.Create(&models.OutboxEvent{
		ID:          utils.GenerateID(),
		Type:        models.EventTypeSendVerificationEmail,
		Payload:     encrypted,
		Status:      models.OutboxStatusPending,
		MaxRetries:  5,
		NextRunAt:   time.Now(),
		ReferenceID: user.ID,
	}); err != nil {
		return fmt.Errorf("create verification email event: %w", err)
	}

	// Trigger outbox worker
	if s.outboxWorker != nil {
		s.outboxWorker.Trigger()
	}

	s.logger.Info("resend verification email queued",
		zap.String("user_id", user.ID),
		zap.String("email", email),
	)

	return nil
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
