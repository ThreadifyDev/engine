package service

import (
	"context"
	"time"

	"threadify-go/api/internal/domain"
	"threadify-go/api/internal/ports"
	"threadify-go/api/internal/utils"

	"go.uber.org/zap"
)

const defaultServiceAccountRole = "standard_service"

type APIKeyService struct {
	apiKeyRepo         domain.APIKeyRepository
	serviceAccountRepo domain.ServiceAccountRepository
	userRoleRepo       domain.UserRoleRepository
	rbacLoader         ports.RBACRoleLoader
	logger             *zap.Logger
}

func NewAPIKeyService(
	apiKeyRepo domain.APIKeyRepository,
	serviceAccountRepo domain.ServiceAccountRepository,
	userRoleRepo domain.UserRoleRepository,
	rbacLoader ports.RBACRoleLoader,
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
		s.logger.Error("create api key: failed to generate key",
			zap.String("company_id", companyID),
			zap.Error(err),
		)
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
		s.logger.Error("create api key: failed to persist",
			zap.String("company_id", companyID),
			zap.String("name", req.Name),
			zap.Error(err),
		)
		return nil, err
	}

	s.logger.Info("create api key: key created",
		zap.String("key_id", apiKey.ID),
		zap.String("key_prefix", keyPrefix),
		zap.String("company_id", companyID),
		zap.String("name", req.Name),
	)

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
			s.logger.Warn("resolve service account: not found",
				zap.String("service_account_id", *req.ServiceAccountID),
				zap.Error(err),
			)
			return nil, ErrServiceAccountNotFound
		}
		if sa.CompanyID != companyID {
			s.logger.Warn("resolve service account: company mismatch",
				zap.String("service_account_id", *req.ServiceAccountID),
				zap.String("expected_company", companyID),
				zap.String("actual_company", sa.CompanyID),
			)
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
			s.logger.Warn("resolve service account: invalid role",
				zap.String("role", role),
				zap.String("company_id", companyID),
			)
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
		s.logger.Error("resolve service account: failed to create service account",
			zap.String("company_id", companyID),
			zap.String("name", name),
			zap.Error(err),
		)
		return nil, err
	}

	if err := s.userRoleRepo.AssignRoleToServiceAccount(ctx, sa.ID, role, userID); err != nil {
		s.logger.Error("resolve service account: failed to assign role",
			zap.String("service_account_id", sa.ID),
			zap.String("role", role),
			zap.Error(err),
		)
		return nil, ErrFailedToAssignRole
	}

	s.logger.Info("resolve service account: service account created",
		zap.String("service_account_id", sa.ID),
		zap.String("company_id", companyID),
		zap.String("role", role),
	)

	return &sa.ID, nil
}

func (s *APIKeyService) ListAPIKeys(ctx context.Context, companyID string) ([]*domain.APIKey, error) {
	keys, err := s.apiKeyRepo.FindByCompanyID(ctx, companyID)
	if err != nil {
		s.logger.Error("list api keys: failed to fetch keys",
			zap.String("company_id", companyID),
			zap.Error(err),
		)
		return nil, err
	}
	return keys, nil
}

func (s *APIKeyService) RevokeAPIKey(ctx context.Context, keyID, companyID string) error {
	key, err := s.apiKeyRepo.FindByID(ctx, keyID)
	if err != nil {
		s.logger.Error("revoke api key: failed to fetch key",
			zap.String("key_id", keyID),
			zap.Error(err),
		)
		return err
	}
	if key == nil {
		return ErrApiKeyNotFound
	}
	if key.CompanyID != companyID {
		s.logger.Warn("revoke api key: company mismatch",
			zap.String("key_id", keyID),
			zap.String("expected_company", companyID),
			zap.String("actual_company", key.CompanyID),
		)
		return ErrUnauthorized
	}

	if err := s.apiKeyRepo.Revoke(ctx, keyID); err != nil {
		s.logger.Error("revoke api key: failed to revoke",
			zap.String("key_id", keyID),
			zap.Error(err),
		)
		return err
	}

	s.logger.Info("revoke api key: key revoked",
		zap.String("key_id", keyID),
		zap.String("company_id", companyID),
	)

	return nil
}

func (s *APIKeyService) ValidateAPIKey(ctx context.Context, key string) (*domain.APIKey, error) {
	keyHash := utils.HashAPIKey(key)

	apiKey, err := s.apiKeyRepo.FindByHash(ctx, keyHash)
	if err != nil {
		s.logger.Error("validate api key: failed to fetch key", zap.Error(err))
		return nil, err
	}
	if apiKey == nil {
		return nil, ErrInvalidApiKey
	}
	if apiKey.RevokedAt != nil {
		s.logger.Warn("validate api key: key revoked", zap.String("key_id", apiKey.ID))
		return nil, ErrApiKeyRevoked
	}
	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
		s.logger.Warn("validate api key: key expired",
			zap.String("key_id", apiKey.ID),
			zap.Time("expires_at", *apiKey.ExpiresAt),
		)
		return nil, ErrApiKeyExpiredAPI
	}

	return apiKey, nil
}
