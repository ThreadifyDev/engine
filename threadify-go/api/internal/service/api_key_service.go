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
	if req.Name == "" {
		return nil, ErrApiKeyNameRequired
	}

	serviceAccountID, err := s.resolveServiceAccount(userID, companyID, req)
	if err != nil {
		return nil, err
	}

	key, err := utils.GenerateAPIKey()
	if err != nil {
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

	if err := s.apiKeyRepo.Create(apiKey); err != nil {
		return nil, err
	}
	return &CreateAPIKeyResponse{
		Key:       key,
		KeyPrefix: keyPrefix,
		APIKey:    apiKey,
	}, nil
}

// resolveServiceAccount returns the service account ID to associate with the
// new API key, creating one if necessary.
func (s *APIKeyService) resolveServiceAccount(userID, companyID string, req *CreateAPIKeyRequest) (*string, error) {
	if req.ServiceAccountID != nil {
		sa, err := s.serviceAccountRepo.FindByID(*req.ServiceAccountID)
		if err != nil || sa == nil {
			return nil, ErrServiceAccountNotFound
		}
		if sa.CompanyID != companyID {
			return nil, ErrUnauthorizedCompany
		}
		return req.ServiceAccountID, nil
	}

	role := defaultServiceAccountRole
	if req.CreateServiceAccount && req.ServiceAccountRole != nil {
		role = *req.ServiceAccountRole
	}

	if req.CreateServiceAccount {
		apiLevelRoles := s.rbacLoader.GetRolesByLevel("api_level")
		if _, exists := apiLevelRoles[role]; !exists {
			return nil, ErrInvalidServiceAccountRole
		}
	}

	name := req.Name
	if !req.CreateServiceAccount {
		name += " (auto-generated)"
	}

	sa := &models.ServiceAccount{
		ID:        utils.GenerateID(),
		CompanyID: companyID,
		Name:      name,
		IsActive:  true,
		CreatedBy: &userID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.serviceAccountRepo.Create(sa); err != nil {
		return nil, err
	}

	if err := s.userRoleRepo.AssignRoleToServiceAccount(sa.ID, role, userID); err != nil {
		return nil, ErrFailedToAssignRole
	}
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
