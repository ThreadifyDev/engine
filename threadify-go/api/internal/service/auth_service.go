package service

import (
	"context"
	"encoding/hex"
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
	sharemodels "threadify-go/shared/models"
	sharedrepo "threadify-go/shared/repository"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

type DBPool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

func newSignupCreditAccount(companyID string, billingCycleStart time.Time, balanceMillicents, rateLimitTPS, payloadLimitBytes int64) *sharemodels.CreditAccount {
	return &sharemodels.CreditAccount{
		ID:                               uuid.NewString(),
		CompanyID:                        companyID,
		BillingCycleStart:                billingCycleStart,
		CreditBalanceMillicents:          balanceMillicents,
		CreditMinBalanceMillicents:       0,
		CreditMaxMonthlyChargeMillicents: sharemodels.CreditDisabled,
		CreditAutoTopupMillicents:        0,
		CreditMonthlyChargedMillicents:   0,
		RateLimitTPS:                     rateLimitTPS,
		PayloadLimitBytes:                payloadLimitBytes,
	}
}

type AuthService struct {
	pool           DBPool
	userRepo       repository.UserRepository
	companyRepo    repository.CompanyRepository
	userRoleRepo   repository.UserRoleRepository
	outboxRepo     repository.OutboxRepository
	invitationRepo repository.TeamInvitationRepository
	planRepo       sharedrepo.PlanRepository
	emailSvc       EmailService
	authClient     sharedauth.AuthClient
	jwksVerifier   *sharedauth.JWKSVerifier
	outboxWorker   OutboxWorkerTrigger
	encryptionKey  []byte
	logger         *zap.Logger

	signupCreditsMillicents int64
	signupRateLimitTPS      int64
	signupPayloadLimitBytes int64
}

func NewAuthService(
	pool DBPool,
	userRepo repository.UserRepository,
	companyRepo repository.CompanyRepository,
	userRoleRepo repository.UserRoleRepository,
	emailSvc EmailService,
	authClient sharedauth.AuthClient,
	outboxRepo repository.OutboxRepository,
	invitationRepo repository.TeamInvitationRepository,
	outboxWorker OutboxWorkerTrigger,
	encryptionKey string,
	logger *zap.Logger,
) *AuthService {
	key, err := hex.DecodeString(encryptionKey)
	if err != nil {
		logger.Error("failed to decode outbox encryption key", zap.Error(err))
		key = []byte(encryptionKey)
	}

	return &AuthService{
		pool:           pool,
		userRepo:       userRepo,
		companyRepo:    companyRepo,
		userRoleRepo:   userRoleRepo,
		outboxRepo:     outboxRepo,
		invitationRepo: invitationRepo,
		emailSvc:       emailSvc,
		authClient:     authClient,
		outboxWorker:   outboxWorker,
		encryptionKey:  key,
		logger:         logger,
	}
}

func (s *AuthService) SetJWKSVerifier(verifier *sharedauth.JWKSVerifier) {
	s.jwksVerifier = verifier
}

func (s *AuthService) ConfigureSignupCredits(planRepo sharedrepo.PlanRepository, signupCreditsMillicents, rateLimitTPS, payloadLimitBytes int64) {
	s.planRepo = planRepo
	s.signupCreditsMillicents = signupCreditsMillicents
	s.signupRateLimitTPS = rateLimitTPS
	s.signupPayloadLimitBytes = payloadLimitBytes
}

func (s *AuthService) Signup(ctx context.Context, req *models.SignupRequest) error {
	if err := validation.ValidateSignupRequest(req); err != nil {
		return err
	}

	company, invitation, userRole, err := s.resolveSignupContext(ctx, req)
	if err != nil {
		return err
	}

	user := buildUser(req, company.ID)

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

func (s *AuthService) resolveSignupContext(ctx context.Context, req *models.SignupRequest) (*models.Company, *models.TeamInvitation, string, error) {
	if req.InvitationToken != nil && *req.InvitationToken != "" {
		return s.resolveInvitationSignup(ctx, req)
	}
	return s.resolveRegularSignup(ctx, req)
}

func (s *AuthService) resolveInvitationSignup(ctx context.Context, req *models.SignupRequest) (*models.Company, *models.TeamInvitation, string, error) {
	inv, err := s.invitationRepo.GetByToken(ctx, *req.InvitationToken)
	if err != nil {
		s.logger.Error("signup: failed to get invitation", zap.Error(err))
		return nil, nil, "", fmt.Errorf("invalid invitation token")
	}
	if inv == nil {
		return nil, nil, "", fmt.Errorf("invitation not found")
	}
	if inv.Status != "pending" {
		return nil, nil, "", fmt.Errorf("invitation already used or expired")
	}
	if time.Now().After(inv.ExpiresAt) {
		return nil, nil, "", fmt.Errorf("invitation has expired")
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

func (s *AuthService) resolveRegularSignup(ctx context.Context, req *models.SignupRequest) (*models.Company, *models.TeamInvitation, string, error) {
	if err := s.checkUserExists(ctx, req.Email); err != nil {
		return nil, nil, "", err
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

	return company, nil, "owner", nil
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
	user *models.User,
	company *models.Company,
	invitation *models.TeamInvitation,
	userRole string,
	outboxEvent *models.OutboxEvent,
) error {
	tx, err := s.pool.Begin(ctx)
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

func buildUser(req *models.SignupRequest, companyID string) *models.User {
	now := time.Now()
	return &models.User{
		ID:        utils.GenerateID(),
		CompanyID: companyID,
		Email:     req.Email,
		FullName:  normalizeOptionalString(req.FullName),
		JobRole:   normalizeOptionalString(req.JobRole),
		CreatedAt: now,
		UpdatedAt: now,
	}
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
	if err != nil {
		for i := range payload {
			payload[i] = 0
		}
		return nil, fmt.Errorf("encrypt signup event payload: %w", err)
	}
	for i := range payload {
		payload[i] = 0
	}

	return &models.OutboxEvent{
		ID:          utils.GenerateID(),
		Type:        models.EventTypeRegisterAuthUser,
		Payload:     encrypted,
		Status:      models.OutboxStatusPending,
		MaxRetries:  models.OutboxDefaultMaxRetries,
		NextRunAt:   time.Now(),
		ReferenceID: user.ID,
	}, nil
}

func (s *AuthService) Login(ctx context.Context, req *models.LoginRequest, clientIP string) (*models.AuthResponse, error) {
	if err := validation.ValidateLoginRequest(req); err != nil {
		return nil, err
	}

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

func (s *AuthService) resolveLocalUser(ctx context.Context, email string) (*models.User, error) {
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

func (s *AuthService) handleUnverifiedUser(ctx context.Context, user *models.User) (*models.AuthResponse, error) {
	s.logger.Debug("login: email not verified; re-queuing verification email",
		zap.String("user_id", user.ID),
	)
	if err := s.queueVerificationEmail(ctx, user.ID, user.Email); err != nil {
		s.logger.Error("login: failed to queue verification email", zap.Error(err))
		return nil, fmt.Errorf("login: queue verification email: %w", err)
	}
	return &models.AuthResponse{
		Email:                     user.Email,
		OTPRequired:               true,
		EmailVerificationRequired: true,
		Message:                   "Please verify your email. A verification code has been sent to your email.",
	}, nil
}

func (s *AuthService) handleAuthError(
	ctx context.Context,
	authErr error,
	req *models.LoginRequest,
	localUser *models.User,
) (*models.AuthResponse, error) {
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

func (s *AuthService) handleLegacyLogin(ctx context.Context, req *models.LoginRequest, localUser *models.User) (*models.AuthResponse, error) {
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

func (s *AuthService) clearStaleLegacyHash(ctx context.Context, localUser *models.User) error {
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

func (s *AuthService) issueLoginOTP(ctx context.Context, email string) (*models.AuthResponse, error) {
	code, err := s.authClient.GenerateLoginOTP(ctx, email)
	if err != nil {
		s.logger.Error("login: failed to generate OTP", zap.Error(err))
		return nil, fmt.Errorf("login: generate OTP: %w", err)
	}

	if err := s.emailSvc.SendLoginOTPEmail(ctx, email, code); err != nil {
		s.logger.Error("login: failed to send OTP email", zap.Error(err))
		return nil, fmt.Errorf("login: send OTP email: %w", err)
	}

	return &models.AuthResponse{
		Email:       email,
		OTPRequired: true,
		Message:     "A login code has been sent to your email.",
	}, nil
}

func isLegacyUser(u *models.User) bool {
	return u != nil && (u.AuthUserID == nil || strings.TrimSpace(*u.AuthUserID) == "")
}

// @TODO - We should switch this to resetPassword as forgotPassword should not be resetting the user's account
func (s *AuthService) ForgotPassword(ctx context.Context, req *models.ForgotPasswordRequest) error {
	if err := validation.ValidateForgotPasswordRequest(req); err != nil {
		return err
	}

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

func (s *AuthService) handleLegacyForgotPassword(ctx context.Context, user *models.User) error {
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

func (s *AuthService) ResetPassword(ctx context.Context, req *models.ResetPasswordRequest) error {
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

func (s *AuthService) VerifyEmail(ctx context.Context, req *models.VerifyEmailRequest) (*models.AuthResponse, error) {
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
		if err := s.provisionSignupCredits(ctx, user.CompanyID); err != nil {
			return nil, fmt.Errorf("provision signup credits: %w", err)
		}
		s.sendWelcomeEmailAsync(user)
	}

	return &models.AuthResponse{
		Token:   accessToken,
		User:    user,
		Message: "Email verified and logged in.",
	}, nil
}

func (s *AuthService) provisionSignupCredits(ctx context.Context, companyID string) error {
	if s.planRepo == nil || strings.TrimSpace(companyID) == "" {
		return nil
	}
	if s.signupCreditsMillicents <= 0 && s.signupRateLimitTPS <= 0 && s.signupPayloadLimitBytes <= 0 {
		return nil
	}

	account, err := s.planRepo.GetCreditAccount(ctx, companyID)
	if err != nil {
		return fmt.Errorf("get credit account: %w", err)
	}
	if account != nil {
		return nil
	}

	start := time.Now().UTC().Truncate(24 * time.Hour)
	newAccount := newSignupCreditAccount(
		companyID,
		start,
		s.signupCreditsMillicents,
		s.signupRateLimitTPS,
		s.signupPayloadLimitBytes,
	)

	if err := s.planRepo.CreateCreditAccount(ctx, newAccount); err != nil {
		if errors.Is(err, serror.ErrDuplicateCreditAccount) {
			return nil
		}
		return fmt.Errorf("create credit account: %w", err)
	}

	s.logger.Info("signup credits provisioned",
		zap.String("company_id", companyID),
		zap.Int64("amount_millicents", s.signupCreditsMillicents),
	)

	return nil
}

func (s *AuthService) resolveVerifiedUser(ctx context.Context, sub, email string) (*models.User, error) {
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

func (s *AuthService) sendWelcomeEmailAsync(user *models.User) {
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

func (s *AuthService) resolveUserFromAuthIdentity(ctx context.Context, emailHint string, info *sharedauth.AuthUserInfo) (*models.User, error) {
	email := normalizeEmail(emailHint)
	if info != nil && strings.TrimSpace(info.Email) != "" {
		email = normalizeEmail(info.Email)
	}

	var user *models.User
	var err error

	if email != "" {
		user, err = s.userRepo.FindByEmail(ctx, email)
		if err != nil && !errors.Is(err, serror.ErrUserNotFound) {
			return nil, fmt.Errorf("find user by email: %w", err)
		}
	}

	if user == nil && info != nil && strings.TrimSpace(info.Sub) != "" {
		sub := strings.TrimSpace(info.Sub)
		user, err = s.userRepo.FindByAuthUserID(ctx, sub)
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
		if err := s.userRepo.UpdateAuthUserID(ctx, user.ID, sub); err == nil {
			user.AuthUserID = &sub
		}
	}

	return user, nil
}

func (s *AuthService) queueVerificationEmail(ctx context.Context, userID, email string) error {
	email = normalizeEmail(email)

	inflight, err := s.outboxRepo.ExistsPendingByReference(ctx, models.EventTypeSendVerificationEmail, userID)
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

	if err := s.outboxRepo.Create(ctx, &models.OutboxEvent{
		ID:          utils.GenerateID(),
		Type:        models.EventTypeSendVerificationEmail,
		Payload:     encrypted,
		Status:      models.OutboxStatusPending,
		MaxRetries:  models.OutboxDefaultMaxRetries,
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

func (s *AuthService) ResendVerificationEmail(ctx context.Context, req *models.ResendVerificationEmailRequest) error {
	if err := validation.ValidateResendVerificationEmailRequest(req); err != nil {
		return err
	}

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

	alreadyQueued, err := s.outboxRepo.ExistsByReference(ctx, models.EventTypeSendVerificationEmail, user.ID)
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
