package service

import (
	"errors"
	"threadify-go/api/internal/models"
	"threadify-go/api/internal/repository"
	"threadify-go/api/internal/utils"
	"threadify-go/shared/rbac"
	"time"
)

type APIKeyService struct {
	apiKeyRepo         *repository.APIKeyRepository
	serviceAccountRepo *repository.ServiceAccountRepository
	userRoleRepo       *repository.UserRoleRepository
	rbacLoader         *rbac.Loader
}

func NewAPIKeyService(
	apiKeyRepo *repository.APIKeyRepository,
	serviceAccountRepo *repository.ServiceAccountRepository,
	userRoleRepo *repository.UserRoleRepository,
	rbacLoader *rbac.Loader,
) *APIKeyService {
	return &APIKeyService{
		apiKeyRepo:         apiKeyRepo,
		serviceAccountRepo: serviceAccountRepo,
		userRoleRepo:       userRoleRepo,
		rbacLoader:         rbacLoader,
	}
}

type CreateAPIKeyRequest struct {
	Name                 string  `json:"name" binding:"required"`
	ExpiresIn            *int    `json:"expires_in"`             // days, nil = never expires
	ServiceAccountID     *string `json:"service_account_id"`     // Optional: link to existing service account
	CreateServiceAccount bool    `json:"create_service_account"` // Auto-create service account
	ServiceAccountRole   *string `json:"service_account_role"`   // Role if creating new service account (e.g., "owner", "developer")
}

type CreateAPIKeyResponse struct {
	Key       string         `json:"key"`        // Only returned once!
	KeyPrefix string         `json:"key_prefix"` // For display
	APIKey    *models.APIKey `json:"api_key"`
}

func (s *APIKeyService) CreateAPIKey(userID, companyID string, req *CreateAPIKeyRequest) (*CreateAPIKeyResponse, error) {
	// Validate name
	if req.Name == "" {
		return nil, errors.New("API key name is required")
	}

	var serviceAccountID *string

	// Option 1: Create new service account
	if req.CreateServiceAccount {
		role := "standard_service" // Default role
		if req.ServiceAccountRole != nil {
			role = *req.ServiceAccountRole
		}

		// Validate role
		// Validate role exists in api_level (service account roles)
		apiLevelRoles := s.rbacLoader.GetRolesByLevel("api_level")
		if _, exists := apiLevelRoles[role]; !exists {
			return nil, errors.New("invalid service account role: must be from api_level")
		}

		sa := &models.ServiceAccount{
			ID:        utils.GenerateID(),
			CompanyID: companyID,
			Name:      req.Name, // Use same name as API key
			IsActive:  true,
			CreatedBy: &userID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := s.serviceAccountRepo.Create(sa); err != nil {
			return nil, err
		}

		// Assign role to service account
		if err := s.userRoleRepo.AssignRoleToServiceAccount(sa.ID, role, userID); err != nil {
			return nil, errors.New("failed to assign role to service account")
		}

		serviceAccountID = &sa.ID
	} else if req.ServiceAccountID != nil {
		// Option 2: Link to existing service account
		// Verify service account exists and belongs to company
		sa, err := s.serviceAccountRepo.FindByID(*req.ServiceAccountID)
		if err != nil || sa == nil {
			return nil, errors.New("service account not found")
		}
		if sa.CompanyID != companyID {
			return nil, errors.New("unauthorized: service account belongs to different company")
		}
		serviceAccountID = req.ServiceAccountID
	}
	// Option 3: No service account (legacy - link to user)

	// Generate API key
	key, err := utils.GenerateAPIKey()
	if err != nil {
		return nil, err
	}

	// Hash the key for storage
	keyHash := utils.HashAPIKey(key)
	keyPrefix := utils.GetKeyPrefix(key)

	// Calculate expiration
	var expiresAt *time.Time
	if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
		expiry := time.Now().AddDate(0, 0, *req.ExpiresIn)
		expiresAt = &expiry
	}

	// Create API key record
	apiKey := &models.APIKey{
		ID:               utils.GenerateID(),
		KeyHash:          keyHash,
		KeyPrefix:        keyPrefix,
		Name:             req.Name,
		UserID:           nil, // Don't link to user if service account is used
		ServiceAccountID: serviceAccountID,
		CompanyID:        companyID,
		ExpiresAt:        expiresAt,
		CreatedAt:        time.Now(),
	}

	// If no service account, link to user (legacy behavior)
	if serviceAccountID == nil {
		apiKey.UserID = &userID
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

func (s *APIKeyService) ListAPIKeys(companyID string) ([]*models.APIKey, error) {
	return s.apiKeyRepo.FindByCompanyID(companyID)
}

func (s *APIKeyService) RevokeAPIKey(keyID, companyID string) error {
	// Verify the key belongs to the company
	key, err := s.apiKeyRepo.FindByID(keyID)
	if err != nil {
		return err
	}
	if key == nil {
		return errors.New("API key not found")
	}
	if key.CompanyID != companyID {
		return errors.New("unauthorized")
	}

	return s.apiKeyRepo.Revoke(keyID)
}

func (s *APIKeyService) ValidateAPIKey(key string) (*models.APIKey, error) {
	keyHash := utils.HashAPIKey(key)
	apiKey, err := s.apiKeyRepo.FindByHash(keyHash)
	if err != nil {
		return nil, err
	}
	if apiKey == nil {
		return nil, errors.New("invalid API key")
	}

	// Check if revoked
	if apiKey.RevokedAt != nil {
		return nil, errors.New("API key has been revoked")
	}

	// Check if expired
	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("API key has expired")
	}

	// Update last used timestamp (async, don't wait)
	go s.apiKeyRepo.UpdateLastUsed(apiKey.ID)

	return apiKey, nil
}
