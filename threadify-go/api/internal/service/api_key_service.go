package service

import (
	"context"
	"threadify-go/api/internal/domain"
	"threadify-go/api/internal/utils"
	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/rbac"
	"time"

	"go.uber.org/zap"
)

const defaultServiceAccountRole = "standard_service"

type APIKeyService struct {
	apiKeyRepo         domain.APIKeyRepository
	serviceAccountRepo domain.ServiceAccountRepository
	userRoleRepo       domain.UserRoleRepository
	rbacLoader         *rbac.Loader
	logger             *zap.Logger
}

func NewAPIKeyService(
	apiKeyRepo domain.APIKeyRepository,
	serviceAccountRepo domain.ServiceAccountRepository,
	userRoleRepo domain.UserRoleRepository,
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

func (s *APIKeyService) CreateAPIKey(
	ctx context.Context,
	userID string,
	companyID string,
	req *domain.CreateAPIKeyCmd,
) (*domain.APIKeyCredentials, error) {
	if req.Name == "" {
		return nil, ErrApiKeyNameRequired
	}

	serviceAccountID, err := s.resolveServiceAccount(ctx, userID, companyID, req)
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

	var ownerUserID *string
	if serviceAccountID == nil {
		ownerUserID = &userID
	}

	keyHash := utils.HashAPIKey(key)
	keyPrefix := utils.GetKeyPrefix(key)

	apiKey := &domain.APIKey{
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

	if err := s.apiKeyRepo.Create(ctx, apiKey); err != nil {
		return nil, err
	}
	return &domain.APIKeyCredentials{
		Key:       key,
		KeyPrefix: keyPrefix,
		APIKey:    apiKey,
	}, nil
}

func (s *APIKeyService) resolveServiceAccount(
	ctx context.Context,
	userID string,
	companyID string,
	req *domain.CreateAPIKeyCmd,
) (*string, error) {
	if req.ServiceAccountID != nil {
		sa, err := s.serviceAccountRepo.FindByID(ctx, *req.ServiceAccountID)
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

	sa := &domain.ServiceAccount{
		ID:        utils.GenerateID(),
		CompanyID: companyID,
		Name:      name,
		IsActive:  true,
		CreatedBy: &userID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.serviceAccountRepo.Create(ctx, sa); err != nil {
		return nil, err
	}

	if err := s.userRoleRepo.AssignRoleToServiceAccount(ctx, sa.ID, role, userID); err != nil {
		return nil, ErrFailedToAssignRole
	}
	return &sa.ID, nil
}

func (s *APIKeyService) ListAPIKeys(ctx context.Context, companyID string) ([]*domain.APIKey, error) {
	return s.apiKeyRepo.FindByCompanyID(ctx, companyID)
}

func (s *APIKeyService) RevokeAPIKey(ctx context.Context, keyID, companyID string) error {
	key, err := s.apiKeyRepo.FindByID(ctx, keyID)
	if err != nil {
		return err
	}
	if key == nil {
		return ErrApiKeyNotFound
	}
	if key.CompanyID != companyID {
		return ErrUnauthorized
	}
	return s.apiKeyRepo.Revoke(ctx, keyID)
}

func (s *APIKeyService) ValidateAPIKey(ctx context.Context, key string) (*domain.APIKey, error) {
	apiKey, err := s.apiKeyRepo.FindByHash(ctx, utils.HashAPIKey(key))
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

	if apiKey.UserID != nil && apiKey.ServiceAccountID == nil {
		if err := sharedauth.RequireActiveUser(ctx, *apiKey.UserID, apiKey.CompanyID); err != nil {
			return nil, ErrInvalidApiKey
		}
	}
	// TODO: Move last_used_at tracking to NATS for async processing
	// Currently commented out - spawns unbounded goroutines and data not actively used
	// go s.apiKeyRepo.UpdateLastUsed(apiKey.ID)

	return apiKey, nil
}
