package service

import (
	"threadify-go/api/internal/models"
	"threadify-go/api/internal/repository"
	"threadify-go/api/internal/utils"
	"threadify-go/shared/rbac"
	"time"

	"go.uber.org/zap"
)

const defaultServiceAccountRole = "standard_service"

type APIKeyService struct {
	apiKeyRepo         *repository.APIKeyRepository
	serviceAccountRepo *repository.ServiceAccountRepository
	userRoleRepo       *repository.UserRoleRepository
	rbacLoader         *rbac.Loader
	logger             *zap.Logger
}

func NewAPIKeyService(
	apiKeyRepo *repository.APIKeyRepository,
	serviceAccountRepo *repository.ServiceAccountRepository,
	userRoleRepo *repository.UserRoleRepository,
	rbacLoader *rbac.Loader,
	logger *zap.Logger,
) *APIKeyService {
	return &APIKeyService{
		apiKeyRepo:         apiKeyRepo,
		serviceAccountRepo: serviceAccountRepo,
		userRoleRepo:       userRoleRepo,
		rbacLoader:         rbacLoader,
		logger:             logger,
	}
}

type CreateAPIKeyRequest struct {
	Name                 string  `json:"name" binding:"required"`
	ExpiresIn            *int    `json:"expires_in"`
	ServiceAccountID     *string `json:"service_account_id"`
	CreateServiceAccount bool    `json:"create_service_account"`
	ServiceAccountRole   *string `json:"service_account_role"`
}

type CreateAPIKeyResponse struct {
	Key       string         `json:"key"`
	KeyPrefix string         `json:"key_prefix"`
	APIKey    *models.APIKey `json:"api_key"`
}

func (s *APIKeyService) CreateAPIKey(userID, companyID string, req *CreateAPIKeyRequest) (*CreateAPIKeyResponse, error) {
	s.logger.Info("CreateAPIKey called",
		zap.String("userID", userID),
		zap.String("companyID", companyID),
		zap.String("name", req.Name))

	if req.Name == "" {
		s.logger.Error("API key name is required")
		return nil, ErrApiKeyNameRequired
	}

	s.logger.Info("Resolving service account")
	serviceAccountID, err := s.resolveServiceAccount(userID, companyID, req)
	if err != nil {
		s.logger.Error("Failed to resolve service account", zap.Error(err))
		return nil, err
	}
	s.logger.Info("Service account resolved", zap.Stringp("serviceAccountID", serviceAccountID))

	s.logger.Info("Generating API key")
	key, err := utils.GenerateAPIKey()
	if err != nil {
		s.logger.Error("Failed to generate API key", zap.Error(err))
		return nil, err
	}

	var expiresAt *time.Time
	if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
		expiry := time.Now().AddDate(0, 0, *req.ExpiresIn)
		expiresAt = &expiry
	}

	// DB constraint chk_api_key_owner requires exactly one of user_id /
	// service_account_id to be non-null.
	var ownerUserID *string
	if serviceAccountID == nil {
		ownerUserID = &userID
	}

	keyHash := utils.HashAPIKey(key)
	keyPrefix := utils.GetKeyPrefix(key)

	apiKey := &models.APIKey{
		ID:               utils.GenerateID(),
		KeyHash:          keyHash,
		KeyPrefix:        keyPrefix,
		Name:             req.Name,
		UserID:           ownerUserID,
		ServiceAccountID: serviceAccountID,
		CompanyID:        companyID,
		ExpiresAt:        expiresAt,
		CreatedAt:        time.Now(),
	}

	s.logger.Info("Creating API key in database", zap.String("apiKeyID", apiKey.ID))
	if err := s.apiKeyRepo.Create(apiKey); err != nil {
		s.logger.Error("Failed to create API key in database", zap.Error(err), zap.String("apiKeyID", apiKey.ID))
		return nil, err
	}

	s.logger.Info("API key created successfully", zap.String("apiKeyID", apiKey.ID))
	return &CreateAPIKeyResponse{
		Key:       key,
		KeyPrefix: keyPrefix,
		APIKey:    apiKey,
	}, nil
}

// resolveServiceAccount returns the service account ID to associate with the
// new API key, creating one if necessary.
func (s *APIKeyService) resolveServiceAccount(userID, companyID string, req *CreateAPIKeyRequest) (*string, error) {
	s.logger.Info("Resolving service account",
		zap.Stringp("serviceAccountID", req.ServiceAccountID),
		zap.Bool("createServiceAccount", req.CreateServiceAccount))

	if req.ServiceAccountID != nil {
		s.logger.Info("Using existing service account", zap.String("serviceAccountID", *req.ServiceAccountID))
		sa, err := s.serviceAccountRepo.FindByID(*req.ServiceAccountID)
		if err != nil || sa == nil {
			s.logger.Error("Service account not found", zap.Error(err), zap.Stringp("serviceAccountID", req.ServiceAccountID))
			return nil, ErrServiceAccountNotFound
		}
		if sa.CompanyID != companyID {
			s.logger.Error("Service account company mismatch",
				zap.String("expectedCompanyID", companyID),
				zap.String("actualCompanyID", sa.CompanyID))
			return nil, ErrUnauthorizedCompany
		}
		return req.ServiceAccountID, nil
	}

	role := defaultServiceAccountRole
	if req.CreateServiceAccount && req.ServiceAccountRole != nil {
		role = *req.ServiceAccountRole
	}
	s.logger.Info("Using role", zap.String("role", role))

	if req.CreateServiceAccount {
		s.logger.Info("Validating role against RBAC")
		apiLevelRoles := s.rbacLoader.GetRolesByLevel("api_level")
		s.logger.Info("Available api_level roles", zap.Any("roles", apiLevelRoles))
		if _, exists := apiLevelRoles[role]; !exists {
			s.logger.Error("Invalid role not found in api_level roles", zap.String("role", role))
			return nil, ErrInvalidServiceAccountRole
		}
	}

	name := req.Name
	if !req.CreateServiceAccount {
		name += " (auto-generated)"
	}
	s.logger.Info("Creating service account", zap.String("name", name))

	sa := &models.ServiceAccount{
		ID:        utils.GenerateID(),
		CompanyID: companyID,
		Name:      name,
		IsActive:  true,
		CreatedBy: &userID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	s.logger.Info("Inserting service account into database", zap.String("serviceAccountID", sa.ID))
	if err := s.serviceAccountRepo.Create(sa); err != nil {
		s.logger.Error("Failed to create service account in database", zap.Error(err), zap.String("serviceAccountID", sa.ID))
		return nil, err
	}

	s.logger.Info("Assigning role to service account", zap.String("role", role), zap.String("serviceAccountID", sa.ID))
	if err := s.userRoleRepo.AssignRoleToServiceAccount(sa.ID, role, userID); err != nil {
		s.logger.Error("Failed to assign role to service account", zap.Error(err), zap.String("role", role), zap.String("serviceAccountID", sa.ID))
		return nil, ErrFailedToAssignRole
	}

	s.logger.Info("Service account created successfully", zap.String("serviceAccountID", sa.ID))
	return &sa.ID, nil
}

func (s *APIKeyService) ListAPIKeys(companyID string) ([]*models.APIKey, error) {
	return s.apiKeyRepo.FindByCompanyID(companyID)
}

func (s *APIKeyService) RevokeAPIKey(keyID, companyID string) error {
	key, err := s.apiKeyRepo.FindByID(keyID)
	if err != nil {
		return err
	}
	if key == nil {
		return ErrApiKeyNotFound
	}
	if key.CompanyID != companyID {
		return ErrUnauthorized
	}
	return s.apiKeyRepo.Revoke(keyID)
}

func (s *APIKeyService) ValidateAPIKey(key string) (*models.APIKey, error) {
	apiKey, err := s.apiKeyRepo.FindByHash(utils.HashAPIKey(key))
	if err != nil {
		return nil, err
	}
	if apiKey == nil {
		return nil, ErrInvalidApiKey
	}
	if apiKey.RevokedAt != nil {
		return nil, ErrApiKeyRevoked
	}
	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
		return nil, ErrApiKeyExpiredAPI
	}

	// TODO: Move last_used_at tracking to NATS for async processing
	// Currently commented out - spawns unbounded goroutines and data not actively used
	// go s.apiKeyRepo.UpdateLastUsed(apiKey.ID)

	return apiKey, nil
}
