package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"threadify-go/api/internal/domain"
	"threadify-go/api/internal/utils"
	sharedauth "threadify-go/shared/auth"
	serror "threadify-go/shared/errors"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

const (
	userRole         = "user"
	standardRole     = "standard"
	operationTimeout = 10 * time.Second
)

type OutboxWorkerTrigger interface {
	Trigger()
}

type billingService interface {
	ProvisionSignupCredits(ctx context.Context, companyID string) error
}

type AuthService struct {
	txManager      domain.TxManager
	userRepo       domain.UserRepository
	companyRepo    domain.CompanyRepository
	userRoleRepo   domain.UserRoleRepository
	outboxRepo     domain.OutboxRepository
	invitationRepo domain.TeamInvitationRepository
	emailSvc       EmailService
	authClient     sharedauth.AuthClient
	jwksVerifier   sharedauth.TokenVerifier
	billingService billingService
	outboxWorker   OutboxWorkerTrigger
	encryptionKey  []byte
	logger         *zap.Logger
}

func NewAuthService(
	txManager domain.TxManager,
	userRepo domain.UserRepository,
	companyRepo domain.CompanyRepository,
	userRoleRepo domain.UserRoleRepository,
	emailSvc EmailService,
	authClient sharedauth.AuthClient,
	jwksVerifier sharedauth.TokenVerifier,
	billingSvc billingService,
	outboxRepo domain.OutboxRepository,
	invitationRepo domain.TeamInvitationRepository,
	outboxWorker OutboxWorkerTrigger,
	encryptionKey []byte,
	logger *zap.Logger,
) *AuthService {
	return &AuthService{
		txManager:      txManager,
		userRepo:       userRepo,
		companyRepo:    companyRepo,
		userRoleRepo:   userRoleRepo,
		outboxRepo:     outboxRepo,
		invitationRepo: invitationRepo,
		emailSvc:       emailSvc,
		authClient:     authClient,
		jwksVerifier:   jwksVerifier,
		billingService: billingSvc,
		outboxWorker:   outboxWorker,
		encryptionKey:  encryptionKey,
		logger:         logger,
	}
}

func (s *AuthService) Signup(ctx context.Context, req *domain.SignupCmd) error {
	company, invitation, userRole, err := s.resolveSignupContext(ctx, req)
	if err != nil {
		return err
	}

	user := buildUser(req, company.ID, invitation)

	outboxEvent, err := s.buildRegisterAuthUserEvent(user, company, req.Password, req.FullName)
	if err != nil {
		s.logger.Error("signup: failed to build auth user event", zap.Error(err))
		return err
	}

	if err := s.persistSignup(ctx, user, company, invitation, userRole, outboxEvent); err != nil {
		return err
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

func (s *AuthService) resolveSignupContext(ctx context.Context, req *domain.SignupCmd) (*domain.Company, *domain.TeamInvitation, string, error) {
	if req.InvitationToken != nil && *req.InvitationToken != "" {
		return s.resolveInvitationSignup(ctx, req)
	}
	return s.resolveRegularSignup(ctx, req)
}

func (s *AuthService) resolveInvitationSignup(ctx context.Context, req *domain.SignupCmd) (*domain.Company, *domain.TeamInvitation, string, error) {
	inv, err := s.invitationRepo.GetByToken(ctx, *req.InvitationToken)
	if err != nil {
		s.logger.Error("signup: failed to get invitation", zap.Error(err))
		return nil, nil, "", fmt.Errorf("invalid invitation token")
	}
	if inv == nil {
		return nil, nil, "", fmt.Errorf("invitation not found")
	}
	if err := inv.CanBeAccepted(); err != nil {
		return nil, nil, "", err
	}

	company, err := s.companyRepo.FindByID(ctx, inv.CompanyID)
	if err != nil {
		s.logger.Error("signup: failed to get company for invitation", zap.Error(err))
		return nil, nil, "", fmt.Errorf("company not found")
	}

	req.Email = inv.Email

	if err := s.checkUserExists(ctx, req.Email); err != nil {
		return nil, nil, "", err
	}

	return company, inv, inv.Role, nil
}

func (s *AuthService) resolveRegularSignup(ctx context.Context, req *domain.SignupCmd) (*domain.Company, *domain.TeamInvitation, string, error) {
	if err := s.checkUserExists(ctx, req.Email); err != nil {
		return nil, nil, "", err
	}

	now := time.Now()
	company := &domain.Company{
		ID:        utils.GenerateID(),
		Name:      strings.TrimSpace(req.CompanyName),
		Industry:  req.Industry,
		Size:      req.CompanySize,
		UseCase:   req.UseCase,
		CreatedAt: now,
		UpdatedAt: now,
	}

	return company, nil, "admin", nil
}

func (s *AuthService) checkUserExists(ctx context.Context, email string) error {
	existing, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil && !errors.Is(err, serror.ErrUserNotFound) {
		s.logger.Error("signup: failed to check existing user", zap.Error(err))
		return fmt.Errorf("check existing user: %w", err)
	}
	if existing != nil {
		s.logger.Warn("signup: user already exists", zap.String("email", email))
		return ErrUserAlreadyExists
	}
	return nil
}

func (s *AuthService) persistSignup(
	ctx context.Context,
	user *domain.User,
	company *domain.Company,
	invitation *domain.TeamInvitation,
	userRole string,
	outboxEvent *domain.OutboxEvent,
) error {
	tx, err := s.txManager.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if invitation == nil {
		if err := s.companyRepo.CreateTx(ctx, tx, company); err != nil {
			return fmt.Errorf("create company: %w", err)
		}
	}

	if err := s.userRepo.CreateTx(ctx, tx, user); err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	if err := s.userRoleRepo.AssignRoleToUserTx(ctx, tx, user.ID, userRole, "system"); err != nil {
		return fmt.Errorf("assign default role: %w", err)
	}
	if err := s.outboxRepo.CreateTx(ctx, tx, outboxEvent); err != nil {
		return fmt.Errorf("create outbox event: %w", err)
	}

	if invitation != nil {
		if err := s.invitationRepo.DeleteTx(ctx, tx, invitation.ID); err != nil {
			return fmt.Errorf("delete invitation: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func buildUser(req *domain.SignupCmd, companyID string, invitation *domain.TeamInvitation) *domain.User {

	now := time.Now()
	user := &domain.User{
		ID:        utils.GenerateID(),
		CompanyID: companyID,
		Email:     req.Email,
		FullName:  normalizeOptionalString(req.FullName),
		JobRole:   normalizeOptionalString(req.JobRole),
		CreatedAt: now,
		UpdatedAt: now,
	}

	// For invited users, skip the getting-started flow since
	// the company already has API keys and is fully instrumented.
	if invitation != nil {
		user.FirstInstrumentationDone = true
	}

	return user
}

func (s *AuthService) buildRegisterAuthUserEvent(
	user *domain.User,
	company *domain.Company,
	password, fullName string,
) (*domain.OutboxEvent, error) {
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
	if err != nil {
		for i := range payload {
			payload[i] = 0
		}
		return nil, fmt.Errorf("encrypt signup event payload: %w", err)
	}
	for i := range payload {
		payload[i] = 0
	}

	return &domain.OutboxEvent{
		ID:          utils.GenerateID(),
		Type:        domain.EventTypeRegisterAuthUser,
		Payload:     encrypted,
		Status:      domain.OutboxStatusPending,
		MaxRetries:  domain.OutboxDefaultMaxRetries,
		NextRunAt:   time.Now(),
		ReferenceID: user.ID,
	}, nil
}

func (s *AuthService) Login(ctx context.Context, req *domain.LoginCmd, clientIP string) (*domain.AuthSession, error) {
	localUser, err := s.resolveLocalUser(ctx, req.Email)
	if err != nil {
		return nil, err
	}

	if localUser != nil && !localUser.EmailVerified {
		return s.handleUnverifiedUser(ctx, localUser)
	}

	if isLegacyUser(localUser) {
		return s.handleLegacyLogin(ctx, req, localUser)
	}

	_, userInfo, authErr := s.authClient.LoginWithPassword(ctx, req.Email, req.Password, clientIP)
	if authErr != nil {
		return s.handleAuthError(ctx, authErr, req, localUser)
	}

	_ = s.clearStaleLegacyHash(ctx, localUser)

	return s.issueLoginOTP(ctx, userInfo.Email)
}

func (s *AuthService) resolveLocalUser(ctx context.Context, email string) (*domain.User, error) {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, serror.ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		s.logger.Error("login: failed to find user by email", zap.Error(err))
		return nil, ErrInternalServerError
	}
	return user, nil
}

func (s *AuthService) handleUnverifiedUser(ctx context.Context, user *domain.User) (*domain.AuthSession, error) {
	s.logger.Debug("login: email not verified; re-queuing verification email",
		zap.String("user_id", user.ID),
	)
	if err := s.queueVerificationEmail(ctx, user.ID, user.Email); err != nil {
		s.logger.Error("login: failed to queue verification email", zap.Error(err))
		return nil, fmt.Errorf("login: queue verification email: %w", err)
	}
	return &domain.AuthSession{
		Email:                     user.Email,
		OTPRequired:               true,
		EmailVerificationRequired: true,
		Message:                   "Please verify your email. A verification code has been sent to your email.",
	}, nil
}

func (s *AuthService) handleAuthError(
	ctx context.Context,
	authErr error,
	req *domain.LoginCmd,
	localUser *domain.User,
) (*domain.AuthSession, error) {
	switch {
	case errors.Is(authErr, sharedauth.ErrAuthInvalidCredentials):
		return nil, ErrInvalidCredentials
	case errors.Is(authErr, sharedauth.ErrAuthRateLimit):
		return nil, ErrRateLimit
	default:
		s.logger.Error("login: unexpected auth client error",
			zap.Error(authErr),
			zap.String("error_type", fmt.Sprintf("%T", authErr)),
		)
		return nil, fmt.Errorf("login: authenticate: %w", authErr)
	}
}

func (s *AuthService) handleLegacyLogin(ctx context.Context, req *domain.LoginCmd, localUser *domain.User) (*domain.AuthSession, error) {
	s.logger.Info("login: legacy user detected, verifying stored password hash",
		zap.String("user_id", localUser.ID),
	)

	if err := s.verifyLegacyPassword(ctx, req.Email, req.Password); err != nil {
		return nil, err
	}

	if err := s.queueLegacyUserMigration(ctx, localUser.ID, req.Email, req.Password, "login"); err != nil {
		s.logger.Error("login: failed to queue Supabase migration for legacy user",
			zap.Error(err),
			zap.String("user_id", localUser.ID),
		)
	}

	return s.issueLoginOTP(ctx, req.Email)
}

func (s *AuthService) verifyLegacyPassword(ctx context.Context, email, password string) error {
	hash, err := s.userRepo.GetPasswordHash(ctx, email)
	if err != nil {
		s.logger.Error("login: failed to fetch password hash for legacy user", zap.Error(err))
		return ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}

func (s *AuthService) clearStaleLegacyHash(ctx context.Context, localUser *domain.User) error {
	if localUser == nil {
		return nil
	}
	if localUser.AuthUserID != nil && strings.TrimSpace(*localUser.AuthUserID) != "" {
		return nil
	}

	hash, err := s.userRepo.GetPasswordHash(ctx, localUser.Email)
	if err != nil || hash == "" {
		return nil
	}

	if err := s.userRepo.ClearPasswordHash(ctx, localUser.ID); err != nil {
		s.logger.Error("login: failed to clear stale legacy password hash",
			zap.Error(err),
			zap.String("user_id", localUser.ID),
		)
		return err
	}

	s.logger.Debug("login: cleared stale legacy password hash",
		zap.String("user_id", localUser.ID),
	)
	return nil
}

func (s *AuthService) issueLoginOTP(ctx context.Context, email string) (*domain.AuthSession, error) {
	code, err := s.authClient.GenerateLoginOTP(ctx, email)
	if err != nil {
		s.logger.Error("login: failed to generate OTP", zap.Error(err))
		return nil, fmt.Errorf("login: generate OTP: %w", err)
	}

	if err := s.emailSvc.SendLoginOTPEmail(ctx, email, code); err != nil {
		s.logger.Error("login: failed to send OTP email", zap.Error(err))
		return nil, fmt.Errorf("login: send OTP email: %w", err)
	}

	return &domain.AuthSession{
		Email:       email,
		OTPRequired: true,
		Message:     "A login code has been sent to your email.",
	}, nil
}

func isLegacyUser(u *domain.User) bool {
	return u != nil && (u.AuthUserID == nil || strings.TrimSpace(*u.AuthUserID) == "")
}

// @TODO - We should switch this to resetPassword as forgotPassword should not be resetting the user's account
func (s *AuthService) ForgotPassword(ctx context.Context, req *domain.ForgotPasswordCmd) error {
	user, err := s.userRepo.FindByEmail(ctx, req.Email)
	if err != nil && !errors.Is(err, serror.ErrUserNotFound) {
		return fmt.Errorf("find user: %w", err)
	}
	if user == nil {
		return nil // silently succeed — do not leak user existence
	}

	if user.AuthUserID == nil || strings.TrimSpace(*user.AuthUserID) == "" {
		return s.handleLegacyForgotPassword(ctx, user)
	}

	return s.issueForgotPasswordReset(ctx, req.Email)
}

func (s *AuthService) handleLegacyForgotPassword(ctx context.Context, user *domain.User) error {
	s.logger.Debug("forgot password: legacy user detected, queuing migration",
		zap.String("user_id", user.ID),
	)

	if err := s.queueLegacyUserMigration(ctx, user.ID, user.Email, generateSecurePassword(), "forgot_password"); err != nil {
		s.logger.Error("forgot password: failed to queue migration",
			zap.Error(err),
			zap.String("user_id", user.ID),
		)
		return fmt.Errorf("failed to process password reset request")
	}

	s.logger.Debug("forgot password: migration queued, reset email will follow",
		zap.String("user_id", user.ID),
	)
	return nil
}

func (s *AuthService) issueForgotPasswordReset(ctx context.Context, email string) error {
	token, err := s.authClient.GeneratePasswordResetToken(ctx, email)
	if err != nil {
		s.logger.Error("forgot password: failed to generate reset token", zap.Error(err))
		if errors.Is(err, sharedauth.ErrAuthInvalidEmail) {
			return ErrInvalidEmail
		}
		return nil
	}
	if token == "" {
		return nil
	}

	if err := s.emailSvc.SendPasswordResetEmail(ctx, email, token); err != nil {
		s.logger.Error("forgot password: failed to send reset email", zap.Error(err))
		return fmt.Errorf("send password reset email: %w", err)
	}

	return nil
}

// generateSecurePassword generates a cryptographically secure random password
func generateSecurePassword() string {
	// Use two UUIDs for 64 characters of randomness
	return utils.GenerateID() + utils.GenerateID()
}

func (s *AuthService) ResetPassword(ctx context.Context, req *domain.ResetPasswordCmd) error {
	if err := s.authClient.ResetPasswordWithOTP(ctx, req.Token, req.Password); err != nil {
		switch {
		case errors.Is(err, sharedauth.ErrAuthInvalidToken):
			return ErrAuthInvalidToken
		case errors.Is(err, sharedauth.ErrAuthExpiredToken):
			return ErrExpiredToken
		case errors.Is(err, sharedauth.ErrAuthRateLimit):
			return ErrRateLimit
		}
		return fmt.Errorf("reset password: %w", err)
	}

	return nil
}

func (s *AuthService) VerifyEmail(ctx context.Context, req *domain.VerifyEmailCmd) (*domain.AuthSession, error) {
	accessToken, userInfo, err := s.authClient.VerifyEmailWithOTP(ctx, req.Email, req.Token)
	if err != nil {
		switch {
		case errors.Is(err, sharedauth.ErrAuthInvalidToken):
			return nil, ErrAuthInvalidToken
		case errors.Is(err, sharedauth.ErrAuthExpiredToken):
			return nil, ErrExpiredToken
		case errors.Is(err, sharedauth.ErrAuthRateLimit):
			return nil, ErrRateLimit
		}
		return nil, fmt.Errorf("verify email token: %w", err)
	}

	user, err := s.resolveVerifiedUser(ctx, userInfo.Sub, req.Email)
	if err != nil {
		return nil, err
	}

	wasAlreadyVerified := user.EmailVerified

	if err := s.userRepo.UpdateEmailVerified(ctx, user.ID, true); err != nil {
		return nil, fmt.Errorf("update verification status: %w", err)
	}
	if err := s.userRepo.UpdateLastLogin(ctx, user.ID); err != nil {
		s.logger.Warn("verify email: failed to update last login", zap.Error(err))
	}
	user.EmailVerified = true

	s.logger.Info("verify email: email verified", zap.String("user_id", user.ID))

	if !wasAlreadyVerified {
		if err := s.billingService.ProvisionSignupCredits(ctx, user.CompanyID); err != nil {
			return nil, fmt.Errorf("provision signup credits: %w", err)
		}
		s.sendWelcomeEmailAsync(user)
	}

	return &domain.AuthSession{
		Token:   accessToken,
		User:    user,
		Message: "Email verified and logged in.",
	}, nil
}

func (s *AuthService) resolveVerifiedUser(ctx context.Context, sub, email string) (*domain.User, error) {
	user, err := s.userRepo.FindByAuthUserID(ctx, sub)
	if err != nil && !errors.Is(err, serror.ErrUserNotFound) {
		return nil, fmt.Errorf("find user by auth ID: %w", err)
	}
	if user != nil {
		return user, nil
	}

	user, err = s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, ErrInvalidEmail
	}
	return user, nil
}

func (s *AuthService) sendWelcomeEmailAsync(user *domain.User) {
	var name string
	if user.FullName != nil {
		name = *user.FullName
	}
	userID := user.ID
	email := user.Email
	go func() {
		sendCtx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		if err := s.emailSvc.SendWelcomeEmail(sendCtx, email, name); err != nil {
			s.logger.Error("verify email: failed to send welcome email",
				zap.String("user_id", userID),
				zap.Error(err),
			)
		}
	}()
}

func (s *AuthService) Logout(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	if err := s.authClient.Logout(ctx, token); err != nil {
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

func (s *AuthService) GetUserRoles(ctx context.Context, userID, principalType string) ([]string, error) {
	switch principalType {
	case userRole:
		return s.userRoleRepo.GetUserRoles(ctx, userID)
	case standardRole:
		return s.userRoleRepo.GetServiceAccountRoles(ctx, userID)
	default:
		return nil, fmt.Errorf("invalid principal type: %s", principalType)
	}
}

func (s *AuthService) queueVerificationEmail(ctx context.Context, userID, email string) error {
	email = normalizeEmail(email)

	inflight, err := s.outboxRepo.ExistsPendingByReference(ctx, domain.EventTypeSendVerificationEmail, userID)
	if err != nil {
		return fmt.Errorf("check inflight verification email: %w", err)
	}
	if inflight {
		return nil
	}

	payload, err := json.Marshal(map[string]string{"email": email})
	if err != nil {
		return fmt.Errorf("marshal email event payload: %w", err)
	}
	encrypted, err := utils.Encrypt(payload, s.encryptionKey)
	if err != nil {
		for i := range payload {
			payload[i] = 0
		}
		return fmt.Errorf("encrypt email event payload: %w", err)
	}
	for i := range payload {
		payload[i] = 0
	}

	if err := s.outboxRepo.Create(ctx, &domain.OutboxEvent{
		ID:          utils.GenerateID(),
		Type:        domain.EventTypeSendVerificationEmail,
		Payload:     encrypted,
		Status:      domain.OutboxStatusPending,
		MaxRetries:  domain.OutboxDefaultMaxRetries,
		NextRunAt:   time.Now(),
		ReferenceID: userID,
	}); err != nil {
		return fmt.Errorf("create verification email event: %w", err)
	}

	if s.outboxWorker != nil {
		s.outboxWorker.Trigger()
	}

	s.logger.Debug("auth: verification email queued via outbox",
		zap.String("user_id", userID),
	)
	return nil
}

func (s *AuthService) ResendVerificationEmail(ctx context.Context, req *domain.ResendVerificationEmailCmd) error {
	email := normalizeEmail(req.Email)

	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, serror.ErrUserNotFound) {
			return nil
		}
		return fmt.Errorf("find user: %w", err)
	}

	if user.EmailVerified {
		return nil
	}

	alreadyQueued, err := s.outboxRepo.ExistsByReference(ctx, domain.EventTypeSendVerificationEmail, user.ID)
	if err != nil {
		return fmt.Errorf("check existing verification email event: %w", err)
	}
	if alreadyQueued {
		return nil
	}

	if err := s.queueVerificationEmail(ctx, user.ID, email); err != nil {
		return err
	}

	s.logger.Debug("resend verification email queued",
		zap.String("user_id", user.ID),
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
