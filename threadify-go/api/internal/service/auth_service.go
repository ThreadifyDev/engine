package service

import (
	"context"
	"database/sql"
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

type AuthService struct {
	db             *sql.DB
	userRepo       *repository.UserRepository
	companyRepo    *repository.CompanyRepository
	userRoleRepo   *repository.UserRoleRepository
	outboxRepo     *repository.OutboxRepository
	invitationRepo *repository.TeamInvitationRepository
	emailSvc       *EmailService
	authClient     sharedauth.AuthClient
	jwksVerifier   *sharedauth.JWKSVerifier
	outboxWorker   OutboxWorkerTrigger
	encryptionKey  []byte
	logger         *zap.Logger
}

func NewAuthService(
	db *sql.DB,
	emailSvc *EmailService,
	authClient sharedauth.AuthClient,
	outboxRepo *repository.OutboxRepository,
	invitationRepo *repository.TeamInvitationRepository,
	outboxWorker OutboxWorkerTrigger,
	encryptionKey string,
	logger *zap.Logger,
) *AuthService {
	key, err := hex.DecodeString(encryptionKey)
	if err != nil {
		logger.Error("failed to decode outbox encryption key", zap.Error(err))
		// We fallback to raw bytes if hex decoding fails, but in production, this should be valid hex.
		key = []byte(encryptionKey)
	}

	return &AuthService{
		db:             db,
		userRepo:       repository.NewUserRepository(db),
		companyRepo:    repository.NewCompanyRepository(db),
		userRoleRepo:   repository.NewUserRoleRepository(db),
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

func (s *AuthService) Signup(ctx context.Context, req *models.SignupRequest) error {
	if err := validation.ValidateSignupRequest(req); err != nil {
		return err
	}

	now := time.Now()
	var company *models.Company
	var invitation *models.TeamInvitation
	var userRole string = "standard_account"

	// Check if signing up via invitation
	if req.InvitationToken != nil && *req.InvitationToken != "" {
		// Validate and get invitation
		inv, err := s.invitationRepo.GetByToken(*req.InvitationToken)
		if err != nil {
			s.logger.Error("failed to get invitation", zap.Error(err))
			return fmt.Errorf("invalid invitation token")
		}
		if inv == nil {
			return fmt.Errorf("invitation not found")
		}
		if inv.Status != "pending" {
			return fmt.Errorf("invitation already used or expired")
		}
		if time.Now().After(inv.ExpiresAt) {
			return fmt.Errorf("invitation has expired")
		}

		// Get existing company
		company, err = s.companyRepo.FindByID(inv.CompanyID)
		if err != nil {
			s.logger.Error("failed to get company", zap.Error(err))
			return fmt.Errorf("company not found")
		}

		invitation = inv
		userRole = inv.Role

		// Override email from invitation token (don't trust frontend)
		req.Email = inv.Email

		// Check if user already exists (using email from invitation)
		existing, err := s.userRepo.FindByEmail(req.Email)
		if err != nil && !errors.Is(err, serror.ErrUserNotFound) {
			s.logger.Error("failed to check existing user", zap.Error(err))
			return fmt.Errorf("check existing user: %w", err)
		}
		if existing != nil {
			s.logger.Warn("user already exists")
			return ErrUserAlreadyExists
		}
	} else {
		// Regular signup - check if user already exists
		existing, err := s.userRepo.FindByEmail(req.Email)
		if err != nil && !errors.Is(err, serror.ErrUserNotFound) {
			s.logger.Error("failed to check existing user", zap.Error(err))
			return fmt.Errorf("check existing user: %w", err)
		}
		if existing != nil {
			s.logger.Warn("user already exists")
			return ErrUserAlreadyExists
		}

		// Creating new company
		company = &models.Company{
			ID:        utils.GenerateID(),
			Name:      strings.TrimSpace(req.CompanyName),
			Industry:  req.Industry,
			Size:      req.CompanySize,
			UseCase:   req.UseCase,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}

	// New user signup (either regular or invitation-based)
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
		s.logger.Error("failed to register user", zap.Error(err))
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Only create company if not joining via invitation
	if invitation == nil {
		if err := s.companyRepo.CreateTx(tx, company); err != nil {
			return fmt.Errorf("create company: %w", err)
		}
	}

	if err := s.userRepo.CreateTx(tx, user); err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	if err := s.userRoleRepo.AssignRoleToUserTx(tx, user.ID, userRole, "system"); err != nil {
		return fmt.Errorf("assign default role: %w", err)
	}
	if err := s.outboxRepo.CreateTx(tx, outboxEvent); err != nil {
		return fmt.Errorf("create outbox event: %w", err)
	}

	// Delete invitation after successful signup
	if invitation != nil {
		if err := s.invitationRepo.DeleteTx(tx, invitation.ID); err != nil {
			return fmt.Errorf("delete invitation: %w", err)
		}
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

	// Get local user first (needed for both legacy check and password_hash cleanup)
	localUser, dbErr := s.userRepo.FindByEmail(req.Email)
	if dbErr != nil && !errors.Is(dbErr, serror.ErrUserNotFound) {
		s.logger.Error("login: failed to find user", zap.Error(dbErr))
		return nil, fmt.Errorf("failed to authenticate")
	}

	_, userInfo, err := s.authClient.LoginWithPassword(ctx, req.Email, req.Password, clientIP)
	if err != nil {
		// Check if this is an "invalid credentials" error - could be user doesn't exist in Supabase
		if errors.Is(err, sharedauth.ErrAuthInvalidCredentials) {
			// Check if user exists in local DB (legacy user)
			if localUser != nil && (localUser.AuthUserID == nil || strings.TrimSpace(*localUser.AuthUserID) == "") {
				// Legacy user found - verify password and queue migration
				s.logger.Info("login: legacy user detected, verifying password")

				// Get stored password hash
				passwordHash, err := s.userRepo.GetPasswordHash(req.Email)
				if err != nil {
					s.logger.Error("login: failed to get password hash for legacy user", zap.Error(err))
					return nil, ErrInvalidCredentials
				}

				// Verify password against stored hash
				if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
					// Password doesn't match
					return nil, ErrInvalidCredentials
				}

				// Password verified - queue async migration (don't block login)
				if err := s.queueLegacyUserMigration(localUser.ID, req.Email, req.Password, "login"); err != nil {
					s.logger.Error("login: failed to queue migration for legacy user",
						zap.Error(err),
						zap.String("user_id", localUser.ID),
					)
					// Don't fail login if queueing fails - user can try again
				}

				// Return OTP required response
				return &models.AuthResponse{
					User:        localUser,
					OTPRequired: true,
					Message:     "A login code has been sent to your email.",
				}, nil
			}

			// Not a legacy user, just invalid credentials
			return nil, ErrInvalidCredentials
		} else if errors.Is(err, sharedauth.ErrAuthRateLimit) {
			// Rate limit error
			return nil, ErrRateLimit
		} else {
			// Other auth errors
			s.logger.Error("login: auth client returned error",
				zap.Error(err),
				zap.String("error_type", fmt.Sprintf("%T", err)),
			)
			return nil, fmt.Errorf("authenticate: %w", err)
		}
	}

	// Successful Supabase login - check if password_hash needs clearing
	if localUser != nil {
		// Check if password_hash exists (indicates user was migrated but hash not cleared)
		passwordHash, hashErr := s.userRepo.GetPasswordHash(req.Email)
		if hashErr == nil && passwordHash != "" {
			// Clear password_hash since user is now fully migrated
			if clearErr := s.userRepo.ClearPasswordHash(localUser.ID); clearErr != nil {
				s.logger.Error("login: failed to clear password hash after successful Supabase login",
					zap.Error(clearErr),
					zap.String("user_id", localUser.ID),
				)
				// Don't fail login - just log the error
			} else {
				s.logger.Debug("login: cleared password_hash for migrated user",
					zap.String("user_id", localUser.ID),
				)
			}
		}
	}

	// For login source, OTP verification email is queued separately
	otpCode, err := s.authClient.GenerateLoginOTP(ctx, req.Email)
	if err != nil {
		s.logger.Error("login: failed to generate otp", zap.Error(err))
		return nil, fmt.Errorf("failed to generate login verification code")
	}

	if err := s.emailSvc.SendLoginOTPEmail(ctx, req.Email, otpCode); err != nil {
		s.logger.Error("login: failed to send otp email", zap.Error(err))
		return nil, fmt.Errorf("failed to send login verification email")
	}

	return &models.AuthResponse{
		Email:       userInfo.Email,
		OTPRequired: true,
		Message:     "A login code has been sent to your email.",
	}, nil
}

// @TODO - We should switch this to resetPassword as forgotPassword should not be resetting the user's account
func (s *AuthService) ForgotPassword(ctx context.Context, req *models.ForgotPasswordRequest) error {
	if err := validation.ValidateForgotPasswordRequest(req); err != nil {
		return err
	}

	user, err := s.userRepo.FindByEmail(req.Email)
	if err != nil && !errors.Is(err, serror.ErrUserNotFound) {
		return fmt.Errorf("find user: %w", err)
	}
	if user == nil {
		return nil // silently succeed — do not leak user existence
	}

	if user.AuthUserID == nil || strings.TrimSpace(*user.AuthUserID) == "" {
		return s.handleLegacyForgotPassword(user)
	}

	return s.issueForgotPasswordReset(ctx, req.Email)
}

func (s *AuthService) handleLegacyForgotPassword(user *models.User) error {
	s.logger.Debug("forgot password: legacy user detected, queuing migration",
		zap.String("user_id", user.ID),
	)

	if err := s.queueLegacyUserMigration(user.ID, user.Email, generateSecurePassword(), "forgot_password"); err != nil {
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
	accessToken, userInfo, err := s.authClient.VerifyEmailWithOTP(ctx, req.Email, req.Token)
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

	user, err := s.resolveVerifiedUser(userInfo.Sub, req.Email)
	if err != nil {
		return nil, err
	}

	wasAlreadyVerified := user.EmailVerified

	if err := s.userRepo.UpdateEmailVerified(user.ID, true); err != nil {
		return nil, fmt.Errorf("update verification status: %w", err)
	}
	if err := s.userRepo.UpdateLastLogin(user.ID); err != nil {
		s.logger.Warn("verify email: failed to update last login", zap.Error(err))
	}
	user.EmailVerified = true

	s.logger.Info("verify email: email verified", zap.String("user_id", user.ID))

	if !wasAlreadyVerified {
		s.sendWelcomeEmailAsync(user)
	}

	return &models.AuthResponse{
		Token:   accessToken,
		User:    user,
		Message: "Email verified and logged in.",
	}, nil
}

// resolveVerifiedUser looks up a user by Supabase subject (preferred) and
// falls back to email lookup for accounts registered before auth_user_id was
// populated.
func (s *AuthService) resolveVerifiedUser(sub, email string) (*models.User, error) {
	user, err := s.userRepo.FindByAuthUserID(sub)
	if err != nil && !errors.Is(err, serror.ErrUserNotFound) {
		return nil, fmt.Errorf("find user by auth ID: %w", err)
	}
	if user != nil {
		return user, nil
	}

	user, err = s.userRepo.FindByEmail(email)
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

func (s *AuthService) queueVerificationEmail(userID, email string) error {
	email = normalizeEmail(email)

	inflight, err := s.outboxRepo.ExistsPendingByReference(models.EventTypeSendVerificationEmail, userID)
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

	if err := s.outboxRepo.Create(&models.OutboxEvent{
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

	user, err := s.userRepo.FindByEmail(email)
	if err != nil {
		if errors.Is(err, serror.ErrUserNotFound) {
			return nil
		}
		return fmt.Errorf("find user: %w", err)
	}

	if user.EmailVerified {
		return nil
	}

	alreadyQueued, err := s.outboxRepo.ExistsByReference(models.EventTypeSendVerificationEmail, user.ID)
	if err != nil {
		return fmt.Errorf("check existing verification email event: %w", err)
	}
	if alreadyQueued {
		return nil
	}

	if err := s.queueVerificationEmail(user.ID, email); err != nil {
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
