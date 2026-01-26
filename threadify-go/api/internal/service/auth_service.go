package service

import (
	"database/sql"
	"errors"
	"fmt"
	"threadify-go/api/internal/models"
	"threadify-go/api/internal/repository"
	"threadify-go/api/internal/utils"
	"threadify-go/shared/jwt"
	"time"
)

type AuthService struct {
	userRepo      *repository.UserRepository
	companyRepo   *repository.CompanyRepository
	otpRepo       *repository.OTPRepository
	userRoleRepo  *repository.UserRoleRepository
	emailSvc      *EmailService
	jwtValidator  *jwt.Validator
	jwtExpiration time.Duration
}

func NewAuthService(
	db *sql.DB,
	emailSvc *EmailService,
	jwtValidator *jwt.Validator,
	jwtExpiration time.Duration,
) *AuthService {
	return &AuthService{
		userRepo:      repository.NewUserRepository(db),
		companyRepo:   repository.NewCompanyRepository(db),
		otpRepo:       repository.NewOTPRepository(db),
		userRoleRepo:  repository.NewUserRoleRepository(db),
		emailSvc:      emailSvc,
		jwtValidator:  jwtValidator,
		jwtExpiration: jwtExpiration,
	}
}

// Signup creates a new company and user, sends OTP
func (s *AuthService) Signup(req *models.SignupRequest) error {
	// Check if user already exists
	existingUser, err := s.userRepo.FindByEmail(req.Email)
	if err != nil {
		return fmt.Errorf("failed to check existing user: %w", err)
	}
	if existingUser != nil {
		return errors.New("user with this email already exists")
	}

	// Hash password
	passwordHash, err := utils.HashPassword(req.Password)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Create company
	company := &models.Company{
		ID:        utils.GenerateID(),
		Name:      req.CompanyName,
		Industry:  req.Industry,
		Size:      req.CompanySize,
		UseCase:   req.UseCase,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.companyRepo.Create(company); err != nil {
		return fmt.Errorf("failed to create company: %w", err)
	}

	// Create user (full_name and job_role are optional)
	var fullName *string
	var jobRole *string
	if req.FullName != "" {
		fullName = &req.FullName
	}
	if req.JobRole != "" {
		jobRole = &req.JobRole
	}

	user := &models.User{
		ID:                       utils.GenerateID(),
		CompanyID:                company.ID,
		Email:                    req.Email,
		PasswordHash:             passwordHash,
		FullName:                 fullName,
		JobRole:                  jobRole,
		EmailVerified:            false,
		OnboardingCompleted:      false,
		FirstInstrumentationDone: false,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}

	if err := s.userRepo.Create(user); err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	// Assign default app-level role to new user
	defaultRole := "standard_account" // Standard account role for new users
	if err := s.userRoleRepo.AssignRoleToUser(user.ID, defaultRole, "system"); err != nil {
		// Log error but don't fail signup - user can be assigned role later
		fmt.Printf("Warning: failed to assign default role to user %s: %v\n", user.ID, err)
	}

	// Generate and send OTP
	otpCode, err := utils.GenerateOTP()
	if err != nil {
		return fmt.Errorf("failed to generate OTP: %w", err)
	}

	// Hash the OTP code before storing
	otpHash, err := utils.HashOTP(otpCode)
	if err != nil {
		return fmt.Errorf("failed to hash OTP: %w", err)
	}

	otp := &models.OTPCode{
		ID:        utils.GenerateID(),
		Email:     req.Email,
		Code:      otpHash, // Store hashed code
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Verified:  false,
		CreatedAt: time.Now(),
	}

	if err := s.otpRepo.Create(otp); err != nil {
		return fmt.Errorf("failed to create OTP: %w", err)
	}

	// Send OTP email (send plain code, not hash)
	if err := s.emailSvc.SendOTP(req.Email, otpCode); err != nil {
		return fmt.Errorf("failed to send OTP email: %w", err)
	}

	return nil
}

// Login authenticates user and sends OTP
func (s *AuthService) Login(req *models.LoginRequest) error {
	// Find user
	user, err := s.userRepo.FindByEmail(req.Email)
	if err != nil {
		return fmt.Errorf("failed to find user: %w", err)
	}
	if user == nil {
		return errors.New("invalid email or password")
	}

	// Check password
	if !utils.CheckPassword(req.Password, user.PasswordHash) {
		return errors.New("invalid email or password")
	}

	// Generate and send OTP
	otpCode, err := utils.GenerateOTP()
	if err != nil {
		return fmt.Errorf("failed to generate OTP: %w", err)
	}

	// Hash the OTP code before storing
	otpHash, err := utils.HashOTP(otpCode)
	if err != nil {
		return fmt.Errorf("failed to hash OTP: %w", err)
	}

	otp := &models.OTPCode{
		ID:        utils.GenerateID(),
		Email:     req.Email,
		Code:      otpHash, // Store hashed code
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Verified:  false,
		CreatedAt: time.Now(),
	}

	if err := s.otpRepo.Create(otp); err != nil {
		return fmt.Errorf("failed to create OTP: %w", err)
	}

	// Send OTP email (send plain code, not hash)
	if err := s.emailSvc.SendOTP(req.Email, otpCode); err != nil {
		return fmt.Errorf("failed to send OTP email: %w", err)
	}

	return nil
}

// VerifyOTP verifies OTP and returns JWT token
func (s *AuthService) VerifyOTP(req *models.VerifyOTPRequest) (*models.AuthResponse, error) {
	// Find latest valid OTP for this email
	otp, err := s.otpRepo.FindLatestValidOTP(req.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to find OTP: %w", err)
	}
	if otp == nil {
		return nil, errors.New("invalid or expired OTP code")
	}

	// Verify the OTP code against the hash
	if !utils.CheckOTP(req.Code, otp.Code) {
		return nil, errors.New("invalid or expired OTP code")
	}

	// Mark OTP as verified
	if err := s.otpRepo.MarkAsVerified(otp.ID); err != nil {
		return nil, fmt.Errorf("failed to mark OTP as verified: %w", err)
	}

	// Get user
	user, err := s.userRepo.FindByEmail(req.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to find user: %w", err)
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	// Update email verified status (if first time)
	if !user.EmailVerified {
		if err := s.userRepo.UpdateEmailVerified(user.ID, true); err != nil {
			return nil, fmt.Errorf("failed to update email verified: %w", err)
		}
		user.EmailVerified = true

		// Send welcome email
		fullName := "there"
		if user.FullName != nil {
			fullName = *user.FullName
		}
		go s.emailSvc.SendWelcomeEmail(user.Email, fullName)
	}

	// Update last login
	if err := s.userRepo.UpdateLastLogin(user.ID); err != nil {
		return nil, fmt.Errorf("failed to update last login: %w", err)
	}

	// Load user roles from DB
	roleNames, err := s.userRoleRepo.GetUserRoles(user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load user roles: %w", err)
	}

	// Default to standard role if no roles assigned
	if len(roleNames) == 0 {
		roleNames = []string{"standard_account"}
	}

	// Generate JWT token with roles
	token, err := s.jwtValidator.CreateToken(
		user.ID,
		user.CompanyID,
		user.Email,
		roleNames,
		s.jwtExpiration,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}

	return &models.AuthResponse{
		Token: token,
		User:  user,
	}, nil
}
